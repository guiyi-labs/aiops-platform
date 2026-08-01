<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  BookOpen,
  Brain,
  ChevronDown,
  ChevronRight,
  ClipboardList,
  Cpu,
  FileSearch,
  Lightbulb,
  RefreshCw,
  RotateCcw,
  Sparkles,
} from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import {
  generateInvestigation,
  getInvestigation,
  listCorrelationCases,
  listInvestigations,
  listInvestigatorRunbooks,
} from '../api/aiops'
import { listClusters } from '../api/clusters'
import type {
  CorrelationCase,
  HypothesisConfidence,
  Investigation,
  InvestigationListResponse,
  InvestigationStatus,
  Runbook,
} from '../types/aiops'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()

// Runbook catalog
const runbooks = ref<Runbook[]>([])
const investigatorVersion = ref('')
const runbooksLoading = ref(true)
const runbooksError = ref('')

// Case selector
const clusters = ref<Cluster[]>([])
const cases = ref<CorrelationCase[]>([])
const casesLoading = ref(false)
const casesError = ref('')
const selectedCaseId = ref<number | null>(null)

// Investigation list
const investigations = ref<Investigation[]>([])
const investigationsLoading = ref(false)
const investigationsError = ref('')

// Investigation detail
const expandedInvestigationId = ref<number | null>(null)
const investigationDetail = ref<Investigation | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

// Generation
const generating = ref(false)
const generateError = ref('')
const generateSuccess = ref('')

const levelLabels: Record<string, string> = { L0: 'L0', L1: 'L1', L2: 'L2', L3: 'L3' }

function levelClass(level: string): string {
  switch (level) {
    case 'L0': return 'level-green'
    case 'L1': return 'level-blue'
    case 'L2': return 'level-amber'
    case 'L3': return 'level-red'
    default: return 'level-neutral'
  }
}

const statusLabels: Record<InvestigationStatus, string> = {
  completed: '已完成',
  failed: '失败',
  stale: '已过期',
}

function statusClass(status: InvestigationStatus): string {
  switch (status) {
    case 'completed': return 'inv-status-green'
    case 'failed': return 'inv-status-red'
    case 'stale': return 'inv-status-gray'
  }
}

const confidenceLabels: Record<HypothesisConfidence, string> = { high: '高', medium: '中', low: '低' }

function confidenceClass(confidence: HypothesisConfidence): string {
  switch (confidence) {
    case 'high': return 'conf-green'
    case 'medium': return 'conf-amber'
    case 'low': return 'conf-red'
  }
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

function clusterName(clusterId: number): string {
  return clusters.value.find((c) => c.id === clusterId)?.name ?? `集群 #${clusterId}`
}

const selectedCase = computed(() => cases.value.find((c) => c.id === selectedCaseId.value) ?? null)

async function loadRunbooks() {
  runbooksLoading.value = true
  runbooksError.value = ''
  try {
    const resp = await listInvestigatorRunbooks(auth.accessToken)
    runbooks.value = resp.items ?? []
    investigatorVersion.value = resp.investigator_version
  } catch (error) {
    runbooksError.value = error instanceof Error ? error.message : '无法加载调查 Runbook 目录'
  } finally {
    runbooksLoading.value = false
  }
}

async function loadCases() {
  casesLoading.value = true
  casesError.value = ''
  try {
    const [clusterResp, caseResp] = await Promise.all([
      listClusters(auth.accessToken),
      listCorrelationCases(auth.accessToken, { limit: 100 }),
    ])
    clusters.value = clusterResp.items ?? []
    cases.value = caseResp.items ?? []
    if (cases.value.length > 0 && selectedCaseId.value === null) {
      selectedCaseId.value = cases.value[0].id
    }
  } catch (error) {
    casesError.value = error instanceof Error ? error.message : '无法加载关联案例'
  } finally {
    casesLoading.value = false
  }
}

async function loadInvestigations() {
  if (selectedCaseId.value === null) {
    investigations.value = []
    return
  }
  investigationsLoading.value = true
  investigationsError.value = ''
  investigationDetail.value = null
  expandedInvestigationId.value = null
  try {
    const resp: InvestigationListResponse = await listInvestigations(auth.accessToken, selectedCaseId.value)
    investigations.value = resp.items ?? []
  } catch (error) {
    investigations.value = []
    investigationsError.value = error instanceof Error ? error.message : '无法加载调查记录'
  } finally {
    investigationsLoading.value = false
  }
}

async function toggleInvestigation(inv: Investigation) {
  if (expandedInvestigationId.value === inv.id) {
    expandedInvestigationId.value = null
    investigationDetail.value = null
    detailError.value = ''
    return
  }
  expandedInvestigationId.value = inv.id
  detailError.value = ''
  investigationDetail.value = inv
  detailLoading.value = true
  try {
    investigationDetail.value = await getInvestigation(auth.accessToken, inv.id)
  } catch (error) {
    detailError.value = error instanceof Error ? error.message : '无法加载调查详情'
  } finally {
    detailLoading.value = false
  }
}

async function generateNew() {
  if (selectedCaseId.value === null) return
  generating.value = true
  generateError.value = ''
  generateSuccess.value = ''
  try {
    const created = await generateInvestigation(auth.accessToken, selectedCaseId.value)
    generateSuccess.value = `调查 #${created.id} 已生成`
    await loadInvestigations()
    expandedInvestigationId.value = created.id
    investigationDetail.value = created
  } catch (error) {
    generateError.value = error instanceof Error ? error.message : '生成调查失败'
  } finally {
    generating.value = false
  }
}

watch(selectedCaseId, () => {
  generateSuccess.value = ''
  generateError.value = ''
  void loadInvestigations()
})

onMounted(() => {
  void loadRunbooks()
  void loadCases()
})
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="AI 调查">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="runbooksLoading" @click="loadRunbooks">
        <RefreshCw :size="16" :class="{ spinning: runbooksLoading }" />
        <span>刷新</span>
      </button>
    </template>

    <!-- Runbook catalog -->
    <section class="panel runbook-panel">
      <header class="panel-header">
        <div class="panel-title">
          <BookOpen :size="18" />
          <strong>调查 Runbook 目录</strong>
          <span v-if="investigatorVersion" class="muted small">v{{ investigatorVersion }}</span>
        </div>
      </header>

      <div v-if="runbooksLoading" class="panel-empty">加载中…</div>
      <div v-else-if="runbooksError" class="panel-empty error">{{ runbooksError }}</div>
      <div v-else-if="runbooks.length === 0" class="panel-empty muted">暂无可用 Runbook</div>
      <div v-else class="runbook-grid">
        <article v-for="rb in runbooks" :key="rb.id" class="runbook-card">
          <div class="runbook-card-head">
            <strong>{{ rb.display_name }}</strong>
            <span :class="['level-badge', levelClass(rb.level)]">{{ levelLabels[rb.level] ?? rb.level }}</span>
          </div>
          <p class="runbook-desc">{{ rb.description }}</p>
          <div class="runbook-block">
            <p class="muted small">动作编码</p>
            <div class="chip-list">
              <span v-for="code in rb.action_codes" :key="code" class="chip">{{ code }}</span>
              <span v-if="rb.action_codes.length === 0" class="muted small">—</span>
            </div>
          </div>
          <div class="runbook-block">
            <p class="muted small">前置条件</p>
            <ul class="runbook-list">
              <li v-for="(pre, idx) in rb.prerequisites" :key="`pre-${idx}`">{{ pre }}</li>
              <li v-if="rb.prerequisites.length === 0" class="muted small">—</li>
            </ul>
          </div>
          <div class="runbook-block">
            <p class="muted small">回滚策略</p>
            <p class="runbook-rollback">{{ rb.rollback_strategy || '—' }}</p>
          </div>
        </article>
      </div>
    </section>

    <!-- Two-column: case selector + list / detail -->
    <div class="two-column">
      <!-- Left: case selector + investigation list -->
      <section class="panel">
        <header class="panel-header">
          <div class="panel-title">
            <ClipboardList :size="18" />
            <strong>关联案例与调查</strong>
          </div>
        </header>

        <div class="case-selector">
          <label class="form-label">
            <span>关联案例</span>
            <select v-model.number="selectedCaseId" class="case-select" :disabled="casesLoading || cases.length === 0">
              <option :value="null">请选择案例</option>
              <option v-for="c in cases" :key="c.id" :value="c.id">
                #{{ c.id }} · {{ clusterName(c.cluster_id) }} · {{ c.primary_resource.kind }}/{{ c.primary_resource.name }}
              </option>
            </select>
          </label>
          <p v-if="casesError" class="error-text">{{ casesError }}</p>
          <p v-else-if="!casesLoading && cases.length === 0" class="muted small">暂无关联案例</p>
        </div>

        <div class="investigation-list">
          <div v-if="!selectedCaseId" class="panel-empty muted">请先选择关联案例</div>
          <div v-else-if="investigationsLoading" class="panel-empty">加载中…</div>
          <div v-else-if="investigationsError" class="panel-empty error">{{ investigationsError }}</div>
          <div v-else-if="investigations.length === 0" class="panel-empty muted">该案例暂无调查记录</div>
          <div v-else class="inv-items">
            <button
              v-for="inv in investigations"
              :key="inv.id"
              type="button"
              class="inv-row"
              :class="{ active: expandedInvestigationId === inv.id }"
              @click="toggleInvestigation(inv)"
            >
              <div class="inv-row-main">
                <div class="inv-row-head">
                  <strong>调查 #{{ inv.id }}</strong>
                  <span :class="['inv-status', statusClass(inv.status)]">{{ statusLabels[inv.status] }}</span>
                </div>
                <span class="muted small inv-provider">{{ inv.provider }} · {{ inv.model }}</span>
                <span class="inv-summary">{{ inv.summary || '（无摘要）' }}</span>
                <span class="muted small">{{ formatTimestamp(inv.created_at) }}</span>
              </div>
              <ChevronRight v-if="expandedInvestigationId !== inv.id" :size="16" class="inv-chevron" />
              <ChevronDown v-else :size="16" class="inv-chevron" />
            </button>
          </div>
        </div>
      </section>

      <!-- Right: investigation detail -->
      <section class="panel detail-panel">
        <header class="panel-header sticky">
          <div class="panel-title">
            <Brain :size="18" />
            <strong>调查详情</strong>
          </div>
          <button
            v-if="selectedCaseId"
            type="button"
            class="primary-button"
            :disabled="generating"
            @click="generateNew"
          >
            <Sparkles :size="16" />
            <span>{{ generating ? '生成中…' : '生成新调查' }}</span>
          </button>
        </header>

        <!-- Empty state -->
        <div v-if="!selectedCase" class="panel-empty muted">选择关联案例后查看或生成调查</div>

        <div v-else-if="!investigationDetail && !detailLoading && !investigations.length" class="panel-empty muted">
          该案例暂无调查记录，点击「生成新调查」开始
        </div>

        <div v-else-if="!investigationDetail" class="panel-empty muted">点击左侧调查记录查看详情</div>

        <div v-else class="inv-detail">
          <p v-if="generateSuccess" class="success-banner">{{ generateSuccess }}</p>
          <p v-if="generateError" class="error-text">{{ generateError }}</p>
          <p v-if="detailError" class="error-text">{{ detailError }}</p>

          <div class="detail-loading" v-if="detailLoading">加载详情中…</div>

          <template v-if="investigationDetail">
            <!-- Meta -->
            <div class="detail-meta">
              <div class="meta-row">
                <span class="muted">调查 ID</span>
                <strong>#{{ investigationDetail.id }}</strong>
              </div>
              <div class="meta-row">
                <span class="muted">状态</span>
                <span :class="['inv-status', statusClass(investigationDetail.status)]">{{ statusLabels[investigationDetail.status] }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">模型</span>
                <span class="mono small">{{ investigationDetail.provider }} / {{ investigationDetail.model }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">引擎版本</span>
                <span class="mono small">{{ investigationDetail.investigator_version }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">创建时间</span>
                <span>{{ formatTimestamp(investigationDetail.created_at) }}</span>
              </div>
              <div class="meta-row" v-if="investigationDetail.failure_reason">
                <span class="muted">失败原因</span>
                <span class="error-text">{{ investigationDetail.failure_reason }}</span>
              </div>
            </div>

            <!-- Summary & impact -->
            <section class="detail-section">
              <div class="section-head"><FileSearch :size="16" /><h3>摘要</h3></div>
              <p class="detail-text">{{ investigationDetail.summary || '（无摘要）' }}</p>
            </section>
            <section class="detail-section">
              <div class="section-head"><AlertTriangle :size="16" /><h3>影响</h3></div>
              <p class="detail-text">{{ investigationDetail.impact || '（无影响说明）' }}</p>
            </section>

            <!-- Hypotheses -->
            <section class="detail-section">
              <div class="section-head"><Lightbulb :size="16" /><h3>假设 ({{ investigationDetail.hypotheses.length }})</h3></div>
              <div v-if="investigationDetail.hypotheses.length === 0" class="muted small">暂无假设</div>
              <div v-else class="hypothesis-list">
                <article v-for="(h, idx) in investigationDetail.hypotheses" :key="`h-${idx}`" class="hypothesis-card">
                  <div class="hypothesis-head">
                    <span :class="['conf-badge', confidenceClass(h.confidence)]">置信度 {{ confidenceLabels[h.confidence] }}</span>
                    <span class="muted small">证据 {{ h.evidence_ids.length }} · 反证 {{ h.disconfirming_evidence.length }}</span>
                  </div>
                  <p class="hypothesis-claim">{{ h.claim }}</p>
                  <div class="hypothesis-block" v-if="h.next_checks.length">
                    <p class="muted small">后续检查</p>
                    <ul class="runbook-list">
                      <li v-for="(check, ci) in h.next_checks" :key="`c-${idx}-${ci}`">{{ check }}</li>
                    </ul>
                  </div>
                </article>
              </div>
            </section>

            <!-- Citations -->
            <section class="detail-section">
              <div class="section-head"><ClipboardList :size="16" /><h3>引用 ({{ investigationDetail.citations.length }})</h3></div>
              <div v-if="investigationDetail.citations.length === 0" class="muted small">暂无引用</div>
              <table v-else class="compact-table">
                <thead><tr><th>证据类型</th><th>证据 ID</th><th>声明</th></tr></thead>
                <tbody>
                  <tr v-for="(cit, idx) in investigationDetail.citations" :key="`cit-${idx}`">
                    <td class="mono">{{ cit.evidence_ref.kind }}</td>
                    <td class="mono">{{ cit.evidence_ref.id }}</td>
                    <td>{{ cit.claim }}</td>
                  </tr>
                </tbody>
              </table>
            </section>

            <!-- Uncertainties -->
            <section class="detail-section">
              <div class="section-head"><AlertTriangle :size="16" /><h3>不确定性</h3></div>
              <div v-if="investigationDetail.uncertainties.length === 0" class="muted small">无</div>
              <ul v-else class="runbook-list">
                <li v-for="(u, idx) in investigationDetail.uncertainties" :key="`u-${idx}`">{{ u }}</li>
              </ul>
            </section>

            <!-- Token usage -->
            <section class="detail-section">
              <div class="section-head"><Cpu :size="16" /><h3>Token 用量</h3></div>
              <div class="token-grid">
                <div class="token-card">
                  <p class="muted small">输入 Tokens</p>
                  <strong>{{ investigationDetail.input_tokens }}</strong>
                </div>
                <div class="token-card">
                  <p class="muted small">输出 Tokens</p>
                  <strong>{{ investigationDetail.output_tokens }}</strong>
                </div>
                <div class="token-card">
                  <p class="muted small">合计</p>
                  <strong>{{ investigationDetail.input_tokens + investigationDetail.output_tokens }}</strong>
                </div>
              </div>
            </section>

            <!-- Recommended runbook -->
            <section class="detail-section" v-if="investigationDetail.recommended_runbook_id">
              <div class="section-head"><RotateCcw :size="16" /><h3>推荐 Runbook</h3></div>
              <p class="mono small">{{ investigationDetail.recommended_runbook_id }}</p>
            </section>
          </template>
        </div>
      </section>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.runbook-panel { margin-top: 18px; }

.runbook-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
  padding: 16px;
}

.runbook-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.runbook-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.runbook-card-head strong { color: var(--text-primary); font-size: 14px; }

.level-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 9px;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-full);
}
.level-green { color: var(--status-success); background: var(--success-bg); }
.level-blue { color: var(--status-info); background: var(--info-bg); }
.level-amber { color: var(--status-warning); background: var(--warning-bg); }
.level-red { color: var(--status-danger); background: var(--danger-bg); }
.level-neutral { color: var(--text-secondary); background: var(--bg-tertiary); }

.runbook-desc { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.55; }

.runbook-block { display: flex; flex-direction: column; gap: 5px; }

.chip-list { display: flex; flex-wrap: wrap; gap: 6px; }
.chip {
  padding: 2px 8px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.runbook-list { margin: 0; padding-left: 18px; color: var(--text-secondary); font-size: 12px; line-height: 1.6; }
.runbook-rollback { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.55; }

/* Case selector */
.case-selector { padding: 14px; border-bottom: 1px solid var(--border-soft); }
.form-label { display: flex; flex-direction: column; gap: 5px; font-size: 12px; color: var(--text-muted); }
.case-select {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-inset);
}
.case-select:focus { border-color: var(--accent-primary); box-shadow: 0 0 0 3px var(--accent-soft); outline: none; }

/* Investigation list */
.investigation-list { padding: 8px; }
.inv-items { display: flex; flex-direction: column; gap: 6px; }
.inv-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  text-align: left;
  padding: 11px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}
.inv-row:hover { background: var(--bg-tertiary); }
.inv-row.active { border-color: var(--accent-primary); box-shadow: 0 0 0 2px var(--accent-soft); }

.inv-row-main { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.inv-row-head { display: flex; align-items: center; gap: 8px; }
.inv-row-head strong { color: var(--text-primary); font-size: 13px; }
.inv-provider { font-family: var(--font-mono); }
.inv-summary {
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.inv-chevron { color: var(--text-muted); flex: 0 0 auto; margin-top: 2px; }

.inv-status {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-full);
}
.inv-status-green { color: var(--status-success); background: var(--success-bg); }
.inv-status-red { color: var(--status-danger); background: var(--danger-bg); }
.inv-status-gray { color: var(--text-secondary); background: var(--bg-tertiary); }

/* Detail panel */
.inv-detail { display: flex; flex-direction: column; gap: 18px; padding: 16px; }

.detail-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 18px;
  padding: 14px;
  background: var(--bg-secondary);
  border-radius: var(--radius-lg);
}
.meta-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }

.detail-section { display: flex; flex-direction: column; gap: 8px; }
.section-head { display: flex; align-items: center; gap: 8px; color: var(--text-primary); }
.section-head h3 { margin: 0; font-size: 14px; }
.detail-text { margin: 0; color: var(--text-secondary); font-size: 13px; line-height: 1.65; }

.hypothesis-list { display: flex; flex-direction: column; gap: 10px; }
.hypothesis-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}
.hypothesis-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.conf-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 9px;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-full);
}
.conf-green { color: var(--status-success); background: var(--success-bg); }
.conf-amber { color: var(--status-warning); background: var(--warning-bg); }
.conf-red { color: var(--status-danger); background: var(--danger-bg); }
.hypothesis-claim { margin: 0; color: var(--text-primary); font-size: 13px; line-height: 1.6; }
.hypothesis-block { display: flex; flex-direction: column; gap: 4px; }

.token-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.token-card {
  padding: 12px 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.token-card strong { display: block; margin-top: 4px; font-size: 20px; color: var(--text-primary); }

.success-banner {
  margin: 0;
  padding: 9px 12px;
  color: var(--status-success);
  font-size: 12px;
  background: var(--success-bg);
  border-left: 3px solid var(--status-success);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.error-text {
  margin: 0;
  padding: 9px 12px;
  color: var(--status-danger);
  font-size: 12px;
  background: var(--danger-bg);
  border-left: 3px solid var(--status-danger);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.detail-loading { color: var(--text-secondary); font-size: 12px; }

.mono { font-family: var(--font-mono); }
.small { font-size: 12px; }

@media (max-width: 900px) {
  .detail-meta { grid-template-columns: 1fr; }
  .token-grid { grid-template-columns: 1fr; }
}
</style>
