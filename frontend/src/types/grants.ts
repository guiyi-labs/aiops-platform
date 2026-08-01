// ClusterGrant authorizes a user to access all namespaces in a cluster.
export interface ClusterGrant {
  id: number
  user_id: number
  cluster_id: number
  created_at: string
}

export interface ClusterGrantList {
  items: ClusterGrant[]
}

// NamespaceGrant authorizes a user to access one exact namespace in a cluster.
export interface NamespaceGrant {
  id: number
  user_id: number
  cluster_id: number
  namespace: string
  created_at: string
}

export interface NamespaceGrantList {
  items: NamespaceGrant[]
}

// MyGrants bundles the current caller's grants, returned by GET /auth/me/grants.
export interface MyGrants {
  cluster_grants: ClusterGrant[]
  namespace_grants: NamespaceGrant[]
}
