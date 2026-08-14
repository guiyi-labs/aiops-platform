import { authorizedRequest } from './client'
import { APIError } from './auth'
import type { APIErrorBody } from '../types/auth'
import type { Incident, IncidentBatchAssignResult, IncidentContextCockpit, IncidentCreateInput, IncidentEvidenceItem, IncidentListResponse, IncidentMetrics, IncidentResponseCatalog, IncidentRunbookResponse, IncidentSeverity, IncidentStatus, IncidentSummary } from '../types/incident'

export function listIncidents(token: string, filters: { clusterID?: number; status?: IncidentStatus | ''; assigneeID?: number; followerID?: number; limit?: number } = {}): Promise<IncidentListResponse> {
  const query = new URLSearchParams({ limit: String(filters.limit ?? 50) })
  if (filters.clusterID) query.set('cluster_id', String(filters.clusterID))
  if (filters.status) query.set('status', filters.status)
  if (filters.assigneeID) query.set('assignee_id', String(filters.assigneeID))
  if (filters.followerID) query.set('follower_id', String(filters.followerID))
  return authorizedRequest(`/api/v1/incidents?${query}`, token)
}

export function getIncident(token: string, incidentID: number): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}`, token)
}
export function listIncidentResponseCatalog(token: string): Promise<IncidentResponseCatalog> {
  return authorizedRequest('/api/v1/incidents/templates', token)
}
export function getIncidentEvidence(token: string, incidentID: number): Promise<{ items: IncidentEvidenceItem[] }> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/evidence`, token)
}

export function getIncidentSummary(token: string): Promise<IncidentSummary> {
  return authorizedRequest('/api/v1/incidents/summary', token)
}

export function getIncidentMetrics(token: string, filters: { clusterID?: number; days?: number } = {}): Promise<IncidentMetrics> {
  const query = new URLSearchParams()
  if (filters.clusterID !== undefined) query.set('cluster_id', String(filters.clusterID))
  if (filters.days !== undefined) query.set('days', String(filters.days))
  const suffix = query.toString()
  return authorizedRequest(`/api/v1/incidents/metrics${suffix ? `?${suffix}` : ''}`, token)
}

export function getIncidentRunbook(token: string, incidentID: number): Promise<IncidentRunbookResponse> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/runbook`, token)
}

export function getIncidentContext(token: string, incidentID: number): Promise<IncidentContextCockpit> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/context`, token)
}

export function createIncident(token: string, input: IncidentCreateInput): Promise<Incident> {
  return authorizedRequest('/api/v1/incidents', token, {
    method: 'POST', body: JSON.stringify(input),
  })
}

export function transitionIncident(token: string, incidentID: number, expectedVersion: number, status: IncidentStatus, comment: string): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}`, token, {
    method: 'PATCH', body: JSON.stringify({ expected_version: expectedVersion, status, comment }),
  })
}

export function assignIncident(token: string, incidentID: number, expectedVersion: number, assigneeUserID: number, comment: string): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/assignment`, token, {
    method: 'PATCH', body: JSON.stringify({ expected_version: expectedVersion, assignee_user_id: assigneeUserID, comment }),
  })
}

export function batchAssignIncidents(token: string, input: { incident_ids: number[]; assignee_user_id: number; comment: string }): Promise<IncidentBatchAssignResult> {
  return authorizedRequest('/api/v1/incidents/batch-assign', token, {
    method: 'POST', body: JSON.stringify(input),
  })
}

export function addIncidentFollower(token: string, incidentID: number, userID: number): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/followers`, token, {
    method: 'POST', body: JSON.stringify({ user_id: userID }),
  })
}

export function removeIncidentFollower(token: string, incidentID: number, userID: number): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/followers/${userID}`, token, { method: 'DELETE' })
}

export function addIncidentNote(token: string, incidentID: number, expectedVersion: number, content: string): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/notes`, token, {
    method: 'POST', body: JSON.stringify({ expected_version: expectedVersion, content }),
  })
}

export function setIncidentPostmortem(token: string, incidentID: number, expectedVersion: number, content: string): Promise<Incident> {
  return authorizedRequest(`/api/v1/incidents/${incidentID}/postmortem`, token, {
    method: 'PUT', body: JSON.stringify({ expected_version: expectedVersion, content }),
  })
}

export async function exportIncidentPostmortem(token: string, incidentID: number): Promise<{ blob: Blob; filename: string }> {
  const response = await fetch(`/api/v1/incidents/${incidentID}/postmortem/export`, {
    headers: { Accept: 'text/markdown', Authorization: `Bearer ${token}` },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => undefined) as APIErrorBody | undefined
    throw new APIError(response.status, body?.code ?? 'REQUEST_FAILED', body?.message ?? `Request failed with status ${response.status}`, body?.request_id)
  }
  const disposition = response.headers.get('Content-Disposition') || ''
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] || `incident-${incidentID}-postmortem.md`
  return { blob: await response.blob(), filename }
}

export const severityLabels: Record<IncidentSeverity, string> = {
  info: '信息',
  warning: '警告',
  high: '高',
  critical: '严重',
}

export const statusLabels: Record<IncidentStatus, string> = {
  open: '待确认',
  confirmed: '已确认',
  resolved: '已解决',
  dismissed: '已驳回',
}
