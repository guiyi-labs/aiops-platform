import { authorizedRequest } from './client'
import type {
  ClusterGrant,
  ClusterGrantList,
  MyGrants,
  NamespaceGrant,
  NamespaceGrantList,
} from '../types/grants'

// Cluster grant endpoints (SystemAdmin only, operate on a target user's grants).

export function listClusterGrants(accessToken: string, userID: number): Promise<ClusterGrantList> {
  return authorizedRequest(`/api/v1/users/${userID}/cluster-grants`, accessToken)
}

export function createClusterGrant(accessToken: string, userID: number, clusterID: number): Promise<ClusterGrant> {
  return authorizedRequest(`/api/v1/users/${userID}/cluster-grants`, accessToken, {
    method: 'POST',
    body: JSON.stringify({ cluster_id: clusterID }),
  })
}

export function deleteClusterGrant(accessToken: string, userID: number, clusterID: number): Promise<void> {
  return authorizedRequest(`/api/v1/users/${userID}/cluster-grants/${clusterID}`, accessToken, {
    method: 'DELETE',
  })
}

// Namespace grant endpoints (SystemAdmin only, operate on a target user's grants).

export function listNamespaceGrants(accessToken: string, userID: number): Promise<NamespaceGrantList> {
  return authorizedRequest(`/api/v1/users/${userID}/namespace-grants`, accessToken)
}

export function createNamespaceGrant(
  accessToken: string,
  userID: number,
  clusterID: number,
  namespace: string,
): Promise<NamespaceGrant> {
  return authorizedRequest(`/api/v1/users/${userID}/namespace-grants`, accessToken, {
    method: 'POST',
    body: JSON.stringify({ cluster_id: clusterID, namespace }),
  })
}

export function deleteNamespaceGrant(
  accessToken: string,
  userID: number,
  clusterID: number,
  namespace: string,
): Promise<void> {
  const encodedNamespace = encodeURIComponent(namespace)
  return authorizedRequest(
    `/api/v1/users/${userID}/namespace-grants/${clusterID}/${encodedNamespace}`,
    accessToken,
    { method: 'DELETE' },
  )
}

// My grants endpoint (any authenticated user reads their own grants).

export function getMyGrants(accessToken: string): Promise<MyGrants> {
  return authorizedRequest('/api/v1/auth/me/grants', accessToken)
}
