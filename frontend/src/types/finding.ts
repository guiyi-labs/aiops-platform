export type FindingSeverity = 'info' | 'warning' | 'critical'

export type FindingRecommendationKind = 'advisory' | 'controlled_action_available' | 'manual_only'

export type FindingEvidenceKind = 'resource_state' | 'event' | 'log' | 'alert' | 'change' | 'automation'

export interface FindingResource {
  kind: string
  namespace?: string
  name: string
  uid?: string
  resource_version?: string
}

export interface FindingRuleIdentity {
  rule_id: string
  framework?: string
  source?: string
  version?: string
}

export interface FindingEvidenceRef {
  id: string
  kind: FindingEvidenceKind
  summary?: string
  observed_at?: string
  missing?: boolean
  missing_reason?: string
  source?: string
}

export interface FindingRecommendation {
  kind: FindingRecommendationKind
  text: string
  capability?: string
}

export interface FindingDetailV2 {
  schema_version: string
  rule: FindingRuleIdentity
  code: string
  severity: string
  summary: string
  resource: FindingResource
  details?: Record<string, string>
  observed_at: string
  evidence: FindingEvidenceRef[]
  recommendations: FindingRecommendation[]
  origin_ids: string[]
}
