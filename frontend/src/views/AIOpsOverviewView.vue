<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Activity,
  Boxes,
  CheckCircle2,
  Clock,
  GitBranch,
  Layers,
  Network,
  RefreshCw,
  Search,
  Signal,
  TriangleAlert,
  XCircle,
} from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import {
  getAIOpsOverview,
  getTopologyGraph,
  listSignalCatalog,
  listSignals,
  listTopologyChanges,
} from '../api/aiops'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type {
  ChangeEvent,
  ChangeTimelineResponse,
  SignalDescriptor,
  SignalOccurrence,
  SignalOverview,
  SignalProducer,
  TopologyGraph,
} from '../types/aiops'

type LoadStatus = 'loading' | 'ready' | 'error'
type TopologyStatus = 'idle' | 'loading' | 'ready' | 'error'
type Tab = 'overview' | 'signals' | 'topology'

const auth = useAuthStore()

const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const namespace = ref('')

const overview = ref<SignalOverview | null>(null)
const status = ref<LoadStatus>('loading')
const errorMessage = ref('')

const signals = ref<SignalOccurrence[]>([])
const signalsTotal = ref(0)
const signalsTruncated = ref(false)
const signalsCursor = ref<string | undefined>(undefined)
const signalsLoading = ref(false)
const severityFilter = ref('')
const stateFilter = ref('')
const producerFilter = ref('')

const catalog = ref<SignalDescriptor[]>([])

const topology = ref<TopologyGraph | null>(null)
const changes = ref<ChangeEvent[]>([])
const topologyStatus = ref<TopologyStatus>('idle')
const topologyError = ref('')

const tab = ref<Tab>('overview')

const PRODUCER_LABELS: Record<SignalProducer, string> = {
  diagnosis_engine: '诊断引擎',
  alert_router: '告警路由',
  inspection: '巡检',
  slo_evaluator: 'SLO 评估',
  topology_change: '拓扑变更',
  service_mesh: '服务网格',
  external: '外部',
}

const NODE_COLORS: Record<string, string> = {
  Deployment: '#326ce5',
  Service: '#16a34a',
  Pod: '#6b7280',
  Ingress: '#7d5e8f',
  EndpointSlice: '#0ea5e9',
  StatefulSet: '#d97706',
  DaemonSet: '#dc2626',
  ReplicaSet: '#0d9488',
  Node: '#374151',
  Namespace: '#475569',
}

const generatedLabel = computed(() =>
  overview.value?.generated_at ? formatTime(overview.value.generated_at) : '--',
)

const catalogMap = computed<Record<string, SignalDescriptor>>(() => {
  const map: Record<string, SignalDescriptor> = {}
  for (const item of catalog.value) map[item.code] = item
  return map
})

const sourceCompleteness = computed(() => {
  if (!overview.value) return []
  return Object.entries(overview.value.source_completeness).map(([key, entry]) => ({
    key,
    entry,
    label: PRODUCER_LABELS[entry.producer as SignalProducer] ?? entry.producer,
  }))
})

const filteredSignals = computed(() =>
  signals.value.filter(
    (s) =>
      (!severityFilter.value || s.severity === severityFilter.value) &&
      (!stateFilter.value || s.state === stateFilter.value) &&
      (!producerFilter.value || s.producer === producerFilter.value),
  ),
)

const producerOptions = computed<SignalProducer[]>(() => {
  const set = new Set<SignalProducer>()
  signals.value.forEach((s) => set.add(s.producer))
  return [...set]
})

interface LayoutNode {
  key: string
  kind: string
  name: string
  ns: string
  x: number
  y: number
  w: number
  h: number
  color: string
  edgeCount: number
}
interface LayoutEdge {
  key: string
  kind: string
  x1: number
  y1: number
  x2: number
  y2: number
  mx: number
  my: number
  labelW: number
}
interface TopologyLayout {
  width: number
  height: number
  nodes: LayoutNode[]
  edges: LayoutEdge[]
}

const topologyLayout = computed<TopologyLayout | null>(() => {
  const g = topology.value
  if (!g || g.nodes.length === 0) return null
  const nodeW = 184
  const nodeH = 58
  const colGap = 76
  const rowGap = 30
  const padX = 24
  const padY = 24

  const kindOrder: string[] = []
  const byKind = new Map<string, typeof g.nodes>()
  for (const n of g.nodes) {
    if (!byKind.has(n.resource.kind)) {
      byKind.set(n.resource.kind, [])
      kindOrder.push(n.resource.kind)
    }
    byKind.get(n.resource.kind)!.push(n)
  }

  const pos = new Map<string, { x: number; y: number; w: number; h: number }>()
  const layoutNodes: LayoutNode[] = []
  let maxRows = 0
  kindOrder.forEach((kind, col) => {
    const list = byKind.get(kind)!
    if (list.length > maxRows) maxRows = list.length
    list.forEach((n, row) => {
      const x = padX + col * (nodeW + colGap)
      const y = padY + row * (nodeH + rowGap)
      const key = nodeKey(n.resource)
      pos.set(key, { x, y, w: nodeW, h: nodeH })
      layoutNodes.push({
        key,
        kind: n.resource.kind,
        name: n.resource.name,
        ns: n.resource.namespace,
        x,
        y,
        w: nodeW,
        h: nodeH,
        color: nodeColor(n.resource.kind),
        edgeCount: n.edge_count,
      })
    })
  })

  const width = padX * 2 + kindOrder.length * nodeW + Math.max(0, kindOrder.length - 1) * colGap
  const height = padY * 2 + maxRows * nodeH + Math.max(0, maxRows - 1) * rowGap

  const layoutEdges: LayoutEdge[] = []
  for (const e of g.edges) {
    const s = pos.get(nodeKey(e.source))
    const t = pos.get(nodeKey(e.target))
    if (!s || !t) continue
    const x1 = s.x + s.w / 2
    const y1 = s.y + s.h / 2
    const x2 = t.x + t.w / 2
    const y2 = t.y + t.h / 2
    layoutEdges.push({
      key: String(e.id),
      kind: e.kind,
      x1,
      y1,
      x2,
      y2,
      mx: (x1 + x2) / 2,
      my: (y1 + y2) / 2,
      labelW: e.kind.length * 5.6 + 8,
    })
  }

  return { width, height, nodes: layoutNodes, edges: layoutEdges }
})

const missingProducers = computed(() => topology.value?.completeness?.missing_producers ?? [])

function nodeKey(r: { kind: string; namespace: string; name: string }): string {
  return `${r.kind}|${r.namespace}|${r.name}`
}

function nodeColor(kind: string): string {
  return NODE_COLORS[kind] ?? '#4b5563'
}

function formatTime(value?: string): string {
  if (!value) return '--'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(d)
}

function clusterName(id: number): string {
  return clusters.value.find((c) => c.id === id)?.name ?? `集群 #${id}`
}

function coverageLabel(c: string): string {
  return (
    { complete: '完整', partial: '部分', missing: '缺失', unavailable: '无数据', truncated: '截断' } as Record<string, string>
  )[c] ?? c
}

function coverageTitle(c: string): string {
  return ({
    complete: '数据完整',
    partial: '仅部分采样，结果可能低估真实状态',
    missing: '缺少样本',
    unavailable: '无样本窗口（fail-closed，不视为健康）',
    truncated: '采样达到预算上限，窗口被截断',
  } as Record<string, string>)[c] ?? c
}

function signalWindowLabel(s: { window_start?: string; window_end?: string }): string {
  if (!s.window_start || !s.window_end) return '--'
  const start = new Date(s.window_start).getTime()
  const end = new Date(s.window_end).getTime()
  if (Number.isNaN(start) || Number.isNaN(end)) return '--'
  const seconds = Math.round((end - start) / 1000)
  if (seconds <= 0) return '--'
  if (seconds % 86400 === 0) return `${seconds / 86400}d`
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

function signalLatencyLabel(s: { observed_at: string; ingested_at?: string }): string {
  if (!s.ingested_at) return '--'
  const observed = new Date(s.observed_at).getTime()
  const ingested = new Date(s.ingested_at).getTime()
  if (Number.isNaN(observed) || Number.isNaN(ingested)) return '--'
  const ms = Math.max(0, ingested - observed)
  if (ms < 1000) return '<1s'
  if (ms < 60000) return `${Math.round(ms / 1000)}s`
  if (ms < 3600000) return `${Math.round(ms / 60000)}m`
  return `${(ms / 3600000).toFixed(1)}h`
}

function severityLabel(s: string): string {
  return ({ critical: '严重', warning: '警告', info: '提示' } as Record<string, string>)[s] ?? s
}

function stateLabel(s: string): string {
  return (
    { active: '活跃', resolved: '已恢复', stale: '已过期', suppressed: '已抑制' } as Record<string, string>
  )[s] ?? s
}

function producerLabel(p: SignalProducer): string {
  return PRODUCER_LABELS[p] ?? p
}

function resourceRef(r: { kind: string; namespace: string; name: string }): string {
  return `${r.kind}/${r.namespace || 'cluster'}/${r.name}`
}

async function loadOverview() {
  status.value = 'loading'
  errorMessage.value = ''
  try {
    overview.value = await getAIOpsOverview(auth.accessToken)
    status.value = 'ready'
  } catch (err) {
    status.value = 'error'
    errorMessage.value = err instanceof APIError ? err.message : '概览数据加载失败'
  }
}

async function loadSignals(append = false) {
  if (!append) signalsCursor.value = undefined
  const cursor = append ? signalsCursor.value : undefined
  signalsLoading.value = true
  try {
    const res = await listSignals(auth.accessToken, { limit: 50, cursor })
    signals.value = append ? [...signals.value, ...res.items] : res.items
    signalsTotal.value = res.total
    signalsTruncated.value = res.truncated
    signalsCursor.value = res.next_cursor
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '信号列表加载失败'
  } finally {
    signalsLoading.value = false
  }
}

async function loadCatalog() {
  try {
    const res = await listSignalCatalog(auth.accessToken)
    catalog.value = res.items
  } catch {
    catalog.value = []
  }
}

async function loadTopology() {
  const cid = selectedClusterID.value
  if (!cid) {
    topology.value = null
    changes.value = []
    topologyStatus.value = 'idle'
    return
  }
  topologyStatus.value = 'loading'
  topologyError.value = ''
  try {
    const ns = namespace.value.trim() || undefined
    const [g, t] = await Promise.all([
      getTopologyGraph(auth.accessToken, { cluster_id: cid, namespace: ns }),
      listTopologyChanges(auth.accessToken, { cluster_id: cid, namespace: ns, limit: 50 }),
    ])
    topology.value = g
    changes.value = (t as ChangeTimelineResponse).items
    topologyStatus.value = 'ready'
  } catch (err) {
    topologyStatus.value = 'error'
    topologyError.value = err instanceof APIError ? err.message : '拓扑数据加载失败'
  }
}

async function changeCluster() {
  topology.value = null
  changes.value = []
  topologyStatus.value = selectedClusterID.value ? 'idle' : 'idle'
  await loadTopology()
}

function switchToTopology() {
  tab.value = 'topology'
  if (topologyStatus.value === 'idle' && selectedClusterID.value) {
    void loadTopology()
  }
}

async function refreshAll() {
  await Promise.all([loadOverview(), loadSignals(false), loadCatalog()])
}

async function initialize() {
  try {
    const res = await listClusters(auth.accessToken)
    clusters.value = res.items.filter((c) => c.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch (err) {
    status.value = 'error'
    errorMessage.value = err instanceof APIError ? err.message : '无法加载集群列表'
    return
  }
  await Promise.all([loadOverview(), loadSignals(false), loadCatalog(), loadTopology()])
}

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="智能运维概览">
    <template #actions>
      <span class="sync-time">生成于 {{ generatedLabel }}</span>
      <button
        class="icon-button"
        type="button"
        title="刷新概览"
        aria-label="刷新概览"
        :disabled="status === 'loading'"
        @click="refreshAll"
      >
        <RefreshCw :size="18" :class="{ spinning: status === 'loading' }" />
      </button>
    </template>

    <nav class="aiops-tabs" aria-label="AIOps 视图切换">
      <button type="button" :class="{ active: tab === 'overview' }" @click="tab = 'overview'">
        <Activity :size="16" /> 概览
      </button>
      <button type="button" :class="{ active: tab === 'signals' }" @click="tab = 'signals'">
        <Signal :size="16" /> 信号列表
      </button>
      <button type="button" :class="{ active: tab === 'topology' }" @click="switchToTopology">
        <Network :size="16" /> 拓扑图
      </button>
    </nav>

    <p v-if="errorMessage && tab !== 'topology'" class="error-message">{{ errorMessage }}</p>

    <!-- ── 概览 ─────────────────────────────────────────────── -->
    <section v-if="tab === 'overview'" class="aiops-section">
      <div v-if="status === 'loading' && !overview" class="aiops-empty">
        <span class="loading-indicator" /> 正在加载概览…
      </div>
      <div v-else-if="!overview" class="aiops-empty">
        <Activity :size="28" />
        <strong>暂无概览数据</strong>
        <span>稍后刷新以获取最新信号概览。</span>
      </div>
      <template v-else>
        <div v-if="overview.partial" class="partial-banner">
          <TriangleAlert :size="18" />
          <div>
            <strong>数据可能不完整</strong>
            <span>部分信号生产者缺失或部分覆盖，概览仅反映当前可用证据。</span>
          </div>
        </div>

        <div class="aiops-stat-grid">
          <article class="aiops-card aiops-stat tone-blue">
            <span><Activity :size="15" /> 活跃诊断</span>
            <strong>{{ overview.active_diagnoses }}</strong>
            <small>进行中的智能诊断</small>
          </article>
          <article class="aiops-card aiops-stat tone-green">
            <span><CheckCircle2 :size="15" /> 执行成功</span>
            <strong>{{ overview.action_outcomes.succeeded }}</strong>
            <small>已生效的处置动作</small>
          </article>
          <article class="aiops-card aiops-stat tone-red">
            <span><XCircle :size="15" /> 执行失败</span>
            <strong>{{ overview.action_outcomes.failed }}</strong>
            <small>失败或回滚的处置</small>
          </article>
          <article class="aiops-card aiops-stat tone-amber">
            <span><Clock :size="15" /> 待处理</span>
            <strong>{{ overview.action_outcomes.pending }}</strong>
            <small>等待审批或执行</small>
          </article>
        </div>

        <div class="aiops-grid-2">
          <article class="aiops-card">
            <header class="aiops-card-heading">
              <h3><Layers :size="16" /> 信号源完整性</h3>
              <span>{{ sourceCompleteness.length }} 个生产者</span>
            </header>
            <div v-if="sourceCompleteness.length === 0" class="aiops-inline-empty">没有生产者数据</div>
            <ul v-else class="producer-list">
              <li v-for="item in sourceCompleteness" :key="item.key">
                <div class="producer-main">
                  <strong>{{ item.label }}</strong>
                  <small>{{ item.entry.producer }}</small>
                </div>
                <span class="badge" :class="`badge-${item.entry.coverage}`">
                  {{ coverageLabel(item.entry.coverage) }}
                </span>
                <small class="producer-meta">
                  {{ item.entry.last_seen ? `最近 ${formatTime(item.entry.last_seen)}` : '尚未观测' }}
                  <template v-if="item.entry.gap_reason"> · {{ item.entry.gap_reason }}</template>
                </small>
              </li>
            </ul>
          </article>

          <article class="aiops-card">
            <header class="aiops-card-heading">
              <h3><Signal :size="16" /> Top 信号</h3>
              <span>{{ overview.top_signals.length }} 条</span>
            </header>
            <div v-if="overview.top_signals.length === 0" class="aiops-inline-empty">没有活跃信号</div>
            <table v-else class="aiops-table">
              <thead>
                <tr>
                  <th>信号代码</th>
                  <th>级别</th>
                  <th>数量</th>
                  <th>集群</th>
                  <th>命名空间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(s, idx) in overview.top_signals" :key="`${s.signal_code}-${idx}`">
                  <td>
                    <strong>{{ s.signal_code }}</strong>
                    <small v-if="catalogMap[s.signal_code]?.domain">{{ catalogMap[s.signal_code].domain }}</small>
                  </td>
                  <td><span class="badge" :class="`badge-${s.severity}`">{{ severityLabel(s.severity) }}</span></td>
                  <td>{{ s.count }}</td>
                  <td>{{ clusterName(s.cluster_id) }}</td>
                  <td>{{ s.namespace || 'cluster-scoped' }}</td>
                </tr>
              </tbody>
            </table>
          </article>
        </div>

        <article class="aiops-card">
          <header class="aiops-card-heading">
            <h3><GitBranch :size="16" /> 最近变更时间线</h3>
            <span>{{ overview.recent_changes.length }} 条</span>
          </header>
          <div v-if="overview.recent_changes.length === 0" class="aiops-inline-empty">没有最近的变更事件</div>
          <ol v-else class="timeline">
            <li v-for="c in overview.recent_changes" :key="c.change_event_id" class="timeline-item">
              <span class="timeline-dot" />
              <div class="timeline-body">
                <div class="timeline-head">
                  <span class="badge badge-kind">{{ c.kind }}</span>
                  <strong>{{ resourceRef(c.target) }}</strong>
                  <span class="badge" :class="`badge-result-${c.result}`">{{ c.result }}</span>
                </div>
                <small class="timeline-time">{{ formatTime(c.started_at) }}</small>
              </div>
            </li>
          </ol>
        </article>
      </template>
    </section>

    <!-- ── 信号列表 ─────────────────────────────────────────── -->
    <section v-else-if="tab === 'signals'" class="aiops-section">
      <div class="aiops-toolbar">
        <label class="aiops-filter">
          <span>级别</span>
          <select v-model="severityFilter">
            <option value="">全部</option>
            <option value="critical">严重</option>
            <option value="warning">警告</option>
            <option value="info">提示</option>
          </select>
        </label>
        <label class="aiops-filter">
          <span>状态</span>
          <select v-model="stateFilter">
            <option value="">全部</option>
            <option value="active">活跃</option>
            <option value="resolved">已恢复</option>
            <option value="stale">已过期</option>
            <option value="suppressed">已抑制</option>
          </select>
        </label>
        <label class="aiops-filter">
          <span>生产者</span>
          <select v-model="producerFilter">
            <option value="">全部</option>
            <option v-for="p in producerOptions" :key="p" :value="p">{{ producerLabel(p) }}</option>
          </select>
        </label>
        <div class="aiops-toolbar-meta">
          共 {{ signalsTotal }} 条 · 当前 {{ filteredSignals.length }} 条
        </div>
      </div>

      <div v-if="signalsLoading && signals.length === 0" class="aiops-empty">
        <span class="loading-indicator" /> 正在加载信号…
      </div>
      <div v-else-if="filteredSignals.length === 0" class="aiops-empty">
        <Signal :size="28" />
        <strong>没有匹配的信号</strong>
        <span>调整筛选条件或稍后刷新。</span>
      </div>
      <div v-else class="aiops-table-wrap">
        <table class="aiops-table">
          <thead>
            <tr>
              <th>信号代码</th>
              <th>级别</th>
              <th>状态</th>
              <th>覆盖度</th>
              <th>资源</th>
              <th>集群</th>
              <th>命名空间</th>
              <th>观测时间</th>
              <th>时间窗口</th>
              <th>数据延迟</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in filteredSignals" :key="s.id">
              <td>
                <strong>{{ s.signal_code }}</strong>
                <small v-if="catalogMap[s.signal_code]?.domain">{{ catalogMap[s.signal_code].domain }}</small>
              </td>
              <td><span class="badge" :class="`badge-${s.severity}`">{{ severityLabel(s.severity) }}</span></td>
              <td><span class="badge" :class="`badge-${s.state}`">{{ stateLabel(s.state) }}</span></td>
              <td>
                <span class="badge" :class="`badge-${s.coverage}`" :title="coverageTitle(s.coverage)">{{ coverageLabel(s.coverage) }}</span>
              </td>
              <td>{{ s.resource.kind }} / {{ s.resource.name }}</td>
              <td>{{ clusterName(s.cluster_id) }}</td>
              <td>{{ s.namespace || 'cluster-scoped' }}</td>
              <td>{{ formatTime(s.observed_at) }}</td>
              <td>{{ signalWindowLabel(s) }}</td>
              <td>{{ signalLatencyLabel(s) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="signalsCursor" class="aiops-load-more">
        <button type="button" class="secondary-button" :disabled="signalsLoading" @click="loadSignals(true)">
          <RefreshCw :size="14" :class="{ spinning: signalsLoading }" /> 加载更多
        </button>
        <span v-if="signalsTruncated">列表已截断</span>
      </div>
    </section>

    <!-- ── 拓扑图 ───────────────────────────────────────────── -->
    <section v-else class="aiops-section">
      <div class="aiops-toolbar">
        <label class="aiops-filter">
          <span>集群</span>
          <select v-model="selectedClusterID" @change="changeCluster">
            <option :value="null" disabled>选择已启用集群</option>
            <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </label>
        <label class="aiops-filter aiops-filter-grow">
          <span>命名空间</span>
          <input
            v-model="namespace"
            type="text"
            placeholder="留空查看全部"
            @keydown.enter="loadTopology"
          />
        </label>
        <button type="button" class="secondary-button" :disabled="!selectedClusterID || topologyStatus === 'loading'" @click="loadTopology">
          <Search :size="14" /> 查询
        </button>
      </div>

      <div v-if="!selectedClusterID" class="aiops-empty">
        <Boxes :size="30" />
        <strong>没有已启用的集群</strong>
        <span>接入集群后可查看资源拓扑关系。</span>
      </div>
      <template v-else>
        <article class="aiops-card aiops-completeness">
          <div class="completeness-main">
            <span class="completeness-label">拓扑完整性</span>
            <span v-if="topology" class="badge" :class="`badge-${topology.completeness.coverage}`">
              {{ coverageLabel(topology.completeness.coverage) }}
            </span>
            <span v-else class="muted">未加载</span>
            <small v-if="topology">陈旧边 {{ topology.completeness.stale_edges }} 条</small>
          </div>
          <div v-if="missingProducers.length" class="completeness-missing">
            <TriangleAlert :size="14" />
            <span>缺失生产者：</span>
            <em v-for="p in missingProducers" :key="p">{{ p }}</em>
          </div>
        </article>

        <article class="aiops-card topology-canvas-card">
          <header class="aiops-card-heading">
            <h3><Network :size="16" /> 资源拓扑图</h3>
            <button
              type="button"
              class="icon-button"
              title="刷新拓扑"
              aria-label="刷新拓扑"
              :disabled="topologyStatus === 'loading'"
              @click="loadTopology"
            >
              <RefreshCw :size="16" :class="{ spinning: topologyStatus === 'loading' }" />
            </button>
          </header>

          <p v-if="topologyError" class="error-message">{{ topologyError }}</p>
          <div v-else-if="topologyStatus === 'loading'" class="aiops-empty">
            <span class="loading-indicator" /> 正在加载拓扑…
          </div>
          <div v-else-if="!topologyLayout" class="aiops-empty">
            <Network :size="28" />
            <strong>没有拓扑节点</strong>
            <span>当前范围内未发现资源关系。</span>
          </div>
          <div v-else class="topology-svg-wrap">
            <svg
              :viewBox="`0 0 ${topologyLayout.width} ${topologyLayout.height}`"
              :width="topologyLayout.width"
              :height="topologyLayout.height"
              class="topology-svg"
              role="img"
              aria-label="资源拓扑图"
            >
              <defs>
                <marker
                  id="aiops-arrow"
                  viewBox="0 0 10 10"
                  refX="9"
                  refY="5"
                  markerWidth="7"
                  markerHeight="7"
                  orient="auto-start-reverse"
                >
                  <path d="M0,0 L10,5 L0,10 z" fill="#9ca3af" />
                </marker>
              </defs>
              <g class="topology-edges">
                <g v-for="e in topologyLayout.edges" :key="e.kind + e.key">
                  <line
                    :x1="e.x1"
                    :y1="e.y1"
                    :x2="e.x2"
                    :y2="e.y2"
                    stroke="#9ca3af"
                    stroke-width="1.5"
                    marker-end="url(#aiops-arrow)"
                  />
                  <title>关系：{{ e.kind }}</title>
                  <rect
                    :x="e.mx - e.labelW / 2"
                    :y="e.my - 7"
                    :width="e.labelW"
                    height="14"
                    rx="3"
                    fill="#ffffff"
                    fill-opacity="0.88"
                  />
                  <text :x="e.mx" :y="e.my + 3" text-anchor="middle" class="edge-label">{{ e.kind }}</text>
                </g>
              </g>
              <g class="topology-nodes">
                <g v-for="n in topologyLayout.nodes" :key="n.key" :transform="`translate(${n.x},${n.y})`">
                  <title>{{ n.kind }} / {{ n.ns || 'cluster-scoped' }} / {{ n.name }} · 关联边 {{ n.edgeCount }}</title>
                  <rect
                    :width="n.w"
                    :height="n.h"
                    rx="6"
                    :fill="n.color"
                    fill-opacity="0.1"
                    :stroke="n.color"
                    stroke-width="1.5"
                  />
                  <text :x="10" :y="20" class="node-kind" :fill="n.color">{{ n.kind }}</text>
                  <text :x="10" :y="39" class="node-name">{{ n.name }}</text>
                  <text :x="10" :y="51" class="node-ns">{{ n.ns || 'cluster-scoped' }}</text>
                </g>
              </g>
            </svg>
          </div>
        </article>

        <article class="aiops-card">
          <header class="aiops-card-heading">
            <h3><GitBranch :size="16" /> 拓扑变更时间线</h3>
            <span>{{ changes.length }} 条</span>
          </header>
          <div v-if="changes.length === 0" class="aiops-inline-empty">没有变更事件</div>
          <ol v-else class="timeline">
            <li v-for="c in changes" :key="c.id" class="timeline-item">
              <span class="timeline-dot" />
              <div class="timeline-body">
                <div class="timeline-head">
                  <span class="badge badge-kind">{{ c.kind }}</span>
                  <strong>{{ resourceRef(c.target) }}</strong>
                  <span class="timeline-action">{{ c.action }}</span>
                  <span class="badge" :class="`badge-result-${c.result}`">{{ c.result }}</span>
                </div>
                <small class="timeline-time">
                  {{ formatTime(c.started_at) }}<template v-if="c.finished_at"> → {{ formatTime(c.finished_at) }}</template>
                  · {{ c.actor }}<template v-if="c.source"> · {{ c.source }}</template>
                </small>
              </div>
            </li>
          </ol>
        </article>
      </template>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.aiops-section {
  display: grid;
  gap: 16px;
}

.aiops-tabs {
  display: flex;
  gap: 4px;
  margin: 18px 0;
  padding: 4px;
  width: fit-content;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
}

.aiops-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
  background: transparent;
  border: 0;
  border-radius: var(--radius-md);
}

.aiops-tabs button:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.6);
}

.aiops-tabs button.active {
  color: #ffffff;
  background: var(--accent-primary);
  box-shadow: var(--shadow-sm);
}

.aiops-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: 20px;
  box-shadow: var(--shadow-sm);
}

.aiops-card-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.aiops-card-heading h3 {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 600;
}

.aiops-card-heading span {
  color: var(--text-tertiary);
  font-size: 12px;
}

.partial-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  color: var(--status-warning);
  background: var(--warning-bg);
  border: 1px solid #ead7a8;
  border-radius: var(--radius-md);
}

.partial-banner svg {
  flex: 0 0 18px;
  margin-top: 1px;
}

.partial-banner strong {
  display: block;
  color: #8a6d10;
  font-size: 13px;
}

.partial-banner span {
  color: #97732c;
  font-size: 12px;
}

.aiops-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.aiops-stat {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px 18px;
  border-left: 3px solid var(--gray-400);
}

.aiops-stat.tone-blue { border-left-color: var(--status-info); }
.aiops-stat.tone-green { border-left-color: var(--status-success); }
.aiops-stat.tone-red { border-left-color: var(--status-danger); }
.aiops-stat.tone-amber { border-left-color: var(--status-warning); }

.aiops-stat span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.aiops-stat strong {
  color: var(--text-primary);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.1;
}

.aiops-stat small {
  color: var(--text-muted);
  font-size: 11px;
}

.aiops-grid-2 {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
  gap: 14px;
}

.producer-list {
  display: grid;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.producer-list li {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-rows: auto auto;
  gap: 2px 12px;
  align-items: center;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}

.producer-main {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.producer-main strong {
  color: var(--text-primary);
  font-size: 13px;
}

.producer-main small {
  color: var(--text-tertiary);
  font-size: 11px;
}

.producer-meta {
  grid-column: 1 / -1;
  color: var(--text-muted);
  font-size: 11px;
}

.aiops-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}

.aiops-table {
  width: 100%;
  min-width: 720px;
  border-collapse: collapse;
}

.aiops-table th {
  padding: 9px 12px;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  text-align: left;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
}

.aiops-table td {
  padding: 10px 12px;
  color: var(--text-primary);
  font-size: 12px;
  border-bottom: 1px solid var(--border-subtle);
  vertical-align: top;
}

.aiops-table tbody tr:last-child td {
  border-bottom: 0;
}

.aiops-table tbody tr:hover {
  background: var(--bg-secondary);
}

.aiops-table td strong {
  display: block;
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 600;
}

.aiops-table td small {
  display: block;
  margin-top: 2px;
  color: var(--text-tertiary);
  font-size: 10px;
}

.badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  white-space: nowrap;
}

.badge-critical { color: var(--status-danger); background: var(--danger-bg); }
.badge-warning { color: var(--status-warning); background: var(--warning-bg); }
.badge-info { color: var(--status-info); background: var(--info-bg); }

.badge-active { color: var(--status-danger); background: var(--danger-bg); }
.badge-resolved { color: var(--status-success); background: var(--success-bg); }
.badge-stale,
.badge-suppressed { color: var(--text-secondary); background: var(--bg-tertiary); }

.badge-complete { color: var(--status-success); background: var(--success-bg); }
.badge-partial { color: var(--status-warning); background: var(--warning-bg); }
.badge-missing { color: var(--status-danger); background: var(--danger-bg); }
.badge-unavailable { color: var(--text-secondary); background: var(--bg-tertiary); }
.badge-truncated { color: var(--status-warning); background: var(--warning-bg); }

.badge-kind {
  color: var(--accent-primary);
  background: var(--accent-subtle);
}

.badge-result-success,
.badge-result-succeeded,
.badge-result-applied,
.badge-result-effective { color: var(--status-success); background: var(--success-bg); }

.badge-result-failed,
.badge-result-failure,
.badge-result-error,
.badge-result-ineffective { color: var(--status-danger); background: var(--danger-bg); }

.badge-result-pending,
.badge-result-running,
.badge-result-progress { color: var(--status-warning); background: var(--warning-bg); }

.timeline {
  display: grid;
  gap: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}

.timeline-item {
  display: grid;
  grid-template-columns: 14px minmax(0, 1fr);
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--border-subtle);
}

.timeline-item:first-child {
  border-top: 0;
}

.timeline-dot {
  width: 9px;
  height: 9px;
  margin-top: 5px;
  background: var(--accent-primary);
  border-radius: 50%;
}

.timeline-body {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.timeline-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.timeline-head strong {
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.timeline-action {
  color: var(--text-secondary);
  font-size: 11px;
}

.timeline-time {
  color: var(--text-muted);
  font-size: 11px;
}

.aiops-toolbar {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  padding: 14px 16px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  flex-wrap: wrap;
}

.aiops-filter {
  display: grid;
  gap: 4px;
  min-width: 140px;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.aiops-filter-grow {
  flex: 1;
  min-width: 180px;
}

.aiops-filter select,
.aiops-filter input {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  outline: none;
  box-shadow: var(--shadow-inset);
}

.aiops-filter input {
  width: 100%;
}

.aiops-filter select:focus,
.aiops-filter input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.aiops-toolbar-meta {
  margin-left: auto;
  align-self: center;
  color: var(--text-tertiary);
  font-size: 12px;
}

.aiops-load-more {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: center;
  padding: 4px 0;
}

.aiops-load-more span {
  color: var(--text-muted);
  font-size: 11px;
}

.aiops-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 220px;
  gap: 8px;
  color: var(--text-muted);
  text-align: center;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.aiops-empty strong {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 13px;
}

.aiops-empty span {
  font-size: 11px;
}

.aiops-inline-empty {
  padding: 24px 0;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}

.aiops-completeness {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
  flex-wrap: wrap;
}

.completeness-main {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.completeness-label {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.completeness-main small {
  color: var(--text-muted);
  font-size: 11px;
}

.completeness-missing {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  color: var(--status-warning);
  font-size: 11px;
}

.completeness-missing em {
  padding: 2px 7px;
  color: #8a6d10;
  font-style: normal;
  background: var(--warning-bg);
  border-radius: var(--radius-full);
}

.muted {
  color: var(--text-muted);
  font-size: 12px;
}

.topology-canvas-card {
  padding: 16px;
}

.topology-svg-wrap {
  overflow: auto;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background:
    linear-gradient(var(--bg-secondary), var(--bg-secondary)),
    repeating-linear-gradient(0deg, var(--border-subtle) 0 1px, transparent 1px 24px),
    repeating-linear-gradient(90deg, var(--border-subtle) 0 1px, transparent 1px 24px);
  background-blend-mode: normal, multiply, multiply;
}

.topology-svg {
  display: block;
  min-width: 100%;
}

.edge-label {
  fill: #6b7280;
  font-size: 9px;
  font-weight: 600;
}

.node-kind {
  font-size: 9px;
  font-weight: 700;
}

.node-name {
  fill: #1d2226;
  font-size: 12px;
  font-weight: 600;
}

.node-ns {
  fill: #6b7280;
  font-size: 9px;
}

.error-message {
  margin: 0 0 12px;
  padding: 10px 12px;
  color: var(--status-danger);
  font-size: 12px;
  background: var(--danger-bg);
  border-left: 3px solid var(--status-danger);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.loading-indicator {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid var(--border-default);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.spinning {
  animation: spin 0.8s linear infinite;
}

.secondary-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 14px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-xs);
}

.secondary-button:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--bg-secondary);
  border-color: var(--border-strong);
}

.secondary-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.icon-button {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-xs);
}

.icon-button:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--bg-secondary);
  border-color: var(--border-strong);
}

.icon-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1100px) {
  .aiops-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .aiops-grid-2 {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 720px) {
  .aiops-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
  .aiops-filter,
  .aiops-filter-grow {
    min-width: 0;
  }
  .aiops-toolbar-meta {
    margin-left: 0;
  }
  .aiops-table {
    min-width: 640px;
  }
}
</style>
