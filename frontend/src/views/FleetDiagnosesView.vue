<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Globe, RefreshCw } from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'

interface FleetDiagnosisRow {
  id: number
  cluster_id: number
  cluster_name?: string
  rule_id: string
  severity: string
  resource_kind: string
  resource_name: string
  resource_namespace?: string
  status: string
  summary: string
  observed_at: string
  resolved_at?: string | null
}

interface FleetDiagnosisStats {
  total: number
  by_status: Record<string, number>
  by_severity: Record<string, number>
  by_cluster: { cluster_id: number; cluster_name?: string; count: number }[]
}

const auth = useAuthStore()
const router = useRouter()

const statusFilter = ref('all')
const severityFilter = ref('all')
const limitInput = ref<number>(50)

const diagnoses = ref<FleetDiagnosisRow[]>([])
const stats = ref<FleetDiagnosisStats | null>(null)
const loading = ref(false)
const statsLoading = ref(false)
const errorMessage = ref('')

const totalCount = computed(() => stats.value?.total ?? 0)
const openCount = computed(() => stats.value?.by_status?.open ?? 0)
const resolvedCount = computed(() => stats.value?.by_status?.resolved ?? 0)

function formatRelativeTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  const diff = Date.now() - date.getTime()
  if (diff < 0) return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date)
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return '刚刚'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}天前`
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date)
}

function severityTone(severity: string): string {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low' || severity === 'info') return 'info'
  return ''
}

function buildQuery(): string {
  const params = new URLSearchParams()
  if (statusFilter.value && statusFilter.value !== 'all') params.set('status', statusFilter.value)
  if (severityFilter.value && severityFilter.value !== 'all') params.set('severity', severityFilter.value)
  const limit = Number(limitInput.value)
  const clamped = Number.isFinite(limit) ? Math.min(200, Math.max(1, Math.trunc(limit))) : 50
  params.set('limit', String(clamped))
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

async function loadStats() {
  statsLoading.value = true
  try {
    const response = await fetch('/api/v1/federation/diagnoses/stats', {
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${auth.accessToken}`,
      },
    })
    if (!response.ok) {
      const body = await response.json().catch(() => undefined) as { message?: string } | undefined
      throw new Error(body?.message || `请求失败 ${response.status}`)
    }
    const data = (await response.json()) as FleetDiagnosisStats
    stats.value = {
      total: data.total ?? 0,
      by_status: data.by_status ?? {},
      by_severity: data.by_severity ?? {},
      by_cluster: data.by_cluster ?? [],
    }
  } catch (err) {
    // stats failure does not block table; keep previous or zeroed
    if (!stats.value) {
      stats.value = { total: 0, by_status: {}, by_severity: {}, by_cluster: [] }
    }
    if (err instanceof Error) {
      // silently keep stats, but show if table also fails
    }
  } finally {
    statsLoading.value = false
  }
}

async function loadDiagnoses() {
  loading.value = true
  errorMessage.value = ''
  try {
    const qs = buildQuery()
    const response = await fetch(`/api/v1/federation/diagnoses${qs}`, {
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${auth.accessToken}`,
      },
    })
    if (!response.ok) {
      const body = await response.json().catch(() => undefined) as { message?: string } | undefined
      throw new Error(body?.message || `请求失败 ${response.status}`)
    }
    const data = (await response.json()) as { items: FleetDiagnosisRow[]; total: number }
    diagnoses.value = data.items ?? []
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '无法加载舰队诊断记录'
    diagnoses.value = []
  } finally {
    loading.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadStats(), loadDiagnoses()])
}

function openDiagnosis(row: FleetDiagnosisRow) {
  void router.push(`/diagnoses/${row.id}`)
}

function onLimitInput(event: Event) {
  const target = event.target as HTMLInputElement
  const next = Number(target.value)
  if (!Number.isFinite(next)) return
  limitInput.value = Math.min(200, Math.max(1, Math.trunc(next)))
}

watch([statusFilter, severityFilter, limitInput], () => {
  void loadDiagnoses()
})

onMounted(() => {
  void refreshAll()
})
</script>

<template>
  <ConsoleLayout eyebrow="故障分析" title="舰队诊断">
    <template #actions>
      <button class="icon-button" type="button" title="刷新" aria-label="刷新舰队诊断" :disabled="loading || statsLoading" @click="refreshAll">
        <RefreshCw :size="18" :class="{ spinning: loading || statsLoading }" />
      </button>
    </template>

    <!-- 标题区：左侧标题 + 右侧汇总数字 -->
    <section class="fleet-header">
      <div class="fleet-title">
        <h2><Globe :size="18" />舰队诊断</h2>
        <span>跨集群聚合的诊断记录，基于已中心化的诊断仓储只读聚合。</span>
      </div>
      <div class="fleet-stats" aria-label="舰队诊断汇总">
        <article class="stat-card"><strong>{{ totalCount }}</strong><span>总计</span></article>
        <article class="stat-card"><strong>{{ openCount }}</strong><span>待处理</span></article>
        <article class="stat-card"><strong>{{ resolvedCount }}</strong><span>已解决</span></article>
      </div>
    </section>

    <!-- 过滤区 -->
    <section class="fleet-filters">
      <label class="filter-field">
        <span>状态</span>
        <select v-model="statusFilter" aria-label="按状态筛选">
          <option value="all">全部</option>
          <option value="open">open</option>
          <option value="confirmed">confirmed</option>
          <option value="resolved">resolved</option>
          <option value="dismissed">dismissed</option>
        </select>
      </label>
      <label class="filter-field">
        <span>严重度</span>
        <select v-model="severityFilter" aria-label="按严重度筛选">
          <option value="all">全部</option>
          <option value="critical">critical</option>
          <option value="high">high</option>
          <option value="medium">medium</option>
          <option value="low">low</option>
          <option value="info">info</option>
        </select>
      </label>
      <label class="filter-field">
        <span>数量限制</span>
        <input
          :value="limitInput"
          type="number"
          min="1"
          max="200"
          step="1"
          aria-label="数量限制"
          @change="onLimitInput"
          @input="onLimitInput"
        />
      </label>
      <button class="secondary-button" type="button" :disabled="loading" @click="loadDiagnoses">
        <RefreshCw :size="14" :class="{ spinning: loading }" />刷新
      </button>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <!-- 表格 -->
    <section class="fleet-diagnoses-panel">
      <div v-if="!loading && diagnoses.length === 0" class="resource-empty">
        <Globe :size="30" />
        <strong>当前舰队无诊断记录</strong>
        <span>调整筛选条件或等待各集群产生新的诊断。</span>
      </div>

      <div v-else class="fleet-table-wrap">
        <table class="compact-table fleet-table">
          <thead>
            <tr>
              <th>集群名</th>
              <th>规则</th>
              <th>严重度</th>
              <th>资源</th>
              <th>状态</th>
              <th>观测时间</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in diagnoses"
              :key="row.id"
              class="fleet-row"
              tabindex="0"
              role="button"
              :aria-label="`查看诊断 ${row.id} 详情`"
              @click="openDiagnosis(row)"
              @keydown.enter.prevent="openDiagnosis(row)"
              @keydown.space.prevent="openDiagnosis(row)"
            >
              <td data-label="集群名">{{ row.cluster_name || `集群 #${row.cluster_id}` }}</td>
              <td data-label="规则"><span class="fleet-rule">{{ row.rule_id }}</span></td>
              <td data-label="严重度"><span class="severity-badge" :class="severityTone(row.severity)">{{ row.severity }}</span></td>
              <td data-label="资源"><span class="fleet-resource">{{ row.resource_kind }}/{{ row.resource_name }}</span><small v-if="row.resource_namespace" class="fleet-ns">{{ row.resource_namespace }}</small></td>
              <td data-label="状态"><span class="workflow-status" :class="row.status">{{ row.status }}</span></td>
              <td data-label="观测时间"><time :datetime="row.observed_at">{{ formatRelativeTime(row.observed_at) }}</time></td>
            </tr>
          </tbody>
        </table>
        <p v-if="loading" class="fleet-loading">加载中…</p>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.fleet-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
  padding: 14px 16px;
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-subtle, #e3e8ea);
  border-radius: 8px;
}
.fleet-title {
  display: grid;
  gap: 4px;
}
.fleet-title h2 {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--text-primary, #2f3d45);
  font-size: 15px;
  font-weight: 700;
}
.fleet-title span {
  color: var(--text-secondary, #5a6672);
  font-size: 11px;
}
.fleet-stats {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.fleet-stats .stat-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 88px;
  padding: 10px 14px;
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-subtle, #e3e8ea);
  border-radius: 8px;
}
.fleet-stats .stat-card strong {
  color: var(--text-primary, #2f3d45);
  font-size: 20px;
  line-height: 1;
}
.fleet-stats .stat-card span {
  color: var(--text-muted, #77858d);
  font-size: 11px;
  margin-top: 4px;
}

.fleet-filters {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
  padding: 12px 16px;
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-subtle, #e3e8ea);
  border-radius: 8px;
}
.filter-field {
  display: grid;
  gap: 4px;
}
.filter-field span {
  color: var(--text-secondary, #5a6672);
  font-size: 11px;
  font-weight: 600;
}
.filter-field select,
.filter-field input {
  min-width: 150px;
  height: 34px;
  padding: 0 10px;
  color: var(--text-primary, #43515a);
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-default, #cfd8dc);
  border-radius: 5px;
  font-size: 12px;
}
.filter-field input {
  min-width: 110px;
  max-width: 120px;
}
.secondary-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 14px;
  color: var(--text-primary, #43515a);
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-default, #cfd8dc);
  border-radius: 5px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.secondary-button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.fleet-diagnoses-panel {
  background: var(--bg-elevated, #ffffff);
  border: 1px solid var(--border-subtle, #e3e8ea);
  border-radius: 8px;
  padding: 4px 14px 12px;
}
.fleet-table-wrap {
  overflow-x: auto;
}
.fleet-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.fleet-table th {
  text-align: left;
  padding: 10px 8px;
  color: var(--text-secondary, #5a6672);
  background: var(--bg-secondary, #f6f7f9);
  font-weight: 600;
  white-space: nowrap;
}
.fleet-table td {
  padding: 10px 8px;
  border-top: 1px solid var(--border-subtle, #eef1f3);
  color: var(--text-primary, #2f3d45);
  vertical-align: middle;
}
.fleet-row {
  cursor: pointer;
  transition: background 160ms ease, transform 160ms ease;
}
.fleet-row:hover {
  background: var(--bg-secondary, #f6f7f9);
}
.fleet-row:focus-visible {
  outline: 2px solid var(--accent-primary, #326ce5);
  outline-offset: -2px;
}
.fleet-rule {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: var(--text-primary, #2f3d45);
  overflow-wrap: anywhere;
}
.fleet-resource {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.fleet-ns {
  display: block;
  color: var(--text-muted, #77858d);
  font-size: 11px;
  line-height: 1.4;
}
.severity-badge {
  display: inline-flex;
  padding: 3px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 650;
  color: var(--text-secondary, #5a6672);
  background: var(--bg-tertiary, #eef1f3);
}
.severity-badge.danger {
  color: #a43b2e;
  background: #fbeae6;
}
.severity-badge.warning {
  color: #8c6225;
  background: #fff3d8;
}
.severity-badge.info {
  color: #3b6ea5;
  background: #e6eef7;
}
.workflow-status {
  display: inline-flex;
  padding: 3px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 650;
}
.workflow-status.open {
  color: #8c6225;
  background: #fff3d8;
}
.workflow-status.confirmed {
  color: #3b6ea5;
  background: #e6eef7;
}
.workflow-status.resolved {
  color: #2e7867;
  background: #e3f1ed;
}
.workflow-status.dismissed {
  color: #5a6672;
  background: #eef1f3;
}
.error-message {
  margin: 0 0 12px;
  padding: 10px 12px;
  color: #8a2a1d;
  background: #fdf0ee;
  border: 1px solid #f3c9c2;
  border-radius: 6px;
  font-size: 12px;
}
.fleet-loading {
  margin: 10px 0 0;
  color: var(--text-muted, #77858d);
  font-size: 12px;
}
.resource-empty {
  display: grid;
  place-items: center;
  gap: 6px;
  padding: 36px 16px;
  text-align: center;
  color: var(--text-muted, #77858d);
}
.resource-empty strong {
  color: var(--text-primary, #2f3d45);
  font-size: 13px;
}
.resource-empty span {
  font-size: 11px;
}

.spinning {
  animation: fleet-spin 0.9s linear infinite;
}
@keyframes fleet-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 响应式：小屏幕下表格不水平滚动，改为紧凑卡片布局 */
@media (max-width: 720px) {
  .fleet-table-wrap {
    overflow-x: visible;
  }
  .fleet-table thead {
    display: none;
  }
  .fleet-table,
  .fleet-table tbody,
  .fleet-table tr,
  .fleet-table td {
    display: block;
    width: 100%;
  }
  .fleet-table tr {
    padding: 10px 8px;
    border-top: 1px solid var(--border-subtle, #eef1f3);
  }
  .fleet-table tr:first-child {
    border-top: 0;
  }
  .fleet-table td {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 6px 0;
    border: 0;
  }
  .fleet-table td::before {
    content: attr(data-label);
    flex: 0 0 auto;
    color: var(--text-muted, #77858d);
    font-size: 11px;
    font-weight: 600;
  }
  .fleet-table td > * {
    text-align: right;
  }
}

@media (prefers-reduced-motion: reduce) {
  .fleet-row {
    transition: none;
  }
  .spinning {
    animation: none;
  }
}
</style>
