<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { BoxIcon, Gauge, RefreshCw, Search, Terminal, TriangleAlert } from 'lucide-vue-next'

import { getClusterDashboard, queryLogs } from '../api/monitoring'
import { listClusters } from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { ClusterDashboardResponse, MonitoringPanel, LogResult } from '../types/monitoring'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const templates = [
  { value: 'node_overview', label: '节点概览' },
  { value: 'workload_overview', label: '工作负载概览' },
  { value: 'pod_overview', label: 'Pod 概览' },
]
const selectedTemplate = ref('node_overview')

const dashboard = ref<ClusterDashboardResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')

// Log query state
const logNamespace = ref('')
const logPod = ref('')
const logContainer = ref('')
const logTextFilter = ref('')
const logStart = ref(defaultStartTime())
const logEnd = ref(defaultEndTime())
const logResult = ref<LogResult | null>(null)
const logsLoading = ref(false)
const logsError = ref('')

const selectedCluster = computed(() => clusters.value.find((item) => item.id === selectedClusterID.value) ?? null)
const panels = computed<MonitoringPanel[]>(() => dashboard.value?.panels ?? [])

function formatDateTimeLocal(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function defaultStartTime(): string {
  return formatDateTimeLocal(new Date(Date.now() - 60 * 60 * 1000))
}

function defaultEndTime(): string {
  return formatDateTimeLocal(new Date())
}

function toIso(localValue: string): string {
  if (!localValue) return ''
  const d = new Date(localValue)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}

function formatTime(value: string): string {
  if (!value) return '--'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(d)
}

async function loadClusters() {
  try {
    const result = await listClusters(auth.accessToken)
    clusters.value = result.items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch {
    errorMessage.value = '无法加载集群列表'
  }
}

async function loadDashboard() {
  const clusterID = selectedClusterID.value
  if (!clusterID) {
    dashboard.value = null
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    dashboard.value = await getClusterDashboard(auth.accessToken, clusterID, selectedTemplate.value)
  } catch (err) {
    dashboard.value = null
    errorMessage.value = err instanceof Error ? err.message : '无法加载监控大盘，请确认集群可达且指标采集已启用'
  } finally {
    loading.value = false
  }
}

async function runLogQuery() {
  const clusterID = selectedClusterID.value
  if (!clusterID || !logNamespace.value.trim()) return
  logsLoading.value = true
  logsError.value = ''
  logResult.value = null
  try {
    const start = toIso(logStart.value)
    const end = toIso(logEnd.value)
    if (!start || !end) {
      logsError.value = '请提供有效的开始与结束时间'
      return
    }
    logResult.value = await queryLogs(auth.accessToken, clusterID, {
      namespace: logNamespace.value.trim(),
      pod: logPod.value.trim() || undefined,
      container: logContainer.value.trim() || undefined,
      text_filter: logTextFilter.value.trim() || undefined,
      start,
      end,
    })
  } catch (err) {
    logResult.value = null
    logsError.value = err instanceof Error ? err.message : '日志查询失败，请确认日志采集后端可用'
  } finally {
    logsLoading.value = false
  }
}

watch(selectedClusterID, () => {
  void loadDashboard()
})

watch(selectedTemplate, () => {
  void loadDashboard()
})

onMounted(loadClusters)
</script>

<template>
  <ConsoleLayout eyebrow="可观测性" title="监控大盘">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="loading || !selectedClusterID" @click="loadDashboard">
        <RefreshCw :size="15" :class="{ spinning: loading }" />刷新
      </button>
    </template>

    <section class="resource-toolbar monitoring-toolbar">
      <select v-model="selectedClusterID" aria-label="选择集群">
        <option :value="null" disabled>选择已启用集群</option>
        <option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option>
      </select>
      <select v-model="selectedTemplate" aria-label="监控模板">
        <option v-for="tpl in templates" :key="tpl.value" :value="tpl.value">{{ tpl.label }}</option>
      </select>
      <button class="secondary-button" type="button" :disabled="loading || !selectedClusterID" @click="loadDashboard">
        <RefreshCw :size="15" :class="{ spinning: loading }" />加载面板
      </button>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <div v-if="clusters.length === 0" class="resource-empty">
      <BoxIcon :size="30" />
      <strong>没有已启用的集群</strong>
      <span>请先接入并启用集群，再查看监控大盘。</span>
    </div>

    <template v-else>
      <!-- Dashboard panels grid -->
      <section class="resource-panel monitoring-panels-panel">
        <div class="section-heading">
          <div>
            <p class="context-label">DASHBOARD PANELS</p>
            <h2>监控面板 · {{ panels.length }}</h2>
          </div>
          <span v-if="dashboard" class="monitoring-meta">模板 {{ dashboard.template }} · {{ selectedCluster?.name || '--' }}</span>
        </div>

        <div v-if="loading" class="empty-state"><RefreshCw class="spinning" :size="24" /><span>正在加载监控面板</span></div>
        <div v-else-if="panels.length === 0" class="empty-state">
          <Gauge :size="28" />
          <strong>当前模板没有可用的监控面板</strong>
          <span>切换模板或确认指标采集组件已就绪。</span>
        </div>
        <div v-else class="panel-grid">
          <article v-for="(panel, idx) in panels" :key="idx" class="panel-card">
            <header>
              <Gauge :size="16" />
              <strong>{{ panel.title }}</strong>
            </header>
            <dl>
              <div><dt>指标</dt><dd class="mono">{{ panel.metric }}</dd></div>
              <div><dt>单位</dt><dd>{{ panel.unit || '—' }}</dd></div>
              <div><dt>资源类型</dt><dd>{{ panel.resource_kind }}</dd></div>
            </dl>
            <p v-if="panel.description" class="panel-desc">{{ panel.description }}</p>
          </article>
        </div>
      </section>

      <!-- Log viewer -->
      <section class="resource-panel log-viewer-panel">
        <div class="section-heading">
          <div>
            <p class="context-label">LOG QUERY</p>
            <h2>日志查询</h2>
          </div>
          <span v-if="logResult" class="monitoring-meta">返回 {{ logResult.TotalReturned }} 条 · 状态 {{ logResult.State || '--' }}</span>
        </div>

        <form class="log-form" @submit.prevent="runLogQuery">
          <label class="log-field log-field-ns">
            <span>Namespace <em>*</em></span>
            <input v-model="logNamespace" type="text" placeholder="default" required />
          </label>
          <label class="log-field">
            <span>Pod</span>
            <input v-model="logPod" type="text" placeholder="可选" />
          </label>
          <label class="log-field">
            <span>Container</span>
            <input v-model="logContainer" type="text" placeholder="可选" />
          </label>
          <label class="log-field">
            <span>文本过滤</span>
            <input v-model="logTextFilter" type="text" placeholder="可选" />
          </label>
          <label class="log-field">
            <span>开始时间</span>
            <input v-model="logStart" type="datetime-local" />
          </label>
          <label class="log-field">
            <span>结束时间</span>
            <input v-model="logEnd" type="datetime-local" />
          </label>
          <button type="submit" class="primary-button" :disabled="logsLoading || !selectedClusterID || !logNamespace.trim()">
            <Search :size="15" />{{ logsLoading ? '查询中…' : '查询日志' }}
          </button>
        </form>

        <p v-if="logsError" class="error-message">{{ logsError }}</p>

        <div v-if="logResult && logResult.Error" class="log-result-error">
          <TriangleAlert :size="14" />{{ logResult.Error }}
        </div>

        <div v-if="logResult" class="log-result-container">
          <div v-if="logResult.Entries.length === 0" class="empty-state log-empty">
            <Terminal :size="24" />
            <strong>当前查询范围没有日志</strong>
            <span>调整 Namespace、Pod 或时间范围后重新查询。</span>
          </div>
          <div v-else>
            <div v-for="(entry, idx) in logResult.Entries" :key="idx" class="log-line">
              <time class="mono">{{ formatTime(entry.Timestamp) }}</time>
              <span class="log-source mono">{{ entry.Namespace }}/{{ entry.Pod }}/{{ entry.Container }}</span>
              <span class="log-text">{{ entry.Line }}</span>
            </div>
          </div>
        </div>
      </section>
    </template>
  </ConsoleLayout>
</template>

<style scoped>
.monitoring-toolbar {
  grid-template-columns: minmax(180px, 240px) minmax(180px, 220px) auto;
  align-items: center;
}

.monitoring-meta {
  color: var(--text-tertiary);
  font-size: 11px;
  white-space: nowrap;
}

.monitoring-panels-panel {
  margin-top: 14px;
  margin-bottom: 18px;
}

.panel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
  margin-top: 14px;
}

.panel-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.panel-card header {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-primary);
}

.panel-card header strong {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-card dl {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin: 0;
}

.panel-card dl > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.panel-card dt {
  color: var(--text-tertiary);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.panel-card dd {
  margin: 0;
  overflow: hidden;
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-desc {
  margin: 0;
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.55;
}

.log-viewer-panel {
  margin-bottom: 30px;
}

.log-form {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) minmax(140px, 1fr) minmax(140px, 1fr) minmax(160px, 1.2fr) minmax(170px, 1fr) minmax(170px, 1fr) auto;
  gap: 10px;
  margin-top: 14px;
  align-items: end;
}

.log-field {
  display: grid;
  gap: 5px;
  min-width: 0;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.log-field em {
  color: var(--status-danger);
  font-style: normal;
}

.log-field input {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-inset);
  outline: none;
}

.log-field input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.log-result-error {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-top: 12px;
  padding: 9px 11px;
  color: var(--status-danger);
  font-size: 12px;
  background: var(--danger-bg);
  border-left: 3px solid var(--status-danger);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.log-result-container {
  max-height: 460px;
  margin-top: 12px;
  overflow: auto;
  background: #17242a;
  border-radius: var(--radius-md);
}

.log-empty {
  min-height: 200px;
  color: #9fb3b8;
}

.log-empty strong {
  color: #cfe0e2;
}

.log-line {
  display: grid;
  grid-template-columns: minmax(150px, 180px) minmax(180px, 260px) minmax(0, 1fr);
  gap: 14px;
  padding: 7px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  font-size: 12px;
  line-height: 1.5;
}

.log-line:last-child {
  border-bottom: 0;
}

.log-line time {
  color: #7fb4c4;
  white-space: nowrap;
}

.log-source {
  color: #9fb3b8;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-text {
  color: #d9e5e3;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.mono {
  font-family: var(--font-mono);
}

@media (max-width: 1100px) {
  .log-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .log-form .primary-button {
    grid-column: 1 / -1;
  }
}

@media (max-width: 720px) {
  .monitoring-toolbar {
    grid-template-columns: 1fr;
  }
  .panel-card dl {
    grid-template-columns: 1fr;
  }
  .log-form {
    grid-template-columns: 1fr;
  }
  .log-line {
    grid-template-columns: 1fr;
    gap: 4px;
  }
}
</style>
