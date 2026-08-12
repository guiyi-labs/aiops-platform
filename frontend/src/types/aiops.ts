// AIOps TypeScript types — covers M39 (signal), M40 (topology), M41 (SLO),
// M42 (correlation), M43 (investigator), M44 (automation), M45/M56 (quality).
// Field names mirror the backend Go struct json tags exactly.

// ── Shared ──────────────────────────────────────────────────────────────────

export interface ResourceCitation {
  cluster_id: number
  namespace: string
  kind: string
  name: string
  uid?: string
  resource_version?: string
}

export interface EvidenceRef {
  kind: string
  id: number
  content_hash?: string
}

export interface ActorRef {
  user_id: number
  name: string
  roles?: string[]
}

// ── M39 Signal Model ────────────────────────────────────────────────────────

export type SignalProducer =
  | 'diagnosis_engine'
  | 'alert_router'
  | 'inspection'
  | 'slo_evaluator'
  | 'topology_change'
  | 'service_mesh'
  | 'external'

export type SignalSeverity = 'critical' | 'warning' | 'info'
export type SignalState = 'active' | 'resolved' | 'stale' | 'suppressed'
// complete | partial | missing | unavailable | truncated — 'unavailable' means
// no samples at all (fail-closed, never treated as healthy); 'truncated' means
// the source hit its sampling budget.
export type SignalCoverage = 'complete' | 'partial' | 'missing' | 'unavailable' | 'truncated'

export interface SignalOccurrence {
  id: number
  signal_id: string
  signal_code: string
  schema_version: string
  producer: SignalProducer
  cluster_id: number
  namespace: string
  resource: ResourceCitation
  severity: SignalSeverity
  state: SignalState
  fingerprint: string
  coverage: SignalCoverage
  freshness: string
  window_start: string
  window_end: string
  observed_at: string
  ingested_at: string
  expires_at?: string
  attributes?: Record<string, unknown>
  evidence?: EvidenceRef[]
  ingestion_run_id?: string
}

export interface SignalDescriptor {
  code: string
  schema_version: string
  domain: string
  severity_policy: string
  correlation_dims: string[]
  required_evidence: string[]
  allowed_actions: string[]
  retention: string
  description: string
}

export interface SignalCoverageEntry {
  producer: SignalProducer
  coverage: SignalCoverage
  last_seen?: string
  gap_reason?: string
}

export interface OverviewSignal {
  signal_code: string
  severity: SignalSeverity
  count: number
  latest_occurrence: string
  cluster_id: number
  namespace: string
}

export interface OverviewChange {
  change_event_id: number
  kind: string
  target: ResourceCitation
  result: string
  started_at: string
}

export interface OverviewOutcomes {
  succeeded: number
  failed: number
  pending: number
}

export interface SignalOverview {
  source_completeness: Record<string, SignalCoverageEntry>
  active_diagnoses: number
  top_signals: OverviewSignal[]
  recent_changes: OverviewChange[]
  action_outcomes: OverviewOutcomes
  generated_at: string
  partial: boolean
}

export interface SignalListResponse {
  items: SignalOccurrence[]
  total: number
  truncated: boolean
  next_cursor?: string
}

// ── M40 Topology ────────────────────────────────────────────────────────────

export type EdgeKind =
  | 'Owns'
  | 'Selects'
  | 'RoutesTo'
  | 'BackedBy'
  | 'RunsOn'
  | 'Mounts'
  | 'Scales'
  | 'ProtectedBy'

export interface DerivationMethod {
  strategy: string
  evidence?: EvidenceRef[]
}

export interface TopologyEdge {
  id: number
  cluster_id: number
  kind: EdgeKind
  source: ResourceCitation
  target: ResourceCitation
  derivation: DerivationMethod
  first_observed_at: string
  last_observed_at: string
  valid_from: string
  valid_to?: string
  review_evidence?: EvidenceRef[]
  source_hash?: string
}

export interface TopologyNode {
  resource: ResourceCitation
  edge_count: number
}

export interface GraphCompleteness {
  coverage: SignalCoverage
  missing_producers?: string[]
  stale_edges: number
}

export interface TopologyGraph {
  cluster_id: number
  namespace: string
  nodes: TopologyNode[]
  edges: TopologyEdge[]
  completeness: GraphCompleteness
  generated_at: string
}

export interface ChangeEvent {
  id: number
  cluster_id: number
  namespace: string
  kind: string
  plan_id?: string
  target: ResourceCitation
  action: string
  safe_diff_hash?: string
  revision?: string
  actor: string
  started_at: string
  finished_at?: string
  result: string
  audit_id?: number
  request_id?: string
  evidence?: EvidenceRef[]
  confidence?: string
  source: string
}

export interface ChangeTimelineResponse {
  items: ChangeEvent[]
  total: number
  truncated: boolean
}

// ── M41 SLO ─────────────────────────────────────────────────────────────────

export type EvaluationState =
  | 'healthy'
  | 'burning_slow'
  | 'burning_fast'
  | 'breached'
  | 'unavailable'

export interface ServiceRef {
  cluster_id: number
  namespace: string
  name: string
  kind: string
}

export interface SLODefinition {
  id: number
  cluster_id: number
  service: ServiceRef
  template: string
  template_version: string
  objective: number
  rolling_window_seconds: number
  missing_data_policy: string
  latency_threshold_ms?: number
  owner?: ActorRef
  fast_burn_rate: number
  fast_burn_window_seconds: number
  slow_burn_rate: number
  slow_burn_window_seconds: number
  enabled: boolean
  version: number
  creator?: ActorRef
  created_at: string
  updated_at: string
}

export interface SLOEvaluation {
  id: number
  slo_id: number
  version: number
  window_start: string
  window_end: string
  good_events: number
  total_events: number
  ratio: number
  target_ratio: number
  error_budget: number
  remaining_budget: number
  burn_rate: number
  state: EvaluationState
  coverage: SignalCoverage
  evaluated_at: string
}

export interface SLODefinitionListResponse {
  items: SLODefinition[]
  total: number
  truncated: boolean
}

export interface SLOEvaluationListResponse {
  items: SLOEvaluation[]
  total: number
  truncated: boolean
}

export interface SLITemplate {
  code: string
  display_name: string
  metric: string
  direction: 'higher_is_better' | 'lower_is_better'
  unit: string
  default_objective: number
  description: string
}

export interface SLOCreateRequest {
  cluster_id: number
  service: ServiceRef
  template: string
  objective: number
  rolling_window_seconds: number
  missing_data_policy?: string
  latency_threshold_ms?: number
  fast_burn_rate?: number
  fast_burn_window_seconds?: number
  slow_burn_rate?: number
  slow_burn_window_seconds?: number
}

export interface SLOPatchRequest {
  objective?: number
  rolling_window_seconds?: number
  enabled?: boolean
  missing_data_policy?: string
  latency_threshold_ms?: number
}

// ── M42 Correlation ─────────────────────────────────────────────────────────

export type CaseStatus = 'active' | 'resolved' | 'stale'
export type CaseConfidence = 'confirmed' | 'candidate' | 'contradicted' | 'unknown'

export interface Factor {
  dimension: string
  weight: number
  evidence_refs: EvidenceRef[]
  reason: string
}

export interface CorrelationCase {
  id: number
  case_key: string
  cluster_id: number
  rule_id: string
  correlation_version: string
  primary_resource: ResourceCitation
  status: CaseStatus
  confidence: CaseConfidence
  evidence_completeness: number
  factors: Factor[]
  first_observed_at: string
  last_observed_at: string
  diagnosis_ids: number[]
  root_change_candidate_id?: number
  created_at: string
  updated_at: string
}

export interface SignalLink {
  id: number
  case_id: number
  signal_occurrence_id: number
  relation: string
  signal_id: string
  producer: string
  observed_at: string
  coverage?: SignalCoverage
  freshness?: string
  window_start?: string
  window_end?: string
  created_at: string
}

export interface ResourceLink {
  id: number
  case_id: number
  resource: ResourceCitation
  relation: string
  topology_path: string[]
  edge_ids: number[]
  created_at: string
}

export interface ChangeCandidate {
  id: number
  case_id: number
  change_event_id: number
  rule_id: string
  confidence: CaseConfidence
  rank: number
  factors: Factor[]
  evidence_refs: EvidenceRef[]
  contradicting_refs: EvidenceRef[]
  reason_code: string
  created_at: string
  updated_at: string
}

export interface CaseView {
  case: CorrelationCase
  signal_links: SignalLink[]
  resource_links: ResourceLink[]
  change_candidates: ChangeCandidate[]
  generated_at: string
}

export interface CaseListResponse {
  items: CorrelationCase[]
  total: number
  truncated: boolean
}

export interface CaseTimelineResponse {
  items: CorrelationCase[]
  total: number
  truncated: boolean
}

export interface CorrelationRule {
  id: string
  version: string
  signal_codes: string[]
  dimensions: string[]
  min_evidence: number
  retention: string
  description: string
}

export interface ActionCandidate {
  code: string
  target: ResourceCitation
  eligible: boolean
  blocked_reasons: string[]
  source_case_id: number
  source_candidate_id?: number
}

export interface ActionCandidateListResponse {
  items: ActionCandidate[]
  total: number
}

// ── M43 AI Investigator ─────────────────────────────────────────────────────

export type InvestigationStatus = 'completed' | 'failed' | 'stale'
export type HypothesisConfidence = 'high' | 'medium' | 'low'
export type EvidenceKind =
  | 'signal_occurrence'
  | 'topology_edge'
  | 'change_event'
  | 'diagnosis_record'
  | 'slo_evaluation'
  | 'correlation_case'
  | 'change_candidate'

export interface InvestigationEvidenceRef {
  kind: EvidenceKind
  id: number
  content_hash?: string
}

export interface Hypothesis {
  claim: string
  confidence: HypothesisConfidence
  evidence_ids: InvestigationEvidenceRef[]
  disconfirming_evidence: InvestigationEvidenceRef[]
  next_checks: string[]
}

export interface InvestigationCitation {
  evidence_ref: InvestigationEvidenceRef
  claim: string
}

export interface Runbook {
  id: string
  display_name: string
  action_codes: string[]
  level: string
  description: string
  prerequisites: string[]
  rollback_strategy: string
}

export interface Investigation {
  id: number
  case_id: number
  investigation_key: string
  investigator_version: string
  actor: ActorRef
  provider: string
  model: string
  provider_response_id?: string
  status: InvestigationStatus
  summary: string
  impact: string
  hypotheses: Hypothesis[]
  recommended_runbook_id?: string
  uncertainties: string[]
  citations: InvestigationCitation[]
  input_tokens: number
  output_tokens: number
  failure_reason?: string
  created_at: string
}

export interface InvestigationListResponse {
  items: Investigation[]
  total: number
  truncated: boolean
}

// ── M44 Automation ──────────────────────────────────────────────────────────

export type PlanStatus =
  | 'draft'
  | 'previewed'
  | 'approved'
  | 'executing'
  | 'succeeded'
  | 'failed'
  | 'expired'
  | 'cancelled'
  | 'verified'

export type PlanLevel = 'L0' | 'L1' | 'L2' | 'L3'
export type ApprovalType = 'single' | 'four_eyes'
export type GateStatus = 'passed' | 'failed' | 'skipped'
export type GateCode =
  | 'uid_rv_recheck'
  | 'scope'
  | 'pdb_blast_radius'
  | 'slo_burn'
  | 'freeze_window'
  | 'concurrent_plans'
  | 'attempt_cap'
  | 'rollback_point'

export type VerificationStatus =
  | 'pending'
  | 'effective'
  | 'ineffective'
  | 'failed'
  | 'unknown'

export type EvidenceComparison =
  | 'improved'
  | 'unchanged'
  | 'worse'
  | 'insufficient'

export interface PolicyGate {
  code: GateCode
  status: GateStatus
  reason: string
  checked_at: string
  rechecked: boolean
}

export interface TargetRef {
  cluster_id: number
  namespace: string
  kind: string
  name: string
}

export interface OperationParameters {
  [key: string]: unknown
}

export interface OperationChange {
  kind: string
  target: ResourceCitation
  diff_summary?: string
  safe: boolean
}

export interface ActionPlan {
  id: string
  plan_key: string
  automation_version: string
  case_id?: number
  investigation_id?: number
  action_candidate_id?: number
  runbook_id?: string
  action_code: string
  cluster_id: number
  policy_gates: PolicyGate[]
  status: PlanStatus
  level: PlanLevel
  approval_type: ApprovalType
  approved_at?: string
  expires_at?: string
  executed_at?: string
  attempt_count: number
  last_error?: string
  verification_id?: string
  correlation_request_id?: string
  created_at: string
  updated_at: string
}

export interface ActionPlanResponse extends ActionPlan {
  target: TargetRef
  requested_by: ActorRef
  approver?: ActorRef
  parameters: OperationParameters
  change?: OperationChange
  confirmation_token?: string
}

export interface ActionPlanListResponse {
  items: ActionPlanResponse[]
  total: number
  truncated: boolean
}

export interface EvidenceSnapshot {
  signals: string[]
  slo_state?: string
  topology_version?: string
  captured_at: string
}

export interface ActionVerification {
  id: string
  plan_id: string
  verification_key: string
  verifier_version: string
  status: VerificationStatus
  evidence_comparison: EvidenceComparison
  pre_snapshot: EvidenceSnapshot
  post_snapshot: EvidenceSnapshot
  slo_evaluation_before_id?: number
  slo_evaluation_after_id?: number
  missing_evidence: boolean
  cooldown_seconds: number
  verified_at?: string
  reason?: string
  rollback_triggered: boolean
  rollback_plan_id?: string
  created_at: string
  updated_at: string
}

export interface CreatePlanRequest {
  case_id?: number
  investigation_id?: number
  action_candidate_id?: number
  runbook_id?: string
  action_code: string
  cluster_id: number
  target: TargetRef
  parameters?: OperationParameters
  approval_type?: ApprovalType
}

export interface ExecutePlanRequest {
  confirmation_token: string
  idempotency_key?: string
}

// ── M45 / M56 Quality Report ────────────────────────────────────────────────

export interface EngineVersions {
  signal_version: string
  topology_version: string
  slo_version: string
  correlation_version: string
  investigator_version: string
  automation_version: string
  verifier_version: string
}

export type ScenarioDelta =
  | 'preserved'
  | 'improved'
  | 'regressed'
  | 'unchanged'

export interface ScenarioQuality {
  scenario_id: string
  passed_before: boolean
  passed_after: boolean
  delta: ScenarioDelta
  steps_passed_before: number
  steps_passed_after: number
  steps_total: number
  notes: string
}

export interface QualitySummary {
  total_scenarios: number
  passed_before: number
  passed_after: number
  improved: number
  regressed: number
  preserved: number
  unchanged: number
  total_steps_before: number
  total_steps_after: number
  total_steps: number
}

export interface QualityReport {
  report_version: string
  dataset_version_before: string
  dataset_version_after: string
  engine_versions_before: EngineVersions
  engine_versions_after: EngineVersions
  scenario_results: ScenarioQuality[]
  summary: QualitySummary
  generated_at: string
  changed_components: string[]
  reviewer?: string
  approved: boolean
}
