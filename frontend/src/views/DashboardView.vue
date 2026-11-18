<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { Activity, ArrowUpRight, Boxes, Check, CircleDot, Cpu, Database, Expand, Layers3, Minimize2, Network, RefreshCw, ScanLine, Server, ShieldCheck, TriangleAlert } from 'lucide-vue-next'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import DashboardTrend from '../components/DashboardTrend.vue'
import { listClusters } from '../api/clusters'
import { listNodes, listPods, listEvents, listNodeMetrics, listPodMetrics } from '../api/kubernetes'
import { listDiagnoses } from '../api/diagnosis'
import { getReadiness } from '../api/health'
import { useAuthStore } from '../stores/auth'
import { aggregateNodeAllocatable, aggregateNodeMetrics, cpuMillicores, formatCPU, formatMemory, memoryBytes, rankPodMetrics, utilizationPercent } from '../utils/resource-metrics'
import { podHealth } from '../utils/resource-health'
import type { Cluster } from '../types/cluster'
import type { DiagnosisRecord, DiagnosisStatus } from '../types/diagnosis'
import type { KubernetesEvent, NodeMetric, NodeResource, Pod, PodMetric } from '../types/kubernetes'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selected = ref(0)
const loading = ref(false)
const initialized = ref(false)
const error = ref('')
const ready = ref<boolean | null>(null)
const nodes = ref<NodeResource[] | null>(null)
const pods = ref<Pod[] | null>(null)
const events = ref<KubernetesEvent[] | null>(null)
const diagnoses = ref<DiagnosisRecord[] | null>(null)
const nodeMetrics = ref<NodeMetric[] | null>(null)
const podMetrics = ref<PodMetric[] | null>(null)
const nodeCount = ref<number | null>(null)
const podCount = ref<number | null>(null)
const diagnosisCount = ref<number | null>(null)
const truncated = ref<string[]>([])
const refreshKey = ref(0)
const updatedAt = ref('')
const autoRefresh = ref(true)
const presentation = ref(false)
const dashboard = ref<HTMLElement | null>(null)
const presentButton = ref<HTMLButtonElement | null>(null)
const rankMode = ref<'cpu' | 'memory'>('cpu')
let sequence = 0
let timer: ReturnType<typeof setInterval> | undefined
let previousOverflow = ''
const current = computed(() => clusters.value.find(item => item.id === selected.value))
const isNodeReady = (node: NodeResource) => node.status.conditions.some(condition => condition.type === 'Ready' && condition.status === 'True')
const readyNodes = computed(() => nodes.value?.filter(isNodeReady).length ?? null)
const warningCount = computed(() => events.value?.filter(item => item.type === 'Warning').length ?? null)
const runningPods = computed(() => pods.value?.filter(pod => pod.status.phase === 'Running').length ?? null)
const healthyPods = computed(() => pods.value?.filter(pod => podHealth(pod) === 'healthy').length ?? 0)
const podDistribution = computed(() => [
  { label: '健康', count: healthyPods.value, color: 'var(--dash-green)' },
  { label: '关注', count: pods.value?.filter(pod => podHealth(pod) === 'warning').length ?? 0, color: 'var(--dash-amber)' },
  { label: '异常', count: pods.value?.filter(pod => podHealth(pod) === 'critical').length ?? 0, color: 'var(--dash-red)' },
  { label: '未知', count: pods.value?.filter(pod => podHealth(pod) === 'unknown').length ?? 0, color: 'var(--text-muted)' },
])
const podRing = computed(() => {
  let offset = 0
  return podDistribution.value.map(item => {
    const length = pods.value?.length ? item.count / pods.value.length * 100 : 0
    const segment = { ...item, length, offset: -offset }
    offset += length
    return segment
  })
})
const validMetrics = computed(() => (nodeMetrics.value ?? []).filter(item => cpuMillicores(item.usage.cpu) !== null && memoryBytes(item.usage.memory) !== null))
const totals = computed(() => aggregateNodeMetrics(validMetrics.value))
const capacity = computed(() => aggregateNodeAllocatable(nodes.value ?? [], validMetrics.value))
const gauges = computed(() => [
  { label: 'CPU', value: capacity.value ? utilizationPercent(totals.value.cpuMillicores, capacity.value.cpuMillicores) : null, detail: validMetrics.value.length ? formatCPU(totals.value.cpuMillicores) : '暂无指标', color: 'var(--dash-cyan)', icon: Cpu },
  { label: '内存', value: capacity.value ? utilizationPercent(totals.value.memoryBytes, capacity.value.memoryBytes) : null, detail: validMetrics.value.length ? formatMemory(totals.value.memoryBytes) : '暂无指标', color: 'var(--dash-violet)', icon: Database },
])
const rank = computed(() => rankPodMetrics((podMetrics.value ?? []).filter(item => item.containers.length && item.containers.every(container => cpuMillicores(container.usage.cpu) !== null && memoryBytes(container.usage.memory) !== null)), rankMode.value))
const rankValue = (item: { cpuMillicores: number; memoryBytes: number }) => rankMode.value === 'cpu' ? item.cpuMillicores : item.memoryBytes
const rankWidth = (item: { cpuMillicores: number; memoryBytes: number }) => rank.value.length && rankValue(rank.value[0]) > 0 ? rankValue(item) / rankValue(rank.value[0]) * 100 : 0
const statusLabels: Record<DiagnosisStatus, string> = { open: '待确认', confirmed: '已确认', resolved: '已解决', dismissed: '已忽略' }
const statusRows = computed(() => (['open', 'confirmed', 'resolved', 'dismissed'] as const).map((status, index) => ({ status, label: statusLabels[status], count: diagnoses.value?.filter(item => item.status === status).length ?? 0, color: ['var(--dash-amber)', 'var(--dash-red)', 'var(--dash-green)', 'var(--dash-violet)'][index] })))
const topologyNodes = computed(() => (nodes.value ?? []).slice(0, 6).map((node, index, items) => {
  const angle = -Math.PI / 2 + index * Math.PI * 2 / items.length
  return { name: node.metadata.name, ready: isNodeReady(node), x: 260 + Math.cos(angle) * 162, y: 145 + Math.sin(angle) * 96, pods: pods.value?.filter(pod => pod.spec.nodeName === node.metadata.name).length ?? null }
}))
const latestEvents = computed(() => [...(events.value ?? [])].filter(item => item.type === 'Warning').sort((a, b) => Date.parse(b.lastTimestamp || b.metadata.creationTimestamp || '') - Date.parse(a.lastTimestamp || a.metadata.creationTimestamp || '')).slice(0, 3))
function timestamp(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) }

async function load() {
  const request = ++sequence
  const clusterID = selected.value
  if (!clusterID) return
  loading.value = true
  error.value = ''
  // A refresh keeps its snapshot visible until every independent read settles.
  const results = await Promise.allSettled([
    listNodes(auth.accessToken, clusterID), listPods(auth.accessToken, clusterID),
    listEvents(auth.accessToken, clusterID), listDiagnoses(auth.accessToken, { clusterID, limit: 100 }),
    listNodeMetrics(auth.accessToken, clusterID), listPodMetrics(auth.accessToken, clusterID),
  ])
  if (request !== sequence) return
  const [nodeResult, podResult, eventResult, diagnosisResult, metricResult, podMetricResult] = results
  nodes.value = nodeResult.status === 'fulfilled' ? nodeResult.value.items : null
  nodeCount.value = nodeResult.status === 'fulfilled' ? nodeResult.value.total : null
  pods.value = podResult.status === 'fulfilled' ? podResult.value.items : null
  podCount.value = podResult.status === 'fulfilled' ? podResult.value.total : null
  events.value = eventResult.status === 'fulfilled' ? eventResult.value.items : null
  diagnoses.value = diagnosisResult.status === 'fulfilled' ? diagnosisResult.value.items : null
  diagnosisCount.value = diagnosisResult.status === 'fulfilled' ? diagnosisResult.value.total : null
  nodeMetrics.value = metricResult.status === 'fulfilled' ? metricResult.value.items : null
  podMetrics.value = podMetricResult.status === 'fulfilled' ? podMetricResult.value.items : null
  truncated.value = results.flatMap((result, index) => result.status === 'fulfilled' && result.value.remaining > 0 ? [['节点', 'Pod', '事件', '诊断', '节点指标', 'Pod 指标'][index]] : [])
  if (results.some(result => result.status === 'rejected')) error.value = '部分数据暂不可用；对应图表已标记，请检查连接或读取权限。'
  updatedAt.value = new Date().toISOString()
  refreshKey.value++
  loading.value = false
}
function changeCluster() {
  nodes.value = pods.value = events.value = diagnoses.value = nodeMetrics.value = podMetrics.value = null
  nodeCount.value = podCount.value = diagnosisCount.value = null
  truncated.value = []
  updatedAt.value = ''
  void load()
}
async function initialize() {
  void getReadiness().then(value => { ready.value = value.status === 'ready' || value.status === 'ok' }).catch(() => { ready.value = false })
  try {
    clusters.value = (await listClusters(auth.accessToken)).items
    selected.value = clusters.value.find(item => item.enabled)?.id ?? clusters.value[0]?.id ?? 0
    await load()
  } catch { error.value = '无法读取集群配置，请稍后重试。' }
  finally { initialized.value = true }
}
async function enterPresentation() {
  previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  presentation.value = true
  await nextTick()
  dashboard.value?.focus()
  // The expanded canvas also works when the browser does not support Fullscreen.
  try { await dashboard.value?.requestFullscreen?.() } catch { /* Keep expanded mode. */ }
}
async function exitPresentation() {
  presentation.value = false
  document.body.style.overflow = previousOverflow
  if (document.fullscreenElement === dashboard.value) await document.exitFullscreen().catch(() => {})
  await nextTick()
  presentButton.value?.focus()
}
function onFullscreenChange() { if (!document.fullscreenElement && presentation.value) void exitPresentation() }
function onKeydown(event: KeyboardEvent) {
  if (!presentation.value) return
  if (event.key === 'Escape') void exitPresentation()
  if (event.key === 'Tab') {
    const elements = [...(dashboard.value?.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], select:not(:disabled)') ?? [])].filter(element => element.getClientRects().length)
    const first = elements[0]
    const last = elements.at(-1)
    if (event.shiftKey && (document.activeElement === first || document.activeElement === dashboard.value)) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && (document.activeElement === last || document.activeElement === dashboard.value)) { event.preventDefault(); first?.focus() }
  }
}
onMounted(() => {
  void initialize()
  timer = setInterval(() => { if (autoRefresh.value && !loading.value && !document.hidden && selected.value) void load() }, 30_000)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  sequence++
  clearInterval(timer)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('keydown', onKeydown)
  if (presentation.value) { document.body.style.overflow = previousOverflow; if (document.fullscreenElement === dashboard.value) void document.exitFullscreen().catch(() => {}) }
})
</script>

<template>
  <ConsoleLayout eyebrow="K8s 故障诊断" title="系统概览">
    <Teleport to="body" :disabled="!presentation">
      <div ref="dashboard" class="dashboard" :class="{ 'dashboard-presentation': presentation }" :tabindex="presentation ? -1 : undefined" :role="presentation ? 'dialog' : undefined" :aria-modal="presentation ? true : undefined" aria-label="集群运行态势">
        <header class="dash-hero">
          <div class="hero-orbit" aria-hidden="true"><ScanLine :size="62" /><i /><i /></div>
          <div class="hero-copy"><span class="hero-eyebrow"><i /> CLUSTER OBSERVATORY <span>/ 运行观测</span></span><h2>集群运行态势<span class="hero-slash"> / </span><small>从资源状态到故障线索</small></h2><p><span :class="{ 'service-unready': ready === false }">{{ ready === null ? '正在连接平台' : ready ? '平台服务就绪' : '平台服务暂不可用' }}</span><b>·</b>{{ current?.name ?? '等待接入集群' }}<b>·</b>{{ updatedAt ? `最近读取 ${timestamp(updatedAt)}` : '等待采集快照' }}</p></div>
          <div class="hero-actions">
            <select v-if="clusters.length" v-model="selected" aria-label="当前诊断集群" @change="changeCluster"><option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option></select>
            <div class="hero-buttons"><button type="button" class="dash-action" :disabled="loading || !selected" aria-label="刷新概览" @click="load"><RefreshCw :size="14" :class="{ 'dash-spinning': loading }" />刷新</button><button v-if="!presentation" ref="presentButton" type="button" class="dash-action dash-action-primary" @click="enterPresentation"><Expand :size="14" />全屏展示</button><button v-else type="button" class="dash-action dash-action-primary" @click="exitPresentation"><Minimize2 :size="14" />退出展示</button></div>
          </div>
        </header>
        <div class="dash-status-strip"><span><i :class="{ paused: !autoRefresh }" />{{ loading ? '正在更新快照…' : '集群观测快照' }}</span><button type="button" :aria-pressed="autoRefresh" @click="autoRefresh = !autoRefresh"><Check v-if="autoRefresh" :size="12" />30 秒自动刷新{{ autoRefresh ? '已开启' : '已暂停' }}</button></div>
        <p v-if="error" class="error-message" role="alert">{{ error }}<button v-if="!clusters.length" class="dash-retry" @click="initialize">重试连接</button></p>
        <section v-if="initialized && !clusters.length" class="dash-panel overview-panel"><h2>接入实验集群</h2><p>配置集群连接后，即可查看资源、分析故障并积累历史案例。</p><RouterLink to="/settings/clusters">配置集群连接 →</RouterLink></section>
        <template v-else>
          <section class="dash-stats" aria-label="集群关键指标" :aria-busy="loading">
            <article class="dash-stat" style="--stat-color: var(--dash-cyan)"><div><span>集群节点</span><Server :size="19" /></div><strong>{{ nodeCount ?? '—' }}<small>NODES</small></strong><footer><i />就绪 {{ readyNodes ?? '—' }} / {{ nodes?.length ?? '—' }} 个已读取节点</footer></article>
            <article class="dash-stat" style="--stat-color: var(--dash-violet)"><div><span>容器组</span><Boxes :size="19" /></div><strong>{{ podCount ?? '—' }}<small>PODS</small></strong><footer><i />运行中 {{ runningPods ?? '—' }} · 已读取 {{ pods?.length ?? '—' }}</footer></article>
            <article class="dash-stat" style="--stat-color: var(--dash-amber)"><div><span>警告事件</span><TriangleAlert :size="19" /></div><strong>{{ warningCount ?? '—' }}<small>WARNINGS</small></strong><footer><i />当前事件样本 · 共读取 {{ events?.length ?? '—' }} 条</footer></article>
            <article class="dash-stat" style="--stat-color: var(--dash-green)"><div><span>诊断记录</span><ShieldCheck :size="19" /></div><strong>{{ diagnosisCount ?? '—' }}<small>DIAGNOSES</small></strong><footer><i />当前集群 · 分布取最近 {{ diagnoses?.length ?? '—' }} 条</footer></article>
          </section>
          <div class="dash-main-grid">
            <section class="dash-panel pressure-panel" aria-labelledby="pressure-title">
              <header class="dash-panel-heading"><div><span class="dash-kicker">RESOURCE LOAD</span><h2 id="pressure-title"><Cpu :size="16" />资源水位</h2></div><span class="dash-badge">快照</span></header>
              <div class="gauge-pair"><div v-for="gauge in gauges" :key="gauge.label" class="resource-gauge" :style="{ '--gauge-color': gauge.color }"><svg viewBox="0 0 112 112" aria-hidden="true"><circle cx="56" cy="56" r="46" class="gauge-track" /><circle cx="56" cy="56" r="46" class="gauge-value" pathLength="100" :stroke-dasharray="`${Math.max(0, Math.min(100, gauge.value ?? 0))} 100`" transform="rotate(-90 56 56)" /></svg><div class="gauge-label"><component :is="gauge.icon" :size="16" /><strong>{{ gauge.value ?? '—' }}<small v-if="gauge.value !== null">%</small></strong></div><b>{{ gauge.label }}</b><span>{{ gauge.detail }}</span></div></div>
              <footer class="dash-footnote" :title="validMetrics.map(item => `${item.metadata.name} · 源时间 ${timestamp(item.timestamp)}`).join('；')">{{ validMetrics.length }} / {{ nodeCount ?? '—' }} 个节点有有效指标<span>占对应节点可分配资源</span></footer>
              <div class="pressure-rule"><span>Pod 健康分布</span><strong>{{ pods === null ? '不可用' : `${healthyPods} / ${pods.length}` }}</strong></div>
              <div class="pod-health"><svg viewBox="0 0 72 72" role="img" :aria-label="pods === null ? 'Pod 健康分布不可用' : podDistribution.map(item => `${item.label} ${item.count}`).join('，')"><circle cx="36" cy="36" r="27" class="gauge-track" /><circle v-for="segment in podRing" :key="segment.label" cx="36" cy="36" r="27" fill="none" :stroke="segment.color" stroke-width="8" pathLength="100" :stroke-dasharray="`${segment.length} ${100 - segment.length}`" :stroke-dashoffset="segment.offset" transform="rotate(-90 36 36)" /><text x="36" y="40" text-anchor="middle">{{ pods?.length ?? '—' }}</text></svg><div><span v-for="item in podDistribution" :key="item.label"><i :style="{ background: item.color }" />{{ item.label }}<b>{{ pods === null ? '—' : item.count }}</b></span></div></div>
            </section>
            <section class="dash-panel topology-panel" aria-labelledby="topology-title">
              <header class="dash-panel-heading"><div><span class="dash-kicker">CLUSTER MAP</span><h2 id="topology-title"><Network :size="16" />集群资源拓扑</h2></div><RouterLink :to="{ path: '/diagnoses', query: { cluster: selected, view: 'topology' } }" aria-label="查看完整资源拓扑"><ArrowUpRight :size="18" /></RouterLink></header>
              <svg class="cluster-map" viewBox="0 0 520 292" role="img" :aria-label="`当前集群展示 ${topologyNodes.length} 个节点及其 Pod 分布`">
                <defs><radialGradient id="cluster-map-glow"><stop offset="0%" stop-color="var(--map-cyan)" stop-opacity=".13" /><stop offset="100%" stop-color="var(--map-cyan)" stop-opacity="0" /></radialGradient></defs>
                <circle cx="260" cy="145" r="143" fill="url(#cluster-map-glow)" /><ellipse cx="260" cy="145" rx="162" ry="96" class="map-orbit" /><ellipse cx="260" cy="145" rx="105" ry="64" class="map-orbit inner" />
                <g v-for="node in topologyNodes" :key="node.name" class="map-node" :class="{ 'map-node-warning': !node.ready }"><path :d="`M 260 145 Q ${node.x} 145 ${node.x} ${node.y}`" class="map-connection" /><circle :cx="node.x" :cy="node.y" r="19" class="map-node-halo" /><rect :x="node.x - 12" :y="node.y - 12" width="24" height="24" rx="7" class="map-node-body" /><path :d="`M ${node.x - 5} ${node.y - 3} h 10 M ${node.x - 5} ${node.y + 3} h 10`" class="map-node-icon" /><text :x="node.x" :y="node.y + 33" class="map-node-name" text-anchor="middle">{{ node.name.length > 22 ? node.name.slice(0, 20) + '…' : node.name }}</text><text :x="node.x" :y="node.y + 48" class="map-node-detail" text-anchor="middle">{{ node.ready ? 'Ready' : '未就绪' }} · {{ node.pods ?? '—' }} Pod</text><title>{{ node.name }} · {{ node.ready ? '就绪' : '未就绪' }} · {{ node.pods ?? '未知数量' }} 个样本 Pod</title></g>
                <circle cx="260" cy="145" r="42" class="map-core-ring" /><circle cx="260" cy="145" r="32" class="map-core" /><path d="M260 126 l15 9 v18 l-15 9 -15 -9 v-18 Z M245 135 l15 9 15 -9 M260 144 v18" class="map-core-icon" />
                <text v-if="!topologyNodes.length" x="260" y="224" text-anchor="middle" class="map-empty">{{ loading ? '正在读取集群节点' : nodes === null ? '节点数据暂不可用' : '当前集群暂无节点' }}</text>
              </svg>
              <footer class="map-footer"><span><i />就绪节点 <b>{{ readyNodes ?? '—' }}</b></span><span><i class="warn" />待关注 <b>{{ nodes === null ? '—' : nodes.length - (readyNodes ?? 0) }}</b></span><span>展示 {{ topologyNodes.length }} / {{ nodeCount ?? '—' }} 节点</span></footer><p class="map-note">连线表示集群归属 · Pod 数量来自已读取样本</p>
            </section>
            <section class="dash-panel diagnosis-panel" aria-labelledby="distribution-title">
              <header class="dash-panel-heading"><div><span class="dash-kicker">DIAGNOSIS INSIGHT</span><h2 id="distribution-title"><ScanLine :size="16" />诊断处理分布</h2></div><RouterLink :to="{ path: '/diagnoses', query: { cluster: selected } }" aria-label="查看诊断记录"><ArrowUpRight :size="18" /></RouterLink></header>
              <div class="diagnosis-summary"><strong>{{ diagnoses?.filter(item => item.status === 'open' || item.status === 'confirmed').length ?? '—' }}</strong><span>条待处理记录<small>最近 {{ diagnoses?.length ?? '—' }} 条诊断样本</small></span></div>
              <div class="status-bars"><div v-for="row in statusRows" :key="row.status"><div><span><i :style="{ background: row.color }" />{{ row.label }}</span><b>{{ diagnoses === null ? '—' : row.count }}</b></div><div class="dash-bar-track"><i :style="{ width: `${diagnoses?.length ? row.count / diagnoses.length * 100 : 0}%`, background: row.color }" /></div></div></div>
              <footer class="dash-footnote">按当前处理状态统计<span>不代表实时故障数量</span></footer>
            </section>
          </div>
          <div class="dash-secondary-grid">
            <DashboardTrend :token="auth.accessToken" :cluster-id="selected" :nodes="(nodes ?? []).map(node => node.metadata.name)" :refresh-key="refreshKey" />
            <section class="dash-panel rank-panel" aria-labelledby="rank-title"><header class="dash-panel-heading"><div><span class="dash-kicker">WORKLOAD RANKING</span><h2 id="rank-title"><Layers3 :size="16" />Pod 资源用量 TOP 5</h2></div><div class="dash-segments" role="group" aria-label="排行指标"><button :aria-pressed="rankMode === 'cpu'" @click="rankMode = 'cpu'">CPU</button><button :aria-pressed="rankMode === 'memory'" @click="rankMode = 'memory'">内存</button></div></header>
              <ol v-if="rank.length" class="rank-list"><li v-for="(item, index) in rank" :key="`${item.namespace}/${item.name}`"><span class="rank-index">{{ String(index + 1).padStart(2, '0') }}</span><div class="rank-resource"><div><span :title="`${item.namespace}/${item.name}`">{{ item.name }}</span><b>{{ rankMode === 'cpu' ? formatCPU(item.cpuMillicores) : formatMemory(item.memoryBytes) }}</b></div><div class="dash-bar-track"><i :style="{ width: `${rankWidth(item)}%` }" /></div><small>{{ item.namespace }} · {{ timestamp(item.timestamp) }}</small></div></li></ol>
              <div v-else class="dash-empty"><Database :size="28" /><strong>{{ podMetrics === null ? 'Pod 指标暂不可用' : '暂无 Pod 指标样本' }}</strong><span>指标服务就绪后显示资源用量排行</span></div><footer class="dash-footnote">{{ podMetrics?.length ?? '—' }} 个已读取指标样本<span>条形相对于榜首用量</span></footer>
            </section>
          </div>
          <div class="dash-bottom-grid">
            <section class="dash-panel recent-panel" aria-labelledby="recent-title"><header class="dash-panel-heading"><div><span class="dash-kicker">RECENT DIAGNOSES</span><h2 id="recent-title"><Activity :size="16" />最近诊断</h2></div><RouterLink :to="{ path: '/diagnoses', query: { cluster: selected } }">全部记录 <ArrowUpRight :size="13" /></RouterLink></header><div v-if="!diagnoses?.length" class="dash-empty compact"><span>{{ diagnoses === null ? '诊断记录暂不可用' : '暂无诊断记录，可从资源页面发起诊断。' }}</span></div><ul v-else class="dash-recent-list"><li v-for="record in diagnoses.slice(0, 4)" :key="record.id"><CircleDot :size="15" :class="`severity-${record.severity}`" /><div><RouterLink :to="{ path: '/diagnoses', query: { id: record.id, cluster: selected } }">{{ record.summary }}</RouterLink><small>{{ record.resource.kind }}/{{ record.resource.name }}</small></div><span class="diagnosis-status" :class="record.status">{{ statusLabels[record.status] }}</span><time :datetime="record.created_at">{{ timestamp(record.created_at) }}</time></li></ul></section>
            <section class="dash-panel event-panel" aria-labelledby="events-title"><header class="dash-panel-heading"><div><span class="dash-kicker">WARNING SIGNALS</span><h2 id="events-title"><TriangleAlert :size="16" />警告线索</h2></div><span class="dash-badge">当前样本</span></header><ul v-if="latestEvents.length" class="warning-list"><li v-for="(event, index) in latestEvents" :key="event.metadata.uid ?? index"><i /><div><strong>{{ event.reason }}</strong><p :title="event.message">{{ event.message }}</p><small>{{ event.involvedObject.kind }}/{{ event.involvedObject.name }}</small></div></li></ul><div v-else class="dash-empty compact"><ShieldCheck :size="26" /><span>{{ events === null ? '事件数据暂不可用' : '当前事件样本中没有警告' }}</span></div></section>
          </div>
          <footer class="dashboard-footer"><span><CircleDot :size="12" />{{ truncated.length ? `${truncated.join('、')}列表存在分页，分布与排行仅基于已读取样本。` : '资源分布基于当前快照；诊断统计基于最近记录。' }}</span><RouterLink :to="{ path: '/workloads', query: { cluster: selected } }">查看资源并诊断 <ArrowUpRight :size="13" /></RouterLink></footer>
        </template>
      </div>
    </Teleport>
  </ConsoleLayout>
</template>

<style>
/* Shared dashboard surfaces also style the resource trend child. */
.dashboard { --dash-cyan: var(--status-info); --dash-violet: var(--topo-ingress); --dash-amber: var(--status-warning); --dash-green: var(--status-success); --dash-red: var(--status-danger); --map-bg: #0b182b; --map-surface: #10243b; --map-border: #243f5c; --map-text: #eef6ff; --map-muted: #a8bfd8; --map-cyan: #59d8eb; --map-green: #64e6b0; --map-amber: #ffd080; color: var(--text-primary); }
.dashboard-presentation { position: fixed; inset: 0; z-index: 1000; overflow: auto; padding: 24px max(24px, calc((100vw - 1800px) / 2)); background: var(--bg-primary); outline: none; }
.dashboard:fullscreen { width: 100%; height: 100%; box-sizing: border-box; }
.dash-panel { min-width: 0; padding: 19px 20px 15px; border: 1px solid var(--border-subtle); border-radius: 14px; background: var(--bg-elevated); box-shadow: var(--shadow-xs); }
.dash-panel-heading { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.dash-kicker { display: block; margin-bottom: 8px; color: var(--text-secondary); font: 9px var(--font-mono); letter-spacing: 1.5px; }
.dash-panel h2 { display: flex; align-items: center; gap: 7px; margin: 0; font-size: 14px; font-weight: 650; }
.dash-panel h2 svg { color: var(--text-secondary); }
.dash-panel-heading > a { display: inline-flex; align-items: center; gap: 4px; color: var(--text-secondary); font-size: 11px; text-decoration: none; min-height: 28px; }
.dash-caption { color: var(--text-secondary); font-size: 10px; }
.dash-caption strong { font: 11px var(--font-mono); color: var(--text-primary); }
.dash-badge { font-size: 10px; padding: 3px 7px; border: 1px solid var(--border-subtle); border-radius: 5px; color: var(--text-secondary); white-space: nowrap; }
.dash-footnote { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 5px; margin-top: 14px; color: var(--text-secondary); font-size: 10px; line-height: 1.5; }
.dash-footnote span { color: var(--text-muted); }
.dashboard select { border: 1px solid var(--border-default); border-radius: 6px; padding: 6px 9px; font-size: 11px; color: var(--text-primary); background: var(--bg-secondary); min-height: 32px; }
.dash-segments { display: flex; flex-shrink: 0; gap: 2px; padding: 2px; background: var(--bg-secondary); border: 1px solid var(--border-subtle); border-radius: 6px; }
.dash-segments button { border: 0; border-radius: 4px; padding: 4px 8px; background: transparent; color: var(--text-secondary); font-size: 10px; min-height: 25px; cursor: pointer; }
.dash-segments button[aria-pressed="true"] { background: var(--accent-subtle); color: var(--accent-strong); box-shadow: var(--shadow-xs); }
.dash-bar-track { height: 5px; border-radius: 9px; background: var(--bg-tertiary); overflow: hidden; }
.dash-bar-track > i { display: block; height: 100%; border-radius: inherit; transition: width .6s ease; }
.dash-empty { display: flex; min-height: 220px; align-items: center; justify-content: center; flex-direction: column; gap: 12px; color: var(--text-secondary); text-align: center; font-size: 12px; }
.dash-empty svg { color: var(--dash-cyan); opacity: .65; }
.dash-empty strong { font-weight: 500; }
.dash-empty span { font-size: 11px; }
.dash-empty.compact { min-height: 160px; }
.dashboard button:focus-visible, .dashboard a:focus-visible, .dashboard select:focus-visible { outline: 2px solid var(--accent-strong); outline-offset: 3px; }
@media (min-width: 1200px) {
  .dashboard-presentation { padding: 18px 28px; }
  .dashboard.dashboard-presentation .dash-hero { padding: 15px 22px; }
  .dashboard.dashboard-presentation .hero-orbit { width: 66px; height: 66px; }
  .dashboard.dashboard-presentation .hero-orbit > svg { width: 44px; }
  .dashboard.dashboard-presentation .dash-stats { gap: 12px; margin-bottom: 12px; }
  .dashboard.dashboard-presentation .dash-stat { padding: 12px 18px; }
  .dashboard.dashboard-presentation .dash-stat > strong { font-size: 30px; margin: 7px 0; }
  .dashboard.dashboard-presentation .dash-main-grid { grid-template-columns: 1fr 1.5fr 1fr; gap: 12px; }
  .dashboard.dashboard-presentation .cluster-map { flex: none; height: 220px; min-height: 220px; margin: auto 0; }
  .dashboard.dashboard-presentation .gauge-pair { margin-top: 14px; }
  .dashboard.dashboard-presentation .resource-gauge > svg { max-width: 100px; }
  .dashboard.dashboard-presentation .gauge-label { height: 100px; }
  .dashboard.dashboard-presentation .status-bars { margin-bottom: 15px; }
  .dashboard.dashboard-presentation .dash-secondary-grid { margin-top: 12px; gap: 12px; }
  .dashboard.dashboard-presentation .rank-list li { padding: 5px 0; }
  .dashboard.dashboard-presentation .dash-bottom-grid { display: none; }
  .dashboard.dashboard-presentation .dashboard-footer { margin-top: 12px; }
}
@media (max-width: 720px) { .dashboard-presentation { padding: 14px; } .dashboard .dash-segments button { min-height: 36px; } }
@media (prefers-reduced-motion: reduce) { .dashboard *, .dashboard *::before, .dashboard *::after { animation: none !important; transition: none !important; } }
</style>

<style scoped>
.dash-hero { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 25px 26px; border: 1px solid var(--map-border); border-radius: 16px; color: var(--map-text); background: radial-gradient(ellipse at 10% 0%, var(--map-surface), transparent 60%), var(--map-bg); overflow: hidden; }
.dash-hero::after { content: ''; position: absolute; pointer-events: none; width: 320px; height: 320px; right: 24%; top: -120px; border: 1px solid var(--map-border); border-radius: 50%; box-shadow: 0 0 0 40px color-mix(in srgb, var(--map-border) 28%, transparent), 0 0 0 80px color-mix(in srgb, var(--map-border) 12%, transparent); }
.hero-copy { position: relative; z-index: 1; flex: 1; min-width: 0; }
.hero-eyebrow { display: flex; align-items: center; gap: 8px; color: var(--map-cyan); font: 10px var(--font-mono); letter-spacing: 2px; }
.hero-eyebrow i { width: 5px; height: 5px; background: var(--map-cyan); box-shadow: 0 0 10px var(--map-cyan); }
.hero-eyebrow > span { color: var(--map-muted); letter-spacing: 1px; }
.hero-copy h2 { margin: 11px 0 9px; font-size: 23px; letter-spacing: 1px; font-weight: 650; color: var(--map-text); }
.hero-copy h2 small { font-size: 12px; font-weight: 400; letter-spacing: 0; color: var(--map-muted); }
.hero-slash { margin: 0 6px; color: var(--map-border); font-weight: 300; }
.hero-copy p { display: flex; flex-wrap: wrap; align-items: center; gap: 9px; margin: 0; font-size: 10px; color: var(--map-muted); }
.hero-copy p > span { color: var(--map-green); }
.hero-copy p > .service-unready { color: var(--map-amber); }
.hero-copy p b { color: var(--map-muted); font-weight: 400; }
.hero-orbit { position: relative; flex-shrink: 0; display: grid; place-items: center; width: 88px; height: 88px; color: var(--map-cyan); border: 1px solid var(--map-border); border-radius: 50%; background: color-mix(in srgb, var(--map-cyan) 6%, transparent); }
.hero-orbit i { position: absolute; inset: 6px; border: 1px dashed var(--map-border); border-radius: 50%; animation: orbit-spin 40s linear infinite; }
.hero-orbit i:last-child { inset: -8px; border-style: solid; border-color: transparent var(--map-cyan) transparent transparent; animation-duration: 15s; }
.hero-actions { z-index: 1; min-width: 185px; display: flex; flex-direction: column; gap: 9px; }
.hero-actions select { width: 100%; color: var(--map-text); background: var(--map-surface); border-color: var(--map-border); }
.hero-buttons { display: flex; gap: 8px; justify-content: flex-end; }
.dash-action { display: flex; justify-content: center; align-items: center; gap: 6px; min-height: 32px; padding: 6px 10px; border: 1px solid var(--map-border); border-radius: 6px; font-size: 11px; color: var(--map-text); background: var(--map-surface); cursor: pointer; }
.dash-action:disabled { opacity: .55; cursor: wait; }
.dash-action-primary { color: var(--map-cyan); border-color: color-mix(in srgb, var(--map-cyan) 45%, var(--map-border)); }
.dash-status-strip { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: 12px 2px; font-size: 10px; color: var(--text-secondary); }
.dash-status-strip > span { display: flex; align-items: center; gap: 6px; }
.dash-status-strip i { width: 5px; height: 5px; border-radius: 50%; background: var(--dash-green); box-shadow: 0 0 0 3px color-mix(in srgb, var(--dash-green) 12%, transparent); }
.dash-status-strip i.paused { background: var(--text-secondary); box-shadow: none; }
.dash-status-strip button { display: inline-flex; gap: 4px; align-items: center; border: 0; padding: 3px; color: var(--text-secondary); background: transparent; font-size: 10px; cursor: pointer; }
.dash-retry { margin-left: 12px; color: inherit; background: transparent; border: 0; text-decoration: underline; cursor: pointer; }
.dash-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; margin-bottom: 16px; }
.dash-stat { position: relative; padding: 16px 19px 13px; border: 1px solid var(--border-subtle); border-radius: 12px; background: var(--bg-elevated); overflow: hidden; }
.dash-stat::after { content: ''; position: absolute; right: -12px; bottom: -35px; width: 105px; height: 105px; border: 15px solid color-mix(in srgb, var(--stat-color) 6%, transparent); border-radius: 50%; pointer-events: none; }
.dash-stat > div { display: flex; justify-content: space-between; align-items: center; color: var(--text-secondary); font-size: 12px; }
.dash-stat > div svg { color: var(--stat-color); }
.dash-stat > strong { display: flex; align-items: baseline; gap: 9px; margin: 9px 0 7px; font: 600 34px/1.15 var(--font-mono); letter-spacing: -1.5px; }
.dash-stat strong small { color: var(--text-secondary); font-size: 9px; font-weight: 400; letter-spacing: 1px; }
.dash-stat footer { display: flex; align-items: center; gap: 5px; font-size: 10px; color: var(--text-secondary); }
.dash-stat footer i { width: 4px; height: 4px; border-radius: 50%; background: var(--stat-color); }
.dash-main-grid { display: grid; grid-template-columns: 1fr 1.5fr 1fr; gap: 16px; align-items: stretch; }
.gauge-pair { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin: 22px 0 0; }
.resource-gauge { position: relative; min-width: 0; text-align: center; }
.resource-gauge > svg { display: block; width: 100%; max-width: 120px; margin: 0 auto 8px; }
.gauge-track { stroke: var(--border-subtle); stroke-width: 7; fill: none; }
.gauge-value { stroke: var(--gauge-color); stroke-width: 7; stroke-linecap: round; fill: none; transition: stroke-dasharray .7s ease; }
.gauge-label { position: absolute; top: 0; width: 100%; height: min(100%, 112px); aspect-ratio: 1; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 5px; }
.gauge-label svg { color: var(--gauge-color); }
.gauge-label strong { font: 600 20px var(--font-mono); }
.gauge-label small { font-size: 10px; margin-left: 1px; }
.resource-gauge > b { display: block; font-size: 11px; font-weight: 500; }
.resource-gauge > span { display: block; font: 10px var(--font-mono); color: var(--text-secondary); margin-top: 5px; }
.pressure-rule { display: flex; align-items: center; justify-content: space-between; border-top: 1px solid var(--border-subtle); padding-top: 15px; margin-top: 14px; font-size: 11px; }
.pressure-rule strong { color: var(--text-secondary); font: 11px var(--font-mono); }
.pod-health { display: flex; align-items: center; gap: 12px; margin-top: 6px; }
.pod-health svg { width: 72px; height: 72px; flex-shrink: 0; }
.pod-health text { font: 600 15px var(--font-mono); fill: var(--text-primary); }
.pod-health > div { flex: 1; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.pod-health span { display: flex; align-items: center; gap: 4px; font-size: 10px; color: var(--text-secondary); }
.pod-health i, .status-bars span i { width: 5px; height: 5px; border-radius: 50%; display: inline-block; }
.pod-health b { font: 11px var(--font-mono); color: var(--text-primary); margin-left: auto; }
.topology-panel { background: radial-gradient(ellipse at 50% 50%, var(--map-surface), var(--map-bg)); color: var(--map-text); border-color: var(--map-border); position: relative; overflow: hidden; display: flex; flex-direction: column; }
.topology-panel::before { content: ''; pointer-events: none; position: absolute; inset: 0; opacity: .17; background-image: linear-gradient(var(--map-border) 1px, transparent 1px), linear-gradient(90deg, var(--map-border) 1px, transparent 1px); background-size: 24px 24px; mask-image: radial-gradient(ellipse, black, transparent 75%); }
.topology-panel .dash-panel-heading { z-index: 1; }
.topology-panel h2 { color: var(--map-text); }
.topology-panel .dash-kicker, .topology-panel h2 svg { color: var(--map-muted); }
.topology-panel .dash-panel-heading > a { color: var(--map-cyan); }
.cluster-map { width: 100%; min-height: 238px; flex: 1; overflow: visible; z-index: 1; }
.map-orbit { stroke: var(--map-border); stroke-width: 1; fill: none; stroke-dasharray: 3 6; }
.map-orbit.inner { stroke-dasharray: none; opacity: .6; }
.map-connection { stroke: var(--map-cyan); stroke-width: 1; fill: none; stroke-dasharray: 4 7; opacity: .6; animation: map-flow 16s linear infinite; }
.map-node-halo { fill: var(--map-surface); stroke: var(--map-border); }
.map-node-body { fill: var(--map-surface); stroke: var(--map-green); stroke-width: 1.5; }
.map-node-icon { stroke: var(--map-green); fill: none; stroke-width: 1.5; }
.map-node-warning .map-node-body, .map-node-warning .map-node-icon { stroke: var(--map-amber); }
.map-node-name { fill: var(--map-text); font: 10px var(--font-mono); }
.map-node-detail { fill: var(--map-muted); font: 9px var(--font-mono); }
.map-core-ring { fill: color-mix(in srgb, var(--map-cyan) 5%, transparent); stroke: var(--map-border); stroke-width: 1; }
.map-core { fill: var(--map-surface); stroke: var(--map-cyan); }
.map-core-icon { stroke: var(--map-cyan); fill: none; stroke-width: 1.5; }
.map-empty { fill: var(--map-muted); font-size: 12px; }
.map-footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 14px; margin-top: 5px; font-size: 10px; color: var(--map-muted); }
.map-footer span { display: flex; align-items: center; gap: 5px; }
.map-footer i { width: 5px; height: 5px; border-radius: 50%; background: var(--map-green); }
.map-footer i.warn { background: var(--map-amber); }
.map-footer b { color: var(--map-text); font: 12px var(--font-mono); }
.map-note { margin: 12px 0 0; text-align: center; font-size: 9px; color: var(--map-muted); }
.diagnosis-summary { display: flex; align-items: center; gap: 12px; margin: 24px 0; }
.diagnosis-summary > strong { font: 500 43px/1 var(--font-mono); letter-spacing: -2px; }
.diagnosis-summary > span { font-size: 12px; color: var(--text-secondary); }
.diagnosis-summary small { display: block; font-size: 10px; margin-top: 6px; }
.status-bars { display: grid; gap: 18px; margin-bottom: 25px; }
.status-bars > div > div:first-child { display: flex; align-items: center; justify-content: space-between; margin-bottom: 7px; font-size: 11px; }
.status-bars span { display: flex; align-items: center; gap: 6px; color: var(--text-secondary); }
.status-bars b { font: 12px var(--font-mono); }
.dash-secondary-grid { display: grid; grid-template-columns: 1.5fr 1fr; gap: 16px; margin-top: 16px; }
.rank-list { list-style: none; margin: 15px 0 0; padding: 0; }
.rank-list li { display: flex; gap: 10px; align-items: flex-start; padding: 7px 0; }
.rank-index { font: 11px var(--font-mono); color: var(--text-secondary); padding-top: 2px; }
.rank-list li:first-child .rank-index { color: var(--dash-cyan); }
.rank-resource { flex: 1; min-width: 0; }
.rank-resource > div:first-child { display: flex; justify-content: space-between; gap: 8px; margin-bottom: 6px; font-size: 11px; }
.rank-resource > div:first-child > span { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rank-resource b { flex-shrink: 0; font: 11px var(--font-mono); }
.rank-resource .dash-bar-track { height: 3px; }
.rank-resource .dash-bar-track i { background: linear-gradient(90deg, var(--dash-cyan), color-mix(in srgb, var(--dash-cyan) 45%, var(--bg-elevated))); }
.rank-resource small { display: block; margin-top: 5px; font-size: 9px; color: var(--text-secondary); }
.dash-bottom-grid { display: grid; grid-template-columns: 1.5fr 1fr; gap: 16px; margin-top: 16px; }
.dash-recent-list { list-style: none; margin: 14px 0 0; padding: 0; }
.dash-recent-list li { display: flex; align-items: center; gap: 10px; padding: 11px 0; border-bottom: 1px solid var(--border-subtle); }
.dash-recent-list li:last-child { border-bottom: 0; padding-bottom: 0; }
.dash-recent-list li > svg { color: var(--dash-cyan); flex-shrink: 0; }
.dash-recent-list li > .severity-warning { color: var(--dash-amber); }
.dash-recent-list li > .severity-high, .dash-recent-list li > .severity-critical { color: var(--dash-red); }
.dash-recent-list li > div { flex: 1; min-width: 0; }
.dash-recent-list a { display: block; color: var(--text-primary); font-size: 11px; white-space: nowrap; text-overflow: ellipsis; overflow: hidden; text-decoration: none; }
.dash-recent-list a:hover { color: var(--accent-strong); }
.dash-recent-list small { display: block; margin-top: 5px; font-size: 9px; color: var(--text-secondary); overflow-wrap: anywhere; }
.dash-recent-list time { font: 9px var(--font-mono); color: var(--text-secondary); white-space: nowrap; }
.diagnosis-status { padding: 4px 6px; font-size: 9px; border-radius: 4px; color: var(--text-secondary); background: var(--bg-tertiary); white-space: nowrap; }
.diagnosis-status.open { color: var(--dash-amber); background: var(--warning-bg); }
.diagnosis-status.confirmed { color: var(--dash-red); background: var(--danger-bg); }
.diagnosis-status.resolved { color: var(--dash-green); background: var(--success-bg); }
.warning-list { list-style: none; margin: 18px 0 0; padding: 0; }
.warning-list li { display: flex; gap: 10px; margin-top: 14px; }
.warning-list li > i { width: 5px; height: 5px; border-radius: 50%; flex-shrink: 0; margin-top: 5px; background: var(--dash-amber); box-shadow: 0 0 0 3px color-mix(in srgb, var(--dash-amber) 10%, transparent); }
.warning-list li > div { min-width: 0; }
.warning-list strong { font: 11px var(--font-mono); }
.warning-list p { margin: 5px 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; color: var(--text-secondary); }
.warning-list small { font-size: 9px; color: var(--text-secondary); overflow-wrap: anywhere; }
.dashboard-footer { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; margin: 17px 2px 5px; font-size: 10px; color: var(--text-secondary); }
.dashboard-footer > span, .dashboard-footer > a { display: flex; align-items: center; gap: 6px; }
.dashboard-footer > a { color: var(--accent-strong); text-decoration: none; }
.overview-panel { padding: 28px; }
.overview-panel h2 { font-size: 18px; }
.overview-panel p { color: var(--text-secondary); font-size: 13px; line-height: 1.8; }
.dash-spinning { animation: orbit-spin 1s linear infinite; }
@keyframes orbit-spin { to { transform: rotate(360deg); } }
@keyframes map-flow { to { stroke-dashoffset: -110; } }
@media (max-width: 1180px) { .hero-orbit { display: none; } .hero-copy h2 small, .hero-slash { display: none; } .dash-main-grid { grid-template-columns: 1fr 1.3fr; } .diagnosis-panel { grid-column: 1 / -1; } .diagnosis-summary { margin: 14px 0; } .diagnosis-panel .status-bars { grid-template-columns: repeat(4, 1fr); gap: 20px; margin-bottom: 10px; } .diagnosis-panel .dash-footnote { margin-top: 10px; } }
@media (max-width: 860px) { .dash-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } .dash-secondary-grid, .dash-bottom-grid { grid-template-columns: 1fr; } }
@media (max-width: 600px) { .dash-hero { flex-direction: column; align-items: stretch; padding: 20px; gap: 18px; } .hero-copy h2 { font-size: 22px; } .hero-eyebrow { font-size: 9px; letter-spacing: 1px; } .hero-actions { min-width: 0; flex-direction: row; flex-wrap: wrap; } .hero-actions select { flex: 1; min-width: 145px; } .hero-buttons { flex: 1; } .dash-action { flex: 1; min-height: 40px; } .dash-main-grid { grid-template-columns: 1fr; } .diagnosis-panel { grid-column: auto; } .dash-panel { padding: 18px; } .dash-stat { padding: 14px; } .dash-stat strong small { font-size: 8px; letter-spacing: 0; } .dash-stat > strong { font-size: 29px; gap: 5px; } .dash-stat footer { font-size: 9px; line-height: 1.6; } .dash-stats { gap: 10px; } .gauge-pair { margin-top: 18px; } .gauge-label { height: 120px; } .pod-health { max-width: 290px; margin: 7px auto 0; } .diagnosis-panel .status-bars { grid-template-columns: repeat(2, 1fr); gap: 16px; margin-top: 20px; } .cluster-map { min-height: 250px; } .dash-recent-list time { display: none; } .dash-status-strip button { min-height: 32px; font-size: 9px; } .dash-status-strip { font-size: 9px; } .dashboard-footer { line-height: 1.6; } }
</style>
