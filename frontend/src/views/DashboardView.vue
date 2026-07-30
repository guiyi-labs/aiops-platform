<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Activity,
  Bell,
  Boxes,
  CheckCircle2,
  ChevronRight,
  CircleDot,
  Cpu,
  Database,
  MemoryStick,
  Network,
  RefreshCw,
  Server,
  Stethoscope,
  TriangleAlert,
} from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { getFleetHealth } from '../api/fleet'
import { diagnoseNodeMetrics, getDiagnosisSummary } from '../api/diagnosis'
import { getReadiness } from '../api/health'
import { APIError } from '../api/auth'
import { evaluateMetricHistory, getMetricHistory } from '../api/metrics-history'
import { listDeployments, listEvents, listNodeMetrics, listNodes, listPodMetrics, listPods, listServices } from '../api/kubernetes'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { DiagnosisRecord, DiagnosisSummary } from '../types/diagnosis'
import type { HealthResponse } from '../types/health'
import type { FleetClusterHealth, FleetHealthResponse, FleetHealthStatus, FleetResourceSummary } from '../types/fleet'
import type { Deployment, KubernetesEvent, NodeMetric, NodeResource, Pod, PodMetric, ServiceResource } from '../types/kubernetes'
import type { MetricHistoryEvaluationResponse, MetricHistoryResponse } from '../types/metrics-history'
import { deploymentHealth, podHealth } from '../utils/resource-health'
import { aggregateNodeAllocatable, aggregateNodeMetrics, cpuMillicores, formatCPU, formatMemory, memoryBytes, rankPodMetrics, utilizationPercent } from '../utils/resource-metrics'
import type { PodMetricSummary } from '../utils/resource-metrics'
import { buildMetricChart, formatMetricValue, METRIC_CHART, metricHistoryWindow } from '../utils/metrics-history'

type LoadState = 'loading' | 'ready' | 'unavailable'
type MetricsState = 'idle' | 'loading' | 'ready' | 'unavailable' | 'error'

const auth = useAuthStore()
const router = useRouter()
const health = ref<HealthResponse | null>(null)
const loadState = ref<LoadState>('loading')
const resourceLoading = ref(false)
const lastError = ref('')
const resourceError = ref('')
const fleetLoading = ref(false)
const fleetError = ref('')
const fleetHealth = ref<FleetHealthResponse | null>(null)
const metricsState = ref<MetricsState>('idle')
const metricRankingMode = ref<'cpu' | 'memory'>('cpu')
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const nodes = ref<NodeResource[]>([])
const pods = ref<Pod[]>([])
const deployments = ref<Deployment[]>([])
const services = ref<ServiceResource[]>([])
const events = ref<KubernetesEvent[]>([])
const nodeMetrics = ref<NodeMetric[]>([])
const podMetrics = ref<PodMetric[]>([])
const podMetricsTotal = ref(0)
const lastSyncedAt = ref<Date | null>(null)
const diagnosisSummary = ref<DiagnosisSummary>({ total: 0, open: 0, confirmed: 0, resolved: 0, dismissed: 0, overdue: 0, recent: [] })
const trendState = ref<'idle' | 'loading' | 'ready' | 'unavailable' | 'error'>('idle')
const trendCpuHistory = ref<MetricHistoryResponse | null>(null)
const trendCpuEvaluation = ref<MetricHistoryEvaluationResponse | null>(null)
const trendMemoryHistory = ref<MetricHistoryResponse | null>(null)
const trendMemoryEvaluation = ref<MetricHistoryEvaluationResponse | null>(null)
const trendError = ref('')
const diagnoseMetricsLoading = ref(false)
const diagnoseMetricsError = ref('')
const diagnoseMetricsRecord = ref<DiagnosisRecord | null>(null)
let requestController: AbortController | null = null

const selectedCluster = computed(() => clusters.value.find((item) => item.id === selectedClusterID.value))
const readyNodes = computed(() => nodes.value.filter((node) => node.status.conditions.some((condition) => condition.type === 'Ready' && condition.status === 'True')).length)
const healthyPods = computed(() => pods.value.filter((pod) => podHealth(pod) === 'healthy').length)
const criticalPods = computed(() => pods.value.filter((pod) => podHealth(pod) === 'critical').length)
const readyDeployments = computed(() => deployments.value.filter((item) => deploymentHealth(item) === 'healthy').length)
const warningEvents = computed(() => events.value
  .filter((item) => item.type === 'Warning')
  .sort((left, right) => eventTimeValue(right) - eventTimeValue(left)))
const pendingCount = computed(() => diagnosisSummary.value.open + diagnosisSummary.value.confirmed)
const checkedAt = computed(() => lastSyncedAt.value
  ? new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(lastSyncedAt.value)
  : '--')
const nodeHealthPercent = computed(() => percentage(readyNodes.value, nodes.value.length))
const podHealthPercent = computed(() => percentage(healthyPods.value, pods.value.length))
const deploymentHealthPercent = computed(() => percentage(readyDeployments.value, deployments.value.length))
const nodeUsage = computed(() => aggregateNodeMetrics(nodeMetrics.value))
const nodeAllocatable = computed(() => aggregateNodeAllocatable(nodes.value, nodeMetrics.value))
const cpuUtilization = computed(() => nodeAllocatable.value ? utilizationPercent(nodeUsage.value.cpuMillicores, nodeAllocatable.value.cpuMillicores) : null)
const memoryUtilization = computed(() => nodeAllocatable.value ? utilizationPercent(nodeUsage.value.memoryBytes, nodeAllocatable.value.memoryBytes) : null)
const topPodConsumers = computed(() => rankPodMetrics(podMetrics.value, metricRankingMode.value))
const topConsumerValue = computed(() => topPodConsumers.value[0]?.[metricRankingMode.value === 'cpu' ? 'cpuMillicores' : 'memoryBytes'] ?? 0)
const fleetHealthyCount = computed(() => fleetHealth.value?.items.filter((item) => item.status === 'healthy').length ?? 0)
const topNodeName = computed(() => {
  if (nodeMetrics.value.length > 0) return nodeMetrics.value[0].metadata.name
  return nodes.value[0]?.metadata.name ?? ''
})
const topNodeAllocatable = computed(() => {
  const nodeName = topNodeName.value
  if (!nodeName) return null
  const node = nodes.value.find((n) => n.metadata.name === nodeName)
  if (!node?.status?.allocatable) return null
  const cpu = cpuMillicores(node.status.allocatable.cpu)
  const memory = memoryBytes(node.status.allocatable.memory)
  if (cpu === null || memory === null || cpu <= 0 || memory <= 0) return null
  return { cpu, memory }
})
const trendCpuThreshold = computed(() => {
  if (!topNodeAllocatable.value) return null
  return Math.round(topNodeAllocatable.value.cpu * 1_000_000 * 0.8)
})
const trendMemoryThreshold = computed(() => {
  if (!topNodeAllocatable.value) return null
  return Math.round(topNodeAllocatable.value.memory * 0.85)
})
const trendCpuChart = computed(() => {
  if (!trendCpuHistory.value) return null
  const model = buildMetricChart(trendCpuHistory.value.points, trendCpuHistory.value.from, trendCpuHistory.value.to, trendCpuHistory.value.series.unit, trendCpuHistory.value.coverage.collections)
  return {
    linePoints: model.points.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '),
    areaPath: model.points.length > 0
      ? `M ${model.points[0].x.toFixed(2)} ${(METRIC_CHART.height - METRIC_CHART.bottom).toFixed(2)} ${model.points.map((p) => `L ${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ')} L ${model.points[model.points.length - 1].x.toFixed(2)} ${(METRIC_CHART.height - METRIC_CHART.bottom).toFixed(2)} Z`
      : '',
  }
})
const trendMemoryChart = computed(() => {
  if (!trendMemoryHistory.value) return null
  const model = buildMetricChart(trendMemoryHistory.value.points, trendMemoryHistory.value.from, trendMemoryHistory.value.to, trendMemoryHistory.value.series.unit, trendMemoryHistory.value.coverage.collections)
  return {
    linePoints: model.points.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '),
    areaPath: model.points.length > 0
      ? `M ${model.points[0].x.toFixed(2)} ${(METRIC_CHART.height - METRIC_CHART.bottom).toFixed(2)} ${model.points.map((p) => `L ${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ')} L ${model.points[model.points.length - 1].x.toFixed(2)} ${(METRIC_CHART.height - METRIC_CHART.bottom).toFixed(2)} Z`
      : '',
  }
})
const trendCpuPeak = computed(() => {
  if (!trendCpuHistory.value?.points.length) return null
  return formatMetricValue(Math.max(...trendCpuHistory.value.points.map((p) => p.value)), trendCpuHistory.value.series.unit)
})
const trendMemoryPeak = computed(() => {
  if (!trendMemoryHistory.value?.points.length) return null
  return formatMetricValue(Math.max(...trendMemoryHistory.value.points.map((p) => p.value)), trendMemoryHistory.value.series.unit)
})
const trendCpuCoverage = computed(() => {
  const c = trendCpuHistory.value?.coverage
  if (!c || c.collections === 0) return null
  const pct = Math.min(100, Math.round((c.points / c.collections) * 100))
  return { pct, degraded: c.missing > 0 || c.unavailable > 0 || c.timed_out > 0 || c.failed > 0 }
})
const trendMemoryCoverage = computed(() => {
  const c = trendMemoryHistory.value?.coverage
  if (!c || c.collections === 0) return null
  const pct = Math.min(100, Math.round((c.points / c.collections) * 100))
  return { pct, degraded: c.missing > 0 || c.unavailable > 0 || c.timed_out > 0 || c.failed > 0 }
})

function percentage(value: number, total: number): number { return total > 0 ? Math.round((value / total) * 100) : 0 }
function eventTimestamp(event: KubernetesEvent): string | undefined { return event.series?.lastObservedTime || event.eventTime || event.lastTimestamp || event.firstTimestamp || event.metadata.creationTimestamp }
function eventTimeValue(event: KubernetesEvent): number { const value = eventTimestamp(event); return value ? new Date(value).getTime() || 0 : 0 }
function formatTime(value?: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '--' : new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}
function formatActivityTime(value: string): string { return formatTime(value) }
function formatUtilization(value: number | null): string { return value === null ? '分母不可用' : `${value.toFixed(1)}% allocatable` }
function consumerValue(item: PodMetricSummary): number { return metricRankingMode.value === 'cpu' ? item.cpuMillicores : item.memoryBytes }
function formatConsumerValue(item: PodMetricSummary): string { return metricRankingMode.value === 'cpu' ? formatCPU(item.cpuMillicores) : formatMemory(item.memoryBytes) }
function consumerWidth(item: PodMetricSummary): string { return `${topConsumerValue.value > 0 ? Math.max(3, (consumerValue(item) / topConsumerValue.value) * 100) : 0}%` }
function fleetStatusLabel(status: FleetHealthStatus): string {
  return { healthy: '健康', degraded: '需关注', partial: '数据不完整', unavailable: '不可用', timed_out: '超时' }[status]
}
function fleetResourceValue(summary: FleetResourceSummary): string { return `${summary.healthy}/${summary.complete ? summary.total : summary.sampled}` }
function fleetResourceTitle(summary: FleetResourceSummary): string { return summary.complete ? `${summary.healthy} / ${summary.total}` : `${summary.healthy} / ${summary.sampled}，总数 ${summary.total}` }
function openWorkloads(kind?: 'Pod' | 'Deployment' | 'Service' | 'Node') {
  const query: Record<string, string> = {}
  if (selectedClusterID.value) query.cluster = String(selectedClusterID.value)
  if (kind) query.kind = kind
  void router.push({ path: '/workloads', query })
}
function openMetricPod(item: PodMetricSummary) {
  void router.push({ path: '/workloads', query: { cluster: String(selectedClusterID.value ?? ''), kind: 'Pod', namespace: item.namespace, name: item.name } })
}

async function selectFleetCluster(item: FleetClusterHealth) {
  if (!clusters.value.some((cluster) => cluster.id === item.cluster_id)) return
  selectedClusterID.value = item.cluster_id
  await loadClusterData()
}

async function refreshHealth() {
  requestController?.abort()
  requestController = new AbortController()
  loadState.value = 'loading'
  try {
    health.value = await getReadiness(requestController.signal)
    loadState.value = 'ready'
  } catch (error) {
    if (requestController.signal.aborted) return
    loadState.value = 'unavailable'
    lastError.value = error instanceof Error ? error.message : '平台就绪探针失败'
  }
}

async function loadOverview() {
  try {
    const [clusterResponse, summary] = await Promise.all([listClusters(auth.accessToken), getDiagnosisSummary(auth.accessToken)])
    clusters.value = clusterResponse.items.filter((item) => item.enabled)
    diagnosisSummary.value = summary
    if (!selectedClusterID.value || !clusters.value.some((item) => item.id === selectedClusterID.value)) selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch { lastError.value = '平台统计数据加载失败' }
}

async function loadFleetHealth() {
  fleetLoading.value = true
  fleetError.value = ''
  try { fleetHealth.value = await getFleetHealth(auth.accessToken) }
  catch { fleetError.value = '多集群健康比较加载失败' }
  finally { fleetLoading.value = false }
}

async function loadClusterResources() {
  const clusterID = selectedClusterID.value
  if (!clusterID) { nodes.value = []; pods.value = []; deployments.value = []; services.value = []; events.value = []; return }
  resourceLoading.value = true
  resourceError.value = ''
  try {
    const [nodeResponse, podResponse, deploymentResponse, serviceResponse, eventResponse] = await Promise.all([
      listNodes(auth.accessToken, clusterID),
      listPods(auth.accessToken, clusterID),
      listDeployments(auth.accessToken, clusterID),
      listServices(auth.accessToken, clusterID),
      listEvents(auth.accessToken, clusterID),
    ])
    nodes.value = nodeResponse.items
    pods.value = podResponse.items
    deployments.value = deploymentResponse.items
    services.value = serviceResponse.items
    events.value = eventResponse.items
    lastSyncedAt.value = new Date()
  } catch { resourceError.value = '集群资源态势读取失败，请检查集群网络和观察账号权限' }
  finally { resourceLoading.value = false }
}

async function loadClusterMetrics() {
  const clusterID = selectedClusterID.value
  nodeMetrics.value = []
  podMetrics.value = []
  podMetricsTotal.value = 0
  if (!clusterID) { metricsState.value = 'idle'; return }
  metricsState.value = 'loading'
  try {
    const [nodeResponse, podResponse] = await Promise.all([
      listNodeMetrics(auth.accessToken, clusterID),
      listPodMetrics(auth.accessToken, clusterID),
    ])
    nodeMetrics.value = nodeResponse.items
    podMetrics.value = podResponse.items
    podMetricsTotal.value = podResponse.total
    metricsState.value = 'ready'
  } catch (error) {
    metricsState.value = error instanceof APIError && error.code === 'METRICS_API_UNAVAILABLE' ? 'unavailable' : 'error'
  }
}

async function loadTrendData() {
  const clusterID = selectedClusterID.value
  const nodeName = topNodeName.value
  if (!clusterID || !nodeName || metricsState.value !== 'ready') {
    trendState.value = 'idle'
    trendCpuHistory.value = null
    trendCpuEvaluation.value = null
    trendMemoryHistory.value = null
    trendMemoryEvaluation.value = null
    return
  }
  trendState.value = 'loading'
  trendError.value = ''
  const { from, to } = metricHistoryWindow(new Date(), 6)
  const abort = new AbortController()
  try {
    const [cpuHistory, memHistory] = await Promise.all([
      getMetricHistory(auth.accessToken, clusterID, { resourceKind: 'Node', name: nodeName, metric: 'cpu', from, to }),
      getMetricHistory(auth.accessToken, clusterID, { resourceKind: 'Node', name: nodeName, metric: 'memory', from, to }),
    ])
    trendCpuHistory.value = cpuHistory
    trendMemoryHistory.value = memHistory
    trendState.value = 'ready'
    if (cpuHistory.points.length >= 2 && trendCpuThreshold.value !== null) {
      try {
        trendCpuEvaluation.value = await evaluateMetricHistory(
          auth.accessToken,
          clusterID,
          { resourceKind: 'Node', name: nodeName, metric: 'cpu', from, to, operator: 'gte', threshold: trendCpuThreshold.value, forSeconds: 60, minimumPoints: 2 },
        )
      } catch {
        trendCpuEvaluation.value = null
      }
    }
    if (memHistory.points.length >= 2 && trendMemoryThreshold.value !== null) {
      try {
        trendMemoryEvaluation.value = await evaluateMetricHistory(
          auth.accessToken,
          clusterID,
          { resourceKind: 'Node', name: nodeName, metric: 'memory', from, to, operator: 'gte', threshold: trendMemoryThreshold.value, forSeconds: 60, minimumPoints: 2 },
        )
      } catch {
        trendMemoryEvaluation.value = null
      }
    }
  } catch (error) {
    if (abort.signal.aborted) return
    trendState.value = 'error'
    trendError.value = error instanceof Error ? error.message : '趋势历史加载失败'
  }
}

async function runDiagnoseNodeMetrics(metric: 'node_cpu' | 'node_memory') {
  const clusterID = selectedClusterID.value
  const nodeName = topNodeName.value
  if (!clusterID || !nodeName) return
  const threshold = metric === 'node_cpu' ? trendCpuThreshold.value : trendMemoryThreshold.value
  if (threshold === null) return
  diagnoseMetricsLoading.value = true
  diagnoseMetricsError.value = ''
  diagnoseMetricsRecord.value = null
  try {
    diagnoseMetricsRecord.value = await diagnoseNodeMetrics(auth.accessToken, clusterID, {
      name: nodeName,
      metric,
      operator: 'gte',
      threshold,
      for_seconds: 60,
      minimum_points: 2,
    })
  } catch (error) {
    const message = error instanceof APIError ? `${error.code}: ${error.message}` : error instanceof Error ? error.message : '指标诊断失败'
    if (error instanceof APIError && error.code === 'NO_RULE_MATCH') {
      diagnoseMetricsError.value = '过去 6 小时内未检测到持续突破阈值的指标'
    } else {
      diagnoseMetricsError.value = message
    }
  } finally {
    diagnoseMetricsLoading.value = false
  }
}

function openDiagnosisDetail(record: DiagnosisRecord) {
  const query: Record<string, string> = { cluster: String(record.cluster_id) }
  void router.push({ path: '/diagnoses', query })
}

async function loadClusterData() {
  await Promise.all([loadClusterResources(), loadClusterMetrics()])
  await loadTrendData()
}

async function refreshAll() {
  lastError.value = ''
  await Promise.all([refreshHealth(), loadOverview(), loadFleetHealth()])
  await loadClusterData()
}

onMounted(refreshAll)
onBeforeUnmount(() => requestController?.abort())
</script>

<template>
  <ConsoleLayout eyebrow="运维工作台" title="集群态势">
    <template #actions>
      <span class="sync-time">同步于 {{ checkedAt }}</span>
      <button type="button" class="icon-button" title="刷新全部数据" aria-label="刷新全部数据" :disabled="loadState === 'loading' || resourceLoading || fleetLoading" @click="refreshAll"><RefreshCw :size="18" :class="{ spinning: loadState === 'loading' || resourceLoading || fleetLoading }" /></button>
    </template>

    <section class="cockpit-context" aria-label="当前集群上下文">
      <div class="cluster-context-copy">
        <span class="live-indicator"><CircleDot :size="14" />LIVE</span>
        <div><strong>{{ selectedCluster?.name || '尚未选择集群' }}</strong><span>{{ selectedCluster?.kubernetes_version || '等待集群数据' }} · {{ selectedCluster?.api_server || '--' }}</span></div>
      </div>
      <div class="cockpit-context-actions">
        <select v-model="selectedClusterID" aria-label="总览集群" @change="loadClusterData"><option :value="null" disabled>选择已启用集群</option><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select>
        <button class="secondary-button" type="button" @click="router.push('/topology')"><Network :size="15" />资源拓扑</button>
        <button class="secondary-button" type="button" @click="openWorkloads()"><Boxes :size="15" />资源列表</button>
      </div>
    </section>

    <section class="fleet-comparison" aria-label="多集群健康比较">
      <header>
        <div><p class="context-label">FLEET HEALTH</p><h2>多集群健康比较</h2></div>
        <span v-if="fleetHealth">{{ fleetHealthyCount }} 健康 / {{ fleetHealth.total }} 已启用<span v-if="fleetHealth.remaining"> · {{ fleetHealth.remaining }} 未载入</span></span>
      </header>
      <p v-if="fleetError" class="fleet-error"><TriangleAlert :size="15" />{{ fleetError }}</p>
      <div v-else-if="fleetLoading && !fleetHealth" class="fleet-empty">正在读取集群健康状态</div>
      <div v-else-if="!fleetHealth?.items.length" class="fleet-empty">没有已启用的集群</div>
      <div v-else class="fleet-table-scroll">
        <table class="fleet-table">
          <thead><tr><th>集群</th><th>状态</th><th>Node</th><th>Pod</th><th>Deployment</th><th>Warning</th><th>耗时</th><th aria-label="切换集群" /></tr></thead>
          <tbody>
            <tr v-for="item in fleetHealth.items" :key="item.cluster_id" :class="{ selected: item.cluster_id === selectedClusterID }">
              <td><strong>{{ item.cluster_name }}</strong><small>{{ item.kubernetes_version || '--' }}</small></td>
              <td><span class="fleet-status" :class="item.status"><i />{{ fleetStatusLabel(item.status) }}</span><small v-if="item.failures.length">{{ item.failures.map((failure) => failure.scope).join(' / ') }}</small></td>
              <td :title="fleetResourceTitle(item.nodes)">{{ fleetResourceValue(item.nodes) }}<small v-if="!item.nodes.complete">采样</small></td>
              <td :title="fleetResourceTitle(item.pods)">{{ fleetResourceValue(item.pods) }}<small v-if="!item.pods.complete">采样</small></td>
              <td :title="fleetResourceTitle(item.deployments)">{{ fleetResourceValue(item.deployments) }}<small v-if="!item.deployments.complete">采样</small></td>
              <td>{{ item.warnings.count }}<small v-if="!item.warnings.complete">/{{ item.warnings.sampled }} 采样</small></td>
              <td>{{ item.duration_ms }}ms</td>
              <td><button class="icon-button compact" type="button" :disabled="item.cluster_id === selectedClusterID" :title="`切换到 ${item.cluster_name}`" :aria-label="`切换到 ${item.cluster_name}`" @click="selectFleetCluster(item)"><ChevronRight :size="16" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <p v-if="lastError || resourceError" class="error-message">{{ lastError || resourceError }}</p>
    <div v-if="metricsState === 'unavailable' || metricsState === 'error'" class="metrics-capability" :class="metricsState"><TriangleAlert :size="16" /><span><strong>资源用量不可用</strong>{{ metricsState === 'unavailable' ? '目标集群未提供 Metrics API' : 'Metrics API 查询失败，资源健康数据仍可用' }}</span></div>

    <section class="cockpit-metrics" aria-label="实时资源指标">
      <article class="cockpit-metric tone-green"><span><Server :size="17" />Node Ready</span><strong>{{ readyNodes }}<small>/{{ nodes.length }}</small></strong><em>{{ nodeHealthPercent }}% 节点就绪</em></article>
      <article class="cockpit-metric tone-blue"><span><Boxes :size="17" />Pod Healthy</span><strong>{{ healthyPods }}<small>/{{ pods.length }}</small></strong><em>{{ criticalPods > 0 ? `${criticalPods} 个严重异常` : '容器状态稳定' }}</em></article>
      <article class="cockpit-metric tone-amber" :class="{ 'metric-unavailable': metricsState !== 'ready' }"><span><Cpu :size="17" />CPU Usage</span><strong>{{ metricsState === 'ready' ? formatCPU(nodeUsage.cpuMillicores) : '--' }}</strong><em>{{ metricsState === 'ready' ? formatUtilization(cpuUtilization) : metricsState === 'loading' ? '正在读取 Metrics API' : '没有可用的真实指标' }}</em></article>
      <article class="cockpit-metric tone-coral" :class="{ 'metric-unavailable': metricsState !== 'ready' }"><span><MemoryStick :size="17" />Memory Usage</span><strong>{{ metricsState === 'ready' ? formatMemory(nodeUsage.memoryBytes) : '--' }}</strong><em>{{ metricsState === 'ready' ? formatUtilization(memoryUtilization) : metricsState === 'loading' ? '正在读取 Metrics API' : '没有可用的真实指标' }}</em></article>
      <article class="cockpit-metric tone-red"><span><Bell :size="17" />Warning</span><strong>{{ warningEvents.length }}</strong><em>最近 100 条 Event</em></article>
      <article class="cockpit-metric tone-neutral"><span><Stethoscope :size="17" />待处置诊断</span><strong>{{ pendingCount }}</strong><em>{{ diagnosisSummary.overdue }} 条已逾期</em></article>
    </section>

    <section v-if="metricsState === 'ready'" class="metrics-consumers" aria-label="Pod 资源消费排行">
      <header>
        <div><p class="context-label">RESOURCE CONSUMERS</p><h2>Pod 实时消费排行</h2><span>当前有界采样 {{ podMetrics.length }} / {{ podMetricsTotal }} Pods</span></div>
        <div class="metric-mode" aria-label="排行指标">
          <button type="button" :class="{ active: metricRankingMode === 'cpu' }" :aria-pressed="metricRankingMode === 'cpu'" @click="metricRankingMode = 'cpu'"><Cpu :size="14" />CPU</button>
          <button type="button" :class="{ active: metricRankingMode === 'memory' }" :aria-pressed="metricRankingMode === 'memory'" @click="metricRankingMode = 'memory'"><MemoryStick :size="14" />Memory</button>
        </div>
      </header>
      <div v-if="topPodConsumers.length === 0" class="metrics-consumers-empty">当前采样没有 Pod 指标</div>
      <div v-else class="metrics-consumer-list">
        <button v-for="(item, index) in topPodConsumers" :key="`${item.namespace}/${item.name}`" type="button" class="metrics-consumer-row" @click="openMetricPod(item)">
          <b>{{ index + 1 }}</b><span><strong>{{ item.name }}</strong><small>{{ item.namespace || 'default' }} · {{ item.containers }} containers</small></span><div><i :style="{ width: consumerWidth(item) }" /></div><em>{{ formatConsumerValue(item) }}</em><ChevronRight :size="15" />
        </button>
      </div>
    </section>

    <section v-if="metricsState === 'ready'" class="trend-consumers" aria-label="节点历史趋势">
      <header>
        <div><p class="context-label">METRICS TREND</p><h2>节点历史趋势（6h）</h2><span v-if="topNodeName">节点：{{ topNodeName }}</span></div>
        <div class="trend-actions">
          <span v-if="trendState === 'loading'" class="trend-loading">加载中...</span>
          <button v-if="trendState === 'ready' && trendCpuEvaluation?.state === 'firing'" type="button" class="trend-action-btn" :disabled="diagnoseMetricsLoading" @click="runDiagnoseNodeMetrics('node_cpu')">
            <Stethoscope :size="14" />{{ diagnoseMetricsLoading ? '诊断中...' : '诊断 CPU 突破' }}
          </button>
          <button v-if="trendState === 'ready' && trendMemoryEvaluation?.state === 'firing'" type="button" class="trend-action-btn" :disabled="diagnoseMetricsLoading" @click="runDiagnoseNodeMetrics('node_memory')">
            <Stethoscope :size="14" />{{ diagnoseMetricsLoading ? '诊断中...' : '诊断内存突破' }}
          </button>
        </div>
      </header>
      <div v-if="trendState === 'error'" class="trend-error"><TriangleAlert :size="15" />{{ trendError }}</div>
      <div v-else-if="trendState === 'idle'" class="trend-empty">选择集群后自动查询历史指标</div>
      <div v-else class="trend-grid">
        <article class="trend-card">
          <header>
            <span><Cpu :size="15" />CPU Usage</span>
            <em v-if="trendCpuPeak">峰值 {{ trendCpuPeak }}</em>
          </header>
          <div v-if="trendCpuChart" class="trend-chart-wrapper">
            <svg class="trend-chart" :viewBox="`0 0 ${METRIC_CHART.width} ${METRIC_CHART.height}`" preserveAspectRatio="none">
              <path class="trend-chart-area" :d="trendCpuChart.areaPath" />
              <polyline class="trend-chart-line" :points="trendCpuChart.linePoints" />
            </svg>
          </div>
          <div v-if="trendCpuCoverage" class="trend-meta">
            <span>覆盖 {{ trendCpuCoverage.pct }}%</span>
            <span v-if="trendCpuCoverage.degraded" class="degraded">存在缺失</span>
            <span v-if="trendCpuEvaluation" :class="['eval-state', trendCpuEvaluation.state]">
              {{ trendCpuEvaluation.state === 'firing' ? 'Firing' : trendCpuEvaluation.state === 'normal' ? 'Normal' : 'Insufficient' }}
            </span>
          </div>
        </article>
        <article class="trend-card">
          <header>
            <span><MemoryStick :size="15" />Memory Usage</span>
            <em v-if="trendMemoryPeak">峰值 {{ trendMemoryPeak }}</em>
          </header>
          <div v-if="trendMemoryChart" class="trend-chart-wrapper">
            <svg class="trend-chart" :viewBox="`0 0 ${METRIC_CHART.width} ${METRIC_CHART.height}`" preserveAspectRatio="none">
              <path class="trend-chart-area memory" :d="trendMemoryChart.areaPath" />
              <polyline class="trend-chart-line memory" :points="trendMemoryChart.linePoints" />
            </svg>
          </div>
          <div v-if="trendMemoryCoverage" class="trend-meta">
            <span>覆盖 {{ trendMemoryCoverage.pct }}%</span>
            <span v-if="trendMemoryCoverage.degraded" class="degraded">存在缺失</span>
            <span v-if="trendMemoryEvaluation" :class="['eval-state', trendMemoryEvaluation.state]">
              {{ trendMemoryEvaluation.state === 'firing' ? 'Firing' : trendMemoryEvaluation.state === 'normal' ? 'Normal' : 'Insufficient' }}
            </span>
          </div>
        </article>
      </div>
      <div v-if="diagnoseMetricsError" class="trend-diagnosis-result error"><TriangleAlert :size="15" />{{ diagnoseMetricsError }}</div>
      <div v-else-if="diagnoseMetricsRecord" class="trend-diagnosis-result success" @click="openDiagnosisDetail(diagnoseMetricsRecord!)">
        <CheckCircle2 :size="15" />
        <span><strong>{{ diagnoseMetricsRecord.summary }}</strong><small>规则：{{ diagnoseMetricsRecord.rule_id }} · 严重度：{{ diagnoseMetricsRecord.severity }}</small></span>
        <em>查看详情<ChevronRight :size="14" /></em>
      </div>
    </section>

    <section class="dashboard-workspace">
      <div class="dashboard-workspace-panel health-overview">
        <div class="section-heading"><div><p class="context-label">RESOURCE HEALTH</p><h2>工作负载健康度</h2></div><span>{{ services.length }} Services</span></div>
        <div class="health-meter-list">
          <article><div><strong>Node</strong><span>{{ readyNodes }} / {{ nodes.length }} Ready</span></div><div class="health-track"><span class="healthy" :style="{ width: `${nodeHealthPercent}%` }" /></div><b>{{ nodeHealthPercent }}%</b></article>
          <article><div><strong>Pod</strong><span>{{ healthyPods }} / {{ pods.length }} Healthy</span></div><div class="health-track"><span :class="criticalPods > 0 ? 'critical' : 'healthy'" :style="{ width: `${podHealthPercent}%` }" /></div><b>{{ podHealthPercent }}%</b></article>
          <article><div><strong>Deployment</strong><span>{{ readyDeployments }} / {{ deployments.length }} Available</span></div><div class="health-track"><span class="warning" :style="{ width: `${deploymentHealthPercent}%` }" /></div><b>{{ deploymentHealthPercent }}%</b></article>
        </div>
        <button class="panel-link" type="button" @click="openWorkloads()">打开资源工作台<ChevronRight :size="15" /></button>
      </div>

      <div class="dashboard-workspace-panel warning-overview">
        <div class="section-heading"><div><p class="context-label">LATEST SIGNALS</p><h2>最近 Warning</h2></div><button class="panel-link" type="button" @click="router.push('/events')">全部事件<ChevronRight :size="15" /></button></div>
        <div v-if="warningEvents.length === 0" class="empty-state compact-dashboard-empty"><CheckCircle2 :size="25" /><strong>没有 Warning Event</strong><span>当前集群最近事件保持平稳</span></div>
        <button v-for="event in warningEvents.slice(0, 4)" v-else :key="event.metadata.uid || `${event.reason}-${event.involvedObject.name}`" class="signal-row" type="button" @click="router.push('/events')"><TriangleAlert :size="16" /><span><strong>{{ event.reason || 'Warning' }}</strong><small>{{ event.involvedObject.kind }}/{{ event.involvedObject.name }}</small></span><time>{{ formatTime(eventTimestamp(event)) }}</time></button>
      </div>

      <div class="dashboard-workspace-panel diagnosis-overview">
        <div class="section-heading"><div><p class="context-label">DIAGNOSIS QUEUE</p><h2>待处置队列</h2></div><span>{{ diagnosisSummary.total }} total</span></div>
        <div v-if="diagnosisSummary.recent.length === 0" class="empty-state compact-dashboard-empty"><Stethoscope :size="25" /><strong>暂无诊断记录</strong></div>
        <button v-for="item in diagnosisSummary.recent.slice(0, 4)" v-else :key="item.id" type="button" class="dashboard-activity-row" @click="router.push('/diagnoses')"><span class="workflow-status" :class="item.status">{{ item.status }}</span><span><strong>{{ item.summary }}</strong><small>{{ item.resource.kind }}/{{ item.resource.namespace }}/{{ item.resource.name }}</small></span><time>{{ formatActivityTime(item.updated_at || item.created_at) }}</time></button>
      </div>

      <div class="dashboard-workspace-panel platform-overview">
        <div class="section-heading"><div><p class="context-label">PLATFORM</p><h2>平台依赖</h2></div><span class="status-pill" :class="loadState"><span class="status-dot" />{{ loadState === 'ready' ? '运行正常' : loadState === 'loading' ? '检测中' : '服务不可用' }}</span></div>
        <div class="service-list compact-service-list">
          <div class="service-row"><span class="service-icon"><Activity :size="18" /></span><div class="service-name"><strong>API 服务</strong><span>{{ health?.version ?? '--' }}</span></div><CheckCircle2 v-if="loadState === 'ready'" :size="19" class="success-icon" /><TriangleAlert v-else :size="19" class="error-icon" /></div>
          <div class="service-row"><span class="service-icon"><Database :size="18" /></span><div class="service-name"><strong>PostgreSQL</strong><span>readiness dependency</span></div><CheckCircle2 v-if="loadState === 'ready'" :size="19" class="success-icon" /><TriangleAlert v-else :size="19" class="error-icon" /></div>
        </div>
      </div>
    </section>
  </ConsoleLayout>
</template>
