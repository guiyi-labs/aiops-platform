export interface InspectionRule {
  code: string
  schema_version: string
  domain: string
  default_severity: string
  signal_code: string
  description: string
  remediation: string
  timeout_seconds: number
}

export interface InspectionPlanView {
  id: number
  name: string
  creator_id: number
  cluster_ids: number[]
  rule_codes: string[]
  cron_spec: string
  enabled: boolean
  last_run_at?: string
  next_run_at?: string
  created_at: string
  updated_at: string
}

export interface InspectionTaskView {
  id: number
  plan_id?: number
  plan_name_snapshot: string
  triggered_by?: string
  trigger_reason: string
  cluster_ids: number[]
  rule_codes: string[]
  status: string
  started_at?: string
  finished_at?: string
  total_clusters: number
  completed_clusters: number
  finding_count: number
  error_summary: string
  created_at: string
}

export interface InspectionResultView {
  id: number
  task_id: number
  cluster_id: number
  rule_code: string
  signal_code: string
  severity: string
  state: string
  namespace?: string
  resource_kind?: string
  resource_name?: string
  resource_uid?: string
  fingerprint: string
  evidence?: Record<string, unknown>
  observed_at: string
}

export interface CreateInspectionPlanRequest {
  name: string
  cluster_ids: number[]
  rule_codes: string[]
  cron_spec?: string
  enabled: boolean
}

export interface RunInspectionRequest {
  cluster_ids: number[]
  rule_codes: string[]
}
