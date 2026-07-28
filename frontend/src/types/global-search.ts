export type GlobalSearchKind = 'Pod' | 'Deployment' | 'Service' | 'Ingress'
export type GlobalSearchHealth = 'healthy' | 'degraded' | 'unknown'

export interface GlobalSearchItem {
  cluster_id: number
  cluster_name: string
  kind: GlobalSearchKind
  namespace: string
  name: string
  health: GlobalSearchHealth
  summary: string
}

export interface GlobalSearchFailure {
  cluster_id: number
  cluster_name: string
  kind: GlobalSearchKind
  code: 'QUERY_FAILED' | 'TIMEOUT'
}

export interface GlobalSearchResponse {
  query: string
  namespace?: string
  kinds: GlobalSearchKind[]
  items: GlobalSearchItem[]
  total: number
  remaining: number
  clusters_total: number
  clusters_searched: number
  clusters_remaining: number
  complete: boolean
  failures: GlobalSearchFailure[]
  checked_at: string
  limits: {
    max_clusters: number
    max_concurrent_clusters: number
    per_cluster_timeout_ms: number
    max_results: number
    per_kind_limit: number
  }
}

export interface GlobalSearchParameters {
  query: string
  namespace?: string
  kinds: GlobalSearchKind[]
  clusterLimit?: number
  limit?: number
}

export type SavedFilterIncompatibilityCode = 'SCHEMA_VERSION' | 'QUERY_SHAPE'

export interface SavedGlobalSearchFilter {
  id: number
  name: string
  query: string
  namespace?: string
  kinds: GlobalSearchKind[]
  schema_version: number
  compatible: boolean
  incompatibility_code?: SavedFilterIncompatibilityCode
  created_at: string
  updated_at: string
}

export interface SavedGlobalSearchFilterList {
  items: SavedGlobalSearchFilter[]
  total: number
  limit: number
}

export interface SavedGlobalSearchFilterQuery {
  query: string
  namespace: string
  kinds: GlobalSearchKind[]
}

export interface CreateSavedGlobalSearchFilter extends SavedGlobalSearchFilterQuery {
  name: string
}

export interface UpdateSavedGlobalSearchFilter {
  name?: string
  query?: string
  namespace?: string
  kinds?: GlobalSearchKind[]
}
