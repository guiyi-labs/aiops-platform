import { authorizedRequest } from './client'
import type { BackupPlan, ConfigMapResource, CronJobResource, DaemonSetResource, Deployment, EndpointSliceResource, HorizontalPodAutoscalerResource, IngressResource, JobResource, KubernetesEvent, LimitRangeResource, ListResponse, MaintenanceAction, MaintenancePlan, Namespace, NamespacePosture, NodeMetric, NodeResource, PersistentVolume, PersistentVolumeClaim, Pod, PodContainerInfo, PodContainerLog, PodDisruptionBudgetResource, PodLogsResponse, PodMetric, NetworkPolicyResource, PostureListEntry, ReplicaSetResource, ResourceQuotaResource, RestorePlan, SecretResource, ServiceAccountResource, ServiceResource, StatefulSetResource, StorageClassResource, VeleroBackup, VeleroCapability } from '../types/kubernetes'

function queryString(values: Record<string, string | number | boolean | undefined>): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== '') query.set(key, String(value))
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

export function listNamespaces(token: string, clusterID: number): Promise<ListResponse<Namespace>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/namespaces?limit=100&sort_by=name&ascending=true`, token)
}

export function listNodes(token: string, clusterID: number, name = ''): Promise<ListResponse<NodeResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/nodes${queryString({ name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listNodeMetrics(token: string, clusterID: number, name = ''): Promise<ListResponse<NodeMetric>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/metrics/nodes${queryString({ name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listPodMetrics(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<PodMetric>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/metrics/pods${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listDeployments(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<Deployment>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/deployments${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listStatefulSets(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<StatefulSetResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/statefulsets${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listDaemonSets(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<DaemonSetResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/daemonsets${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listReplicaSets(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<ReplicaSetResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/replicasets${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listJobs(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<JobResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/jobs${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listCronJobs(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<CronJobResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/cronjobs${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listHorizontalPodAutoscalers(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<HorizontalPodAutoscalerResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/horizontalpodautoscalers${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listResourceQuotas(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<ResourceQuotaResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/resourcequotas${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listLimitRanges(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<LimitRangeResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/limitranges${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listSecrets(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<SecretResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/secrets${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listServices(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<ServiceResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/services${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listIngresses(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<IngressResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/ingresses${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listEndpointSlices(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<EndpointSliceResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/endpointslices${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listPersistentVolumeClaims(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<PersistentVolumeClaim>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/persistentvolumeclaims${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listStorageClasses(token: string, clusterID: number, name = ''): Promise<ListResponse<StorageClassResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/storageclasses${queryString({ name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listConfigMaps(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<ConfigMapResource>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/configmaps${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listPods(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<Pod>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function listEvents(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<KubernetesEvent>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/events${queryString({ namespace, name, limit: 100 })}`, token)
}

export function getNode(token: string, clusterID: number, name: string): Promise<NodeResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/nodes/${encodeURIComponent(name)}`, token)
}

export function getPod(token: string, clusterID: number, namespace: string, name: string): Promise<Pod> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getDeployment(token: string, clusterID: number, namespace: string, name: string): Promise<Deployment> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getStatefulSet(token: string, clusterID: number, namespace: string, name: string): Promise<StatefulSetResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/statefulsets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getDaemonSet(token: string, clusterID: number, namespace: string, name: string): Promise<DaemonSetResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/daemonsets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getReplicaSet(token: string, clusterID: number, namespace: string, name: string): Promise<ReplicaSetResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/replicasets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getJob(token: string, clusterID: number, namespace: string, name: string): Promise<JobResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/jobs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getCronJob(token: string, clusterID: number, namespace: string, name: string): Promise<CronJobResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/cronjobs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getHorizontalPodAutoscaler(token: string, clusterID: number, namespace: string, name: string): Promise<HorizontalPodAutoscalerResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/horizontalpodautoscalers/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getResourceQuota(token: string, clusterID: number, namespace: string, name: string): Promise<ResourceQuotaResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/resourcequotas/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getLimitRange(token: string, clusterID: number, namespace: string, name: string): Promise<LimitRangeResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/limitranges/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getSecret(token: string, clusterID: number, namespace: string, name: string): Promise<SecretResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getService(token: string, clusterID: number, namespace: string, name: string): Promise<ServiceResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getIngress(token: string, clusterID: number, namespace: string, name: string): Promise<IngressResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/ingresses/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getPersistentVolumeClaim(token: string, clusterID: number, namespace: string, name: string): Promise<PersistentVolumeClaim> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/persistentvolumeclaims/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getStorageClass(token: string, clusterID: number, name: string): Promise<StorageClassResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/storageclasses/${encodeURIComponent(name)}`, token)
}

export function getConfigMap(token: string, clusterID: number, namespace: string, name: string): Promise<ConfigMapResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getPodLogs(token: string, clusterID: number, namespace: string, name: string, container = '', previous = false): Promise<{ logs: string; previous: boolean }> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/logs${queryString({ container, previous, tail_lines: 200 })}`, token)
}

export function listPodContainers(token: string, clusterID: number, namespace: string, name: string): Promise<ListResponse<PodContainerInfo>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/containers`, token)
}

export function getPodLogsSince(token: string, clusterID: number, namespace: string, name: string, opts: { container?: string; previous?: boolean; tail_lines?: number; since_seconds?: number; since_time?: string }): Promise<PodContainerLog> {
  const params: Record<string, string | number | boolean | undefined> = {
    container: opts.container,
    previous: opts.previous ?? false,
    tail_lines: opts.tail_lines ?? 200,
  }
  if (opts.since_seconds) params.since_seconds = opts.since_seconds
  if (opts.since_time) params.since_time = opts.since_time
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/logs_since${queryString(params)}`, token)
}

export function getAllPodLogs(token: string, clusterID: number, namespace: string, name: string, opts: { previous?: boolean; tail_lines?: number; since_seconds?: number } = {}): Promise<PodLogsResponse> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/all_logs${queryString({ previous: opts.previous ?? false, tail_lines: opts.tail_lines ?? 200, since_seconds: opts.since_seconds })}`, token)
}

export function listPersistentVolumes(token: string, clusterID: number): Promise<ListResponse<PersistentVolume>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/persistentvolumes`, token)
}

export function getPersistentVolume(token: string, clusterID: number, name: string): Promise<PersistentVolume> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/persistentvolumes/${encodeURIComponent(name)}`, token)
}

export function listPodDisruptionBudgets(token: string, clusterID: number, namespace?: string): Promise<ListResponse<PodDisruptionBudgetResource>> {
  const qs = queryString({ namespace })
  return authorizedRequest(`/api/v1/clusters/${clusterID}/poddisruptionbudgets${qs}`, token)
}

export function getPodDisruptionBudget(token: string, clusterID: number, namespace: string, name: string): Promise<PodDisruptionBudgetResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/poddisruptionbudgets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function listNetworkPolicies(token: string, clusterID: number, namespace?: string): Promise<ListResponse<NetworkPolicyResource>> {
  const qs = queryString({ namespace })
  return authorizedRequest(`/api/v1/clusters/${clusterID}/networkpolicies${qs}`, token)
}

export function getNetworkPolicy(token: string, clusterID: number, namespace: string, name: string): Promise<NetworkPolicyResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/networkpolicies/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function listServiceAccounts(token: string, clusterID: number, namespace?: string): Promise<ListResponse<ServiceAccountResource>> {
  const qs = queryString({ namespace })
  return authorizedRequest(`/api/v1/clusters/${clusterID}/serviceaccounts${qs}`, token)
}

export function getServiceAccount(token: string, clusterID: number, namespace: string, name: string): Promise<ServiceAccountResource> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/serviceaccounts/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function getResourceManifest(token: string, clusterID: number, kind: string, namespace: string, name: string): Promise<Record<string, unknown>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/resources/${encodeURIComponent(kind)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/manifest`, token)
}

export function getVeleroCapability(token: string, clusterID: number): Promise<VeleroCapability> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/velero/capability`, token)
}

export function listBackups(token: string, clusterID: number, namespace = '', name = ''): Promise<ListResponse<VeleroBackup>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/backups${queryString({ namespace, name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function getBackup(token: string, clusterID: number, namespace: string, name: string): Promise<VeleroBackup> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/backups/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, token)
}

export function previewBackupPlan(token: string, clusterID: number, body: {
  backup_name: string
  backup_namespace: string
  included_namespaces: string[]
  storage_location: string
  ttl?: string
  include_cluster_resources?: boolean
  snapshot_volumes?: boolean
  label_selector?: Record<string, string>
}): Promise<BackupPlan> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/backup-plans/preview`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function executeBackupPlan(token: string, planID: string, confirmationToken: string, idempotencyKey: string): Promise<BackupPlan> {
  return authorizedRequest(`/api/v1/backup-plans/${encodeURIComponent(planID)}/execute`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ confirmation_token: confirmationToken }),
  })
}

export function listBackupPlans(token: string, clusterID: number): Promise<{ items: BackupPlan[]; total: number }> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/backup-plans`, token)
}

export function listNamespacePostures(token: string, clusterID: number, name = ''): Promise<ListResponse<PostureListEntry>> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/namespace-postures${queryString({ name, limit: 100, sort_by: 'name', ascending: true })}`, token)
}

export function getNamespacePosture(token: string, clusterID: number, namespace: string): Promise<NamespacePosture> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/namespace-postures/${encodeURIComponent(namespace)}`, token)
}

// --- Node Maintenance (M30) ---

export function listMaintenancePlans(token: string, clusterID: number): Promise<{ items: MaintenancePlan[]; total: number }> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/maintenance-plans`, token)
}

export function previewMaintenancePlan(token: string, clusterID: number, action: MaintenanceAction, nodeName: string): Promise<MaintenancePlan> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/maintenance-plans/preview`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action, node_name: nodeName }),
  })
}

export function executeMaintenancePlan(token: string, planID: string, confirmationToken: string, idempotencyKey: string): Promise<MaintenancePlan> {
  return authorizedRequest(`/api/v1/maintenance-plans/${encodeURIComponent(planID)}/execute`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ confirmation_token: confirmationToken }),
  })
}

// --- Restore Rehearsal (M31) ---

export function listRestorePlans(token: string, clusterID: number): Promise<{ items: RestorePlan[]; total: number }> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/restore-plans`, token)
}

export function previewRestorePlan(token: string, clusterID: number, body: {
  source_backup_name: string
  source_backup_namespace: string
}): Promise<RestorePlan> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/restore-plans/preview`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function executeRestorePlan(token: string, planID: string, confirmationToken: string, idempotencyKey: string): Promise<RestorePlan> {
  return authorizedRequest(`/api/v1/restore-plans/${encodeURIComponent(planID)}/execute`, token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ confirmation_token: confirmationToken }),
  })
}
