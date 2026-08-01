<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  Boxes,
  ChevronDown,
  Gauge,
  Network,
  RefreshCw,
  Route,
  TrafficCone,
} from 'lucide-vue-next'

import { getTrafficMetrics, listDestinationRules, listVirtualServices } from '../api/servicemesh'
import { listClusters } from '../api/clusters'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { DestinationRuleView, ServiceMetric, TrafficMetrics, VirtualServiceView } from '../types/servicemesh'
import type { RouteDestinationView } from '../types/servicemesh'
import type { Cluster } from '../types/cluster'

type TabKey = 'virtual-services' | 'destination-rules' | 'metrics'

const auth = useAuthStore()

const activeTab = ref<TabKey>('virtual-services')
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const namespaceFilter = ref('')
const topK = ref(10)

const virtualServices = ref<VirtualServiceView[]>([])
const destinationRules = ref<DestinationRuleView[]>([])
const trafficMetrics = ref<TrafficMetrics | null>(null)

const services = computed<ServiceMetric[]>(() => trafficMetrics.value?.services ?? [])

const vsLoading = ref(false)
const vsError = ref('')
const vsTruncated = ref(false)
const drLoading = ref(false)
const drError = ref('')
const drTruncated = ref(false)
const metricsLoading = ref(false)
const metricsError = ref('')

const expandedVSUid = ref<string | null>(null)
const initialized = ref(false)

const tabs: { key: TabKey; label: string; icon: typeof Route }[] = [
  { key: 'virtual-services', label: 'VirtualServices', icon: Route },
  { key: 'destination-rules', label: 'DestinationRules', icon: Network },
  { key: 'metrics', label: '流量指标', icon: Gauge },
]

function formatTime(value?: string): string {
  if (!value) return '--'
  try {
    return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
  } catch {
    return value
  }
}

function joinList(items?: string[]): string {
  if (!items || items.length === 0) return '—'
  return items.join(', ')
}

function destinationsSummary(dest?: RouteDestinationView[]): string {
  if (!dest || dest.length === 0) return '—'
  return dest
    .map((d) => (d.weight != null ? `${d.host} (${d.weight}%)` : d.host))
    .join(', ')
}

function labelsToEntries(map?: Record<string, string>): { key: string; value: string }[] {
  if (!map) return []
  return Object.entries(map).map(([key, value]) => ({ key, value }))
}

function errorRateClass(rate: number): string {
  if (rate > 5) return 'critical'
  if (rate > 1) return 'warning'
  return 'normal'
}

function p99LatencyClass(ms: number): string {
  if (ms > 1000) return 'critical'
  if (ms > 500) return 'warning'
  return 'normal'
}

async function loadClusters() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((c) => c.enabled)
    if (clusters.value.length > 0 && !selectedClusterID.value) {
      selectedClusterID.value = clusters.value[0].id
    }
  } catch (err) {
    vsError.value = err instanceof APIError ? err.message : '加载集群列表失败'
  }
}

async function loadVirtualServices() {
  if (!selectedClusterID.value) {
    virtualServices.value = []
    return
  }
  vsLoading.value = true
  vsError.value = ''
  try {
    const resp = await listVirtualServices(auth.accessToken, selectedClusterID.value, {
      namespace: namespaceFilter.value || undefined,
      limit: 200,
    })
    virtualServices.value = resp.items
    vsTruncated.value = resp.truncated
  } catch (err) {
    vsError.value = err instanceof APIError ? err.message : '加载 VirtualService 失败'
    virtualServices.value = []
  } finally {
    vsLoading.value = false
  }
}

async function loadDestinationRules() {
  if (!selectedClusterID.value) {
    destinationRules.value = []
    return
  }
  drLoading.value = true
  drError.value = ''
  try {
    const resp = await listDestinationRules(auth.accessToken, selectedClusterID.value, {
      namespace: namespaceFilter.value || undefined,
      limit: 200,
    })
    destinationRules.value = resp.items
    drTruncated.value = resp.truncated
  } catch (err) {
    drError.value = err instanceof APIError ? err.message : '加载 DestinationRule 失败'
    destinationRules.value = []
  } finally {
    drLoading.value = false
  }
}

async function loadTrafficMetrics() {
  if (!selectedClusterID.value) {
    trafficMetrics.value = null
    return
  }
  metricsLoading.value = true
  metricsError.value = ''
  try {
    const metrics = await getTrafficMetrics(auth.accessToken, selectedClusterID.value, {
      namespace: namespaceFilter.value || undefined,
      top_k: topK.value > 0 ? topK.value : undefined,
    })
    trafficMetrics.value = metrics
  } catch (err) {
    metricsError.value = err instanceof APIError ? err.message : '加载流量指标失败'
    trafficMetrics.value = null
  } finally {
    metricsLoading.value = false
  }
}

async function loadActiveTab() {
  if (!selectedClusterID.value) return
  if (activeTab.value === 'virtual-services') await loadVirtualServices()
  else if (activeTab.value === 'destination-rules') await loadDestinationRules()
  else await loadTrafficMetrics()
}

function refresh() {
  void loadActiveTab()
}

function toggleVS(vs: VirtualServiceView) {
  expandedVSUid.value = expandedVSUid.value === vs.uid ? null : vs.uid
}

watch(selectedClusterID, () => {
  expandedVSUid.value = null
  void loadActiveTab()
})

watch(namespaceFilter, () => {
  void loadActiveTab()
})

watch(activeTab, () => {
  void loadActiveTab()
})

watch(topK, () => {
  if (activeTab.value === 'metrics') void loadTrafficMetrics()
})

onMounted(async () => {
  await loadClusters()
  initialized.value = true
  if (selectedClusterID.value) await loadActiveTab()
})
</script>

<template>
  <ConsoleLayout eyebrow="可观测性" title="服务网格">
    <template #actions>
      <select
        v-model="selectedClusterID"
        class="cluster-select"
        aria-label="选择集群"
        :disabled="clusters.length === 0"
      >
        <option :value="null" disabled>选择已启用集群</option>
        <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <button
        type="button"
        class="icon-button"
        title="刷新"
        aria-label="刷新服务网格数据"
        :disabled="vsLoading || drLoading || metricsLoading || !selectedClusterID"
        @click="refresh"
      >
        <RefreshCw :size="18" :class="{ spinning: vsLoading || drLoading || metricsLoading }" />
      </button>
    </template>

    <div v-if="!selectedClusterID && initialized" class="resource-empty">
      <Boxes :size="30" />
      <strong>没有已启用的集群</strong>
      <span>接入并启用集群后可查看服务网格配置与流量指标。</span>
    </div>

    <template v-else>
      <nav class="mesh-tabs" aria-label="服务网格视图">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          class="mesh-tab"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          <component :is="tab.icon" :size="16" />
          <span>{{ tab.label }}</span>
        </button>
      </nav>

      <!-- VirtualServices -->
      <section v-if="activeTab === 'virtual-services'" class="tab-panel">
        <div class="mesh-toolbar">
          <label class="field">
            <span>命名空间</span>
            <input
              v-model="namespaceFilter"
              type="text"
              class="compact-input ns-input"
              placeholder="留空查看全部"
            />
          </label>
          <span v-if="vsTruncated" class="truncation-hint">结果已截断，仅显示前 {{ virtualServices.length }} 项</span>
        </div>

        <p v-if="vsError" class="error-message">{{ vsError }}</p>
        <div v-else-if="vsLoading" class="panel-empty">加载中…</div>
        <div v-else-if="virtualServices.length === 0" class="panel-empty">暂无 VirtualService 资源</div>
        <div v-else class="table-scroll">
          <table class="compact-table vs-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>命名空间</th>
                <th>Hosts</th>
                <th>Gateways</th>
                <th>HTTP</th>
                <th>TCP</th>
                <th>目标</th>
                <th>创建时间</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="vs in virtualServices" :key="vs.uid">
                <tr class="vs-row" :class="{ active: expandedVSUid === vs.uid }" @click="toggleVS(vs)">
                  <td class="mono"><strong>{{ vs.name }}</strong></td>
                  <td>{{ vs.namespace }}</td>
                  <td>{{ joinList(vs.hosts) }}</td>
                  <td>{{ joinList(vs.gateways) }}</td>
                  <td class="mono">{{ vs.http_route_count }}</td>
                  <td class="mono">{{ vs.tcp_route_count }}</td>
                  <td class="dest-cell">{{ destinationsSummary(vs.destinations) }}</td>
                  <td>{{ formatTime(vs.created_at) }}</td>
                  <td class="chevron-cell">
                    <ChevronDown :size="14" class="row-chevron" :class="{ open: expandedVSUid === vs.uid }" />
                  </td>
                </tr>
                <tr v-if="expandedVSUid === vs.uid" class="vs-detail-row">
                  <td colspan="9">
                    <div class="vs-detail">
                      <div class="detail-columns">
                        <div class="detail-column">
                          <h4>标签 · {{ labelsToEntries(vs.labels).length }}</h4>
                          <div v-if="labelsToEntries(vs.labels).length === 0" class="detail-empty muted">无标签</div>
                          <ul v-else class="kv-list">
                            <li v-for="entry in labelsToEntries(vs.labels)" :key="entry.key">
                              <span class="mono">{{ entry.key }}</span>
                              <em>{{ entry.value }}</em>
                            </li>
                          </ul>
                        </div>
                        <div class="detail-column">
                          <h4>注解 · {{ labelsToEntries(vs.annotations).length }}</h4>
                          <div v-if="labelsToEntries(vs.annotations).length === 0" class="detail-empty muted">无注解</div>
                          <ul v-else class="kv-list">
                            <li v-for="entry in labelsToEntries(vs.annotations)" :key="entry.key">
                              <span class="mono">{{ entry.key }}</span>
                              <em>{{ entry.value }}</em>
                            </li>
                          </ul>
                        </div>
                        <div class="detail-column">
                          <h4>目标路由 · {{ vs.destinations?.length ?? 0 }}</h4>
                          <div v-if="!vs.destinations || vs.destinations.length === 0" class="detail-empty muted">无目标</div>
                          <table v-else class="compact-table inner-table">
                            <thead>
                              <tr><th>Host</th><th>Subset</th><th>权重</th></tr>
                            </thead>
                            <tbody>
                              <tr v-for="(d, idx) in vs.destinations" :key="idx">
                                <td class="mono">{{ d.host }}</td>
                                <td>{{ d.subset || '—' }}</td>
                                <td class="mono">{{ d.weight != null ? `${d.weight}%` : '—' }}</td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </section>

      <!-- DestinationRules -->
      <section v-else-if="activeTab === 'destination-rules'" class="tab-panel">
        <div class="mesh-toolbar">
          <label class="field">
            <span>命名空间</span>
            <input
              v-model="namespaceFilter"
              type="text"
              class="compact-input ns-input"
              placeholder="留空查看全部"
            />
          </label>
          <span v-if="drTruncated" class="truncation-hint">结果已截断，仅显示前 {{ destinationRules.length }} 项</span>
        </div>

        <p v-if="drError" class="error-message">{{ drError }}</p>
        <div v-else-if="drLoading" class="panel-empty">加载中…</div>
        <div v-else-if="destinationRules.length === 0" class="panel-empty">暂无 DestinationRule 资源</div>
        <div v-else class="table-scroll">
          <table class="compact-table dr-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>命名空间</th>
                <th>Host</th>
                <th>Subset 数</th>
                <th>Subset 名称</th>
                <th>流量策略</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="dr in destinationRules" :key="dr.uid">
                <td class="mono"><strong>{{ dr.name }}</strong></td>
                <td>{{ dr.namespace }}</td>
                <td class="mono">{{ dr.host }}</td>
                <td class="mono">{{ dr.subset_count }}</td>
                <td>{{ joinList(dr.subset_names) }}</td>
                <td>{{ dr.traffic_policy_summary || '—' }}</td>
                <td>{{ formatTime(dr.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 流量指标 -->
      <section v-else class="tab-panel">
        <div class="mesh-toolbar">
          <label class="field">
            <span>命名空间</span>
            <input
              v-model="namespaceFilter"
              type="text"
              class="compact-input ns-input"
              placeholder="留空查看全部"
            />
          </label>
          <label class="field">
            <span>Top K</span>
            <input
              v-model.number="topK"
              type="number"
              min="1"
              max="100"
              class="compact-input topk-input"
            />
          </label>
          <div v-if="trafficMetrics" class="metrics-window">
            <TrafficCone :size="14" />
            <span>窗口 {{ formatTime(trafficMetrics.window_start) }} ~ {{ formatTime(trafficMetrics.window_end) }}</span>
            <span v-if="trafficMetrics.partial" class="partial-badge">部分数据</span>
          </div>
        </div>

        <p v-if="metricsError" class="error-message">{{ metricsError }}</p>
        <div v-else-if="metricsLoading" class="panel-empty">加载中…</div>
        <div v-else-if="services.length === 0" class="panel-empty">暂无流量指标数据</div>
        <div v-else class="table-scroll">
          <table class="compact-table metrics-table">
            <thead>
              <tr>
                <th>服务</th>
                <th>命名空间</th>
                <th>请求速率 (rps)</th>
                <th>错误率 (%)</th>
                <th>P50 (ms)</th>
                <th>P95 (ms)</th>
                <th>P99 (ms)</th>
                <th>总请求</th>
                <th>总错误</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(svc, idx) in services" :key="`${svc.namespace}-${svc.service_name}-${idx}`">
                <td class="mono"><strong>{{ svc.service_name }}</strong></td>
                <td>{{ svc.namespace }}</td>
                <td class="mono">{{ svc.request_rate_rps.toFixed(2) }}</td>
                <td :class="errorRateClass(svc.error_rate_pct)" class="rate-cell mono">
                  {{ svc.error_rate_pct.toFixed(2) }}%
                </td>
                <td class="mono">{{ svc.p50_latency_ms.toFixed(0) }}</td>
                <td class="mono">{{ svc.p95_latency_ms.toFixed(0) }}</td>
                <td :class="p99LatencyClass(svc.p99_latency_ms)" class="rate-cell mono">
                  {{ svc.p99_latency_ms.toFixed(0) }}
                </td>
                <td class="mono">{{ svc.total_requests }}</td>
                <td class="mono" :class="{ 'error-count': svc.total_errors > 0 }">{{ svc.total_errors }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </ConsoleLayout>
</template>

<style scoped>
.cluster-select {
  height: 36px;
  min-width: 180px;
  padding: 0 10px;
  color: var(--text-primary);
  font: inherit;
  font-size: var(--text-sm);
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  outline: none;
  box-shadow: var(--shadow-inset);
}
.cluster-select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.cluster-select:disabled {
  color: var(--text-muted);
  background: var(--bg-tertiary);
  cursor: not-allowed;
}

.mesh-tabs {
  display: inline-flex;
  gap: 3px;
  margin-top: 18px;
  padding: 4px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
}
.mesh-tab {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 7px 14px;
  color: var(--text-secondary);
  font-size: var(--text-sm);
  font-weight: 600;
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
}
.mesh-tab:hover {
  color: var(--text-primary);
  background: var(--bg-elevated);
}
.mesh-tab.active {
  color: var(--accent-primary);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
}

.tab-panel {
  margin-top: 14px;
}

.mesh-toolbar {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
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
  font-weight: 600;
}
.ns-input {
  width: 220px;
}
.topk-input {
  width: 90px;
}
.truncation-hint {
  color: var(--status-warning);
  font-size: 12px;
  font-weight: 600;
}
.metrics-window {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  color: var(--text-tertiary);
  font-size: 12px;
  flex-wrap: wrap;
}
.partial-badge {
  padding: 2px 8px;
  color: var(--status-warning);
  background: var(--warning-bg);
  border-radius: var(--radius-full);
  font-size: 11px;
  font-weight: 600;
}

.table-scroll {
  margin-top: 14px;
  width: 100%;
  overflow-x: auto;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.mono {
  font-family: var(--font-mono);
  font-size: 11px;
}

.vs-table .vs-row {
  cursor: pointer;
  transition: background var(--transition-fast);
}
.vs-table .vs-row:hover {
  background: var(--bg-secondary);
}
.vs-table .vs-row.active {
  background: var(--accent-subtle);
}
.dest-cell {
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chevron-cell {
  width: 24px;
  text-align: center;
}
.row-chevron {
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}
.row-chevron.open {
  transform: rotate(180deg);
}

.vs-detail-row td {
  padding: 0 !important;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle) !important;
}
.vs-detail {
  padding: 16px;
}
.detail-columns {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.detail-column h4 {
  margin: 0 0 10px;
  font-size: 12px;
  color: var(--text-primary);
}
.detail-empty {
  padding: 8px 0;
  font-size: 12px;
}
.kv-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
  max-height: 200px;
  overflow-y: auto;
}
.kv-list li {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
  gap: 8px;
  padding: 5px 8px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  font-size: 11px;
}
.kv-list em {
  color: var(--text-secondary);
  font-style: normal;
  overflow-wrap: anywhere;
}
.inner-table {
  margin: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.rate-cell.critical {
  color: var(--status-danger);
  font-weight: 700;
}
.rate-cell.warning {
  color: var(--status-warning);
  font-weight: 700;
}
.rate-cell.normal {
  color: var(--text-primary);
}
.error-count {
  color: var(--status-danger);
  font-weight: 700;
}

@media (max-width: 960px) {
  .detail-columns {
    grid-template-columns: minmax(0, 1fr);
  }
  .ns-input {
    width: 100%;
  }
  .metrics-window {
    margin-left: 0;
    width: 100%;
  }
}
</style>
