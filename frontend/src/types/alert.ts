export interface AlertActorRef {
  id: number
  name: string
}

export interface AlertRule {
  id: number
  cluster_id: number
  display_name: string
  resource_kind: 'Node'
  resource_name: string
  metric_name: 'cpu' | 'memory'
  operator: 'gte' | 'lte'
  threshold: number
  for_seconds: number
  minimum_points: number
  enabled: boolean
  deleted: boolean
  last_evaluation_state: '' | 'firing' | 'normal' | 'insufficient_data' | 'error'
  last_evaluation_at?: string
  last_error_code: string
  next_due_at: string
  creator: AlertActorRef
  created_at: string
  updated_at: string
}

export interface AlertRuleCreate {
  display_name: string
  resource_kind: 'Node'
  resource_name: string
  metric_name: 'cpu' | 'memory'
  operator: 'gte' | 'lte'
  threshold: number
  for_seconds: number
  minimum_points: number
}

export interface AlertRulePatch {
  display_name?: string
  enabled?: boolean
}

export type AlertInstanceState = 'firing' | 'resolved'

export interface AlertInstance {
  id: number
  rule_id: number
  diagnosis_id: number
  state: AlertInstanceState
  first_fired_at: string
  last_fired_at: string
  resolved_at?: string
  latest_evidence_anchor: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface AlertOverviewGroup {
  rule_id: number
  display_name: string
  resource_kind: string
  resource_name: string
  metric_name: string
  firing_count: number
  resolved_count: number
  first_fired_at: string
  last_fired_at: string
  related_case_ids?: number[]
}

export interface AlertOverviewResponse {
  scope: string
  observed_at: string
  window_minutes: number
  groups_total: number
  groups: AlertOverviewGroup[]
  total_firing: number
  total_resolved: number
  fail_closed: boolean
  empty_note?: string
}
