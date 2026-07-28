import { authorizedRequest } from './client'
import { APIError } from './auth'
import type { APIErrorBody } from '../types/auth'
import type { AuditListResponse, AuditResult } from '../types/audit'

export interface AuditExportResult { blob: Blob; filename: string; rows: number; total: number; truncated: boolean }

type AuditFilters = { clusterID?: number; action?: string; result?: AuditResult | ''; limit?: number }

function auditQuery(filters: AuditFilters, defaultLimit: number): URLSearchParams {
  const query = new URLSearchParams({ limit: String(filters.limit ?? defaultLimit) })
  if (filters.clusterID) query.set('cluster_id', String(filters.clusterID))
  if (filters.action) query.set('action', filters.action)
  if (filters.result) query.set('result', filters.result)
  return query
}

export function listAuditLogs(token: string, filters: AuditFilters = {}): Promise<AuditListResponse> {
  const query = auditQuery(filters, 100)
  return authorizedRequest(`/api/v1/audit-logs?${query}`, token)
}

export async function exportAuditLogs(token: string, filters: AuditFilters = {}): Promise<AuditExportResult> {
  const response = await fetch(`/api/v1/audit-logs/export?${auditQuery(filters, 5000)}`, {
    headers: { Accept: 'text/csv', Authorization: `Bearer ${token}` },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => undefined) as APIErrorBody | undefined
    throw new APIError(response.status, body?.code ?? 'REQUEST_FAILED', body?.message ?? `Request failed with status ${response.status}`, body?.request_id)
  }
  const disposition = response.headers.get('Content-Disposition') || ''
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] || 'audit-logs.csv'
  return {
    blob: await response.blob(), filename,
    rows: Number(response.headers.get('X-Audit-Export-Rows') || 0),
    total: Number(response.headers.get('X-Audit-Export-Total') || 0),
    truncated: response.headers.get('X-Audit-Export-Truncated') === 'true',
  }
}
