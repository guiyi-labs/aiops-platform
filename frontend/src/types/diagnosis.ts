export interface DiagnosisEvidence { type: string; source: string; content: Record<string, unknown> }
export interface DiagnosisActor { id: number; name: string }
export interface DiagnosisActivity { id: number; actor: DiagnosisActor; from_status: DiagnosisStatus; to_status: DiagnosisStatus; comment: string; created_at: string }
export interface DiagnosisFeedback { id: number; actor: DiagnosisActor; verdict: FeedbackVerdict; comment: string; created_at: string }
export interface DiagnosisAssignment { id: number; actor: DiagnosisActor; from_assignee?: DiagnosisActor; to_assignee: DiagnosisActor; comment: string; created_at: string }
export interface DiagnosisAICitation { evidence_id: string; claim: string }
export interface DiagnosisAIAction { action: string; priority: 'high' | 'medium' | 'low'; evidence_ids: string[] }
export type AIExplanationFeedbackVerdict = 'helpful' | 'partially_helpful' | 'not_helpful'
export interface AIExplanationFeedbackSummary { total: number; helpful: number; partially_helpful: number; not_helpful: number; helpful_rate: number }
export interface AIExplanationFeedback { id: number; explanation_id: number; actor: DiagnosisActor; verdict: AIExplanationFeedbackVerdict; comment: string; created_at: string }
export interface AIExplanationFeedbackResult { feedback: AIExplanationFeedback; summary: AIExplanationFeedbackSummary }
export interface AIModelQualitySummary extends Omit<AIExplanationFeedbackSummary, 'total'> { model: string; total_feedback: number }
export interface AIQualitySummary extends Omit<AIExplanationFeedbackSummary, 'total'> {
  total_feedback: number
  explanations_with_feedback: number
  contributors: number
  by_model: AIModelQualitySummary[]
}
export interface DiagnosisAIExplanation {
  id: number
  diagnosis_id: number
  actor: DiagnosisActor
  provider: string
  model: string
  provider_response_id?: string
  summary: string
  analysis: string
  recommended_actions: DiagnosisAIAction[]
  citations: DiagnosisAICitation[]
  input_tokens: number
  output_tokens: number
  feedback_summary: AIExplanationFeedbackSummary
  my_feedback?: AIExplanationFeedback
  created_at: string
}
export interface AIRuntimeStatus {
  enabled: boolean
  available: boolean
  provider: string
  model: string
  max_concurrent_requests: number
  active_requests: number
  max_output_tokens: number
  daily_token_budget: number
  used_tokens_today: number
  reserved_tokens: number
  remaining_tokens?: number
  explanation_count_today: number
  last_success_at?: string
}
export type DiagnosisStatus = 'open' | 'confirmed' | 'resolved' | 'dismissed'
export type FeedbackVerdict = 'accurate' | 'inaccurate' | 'uncertain'

export interface DiagnosisTimelineEntry {
  index: number
  category: 'resource_state' | 'event' | 'log' | 'alert' | 'change' | 'automation'
  type: string
  source: string
  ref: string
  integrity: string
  occurred_at?: string
  missing: boolean
  missing_reason?: string
  summary: string
}
export interface RootCauseCard {
  conclusion: string
  severity: 'info' | 'warning' | 'high' | 'critical'
  status: DiagnosisStatus
  first_observed_at: string
  confidence: string
  confidence_source: string
  resource: { kind: string; namespace?: string; name: string; uid?: string }
  key_evidence_refs: string[]
}
export interface DiagnosisRecord {
  id: number
  cluster_id: number
  rule_id: string
  severity: 'info' | 'warning' | 'high' | 'critical'
  resource: { kind: string; namespace?: string; name: string; uid?: string }
  status: DiagnosisStatus
  summary: string
  root_causes: string[]
  recommendations: string[]
  evidence: DiagnosisEvidence[]
  timeline?: DiagnosisTimelineEntry[]
  root_cause_card?: RootCauseCard
  assignee?: DiagnosisActor
  activities?: DiagnosisActivity[]
  feedback?: DiagnosisFeedback[]
  assignments?: DiagnosisAssignment[]
  observed_at: string
  sla_due_at: string
  resolved_at?: string
  overdue: boolean
  created_at: string
  updated_at: string
}
export interface DiagnosisSummary { total: number; open: number; confirmed: number; resolved: number; dismissed: number; overdue: number; recent: DiagnosisRecord[] }
export interface DiagnoseNodeMetricsRequest {
  name: string
  metric: 'node_cpu' | 'node_memory'
  operator: 'gte' | 'lte'
  threshold: number
  for_seconds: number
  minimum_points?: number
}
export type RemediationAction = 'deployment.rollout_restart' | 'deployment.scale' | 'deployment.image_update' | 'deployment.rollback' | 'cronjob.suspend' | 'cronjob.resume'
export type RemediationStatus = 'awaiting_confirmation' | 'executing' | 'succeeded' | 'failed' | 'expired'
export type ControlledOperationRequest =
  | { action: 'deployment.scale'; namespace: string; target_name: string; desired_replicas: number }
  | { action: 'cronjob.suspend' | 'cronjob.resume'; namespace: string; target_name: string }
  | { action: 'deployment.image_update'; namespace: string; target_name: string; container_name: string; desired_image: string }
  | { action: 'deployment.rollback'; namespace: string; target_name: string; rollback_revision: number }
export interface RemediationPlan {
  id: string
  diagnosis_id?: number
  cluster_id: number
  action: RemediationAction
  status: RemediationStatus
  target: { kind: string; namespace: string; name: string; uid: string; resource_version: string }
  restart_at?: string
  parameters: { desired_replicas?: number; desired_suspended?: boolean; container_name?: string; before_image?: string; desired_image?: string; rollback_revision?: number }
  change?: { field: string; before: string | number | boolean; after: string | number | boolean }
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  requested_by: DiagnosisActor
  confirmation_token?: string
}
export interface RolloutRevision {
  revision: number
  replicaset_name: string
  uid: string
  resource_version: string
  created_at: string
  replicas: number
  ready_replicas: number
  available_replicas: number
  images: string[]
  current: boolean
}
export interface RolloutHistory {
  deployment: string
  namespace: string
  current_revision: number
  revisions: RolloutRevision[]
}
export interface RolloutStatus {
  deployment: string
  namespace: string
  current_revision: number
  desired_replicas: number
  updated_replicas: number
  ready_replicas: number
  available_replicas: number
  unavailable_replicas: number
  phase: string
  reason?: string
  message?: string
  conditions?: { type: string; status: string; reason?: string; message?: string; lastTransitionTime?: string }[]
}
