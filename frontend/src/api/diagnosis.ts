import { authorizedRequest } from './client'
import type { AIExplanationFeedbackResult, AIExplanationFeedbackVerdict, AICoverage, AIQualitySummary, AIRuntimeStatus, ControlledOperationRequest, DiagnosisAIExplanation, DiagnosisRecord, DiagnosisReplayView, DiagnosisStatus, DiagnosisSummary, DiagnoseNodeMetricsRequest, FeedbackVerdict, RemediationAction, RemediationPlan, RolloutHistory, RolloutStatus } from '../types/diagnosis'

export function diagnosePod(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'Pod', namespace, name }),
  })
}

export function diagnoseService(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'Service', namespace, name }),
  })
}

export function diagnoseNode(token: string, clusterID: number, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'Node', namespace: '', name }),
  })
}

export function diagnoseNodeMetrics(token: string, clusterID: number, request: DiagnoseNodeMetricsRequest): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses/node_metrics`, token, {
    method: 'POST', body: JSON.stringify(request),
  })
}

export function diagnoseDeployment(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'Deployment', namespace, name }),
  })
}

export function diagnoseIngress(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'Ingress', namespace, name }),
  })
}

export function diagnosePersistentVolumeClaim(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'PersistentVolumeClaim', namespace, name }),
  })
}

export function diagnoseHorizontalPodAutoscaler(token: string, clusterID: number, namespace: string, name: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/diagnoses`, token, {
    method: 'POST', body: JSON.stringify({ resource_kind: 'HorizontalPodAutoscaler', namespace, name }),
  })
}

export function listDiagnoses(token: string, filters: { clusterID?: number; status?: DiagnosisStatus | ''; overdue?: boolean; limit?: number } = {}): Promise<{ items: DiagnosisRecord[]; total: number; remaining: number }> {
  const query = new URLSearchParams({ limit: String(filters.limit ?? 50) })
  if (filters.clusterID) query.set('cluster_id', String(filters.clusterID))
  if (filters.status) query.set('status', filters.status)
  if (filters.overdue !== undefined) query.set('overdue', String(filters.overdue))
  return authorizedRequest(`/api/v1/diagnoses?${query}`, token)
}

export function getDiagnosis(token: string, diagnosisID: number): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}`, token)
}

export function transitionDiagnosis(token: string, diagnosisID: number, status: DiagnosisStatus, comment: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}`, token, {
    method: 'PATCH', body: JSON.stringify({ status, comment }),
  })
}

export function addDiagnosisFeedback(token: string, diagnosisID: number, verdict: FeedbackVerdict, comment: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/feedback`, token, {
    method: 'POST', body: JSON.stringify({ verdict, comment }),
  })
}

export function getDiagnosisSummary(token: string): Promise<DiagnosisSummary> {
  return authorizedRequest('/api/v1/diagnoses/summary', token)
}

export function assignDiagnosis(token: string, diagnosisID: number, assigneeUserID: number, comment: string): Promise<DiagnosisRecord> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/assignment`, token, {
    method: 'PATCH', body: JSON.stringify({ assignee_user_id: assigneeUserID, comment }),
  })
}

export function listRemediationPlans(token: string, diagnosisID: number): Promise<{ items: RemediationPlan[]; total: number; remaining: number }> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/remediations`, token)
}

export function previewRemediation(token: string, diagnosisID: number, action: RemediationAction, targetName: string): Promise<RemediationPlan> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/remediations/preview`, token, {
    method: 'POST', body: JSON.stringify({ action, target_name: targetName }),
  })
}

export function executeRemediation(token: string, remediationID: string, confirmationToken: string, idempotencyKey: string): Promise<RemediationPlan> {
  return authorizedRequest(`/api/v1/remediations/${remediationID}/execute`, token, {
    method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ confirmation_token: confirmationToken }),
  })
}

export function previewControlledOperation(token: string, clusterID: number, request: ControlledOperationRequest): Promise<RemediationPlan> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/operations/preview`, token, {
    method: 'POST', body: JSON.stringify(request),
  })
}

export function listControlledOperations(token: string, clusterID: number, namespace: string, targetKind: 'Deployment' | 'CronJob', targetName: string): Promise<{ items: RemediationPlan[]; total: number; remaining: number }> {
  const query = new URLSearchParams({ namespace, target_kind: targetKind, target_name: targetName })
  return authorizedRequest(`/api/v1/clusters/${clusterID}/operations?${query}`, token)
}

export function getRolloutHistory(token: string, clusterID: number, namespace: string, name: string): Promise<RolloutHistory> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/deployments/${namespace}/${name}/rollout/history`, token)
}

export function getRolloutStatus(token: string, clusterID: number, namespace: string, name: string): Promise<RolloutStatus> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/deployments/${namespace}/${name}/rollout/status`, token)
}

export function listDiagnosisExplanations(token: string, diagnosisID: number): Promise<{ items: DiagnosisAIExplanation[]; total: number; remaining: number }> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/explanations`, token)
}

export function generateDiagnosisExplanation(token: string, diagnosisID: number): Promise<DiagnosisAIExplanation> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/explanations`, token, { method: 'POST' })
}

export function getAIRuntimeStatus(token: string): Promise<AIRuntimeStatus> {
  return authorizedRequest('/api/v1/ai/status', token)
}

export function getAIQualitySummary(token: string): Promise<AIQualitySummary> {
  return authorizedRequest('/api/v1/ai/quality', token)
}

export function getAICoverage(token: string): Promise<AICoverage> {
  return authorizedRequest('/api/v1/ai/coverage', token)
}

export function addAIExplanationFeedback(token: string, explanationID: number, verdict: AIExplanationFeedbackVerdict, comment: string): Promise<AIExplanationFeedbackResult> {
  return authorizedRequest(`/api/v1/ai/explanations/${explanationID}/feedback`, token, {
    method: 'POST', body: JSON.stringify({ verdict, comment }),
  })
}

export function getDiagnosisReplay(token: string, diagnosisID: number): Promise<DiagnosisReplayView> {
  return authorizedRequest(`/api/v1/diagnoses/${diagnosisID}/replay`, token)
}
