<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  BadgeCheck,
  CheckCircle2,
  CircleDollarSign,
  Cpu,
  History,
  MemoryStick,
  PackageX,
  RefreshCw,
  ShieldCheck,
  Wallet,
} from 'lucide-vue-next'

import * as clusterAPI from '../api/clusters'
import * as optimizationAPI from '../api/optimization'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { CISStatus, DeprecatedAPIStatus, FinOpsWasteSummary, OptimizationFinding } from '../types/optimization'

// Read-only optimization console (M66) over the M61-M65 analyzers.
//
// Each tab posts only the cluster id, which makes the server auto-collect the
// observation bundle and run the corresponding pure analyzer. No request from
// this view can mutate cluster state (ADR 0004).

type TabKey = 'finops' | 'cis' | 'deprecated'

const auth = useAuthStore()

const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const activeTab = ref<TabKey>('finops')
const clustersLoading = ref(true)
const clustersError = ref('')

const finops = ref<FinOpsWasteSummary | null>(null)
const cis = ref<CISStatus | null>(null)
const deprecated = ref<DeprecatedAPIStatus | null>(null)

const loading = ref<Record<TabKey, boolean>>({ finops: false, cis: false, deprecated: false })
const errors = ref<Record<TabKey, string>>({ finops: '', cis: '', deprecated: '' })

const targetVersion = ref('1.29')

// Guards against a slow response for a previous cluster overwriting a newer one.
let requestSequence = 0

const tabs: { key: TabKey; label: string; icon: typeof Wallet }[] = [
  { key: 'finops', label: '成本优化', icon: Wallet },
  { key: 'cis', label: 'CIS 合规', icon: ShieldCheck },
  { key: 'deprecated', label: '废弃 API', icon: PackageX },
]

const severityLabels: Record<string, string> = {
  critical: '严重',
  warning: '警告',
  info: '提示',
}

/** Maps an analyzer severity onto the console's shared badge palette. */
function severityClass(severity: string): string {
  if (severity === 'critical') return 'failed'
  if (severity === 'warning') return 'pending'
  return 'unknown'
}

function severityLabel(severity: string): string {
  return severityLabels[severity] ?? severity
}

/** Formats nanocores as cores; `-1` means the workload set no value. */
function formatCPU(nanocores: number): string {
  if (nanocores < 0) return '未设置'
  if (nanocores === 0) return '0'
  const cores = nanocores / 1e9
  return cores >= 1 ? `${cores.toFixed(2)} core` : `${Math.round(nanocores / 1e6)} m`
}

/** Formats bytes as GiB/MiB; `-1` means the workload set no value. */
function formatMemory(bytes: number): string {
  if (bytes < 0) return '未设置'
  if (bytes === 0) return '0'
  const gib = bytes / 1024 ** 3
  return gib >= 1 ? `${gib.toFixed(2)} GiB` : `${Math.round(bytes / 1024 ** 2)} MiB`
}

/** The backend costs resources in USD per core-/GB-month, so keep the unit. */
function formatUSD(amount: number): string {
  return `$${amount.toFixed(2)}`
}

function formatTimestamp(value: string | undefined): string {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN')
}

const selectedCluster = computed(() => clusters.value.find((c) => c.id === selectedClusterID.value) ?? null)

/** CIS findings ordered critical → warning → info so the worst surface first. */
const cisFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(cis.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

const deprecatedFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(deprecated.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** Highest monthly waste first — the recommendations worth acting on. */
const finopsRecommendations = computed(() =>
  [...(finops.value?.recommendations ?? [])].sort((a, b) => b.monthly_waste_usd - a.monthly_waste_usd),
)

const cisFamilies = computed(() => Object.entries(cis.value?.by_family ?? {}).sort((a, b) => b[1] - a[1]))

/** CIS pass rate, used as the headline compliance score. */
const cisScore = computed(() => {
  const total = cis.value?.total ?? 0
  if (total === 0) return 0
  return Math.round(((cis.value?.passed ?? 0) / total) * 100)
})

function describeError(err: unknown): string {
  const apiError = err as { code?: string; message?: string }
  if (apiError?.code === 'COLLECT_FAILED') {
    return `无法从集群采集数据：${apiError.message ?? '集群不可达'}`
  }
  if (apiError?.code === 'NO_INPUTS') {
    return '服务端未配置自动采集，且未提供观测数据'
  }
  return apiError?.message ?? '分析失败，请稍后重试'
}

async function runAnalysis(tab: TabKey, force = false) {
  const clusterId = selectedClusterID.value
  if (!clusterId || !auth.accessToken) return
  const cached = tab === 'finops' ? finops.value : tab === 'cis' ? cis.value : deprecated.value
  if (cached && !force) return

  const sequence = ++requestSequence
  loading.value = { ...loading.value, [tab]: true }
  errors.value = { ...errors.value, [tab]: '' }
  try {
    if (tab === 'finops') {
      const result = await optimizationAPI.analyzeFinOps(auth.accessToken, clusterId)
      if (sequence === requestSequence) finops.value = result
    } else if (tab === 'cis') {
      const result = await optimizationAPI.analyzeCIS(auth.accessToken, clusterId)
      if (sequence === requestSequence) cis.value = result
    } else {
      const result = await optimizationAPI.analyzeDeprecatedAPI(auth.accessToken, clusterId, targetVersion.value)
      if (sequence === requestSequence) deprecated.value = result
    }
  } catch (err) {
    if (sequence === requestSequence) errors.value = { ...errors.value, [tab]: describeError(err) }
  } finally {
    if (sequence === requestSequence) loading.value = { ...loading.value, [tab]: false }
  }
}

function resetResults() {
  finops.value = null
  cis.value = null
  deprecated.value = null
  errors.value = { finops: '', cis: '', deprecated: '' }
}

async function loadClusters() {
  clustersLoading.value = true
  clustersError.value = ''
  try {
    if (!auth.accessToken) return
    const list = await clusterAPI.listClusters(auth.accessToken)
    clusters.value = list.items.filter((c) => c.enabled)
    if (clusters.value.length > 0) selectedClusterID.value = clusters.value[0].id
  } catch {
    clustersError.value = '无法加载集群列表'
  } finally {
    clustersLoading.value = false
  }
}

// Switching clusters invalidates every analyzer result.
watch(selectedClusterID, () => {
  resetResults()
  void runAnalysis(activeTab.value)
})

// Analyse a tab the first time it is opened; later visits reuse the result.
watch(activeTab, (tab) => void runAnalysis(tab))

onMounted(() => void loadClusters())
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="优化中心">
    <template #actions>
      <button
        type="button"
        class="secondary-button"
        :disabled="!selectedClusterID || loading[activeTab]"
        @click="runAnalysis(activeTab, true)"
      >
        <RefreshCw :size="16" :class="{ spin: loading[activeTab] }" />
        <span>重新分析</span>
      </button>
      <select v-model="selectedClusterID" class="cluster-select" aria-label="选择集群" :disabled="clusters.length === 0">
        <option v-if="clusters.length === 0" :value="null">无可用集群</option>
        <option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option>
      </select>
    </template>

    <p class="view-intro muted">
      只读优化分析：服务端自动从
      <strong>{{ selectedCluster?.name ?? '所选集群' }}</strong>
      采集观测数据并运行分析器，不会对集群做任何变更。
    </p>

    <nav class="optimization-tabs" aria-label="分析器切换">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        <component :is="tab.icon" :size="15" />
        <span>{{ tab.label }}</span>
      </button>
    </nav>

    <div v-if="clustersLoading" class="panel-empty">加载集群列表…</div>
    <div v-else-if="clustersError" class="panel-empty error">{{ clustersError }}</div>
    <div v-else-if="clusters.length === 0" class="panel-empty muted">没有已启用的集群，请先在集群管理中接入集群</div>

    <template v-else>
      <!-- ------------------------------------------------ FinOps 成本优化 -->
      <section v-if="activeTab === 'finops'" class="optimization-tab">
        <div v-if="loading.finops" class="panel-empty">正在采集资源用量并计算右配建议…</div>
        <div v-else-if="errors.finops" class="panel-empty error">{{ errors.finops }}</div>
        <template v-else-if="finops">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><CircleDollarSign :size="16" />每月可节省</p>
              <strong>{{ formatUSD(finops.monthly_waste_usd) }}</strong>
              <span>按闲置的 request 估算</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />过度申请容器</p>
              <strong>{{ finops.containers_over_provisioned }}</strong>
              <span>共评估 {{ finops.containers_evaluated }} 个容器</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><Cpu :size="16" />闲置 CPU</p>
              <strong>{{ finops.cpu_idle_cores.toFixed(2) }}</strong>
              <span>核（request 减去 P95 用量）</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><MemoryStick :size="16" />闲置内存</p>
              <strong>{{ finops.mem_idle_gb.toFixed(2) }}</strong>
              <span>GB（request 减去 P95 用量）</span>
            </article>
          </div>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <Wallet :size="18" />
                <strong>右配建议</strong>
                <span class="muted">{{ finopsRecommendations.length }} 条</span>
              </div>
              <span class="muted">分析时间 {{ formatTimestamp(finops.evaluated_at) }}</span>
            </header>
            <div v-if="finopsRecommendations.length === 0" class="panel-empty muted">
              没有发现过度申请的容器，当前资源配置合理
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>工作负载</th>
                    <th>容器</th>
                    <th>建议 CPU request</th>
                    <th>建议内存 request</th>
                    <th>副本</th>
                    <th>每月浪费</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="rec in finopsRecommendations" :key="`${rec.namespace}/${rec.workload_name}/${rec.container_name}`">
                    <td>
                      <div class="cell-main">{{ rec.workload_name }}</div>
                      <div class="cell-sub muted">{{ rec.workload_kind }} · {{ rec.namespace }}</div>
                    </td>
                    <td>{{ rec.container_name }}</td>
                    <td>{{ formatCPU(rec.suggested_requests.cpu_request) }}</td>
                    <td>{{ formatMemory(rec.suggested_requests.mem_request) }}</td>
                    <td>{{ rec.replicas }}</td>
                    <td class="numeric">{{ formatUSD(rec.monthly_waste_usd) }}</td>
                    <td>
                      <span :class="['phase-badge', severityClass(rec.severity)]" :title="rec.rationale">
                        {{ severityLabel(rec.severity) }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </template>
        <div v-else class="panel-empty muted">选择集群后开始分析</div>
      </section>

      <!-- --------------------------------------------------- CIS 合规态势 -->
      <section v-else-if="activeTab === 'cis'" class="optimization-tab">
        <div v-if="loading.cis" class="panel-empty">正在采集工作负载与 RBAC 配置并执行 CIS 基线检查…</div>
        <div v-else-if="errors.cis" class="panel-empty error">{{ errors.cis }}</div>
        <template v-else-if="cis">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><BadgeCheck :size="16" />合规得分</p>
              <strong>{{ cisScore }}%</strong>
              <span>{{ cis.passed }} / {{ cis.total }} 项检查通过</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />未通过检查</p>
              <strong>{{ cis.failed }}</strong>
              <span>需要整改的控制项</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><ShieldCheck :size="16" />严重问题</p>
              <strong>{{ cis.by_severity.critical ?? 0 }}</strong>
              <span>警告 {{ cis.by_severity.warning ?? 0 }} · 提示 {{ cis.by_severity.info ?? 0 }}</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />检查项总数</p>
              <strong>{{ cis.total }}</strong>
              <span>分析时间 {{ formatTimestamp(cis.evaluated_at) }}</span>
            </article>
          </div>

          <div v-if="cisFamilies.length > 0" class="family-chips">
            <span v-for="[family, count] in cisFamilies" :key="family" class="family-chip">
              {{ family }}<em>{{ count }}</em>
            </span>
          </div>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <ShieldCheck :size="18" />
                <strong>未通过的控制项</strong>
                <span class="muted">{{ cisFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="cisFindings.length === 0" class="panel-empty muted">所有已采集的控制项均已通过</div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>控制项</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in cisFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                      </div>
                    </td>
                    <td><span :class="['phase-badge', severityClass(item.severity)]">{{ severityLabel(item.severity) }}</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </template>
        <div v-else class="panel-empty muted">选择集群后开始分析</div>
      </section>

      <!-- ------------------------------------------------- 废弃 API 检查 -->
      <section v-else class="optimization-tab">
        <section class="deprecated-toolbar">
          <label class="toolbar-field">
            <span class="muted">目标 Kubernetes 版本</span>
            <input
              v-model="targetVersion"
              type="text"
              class="compact-input"
              placeholder="例如 1.29"
              aria-label="目标 Kubernetes 版本"
              @keyup.enter="runAnalysis('deprecated', true)"
            />
          </label>
          <button
            type="button"
            class="secondary-button"
            :disabled="!targetVersion || loading.deprecated"
            @click="runAnalysis('deprecated', true)"
          >
            <History :size="15" />
            <span>检查升级兼容性</span>
          </button>
        </section>

        <div v-if="loading.deprecated" class="panel-empty">正在扫描集群资源的 API 版本…</div>
        <div v-else-if="errors.deprecated" class="panel-empty error">{{ errors.deprecated }}</div>
        <template v-else-if="deprecated">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><PackageX :size="16" />已移除 API</p>
              <strong>{{ deprecated.removed }}</strong>
              <span>升级到 1.{{ deprecated.target_minor }} 后将不可用</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />已废弃 API</p>
              <strong>{{ deprecated.deprecated }}</strong>
              <span>仍可用但应尽快迁移</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />兼容对象</p>
              <strong>{{ deprecated.clean }}</strong>
              <span>共扫描 {{ deprecated.total }} 个对象</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><History :size="16" />目标版本</p>
              <strong>1.{{ deprecated.target_minor }}</strong>
              <span>分析时间 {{ formatTimestamp(deprecated.evaluated_at) }}</span>
            </article>
          </div>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <PackageX :size="18" />
                <strong>需要迁移的对象</strong>
                <span class="muted">{{ deprecatedFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="deprecatedFindings.length === 0" class="panel-empty muted">
              没有发现废弃或已移除的 API，可安全升级到 1.{{ deprecated.target_minor }}
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>资源</th>
                    <th>当前 API 版本</th>
                    <th>建议替换为</th>
                    <th>说明</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in deprecatedFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                      </div>
                    </td>
                    <td><code>{{ item.details?.api_version ?? '—' }}</code></td>
                    <td><code>{{ item.details?.replacement ?? '—' }}</code></td>
                    <td>{{ item.summary }}</td>
                    <td><span :class="['phase-badge', severityClass(item.severity)]">{{ severityLabel(item.severity) }}</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </template>
        <div v-else class="panel-empty muted">输入目标版本后开始检查</div>
      </section>
    </template>
  </ConsoleLayout>
</template>

<style scoped>
.view-intro {
  margin-top: 16px;
  font-size: 12px;
}

/* The console header slot has no shared select style, so match .compact-input. */
.cluster-select {
  height: 36px;
  min-width: 160px;
  padding: 0 10px;
  color: #43515a;
  font-size: 12px;
  background: #ffffff;
  border: 1px solid #cfd8dc;
  border-radius: 5px;
}

.optimization-tabs {
  display: flex;
  gap: 4px;
  width: fit-content;
  margin-top: 14px;
  padding: 4px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.optimization-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 0 16px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
}

.optimization-tabs button:hover {
  color: var(--text-primary);
}

.optimization-tabs button.active {
  color: var(--text-primary);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
}

.optimization-tab .panel {
  margin-top: 16px;
}

.deprecated-toolbar {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  margin-top: 16px;
}

.toolbar-field {
  display: grid;
  gap: 6px;
  font-size: 12px;
}

.toolbar-field .compact-input {
  width: 180px;
}

.family-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

.family-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  color: var(--text-secondary);
  font-size: 12px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.family-chip em {
  font-style: normal;
  font-weight: 700;
  color: var(--text-primary);
}

.table-scroll {
  overflow-x: auto;
}

.cell-main {
  font-weight: 600;
}

.cell-sub {
  margin-top: 2px;
  font-size: 11px;
}

.numeric {
  font-variant-numeric: tabular-nums;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
