<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Boxes, Check, ChevronRight, Clock3, Database, FileText, Gauge, Globe2, HardDrive, KeyRound, Layers, ListChecks, Network, Pause, Play, RefreshCw, Search, Server, Settings, ShieldCheck, SlidersHorizontal, Sparkles, X } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { diagnoseDeployment, diagnoseHorizontalPodAutoscaler, diagnoseIngress, diagnoseNode, diagnosePersistentVolumeClaim, diagnosePod, diagnoseService, executeRemediation, listControlledOperations, previewControlledOperation } from '../api/diagnosis'
import {
  getConfigMap,
  getCronJob,
  getDaemonSet,
  getDeployment,
  getHorizontalPodAutoscaler,
  getIngress,
  getJob,
  getLimitRange,
  getNode,
  getPersistentVolumeClaim,
  getPod,
  getPodLogs,
  getReplicaSet,
  getResourceQuota,
  getSecret,
  getService,
  getStatefulSet,
  getStorageClass,
  listConfigMaps,
  listCronJobs,
  listDaemonSets,
  listDeployments,
  listEvents,
  listHorizontalPodAutoscalers,
  listIngresses,
  listJobs,
  listLimitRanges,
  listNamespaces,
  listNodes,
  listPersistentVolumeClaims,
  listPods,
  listReplicaSets,
  listResourceQuotas,
  listSecrets,
  listServices,
  listStatefulSets,
  listStorageClasses,
} from '../api/kubernetes'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { ControlledOperationRequest, DiagnosisRecord, RemediationPlan } from '../types/diagnosis'
import type {
  ConfigMapResource,
  CronJobResource,
  DaemonSetResource,
  Deployment,
  HorizontalPodAutoscalerResource,
  IngressBackend,
  IngressResource,
  JobResource,
  KubernetesEvent,
  LimitRangeResource,
  Namespace,
  NodeResource,
  PersistentVolumeClaim,
  Pod,
  ReplicaSetResource,
  ResourceQuotaResource,
  SecretResource,
  ServiceResource,
  StatefulSetResource,
  StorageClassResource,
} from '../types/kubernetes'
import { eventTimestamp, relatedResourceEvents } from '../utils/kubernetes-events'
import type { EventResourceKind } from '../utils/kubernetes-events'

type ResourceKind = EventResourceKind
type M17ResourceKind = 'StatefulSet' | 'DaemonSet' | 'ReplicaSet' | 'Job' | 'CronJob' | 'HPA' | 'ResourceQuota' | 'LimitRange' | 'Secret'
type M17Resource = StatefulSetResource | DaemonSetResource | ReplicaSetResource | JobResource | CronJobResource | HorizontalPodAutoscalerResource | ResourceQuotaResource | LimitRangeResource | SecretResource
type ResourceCategory = 'workloads' | 'network' | 'storage' | 'configuration'
type DiagnosableKind = 'Pod' | 'Deployment' | 'Node' | 'Service' | 'Ingress' | 'PVC' | 'HPA'
type ResourceDetail = Pod | Deployment | NodeResource | ServiceResource | IngressResource | PersistentVolumeClaim | StorageClassResource | ConfigMapResource | M17Resource
interface DetailSelection { kind: ResourceKind; namespace: string; name: string }
interface M17Row { item: M17Resource; status: string; statusTone: string; primary: string; secondary: string; detail: string }
interface DetailValue { label: string; value: string }

const resourceCategories: Array<{ id: ResourceCategory; label: string; kinds: ResourceKind[] }> = [
  { id: 'workloads', label: '工作负载', kinds: ['Pod', 'Deployment', 'StatefulSet', 'DaemonSet', 'ReplicaSet', 'Job', 'CronJob', 'Node'] },
  { id: 'network', label: '网络', kinds: ['Service', 'Ingress'] },
  { id: 'storage', label: '存储', kinds: ['PVC', 'StorageClass'] },
  { id: 'configuration', label: '弹性与配置', kinds: ['HPA', 'ResourceQuota', 'LimitRange', 'ConfigMap', 'Secret'] },
]
const resourceKinds = resourceCategories.flatMap((category) => category.kinds)
const clusterScopedKinds = new Set<ResourceKind>(['Node', 'StorageClass'])
const kindIcons = {
  Pod: Boxes,
  Deployment: Server,
  StatefulSet: Database,
  DaemonSet: ShieldCheck,
  ReplicaSet: Layers,
  Job: ListChecks,
  CronJob: Clock3,
  Node: HardDrive,
  Service: Network,
  Ingress: Globe2,
  PVC: Database,
  StorageClass: Layers,
  HPA: Gauge,
  ResourceQuota: ShieldCheck,
  LimitRange: Settings,
  ConfigMap: FileText,
  Secret: KeyRound,
}
const categoryIcons = { workloads: Boxes, network: Network, storage: Database, configuration: Settings }
const diagnosticPodReasons = ['ImagePullBackOff', 'ErrImagePull', 'CrashLoopBackOff', 'Pending', 'OOMKilled']
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const namespaces = ref<Namespace[]>([])
const namespace = ref('')
const nameFilter = ref('')
const selectedKind = ref<ResourceKind>('Pod')
const pods = ref<Pod[]>([])
const nodes = ref<NodeResource[]>([])
const deployments = ref<Deployment[]>([])
const statefulSets = ref<StatefulSetResource[]>([])
const daemonSets = ref<DaemonSetResource[]>([])
const replicaSets = ref<ReplicaSetResource[]>([])
const jobs = ref<JobResource[]>([])
const cronJobs = ref<CronJobResource[]>([])
const services = ref<ServiceResource[]>([])
const ingresses = ref<IngressResource[]>([])
const persistentVolumeClaims = ref<PersistentVolumeClaim[]>([])
const storageClasses = ref<StorageClassResource[]>([])
const configMaps = ref<ConfigMapResource[]>([])
const horizontalPodAutoscalers = ref<HorizontalPodAutoscalerResource[]>([])
const resourceQuotas = ref<ResourceQuotaResource[]>([])
const limitRanges = ref<LimitRangeResource[]>([])
const secrets = ref<SecretResource[]>([])
const loading = ref(false)
const initializing = ref(true)
const errorMessage = ref('')
const detailSelection = ref<DetailSelection | null>(null)
const detail = ref<ResourceDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const relatedEvents = ref<KubernetesEvent[]>([])
const eventsLoading = ref(false)
const eventsError = ref('')
const logTitle = ref('')
const logs = ref('')
const logLoading = ref(false)
const diagnosis = ref<DiagnosisRecord | null>(null)
const diagnosisLoading = ref(false)
const operationHistory = ref<RemediationPlan[]>([])
const operationPlan = ref<RemediationPlan | null>(null)
const operationToken = ref('')
const operationIdempotencyKey = ref('')
const operationConfirmed = ref(false)
const operationBusy = ref(false)
const operationError = ref('')
const scaleReplicas = ref(1)
let initialized = false
let resourceSequence = 0
let detailSequence = 0

const searchQuery = computed(() => nameFilter.value.trim().toLowerCase())
const filteredPods = computed(() => filterByName(pods.value))
const filteredNodes = computed(() => filterByName(nodes.value))
const filteredDeployments = computed(() => filterByName(deployments.value))
const filteredServices = computed(() => filterByName(services.value))
const filteredIngresses = computed(() => filterByName(ingresses.value))
const filteredPersistentVolumeClaims = computed(() => filterByName(persistentVolumeClaims.value))
const filteredStorageClasses = computed(() => filterByName(storageClasses.value))
const filteredConfigMaps = computed(() => filterByName(configMaps.value))
const filteredM17Rows = computed(() => m17Items(selectedKind.value).filter((item) => item.metadata.name.toLowerCase().includes(searchQuery.value)).map((item) => m17Row(selectedKind.value as M17ResourceKind, item)))
const selectedCount = computed(() => resourceCount(selectedKind.value))
const matchedCount = computed(() => filteredCount(selectedKind.value))
const searchPlaceholder = computed(() => `按 ${kindLabel(selectedKind.value)} 名称筛选`)
const selectedCategory = computed(() => resourceCategories.find((category) => category.kinds.includes(selectedKind.value)) ?? resourceCategories[0])
const categoryKinds = computed(() => selectedCategory.value.kinds)
const podDetail = computed(() => detailSelection.value?.kind === 'Pod' ? detail.value as Pod | null : null)
const deploymentDetail = computed(() => detailSelection.value?.kind === 'Deployment' ? detail.value as Deployment | null : null)
const cronJobDetail = computed(() => detailSelection.value?.kind === 'CronJob' ? detail.value as CronJobResource | null : null)
const nodeDetail = computed(() => detailSelection.value?.kind === 'Node' ? detail.value as NodeResource | null : null)
const serviceDetail = computed(() => detailSelection.value?.kind === 'Service' ? detail.value as ServiceResource | null : null)
const ingressDetail = computed(() => detailSelection.value?.kind === 'Ingress' ? detail.value as IngressResource | null : null)
const persistentVolumeClaimDetail = computed(() => detailSelection.value?.kind === 'PVC' ? detail.value as PersistentVolumeClaim | null : null)
const storageClassDetail = computed(() => detailSelection.value?.kind === 'StorageClass' ? detail.value as StorageClassResource | null : null)
const configMapDetail = computed(() => detailSelection.value?.kind === 'ConfigMap' ? detail.value as ConfigMapResource | null : null)
const m17Detail = computed(() => detailSelection.value && isM17Kind(detailSelection.value.kind) ? detail.value as M17Resource | null : null)
const secretDetail = computed(() => detailSelection.value?.kind === 'Secret' ? detail.value as SecretResource | null : null)
const canOperate = computed(() => auth.user?.roles.some((role) => role === 'system_admin' || role === 'operations_admin') ?? false)

function filterByName<T extends { metadata: { name: string } }>(items: T[]): T[] {
  return items.filter((item) => item.metadata.name.toLowerCase().includes(searchQuery.value))
}

function isM17Kind(kind: ResourceKind): kind is M17ResourceKind {
  return ['StatefulSet', 'DaemonSet', 'ReplicaSet', 'Job', 'CronJob', 'HPA', 'ResourceQuota', 'LimitRange', 'Secret'].includes(kind)
}

function m17Items(kind: ResourceKind): M17Resource[] {
  if (kind === 'StatefulSet') return statefulSets.value
  if (kind === 'DaemonSet') return daemonSets.value
  if (kind === 'ReplicaSet') return replicaSets.value
  if (kind === 'Job') return jobs.value
  if (kind === 'CronJob') return cronJobs.value
  if (kind === 'HPA') return horizontalPodAutoscalers.value
  if (kind === 'ResourceQuota') return resourceQuotas.value
  if (kind === 'LimitRange') return limitRanges.value
  if (kind === 'Secret') return secrets.value
  return []
}

function queryValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? '')
  return typeof value === 'string' ? value : ''
}

function parseKind(value: unknown): ResourceKind | null {
  const candidate = queryValue(value)
  return resourceKinds.includes(candidate as ResourceKind) ? candidate as ResourceKind : null
}

function routeClusterID(): number | null {
  const value = Number(queryValue(route.query.cluster))
  return Number.isInteger(value) && value > 0 ? value : null
}

function isClusterScoped(kind: ResourceKind): boolean {
  return clusterScopedKinds.has(kind)
}

function selectionFromRoute(): DetailSelection | null {
  const kind = parseKind(route.query.kind)
  const name = queryValue(route.query.name)
  const resourceNamespace = queryValue(route.query.namespace)
  if (!kind || !name || (!isClusterScoped(kind) && !resourceNamespace)) return null
  return { kind, namespace: isClusterScoped(kind) ? '' : resourceNamespace, name }
}

function kindLabel(kind: ResourceKind): string {
  return kind === 'StorageClass' ? 'StorageClass' : kind
}

function resourceCount(kind: ResourceKind): number {
  if (kind === 'Pod') return pods.value.length
  if (kind === 'Deployment') return deployments.value.length
  if (kind === 'Node') return nodes.value.length
  if (kind === 'Service') return services.value.length
  if (kind === 'Ingress') return ingresses.value.length
  if (kind === 'PVC') return persistentVolumeClaims.value.length
  if (kind === 'StorageClass') return storageClasses.value.length
  if (isM17Kind(kind)) return m17Items(kind).length
  return configMaps.value.length
}

function filteredCount(kind: ResourceKind): number {
  if (kind === 'Pod') return filteredPods.value.length
  if (kind === 'Deployment') return filteredDeployments.value.length
  if (kind === 'Node') return filteredNodes.value.length
  if (kind === 'Service') return filteredServices.value.length
  if (kind === 'Ingress') return filteredIngresses.value.length
  if (kind === 'PVC') return filteredPersistentVolumeClaims.value.length
  if (kind === 'StorageClass') return filteredStorageClasses.value.length
  if (isM17Kind(kind)) return m17Items(kind).filter((item) => item.metadata.name.toLowerCase().includes(searchQuery.value)).length
  return filteredConfigMaps.value.length
}

function categoryCount(category: { kinds: ResourceKind[] }): number {
  return category.kinds.reduce((total, kind) => total + resourceCount(kind), 0)
}

function podReason(pod: Pod): string {
  for (const container of pod.status.containerStatuses ?? []) {
    if (container.state.waiting?.reason && diagnosticPodReasons.includes(container.state.waiting.reason)) return container.state.waiting.reason
  }
  if (pod.status.phase === 'Pending') return pod.status.reason || 'Pending'
  for (const container of pod.status.containerStatuses ?? []) if (container.state.waiting?.reason) return container.state.waiting.reason
  for (const container of pod.status.containerStatuses ?? []) if (container.state.terminated?.reason) return container.state.terminated.reason
  return pod.status.reason || pod.status.phase
}

function restartCount(pod: Pod): number {
  return (pod.status.containerStatuses ?? []).reduce((sum, item) => sum + item.restartCount, 0)
}

function readyContainerCount(pod: Pod): number {
  return (pod.status.containerStatuses ?? []).filter((item) => item.ready).length
}

function nodeReady(node: NodeResource): string {
  return node.status.conditions.find((condition) => condition.type === 'Ready')?.status || 'Unknown'
}

function nodeAddress(node: NodeResource, type = 'InternalIP'): string {
  return node.status.addresses.find((item) => item.type === type)?.address || '--'
}

function nodeDiagnosable(node: NodeResource): boolean {
  const ready = node.status.conditions.find((condition) => condition.type === 'Ready')
  if (ready?.status !== 'True') return true
  return node.status.conditions.some((condition) => ['MemoryPressure', 'DiskPressure', 'PIDPressure'].includes(condition.type) && condition.status === 'True')
}

function hpaSaturated(item: HorizontalPodAutoscalerResource): boolean {
  const atMaximum = item.spec.maxReplicas > 0 && (item.status.currentReplicas >= item.spec.maxReplicas || item.status.desiredReplicas >= item.spec.maxReplicas)
  return atMaximum && item.status.conditions.some((condition) => condition.type === 'ScalingLimited' && condition.status === 'True' && condition.reason === 'TooManyReplicas')
}

function m17HPASaturated(item: M17Resource): boolean {
  return hpaSaturated(item as HorizontalPodAutoscalerResource)
}

function imageSummary(deployment: Deployment): string {
  return deployment.spec.template.spec.containers.map((item) => item.image).join(', ') || '--'
}

function servicePortSummary(service: ServiceResource): string {
  return service.spec.ports.map((item) => `${item.port}/${item.protocol}`).join(', ') || '--'
}

function ingressHosts(ingress: IngressResource): string {
  return [...new Set((ingress.spec.rules ?? []).map((rule) => rule.host || '*'))].join(', ') || '*'
}

function ingressRouteCount(ingress: IngressResource): number {
  return (ingress.spec.rules ?? []).reduce((total, rule) => total + (rule.http?.paths.length ?? 0), 0) + (ingress.spec.defaultBackend ? 1 : 0)
}

function ingressBackends(ingress: IngressResource): Array<{ host: string; path: string; pathType: string; backend: IngressBackend }> {
  const result = (ingress.spec.rules ?? []).flatMap((rule) => (rule.http?.paths ?? []).map((path) => ({ host: rule.host || '*', path: path.path || '/', pathType: path.pathType || '--', backend: path.backend })))
  if (ingress.spec.defaultBackend) result.unshift({ host: '*', path: 'default', pathType: '--', backend: ingress.spec.defaultBackend })
  return result
}

function backendPort(backend: IngressBackend): string {
  return backend.service?.port.name || String(backend.service?.port.number || '--')
}

function ingressAddresses(ingress: IngressResource): string {
  return (ingress.status.loadBalancer.ingress ?? []).map((item) => item.ip || item.hostname).filter(Boolean).join(', ') || '--'
}

function storageClassName(item: PersistentVolumeClaim): string {
  return item.spec.storageClassName || '--'
}

function isDefaultStorageClass(item: StorageClassResource): boolean {
  return item.metadata.annotations?.['storageclass.kubernetes.io/is-default-class'] === 'true' || item.metadata.annotations?.['storageclass.beta.kubernetes.io/is-default-class'] === 'true'
}

function sortedEntries(values?: Record<string, string>): Array<[string, string]> {
  return Object.entries(values ?? {}).sort(([left], [right]) => left.localeCompare(right))
}

function desiredReplicas(value?: number): number {
  return value ?? 1
}

function templateImages(template: { spec: { containers: Array<{ image: string }> } }): string {
  return template.spec.containers.map((item) => item.image).join(', ') || '--'
}

function quantitySummary(values?: Record<string, string>, limit = 3): string {
  const entries = sortedEntries(values)
  if (entries.length === 0) return '--'
  const visible = entries.slice(0, limit).map(([key, value]) => `${key} ${value}`).join(' · ')
  return entries.length > limit ? `${visible} · +${entries.length - limit}` : visible
}

function metricTargetSummary(target?: { type: string; value?: string; averageValue?: string; averageUtilization?: number }): string {
  if (!target) return '--'
  if (target.averageUtilization !== undefined) return `${target.averageUtilization}% utilization`
  return target.averageValue || target.value || target.type || '--'
}

function hpaMetricSummary(item: HorizontalPodAutoscalerResource): string {
  return item.spec.metrics.map((metric) => {
    if (metric.resource) return `${metric.resource.name} ${metricTargetSummary(metric.resource.target)}`
    if (metric.containerResource) return `${metric.containerResource.container}/${metric.containerResource.name} ${metricTargetSummary(metric.containerResource.target)}`
    if (metric.pods) return `${metric.pods.metric.name} ${metricTargetSummary(metric.pods.target)}`
    if (metric.object) return `${metric.object.metric.name} ${metricTargetSummary(metric.object.target)}`
    if (metric.external) return `${metric.external.metric.name} ${metricTargetSummary(metric.external.target)}`
    return metric.type
  }).join(', ') || '--'
}

function jobStatus(item: JobResource): string {
  if (item.spec.suspend) return 'Suspended'
  if (item.status.failed > 0) return 'Failed'
  if (item.status.succeeded >= desiredReplicas(item.spec.completions)) return 'Succeeded'
  if (item.status.active > 0) return 'Running'
  return 'Pending'
}

function m17Row(kind: M17ResourceKind, resource: M17Resource): M17Row {
  if (kind === 'StatefulSet') {
    const item = resource as StatefulSetResource
    const desired = desiredReplicas(item.spec.replicas)
    return { item, status: `${item.status.readyReplicas}/${desired} Ready`, statusTone: item.status.readyReplicas >= desired ? 'running' : 'pending', primary: item.spec.serviceName || '--', secondary: `${item.status.updatedReplicas} updated`, detail: templateImages(item.spec.template) }
  }
  if (kind === 'DaemonSet') {
    const item = resource as DaemonSetResource
    return { item, status: `${item.status.numberReady}/${item.status.desiredNumberScheduled} Ready`, statusTone: item.status.numberUnavailable > 0 ? 'pending' : 'running', primary: `${item.status.numberAvailable} available`, secondary: `${item.status.updatedNumberScheduled} updated`, detail: templateImages(item.spec.template) }
  }
  if (kind === 'ReplicaSet') {
    const item = resource as ReplicaSetResource
    const desired = desiredReplicas(item.spec.replicas)
    return { item, status: `${item.status.readyReplicas}/${desired} Ready`, statusTone: item.status.readyReplicas >= desired ? 'running' : 'pending', primary: `${item.status.availableReplicas} available`, secondary: `${item.status.fullyLabeledReplicas} labeled`, detail: templateImages(item.spec.template) }
  }
  if (kind === 'Job') {
    const item = resource as JobResource
    const status = jobStatus(item)
    return { item, status, statusTone: status === 'Failed' ? 'failed' : status === 'Succeeded' ? 'running' : 'pending', primary: `${item.status.succeeded}/${desiredReplicas(item.spec.completions)} succeeded`, secondary: `${item.status.active} active · ${item.status.failed} failed`, detail: `backoff ${item.spec.backoffLimit ?? 6}` }
  }
  if (kind === 'CronJob') {
    const item = resource as CronJobResource
    return { item, status: item.spec.suspend ? 'Suspended' : 'Active', statusTone: item.spec.suspend ? 'pending' : 'running', primary: item.spec.schedule, secondary: item.spec.timeZone || 'controller timezone', detail: item.status.lastScheduleTime ? `last ${formatTime(item.status.lastScheduleTime)}` : 'not scheduled' }
  }
  if (kind === 'HPA') {
    const item = resource as HorizontalPodAutoscalerResource
    return { item, status: `${item.status.currentReplicas} → ${item.status.desiredReplicas}`, statusTone: item.status.conditions.some((condition) => condition.type === 'AbleToScale' && condition.status === 'False') ? 'failed' : 'running', primary: `${item.spec.scaleTargetRef.kind}/${item.spec.scaleTargetRef.name}`, secondary: `${item.spec.minReplicas ?? 1}-${item.spec.maxReplicas} replicas`, detail: hpaMetricSummary(item) }
  }
  if (kind === 'ResourceQuota') {
    const item = resource as ResourceQuotaResource
    return { item, status: `${Object.keys(item.status.hard ?? item.spec.hard ?? {}).length} resources`, statusTone: 'running', primary: quantitySummary(item.status.used, 2), secondary: quantitySummary(item.status.hard ?? item.spec.hard, 2), detail: 'used / hard' }
  }
  if (kind === 'LimitRange') {
    const item = resource as LimitRangeResource
    return { item, status: `${item.spec.limits.length} policies`, statusTone: 'running', primary: item.spec.limits.map((limit) => limit.type).join(', ') || '--', secondary: quantitySummary(item.spec.limits[0]?.defaultRequest, 2), detail: quantitySummary(item.spec.limits[0]?.max, 2) }
  }
  const item = resource as SecretResource
  return { item, status: `${item.dataKeys.length} keys`, statusTone: 'running', primary: item.type || 'Opaque', secondary: item.immutable ? 'Immutable' : 'Mutable', detail: item.dataKeys.join(', ') || 'no data keys' }
}

function m17DetailStats(kind: M17ResourceKind, resource: M17Resource): DetailValue[] {
  const row = m17Row(kind, resource)
  return [
    { label: 'Status', value: row.status },
    { label: kind === 'CronJob' ? 'Schedule' : kind === 'HPA' ? 'Target' : 'Primary', value: row.primary },
    { label: 'Scope', value: row.secondary },
    { label: 'Created', value: formatTime(resource.metadata.creationTimestamp) },
  ]
}

function m17DetailValues(kind: M17ResourceKind, resource: M17Resource): DetailValue[] {
  if (kind === 'StatefulSet') {
    const item = resource as StatefulSetResource
    return [{ label: 'Service', value: item.spec.serviceName }, { label: 'Pod management', value: item.spec.podManagementPolicy || 'OrderedReady' }, { label: 'Update strategy', value: item.spec.updateStrategy.type || 'RollingUpdate' }, { label: 'Images', value: templateImages(item.spec.template) }]
  }
  if (kind === 'DaemonSet') {
    const item = resource as DaemonSetResource
    return [{ label: 'Desired', value: String(item.status.desiredNumberScheduled) }, { label: 'Available', value: String(item.status.numberAvailable) }, { label: 'Unavailable', value: String(item.status.numberUnavailable) }, { label: 'Images', value: templateImages(item.spec.template) }]
  }
  if (kind === 'ReplicaSet') {
    const item = resource as ReplicaSetResource
    return [{ label: 'Desired', value: String(desiredReplicas(item.spec.replicas)) }, { label: 'Available', value: String(item.status.availableReplicas) }, { label: 'Fully labeled', value: String(item.status.fullyLabeledReplicas) }, { label: 'Images', value: templateImages(item.spec.template) }]
  }
  if (kind === 'Job') {
    const item = resource as JobResource
    return [{ label: 'Parallelism', value: String(item.spec.parallelism ?? 1) }, { label: 'Completions', value: String(item.spec.completions ?? 1) }, { label: 'Backoff limit', value: String(item.spec.backoffLimit ?? 6) }, { label: 'Images', value: templateImages(item.spec.template) }]
  }
  if (kind === 'CronJob') {
    const item = resource as CronJobResource
    return [{ label: 'Time zone', value: item.spec.timeZone || 'controller timezone' }, { label: 'Concurrency', value: item.spec.concurrencyPolicy || 'Allow' }, { label: 'Last schedule', value: formatTime(item.status.lastScheduleTime) }, { label: 'Last success', value: formatTime(item.status.lastSuccessfulTime) }]
  }
  if (kind === 'HPA') {
    const item = resource as HorizontalPodAutoscalerResource
    return [{ label: 'API target', value: item.spec.scaleTargetRef.apiVersion }, { label: 'Replica range', value: `${item.spec.minReplicas ?? 1} - ${item.spec.maxReplicas}` }, { label: 'Metrics', value: hpaMetricSummary(item) }, { label: 'Last scale', value: formatTime(item.status.lastScaleTime) }]
  }
  if (kind === 'ResourceQuota') {
    const item = resource as ResourceQuotaResource
    return sortedEntries(item.status.hard ?? item.spec.hard).map(([key, hard]) => ({ label: key, value: `${item.status.used?.[key] ?? '0'} / ${hard}` }))
  }
  if (kind === 'LimitRange') {
    const item = resource as LimitRangeResource
    return item.spec.limits.flatMap((limit, index) => [
      { label: `${limit.type} ${index + 1} · min`, value: quantitySummary(limit.min) },
      { label: `${limit.type} ${index + 1} · max`, value: quantitySummary(limit.max) },
      { label: `${limit.type} ${index + 1} · default`, value: quantitySummary(limit.default) },
      { label: `${limit.type} ${index + 1} · request`, value: quantitySummary(limit.defaultRequest) },
    ])
  }
  if (kind === 'Secret') {
    const item = resource as SecretResource
    return [{ label: 'Type', value: item.type || 'Opaque' }, { label: 'Immutable', value: item.immutable ? 'Yes' : 'No' }, { label: 'Data keys', value: String(item.dataKeys.length) }, { label: 'UID', value: item.metadata.uid || '--' }]
  }
  return []
}

function m17Conditions(kind: M17ResourceKind, resource: M17Resource): Array<{ type: string; status: string; reason?: string; message?: string }> {
  if (kind === 'Job') return (resource as JobResource).status.conditions ?? []
  if (kind === 'HPA') return (resource as HorizontalPodAutoscalerResource).status.conditions ?? []
  return []
}

function formatTime(value?: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

function eventCount(event: KubernetesEvent): number {
  return event.series?.count || event.count || 1
}

function statusClass(value: string): string {
  return value.toLowerCase().replace(/[^a-z]+/g, '-')
}

async function initialize() {
  initializing.value = true
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((item) => item.enabled)
    const requestedCluster = routeClusterID()
    selectedClusterID.value = clusters.value.some((item) => item.id === requestedCluster) ? requestedCluster : clusters.value[0]?.id ?? null
    selectedKind.value = parseKind(route.query.kind) ?? 'Pod'
    namespace.value = isClusterScoped(selectedKind.value) ? '' : queryValue(route.query.namespace)
    initialized = true
    await loadResources()
    await loadDetailFromRoute()
  } catch {
    errorMessage.value = '无法加载资源工作台，请稍后重试'
  } finally {
    initializing.value = false
  }
}

async function loadResources() {
  const clusterID = selectedClusterID.value
  const sequence = ++resourceSequence
  if (!clusterID) {
    namespaces.value = []; pods.value = []; nodes.value = []; deployments.value = []; services.value = []
    ingresses.value = []; persistentVolumeClaims.value = []; storageClasses.value = []; configMaps.value = []
    statefulSets.value = []; daemonSets.value = []; replicaSets.value = []; jobs.value = []; cronJobs.value = []
    horizontalPodAutoscalers.value = []; resourceQuotas.value = []; limitRanges.value = []; secrets.value = []
    return
  }
  loading.value = true
  errorMessage.value = ''
  const results = await Promise.allSettled([
    listNamespaces(auth.accessToken, clusterID),
    listPods(auth.accessToken, clusterID, namespace.value),
    listNodes(auth.accessToken, clusterID),
    listDeployments(auth.accessToken, clusterID, namespace.value),
    listStatefulSets(auth.accessToken, clusterID, namespace.value),
    listDaemonSets(auth.accessToken, clusterID, namespace.value),
    listReplicaSets(auth.accessToken, clusterID, namespace.value),
    listJobs(auth.accessToken, clusterID, namespace.value),
    listCronJobs(auth.accessToken, clusterID, namespace.value),
    listServices(auth.accessToken, clusterID, namespace.value),
    listIngresses(auth.accessToken, clusterID, namespace.value),
    listPersistentVolumeClaims(auth.accessToken, clusterID, namespace.value),
    listStorageClasses(auth.accessToken, clusterID),
    listConfigMaps(auth.accessToken, clusterID, namespace.value),
    listHorizontalPodAutoscalers(auth.accessToken, clusterID, namespace.value),
    listResourceQuotas(auth.accessToken, clusterID, namespace.value),
    listLimitRanges(auth.accessToken, clusterID, namespace.value),
    listSecrets(auth.accessToken, clusterID, namespace.value),
  ])
  if (sequence !== resourceSequence) return
  const assign = <T>(index: number, target: { value: T[] }) => {
    const result = results[index]
    target.value = result?.status === 'fulfilled' ? result.value.items as T[] : []
  }
  assign(0, namespaces); assign(1, pods); assign(2, nodes); assign(3, deployments); assign(4, statefulSets)
  assign(5, daemonSets); assign(6, replicaSets); assign(7, jobs); assign(8, cronJobs); assign(9, services)
  assign(10, ingresses); assign(11, persistentVolumeClaims); assign(12, storageClasses); assign(13, configMaps)
  assign(14, horizontalPodAutoscalers); assign(15, resourceQuotas); assign(16, limitRanges); assign(17, secrets)
  const failed = results.filter((result) => result.status === 'rejected').length
  if (failed > 0) errorMessage.value = `${failed} 类资源读取失败，请确认目标集群观察权限和 API 可用性`
  loading.value = false
}

function getSelectedDetail(selection: DetailSelection, clusterID: number): Promise<ResourceDetail> {
  if (selection.kind === 'Pod') return getPod(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'Deployment') return getDeployment(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'StatefulSet') return getStatefulSet(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'DaemonSet') return getDaemonSet(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'ReplicaSet') return getReplicaSet(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'Job') return getJob(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'CronJob') return getCronJob(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'Node') return getNode(auth.accessToken, clusterID, selection.name)
  if (selection.kind === 'Service') return getService(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'Ingress') return getIngress(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'PVC') return getPersistentVolumeClaim(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'StorageClass') return getStorageClass(auth.accessToken, clusterID, selection.name)
  if (selection.kind === 'HPA') return getHorizontalPodAutoscaler(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'ResourceQuota') return getResourceQuota(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'LimitRange') return getLimitRange(auth.accessToken, clusterID, selection.namespace, selection.name)
  if (selection.kind === 'Secret') return getSecret(auth.accessToken, clusterID, selection.namespace, selection.name)
  return getConfigMap(auth.accessToken, clusterID, selection.namespace, selection.name)
}

function controlledOperationKind(selection: DetailSelection | null): 'Deployment' | 'CronJob' | null {
  return selection?.kind === 'Deployment' || selection?.kind === 'CronJob' ? selection.kind : null
}

function clearOperationPreview() {
  operationPlan.value = null
  operationToken.value = ''
  operationIdempotencyKey.value = ''
  operationConfirmed.value = false
  operationError.value = ''
}

async function loadDetailFromRoute() {
  const selection = selectionFromRoute()
  const clusterID = selectedClusterID.value
  const sequence = ++detailSequence
  detailSelection.value = selection
  detail.value = null
  detailError.value = ''
  relatedEvents.value = []
  eventsError.value = ''
  operationHistory.value = []
  clearOperationPreview()
  if (!selection || !clusterID) return
  detailLoading.value = true
  eventsLoading.value = true
  const targetKind = controlledOperationKind(selection)
  const operationRequest = targetKind
    ? listControlledOperations(auth.accessToken, clusterID, selection.namespace, targetKind, selection.name)
    : Promise.resolve({ items: [] as RemediationPlan[], total: 0, remaining: 0 })
  const [detailResult, eventResult, operationResult] = await Promise.allSettled([
    getSelectedDetail(selection, clusterID),
    listEvents(auth.accessToken, clusterID, selection.namespace, selection.name),
    operationRequest,
  ])
  if (sequence !== detailSequence) return
  if (detailResult.status === 'fulfilled') {
    detail.value = detailResult.value
    if (selection.kind === 'Deployment') scaleReplicas.value = (detailResult.value as Deployment).spec.replicas ?? 1
  }
  else detailError.value = '资源详情读取失败。资源可能已被删除，或观察账号缺少读取权限。'
  if (eventResult.status === 'fulfilled') {
    relatedEvents.value = relatedResourceEvents(eventResult.value.items, {
      kind: selection.kind,
      namespace: selection.namespace,
      name: selection.name,
      clusterScoped: isClusterScoped(selection.kind),
    })
  } else {
    eventsError.value = '关联事件暂时不可用'
  }
  if (operationResult.status === 'fulfilled') operationHistory.value = operationResult.value.items
  detailLoading.value = false
  eventsLoading.value = false
}

async function syncFromRoute() {
  if (!initialized) return
  const requestedCluster = routeClusterID()
  if (requestedCluster && requestedCluster !== selectedClusterID.value && clusters.value.some((item) => item.id === requestedCluster)) {
    selectedClusterID.value = requestedCluster
    namespace.value = parseKind(route.query.kind) && isClusterScoped(parseKind(route.query.kind) as ResourceKind) ? '' : queryValue(route.query.namespace)
    await loadResources()
  }
  const kind = parseKind(route.query.kind)
  if (kind) selectedKind.value = kind
  await loadDetailFromRoute()
}

function baseRouteQuery(kind = selectedKind.value): Record<string, string> {
  if (!selectedClusterID.value) return {}
  const query: Record<string, string> = { cluster: String(selectedClusterID.value), kind }
  if (namespace.value && !isClusterScoped(kind)) query.namespace = namespace.value
  return query
}

async function changeCluster() {
  namespace.value = ''
  nameFilter.value = ''
  detailSelection.value = null
  detail.value = null
  await router.replace({ path: '/workloads', query: baseRouteQuery() })
  await loadResources()
}

async function changeNamespace() {
  detailSelection.value = null
  detail.value = null
  await router.replace({ path: '/workloads', query: baseRouteQuery() })
  await loadResources()
}

async function selectCategory(category: { kinds: ResourceKind[] }) {
  await selectKind(category.kinds[0] as ResourceKind)
}

async function selectKind(kind: ResourceKind) {
  selectedKind.value = kind
  nameFilter.value = ''
  await router.replace({ path: '/workloads', query: baseRouteQuery(kind) })
}

async function openResource(kind: ResourceKind, resourceNamespace: string | undefined, name: string) {
  selectedKind.value = kind
  const query: Record<string, string> = { cluster: String(selectedClusterID.value), kind, name }
  if (!isClusterScoped(kind) && resourceNamespace) query.namespace = resourceNamespace
  await router.push({ path: '/workloads', query })
}

async function closeDetail() {
  detailSelection.value = null
  detail.value = null
  relatedEvents.value = []
  operationHistory.value = []
  clearOperationPreview()
  await router.push({ path: '/workloads', query: baseRouteQuery() })
}

async function showLogs(pod: Pod, previous = false) {
  if (!selectedClusterID.value || !pod.metadata.namespace) return
  logTitle.value = `${pod.metadata.namespace}/${pod.metadata.name}${previous ? ' · previous' : ''}`
  logs.value = ''
  logLoading.value = true
  try {
    logs.value = (await getPodLogs(auth.accessToken, selectedClusterID.value, pod.metadata.namespace, pod.metadata.name, pod.spec.containers[0]?.name, previous)).logs || '日志为空'
  } catch {
    logs.value = '日志读取失败。容器可能尚未启动，或集群账号缺少 pods/log 权限。'
  } finally {
    logLoading.value = false
  }
}

async function runDiagnosis(kind: DiagnosableKind, item: ResourceDetail) {
  if (!selectedClusterID.value) return
  diagnosis.value = null
  diagnosisLoading.value = true
  errorMessage.value = ''
  try {
    if (kind === 'Pod') {
      const pod = item as Pod
      if (!pod.metadata.namespace) return
      diagnosis.value = await diagnosePod(auth.accessToken, selectedClusterID.value, pod.metadata.namespace, pod.metadata.name)
    }
    if (kind === 'Deployment') {
      const deployment = item as Deployment
      if (!deployment.metadata.namespace) return
      diagnosis.value = await diagnoseDeployment(auth.accessToken, selectedClusterID.value, deployment.metadata.namespace, deployment.metadata.name)
    }
    if (kind === 'Service') {
      const service = item as ServiceResource
      if (!service.metadata.namespace) return
      diagnosis.value = await diagnoseService(auth.accessToken, selectedClusterID.value, service.metadata.namespace, service.metadata.name)
    }
    if (kind === 'Ingress') {
      const ingress = item as IngressResource
      if (!ingress.metadata.namespace) return
      diagnosis.value = await diagnoseIngress(auth.accessToken, selectedClusterID.value, ingress.metadata.namespace, ingress.metadata.name)
    }
    if (kind === 'PVC') {
      const claim = item as PersistentVolumeClaim
      if (!claim.metadata.namespace) return
      diagnosis.value = await diagnosePersistentVolumeClaim(auth.accessToken, selectedClusterID.value, claim.metadata.namespace, claim.metadata.name)
    }
    if (kind === 'HPA') {
      const hpa = item as HorizontalPodAutoscalerResource
      if (!hpa.metadata.namespace) return
      diagnosis.value = await diagnoseHorizontalPodAutoscaler(auth.accessToken, selectedClusterID.value, hpa.metadata.namespace, hpa.metadata.name)
    }
    if (kind === 'Node') diagnosis.value = await diagnoseNode(auth.accessToken, selectedClusterID.value, (item as NodeResource).metadata.name)
  } catch {
    errorMessage.value = `${kind} 未命中已启用规则，或证据采集失败`
  } finally {
    diagnosisLoading.value = false
  }
}

async function createOperationPreview(request: ControlledOperationRequest) {
  if (!selectedClusterID.value || operationBusy.value) return
  operationBusy.value = true
  operationError.value = ''
  try {
    const plan = await previewControlledOperation(auth.accessToken, selectedClusterID.value, request)
    operationPlan.value = plan
    operationToken.value = plan.confirmation_token ?? ''
    operationIdempotencyKey.value = `operation-${crypto.randomUUID()}`
    operationConfirmed.value = false
    operationHistory.value = [plan, ...operationHistory.value.filter((item) => item.id !== plan.id)]
  } catch (error) {
    operationError.value = error instanceof Error ? error.message : '无法生成受控操作计划'
  } finally {
    operationBusy.value = false
  }
}

function previewDeploymentScale() {
  const target = deploymentDetail.value
  if (!target?.metadata.namespace || !Number.isInteger(scaleReplicas.value) || scaleReplicas.value < 0 || scaleReplicas.value > 1000) {
    operationError.value = '副本数必须是 0 到 1000 之间的整数'
    return
  }
  void createOperationPreview({ action: 'deployment.scale', namespace: target.metadata.namespace, target_name: target.metadata.name, desired_replicas: scaleReplicas.value })
}

function previewCronJobOperation(action: 'cronjob.suspend' | 'cronjob.resume') {
  const target = cronJobDetail.value
  if (!target?.metadata.namespace) return
  void createOperationPreview({ action, namespace: target.metadata.namespace, target_name: target.metadata.name })
}

async function confirmOperation() {
  if (!operationPlan.value || !operationToken.value || !operationConfirmed.value || operationBusy.value) return
  operationBusy.value = true
  operationError.value = ''
  try {
    const result = await executeRemediation(auth.accessToken, operationPlan.value.id, operationToken.value, operationIdempotencyKey.value)
    operationHistory.value = operationHistory.value.map((item) => item.id === result.id ? result : item)
    operationPlan.value = null
    operationToken.value = ''
    operationConfirmed.value = false
    await Promise.all([loadResources(), loadDetailFromRoute()])
  } catch (error) {
    operationError.value = error instanceof Error ? error.message : '受控操作执行失败'
  } finally {
    operationBusy.value = false
  }
}

function operationActionLabel(action: RemediationPlan['action']): string {
  if (action === 'deployment.scale') return '调整副本数'
  if (action === 'cronjob.suspend') return '暂停调度'
  if (action === 'cronjob.resume') return '恢复调度'
  return '滚动重启'
}

function operationStatusLabel(status: RemediationPlan['status']): string {
  return { awaiting_confirmation: '待确认', executing: '执行中', succeeded: '已完成', failed: '失败', expired: '已过期' }[status]
}

function operationValue(value: number | boolean | undefined): string {
  if (typeof value === 'boolean') return value ? '暂停' : '运行'
  return value === undefined ? '--' : String(value)
}

function onEscape(event: KeyboardEvent) {
	if (event.key !== 'Escape') return
	if (operationPlan.value && !operationBusy.value) clearOperationPreview()
	else if (diagnosis.value) diagnosis.value = null
  else if (logTitle.value) logTitle.value = ''
  else if (detailSelection.value) void closeDetail()
}

watch(() => [route.query.cluster, route.query.kind, route.query.namespace, route.query.name], syncFromRoute)
onMounted(() => { window.addEventListener('keydown', onEscape); void initialize() })
onBeforeUnmount(() => window.removeEventListener('keydown', onEscape))
</script>

<template>
  <ConsoleLayout eyebrow="集群资源" title="资源工作台">
    <section class="resource-toolbar workload-toolbar">
      <select v-model="selectedClusterID" aria-label="选择集群" @change="changeCluster"><option :value="null" disabled>选择已启用集群</option><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select>
      <select v-model="namespace" aria-label="Namespace" :disabled="isClusterScoped(selectedKind)" @change="changeNamespace"><option value="">全部 Namespace</option><option v-for="item in namespaces" :key="item.metadata.name" :value="item.metadata.name">{{ item.metadata.name }}</option></select>
      <label class="search-field"><Search :size="15" /><input v-model="nameFilter" :placeholder="searchPlaceholder" /></label>
      <button class="icon-button" type="button" title="刷新资源" aria-label="刷新资源" :disabled="loading || !selectedClusterID" @click="loadResources"><RefreshCw :size="17" :class="{ spinning: loading }" /></button>
    </section>

    <nav class="resource-category-tabs" aria-label="资源分类">
      <button v-for="category in resourceCategories" :key="category.id" type="button" :class="{ active: selectedCategory.id === category.id }" @click="selectCategory(category)">
        <component :is="categoryIcons[category.id]" :size="17" />
        <span>{{ category.label }}</span>
        <strong>{{ categoryCount(category) }}</strong>
      </button>
    </nav>

    <nav class="resource-kind-tabs" :class="`count-${categoryKinds.length}`" aria-label="资源类型">
      <button v-for="kind in categoryKinds" :key="kind" type="button" :class="{ active: selectedKind === kind }" :aria-current="selectedKind === kind ? 'page' : undefined" @click="selectKind(kind)">
        <component :is="kindIcons[kind]" :size="16" />
        <span>{{ kindLabel(kind) }}</span><strong>{{ resourceCount(kind) }}</strong>
      </button>
    </nav>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <div v-if="initializing" class="resource-empty"><RefreshCw :size="28" class="spinning" /><strong>正在建立集群资源视图</strong></div>
    <div v-else-if="clusters.length === 0" class="resource-empty"><Boxes :size="30" /><strong>没有已启用的集群</strong><span>请先在“集群”页面导入、探测并启用集群。</span></div>

    <section v-else class="resource-panel resource-workbench" :class="{ loading }">
      <div class="section-heading workload-heading">
        <div><p class="context-label">{{ selectedKind.toUpperCase() }} INVENTORY</p><h2>{{ kindLabel(selectedKind) }} 资源</h2></div>
        <span>{{ selectedCount }} total<span v-if="searchQuery"> · {{ matchedCount }} matched</span></span>
      </div>

      <div class="pod-table-wrap resource-table-wrap">
        <table v-if="selectedKind === 'Pod'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>状态</th><th>Ready</th><th>重启</th><th>节点</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="pod in filteredPods" :key="pod.metadata.uid || `${pod.metadata.namespace}/${pod.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('Pod', pod.metadata.namespace, pod.metadata.name)" @keydown.enter="openResource('Pod', pod.metadata.namespace, pod.metadata.name)">
              <td><strong>{{ pod.metadata.name }}</strong><span>{{ pod.spec.containers.map((item) => item.image).join(', ') }}</span></td><td>{{ pod.metadata.namespace }}</td><td><span class="resource-status" :class="statusClass(podReason(pod))">{{ podReason(pod) }}</span></td><td>{{ readyContainerCount(pod) }}/{{ pod.spec.containers.length }}</td><td>{{ restartCount(pod) }}</td><td>{{ pod.spec.nodeName || '--' }}</td>
              <td><div class="resource-row-actions"><button class="icon-button compact" type="button" title="查看日志" aria-label="查看日志" @click.stop="showLogs(pod)"><FileText :size="15" /></button><button v-if="diagnosticPodReasons.includes(podReason(pod))" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('Pod', pod)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredPods.length === 0"><td colspan="7" class="table-empty">当前范围没有 Pod</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'Deployment'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>副本</th><th>Available</th><th>Updated</th><th>镜像</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredDeployments" :key="item.metadata.uid || `${item.metadata.namespace}/${item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('Deployment', item.metadata.namespace, item.metadata.name)" @keydown.enter="openResource('Deployment', item.metadata.namespace, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ Object.keys(item.spec.selector.matchLabels ?? {}).join(', ') || 'no selector' }}</span></td><td>{{ item.metadata.namespace }}</td><td>{{ item.status.readyReplicas }}/{{ item.spec.replicas ?? 1 }} Ready</td><td>{{ item.status.availableReplicas }}</td><td>{{ item.status.updatedReplicas }}</td><td class="truncate-cell">{{ imageSummary(item) }}</td>
              <td><div class="resource-row-actions"><button v-if="item.status.unavailableReplicas > 0" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('Deployment', item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredDeployments.length === 0"><td colspan="7" class="table-empty">当前范围没有 Deployment</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'Node'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>状态</th><th>调度</th><th>Internal IP</th><th>Kubelet</th><th>运行时</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredNodes" :key="item.metadata.uid || item.metadata.name" class="resource-row" tabindex="0" @click="openResource('Node', undefined, item.metadata.name)" @keydown.enter="openResource('Node', undefined, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ item.status.nodeInfo.osImage || '--' }}</span></td><td><span class="resource-status" :class="nodeReady(item) === 'True' ? 'running' : 'failed'">{{ nodeReady(item) === 'True' ? 'Ready' : nodeReady(item) }}</span></td><td>{{ item.spec.unschedulable ? '已停止' : '可调度' }}</td><td>{{ nodeAddress(item) }}</td><td>{{ item.status.nodeInfo.kubeletVersion || '--' }}</td><td class="truncate-cell">{{ item.status.nodeInfo.containerRuntimeVersion || '--' }}</td>
              <td><div class="resource-row-actions"><button v-if="nodeDiagnosable(item)" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('Node', item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredNodes.length === 0"><td colspan="7" class="table-empty">当前集群没有 Node</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'Service'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>类型</th><th>Cluster IP</th><th>端口</th><th>Selector</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredServices" :key="item.metadata.uid || `${item.metadata.namespace}/${item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('Service', item.metadata.namespace, item.metadata.name)" @keydown.enter="openResource('Service', item.metadata.namespace, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ item.spec.externalName || 'cluster service' }}</span></td><td>{{ item.metadata.namespace }}</td><td>{{ item.spec.type }}</td><td>{{ item.spec.clusterIP || '--' }}</td><td>{{ servicePortSummary(item) }}</td><td class="truncate-cell">{{ sortedEntries(item.spec.selector).map(([key, value]) => `${key}=${value}`).join(', ') || '--' }}</td>
              <td><div class="resource-row-actions"><button v-if="!item.spec.externalName && Object.keys(item.spec.selector ?? {}).length > 0" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('Service', item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredServices.length === 0"><td colspan="7" class="table-empty">当前范围没有 Service</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'Ingress'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>Class</th><th>Hosts</th><th>路由</th><th>地址</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredIngresses" :key="item.metadata.uid || `${item.metadata.namespace}/${item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('Ingress', item.metadata.namespace, item.metadata.name)" @keydown.enter="openResource('Ingress', item.metadata.namespace, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ formatTime(item.metadata.creationTimestamp) }}</span></td><td>{{ item.metadata.namespace }}</td><td>{{ item.spec.ingressClassName || '--' }}</td><td class="truncate-cell">{{ ingressHosts(item) }}</td><td>{{ ingressRouteCount(item) }}</td><td class="truncate-cell">{{ ingressAddresses(item) }}</td><td><div class="resource-row-actions"><button v-if="ingressBackends(item).some((routeItem) => routeItem.backend.service?.name)" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('Ingress', item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredIngresses.length === 0"><td colspan="7" class="table-empty">当前范围没有 Ingress</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'PVC'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>状态</th><th>StorageClass</th><th>申请</th><th>容量</th><th>访问模式</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredPersistentVolumeClaims" :key="item.metadata.uid || `${item.metadata.namespace}/${item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('PVC', item.metadata.namespace, item.metadata.name)" @keydown.enter="openResource('PVC', item.metadata.namespace, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ item.spec.volumeName || '等待绑定' }}</span></td><td>{{ item.metadata.namespace }}</td><td><span class="resource-status" :class="statusClass(item.status.phase)">{{ item.status.phase || 'Unknown' }}</span></td><td>{{ storageClassName(item) }}</td><td>{{ item.spec.resources.requests?.storage || '--' }}</td><td>{{ item.status.capacity?.storage || '--' }}</td><td>{{ (item.status.accessModes ?? item.spec.accessModes ?? []).join(', ') || '--' }}</td><td><div class="resource-row-actions"><button v-if="item.status.phase === 'Pending'" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('PVC', item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredPersistentVolumeClaims.length === 0"><td colspan="8" class="table-empty">当前范围没有 PVC</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'StorageClass'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>默认</th><th>Provisioner</th><th>回收策略</th><th>绑定模式</th><th>扩容</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredStorageClasses" :key="item.metadata.uid || item.metadata.name" class="resource-row" tabindex="0" @click="openResource('StorageClass', undefined, item.metadata.name)" @keydown.enter="openResource('StorageClass', undefined, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ formatTime(item.metadata.creationTimestamp) }}</span></td><td>{{ isDefaultStorageClass(item) ? '是' : '否' }}</td><td class="truncate-cell">{{ item.provisioner }}</td><td>{{ item.reclaimPolicy || '--' }}</td><td>{{ item.volumeBindingMode || '--' }}</td><td>{{ item.allowVolumeExpansion === undefined ? '--' : item.allowVolumeExpansion ? '允许' : '不允许' }}</td><td><div class="resource-row-actions"><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredStorageClasses.length === 0"><td colspan="7" class="table-empty">当前集群没有 StorageClass</td></tr>
          </tbody>
        </table>

        <table v-else-if="selectedKind === 'ConfigMap'" class="pod-table resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>不可变</th><th>Data keys</th><th>Binary keys</th><th>创建时间</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in filteredConfigMaps" :key="item.metadata.uid || `${item.metadata.namespace}/${item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource('ConfigMap', item.metadata.namespace, item.metadata.name)" @keydown.enter="openResource('ConfigMap', item.metadata.namespace, item.metadata.name)">
              <td><strong>{{ item.metadata.name }}</strong><span>{{ item.dataKeys.length + item.binaryDataKeys.length }} keys</span></td><td>{{ item.metadata.namespace }}</td><td>{{ item.immutable ? '是' : '否' }}</td><td class="truncate-cell">{{ item.dataKeys.join(', ') || '--' }}</td><td class="truncate-cell">{{ item.binaryDataKeys.join(', ') || '--' }}</td><td>{{ formatTime(item.metadata.creationTimestamp) }}</td><td><div class="resource-row-actions"><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredConfigMaps.length === 0"><td colspan="7" class="table-empty">当前范围没有 ConfigMap</td></tr>
          </tbody>
        </table>

        <table v-else class="pod-table resource-table m17-resource-table">
          <thead><tr><th>名称</th><th>Namespace</th><th>状态</th><th>核心配置</th><th>当前范围</th><th>观测摘要</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="row in filteredM17Rows" :key="row.item.metadata.uid || `${row.item.metadata.namespace}/${row.item.metadata.name}`" class="resource-row" tabindex="0" @click="openResource(selectedKind, row.item.metadata.namespace, row.item.metadata.name)" @keydown.enter="openResource(selectedKind, row.item.metadata.namespace, row.item.metadata.name)">
              <td><strong>{{ row.item.metadata.name }}</strong><span>{{ formatTime(row.item.metadata.creationTimestamp) }}</span></td><td>{{ row.item.metadata.namespace }}</td><td><span class="resource-status" :class="row.statusTone">{{ row.status }}</span></td><td class="truncate-cell">{{ row.primary }}</td><td class="truncate-cell">{{ row.secondary }}</td><td class="truncate-cell">{{ row.detail }}</td><td><div class="resource-row-actions"><button v-if="selectedKind === 'HPA' && m17HPASaturated(row.item)" class="icon-button compact diagnose" type="button" title="运行诊断" aria-label="运行诊断" :disabled="diagnosisLoading" @click.stop="runDiagnosis('HPA', row.item)"><Sparkles :size="15" /></button><ChevronRight :size="16" /></div></td>
            </tr>
            <tr v-if="filteredM17Rows.length === 0"><td colspan="7" class="table-empty">当前范围没有 {{ kindLabel(selectedKind) }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-if="detailSelection" class="log-overlay" @click.self="closeDetail">
      <section class="resource-detail-drawer" role="dialog" aria-modal="true" :aria-label="`${detailSelection.kind} 详情`">
        <header class="resource-detail-header">
          <div class="resource-detail-title"><span class="resource-glyph" :class="detailSelection.kind.toLowerCase()"><component :is="kindIcons[detailSelection.kind]" :size="18" /></span><div><p class="context-label">{{ detailSelection.kind.toUpperCase() }} DETAIL</p><h2>{{ detailSelection.name }}</h2><span>{{ detailSelection.namespace || 'cluster-scoped' }}</span></div></div>
          <button class="icon-button" type="button" title="关闭详情" aria-label="关闭详情" @click="closeDetail"><X :size="18" /></button>
        </header>

        <div v-if="detailLoading" class="detail-loading"><RefreshCw :size="24" class="spinning" /><span>正在读取资源详情</span></div>
        <div v-else-if="detailError" class="detail-error"><strong>详情不可用</strong><span>{{ detailError }}</span><button class="secondary-button" type="button" @click="loadDetailFromRoute"><RefreshCw :size="15" />重试</button></div>

        <template v-else>
          <template v-if="podDetail">
            <div class="drawer-action-bar"><button class="secondary-button" type="button" @click="showLogs(podDetail)"><FileText :size="15" />当前日志</button><button v-if="restartCount(podDetail) > 0" class="secondary-button" type="button" @click="showLogs(podDetail, true)"><FileText :size="15" />Previous</button><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('Pod', podDetail)"><Sparkles :size="15" />运行诊断</button></div>
            <dl class="resource-detail-stats"><div><dt>Phase</dt><dd><span class="resource-status" :class="statusClass(podReason(podDetail))">{{ podReason(podDetail) }}</span></dd></div><div><dt>Ready</dt><dd>{{ readyContainerCount(podDetail) }}/{{ podDetail.spec.containers.length }}</dd></div><div><dt>Restarts</dt><dd>{{ restartCount(podDetail) }}</dd></div><div><dt>Created</dt><dd>{{ formatTime(podDetail.metadata.creationTimestamp) }}</dd></div></dl>
            <section class="detail-section"><h3>运行位置</h3><dl class="detail-definition-list"><div><dt>Node</dt><dd>{{ podDetail.spec.nodeName || '--' }}</dd></div><div><dt>Pod IP</dt><dd>{{ podDetail.status.podIP || '--' }}</dd></div><div><dt>Host IP</dt><dd>{{ podDetail.status.hostIP || '--' }}</dd></div><div><dt>UID</dt><dd>{{ podDetail.metadata.uid || '--' }}</dd></div></dl></section>
            <section class="detail-section"><h3>容器</h3><div class="detail-list"><article v-for="container in podDetail.spec.containers" :key="container.name" class="container-row"><span><strong>{{ container.name }}</strong><small>{{ container.image }}</small></span><em :class="podDetail.status.containerStatuses?.find((item) => item.name === container.name)?.ready ? 'ready' : ''">{{ podDetail.status.containerStatuses?.find((item) => item.name === container.name)?.ready ? 'Ready' : 'Not Ready' }}</em></article></div></section>
            <section class="detail-section"><h3>Conditions</h3><div class="condition-list"><article v-for="condition in podDetail.status.conditions ?? []" :key="condition.type"><span :class="['condition-indicator', condition.status === 'True' ? 'true' : 'false']" /><div><strong>{{ condition.type }} · {{ condition.status }}</strong><small>{{ condition.reason || 'No reason' }} · {{ formatTime(condition.lastTransitionTime) }}</small><p v-if="condition.message">{{ condition.message }}</p></div></article><div v-if="!podDetail.status.conditions?.length" class="detail-empty">没有 Condition 数据</div></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(podDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(podDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="deploymentDetail">
            <div class="drawer-action-bar"><div v-if="canOperate" class="scale-control"><label for="deployment-replicas">副本</label><input id="deployment-replicas" v-model.number="scaleReplicas" type="number" min="0" max="1000" step="1" inputmode="numeric" /><button class="secondary-button" type="button" :disabled="operationBusy || !Number.isInteger(scaleReplicas) || scaleReplicas === (deploymentDetail.spec.replicas ?? 1)" @click="previewDeploymentScale"><SlidersHorizontal :size="15" />预览调整</button></div><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('Deployment', deploymentDetail)"><Sparkles :size="15" />运行诊断</button></div>
            <dl class="resource-detail-stats six"><div><dt>Desired</dt><dd>{{ deploymentDetail.spec.replicas ?? 1 }}</dd></div><div><dt>Current</dt><dd>{{ deploymentDetail.status.replicas }}</dd></div><div><dt>Updated</dt><dd>{{ deploymentDetail.status.updatedReplicas }}</dd></div><div><dt>Ready</dt><dd>{{ deploymentDetail.status.readyReplicas }}</dd></div><div><dt>Available</dt><dd>{{ deploymentDetail.status.availableReplicas }}</dd></div><div><dt>Unavailable</dt><dd :class="{ danger: deploymentDetail.status.unavailableReplicas > 0 }">{{ deploymentDetail.status.unavailableReplicas }}</dd></div></dl>
            <section class="detail-section"><h3>容器镜像</h3><div class="detail-list"><article v-for="container in deploymentDetail.spec.template.spec.containers" :key="container.name" class="container-row"><span><strong>{{ container.name }}</strong><small>{{ container.image }}</small></span></article></div></section>
            <section class="detail-section"><h3>Selector</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(deploymentDetail.spec.selector.matchLabels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(deploymentDetail.spec.selector.matchLabels).length" class="detail-empty">没有 Selector</div></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(deploymentDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(deploymentDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="nodeDetail">
            <div v-if="nodeDiagnosable(nodeDetail)" class="drawer-action-bar"><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('Node', nodeDetail)"><Sparkles :size="15" />检查节点</button></div>
            <dl class="resource-detail-stats"><div><dt>Status</dt><dd><span class="resource-status" :class="nodeReady(nodeDetail) === 'True' ? 'running' : 'failed'">{{ nodeReady(nodeDetail) === 'True' ? 'Ready' : nodeReady(nodeDetail) }}</span></dd></div><div><dt>Scheduling</dt><dd>{{ nodeDetail.spec.unschedulable ? 'Disabled' : 'Enabled' }}</dd></div><div><dt>Internal IP</dt><dd>{{ nodeAddress(nodeDetail) }}</dd></div><div><dt>Kubelet</dt><dd>{{ nodeDetail.status.nodeInfo.kubeletVersion || '--' }}</dd></div></dl>
            <section class="detail-section"><h3>系统</h3><dl class="detail-definition-list"><div><dt>OS image</dt><dd>{{ nodeDetail.status.nodeInfo.osImage || '--' }}</dd></div><div><dt>Container runtime</dt><dd>{{ nodeDetail.status.nodeInfo.containerRuntimeVersion || '--' }}</dd></div><div><dt>Hostname</dt><dd>{{ nodeAddress(nodeDetail, 'Hostname') }}</dd></div><div><dt>UID</dt><dd>{{ nodeDetail.metadata.uid || '--' }}</dd></div></dl></section>
            <section class="detail-section"><h3>资源容量</h3><div class="capacity-grid"><article v-for="([key, value]) in sortedEntries(nodeDetail.status.capacity)" :key="key"><span>{{ key }}</span><strong>{{ value }}</strong><small>allocatable {{ nodeDetail.status.allocatable?.[key] || '--' }}</small></article><div v-if="!sortedEntries(nodeDetail.status.capacity).length" class="detail-empty">没有容量数据</div></div></section>
            <section class="detail-section"><h3>Conditions</h3><div class="condition-list"><article v-for="condition in nodeDetail.status.conditions" :key="condition.type"><span :class="['condition-indicator', condition.type === 'Ready' ? (condition.status === 'True' ? 'true' : 'false') : (condition.status === 'True' ? 'warning' : 'true')]" /><div><strong>{{ condition.type }} · {{ condition.status }}</strong><small>{{ condition.reason || 'No reason' }} · {{ formatTime(condition.lastTransitionTime) }}</small><p v-if="condition.message">{{ condition.message }}</p></div></article></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(nodeDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(nodeDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="serviceDetail">
            <div v-if="!serviceDetail.spec.externalName && Object.keys(serviceDetail.spec.selector ?? {}).length > 0" class="drawer-action-bar"><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('Service', serviceDetail)"><Sparkles :size="15" />检查后端</button></div>
            <dl class="resource-detail-stats"><div><dt>Type</dt><dd>{{ serviceDetail.spec.type }}</dd></div><div><dt>Cluster IP</dt><dd>{{ serviceDetail.spec.clusterIP || '--' }}</dd></div><div><dt>External name</dt><dd>{{ serviceDetail.spec.externalName || '--' }}</dd></div><div><dt>Created</dt><dd>{{ formatTime(serviceDetail.metadata.creationTimestamp) }}</dd></div></dl>
            <section class="detail-section"><h3>端口</h3><div class="detail-table"><div class="detail-table-head"><span>Name</span><span>Port</span><span>Target</span><span>NodePort</span></div><div v-for="port in serviceDetail.spec.ports" :key="`${port.name}-${port.port}`"><span>{{ port.name || '--' }}</span><span>{{ port.port }}/{{ port.protocol }}</span><span>{{ port.targetPort || '--' }}</span><span>{{ port.nodePort || '--' }}</span></div></div></section>
            <section class="detail-section"><h3>Selector</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(serviceDetail.spec.selector)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(serviceDetail.spec.selector).length" class="detail-empty">没有 Selector</div></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(serviceDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(serviceDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="ingressDetail">
            <div v-if="ingressBackends(ingressDetail).some((routeItem) => routeItem.backend.service?.name)" class="drawer-action-bar"><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('Ingress', ingressDetail)"><Sparkles :size="15" />检查后端</button></div>
            <dl class="resource-detail-stats"><div><dt>Class</dt><dd>{{ ingressDetail.spec.ingressClassName || '--' }}</dd></div><div><dt>Hosts</dt><dd>{{ ingressHosts(ingressDetail) }}</dd></div><div><dt>Routes</dt><dd>{{ ingressRouteCount(ingressDetail) }}</dd></div><div><dt>Address</dt><dd>{{ ingressAddresses(ingressDetail) }}</dd></div></dl>
            <section class="detail-section"><h3>路由与后端</h3><div class="detail-table ingress-detail-table"><div class="detail-table-head"><span>Host</span><span>Path</span><span>Type</span><span>Backend</span></div><div v-for="(routeItem, index) in ingressBackends(ingressDetail)" :key="`${routeItem.host}-${routeItem.path}-${index}`"><span>{{ routeItem.host }}</span><span>{{ routeItem.path }}</span><span>{{ routeItem.pathType }}</span><span>{{ routeItem.backend.service?.name || '--' }}:{{ backendPort(routeItem.backend) }}</span></div><div v-if="ingressBackends(ingressDetail).length === 0" class="detail-empty">没有 HTTP 路由</div></div></section>
            <section class="detail-section"><h3>TLS</h3><div class="detail-list"><article v-for="(tls, index) in ingressDetail.spec.tls ?? []" :key="`${tls.secretName}-${index}`" class="container-row"><span><strong>{{ (tls.hosts ?? []).join(', ') || '--' }}</strong><small>Secret name: {{ tls.secretName || '--' }}</small></span></article><div v-if="!ingressDetail.spec.tls?.length" class="detail-empty">没有 TLS 配置</div></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(ingressDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(ingressDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="persistentVolumeClaimDetail">
            <div v-if="persistentVolumeClaimDetail.status.phase === 'Pending'" class="drawer-action-bar"><button class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('PVC', persistentVolumeClaimDetail)"><Sparkles :size="15" />检查绑定</button></div>
            <dl class="resource-detail-stats"><div><dt>Phase</dt><dd><span class="resource-status" :class="statusClass(persistentVolumeClaimDetail.status.phase)">{{ persistentVolumeClaimDetail.status.phase || 'Unknown' }}</span></dd></div><div><dt>Requested</dt><dd>{{ persistentVolumeClaimDetail.spec.resources.requests?.storage || '--' }}</dd></div><div><dt>Capacity</dt><dd>{{ persistentVolumeClaimDetail.status.capacity?.storage || '--' }}</dd></div><div><dt>StorageClass</dt><dd>{{ storageClassName(persistentVolumeClaimDetail) }}</dd></div></dl>
            <section class="detail-section"><h3>卷绑定</h3><dl class="detail-definition-list"><div><dt>Volume</dt><dd>{{ persistentVolumeClaimDetail.spec.volumeName || '--' }}</dd></div><div><dt>Volume mode</dt><dd>{{ persistentVolumeClaimDetail.spec.volumeMode || 'Filesystem' }}</dd></div><div><dt>Access modes</dt><dd>{{ (persistentVolumeClaimDetail.status.accessModes ?? persistentVolumeClaimDetail.spec.accessModes ?? []).join(', ') || '--' }}</dd></div><div><dt>UID</dt><dd>{{ persistentVolumeClaimDetail.metadata.uid || '--' }}</dd></div></dl></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(persistentVolumeClaimDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(persistentVolumeClaimDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="storageClassDetail">
            <dl class="resource-detail-stats"><div><dt>Default</dt><dd>{{ isDefaultStorageClass(storageClassDetail) ? 'Yes' : 'No' }}</dd></div><div><dt>Reclaim</dt><dd>{{ storageClassDetail.reclaimPolicy || '--' }}</dd></div><div><dt>Binding</dt><dd>{{ storageClassDetail.volumeBindingMode || '--' }}</dd></div><div><dt>Expansion</dt><dd>{{ storageClassDetail.allowVolumeExpansion === undefined ? '--' : storageClassDetail.allowVolumeExpansion ? 'Allowed' : 'Disabled' }}</dd></div></dl>
            <section class="detail-section"><h3>供应器</h3><dl class="detail-definition-list"><div><dt>Provisioner</dt><dd>{{ storageClassDetail.provisioner }}</dd></div><div><dt>Created</dt><dd>{{ formatTime(storageClassDetail.metadata.creationTimestamp) }}</dd></div></dl></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(storageClassDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(storageClassDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="configMapDetail">
            <dl class="resource-detail-stats"><div><dt>Immutable</dt><dd>{{ configMapDetail.immutable ? 'Yes' : 'No' }}</dd></div><div><dt>Data keys</dt><dd>{{ configMapDetail.dataKeys.length }}</dd></div><div><dt>Binary keys</dt><dd>{{ configMapDetail.binaryDataKeys.length }}</dd></div><div><dt>Created</dt><dd>{{ formatTime(configMapDetail.metadata.creationTimestamp) }}</dd></div></dl>
            <section class="detail-section"><h3>Data key names</h3><div class="detail-chip-list key-name-list"><span v-for="key in configMapDetail.dataKeys" :key="key">{{ key }}</span><div v-if="configMapDetail.dataKeys.length === 0" class="detail-empty">没有 Data key</div></div></section>
            <section class="detail-section"><h3>BinaryData key names</h3><div class="detail-chip-list key-name-list"><span v-for="key in configMapDetail.binaryDataKeys" :key="key">{{ key }}</span><div v-if="configMapDetail.binaryDataKeys.length === 0" class="detail-empty">没有 BinaryData key</div></div></section>
            <section class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(configMapDetail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(configMapDetail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <template v-else-if="m17Detail && isM17Kind(detailSelection.kind)">
            <div v-if="(detailSelection.kind === 'HPA' && m17HPASaturated(m17Detail)) || (detailSelection.kind === 'CronJob' && canOperate)" class="drawer-action-bar"><button v-if="detailSelection.kind === 'CronJob' && !(cronJobDetail?.spec.suspend ?? false)" class="secondary-button" type="button" :disabled="operationBusy" @click="previewCronJobOperation('cronjob.suspend')"><Pause :size="15" />暂停调度</button><button v-if="detailSelection.kind === 'CronJob' && (cronJobDetail?.spec.suspend ?? false)" class="secondary-button" type="button" :disabled="operationBusy" @click="previewCronJobOperation('cronjob.resume')"><Play :size="15" />恢复调度</button><button v-if="detailSelection.kind === 'HPA'" class="diagnose-button" type="button" :disabled="diagnosisLoading" @click="runDiagnosis('HPA', m17Detail)"><Sparkles :size="15" />检查扩容</button></div>
            <dl class="resource-detail-stats"><div v-for="entry in m17DetailStats(detailSelection.kind, m17Detail)" :key="entry.label"><dt>{{ entry.label }}</dt><dd>{{ entry.value }}</dd></div></dl>
            <section class="detail-section"><h3>配置与状态</h3><dl class="detail-definition-list"><div v-for="entry in m17DetailValues(detailSelection.kind, m17Detail)" :key="entry.label"><dt>{{ entry.label }}</dt><dd>{{ entry.value }}</dd></div><div v-if="m17DetailValues(detailSelection.kind, m17Detail).length === 0" class="detail-empty">没有可展示字段</div></dl></section>
            <section v-if="secretDetail" class="detail-section"><h3>Data key names</h3><div class="detail-chip-list key-name-list"><span v-for="key in secretDetail.dataKeys" :key="key">{{ key }}</span><div v-if="secretDetail.dataKeys.length === 0" class="detail-empty">没有 Data key</div></div></section>
            <section v-if="m17Conditions(detailSelection.kind, m17Detail).length" class="detail-section"><h3>Conditions</h3><div class="condition-list"><article v-for="condition in m17Conditions(detailSelection.kind, m17Detail)" :key="condition.type"><span :class="['condition-indicator', condition.status === 'True' ? 'true' : 'false']" /><div><strong>{{ condition.type }} · {{ condition.status }}</strong><small>{{ condition.reason || 'No reason' }}</small><p>{{ condition.message || '没有附加消息' }}</p></div></article></div></section>
            <section v-if="detailSelection.kind !== 'Secret'" class="detail-section"><h3>Labels</h3><div class="detail-chip-list"><span v-for="([key, value]) in sortedEntries(m17Detail.metadata.labels)" :key="key"><b>{{ key }}</b>= {{ value }}</span><div v-if="!sortedEntries(m17Detail.metadata.labels).length" class="detail-empty">没有 Labels</div></div></section>
          </template>

          <p v-if="operationError && !operationPlan" class="operation-inline-error">{{ operationError }}</p>
          <section v-if="controlledOperationKind(detailSelection)" class="detail-section operation-history-section">
            <div class="detail-section-heading"><h3>受控操作记录</h3><span>{{ operationHistory.length }}</span></div>
            <div class="operation-history-list"><article v-for="plan in operationHistory" :key="plan.id"><span class="remediation-status" :class="plan.status">{{ operationStatusLabel(plan.status) }}</span><div><strong>{{ operationActionLabel(plan.action) }}</strong><small>{{ formatTime(plan.created_at) }} · {{ plan.requested_by.name }}</small><p v-if="plan.change">{{ plan.change.field }}: {{ operationValue(plan.change.before) }} → {{ operationValue(plan.change.after) }}</p><p v-if="plan.last_error">{{ plan.last_error }}</p></div></article><div v-if="operationHistory.length === 0" class="detail-empty">暂无受控操作记录</div></div>
          </section>

          <section class="detail-section related-events-section">
            <div class="detail-section-heading"><h3>关联事件</h3><span>{{ relatedEvents.length }}</span></div>
            <div v-if="eventsLoading" class="event-state"><RefreshCw :size="16" class="spinning" />正在读取事件</div>
            <div v-else-if="eventsError" class="event-state warning">{{ eventsError }}</div>
            <div v-else class="related-event-list">
              <article v-for="event in relatedEvents" :key="event.metadata.uid || `${event.reason}-${eventTimestamp(event)}`" :class="{ warning: event.type === 'Warning' }">
                <span class="event-type-marker">{{ event.type === 'Warning' ? 'W' : 'N' }}</span>
                <div><strong>{{ event.reason || event.action || 'Event' }}</strong><small>{{ event.reportingComponent || 'Kubernetes' }} · {{ eventCount(event) }} 次 · {{ formatTime(eventTimestamp(event)) }}</small><p>{{ event.message || '没有事件消息' }}</p></div>
              </article>
              <div v-if="relatedEvents.length === 0" class="detail-empty">没有匹配此资源的事件</div>
            </div>
          </section>
        </template>
      </section>
    </div>

    <div v-if="operationPlan" class="log-overlay operation-overlay" @click.self="!operationBusy && clearOperationPreview()"><section class="operation-confirmation-dialog" role="dialog" aria-modal="true" aria-label="确认受控操作"><header><div><p class="context-label">SERVER-SIDE DRY-RUN PASSED</p><h2>{{ operationActionLabel(operationPlan.action) }}</h2></div><button class="icon-button" type="button" title="取消操作" aria-label="取消操作" :disabled="operationBusy" @click="clearOperationPreview"><X :size="18" /></button></header><dl><div><dt>目标</dt><dd>{{ operationPlan.target.kind }}/{{ operationPlan.target.namespace }}/{{ operationPlan.target.name }}</dd></div><div><dt>资源版本</dt><dd>{{ operationPlan.target.resource_version }}</dd></div><div><dt>变更</dt><dd>{{ operationPlan.change?.field }}: {{ operationValue(operationPlan.change?.before) }} → {{ operationValue(operationPlan.change?.after) }}</dd></div><div><dt>有效期至</dt><dd>{{ formatTime(operationPlan.expires_at) }}</dd></div></dl><p v-if="operationError" class="operation-inline-error">{{ operationError }}</p><label class="operation-confirm-check"><input v-model="operationConfirmed" type="checkbox" />我确认仅对上述固定资源和字段执行该操作。</label><footer><button class="secondary-button" type="button" :disabled="operationBusy" @click="clearOperationPreview">取消</button><button class="primary-button" type="button" :disabled="operationBusy || !operationConfirmed || !operationToken" @click="confirmOperation"><Check :size="15" />{{ operationBusy ? '执行中' : '确认执行' }}</button></footer></section></div>

    <div v-if="logTitle" class="log-overlay" @click.self="logTitle = ''"><section class="log-drawer" role="dialog" aria-modal="true" aria-label="Pod 日志"><header><div><p class="context-label">POD LOGS</p><h2>{{ logTitle }}</h2></div><button class="icon-button" aria-label="关闭日志" @click="logTitle = ''"><X :size="18" /></button></header><pre>{{ logLoading ? '正在读取日志…' : logs }}</pre></section></div>
    <div v-if="diagnosis" class="log-overlay" @click.self="diagnosis = null"><section class="diagnosis-drawer" role="dialog" aria-modal="true" aria-label="规则诊断"><header><div><p class="context-label">RULE DIAGNOSIS</p><h2>{{ diagnosis.rule_id }}</h2></div><button class="icon-button" aria-label="关闭诊断" @click="diagnosis = null"><X :size="18" /></button></header><span class="severity-badge">{{ diagnosis.severity }}</span><p class="diagnosis-summary">{{ diagnosis.summary }}</p><h3>可能根因</h3><ol><li v-for="item in diagnosis.root_causes" :key="item">{{ item }}</li></ol><h3>处理建议</h3><ol><li v-for="item in diagnosis.recommendations" :key="item">{{ item }}</li></ol><h3>证据</h3><article v-for="item in diagnosis.evidence" :key="`${item.type}-${item.source}`" class="evidence-card"><strong>{{ item.type }} · {{ item.source }}</strong><pre>{{ JSON.stringify(item.content, null, 2) }}</pre></article></section></div>
  </ConsoleLayout>
</template>
