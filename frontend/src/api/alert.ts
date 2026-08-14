import { authorizedRequest } from './client'
import type { AlertInstance, AlertOverviewResponse, AlertRule, AlertRuleCreate, AlertRulePatch } from '../types/alert'

export function listAlertRules(token: string, clusterID: number): Promise<AlertRule[]> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alert-rules`, token)
}

export function createAlertRule(token: string, clusterID: number, input: AlertRuleCreate): Promise<AlertRule> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alert-rules`, token, {
    method: 'POST', body: JSON.stringify(input),
  })
}

export function getAlertRule(token: string, clusterID: number, ruleID: number): Promise<AlertRule> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alert-rules/${ruleID}`, token)
}

export function patchAlertRule(token: string, clusterID: number, ruleID: number, input: AlertRulePatch): Promise<AlertRule> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alert-rules/${ruleID}`, token, {
    method: 'PATCH', body: JSON.stringify(input),
  })
}

export function deleteAlertRule(token: string, clusterID: number, ruleID: number): Promise<void> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alert-rules/${ruleID}`, token, {
    method: 'DELETE',
  })
}

export function listAlertInstances(token: string, clusterID: number, filters: { state?: string; ruleID?: number; limit?: number } = {}): Promise<AlertInstance[]> {
  const query = new URLSearchParams()
  if (filters.state) query.set('state', filters.state)
  if (filters.ruleID) query.set('rule_id', String(filters.ruleID))
  if (filters.limit) query.set('limit', String(filters.limit))
  const qs = query.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alerts${qs ? '?' + qs : ''}`, token)
}

export function getAlertInstance(token: string, clusterID: number, alertID: number): Promise<AlertInstance> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alerts/${alertID}`, token)
}

export function getAlertOverview(token: string, clusterID: number, params: { window_minutes?: number; max_groups?: number } = {}): Promise<AlertOverviewResponse> {
  const query = new URLSearchParams()
  if (params.window_minutes) query.set('window_minutes', String(params.window_minutes))
  if (params.max_groups) query.set('max_groups', String(params.max_groups))
  const qs = query.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterID}/alerts/overview${qs ? '?' + qs : ''}`, token)
}
