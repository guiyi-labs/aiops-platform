import { authorizedRequest } from './client'
import type { ClusterDashboardResponse, WorkspaceDashboardResponse, LogResult, LogsQueryRequest } from '../types/monitoring'

// 后端 clusterDashboard 要求 from/to 为必填的 RFC3339 时间戳（见
// internal/httpserver/monitoring.go 的 parseMonitoringTime），且窗口上限 24h。
// 缺省取最近 1 小时，避免前端漏传导致 400 INVALID_QUERY。
export function getClusterDashboard(
  token: string,
  clusterId: number,
  template: string,
  window?: { from?: string; to?: string },
): Promise<ClusterDashboardResponse> {
  const to = window?.to ?? new Date().toISOString()
  const from = window?.from ?? new Date(Date.now() - 60 * 60 * 1000).toISOString()
  const query = new URLSearchParams({ from, to }).toString()
  return authorizedRequest(`/api/v1/clusters/${clusterId}/monitoring/dashboard/${template}?${query}`, token)
}

export function getWorkspaceDashboard(
  token: string,
  workspaceId: number,
  window?: { from?: string; to?: string },
): Promise<WorkspaceDashboardResponse> {
  const to = window?.to ?? new Date().toISOString()
  const from = window?.from ?? new Date(Date.now() - 60 * 60 * 1000).toISOString()
  const query = new URLSearchParams({ from, to }).toString()
  return authorizedRequest(`/api/v1/workspaces/${workspaceId}/monitoring/dashboard?${query}`, token)
}

export function queryLogs(token: string, clusterId: number, query: LogsQueryRequest): Promise<LogResult> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/logs/query`, token, { method: 'POST', body: JSON.stringify(query) })
}
