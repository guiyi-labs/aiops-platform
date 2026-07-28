import { authorizedRequest } from './client'
import type { Cluster, ClusterList } from '../types/cluster'

export function listClusters(accessToken: string): Promise<ClusterList> {
  return authorizedRequest('/api/v1/clusters', accessToken)
}

export function createCluster(accessToken: string, name: string, kubeconfig: string): Promise<Cluster> {
  return authorizedRequest('/api/v1/clusters', accessToken, { method: 'POST', body: JSON.stringify({ name, kubeconfig }) })
}

export function probeCluster(accessToken: string, id: number): Promise<Cluster> {
  return authorizedRequest(`/api/v1/clusters/${id}/probe`, accessToken, { method: 'POST' })
}

export function setClusterEnabled(accessToken: string, id: number, enabled: boolean): Promise<void> {
  return authorizedRequest(`/api/v1/clusters/${id}`, accessToken, { method: 'PATCH', body: JSON.stringify({ enabled }) })
}

export function deleteCluster(accessToken: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/clusters/${id}`, accessToken, { method: 'DELETE' })
}

export function updateClusterCredential(accessToken: string, id: number, kubeconfig: string): Promise<Cluster> {
  return authorizedRequest(`/api/v1/clusters/${id}/credentials`, accessToken, { method: 'PUT', body: JSON.stringify({ kubeconfig }) })
}
