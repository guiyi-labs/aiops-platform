export type MetricResourceKind = 'Node' | 'Pod'
export type MetricName = 'cpu' | 'memory'
export type MetricUnit = 'nanocores' | 'bytes'
export type MetricHistoryRangeHours = 1 | 6 | 24 | 168 | 720

export interface MetricHistoryQuery {
  resourceKind: MetricResourceKind
  name: string
  namespace?: string
  container?: string
  metric: MetricName
  from: string
  to: string
  limit?: number
}

export interface MetricHistorySeries {
  cluster_id: number
  resource_kind: MetricResourceKind
  resource_namespace?: string
  resource_name: string
  container_name?: string
  metric_name: MetricName
  unit: MetricUnit
}

export interface MetricHistoryPoint {
  value: number
  source_timestamp: string
  window_milliseconds: number
  collected_at: string
}

export interface MetricHistoryCoverage {
  collections: number
  succeeded: number
  partial: number
  unavailable: number
  timed_out: number
  failed: number
  points: number
  missing: number
}

export interface MetricHistoryLimits {
  max_window_seconds: 86400 | 2592000
  max_points: 1440
}

export interface MetricHistoryResponse {
  series: MetricHistorySeries
  from: string
  to: string
  points: MetricHistoryPoint[]
  coverage: MetricHistoryCoverage
  limits: MetricHistoryLimits
  truncated: boolean
}

export type MetricEvaluationOperator = 'gte' | 'lte'
export type MetricEvaluationState = 'firing' | 'normal' | 'insufficient_data'

export interface MetricHistoryEvaluationQuery extends Omit<MetricHistoryQuery, 'limit'> {
  operator: MetricEvaluationOperator
  threshold: number
  forSeconds: number
  minimumPoints?: number
}

export interface MetricHistoryEvaluationResponse {
  series: MetricHistorySeries
  from: string
  to: string
  coverage: MetricHistoryCoverage
  state: MetricEvaluationState
  operator: MetricEvaluationOperator
  threshold: number
  for_seconds: number
  minimum_points: number
  points_evaluated: number
  breaching_points: number
  observed_span_seconds: number
  sustained_windows: MetricSustainedWindow[]
  latest_firing_window: MetricSustainedWindow | null
}

export interface MetricSustainedWindow {
  start_collected_at: string
  end_collected_at: string
  breaching_points: number
  span_seconds: number
}
