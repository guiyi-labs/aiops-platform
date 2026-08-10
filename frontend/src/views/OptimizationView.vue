<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  BadgeCheck,
  CheckCircle2,
  CircleDollarSign,
  ClipboardCheck,
  Cpu,
  Container,
  Gauge,
  GitBranch,
  Globe,
  History,
  MemoryStick,
  Network,
  PackageX,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  TrendingUp,
  Wallet,
} from 'lucide-vue-next'

import * as clusterAPI from '../api/clusters'
import * as optimizationAPI from '../api/optimization'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import FindingEvidencePanel from '../components/FindingEvidencePanel.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type {
  CapacityStatus,
  CISStatus,
  DeprecatedAPIStatus,
  FinOpsRecommendation,
  FinOpsWasteSummary,
  GitOpsStatus,
  HPAStatus,
  ImageStatus,
  IngressStatus,
  NetworkStatus,
  OptimizationFinding,
  PDBStatus,
  PolicyStatus,
} from '../types/optimization'
import { fromFinOpsRecommendation, fromOptimizationFinding } from '../utils/finding-detail'

// Read-only optimization console (M66) over the M61-M70 analyzers.
//
// Each tab posts only the cluster id, which makes the server auto-collect the
// observation bundle and run the corresponding pure analyzer. No request from
// this view can mutate cluster state (ADR 0004).

type TabKey = 'finops' | 'cis' | 'deprecated' | 'network' | 'image' | 'gitops' | 'capacity' | 'policy' | 'hpa' | 'pdb' | 'ingress'

const auth = useAuthStore()

const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const activeTab = ref<TabKey>('finops')
const clustersLoading = ref(true)
const clustersError = ref('')

const finops = ref<FinOpsWasteSummary | null>(null)
const cis = ref<CISStatus | null>(null)
const deprecated = ref<DeprecatedAPIStatus | null>(null)
const network = ref<NetworkStatus | null>(null)
const image = ref<ImageStatus | null>(null)
const gitops = ref<GitOpsStatus | null>(null)
const capacity = ref<CapacityStatus | null>(null)
const policy = ref<PolicyStatus | null>(null)
const hpa = ref<HPAStatus | null>(null)
const pdb = ref<PDBStatus | null>(null)
const ingress = ref<IngressStatus | null>(null)

const loading = ref<Record<TabKey, boolean>>({ finops: false, cis: false, deprecated: false, network: false, image: false, gitops: false, capacity: false, policy: false, hpa: false, pdb: false, ingress: false })
const errors = ref<Record<TabKey, string>>({ finops: '', cis: '', deprecated: '', network: '', image: '', gitops: '', capacity: '', policy: '', hpa: '', pdb: '', ingress: '' })

const targetVersion = ref('1.29')

// Guards against a slow response for a previous cluster overwriting a newer one.
let requestSequence = 0

const tabs: { key: TabKey; label: string; icon: typeof Wallet }[] = [
  { key: 'finops', label: '成本优化', icon: Wallet },
  { key: 'cis', label: 'CIS 合规', icon: ShieldCheck },
  { key: 'deprecated', label: '废弃 API', icon: PackageX },
  { key: 'network', label: '网络连通', icon: Network },
  { key: 'image', label: '镜像供应链', icon: Container },
  { key: 'gitops', label: 'GitOps 漂移', icon: GitBranch },
  { key: 'capacity', label: '容量预测', icon: TrendingUp },
  { key: 'policy', label: '策略合规', icon: ClipboardCheck },
  { key: 'hpa', label: 'HPA 扩缩容', icon: Gauge },
  { key: 'pdb', label: 'PDB 保护', icon: ShieldAlert },
  { key: 'ingress', label: 'Ingress 暴露面', icon: Globe },
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

function findingDetail(finding: OptimizationFinding, source: string) {
  return fromOptimizationFinding(finding, { framework: 'optimization', source })
}

function finopsFindingDetail(recommendation: FinOpsRecommendation) {
  return fromFinOpsRecommendation(recommendation, finops.value?.evaluated_at)
}

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

/** Network findings ordered critical → warning → info so the worst surface first. */
const networkFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(network.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** Network policy families, ranked high → low for the chip strip. */
const networkFamilies = computed(() => Object.entries(network.value?.by_family ?? {}).sort((a, b) => b[1] - a[1]))

/** Image findings ordered critical → warning → info so the worst surface first. */
const imageFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(image.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/**
 * Share of distinct images that are fully reproducible (digest-pinned).
 * Images with a mutable tag or no digest pin are both counted as unpinned.
 */
const imagePinnedRate = computed(() => {
  const total = image.value?.images_total ?? 0
  if (total === 0) return 0
  const risky = (image.value?.mutable_tag_images ?? 0) + (image.value?.unpinned_images ?? 0)
  return Math.round(((total - risky) / total) * 100)
})

/** Highest monthly waste first — the recommendations worth acting on. */
const finopsRecommendations = computed(() =>
  [...(finops.value?.recommendations ?? [])].sort((a, b) => b.monthly_waste_usd - a.monthly_waste_usd),
)

/** GitOps findings ordered critical → warning → info so the worst surface first. */
const gitopsFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(gitops.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/**
 * Share of observed resources that have drifted away from their
 * last-applied-configuration. Unmanaged resources are excluded from the
 * denominator — without a recorded manifest, drift cannot even be measured.
 */
const gitopsDriftRate = computed(() => {
  const total = gitops.value?.resources_total ?? 0
  if (total === 0) return 0
  const drifted = gitops.value?.drifted_resources ?? 0
  return Math.round((drifted / total) * 100)
})

/** Capacity findings ordered critical → warning → info so the worst surface first. */
const capacityFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(capacity.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** CPU allocatable in whole cores. */
const cpuCores = computed(() => (capacity.value?.cpu_capacity_nanocores ?? 0) / 1e9)

/** Memory allocatable in GiB. */
const memGiB = computed(() => (capacity.value?.mem_capacity_bytes ?? 0) / 1024 ** 3)

/** Days until CPU saturates, or "—" when the trend is not growing. */
const cpuSaturationDays = computed(() => {
  const days = capacity.value?.cpu_saturation_in_days
  if (days == null || days < 0) return '—'
  return Math.round(days).toString()
})

/** Days until memory saturates, or "—" when the trend is not growing. */
const memSaturationDays = computed(() => {
  const days = capacity.value?.mem_saturation_in_days
  if (days == null || days < 0) return '—'
  return Math.round(days).toString()
})

/** Utilization ratio rendered as a percentage with one decimal place. */
function pct(value: number | string | undefined): string {
  if (value == null || value === '') return '—'
  const ratio = typeof value === 'string' ? Number.parseFloat(value) : value
  if (Number.isNaN(ratio)) return '—'
  return `${(ratio * 100).toFixed(1)}%`
}

/** Policy findings ordered critical → warning → info so the worst surface first. */
const policyFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(policy.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** Share of workloads with no policy finding at all. */
const policyComplianceRate = computed(() => {
  const total = policy.value?.workloads_total ?? 0
  if (total === 0) return 0
  return Math.round(((policy.value?.compliant_workloads ?? 0) / total) * 100)
})

/** HPA findings ordered critical → warning → info so the worst surface first. */
const hpaFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(hpa.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** PDB findings ordered critical → warning → info so the worst surface first. */
const pdbFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(pdb.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

/** Ingress findings ordered critical → warning → info so the worst surface first. */
const ingressFindings = computed<OptimizationFinding[]>(() => {
  const order: Record<string, number> = { critical: 0, warning: 1, info: 2 }
  return [...(ingress.value?.findings ?? [])].sort(
    (a, b) => (order[a.severity] ?? 3) - (order[b.severity] ?? 3),
  )
})

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
  const cached = { finops: finops.value, cis: cis.value, deprecated: deprecated.value, network: network.value, image: image.value, gitops: gitops.value, capacity: capacity.value, policy: policy.value, hpa: hpa.value, pdb: pdb.value, ingress: ingress.value }[tab]
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
    } else if (tab === 'deprecated') {
      const result = await optimizationAPI.analyzeDeprecatedAPI(auth.accessToken, clusterId, targetVersion.value)
      if (sequence === requestSequence) deprecated.value = result
    } else if (tab === 'network') {
      const result = await optimizationAPI.analyzeNetwork(auth.accessToken, clusterId)
      if (sequence === requestSequence) network.value = result
    } else if (tab === 'image') {
      const result = await optimizationAPI.analyzeImage(auth.accessToken, clusterId)
      if (sequence === requestSequence) image.value = result
    } else if (tab === 'gitops') {
      const result = await optimizationAPI.analyzeGitOps(auth.accessToken, clusterId)
      if (sequence === requestSequence) gitops.value = result
    } else if (tab === 'capacity') {
      const result = await optimizationAPI.analyzeCapacity(auth.accessToken, clusterId)
      if (sequence === requestSequence) capacity.value = result
    } else if (tab === 'policy') {
      const result = await optimizationAPI.analyzePolicy(auth.accessToken, clusterId)
      if (sequence === requestSequence) policy.value = result
    } else if (tab === 'hpa') {
      const result = await optimizationAPI.analyzeHPA(auth.accessToken, clusterId)
      if (sequence === requestSequence) hpa.value = result
    } else if (tab === 'pdb') {
      const result = await optimizationAPI.analyzePDB(auth.accessToken, clusterId)
      if (sequence === requestSequence) pdb.value = result
    } else {
      const result = await optimizationAPI.analyzeIngress(auth.accessToken, clusterId)
      if (sequence === requestSequence) ingress.value = result
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
  network.value = null
  image.value = null
  gitops.value = null
  capacity.value = null
  policy.value = null
  hpa.value = null
  pdb.value = null
  ingress.value = null
  errors.value = { finops: '', cis: '', deprecated: '', network: '', image: '', gitops: '', capacity: '', policy: '', hpa: '', pdb: '', ingress: '' }
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
                      <FindingEvidencePanel :finding="finopsFindingDetail(rec)" compact />
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
                      <FindingEvidencePanel :finding="findingDetail(item, 'cis')" compact />
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
      <section v-else-if="activeTab === 'deprecated'" class="optimization-tab">
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
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'deprecated_api')" compact />
                    </td>
                    <td><span :class="['phase-badge', severityClass(item.severity)]">{{ severityLabel(item.severity) }}</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </template>
        <div v-else class="panel-empty muted">输入目标版本后开始检查</div>
      </section>

      <!-- ----------------------------------------------- 网络连通性 / NetworkPolicy -->
      <section v-else-if="activeTab === 'network'" class="optimization-tab">
        <div v-if="loading.network" class="panel-empty">正在采集命名空间、Pod、Service 与 NetworkPolicy 并做静态可达性推理…</div>
        <div v-else-if="errors.network" class="panel-empty error">{{ errors.network }}</div>
        <template v-else-if="network">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><ShieldCheck :size="16" />默认拒绝命名空间</p>
              <strong>{{ network.isolated_namespaces }}</strong>
              <span>共 {{ network.namespaces_total }} 个命名空间</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><Network :size="16" />入向受保护 Pod</p>
              <strong>{{ network.ingress_covered_pods }}</strong>
              <span>共 {{ network.pods_total }} 个 Pod</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />对外暴露服务</p>
              <strong>{{ network.exposed_services }}</strong>
              <span>NodePort / LoadBalancer，共 {{ network.services_total }} 个</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />覆盖检查</p>
              <strong>{{ network.isolated_namespaces > 0 ? '已设基线' : '未设基线' }}</strong>
              <span>{{ network.policies_total }} 条 NetworkPolicy · 分析时间 {{ formatTimestamp(network.evaluated_at) }}</span>
            </article>
          </div>

          <div v-if="networkFamilies.length > 0" class="family-chips">
            <span v-for="[family, count] in networkFamilies" :key="family" class="family-chip">
              {{ family }}<em>{{ count }}</em>
            </span>
          </div>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <Network :size="18" />
                <strong>网络态势发现</strong>
                <span class="muted">{{ networkFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="networkFindings.length === 0" class="panel-empty muted">
              未发现在命名空间隔离、Pod 覆盖、Service 后端或对外暴露方面的异常
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in networkFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'network')" compact />
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

      <!-- ----------------------------------------------- 镜像供应链 / 可复现性 -->
      <section v-else-if="activeTab === 'image'" class="optimization-tab">
        <div v-if="loading.image" class="panel-empty">正在采集工作负载镜像引用并做静态可复现性分析…</div>
        <div v-else-if="errors.image" class="panel-empty error">{{ errors.image }}</div>
        <template v-else-if="image">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Container :size="16" />在用镜像</p>
              <strong>{{ image.images_total }}</strong>
              <span>被 {{ image.containers_total }} 个容器引用</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />可变 tag 镜像</p>
              <strong>{{ image.mutable_tag_images }}</strong>
              <span>使用 :latest 或缺省 tag，重新部署可能换成不同构建</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><History :size="16" />未钉 digest</p>
              <strong>{{ image.unpinned_images }}</strong>
              <span>仅按 tag 引用，tag 可被仓库重新指向</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />可复现率</p>
              <strong>{{ imagePinnedRate }}%</strong>
              <span>已用 digest 钉住 · 分析时间 {{ formatTimestamp(image.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图不访问任何镜像仓库，也不拉取 manifest，仅基于工作负载声明的镜像引用做静态推理。
            可复现性是 CVE 响应的前提：镜像若只由可变 tag 引用，修复版本能否真正落到线上是无法验证的。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <Container :size="18" />
                <strong>镜像供应链发现</strong>
                <span class="muted">{{ imageFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="imageFindings.length === 0" class="panel-empty muted">
              未发现可变 tag、未钉 digest、跨命名空间共享或版本漂移问题
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>镜像</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in imageFindings" :key="`${item.code}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'image')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        <template v-if="item.details?.tag">tag {{ item.details.tag }}</template>
                        <template v-else-if="item.details?.tags">tags {{ item.details.tags }}</template>
                        <template v-if="item.details?.containers"> · {{ item.details.containers }} 个容器</template>
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

      <!-- ----------------------------------------------- GitOps 漂移检测 -->
      <section v-else-if="activeTab === 'gitops'" class="optimization-tab">
        <div v-if="loading.gitops" class="panel-empty">正在采集工作负载、ConfigMap、Secret 与命名空间注解并比对 last-applied…</div>
        <div v-else-if="errors.gitops" class="panel-empty error">{{ errors.gitops }}</div>
        <template v-else-if="gitops">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><GitBranch :size="16" />受管资源</p>
              <strong>{{ gitops.resources_total }}</strong>
              <span>含工作负载、ConfigMap、Secret</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />漂移资源</p>
              <strong>{{ gitops.drifted_resources }}</strong>
              <span>实况与 last-applied 不一致</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><PackageX :size="16" />未受管资源</p>
              <strong>{{ gitops.unmanaged_resources }}</strong>
              <span>受管命名空间内缺 last-applied 注解</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />漂移率</p>
              <strong>{{ gitopsDriftRate }}%</strong>
              <span>警告 {{ gitops.by_severity.warning ?? 0 }} · 提示 {{ gitops.by_severity.info ?? 0 }} · 分析时间 {{ formatTimestamp(gitops.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图不连接任何 Git 仓库、不调用 GitOps 控制器、也不重新 apply 任何对象，仅将实况对象与
            <code>kubectl.kubernetes.io/last-applied-configuration</code> 注解（kubectl apply / Flux / Argo CD 写入）做静态比对。
            漂移意味着 GitOps 已无法干净地 reconcile 该资源；缺失注解则意味着漂移无从发现、也无从修复。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <GitBranch :size="18" />
                <strong>GitOps 漂移发现</strong>
                <span class="muted">{{ gitopsFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="gitopsFindings.length === 0" class="panel-empty muted">
              未发现实况与 last-applied 不一致的资源，也未发现受管命名空间内的未受管资源
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in gitopsFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <div v-if="item.details?.field_count" class="cell-sub muted">{{ item.details.field_count }} 个字段不一致</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'gitops')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                        <template v-if="item.details?.manager"> · {{ item.details.manager }}</template>
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

      <!-- ----------------------------------------------- 容量预测 -->
      <section v-else-if="activeTab === 'capacity'" class="optimization-tab">
        <div v-if="loading.capacity" class="panel-empty">正在汇总节点可分配容量与近 24 小时用量并拟合趋势…</div>
        <div v-else-if="errors.capacity" class="panel-empty error">{{ errors.capacity }}</div>
        <template v-else-if="capacity">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Cpu :size="16" />CPU 容量</p>
              <strong>{{ cpuCores.toFixed(1) }} 核</strong>
              <span>全部节点可分配量合计</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><MemoryStick :size="16" />内存容量</p>
              <strong>{{ memGiB.toFixed(1) }} GiB</strong>
              <span>全部节点可分配量合计</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><TrendingUp :size="16" />当前利用率</p>
              <strong>CPU {{ pct(capacity.cpu_current_pct) }}</strong>
              <span>内存 {{ pct(capacity.mem_current_pct) }}（按拟合曲线）</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />预计耗尽天数</p>
              <strong>CPU {{ cpuSaturationDays }} 天</strong>
              <span>内存 {{ memSaturationDays }} 天 · 严重 {{ capacity.by_severity.critical ?? 0 }} · 警告 {{ capacity.by_severity.warning ?? 0 }} · 分析时间 {{ formatTimestamp(capacity.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图只读汇总节点的 <code>status.allocatable</code> 与指标历史中的近 24 小时节点用量，
            对 CPU / 内存分别做最小二乘线性拟合，再投影到 30 天预测窗口。
            预计 7 天内耗尽或当前已超 100% 判为严重，30 天内达到 80% 判为警告。
            不执行任何变更、不调度 Pod、不触发扩缩容。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <TrendingUp :size="18" />
                <strong>容量饱和预警</strong>
                <span class="muted">{{ capacityFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="capacityFindings.length === 0" class="panel-empty muted">
              在预测窗口内未发现 CPU / 内存的饱和风险
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in capacityFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <div v-if="item.details?.projected_pct != null" class="cell-sub muted">
                        当前 {{ pct(item.details.current_pct) }} → 预计 {{ pct(item.details.projected_pct) }}
                        <template v-if="item.details.days_to_saturation !== 'inf'"> · {{ item.details.days_to_saturation }} 天后耗尽</template>
                      </div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'capacity')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">{{ item.resource.kind }}</div>
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

      <!-- ----------------------------------------------- 策略合规 -->
      <section v-else-if="activeTab === 'policy'" class="optimization-tab">
        <div v-if="loading.policy" class="panel-empty">正在采集工作负载清单并逐容器评估资源、安全上下文、探针与宿主访问…</div>
        <div v-else-if="errors.policy" class="panel-empty error">{{ errors.policy }}</div>
        <template v-else-if="policy">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Container :size="16" />评估工作负载</p>
              <strong>{{ policy.workloads_total }}</strong>
              <span>Deployment / StatefulSet / DaemonSet / 裸 Pod</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><Cpu :size="16" />评估容器</p>
              <strong>{{ policy.containers_total }}</strong>
              <span>逐容器检查资源与安全基线</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><BadgeCheck :size="16" />合规率</p>
              <strong>{{ policyComplianceRate }}%</strong>
              <span>{{ policy.compliant_workloads }} 个工作负载全部通过</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><ClipboardCheck :size="16" />预警数</p>
              <strong>{{ policy.failed }}</strong>
              <span>严重 {{ policy.by_severity.critical ?? 0 }} · 警告 {{ policy.by_severity.warning ?? 0 }} · 提示 {{ policy.by_severity.info ?? 0 }} · 分析时间 {{ formatTimestamp(policy.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图对每个工作负载的 Pod 模板执行声明式基线检查（对标 KubeSphere 默认策略）：
            资源 requests/limits 是否声明、是否 privileged / 允许权限提升 / 以 root 运行、
            是否使用 hostNetwork / hostPID / hostIPC、是否配置 liveness / readiness / startup 探针。
            全部为只读静态评估（ADR 0004），不安装任何策略引擎，也不修改任何对象。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <ClipboardCheck :size="18" />
                <strong>策略违规</strong>
                <span class="muted">{{ policyFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="policyFindings.length === 0" class="panel-empty muted">
              所有工作负载均通过声明式策略基线
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in policyFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'policy')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                        <template v-if="item.details?.container"> · 容器 {{ item.details.container }}</template>
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

      <!-- ----------------------------------------------- HPA 扩缩容 -->
      <section v-else-if="activeTab === 'hpa'" class="optimization-tab">
        <div v-if="loading.hpa" class="panel-empty">正在采集 HorizontalPodAutoscaler 并检查扩缩容目标、上限余量与利用率…</div>
        <div v-else-if="errors.hpa" class="panel-empty error">{{ errors.hpa }}</div>
        <template v-else-if="hpa">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Gauge :size="16" />评估 HPA</p>
              <strong>{{ hpa.hpas_total }}</strong>
              <span>HorizontalPodAutoscaler 数量</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />触顶 HPA</p>
              <strong>{{ hpa.at_max_replicas_count }}</strong>
              <span>当前副本数已达 maxReplicas</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><TrendingUp :size="16" />超目标 HPA</p>
              <strong>{{ hpa.over_target_count }}</strong>
              <span>当前利用率高于扩缩容目标</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />预警数</p>
              <strong>{{ hpa.failed }}</strong>
              <span>严重 {{ hpa.by_severity.critical ?? 0 }} · 警告 {{ hpa.by_severity.warning ?? 0 }} · 提示 {{ hpa.by_severity.info ?? 0 }} · 分析时间 {{ formatTimestamp(hpa.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图只读检查每个 HPA 的扩缩容目标是否显式声明、当前副本是否已触顶、
            maxReplicas 是否留有空间，并在 API 提供利用率时对比目标（未声明目标时按 Kubernetes
            默认 80% 评估）。不修改任何 HPA、不触发任何扩缩容；配合容量预测（M70）判断触顶是真缺资源还是配置问题。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <Gauge :size="18" />
                <strong>HPA 扩缩容发现</strong>
                <span class="muted">{{ hpaFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="hpaFindings.length === 0" class="panel-empty muted">
              所有 HPA 均声明了扩缩容目标且有充足上限余量
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in hpaFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'hpa')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                        <template v-if="item.details?.max_replicas"> · maxReplicas {{ item.details.max_replicas }}</template>
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

      <!-- ----------------------------------------------- PDB 保护 -->
      <section v-else-if="activeTab === 'pdb'" class="optimization-tab">
        <div v-if="loading.pdb" class="panel-empty">正在采集工作负载与 PodDisruptionBudget 并检查保护覆盖与预算可达性…</div>
        <div v-else-if="errors.pdb" class="panel-empty error">{{ errors.pdb }}</div>
        <template v-else-if="pdb">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Container :size="16" />评估工作负载</p>
              <strong>{{ pdb.workloads_total }}</strong>
              <span>Deployment / StatefulSet / DaemonSet</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><ShieldAlert :size="16" />未保护工作负载</p>
              <strong>{{ pdb.unprotected_workloads }}</strong>
              <span>无匹配 PDB 的多副本工作负载</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><ShieldCheck :size="16" />PDB 数量</p>
              <strong>{{ pdb.pdbs_total }}</strong>
              <span>评估的 PodDisruptionBudget</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />预警数</p>
              <strong>{{ pdb.failed }}</strong>
              <span>严重 {{ pdb.by_severity.critical ?? 0 }} · 警告 {{ pdb.by_severity.warning ?? 0 }} · 提示 {{ pdb.by_severity.info ?? 0 }} · 分析时间 {{ formatTimestamp(pdb.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图只读检查可多副本工作负载是否有 PodDisruptionBudget 保护、预算是否可达
            （minAvailable 不小于期望副本数会阻塞一切驱逐）、当前是否允许驱逐（disruptionsAllowed=0
            会阻塞节点排空），以及 selector 是否实际匹配到 Pod。不触发任何驱逐、不修改任何对象；
            是维护窗口与节点排空前的只读预检。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <ShieldAlert :size="18" />
                <strong>PDB 保护发现</strong>
                <span class="muted">{{ pdbFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="pdbFindings.length === 0" class="panel-empty muted">
              所有多副本工作负载均有可达且未被阻塞的 PDB 保护
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in pdbFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'pdb')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                        <template v-if="item.details?.replicas"> · {{ item.details.replicas }} 副本</template>
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

      <!-- ----------------------------------------------- Ingress 暴露面 -->
      <section v-else class="optimization-tab">
        <div v-if="loading.ingress" class="panel-empty">正在采集 Ingress 并检查 TLS 覆盖、通配符 host、ingress class 与后端 Service 可达性…</div>
        <div v-else-if="errors.ingress" class="panel-empty error">{{ errors.ingress }}</div>
        <template v-else-if="ingress">
          <div class="summary-grid">
            <article class="metric-card">
              <p class="metric-heading"><Globe :size="16" />评估 Ingress</p>
              <strong>{{ ingress.ingresses_total }}</strong>
              <span>Ingress 总数</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />明文暴露</p>
              <strong>{{ ingress.no_tls_count }}</strong>
              <span>含 host 规则但无 TLS</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><AlertTriangle :size="16" />死后端</p>
              <strong>{{ ingress.dead_backend_count }}</strong>
              <span>引用了不存在的 Service</span>
            </article>
            <article class="metric-card">
              <p class="metric-heading"><CheckCircle2 :size="16" />预警数</p>
              <strong>{{ ingress.failed }}</strong>
              <span>严重 {{ ingress.by_severity.critical ?? 0 }} · 警告 {{ ingress.by_severity.warning ?? 0 }} · 提示 {{ ingress.by_severity.info ?? 0 }} · 分析时间 {{ formatTimestamp(ingress.evaluated_at) }}</span>
            </article>
          </div>

          <p class="view-intro muted">
            本视图只读审计集群对外暴露面：检查每个 Ingress 是否终止 TLS、host
            是否使用通配符扩大暴露、是否显式钉住 ingress class，以及后端 Service
            是否真实存在。不修改任何 Ingress/Service、不触发任何发布；是上线前的只读暴露面体检。
          </p>

          <section class="panel">
            <header class="panel-header">
              <div class="panel-title">
                <Globe :size="18" />
                <strong>Ingress 暴露发现</strong>
                <span class="muted">{{ ingressFindings.length }} 条</span>
              </div>
            </header>
            <div v-if="ingressFindings.length === 0" class="panel-empty muted">
              所有 Ingress 均终止 TLS、无通配符 host、钉住 ingress class 且后端可达
            </div>
            <div v-else class="table-scroll">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>规则编号</th>
                    <th>说明</th>
                    <th>资源</th>
                    <th>等级</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, index) in ingressFindings" :key="`${item.code}-${item.resource.namespace ?? ''}-${item.resource.name}-${index}`">
                    <td><code>{{ item.code }}</code></td>
                    <td>
                      <div class="cell-main">{{ item.summary }}</div>
                      <div v-if="item.details?.remediation" class="cell-sub muted">{{ item.details.remediation }}</div>
                      <FindingEvidencePanel :finding="findingDetail(item, 'ingress')" compact />
                    </td>
                    <td>
                      <div class="cell-main">{{ item.resource.name }}</div>
                      <div class="cell-sub muted">
                        {{ item.resource.kind }}<template v-if="item.resource.namespace"> · {{ item.resource.namespace }}</template>
                        <template v-if="item.details?.host"> · {{ item.details.host }}</template>
                        <template v-if="item.details?.backend"> → {{ item.details.backend }}</template>
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
  width: 100%;
  min-width: 0;
  margin-top: 14px;
  padding: 4px;
  overflow-x: auto;
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.optimization-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  flex: 0 0 auto;
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

.optimization-tab,
.optimization-tab .panel,
.table-scroll {
  min-width: 0;
  max-width: 100%;
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
