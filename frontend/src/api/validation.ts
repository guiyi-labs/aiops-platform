import { authorizedRequest } from './client'
import type { DiagnosisRecord } from '../types/diagnosis'

export interface RuleValidation {
  rule_id: string
  scenarios: number
  true_positives: number
  true_negatives: number
  false_positives: number
  false_negatives: number
  precision: number
  recall: number
  f1: number
}
export interface ScenarioOutcome {
  id: string
  rule_id: string
  kind: string
  note: string
  expected: boolean
  matched: boolean
  passed: boolean
  pipeline_expected: string
  pipeline_actual: string
  pipeline_passed: boolean
  input: Record<string, unknown>
  record?: DiagnosisRecord
}
export interface ValidationReport {
  tool: string
  corpus_version: string
  observed_at: string
  generated_at: string
  total_scenarios: number
  per_rule: RuleValidation[]
  micro_f1: number
  macro_f1: number
  label_accuracy: number
  pipeline: { total_pod_scenarios: number; correct_top1: number; top1_accuracy: number }
  outcomes: ScenarioOutcome[]
}
export function getDiagnosisValidation(token: string) {
  return authorizedRequest<ValidationReport>('/api/v1/diagnoses/validation', token)
}
