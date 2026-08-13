<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Boxes, Filter, GitBranch, Link2, RefreshCw, Sparkles, X, Zap } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { getCorrelationCase, listCorrelationActions, listCorrelationCases } from '../api/aiops'
import { APIError } from '../api/auth'
import { createIncident } from '../api/incidents'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { ActionCandidate, CaseConfidence, CaseStatus, CaseView, CorrelationCase } from '../types/aiops'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const statusFilter = ref<CaseStatus | ''>('')
const confidenceFilter = ref<CaseConfidence | ''>('')

const cases = ref<CorrelationCase[]>([])
const selectedCaseID = ref<number | null>(null)
const caseView = ref<CaseView | null>(null)
const actions = ref<ActionCandidate[]>([])

const loading = ref(false)
const detailLoading = ref(false)
const actionsLoading = ref(false)
const errorMessage = ref('')
const detailError = ref('')
const noticeMessage = ref('')
const promoting = ref(false)

const hasCases = computed(() => cases.value.length > 0)

function statusColor(status: CaseStatus): string {
  switch (status) {
    case 'active': return 'var(--status-danger)'
    case 'resolved': return 'var(--status-success)'
    case 'stale':
    default: return 'var(--text-muted)'
  }
}

function statusLabel(status: CaseStatus): string {
  return ({ active: '活跃', resolved: '已解决', stale: '已过期' } as const)[status]
}

function confidenceColor(conf: CaseConfidence): string {
  switch (conf) {
    case 'confirmed': return 'var(--status-success)'
    case 'candidate': return 'var(--status-info)'
    case 'contradicted': return 'var(--status-danger)'
    case 'unknown':
    default: return 'var(--text-muted)'
  }
}

function confidenceLabel(conf: CaseConfidence): string {
  return ({ confirmed: '已确认', candidate: '候选', contradicted: '已否决', unknown: '未知' } as const)[conf]
}

function resourceLabel(resource: { kind: string; name: string; namespace?: string }): string {
  return `${resource.kind}/${resource.name}`
}

function formatTime(value: string): string {
  if (!value) return '--'
  try {
    return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
  } catch {
    return value
  }
}

function completenessPercent(value: number): string {
  if (value == null || Number.isNaN(value)) return '--'
  return `${Math.round(value * 100)}%`
}

function coverageLabel(c: string): string {
  return (
    { complete: '完整', partial: '部分样本', missing: '缺失', unavailable: '无数据', truncated: '截断' } as Record<string, string>
  )[c] ?? '--'
}

function coverageTone(c: string): string {
  switch (c) {
    case 'complete': return 'var(--status-success)'
    case 'partial':
    case 'truncated': return 'var(--status-warning)'
    case 'unavailable': return 'var(--text-muted)'
    default: return 'var(--status-danger)'
  }
}

function linkWindowLabel(link: { window_start?: string; window_end?: string }): string {
  if (!link.window_start || !link.window_end) return '--'
  const start = new Date(link.window_start).getTime()
  const end = new Date(link.window_end).getTime()
  if (Number.isNaN(start) || Number.isNaN(end)) return '--'
  const seconds = Math.round((end - start) / 1000)
  if (seconds <= 0) return '--'
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

const hasPartialSignalCoverage = computed(() =>
  caseView.value?.signal_links.some((link) => link.coverage && link.coverage !== 'complete') ?? false,
)

async function loadCases() {
  if (!selectedClusterID.value) {
    cases.value = []
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const resp = await listCorrelationCases(auth.accessToken, {
      cluster_id: selectedClusterID.value,
      status: statusFilter.value || undefined,
      confidence: confidenceFilter.value || undefined,
      limit: 100,
    })
    cases.value = resp.items
  } catch {
    errorMessage.value = '无法加载关联案例列表'
  } finally {
    loading.value = false
  }
}

async function openCase(item: CorrelationCase) {
  selectedCaseID.value = item.id
  caseView.value = null
  actions.value = []
  detailError.value = ''
  detailLoading.value = true
  actionsLoading.value = true
  try {
    const [view, actionResp] = await Promise.all([
      getCorrelationCase(auth.accessToken, item.id),
      listCorrelationActions(auth.accessToken, item.id),
    ])
    caseView.value = view
    actions.value = actionResp.items
  } catch {
    detailError.value = '无法加载案例详情'
  } finally {
    detailLoading.value = false
    actionsLoading.value = false
  }
}

async function promoteToIncident() {
  if (!selectedCaseID.value || !caseView.value?.case) return
  const c = caseView.value.case
  noticeMessage.value = ''
  detailError.value = ''
  promoting.value = true
  try {
    const incident = await createIncident(auth.accessToken!, {
      source_type: 'correlation',
      source_ref: `correlation:${c.id}`,
      cluster_id: c.cluster_id,
      title: `关联案例事故 ${c.case_key}`,
      summary: `从关联案例 #${c.id} 提升的事故工作区`,
    })
    noticeMessage.value = `已创建事故工作区 ${incident.number}`
  } catch (err) {
    if (err instanceof APIError && err.code === 'SOURCE_ALREADY_USED') {
      noticeMessage.value = '该关联案例已存在关联的事故工作区'
    } else {
      detailError.value = err instanceof APIError ? err.message : '创建事故工作区失败'
    }
  } finally {
    promoting.value = false
  }
}

function closeDetail() {
  selectedCaseID.value = null
  caseView.value = null
  actions.value = []
  detailError.value = ''
}

async function changeCluster() {
  selectedCaseID.value = null
  caseView.value = null
  actions.value = []
  detailError.value = ''
  await loadCases()
}

async function initialize() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch {
    errorMessage.value = '无法加载集群列表'
  }
  await loadCases()
}

watch([statusFilter, confidenceFilter], loadCases)

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="关联案例">
    <template #actions>
      <button class="icon-button" type="button" title="刷新案例" aria-label="刷新案例列表" :disabled="loading || !selectedClusterID" @click="loadCases">
        <RefreshCw :size="18" :class="{ spinning: loading }" />
      </button>
    </template>

    <section class="cases-toolbar" aria-label="案例筛选">
      <label class="field">
        <span>集群</span>
        <select v-model="selectedClusterID" aria-label="选择集群" :disabled="clusters.length === 0" @change="changeCluster">
          <option :value="null" disabled>选择已启用集群</option>
          <option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
      </label>
      <label class="field">
        <span>状态</span>
        <select v-model="statusFilter" aria-label="按状态筛选">
          <option value="">全部状态</option>
          <option value="active">活跃</option>
          <option value="resolved">已解决</option>
          <option value="stale">已过期</option>
        </select>
      </label>
      <label class="field">
        <span>置信度</span>
        <select v-model="confidenceFilter" aria-label="按置信度筛选">
          <option value="">全部置信度</option>
          <option value="confirmed">已确认</option>
          <option value="candidate">候选</option>
          <option value="contradicted">已否决</option>
          <option value="unknown">未知</option>
        </select>
      </label>
      <span class="case-count">案例 · {{ cases.length }}</span>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <div v-if="!loading && clusters.length === 0" class="resource-empty">
      <Boxes :size="30" />
      <strong>没有已启用的集群</strong>
      <span>接入集群后可观察跨资源关联案例。</span>
    </div>

    <section v-else class="cases-workspace">
      <aside class="cases-list-panel">
        <header class="list-panel-header">
          <span class="panel-title"><Filter :size="15" />案例列表</span>
          <span class="list-count">{{ cases.length }}</span>
        </header>
        <div v-if="loading" class="panel-empty">加载中…</div>
        <div v-else-if="!hasCases" class="panel-empty">暂无符合条件的案例</div>
        <ul v-else class="case-list">
          <li v-for="item in cases" :key="item.id">
            <button type="button" class="case-row" :class="{ active: selectedCaseID === item.id }" @click="openCase(item)">
              <div class="case-row-main">
                <strong>{{ resourceLabel(item.primary_resource) }}</strong>
                <small>{{ item.primary_resource.namespace }} · 集群 #{{ item.cluster_id }} · {{ item.rule_id }}</small>
              </div>
              <div class="case-row-badges">
                <span class="mini-badge" :style="{ color: statusColor(item.status), borderColor: statusColor(item.status) }">{{ statusLabel(item.status) }}</span>
                <span class="mini-badge" :style="{ color: confidenceColor(item.confidence), borderColor: confidenceColor(item.confidence) }">{{ confidenceLabel(item.confidence) }}</span>
              </div>
              <div class="case-row-meta">
                <span>证据完整度 {{ completenessPercent(item.evidence_completeness) }}</span>
                <time>{{ formatTime(item.last_observed_at) }}</time>
              </div>
            </button>
          </li>
        </ul>
      </aside>

      <section class="case-detail-panel">
        <div v-if="!selectedCaseID" class="resource-empty detail-empty">
          <Sparkles :size="30" />
          <strong>选择左侧案例查看详情</strong>
          <span>详情包含归因因子、信号链路、资源关系、变更候选与可执行动作。</span>
        </div>
        <template v-else>
          <header class="case-detail-header">
            <div class="case-detail-title">
              <p class="context-label">CASE #{{ selectedCaseID }}</p>
              <h2 v-if="caseView?.case">{{ resourceLabel(caseView.case.primary_resource) }}</h2>
              <h2 v-else-if="detailLoading">加载中…</h2>
              <h2 v-else>案例 #{{ selectedCaseID }}</h2>
              <small v-if="caseView?.case">{{ caseView.case.primary_resource.namespace }} · 规则 {{ caseView.case.rule_id }} · 引擎 {{ caseView.case.correlation_version }}</small>
            </div>
            <div class="case-detail-actions">
              <button class="action-button" type="button" :disabled="promoting || !caseView?.case" @click="promoteToIncident">
                <Zap :size="15" />{{ promoting ? '提升中…' : '提升事故' }}
              </button>
              <button class="icon-button" type="button" title="关闭详情" aria-label="关闭详情" @click="closeDetail"><X :size="16" /></button>
            </div>
          </header>

          <p v-if="detailError" class="error-message">{{ detailError }}</p>
          <p v-if="noticeMessage" class="notice-message">{{ noticeMessage }}</p>

          <div v-if="caseView?.case" class="case-detail-badges">
            <span class="mini-badge" :style="{ color: statusColor(caseView.case.status), borderColor: statusColor(caseView.case.status) }">{{ statusLabel(caseView.case.status) }}</span>
            <span class="mini-badge" :style="{ color: confidenceColor(caseView.case.confidence), borderColor: confidenceColor(caseView.case.confidence) }">{{ confidenceLabel(caseView.case.confidence) }}</span>
            <span class="mini-badge neutral">证据完整度 {{ completenessPercent(caseView.case.evidence_completeness) }}</span>
            <span class="mini-badge neutral">首次观察 {{ formatTime(caseView.case.first_observed_at) }}</span>
            <span class="mini-badge neutral">最近观察 {{ formatTime(caseView.case.last_observed_at) }}</span>
          </div>

          <section v-if="caseView" class="detail-section">
            <h3><GitBranch :size="15" />归因因子 · {{ caseView.case.factors.length }}</h3>
            <p v-if="caseView.case.factors.length === 0" class="compact-empty">暂无归因因子</p>
            <ul v-else class="factor-list">
              <li v-for="(factor, idx) in caseView.case.factors" :key="idx">
                <div class="factor-head">
                  <strong>{{ factor.dimension }}</strong>
                  <span class="weight">权重 {{ factor.weight.toFixed(2) }}</span>
                </div>
                <p>{{ factor.reason }}</p>
                <small v-if="factor.evidence_refs.length">证据 {{ factor.evidence_refs.length }} 条</small>
              </li>
            </ul>
          </section>

          <section v-if="caseView" class="detail-section">
            <h3><Link2 :size="15" />信号链路 · {{ caseView.signal_links.length }}</h3>
            <p v-if="hasPartialSignalCoverage" class="coverage-hint">
              部分信号缺样本或覆盖不完整，案例置信度已相应调整。
            </p>
            <p v-if="caseView.signal_links.length === 0" class="compact-empty">暂无信号链路</p>
            <table v-else class="detail-table">
              <thead><tr><th>关系</th><th>信号 ID</th><th>来源</th><th>覆盖度</th><th>时间窗口</th><th>观察时间</th></tr></thead>
              <tbody>
                <tr v-for="link in caseView.signal_links" :key="link.id">
                  <td>{{ link.relation }}</td>
                  <td>{{ link.signal_id }}</td>
                  <td>{{ link.producer }}</td>
                  <td>
                    <span v-if="link.coverage" class="mini-badge" :style="{ color: coverageTone(link.coverage), borderColor: coverageTone(link.coverage) }">
                      {{ coverageLabel(link.coverage) }}
                    </span>
                    <span v-else class="mini-badge neutral">--</span>
                  </td>
                  <td>{{ linkWindowLabel(link) }}</td>
                  <td>{{ formatTime(link.observed_at) }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section v-if="caseView" class="detail-section">
            <h3><Boxes :size="15" />资源关系 · {{ caseView.resource_links.length }}</h3>
            <p v-if="caseView.resource_links.length === 0" class="compact-empty">暂无资源关系</p>
            <table v-else class="detail-table">
              <thead><tr><th>关系</th><th>资源</th><th>拓扑路径</th></tr></thead>
              <tbody>
                <tr v-for="link in caseView.resource_links" :key="link.id">
                  <td>{{ link.relation }}</td>
                  <td>{{ resourceLabel(link.resource) }}<small v-if="link.resource.namespace">{{ link.resource.namespace }}</small></td>
                  <td>{{ link.topology_path?.length ? link.topology_path.join(' / ') : '--' }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section v-if="caseView" class="detail-section">
            <h3><GitBranch :size="15" />变更候选 · {{ caseView.change_candidates.length }}</h3>
            <p v-if="caseView.change_candidates.length === 0" class="compact-empty">暂无变更候选</p>
            <table v-else class="detail-table">
              <thead><tr><th>排名</th><th>置信度</th><th>原因码</th><th>证据数</th></tr></thead>
              <tbody>
                <tr v-for="cand in caseView.change_candidates" :key="cand.id">
                  <td>#{{ cand.rank }}</td>
                  <td>
                    <span class="mini-badge" :style="{ color: confidenceColor(cand.confidence), borderColor: confidenceColor(cand.confidence) }">{{ confidenceLabel(cand.confidence) }}</span>
                  </td>
                  <td>{{ cand.reason_code }}</td>
                  <td>{{ cand.evidence_refs.length }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section class="detail-section">
            <h3><Zap :size="15" />可执行动作候选<span v-if="actionsLoading"> · 加载中…</span></h3>
            <p v-if="!actionsLoading && actions.length === 0" class="compact-empty">暂无可执行动作候选</p>
            <ul v-else class="action-list">
              <li v-for="(action, idx) in actions" :key="idx">
                <div class="action-head">
                  <strong>{{ action.code }}</strong>
                  <span class="mini-badge" :class="{ eligible: action.eligible, blocked: !action.eligible }">{{ action.eligible ? '可执行' : '已阻塞' }}</span>
                </div>
                <small>目标：{{ resourceLabel(action.target) }} · 来源案例 #{{ action.source_case_id }}<span v-if="action.source_candidate_id"> · 候选 #{{ action.source_candidate_id }}</span></small>
                <p v-if="action.blocked_reasons.length" class="action-blocked">阻塞原因：{{ action.blocked_reasons.join('；') }}</p>
              </li>
            </ul>
          </section>
        </template>
      </section>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.cases-toolbar {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 20px;
  padding: 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.field {
  display: grid;
  gap: 4px;
  color: var(--text-secondary);
  font-size: 12px;
}
.field select {
  min-width: 180px;
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}
.field select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}
.case-count {
  margin-left: auto;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.cases-workspace {
  display: grid;
  grid-template-columns: minmax(320px, 0.85fr) minmax(0, 1.15fr);
  align-items: start;
  gap: 14px;
  margin-top: 14px;
}
.cases-list-panel {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}
.list-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 11px 14px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}
.panel-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 600;
}
.list-count {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.panel-empty {
  padding: 40px 16px;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}
.case-list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: calc(100vh - 220px);
  overflow-y: auto;
}
.case-list li {
  border-bottom: 1px solid var(--border-subtle);
}
.case-list li:last-child {
  border-bottom: 0;
}
.case-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-rows: auto auto;
  gap: 6px 10px;
  width: 100%;
  padding: 12px 14px;
  text-align: left;
  background: transparent;
  border: 0;
  cursor: pointer;
}
.case-row:hover {
  background: var(--bg-secondary);
}
.case-row.active {
  background: var(--accent-subtle);
  box-shadow: inset 3px 0 var(--accent-primary);
}
.case-row-main {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.case-row-main strong {
  overflow: hidden;
  color: var(--text-primary);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.case-row-main small {
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.case-row-badges {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}
.case-row-meta {
  grid-column: 1 / -1;
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 11px;
}
.case-row-meta time {
  color: var(--text-muted);
}
.mini-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  font-size: 10px;
  font-weight: 600;
  border: 1px solid currentColor;
  border-radius: var(--radius-full);
  background: var(--bg-elevated);
  white-space: nowrap;
}
.mini-badge.neutral {
  color: var(--text-secondary);
  border-color: var(--border-default);
  background: var(--bg-secondary);
}
.mini-badge.eligible {
  color: var(--status-success);
  border-color: var(--status-success);
  background: var(--success-bg);
}
.mini-badge.blocked {
  color: var(--status-danger);
  border-color: var(--status-danger);
  background: var(--danger-bg);
}
.case-detail-panel {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 18px;
  min-height: 360px;
}
.detail-empty {
  min-height: 360px;
  margin: 0;
  border: 0;
  box-shadow: none;
  background: transparent;
}
.case-detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.case-detail-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.action-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-on-accent, #fff);
  background: var(--accent-primary);
  border: 1px solid var(--accent-primary);
  border-radius: var(--radius-md);
  cursor: pointer;
}
.action-button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.notice-message {
  margin-top: 12px;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--status-success);
  background: color-mix(in srgb, var(--status-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--status-success) 35%, transparent);
  border-radius: var(--radius-md);
}
.case-detail-title {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.case-detail-header h2 {
  margin: 0;
  font-size: 18px;
  color: var(--text-primary);
}
.case-detail-header small {
  color: var(--text-secondary);
  font-size: 11px;
}
.case-detail-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}
.detail-section {
  margin-top: 20px;
}
.detail-section h3 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-primary);
}
.detail-section h3 span {
  color: var(--text-secondary);
  font-weight: 500;
}
.coverage-hint {
  margin: 0 0 8px;
  color: var(--status-warning);
  font-size: 11px;
}
.detail-table {
  width: 100%;
  border-collapse: collapse;
}
.detail-table th {
  padding: 7px 9px;
  color: var(--text-secondary);
  font-size: 10px;
  font-weight: 700;
  text-align: left;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}
.detail-table td {
  padding: 8px 9px;
  color: var(--text-primary);
  font-size: 11px;
  border-bottom: 1px solid var(--border-subtle);
  vertical-align: top;
  overflow-wrap: anywhere;
}
.detail-table td small {
  display: block;
  margin-top: 2px;
  color: var(--text-secondary);
  font-size: 10px;
}
.detail-table tbody tr:last-child td {
  border-bottom: 0;
}
.factor-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 8px;
}
.factor-list li {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.factor-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.factor-head strong {
  color: var(--text-primary);
  font-size: 12px;
}
.factor-head .weight {
  color: var(--accent-primary);
  font-size: 11px;
  font-weight: 600;
}
.factor-list p {
  margin: 4px 0 0;
  color: var(--text-secondary);
  font-size: 12px;
}
.factor-list small {
  color: var(--text-muted);
  font-size: 10px;
}
.action-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 8px;
}
.action-list li {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.action-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.action-head strong {
  color: var(--text-primary);
  font-size: 12px;
}
.action-list small {
  display: block;
  margin-top: 4px;
  color: var(--text-secondary);
  font-size: 11px;
}
.action-blocked {
  margin: 6px 0 0;
  color: var(--status-danger);
  font-size: 11px;
}
.compact-empty {
  margin: 0;
  padding: 20px 0;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}

@media (max-width: 960px) {
  .cases-workspace {
    grid-template-columns: minmax(0, 1fr);
  }
  .case-list {
    max-height: 360px;
  }
}
</style>
