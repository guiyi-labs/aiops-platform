export type IncidentStatus = 'open' | 'confirmed' | 'resolved' | 'dismissed'
export type IncidentSeverity = 'info' | 'warning' | 'high' | 'critical'
export type IncidentSourceType = 'diagnosis' | 'finding'
export type IncidentEventType = 'system' | 'note'

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

export interface IncidentSummary {
  total: number
  open: number
  confirmed: number
  resolved: number
  dismissed: number
  overdue: number
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
