<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, Boxes, ChevronRight, GitBranch, Network, RefreshCw, Route, Search, Server, TriangleAlert, X } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { listDeployments, listEndpointSlices, listEvents, listIngresses, listNamespaces, listPods, listServices } from '../api/kubernetes'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { Deployment, EndpointSliceResource, IngressResource, KubernetesEvent, Namespace, Pod, ServiceResource } from '../types/kubernetes'
import { deploymentHealth, podHealth } from '../utils/resource-health'
import { connectedTopologyKeys, endpointSliceHealth, endpointSliceSelectsService, ingressServiceBackends, topologyResourceKey } from '../utils/resource-topology'
import type { TopologyResourceKind, TopologySelection } from '../utils/resource-topology'

const auth = useAuthStore()
const router = useRouter()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const namespaces = ref<Namespace[]>([])
const namespace = ref('')
const searchText = ref('')
const ingresses = ref<IngressResource[]>([])
const services = ref<ServiceResource[]>([])
const endpointSlices = ref<EndpointSliceResource[]>([])
const pods = ref<Pod[]>([])
const deployments = ref<Deployment[]>([])
const events = ref<KubernetesEvent[]>([])
const selected = ref<TopologySelection | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const lastSyncedAt = ref<Date | null>(null)
let loadSequence = 0

const query = computed(() => searchText.value.trim().toLowerCase())
const filteredIngresses = computed(() => filterByName(ingresses.value))
const filteredServices = computed(() => filterByName(services.value))
const filteredEndpointSlices = computed(() => filterByName(endpointSlices.value))
const filteredPods = computed(() => filterByName(pods.value))
const filteredDeployments = computed(() => filterByName(deployments.value))
const warningCount = computed(() => events.value.filter((item) => item.type === 'Warning').length)
const resources = computed(() => ({
  ingresses: ingresses.value,
  services: services.value,
  endpointSlices: endpointSlices.value,
  pods: pods.value,
  deployments: deployments.value,
}))
const activeKeys = computed(() => connectedTopologyKeys(selected.value, resources.value))
const selectedDetail = computed(() => {
  if (!selected.value) return null
  return {
    ...selected.value,
    ingressCount: countActive('Ingress', ingresses.value),
    serviceCount: countActive('Service', services.value),
    endpointSliceCount: countActive('EndpointSlice', endpointSlices.value),
    podCount: countActive('Pod', pods.value),
    deploymentCount: countActive('Deployment', deployments.value),
  }
})
const canOpenSelectedDetail = computed(() => selectedDetail.value?.kind !== 'EndpointSlice')
const lastSyncedLabel = computed(() => lastSyncedAt.value ? new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(lastSyncedAt.value) : '--')

function filterByName<T extends { metadata: { name: string } }>(items: T[]): T[] {
  return items.filter((item) => item.metadata.name.toLowerCase().includes(query.value))
}

function countActive(kind: TopologyResourceKind, items: Array<{ metadata: { namespace?: string; name: string } }>): number {
  return items.filter((item) => activeKeys.value.has(topologyResourceKey(kind, item))).length
}

function isSelected(kind: TopologyResourceKind, item: { metadata: { namespace?: string; name: string } }): boolean {
  return selected.value?.kind === kind && selected.value.name === item.metadata.name && selected.value.namespace === (item.metadata.namespace ?? '')
}

function isActive(kind: TopologyResourceKind, item: { metadata: { namespace?: string; name: string } }): boolean {
  return activeKeys.value.has(topologyResourceKey(kind, item))
}

function selectResource(kind: TopologyResourceKind, item: { metadata: { namespace?: string; name: string } }) {
  if (isSelected(kind, item)) {
    selected.value = null
    return
  }
  selected.value = { kind, namespace: item.metadata.namespace ?? '', name: item.metadata.name }
}

function selectorLabel(selector?: Record<string, string>): string {
  const entries = Object.entries(selector ?? {})
  return entries.length ? entries.map(([key, value]) => `${key}=${value}`).join(', ') : '无 selector'
}

function ingressBackendLabel(ingress: IngressResource): string {
  const references = ingressServiceBackends(ingress)
  return references.length ? [...new Set(references.map((item) => `${item.serviceName}:${item.port}`))].join(', ') : '无 Service 后端'
}

function serviceDiscoveryLabel(service: ServiceResource): string {
  const count = endpointSlices.value.filter((item) => endpointSliceSelectsService(item, service)).length
  return count > 0 ? `${count} 个 EndpointSlice` : `selector 回退 · ${selectorLabel(service.spec.selector)}`
}

function endpointPortLabel(endpointSlice: EndpointSliceResource): string {
  return endpointSlice.ports?.map((item) => `${item.name || item.port || '--'}/${item.protocol || 'TCP'}`).join(', ') || '未声明端口'
}

function readyEndpointCount(endpointSlice: EndpointSliceResource): number {
  return (endpointSlice.endpoints ?? []).filter((item) => item.conditions?.ready !== false && item.conditions?.serving !== false && item.conditions?.terminating !== true).length
}

function endpointCount(endpointSlice: EndpointSliceResource): number {
  return endpointSlice.endpoints?.length ?? 0
}

function openSelectedDetail() {
  if (!selectedDetail.value || !selectedClusterID.value || !canOpenSelectedDetail.value) return
  void router.push({ path: '/workloads', query: { cluster: String(selectedClusterID.value), kind: selectedDetail.value.kind, namespace: selectedDetail.value.namespace, name: selectedDetail.value.name } })
}

async function initialize() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
    await changeCluster()
  } catch {
    errorMessage.value = '无法加载集群列表'
  }
}

async function changeCluster() {
  namespace.value = ''
  selected.value = null
  const clusterID = selectedClusterID.value
  if (!clusterID) return
  try {
    namespaces.value = (await listNamespaces(auth.accessToken, clusterID)).items
  } catch {
    namespaces.value = []
  }
  await loadTopology()
}

async function loadTopology() {
  const clusterID = selectedClusterID.value
  if (!clusterID) return
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const [ingressResponse, serviceResponse, endpointSliceResponse, podResponse, deploymentResponse, eventResponse] = await Promise.all([
      listIngresses(auth.accessToken, clusterID, namespace.value),
      listServices(auth.accessToken, clusterID, namespace.value),
      listEndpointSlices(auth.accessToken, clusterID, namespace.value),
      listPods(auth.accessToken, clusterID, namespace.value),
      listDeployments(auth.accessToken, clusterID, namespace.value),
      listEvents(auth.accessToken, clusterID, namespace.value),
    ])
    if (sequence !== loadSequence) return
    ingresses.value = ingressResponse.items
    services.value = serviceResponse.items
    endpointSlices.value = endpointSliceResponse.items
    pods.value = podResponse.items
    deployments.value = deploymentResponse.items
    events.value = eventResponse.items
    selected.value = null
    lastSyncedAt.value = new Date()
  } catch {
    if (sequence === loadSequence) errorMessage.value = '拓扑数据读取失败，请检查集群连接和只读权限'
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="资源观察" title="资源拓扑">
    <template #actions>
      <span class="sync-time">同步于 {{ lastSyncedLabel }}</span>
      <button class="icon-button" type="button" title="刷新拓扑" aria-label="刷新拓扑" :disabled="loading || !selectedClusterID" @click="loadTopology">
        <RefreshCw :size="18" :class="{ spinning: loading }" />
      </button>
    </template>

    <section class="topology-toolbar" aria-label="拓扑筛选">
      <select v-model="selectedClusterID" aria-label="拓扑集群" @change="changeCluster">
        <option :value="null" disabled>选择已启用集群</option>
        <option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option>
      </select>
      <select v-model="namespace" aria-label="拓扑 Namespace" @change="loadTopology">
        <option value="">全部 Namespace</option>
        <option v-for="item in namespaces" :key="item.metadata.name" :value="item.metadata.name">{{ item.metadata.name }}</option>
      </select>
      <label class="search-field"><Search :size="15" /><input v-model="searchText" placeholder="筛选资源名称" /></label>
      <div class="topology-legend"><span><i class="healthy" />健康</span><span><i class="warning" />注意</span><span><i class="critical" />异常</span></div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <div v-if="clusters.length === 0" class="resource-empty"><Network :size="30" /><strong>没有已启用的集群</strong><span>接入集群后可观察入口到工作负载的完整关系。</span></div>
    <template v-else>
      <section class="topology-summary-grid" aria-label="拓扑资源摘要">
        <article><span>Ingress</span><strong>{{ ingresses.length }}</strong><small>外部路由</small></article>
        <article><span>Service</span><strong>{{ services.length }}</strong><small>服务入口</small></article>
        <article><span>EndpointSlice</span><strong>{{ endpointSlices.length }}</strong><small>后端发现</small></article>
        <article><span>Pod</span><strong>{{ pods.length }}</strong><small>运行实例</small></article>
        <article><span>Deployment</span><strong>{{ deployments.length }}</strong><small>工作负载控制器</small></article>
        <article :class="{ alert: warningCount > 0 }"><span>Warning Event</span><strong>{{ warningCount }}</strong><small>当前筛选范围</small></article>
      </section>

      <section class="topology-workspace" :class="{ loading }">
        <div class="topology-canvas" tabindex="0">
          <header class="topology-canvas-heading">
            <div><p class="context-label">TRAFFIC &amp; WORKLOAD TOPOLOGY</p><h2>Ingress → Service → EndpointSlice → Pod ← Deployment</h2></div>
            <span>选择节点查看同 Namespace 的真实关联链路</span>
          </header>

          <div class="topology-columns">
            <section class="topology-column ingress-column">
              <header><Route :size="16" /><strong>Ingress</strong><span>{{ filteredIngresses.length }}</span></header>
              <div class="topology-node-list">
                <button v-for="item in filteredIngresses" :key="topologyResourceKey('Ingress', item)" type="button" class="topology-node" :class="{ selected: isSelected('Ingress', item), related: isActive('Ingress', item) }" :aria-pressed="isSelected('Ingress', item)" @click="selectResource('Ingress', item)">
                  <span class="resource-glyph ingress"><Route :size="16" /></span><span><strong>{{ item.metadata.name }}</strong><small>{{ item.metadata.namespace }} · {{ ingressServiceBackends(item).length }} 条后端</small><em>{{ ingressBackendLabel(item) }}</em></span><ChevronRight :size="15" />
                </button>
                <div v-if="filteredIngresses.length === 0" class="topology-empty">没有 Ingress</div>
              </div>
            </section>

            <div class="topology-flow" aria-hidden="true"><span /><ChevronRight :size="18" /></div>

            <section class="topology-column network-column">
              <header><Network :size="16" /><strong>Network</strong><span>{{ filteredServices.length + filteredEndpointSlices.length }}</span></header>
              <div class="topology-stack">
                <section class="topology-stack-group">
                  <header><strong>Service</strong><span>{{ filteredServices.length }}</span></header>
                  <div class="topology-node-list compact">
                    <button v-for="item in filteredServices" :key="topologyResourceKey('Service', item)" type="button" class="topology-node" :class="{ selected: isSelected('Service', item), related: isActive('Service', item) }" :aria-pressed="isSelected('Service', item)" @click="selectResource('Service', item)">
                      <span class="resource-glyph service"><Network :size="16" /></span><span><strong>{{ item.metadata.name }}</strong><small>{{ item.metadata.namespace }} · {{ item.spec.type }}</small><em>{{ serviceDiscoveryLabel(item) }}</em></span><ChevronRight :size="15" />
                    </button>
                    <div v-if="filteredServices.length === 0" class="topology-empty compact">没有 Service</div>
                  </div>
                </section>
                <div class="topology-stack-link" aria-hidden="true"><span /><GitBranch :size="14" /><span /></div>
                <section class="topology-stack-group">
                  <header><strong>EndpointSlice</strong><span>{{ filteredEndpointSlices.length }}</span></header>
                  <div class="topology-node-list compact">
                    <button v-for="item in filteredEndpointSlices" :key="topologyResourceKey('EndpointSlice', item)" type="button" class="topology-node" :class="[endpointSliceHealth(item), { selected: isSelected('EndpointSlice', item), related: isActive('EndpointSlice', item) }]" :aria-pressed="isSelected('EndpointSlice', item)" @click="selectResource('EndpointSlice', item)">
                      <span class="resource-glyph endpoint-slice"><GitBranch :size="16" /></span><span><strong>{{ item.metadata.name }}</strong><small>{{ item.metadata.namespace }} · {{ readyEndpointCount(item) }}/{{ endpointCount(item) }} Ready</small><em>{{ item.serviceName || '未关联 Service' }} · {{ endpointPortLabel(item) }}</em></span><ChevronRight :size="15" />
                    </button>
                    <div v-if="filteredEndpointSlices.length === 0" class="topology-empty compact">没有 EndpointSlice</div>
                  </div>
                </section>
              </div>
            </section>

            <div class="topology-flow" aria-hidden="true"><span /><ChevronRight :size="18" /></div>

            <section class="topology-column pods">
              <header><Boxes :size="16" /><strong>Runtime</strong><span>{{ filteredPods.length }}</span></header>
              <div class="topology-node-list">
                <button v-for="item in filteredPods" :key="topologyResourceKey('Pod', item)" type="button" class="topology-node" :class="[podHealth(item), { selected: isSelected('Pod', item), related: isActive('Pod', item) }]" :aria-pressed="isSelected('Pod', item)" @click="selectResource('Pod', item)">
                  <span class="resource-glyph pod"><Boxes :size="16" /></span><span><strong>{{ item.metadata.name }}</strong><small>{{ item.metadata.namespace }} · {{ item.status.phase }}</small><em>{{ item.status.podIP || 'Pod IP pending' }}</em></span><ChevronRight :size="15" />
                </button>
                <div v-if="filteredPods.length === 0" class="topology-empty">没有 Pod</div>
              </div>
            </section>

            <div class="topology-flow reverse" aria-hidden="true"><ChevronRight :size="18" /><span /></div>

            <section class="topology-column deployment-column">
              <header><Server :size="16" /><strong>Workloads</strong><span>{{ filteredDeployments.length }}</span></header>
              <div class="topology-node-list">
                <button v-for="item in filteredDeployments" :key="topologyResourceKey('Deployment', item)" type="button" class="topology-node" :class="[deploymentHealth(item), { selected: isSelected('Deployment', item), related: isActive('Deployment', item) }]" :aria-pressed="isSelected('Deployment', item)" @click="selectResource('Deployment', item)">
                  <span class="resource-glyph deployment"><Server :size="16" /></span><span><strong>{{ item.metadata.name }}</strong><small>{{ item.metadata.namespace }} · {{ item.status.readyReplicas }}/{{ item.spec.replicas ?? 1 }} Ready</small><em>{{ selectorLabel(item.spec.selector.matchLabels) }}</em></span><ChevronRight :size="15" />
                </button>
                <div v-if="filteredDeployments.length === 0" class="topology-empty">没有 Deployment</div>
              </div>
            </section>
          </div>
        </div>

        <aside class="topology-inspector">
          <div v-if="!selectedDetail" class="topology-inspector-empty"><Activity :size="26" /><strong>选择资源查看完整链路</strong><span>关系来自 Ingress 后端、EndpointSlice targetRef 与完整标签匹配。</span></div>
          <template v-else>
            <div class="topology-inspector-heading"><p class="context-label">RESOURCE INSPECTOR</p><button class="icon-button inspector-clear" type="button" title="清除选择" aria-label="清除选择" @click="selected = null"><X :size="15" /></button></div>
            <span class="inspector-kind">{{ selectedDetail.kind }}</span>
            <h2>{{ selectedDetail.name }}</h2>
            <small>{{ selectedDetail.namespace || 'cluster-scoped' }}</small>
            <dl>
              <div><dt>关联 Ingress</dt><dd>{{ selectedDetail.ingressCount }}</dd></div>
              <div><dt>关联 Service</dt><dd>{{ selectedDetail.serviceCount }}</dd></div>
              <div><dt>关联 EndpointSlice</dt><dd>{{ selectedDetail.endpointSliceCount }}</dd></div>
              <div><dt>关联 Pod</dt><dd>{{ selectedDetail.podCount }}</dd></div>
              <div><dt>关联 Deployment</dt><dd>{{ selectedDetail.deploymentCount }}</dd></div>
            </dl>
            <button v-if="canOpenSelectedDetail" class="panel-link inspector-detail-link" type="button" @click="openSelectedDetail">查看资源详情<ChevronRight :size="15" /></button>
            <p class="inspector-note"><TriangleAlert :size="14" />{{ selectedDetail.kind === 'EndpointSlice' ? 'EndpointSlice 当前仅用于只读拓扑检查。' : '空 selector、跨 Namespace 和未知 targetRef 不会生成关系。' }}</p>
          </template>
        </aside>
      </section>
    </template>
  </ConsoleLayout>
</template>
