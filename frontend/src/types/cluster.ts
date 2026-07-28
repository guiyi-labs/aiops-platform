export type ClusterStatus = 'disabled' | 'unknown' | 'ready' | 'unreachable'

export interface ClusterCondition {
  type: 'Ready' | 'CredentialValid' | 'Reachable'
  status: 'True' | 'False' | 'Unknown'
  reason: string
  message: string
  last_transition_time: string
}

export interface Cluster {
  id: number
  name: string
  api_server: string
  enabled: boolean
  status: ClusterStatus
  kubernetes_version?: string
  last_probed_at?: string
  created_at: string
  updated_at: string
  conditions: ClusterCondition[]
}

export interface ClusterList {
  items: Cluster[]
  total: number
  remaining: number
}
