// M81 closed-loop insight types. Field names mirror the Go json tags in
// backend/internal/insight (runbook.go).
export interface InsightDiagnosisRoute {
  resource_kind: string
  rule_ids: string[]
  summary: string
}

export interface InsightInspectionRule {
  rule_code: string
  signal_code: string
  summary: string
}

export interface InsightAIExplanation {
  endpoint: string
  summary: string
}

export interface InsightOperationCandidate {
  action: string
  target_kind: string
  dry_run_first: boolean
  summary: string
}

export interface InsightRunbook {
  cluster_id: number
  domain: string
  finding_code?: string
  kind: string
  namespace?: string
  name: string
  diagnoses: InsightDiagnosisRoute[]
  inspection: InsightInspectionRule[]
  ai_explanation?: InsightAIExplanation
  operations: InsightOperationCandidate[]
  read_only: boolean
}