// AIOps API module — covers M39-M45/M56 endpoints under /api/v1/aiops/.
// All functions follow the project convention: (token, ...params) => Promise<T>.

import { authorizedRequest } from './client'
import type {
  SignalOverview,
  SignalListResponse,
  SignalDescriptor,
  TopologyGraph,
  ChangeTimelineResponse,
  SLODefinition,
  SLODefinitionListResponse,
  SLOEvaluation,
  BurnStatus,
  SLOBurnSummaryResponse, SLOEvaluationListResponse,
  SLITemplate,
  SLOCreateRequest,
  SLOPatchRequest,
  CorrelationRule,
  CaseListResponse,
  CaseTimelineResponse,
  CaseView,
  ActionCandidateListResponse,
  Runbook,
  Investigation,
  InvestigationListResponse,
  ActionPlanResponse,
  ActionPlanListResponse,
  ActionVerification,
  CreatePlanRequest,
  ExecutePlanRequest,
  QualityReport,
} from '../types/aiops'

// ── M39 Signal ──────────────────────────────────────────────────────────────

export function getAIOpsOverview(token: string): Promise<SignalOverview> {
  return authorizedRequest('/api/v1/aiops/overview', token)
}

export function listSignals(
  token: string,
  params?: {
    cluster_id?: number
    namespace?: string
    severity?: string
    state?: string
    producer?: string
    limit?: number
    cursor?: string
  },
): Promise<SignalListResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/signals${q ? `?${q}` : ''}`, token)
}

export function listSignalCatalog(token: string): Promise<{ items: SignalDescriptor[] }> {
  return authorizedRequest('/api/v1/aiops/signals/catalog', token)
}

// ── M40 Topology ────────────────────────────────────────────────────────────

export function getTopologyGraph(
  token: string,
  params: { cluster_id: number; namespace?: string },
): Promise<TopologyGraph> {
  const sp = new URLSearchParams({ cluster_id: String(params.cluster_id) })
  if (params.namespace) sp.set('namespace', params.namespace)
  return authorizedRequest(`/api/v1/aiops/topology/graph?${sp}`, token)
}

export function listTopologyChanges(
  token: string,
  params?: {
    cluster_id?: number
    namespace?: string
    kind?: string
    limit?: number
  },
): Promise<ChangeTimelineResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/topology/changes${q ? `?${q}` : ''}`, token)
}

// ── M41 SLO ─────────────────────────────────────────────────────────────────

export function listSLODefinitions(
  token: string,
  params?: { cluster_id?: number; namespace?: string; enabled?: boolean },
): Promise<SLODefinitionListResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/slos${q ? `?${q}` : ''}`, token)
}

export function listSLITemplates(token: string): Promise<{ items: SLITemplate[]; template_version: string }> {
  return authorizedRequest('/api/v1/aiops/slos/templates', token)
}

export function createSLODefinition(token: string, input: SLOCreateRequest): Promise<SLODefinition> {
  return authorizedRequest('/api/v1/aiops/slos', token, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function getSLODefinition(token: string, id: number): Promise<SLODefinition> {
  return authorizedRequest(`/api/v1/aiops/slos/${id}`, token)
}

export function patchSLODefinition(token: string, id: number, patch: SLOPatchRequest): Promise<SLODefinition> {
  return authorizedRequest(`/api/v1/aiops/slos/${id}`, token, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  })
}

export function deleteSLODefinition(token: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/aiops/slos/${id}`, token, { method: 'DELETE' })
}

export function evaluateSLO(token: string, id: number): Promise<SLOEvaluation> {
  return authorizedRequest(`/api/v1/aiops/slos/${id}/evaluate`, token, { method: 'POST' })
}

export function listSLOEvaluations(
  token: string,
  id: number,
  params?: { limit?: number },
): Promise<SLOEvaluationListResponse> {
  const sp = new URLSearchParams()
  if (params?.limit) sp.set('limit', String(params.limit))
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/slos/${id}/evaluations${q ? `?${q}` : ''}`, token)
}

// M114-1 SLO burn posture (read-only aggregation over definitions + latest evaluations)
export function getSLOBurnSummary(
  token: string,
  params?: { cluster_id?: number; namespace?: string; template?: string; state?: BurnStatus; limit?: number },
): Promise<SLOBurnSummaryResponse> {
  const sp = new URLSearchParams()
  if (params?.cluster_id) sp.set('cluster_id', String(params.cluster_id))
  if (params?.namespace) sp.set('namespace', params.namespace)
  if (params?.template) sp.set('template', params.template)
  if (params?.state) sp.set('state', params.state)
  if (params?.limit) sp.set('limit', String(params.limit))
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/slos/burn-summary${q ? `?${q}` : ''}`, token)
}

// ── M42 Correlation ─────────────────────────────────────────────────────────

export function listCorrelationRules(token: string): Promise<{ items: CorrelationRule[]; correlation_version: string }> {
  return authorizedRequest('/api/v1/aiops/correlation/rules', token)
}

export function listCorrelationCases(
  token: string,
  params?: {
    cluster_id?: number
    namespace?: string
    status?: string
    confidence?: string
    limit?: number
  },
): Promise<CaseListResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/correlation/cases${q ? `?${q}` : ''}`, token)
}

export function listCorrelationTimeline(
  token: string,
  params?: { cluster_id?: number; limit?: number },
): Promise<CaseTimelineResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null) sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/correlation/cases/timeline${q ? `?${q}` : ''}`, token)
}

export function getCorrelationCase(token: string, id: number): Promise<CaseView> {
  return authorizedRequest(`/api/v1/aiops/correlation/cases/${id}`, token)
}

export function getCorrelationCaseGraph(
  token: string,
  id: number,
): Promise<{ items: import('../types/aiops').ResourceLink[]; total: number }> {
  return authorizedRequest(`/api/v1/aiops/correlation/cases/${id}/graph`, token)
}

export function listCorrelationActions(token: string, caseId: number): Promise<ActionCandidateListResponse> {
  return authorizedRequest(`/api/v1/aiops/correlation/cases/${caseId}/actions`, token)
}

// ── M43 AI Investigator ─────────────────────────────────────────────────────

export function listInvestigatorRunbooks(token: string): Promise<{ items: Runbook[]; investigator_version: string }> {
  return authorizedRequest('/api/v1/aiops/investigator/runbooks', token)
}

export function listInvestigations(token: string, caseId: number): Promise<InvestigationListResponse> {
  return authorizedRequest(`/api/v1/aiops/investigator/cases/${caseId}/investigations`, token)
}

export function getInvestigation(token: string, id: number): Promise<Investigation> {
  return authorizedRequest(`/api/v1/aiops/investigator/investigations/${id}`, token)
}

export function generateInvestigation(
  token: string,
  caseId: number,
  params?: { runbook_id?: string; provider?: string },
): Promise<Investigation> {
  const sp = new URLSearchParams()
  if (params?.runbook_id) sp.set('runbook_id', params.runbook_id)
  if (params?.provider) sp.set('provider', params.provider)
  const q = sp.toString()
  return authorizedRequest(
    `/api/v1/aiops/investigator/cases/${caseId}/investigations${q ? `?${q}` : ''}`,
    token,
    { method: 'POST' },
  )
}

// ── M44 Automation ──────────────────────────────────────────────────────────

export function listAutomationRunbooks(token: string): Promise<{ items: Runbook[]; automation_version: string }> {
  return authorizedRequest('/api/v1/aiops/automation/runbooks', token)
}

export function listAutomationPlans(
  token: string,
  params?: { cluster_id?: number; status?: string; limit?: number },
): Promise<ActionPlanListResponse> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/aiops/automation/plans${q ? `?${q}` : ''}`, token)
}

export function createAutomationPlan(token: string, input: CreatePlanRequest): Promise<ActionPlanResponse> {
  return authorizedRequest('/api/v1/aiops/automation/plans', token, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function getAutomationPlan(token: string, planId: string): Promise<ActionPlanResponse> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}`, token)
}

export function previewAutomationPlan(token: string, planId: string): Promise<ActionPlanResponse> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/preview`, token, { method: 'POST' })
}

export function approveAutomationPlan(token: string, planId: string): Promise<ActionPlanResponse> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/approve`, token, { method: 'POST' })
}

export function executeAutomationPlan(token: string, planId: string, input: ExecutePlanRequest): Promise<ActionPlanResponse> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/execute`, token, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function cancelAutomationPlan(token: string, planId: string): Promise<ActionPlanResponse> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/cancel`, token, { method: 'POST' })
}

export function verifyAutomationPlan(token: string, planId: string): Promise<ActionVerification> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/verify`, token, { method: 'POST' })
}

export function getAutomationVerification(token: string, planId: string): Promise<ActionVerification> {
  return authorizedRequest(`/api/v1/aiops/automation/plans/${planId}/verification`, token)
}

// ── M45 / M56 Quality ───────────────────────────────────────────────────────

export function getQualityReport(token: string): Promise<QualityReport> {
  return authorizedRequest('/api/v1/aiops/quality-report', token)
}

export function runQualityReplay(token: string): Promise<{ task_id: string; status: string; message: string }> {
  return authorizedRequest('/api/v1/aiops/quality-report/run', token, { method: 'POST' })
}
