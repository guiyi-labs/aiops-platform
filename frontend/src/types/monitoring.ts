export interface MonitoringPanel {
  title: string
  metric: string
  unit: string
  resource_kind: string
  description?: string
}

export interface ClusterDashboardResponse {
  template: string
  cluster_id: number
  from: string
  to: string
  panels: MonitoringPanel[]
}

export interface WorkspaceClusterEntry {
  cluster_id: number
  namespaces: string[]
}

export interface WorkspaceDashboardResponse {
  template: string
  workspace_id: number
  from: string
  to: string
  panels: MonitoringPanel[]
  clusters: WorkspaceClusterEntry[]
}

export interface LogEntry {
  Timestamp: string
  Namespace: string
  Pod: string
  Container: string
  Stream: string
  Line: string
}

export interface LogResult {
  Entries: LogEntry[]
  State: string
  TotalReturned: number
  Error: string
}

export interface LogsQueryRequest {
  namespace: string
  pod?: string
  container?: string
  text_filter?: string
  start: string
  end: string
  direction?: string
  limit?: number
}
