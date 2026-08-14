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
