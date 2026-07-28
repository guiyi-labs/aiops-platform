export type FleetHealthStatus = 'healthy' | 'degraded' | 'partial' | 'unavailable' | 'timed_out'

export interface FleetResourceSummary {
  healthy: number
  sampled: number
  total: number
  complete: boolean
}

export interface FleetWarningSummary {
  count: number
  sampled: number
  total: number
  complete: boolean
}

export interface FleetClusterHealth {
  cluster_id: number
  cluster_name: string
  kubernetes_version?: string
  status: FleetHealthStatus
  nodes: FleetResourceSummary
  pods: FleetResourceSummary
  deployments: FleetResourceSummary
  warnings: FleetWarningSummary
  failures: Array<{ scope: 'nodes' | 'pods' | 'deployments' | 'events'; code: 'QUERY_FAILED' | 'TIMEOUT' }>
  duration_ms: number
}

export interface FleetHealthResponse {
  items: FleetClusterHealth[]
  total: number
  remaining: number
  checked_at: string
  limits: {
    max_clusters: number
    max_concurrent_clusters: number
    per_cluster_timeout_ms: number
    resource_sample_limit: number
  }
}
