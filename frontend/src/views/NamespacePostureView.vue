<script setup lang="ts">
import { computed, onMounted, ref, watch, h, defineComponent, PropType } from 'vue'
import {
  Activity,
  AlertTriangle,
  Boxes,
  CheckCircle2,
  ChevronRight,
  Clock,
  Database,
  Gauge,
  LayoutGrid,
  RefreshCw,
  Server,
  ShieldCheck,
  X,
} from 'lucide-vue-next'

import * as k8sAPI from '../api/kubernetes'
import * as clusterAPI from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { EvidenceCitation, NamespacePosture, PostureListEntry, SourceStatus } from '../types/kubernetes'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const postureList = ref<PostureListEntry[]>([])
const selectedNamespace = ref<NamespacePosture | null>(null)
const loading = ref(true)
const loadingDetail = ref(false)
const errorMessage = ref('')
const detailError = ref('')
const searchQuery = ref('')
let listSequence = 0

const sectionLabels: Record<string, string> = {
  resource_quotas: '资源配额',
  limit_ranges: '限制范围',
  workloads: '工作负载',
  pods: 'Pod 实例',
  pdbs: '中断预算',
  node_capacity: '节点容量',
}

const statusLabels: Record<SourceStatus, string> = {
  complete: '完整',
  partial: '部分',
  truncated: '已截断',
  unavailable: '不可用',
}

const statusClasses: Record<SourceStatus, string> = {
  complete: 'running',
  partial: 'unknown',
  truncated: 'pending',
  unavailable: 'failed',
}

const EvidenceBadge = defineComponent({
  name: 'EvidenceBadge',
  props: { evidence: { type: Object as PropType<EvidenceCitation>, required: true } },
  setup(props) {
    const status = computed<SourceStatus>(() => props.evidence.status)
    const summary = computed(() => {
      const e = props.evidence
      if (e.total > 0) return `${e.returned}/${e.total}${e.remaining > 0 ? ` +${e.remaining}` : ''}`
      return ''
    })
    const iconComponent = () => {
      switch (status.value) {
        case 'complete': return CheckCircle2
        case 'unavailable': return AlertTriangle
        default: return Clock
      }
    }
    return () => h('span', { class: ['evidence-badge', statusClasses[status.value]] }, [
      h(iconComponent(), { size: 12 }),
      h('span', { style: 'margin-left:4px' }, statusLabels[status.value]),
      summary.value ? h('span', { class: 'muted small', style: 'margin-left:6px' }, ` · ${summary.value}`) : null,
    ])
  },
})

const filteredList = computed(() => {
  if (!searchQuery.value) return postureList.value
  const lower = searchQuery.value.toLowerCase()
  return postureList.value.filter((e) => e.name.toLowerCase().includes(lower))
})

function phaseLabelClass(phase: string): string {
  const p = (phase || '').toLowerCase()
  if (p === 'running' || p === 'succeeded' || p === 'active') return 'running'
  if (p === 'failed' || p === 'unknown') return 'failed'
  return 'pending'
}

function badgeClass(partial: string[]): string {
  if (partial.length === 0) return 'running'
  if (partial.some((s) => s === 'resource_quotas' || s === 'workloads' || s === 'pods')) return 'failed'
  return 'pending'
}

function statusBadge(partial: string[]): string {
  if (partial.length === 0) return '完整'
  return `${partial.length} 段不完整`
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

async function initialize() {
  loading.value = true
  errorMessage.value = ''
  try {
    const list = await clusterAPI.listClusters(auth.accessToken)
    clusters.value = list.items.filter((c) => c.enabled)
    if (clusters.value.length > 0) selectedClusterID.value = clusters.value[0].id
  } catch {
    errorMessage.value = '无法加载集群列表'
  } finally {
    loading.value = false
  }
}

async function loadList() {
  if (!selectedClusterID.value) {
    postureList.value = []
    return
  }
  loading.value = true
  errorMessage.value = ''
  const sequence = ++listSequence
  try {
    const response = await k8sAPI.listNamespacePostures(auth.accessToken, selectedClusterID.value)
    if (sequence !== listSequence) return
    postureList.value = response.items
  } catch (error) {
    if (sequence !== listSequence) return
    postureList.value = []
    errorMessage.value = error instanceof Error ? error.message : '无法加载命名空间态势'
  } finally {
    if (sequence === listSequence) loading.value = false
  }
}

async function openDetail(entry: PostureListEntry) {
  if (!selectedClusterID.value) return
  loadingDetail.value = true
  detailError.value = ''
  selectedNamespace.value = null
  try {
    selectedNamespace.value = await k8sAPI.getNamespacePosture(auth.accessToken, selectedClusterID.value, entry.name)
  } catch (error) {
    detailError.value = error instanceof Error ? error.message : '无法加载命名空间详情'
  } finally {
    loadingDetail.value = false
  }
}

function closeDetail() {
  selectedNamespace.value = null
  detailError.value = ''
}

watch(selectedClusterID, () => {
  selectedNamespace.value = null
  void loadList()
})
onMounted(() => void initialize().then(() => void loadList()))
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="命名空间治理态势">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="loading || !selectedClusterID" @click="loadList">
        <RefreshCw :size="16" :class="{ spin: loading }" />
        <span>刷新</span>
      </button>
      <select v-model="selectedClusterID" class="cluster-select" aria-label="选择集群" :disabled="clusters.length === 0">
        <option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option>
      </select>
    </template>
    <div class="two-column">
      <section class="panel">
        <header class="panel-header">
          <div class="panel-title">
            <LayoutGrid :size="18" />
            <strong>命名空间列表</strong>
            <span class="muted">{{ postureList.length }} 项</span>
          </div>
          <input
            v-model="searchQuery"
            type="search"
            class="compact-input"
            placeholder="按命名空间搜索…"
            aria-label="命名空间搜索"
          />
        </header>
        <div v-if="loading" class="panel-empty">加载中…</div>
        <div v-else-if="errorMessage" class="panel-empty error">{{ errorMessage }}</div>
        <div v-else-if="filteredList.length === 0" class="panel-empty muted">没有匹配的命名空间</div>
        <div v-else class="governance-list">
          <button
            v-for="entry in filteredList"
            :key="entry.name"
            type="button"
            class="governance-row"
            :class="{ active: selectedNamespace?.name === entry.name }"
            @click="openDetail(entry)"
          >
            <div class="governance-row-main">
              <div class="governance-name">
                <Boxes :size="16" />
                <strong>{{ entry.name }}</strong>
                <span :class="['phase-badge', entry.phase.toLowerCase() === 'active' ? 'running' : 'pending']">
                  {{ entry.phase }}
                </span>
              </div>
              <div class="governance-meta">
                <span title="工作负载"><Gauge :size="14" /> {{ entry.workload_count }}</span>
                <span title="Pod"><Server :size="14" /> {{ entry.pod_count }}</span>
                <span title="配额"><Database :size="14" /> {{ entry.quota_count }}</span>
                <span title="限制范围"><Activity :size="14" /> {{ entry.limit_range_count }}</span>
                <span title="PDB"><ShieldCheck :size="14" /> {{ entry.pdb_count }}</span>
              </div>
            </div>
            <div class="governance-row-tail">
              <span :class="['phase-badge', badgeClass(entry.partial_sections)]">
                {{ statusBadge(entry.partial_sections) }}
              </span>
              <ChevronRight :size="16" class="chevron" />
            </div>
          </button>
        </div>
      </section>

      <section class="panel detail-panel">
        <header class="panel-header sticky">
          <div class="panel-title">
            <Boxes :size="18" />
            <strong>{{ selectedNamespace?.name ?? '命名空间详情' }}</strong>
          </div>
          <button v-if="selectedNamespace" type="button" class="icon-button" title="关闭" @click="closeDetail">
            <X :size="16" />
          </button>
        </header>
        <div v-if="!selectedNamespace && !loadingDetail && !detailError" class="panel-empty muted">
          选择左侧某个命名空间查看完整的证据引用态势
        </div>
        <div v-else-if="loadingDetail" class="panel-empty">加载中…</div>
        <div v-else-if="detailError" class="panel-empty error">{{ detailError }}</div>
        <div v-else-if="selectedNamespace" class="posture-detail">
          <div class="posture-meta">
            <div class="meta-row">
              <span class="muted">状态</span>
              <span :class="['phase-badge', selectedNamespace.phase.toLowerCase() === 'active' ? 'running' : 'pending']">
                {{ selectedNamespace.phase }}
              </span>
            </div>
            <div class="meta-row">
              <span class="muted">创建时间</span>
              <span>{{ formatTimestamp(selectedNamespace.created_at) }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">标签</span>
              <span v-if="selectedNamespace.labels && Object.keys(selectedNamespace.labels).length">
                {{ Object.keys(selectedNamespace.labels).length }} 项
              </span>
              <span v-else class="muted">—</span>
            </div>
            <div class="meta-row" v-if="selectedNamespace.partial_sections.length">
              <span class="muted">证据不完整段</span>
              <span class="tag-group">
                <span v-for="section in selectedNamespace.partial_sections" :key="section" class="phase-badge pending">
                  {{ sectionLabels[section] ?? section }}
                </span>
              </span>
            </div>
          </div>

          <section class="posture-section">
            <div class="section-head">
              <AlertTriangle :size="16" /><h3>治理风险</h3>
              <span :class="['phase-badge', selectedNamespace.overall_state === 'healthy' ? 'running' : selectedNamespace.overall_state === 'critical' || selectedNamespace.overall_state === 'incomplete' ? 'failed' : 'pending']">
                {{ selectedNamespace.overall_state }}
              </span>
            </div>
            <div v-if="selectedNamespace.findings.length === 0" class="panel-empty muted small">未发现固定规则风险</div>
            <table v-else class="compact-table">
              <thead><tr><th>级别</th><th>风险码</th><th>对象</th><th>结论</th><th>观测时间</th></tr></thead>
              <tbody>
                <tr v-for="finding in selectedNamespace.findings" :key="`${finding.code}-${finding.resource.kind}-${finding.resource.name}`">
                  <td><span :class="['phase-badge', finding.severity === 'critical' ? 'failed' : finding.severity === 'warning' ? 'pending' : 'running']">{{ finding.severity }}</span></td>
                  <td class="mono small">{{ finding.code }}</td>
                  <td class="mono small">{{ finding.resource.kind }}/{{ finding.resource.name }}</td>
                  <td>{{ finding.summary }}</td>
                  <td class="small">{{ formatTimestamp(finding.observed_at) }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <Database :size="16" /><h3>资源配额 (ResourceQuota)</h3>
              <EvidenceBadge :evidence="selectedNamespace.resource_quotas.evidence" />
            </div>
            <div v-if="selectedNamespace.resource_quotas.quotas.length === 0" class="panel-empty muted small">
              未配置 ResourceQuota
            </div>
            <div v-else class="quota-blocks">
              <div v-for="quota in selectedNamespace.resource_quotas.quotas" :key="quota.name" class="quota-block">
                <div class="quota-title"><strong>{{ quota.name }}</strong></div>
                <table class="compact-table">
                  <thead><tr><th>资源</th><th>上限</th><th>已用</th></tr></thead>
                  <tbody>
                    <tr v-for="(value, key) in quota.hard" :key="`${quota.name}-${key}`">
                      <td class="mono">{{ key }}</td>
                      <td class="mono">{{ value ?? '—' }}</td>
                      <td class="mono">{{ quota.used?.[key as string] ?? '0' }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <Activity :size="16" /><h3>限制范围 (LimitRange)</h3>
              <EvidenceBadge :evidence="selectedNamespace.limit_ranges.evidence" />
            </div>
            <div v-if="selectedNamespace.limit_ranges.ranges.length === 0" class="panel-empty muted small">
              未配置 LimitRange
            </div>
            <div v-else class="limit-range-list">
              <div v-for="lr in selectedNamespace.limit_ranges.ranges" :key="lr.metadata.name" class="limit-range-card">
                <div class="lr-title"><strong>{{ lr.metadata.name }}</strong></div>
                <table class="compact-table">
                  <thead><tr><th>类型</th><th>资源</th><th>默认</th><th>默认请求</th><th>最小</th><th>最大</th></tr></thead>
                  <tbody>
                    <template v-for="item in lr.spec.limits" :key="`${lr.metadata.name}-${item.type}`">
                      <tr v-for="(resKey, idx) in collectLRKeys(item)" :key="`${item.type}-${resKey}`">
                        <td v-if="idx === 0" :rowspan="collectLRKeys(item).length">
                          <span class="phase-badge pending">{{ item.type }}</span>
                        </td>
                        <td class="mono">{{ resKey }}</td>
                        <td class="mono">{{ item.default?.[resKey] ?? '—' }}</td>
                        <td class="mono">{{ item.defaultRequest?.[resKey] ?? '—' }}</td>
                        <td class="mono">{{ item.min?.[resKey] ?? '—' }}</td>
                        <td class="mono">{{ item.max?.[resKey] ?? '—' }}</td>
                      </tr>
                    </template>
                  </tbody>
                </table>
              </div>
            </div>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <Gauge :size="16" /><h3>工作负载汇总</h3>
              <EvidenceBadge :evidence="selectedNamespace.workloads.evidence" />
            </div>
            <div class="summary-grid">
              <div class="summary-card"><p class="muted">总种类</p><strong>{{ selectedNamespace.workloads.total_count }}</strong></div>
              <div class="summary-card"><p class="muted">期望副本</p><strong>{{ selectedNamespace.workloads.desired_total }}</strong></div>
              <div class="summary-card"><p class="muted">就绪副本</p><strong>{{ selectedNamespace.workloads.ready_total }}</strong></div>
              <div class="summary-card">
                <p class="muted">就绪率</p>
                <strong>
                  {{ selectedNamespace.workloads.desired_total > 0
                    ? `${Math.round((selectedNamespace.workloads.ready_total / selectedNamespace.workloads.desired_total) * 100)}%`
                    : '—' }}
                </strong>
              </div>
            </div>
            <table v-if="selectedNamespace.workloads.by_kind.length" class="compact-table">
              <thead><tr><th>类型</th><th>数量</th><th>期望</th><th>就绪</th><th>可用</th><th>已更新</th><th>失败</th></tr></thead>
              <tbody>
                <tr v-for="k in selectedNamespace.workloads.by_kind" :key="k.kind">
                  <td><strong>{{ k.kind }}</strong></td>
                  <td>{{ k.count }}</td>
                  <td>{{ k.desired_replicas }}</td>
                  <td>{{ k.ready_replicas }}</td>
                  <td>{{ k.available_replicas }}</td>
                  <td>{{ k.updated_replicas }}</td>
                  <td>{{ k.failed_replicas }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <Server :size="16" /><h3>Pod 分布</h3>
              <EvidenceBadge :evidence="selectedNamespace.pods.evidence" />
            </div>
            <div class="summary-grid">
              <div class="summary-card"><p class="muted">总 Pod 数</p><strong>{{ selectedNamespace.pods.total }}</strong></div>
              <div class="summary-card"><p class="muted">已调度</p><strong>{{ selectedNamespace.pods.scheduled }}</strong></div>
              <div class="summary-card"><p class="muted">分布节点数</p><strong>{{ selectedNamespace.pods.unique_node_count }}</strong></div>
            </div>
            <div class="dual-subgrid">
              <div>
                <h4>按阶段</h4>
                <div v-if="selectedNamespace.pods.by_phase.length === 0" class="muted small">无数据</div>
                <ul v-else class="inline-list">
                  <li v-for="phase in selectedNamespace.pods.by_phase" :key="phase.phase">
                    <span :class="['phase-badge', phaseLabelClass(phase.phase)]">{{ phase.phase }}</span>
                    <strong>{{ phase.count }}</strong>
                  </li>
                </ul>
              </div>
              <div>
                <h4>按节点 (Top 6)</h4>
                <div v-if="selectedNamespace.pods.by_node.length === 0" class="muted small">无数据</div>
                <ul v-else class="inline-list">
                  <li v-for="n in selectedNamespace.pods.by_node.slice(0, 6)" :key="n.node_name">
                    <span class="mono small">{{ truncateNode(n.node_name) }}</span>
                    <strong>{{ n.count }}</strong>
                  </li>
                </ul>
              </div>
            </div>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <ShieldCheck :size="16" /><h3>Pod 中断预算 (PDB)</h3>
              <EvidenceBadge :evidence="selectedNamespace.pdbs.evidence" />
            </div>
            <div v-if="selectedNamespace.pdbs.pdbs.length === 0" class="panel-empty muted small">
              未配置 PDB，滚动维护时缺少可用性保护
            </div>
            <table v-else class="compact-table">
              <thead><tr><th>名称</th><th>Min</th><th>Max 不可用</th><th>当前健康</th><th>期望</th><th>允许中断</th><th>期望 Pod</th></tr></thead>
              <tbody>
                <tr v-for="pdb in selectedNamespace.pdbs.pdbs" :key="pdb.name">
                  <td><strong>{{ pdb.name }}</strong></td>
                  <td class="mono">{{ pdb.min_available ?? '—' }}</td>
                  <td class="mono">{{ pdb.max_unavailable ?? '—' }}</td>
                  <td>{{ pdb.current_healthy }}</td>
                  <td>{{ pdb.desired_healthy }}</td>
                  <td><span :class="['phase-badge', pdb.disruptions_allowed > 0 ? 'running' : 'pending']">{{ pdb.disruptions_allowed }}</span></td>
                  <td>{{ pdb.expected_pods }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <Boxes :size="16" /><h3>集群节点容量 (上下文)</h3>
              <EvidenceBadge :evidence="selectedNamespace.node_capacity.evidence" />
            </div>
            <p class="muted small">
              以下为整个集群的节点容量与可分配资源，作为命名空间配额的上下文分母；
              平台<strong>不</strong>推断该命名空间占集群的份额，因为这需要 QoS / 抢占 / 超售等调度器语义。
            </p>
            <table v-if="selectedNamespace.node_capacity.nodes.length" class="compact-table">
              <thead><tr><th>节点</th><th>可调度</th><th>CPU 容量</th><th>内存 容量</th><th>CPU 可分配</th><th>内存 可分配</th></tr></thead>
              <tbody>
                <tr v-for="node in selectedNamespace.node_capacity.nodes" :key="node.name">
                  <td class="mono">{{ node.name }}</td>
                  <td>
                    <span v-if="node.schedulable" class="phase-badge running">是</span>
                    <span v-else class="phase-badge pending">否</span>
                  </td>
                  <td class="mono">{{ node.capacity?.cpu ?? '—' }}</td>
                  <td class="mono">{{ node.capacity?.memory ?? '—' }}</td>
                  <td class="mono">{{ node.allocatable?.cpu ?? '—' }}</td>
                  <td class="mono">{{ node.allocatable?.memory ?? '—' }}</td>
                </tr>
              </tbody>
            </table>
          </section>
        </div>
      </section>
    </div>
  </ConsoleLayout>
</template>

<script lang="ts">
import type { LimitRangeItem } from '../types/kubernetes'
function collectLRKeys(item: LimitRangeItem): string[] {
  const keys = new Set<string>()
  for (const k of Object.keys(item.default ?? {})) keys.add(k)
  for (const k of Object.keys(item.defaultRequest ?? {})) keys.add(k)
  for (const k of Object.keys(item.min ?? {})) keys.add(k)
  for (const k of Object.keys(item.max ?? {})) keys.add(k)
  return [...keys].sort()
}
function truncateNode(name: string): string {
  if (name.length <= 18) return name
  return name.slice(0, 18) + '…'
}
</script>

<style scoped>
.governance-list { display: flex; flex-direction: column; gap: 6px; }
.governance-row {
  display: flex; align-items: center; justify-content: space-between;
  width: 100%; text-align: left; padding: 10px 12px;
  background: var(--surface-2); border: 1px solid var(--border-soft);
  border-radius: 10px; cursor: pointer; transition: background 0.15s, border-color 0.15s;
}
.governance-row:hover { background: var(--surface-3); border-color: var(--border); }
.governance-row.active { border-color: var(--accent); background: var(--surface-accent); }
.governance-row-main { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.governance-name { display: flex; align-items: center; gap: 8px; }
.governance-meta { display: flex; gap: 12px; color: var(--text-muted); font-size: 12px; }
.governance-meta > span { display: inline-flex; align-items: center; gap: 4px; }
.governance-row-tail { display: flex; align-items: center; gap: 8px; }
.chevron { color: var(--text-muted); }

.posture-detail { display: flex; flex-direction: column; gap: 18px; padding: 8px 4px; }
.posture-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; padding: 12px; background: var(--surface-2); border-radius: 10px; }
.meta-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.tag-group { display: inline-flex; flex-wrap: wrap; gap: 4px; }

.posture-section { border: 1px solid var(--border-soft); border-radius: 10px; padding: 14px; display: flex; flex-direction: column; gap: 12px; }
.section-head { display: flex; align-items: center; gap: 8px; }
.section-head h3 { font-size: 14px; margin: 0; }
.evidence-badge { display: inline-flex; align-items: center; padding: 2px 8px; border-radius: 999px; font-size: 12px; margin-left: auto; }
.evidence-badge.running { background: rgba(34,197,94,0.12); color: #22c55e; }
.evidence-badge.pending { background: rgba(234,179,8,0.12); color: #eab308; }
.evidence-badge.failed { background: rgba(239,68,68,0.12); color: #ef4444; }
.evidence-badge.unknown { background: rgba(148,163,184,0.16); color: #94a3b8; }

.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.summary-card { padding: 10px 12px; background: var(--surface-2); border-radius: 8px; }
.summary-card p { margin: 0 0 4px; font-size: 12px; }

.quota-blocks { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 10px; }
.quota-block { border: 1px solid var(--border-soft); border-radius: 8px; padding: 10px; }
.quota-title { margin-bottom: 8px; }

.limit-range-list { display: flex; flex-direction: column; gap: 10px; }
.limit-range-card { border: 1px solid var(--border-soft); border-radius: 8px; padding: 10px; }
.lr-title { margin-bottom: 8px; }

.dual-subgrid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.dual-subgrid h4 { font-size: 12px; color: var(--text-muted); margin: 0 0 8px; }
.inline-list { display: flex; flex-direction: column; gap: 4px; list-style: none; padding: 0; margin: 0; }
.inline-list li { display: flex; align-items: center; justify-content: space-between; padding: 4px 8px; background: var(--surface-2); border-radius: 6px; font-size: 12px; }

.mono { font-family: var(--font-mono); }
.small { font-size: 12px; }
</style>
