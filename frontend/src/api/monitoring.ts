import { authorizedRequest } from './client'
import type { ClusterDashboardResponse, WorkspaceDashboardResponse, LogResult, LogsQueryRequest } from '../types/monitoring'

export function getClusterDashboard(token: string, clusterId: number, template: string): Promise<ClusterDashboardResponse> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/monitoring/dashboard/${template}`, token)
}

export function getWorkspaceDashboard(token: string, workspaceId: number): Promise<WorkspaceDashboardResponse> {
  return authorizedRequest(`/api/v1/workspaces/${workspaceId}/monitoring/dashboard`, token)
}

export function queryLogs(token: string, clusterId: number, query: LogsQueryRequest): Promise<LogResult> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/logs/query`, token, { method: 'POST', body: JSON.stringify(query) })
}
