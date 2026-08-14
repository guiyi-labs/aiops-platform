export type IncidentStatus = 'open' | 'confirmed' | 'resolved' | 'dismissed'
export type IncidentSeverity = 'info' | 'warning' | 'high' | 'critical'
export type IncidentSourceType = 'diagnosis' | 'finding' | 'alert' | 'inspection' | 'signal' | 'correlation'
export type IncidentEventType = 'system' | 'note'

export type IncidentRunbookUnavailableReason = 'source_resolver_unavailable' | 'source_unavailable' | 'domain_unavailable'

export interface IncidentActor { id: number; name: string }
export interface IncidentResourceRef { kind: string; namespace?: string; name: string; uid?: string }
export interface IncidentTimelineEvent { id: number; event_type: IncidentEventType; actor: IncidentActor; content: string; created_at: string }
export interface IncidentFollower { user_id: number; name: string; added_at: string }

export interface Incident {
  id: number
  number: string
  title: string
  source_type: IncidentSourceType
  source_ref: string
  template_id?: string
  cluster_id: number
  resource: IncidentResourceRef
  severity: IncidentSeverity
  status: IncidentStatus
  summary: string
  postmortem?: string
  assignee?: IncidentActor
  followers?: IncidentFollower[]
  timeline?: IncidentTimelineEvent[]
  version: number
  observed_at: string
  sla_due_at: string
  resolved_at?: string
  overdue: boolean
  created_at: string
  updated_at: string
}

export interface IncidentAssignFailure {
  incident_id: number
  error: string
}

export interface IncidentBatchAssignResult {
  total: number
  assigned: number
  failed?: IncidentAssignFailure[]
}

export interface IncidentSummary {
  total: number
  open: number
  confirmed: number
  resolved: number
  dismissed: number
  overdue: number
}

export interface IncidentMetrics {
  window_days: number
  cluster_id: number
  sample_limit: number
  sampled: number
  truncated: boolean
  assigned: number
  acknowledged: number
  resolved: number
  overdue: number
  sla_evaluated: number
  sla_compliant: number
  sla_compliance_rate: number | null
  first_assigned_seconds: number | null
  mtta_seconds: number | null
  mttr_seconds: number | null
}

export interface IncidentEvidenceField { label: string; value?: string }

export interface IncidentEvidenceItem {
  source_type: IncidentSourceType
  source_ref: string
  title: string
  summary?: string
  severity?: IncidentSeverity
  resource?: IncidentResourceRef
  observed_at?: string
  deep_link: string
  fields?: IncidentEvidenceField[]
}

export interface IncidentCreateInput {
  source_type: IncidentSourceType
  source_ref: string
  template_id?: string
  cluster_id: number
  title?: string
  severity?: IncidentSeverity
  summary?: string
  observed_at?: string
  resource?: IncidentResourceRef
}

export interface IncidentListResponse { items: Incident[]; total: number; remaining: number }

export interface IncidentRunbookResponse {
  incident_id: number
  available: boolean
  reason?: IncidentRunbookUnavailableReason
  domain?: string
  finding_code?: string
  runbook?: import('../api/insight').InsightRunbook
}

export interface IncidentSeverityTarget { severity: IncidentSeverity; target_minutes: number }
export interface IncidentResponseTemplate {
  id: string
  name: string
  description: string
  source_types: IncidentSourceType[]
  default_title: string
  default_severity: IncidentSeverity
  default_summary: string
  steps: string[]
}
export interface IncidentResponseCatalog {
  templates: IncidentResponseTemplate[]
  severity_matrix: IncidentSeverityTarget[]
}

// --- M112-1 context cockpit ---

export interface IncidentResourceScope {
  cluster_id: number
  namespace?: string
  kind?: string
  name?: string
  source_type?: string
}

export interface IncidentFreshnessInfo {
  age_seconds: number
  as_of: string
}

export interface IncidentEmptySampleInfo {
  count: number
  bounded: boolean
  semantic: 'fail_closed' | 'safe_absent'
}

export interface IncidentResourceContext {
  scope: IncidentResourceScope
  observed_at: string
  source: string
  freshness: IncidentFreshnessInfo
  empty_sample: IncidentEmptySampleInfo
}

export interface IncidentHealthSummary {
  status: IncidentStatus
  overdue: boolean
  evidence_available: boolean
  runbook_available: boolean
  note_count: number
  system_event_count: number
}

export interface IncidentEvidenceSourceSummary {
  source_type: IncidentSourceType
  count: number
  deep_link: string
}

export interface IncidentSLASummary {
  due_at: string
  overdue: boolean
  remaining: string
  deadline_text: string
}

export interface IncidentRunbookBrief {
  domain: string
  finding_code: string
  diagnosis_routes: number
  inspection_rules: number
  operation_count: number
}

export interface IncidentRecommendedAction {
  action: string
  target_kind: string
  dry_run_first: boolean
  summary: string
}

export interface IncidentContextCockpit {
  resource_context: IncidentResourceContext
  incident: {
    id: number
    number: string
    title: string
    severity: IncidentSeverity
    status: IncidentStatus
    summary: string
    source_type: IncidentSourceType
    resource: IncidentResourceRef
    version: number
    created_at: string
    updated_at: string
  }
  sla: IncidentSLASummary
  health: IncidentHealthSummary
  evidence_sources: IncidentEvidenceSourceSummary[]
  recent_events: IncidentTimelineEvent[]
  runbook_brief?: IncidentRunbookBrief
  recommended_actions: IncidentRecommendedAction[]
}

// --- M112-2 incident AI chat ---

export interface IncidentChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface IncidentChatCitation {
  evidence_id: string
  claim: string
}

export interface IncidentChatResponse {
  incident_id: number
  resource_context: IncidentResourceContext
  mode: 'ai' | 'deterministic'
  answer: string
  next_checks?: string[]
  citations: IncidentChatCitation[]
  provider: string
  model: string
  input_tokens: number
  output_tokens: number
  fail_closed: boolean
}
