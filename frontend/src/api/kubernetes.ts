import { authorizedRequest } from './client'
import type { ConfigMapResource, CronJobResource, DaemonSetResource, Deployment, EndpointSliceResource, HorizontalPodAutoscalerResource, IngressResource, JobResource, KubernetesEvent, LimitRangeResource, ListResponse, Namespace, NodeMetric, NodeResource, PersistentVolumeClaim, Pod, PodMetric, ReplicaSetResource, ResourceQuotaResource, SecretResource, ServiceResource, StatefulSetResource, StorageClassResource } from '../types/kubernetes'

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
