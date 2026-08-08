<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { AlertTriangle, ArrowRight, CheckCircle2, ChevronDown, ChevronUp, Info, RefreshCw, Route, ShieldAlert, ShieldCheck, Sparkles, Stethoscope, Workflow, Zap } from 'lucide-vue-next'

import * as clusterAPI from '../api/clusters'
import { getPostureReport } from '../api/optimization'
import { getInsightRunbook } from '../api/insight'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useCountUp } from '../composables/useCountUp'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { PostureDomain, PostureDomainStatus, PostureFinding, PostureReport } from '../types/optimization'
import type { InsightRunbook } from '../types/insight'

// M80 aggregated governance posture view.
//
// Renders the unified posture report across every M61-M78 analyzer: a headline
// severity histogram, per-domain rollup cards, and the risk-sorted finding
// stream (critical first). Wholely read-only (ADR 0004).

const auth = useAuthStore()

const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const clustersLoading = ref(true)
const clustersError = ref('')

const report = ref<PostureReport | null>(null)
const loading = ref(false)
const error = ref('')

const domainMeta: Record<PostureDomain, { label: string }> = {
  cis: { label: 'CIS 合规' },
  finops: { label: '成本优化' },
  deprecated_api: { label: '废弃 API' },
  network: { label: '网络连通' },
  image: { label: '镜像供应链' },
  gitops: { label: 'GitOps 漂移' },
  capacity: { label: '容量预测' },
  policy: { label: '策略合规' },
  hpa: { label: 'HPA 扩缩容' },
  pdb: { label: 'PDB 保护' },
  ingress: { label: 'Ingress 暴露面' },
}

const severityLabels: Record<string, string> = { critical: '严重', warning: '警告', info: '提示' }

function severityLabel(severity: string): string {
  return severityLabels[severity] ?? severity
}

/** Findings already arrive risk-sorted from the API; keep them as-is for display. */
const sortedFindings = computed<PostureFinding[]>(() => report.value?.findings ?? [])

const criticalCount = computed(() => report.value?.by_severity?.critical ?? 0)
const warningCount = computed(() => report.value?.by_severity?.warning ?? 0)
const infoCount = computed(() => report.value?.by_severity?.info ?? 0)
const animatedCritical = useCountUp(0, criticalCount, { duration: 650 })
const animatedWarning = useCountUp(0, warningCount, { duration: 650 })
const animatedInfo = useCountUp(0, infoCount, { duration: 650 })
const animatedFailed = useCountUp(0, computed(() => report.value?.failed_checks ?? 0), { duration: 800 })
const animatedTotal = useCountUp(0, computed(() => report.value?.total_checks ?? 0), { duration: 800 })

/** Domains ordered by failed count desc then critical count, so the worst first. */
const orderedDomains = computed<PostureDomainStatus[]>(() =>
  [...(report.value?.domains ?? [])].sort(
    (a, b) => b.failed - a.failed || severityRank(a, b) || a.domain.localeCompare(b.domain),
  ),
)

function severityRank(a: PostureDomainStatus, b: PostureDomainStatus): number {
  const af = a.by_severity?.critical ?? 0
  const bf = b.by_severity?.critical ?? 0
  return bf - af
}

function domainLabel(domain: PostureDomain): string {
  return domainMeta[domain]?.label ?? domain
}

function domainChecks(domain: PostureDomainStatus): string {
  if (domain.total <= 0) return '未评估'
  return `${domain.passed}/${domain.total} 通过`
}

async function loadClusters() {
  clustersLoading.value = true
  clustersError.value = ''
  try {
    const result = await clusterAPI.listClusters(auth.accessToken)
    clusters.value = result.items
    if (clusters.value.length > 0 && selectedClusterID.value === null) {
      selectedClusterID.value = clusters.value[0].id
    }
  } catch {
    clustersError.value = '集群列表加载失败'
  } finally {
    clustersLoading.value = false
  }
}

async function loadReport() {
  if (selectedClusterID.value === null) return
  loading.value = true
  error.value = ''
  try {
    report.value = await getPostureReport(auth.accessToken ?? '', selectedClusterID.value)
  } catch {
    error.value = '聚合态势报告加载失败，请稍后重试'
    report.value = null
  } finally {
    loading.value = false
  }
}


// M81 closed-loop drill-down: resolve the deterministic runbook for a
// posture finding (diagnosis -> inspection -> AI explanation -> dry-run ops).
const insightMap = ref<Record<string, InsightRunbook>>({})
const insightLoading = ref<Record<string, boolean>>({})
const insightError = ref<Record<string, string>>({})

function findingKey(finding: PostureFinding, index: number): string {
  return [finding.domain, finding.code, finding.resource.kind, finding.resource.namespace ?? '', finding.resource.name, index].join(':')
}

async function toggleInsight(finding: PostureFinding, index: number) {
  const key = findingKey(finding, index)
  if (insightMap.value[key]) {
    delete insightMap.value[key]
    return
  }
  if (selectedClusterID.value === null) return
  insightLoading.value[key] = true
  insightError.value[key] = ''
  try {
    insightMap.value[key] = await getInsightRunbook(auth.accessToken ?? '', {
      clusterId: selectedClusterID.value,
      domain: finding.domain,
      code: finding.code,
      kind: finding.resource.kind,
      namespace: finding.resource.namespace,
      name: finding.resource.name,
    })
  } catch {
    insightError.value[key] = '环路洞察加载失败，请稍后重试'
  } finally {
    delete insightLoading.value[key]
  }
}

function hasInsight(finding: PostureFinding, index: number): boolean {
  return Boolean(insightMap.value[findingKey(finding, index)])
}

function insightOf(finding: PostureFinding, index: number): InsightRunbook | undefined {
  return insightMap.value[findingKey(finding, index)]
}
function refresh() {
  void loadReport()
}

onMounted(async () => {
  await loadClusters()
  await loadReport()
})
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="集群治理态势">
    <template #actions>
      <button type="button" class="icon-button" title="刷新态势" aria-label="刷新态势" :disabled="loading || selectedClusterID === null" @click="refresh">
        <RefreshCw :size="18" :class="{ spinning: loading }" />
      </button>
    </template>

    <section class="page-toolbar">
      <select v-model="selectedClusterID" class="form-select" :disabled="clustersLoading" @change="loadReport">
        <option v-if="clustersLoading" value="">加载集群中…</option>
        <option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option>
      </select>
    </section>

    <p v-if="clustersError" class="notice error">{{ clustersError }}</p>
    <p v-else-if="error" class="notice error">{{ error }}</p>

    <template v-if="report">
      <div class="posture-headline">
        <div class="posture-score">
          <strong>{{ animatedFailed.value }}</strong>
          <span>风险项</span>
        </div>
        <div class="posture-total">
          <strong>{{ animatedTotal.value }}</strong>
          <span>累计检查</span>
        </div>
        <div class="posture-severity-strip">
          <span class="severity-chip critical"><AlertTriangle :size="14" />{{ animatedCritical.value }} 严重</span>
          <span class="severity-chip warning"><AlertTriangle :size="14" />{{ animatedWarning.value }} 警告</span>
          <span class="severity-chip info"><Info :size="14" />{{ animatedInfo.value }} 提示</span>
        </div>
      </div>

      <div class="posture-domains">
        <article v-for="domain in orderedDomains" :key="domain.domain" class="posture-domain-card" :class="{ 'has-issues': domain.failed > 0 }">
          <p class="posture-domain-label">{{ domainLabel(domain.domain) }}</p>
          <p class="posture-domain-checks">{{ domainChecks(domain) }}</p>
          <p class="posture-domain-severity">
            <span v-if="(domain.by_severity?.critical ?? 0) > 0" class="severity-chip critical">{{ domain.by_severity.critical }} 严重</span>
            <span v-if="(domain.by_severity?.warning ?? 0) > 0" class="severity-chip warning">{{ domain.by_severity.warning }} 警告</span>
            <span v-if="(domain.by_severity?.info ?? 0) > 0" class="severity-chip info">{{ domain.by_severity.info }} 提示</span>
            <span v-if="domain.failed === 0 && domain.total > 0" class="severity-chip ok"><CheckCircle2 :size="14" />通过</span>
          </p>
        </article>
      </div>

      <section class="panel posture-findings">
        <header class="posture-findings-header">
          <span class="posture-findings-title"><ShieldAlert :size="16" /> 发现明细</span>
          <span class="posture-findings-hint">{{ sortedFindings.length }} 项</span>
        </header>
        <div v-if="sortedFindings.length === 0" class="posture-empty">
          <ShieldCheck :size="28" />
          <p>未发现风险项，当前集群运行态势良好。</p>
        </div>
        <ul v-else class="posture-finding-list">
          <li v-for="(finding, index) in sortedFindings" :key="`${finding.domain}-${finding.code}-${index}`" class="posture-finding" :class="`posture-severity-${finding.severity}`">
            <span class="posture-finding-severity">{{ severityLabel(finding.severity) }}</span>
            <div class="posture-finding-body">
              <p class="posture-finding-summary">{{ finding.summary }}</p>
              <p class="posture-finding-meta">
                {{ domainLabel(finding.domain) }} · {{ finding.resource.kind }} {{ finding.resource.namespace ? `${finding.resource.namespace}/` : '' }}{{ finding.resource.name }}
              </p>
              <p v-if="finding.code" class="posture-finding-code">{{ finding.code }}</p>
              <button type="button" class="posture-insight-toggle" :disabled="Boolean(insightLoading[findingKey(finding, index)])" @click="toggleInsight(finding, index)">
                <Route :size="14" />
                {{ insightLoading[findingKey(finding, index)] ? '加载中…' : (hasInsight(finding, index) ? '收起闭环' : '查看闭环') }}
                <ChevronUp v-if="hasInsight(finding, index)" :size="14" />
                <ChevronDown v-else :size="14" />
              </button>
            </div>
            <div v-if="insightOf(finding, index)" class="posture-runbook">
              <p v-if="insightError[findingKey(finding, index)]" class="notice error">{{ insightError[findingKey(finding, index)] }}</p>
              <template v-else>
                <div v-for="route in insightOf(finding, index)?.diagnoses" :key="route.resource_kind" class="runbook-step">
                  <span class="runbook-step-icon"><Stethoscope :size="15" /></span>
                  <div class="runbook-step-body">
                    <p class="runbook-step-title">确定性诊断 · {{ route.resource_kind }}</p>
                    <p class="runbook-step-desc">{{ route.summary }}</p>
                    <p class="runbook-step-meta">{{ route.rule_ids.join(' / ') }}</p>
                  </div>
                  <router-link class="runbook-link" to="/diagnoses">去诊断 <ArrowRight :size="13" /></router-link>
                </div>
                <div v-if="insightOf(finding, index)?.inspection.length" class="runbook-step">
                  <span class="runbook-step-icon"><Workflow :size="15" /></span>
                  <div class="runbook-step-body">
                    <p class="runbook-step-title">巡检佐证 · M52</p>
                    <p v-for="rule in insightOf(finding, index)?.inspection" :key="rule.rule_code" class="runbook-step-meta">{{ rule.signal_code }}</p>
                  </div>
                  <router-link class="runbook-link" to="/inspection">去巡检 <ArrowRight :size="13" /></router-link>
                </div>
                <div v-if="insightOf(finding, index)?.ai_explanation" class="runbook-step">
                  <span class="runbook-step-icon"><Sparkles :size="15" /></span>
                  <div class="runbook-step-body">
                    <p class="runbook-step-title">AI 引用解释 · M55</p>
                    <p class="runbook-step-desc">{{ insightOf(finding, index)?.ai_explanation?.summary }}</p>
                  </div>
                  <router-link class="runbook-link" to="/aiops/investigator">AI 调查 <ArrowRight :size="13" /></router-link>
                </div>
                <div v-if="insightOf(finding, index)?.operations.length" class="runbook-step">
                  <span class="runbook-step-icon"><Zap :size="15" /></span>
                  <div class="runbook-step-body">
                    <p class="runbook-step-title">受控操作预览 · M19（dry-run）</p>
                    <p v-for="op in insightOf(finding, index)?.operations" :key="op.action" class="runbook-step-meta">
                      {{ op.action }}{{ op.dry_run_first ? ' · 仅预览' : '' }}
                    </p>
                  </div>
                  <router-link class="runbook-link" to="/diagnoses">操作台 <ArrowRight :size="13" /></router-link>
                </div>
              </template>
            </div>
          </li>
        </ul>
      </section>
    </template>

    <div v-else-if="!loading" class="posture-empty">
      <p>{{ error || '选择集群以查看聚合治理态势' }}</p>
    </div>
    <div v-else class="posture-loading">正在评估集群态势…</div>
  </ConsoleLayout>
</template>

<style scoped>
.posture-headline {
  display: grid;
  grid-template-columns: auto auto 1fr;
  gap: 18px;
  align-items: center;
  margin: 18px 0;
}

.posture-score,
.posture-total {
  display: grid;
  gap: 2px;
  min-width: 120px;
  padding: 14px 20px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}

.posture-score strong,
.posture-total strong {
  font-size: 28px;
  font-weight: 650;
}

.posture-score span,
.posture-total span {
  color: var(--text-secondary);
  font-size: 12px;
}

.posture-severity-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.severity-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 10px;
  font-size: 12px;
  font-weight: 600;
  border-radius: var(--radius-full);
}

.severity-chip.critical { color: var(--status-danger); background: var(--danger-bg); }
.severity-chip.warning { color: var(--status-warning); background: var(--warning-bg); }
.severity-chip.info { color: var(--status-info); background: var(--info-bg); }
.severity-chip.ok { color: var(--status-success); background: var(--success-bg); }


.posture-domain-card {
  display: grid;
  gap: 6px;
  padding: 14px 16px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  transition: transform var(--transition-base), border-color var(--transition-base), box-shadow var(--transition-base);
}

.posture-domain-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: var(--border-soft);
}

.posture-domain-card.has-issues { border-left: 3px solid var(--status-danger); }

.posture-domain-label {
  margin: 0;
  font-weight: 650;
  font-size: 13px;
}

.posture-domain-checks {
  margin: 0;
  color: var(--text-secondary);
  font-size: 12px;
}

.posture-domain-severity {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0;
}

.posture-findings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.posture-findings-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 650;
}

.posture-findings-hint {
  color: var(--text-tertiary);
  font-size: 12px;
}

.posture-empty {
  display: grid;
  place-items: center;
  gap: 10px;
  min-height: 160px;
  color: var(--text-muted);
  text-align: center;
}

.posture-finding-list {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 16px;
  list-style: none;
}

.posture-finding {
  display: grid;
  grid-template-columns: 56px 1fr;
  gap: 12px;
  align-items: start;
  padding: 12px 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}

.posture-finding-severity {
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 650;
  text-align: center;
  border-radius: var(--radius-full);
}

.posture-severity-critical .posture-finding-severity { color: #fff; background: var(--status-danger); }
.posture-severity-warning .posture-finding-severity { color: var(--status-warning); background: var(--warning-bg); }
.posture-severity-info .posture-finding-severity { color: var(--status-info); background: var(--info-bg); }

.posture-finding-body {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.posture-finding-summary { margin: 0; font-size: 13px; }
.posture-finding-meta { margin: 0; color: var(--text-secondary); font-size: 12px; }
.posture-finding-code {
  margin: 0;
  color: var(--text-tertiary);
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Consolas, monospace;
  font-size: 11px;
}

.posture-loading {
  padding: 40px;
  color: var(--text-tertiary);
  text-align: center;
}

.form-select {
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
}

.notice.error { color: var(--status-danger); }	
.posture-insight-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 8px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 600;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent) 25%, transparent);
  border-radius: var(--radius-full);
  cursor: pointer;
  transition: background var(--transition-base), transform var(--transition-base), box-shadow var(--transition-base);
}

.posture-insight-toggle:hover {
  background: color-mix(in srgb, var(--accent) 18%, transparent);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgb(0 0 0 / 0.12);
}

.posture-runbook {
  grid-column: 1 / -1;
  display: grid;
  gap: 10px;
  margin-top: 10px;
  padding: 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  animation: runbook-in 0.35s cubic-bezier(0.22, 1, 0.36, 1);
}

@keyframes runbook-in {
  from { opacity: 0; transform: translateY(-6px); }
  to { opacity: 1; transform: translateY(0); }
}

.runbook-step {
  display: grid;
  grid-template-columns: 28px 1fr auto;
  gap: 10px;
  align-items: center;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  transition: border-color var(--transition-base), transform var(--transition-base);
}

.runbook-step:hover {
  border-color: color-mix(in srgb, var(--accent) 30%, var(--border-subtle));
  transform: translateX(2px);
}

.runbook-step-icon {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  border-radius: var(--radius-full);
}

.runbook-step-body {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.runbook-step-title { margin: 0; font-size: 12px; font-weight: 650; }
.runbook-step-desc { margin: 0; color: var(--text-secondary); font-size: 12px; }
.runbook-step-meta {
  margin: 0;
  color: var(--text-tertiary);
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Consolas, monospace;
  font-size: 11px;
}

.runbook-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--accent);
  text-decoration: none;
  white-space: nowrap;
}

.runbook-link:hover { text-decoration: underline; }
</style>