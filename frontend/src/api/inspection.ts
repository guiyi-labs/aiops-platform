import { authorizedRequest } from './client'
import type { InspectionRule, InspectionPlanView, InspectionTaskView, InspectionResultView, CreateInspectionPlanRequest, RunInspectionRequest, InspectionCoverageSummary } from '../types/inspection'

export function listInspectionRulesCatalog(token: string): Promise<{ items: InspectionRule[] }> {
  return authorizedRequest('/api/v1/aiops/inspection/rules/catalog', token)
}

export function listEffectiveRules(token: string, clusterId: number): Promise<{ cluster_id: number; items: InspectionRule[] }> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/inspection/rules`, token)
}

export function listInspectionPlans(token: string): Promise<{ items: InspectionPlanView[] }> {
  return authorizedRequest('/api/v1/aiops/inspection/plans', token)
}

export function createInspectionPlan(token: string, input: CreateInspectionPlanRequest): Promise<InspectionPlanView> {
  return authorizedRequest('/api/v1/aiops/inspection/plans', token, { method: 'POST', body: JSON.stringify(input) })
}

export function getInspectionPlan(token: string, id: number): Promise<InspectionPlanView> {
  return authorizedRequest(`/api/v1/aiops/inspection/plans/${id}`, token)
}

export function deleteInspectionPlan(token: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/aiops/inspection/plans/${id}`, token, { method: 'DELETE' })
}

export function runInspection(token: string, input: RunInspectionRequest): Promise<InspectionTaskView> {
  return authorizedRequest('/api/v1/aiops/inspection/run', token, { method: 'POST', body: JSON.stringify(input) })
}

export function listInspectionTasks(token: string, params?: { limit?: number }): Promise<{ items: InspectionTaskView[]; total: number }> {
  const sp = new URLSearchParams()
  if (params?.limit) sp.set('limit', String(params.limit))
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/inspection/tasks${q ? `?${q}` : ''}`, token)
}

export function getInspectionTask(token: string, id: number): Promise<InspectionTaskView> {
  return authorizedRequest(`/api/v1/aiops/inspection/tasks/${id}`, token)
}

export function listInspectionResults(token: string, params?: { task_id?: number; cluster_id?: number; limit?: number }): Promise<{ items: InspectionResultView[]; total: number }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null) sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/inspection/results${q ? `?${q}` : ''}`, token)
}

export function getInspectionResult(token: string, id: number): Promise<InspectionResultView> {
  return authorizedRequest(`/api/v1/aiops/inspection/results/${id}`, token)
}

export function getInspectionCoverage(token: string, params?: { window_days?: number }): Promise<InspectionCoverageSummary> {
  const sp = new URLSearchParams()
  if (params?.window_days) sp.set('window_days', String(params.window_days))
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/inspection/coverage${q ? `?${q}` : ''}`, token)
}
