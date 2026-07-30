import { authorizedRequest } from './client'
import type { MetricHistoryEvaluationQuery, MetricHistoryEvaluationResponse, MetricHistoryQuery, MetricHistoryResponse } from '../types/metrics-history'

function historyQueryString(input: MetricHistoryQuery): string {
  const query = new URLSearchParams({
    resource_kind: input.resourceKind,
    name: input.name,
    metric: input.metric,
    from: input.from,
    to: input.to,
    limit: String(input.limit ?? 1440),
  })
  if (input.namespace) query.set('namespace', input.namespace)
  if (input.container) query.set('container', input.container)
  return query.toString()
}

export function getMetricHistory(token: string, clusterID: number, input: MetricHistoryQuery): Promise<MetricHistoryResponse> {
  return authorizedRequest(`/api/v1/clusters/${clusterID}/metrics/history?${historyQueryString(input)}`, token)
}

export function evaluateMetricHistory(token: string, clusterID: number, input: MetricHistoryEvaluationQuery): Promise<MetricHistoryEvaluationResponse> {
  const query = new URLSearchParams(historyQueryString(input))
  query.delete('limit')
  query.set('operator', input.operator)
  query.set('threshold', String(input.threshold))
  query.set('for_seconds', String(input.forSeconds))
  query.set('minimum_points', String(input.minimumPoints ?? 2))
  return authorizedRequest(`/api/v1/clusters/${clusterID}/metrics/history/evaluate?${query.toString()}`, token)
}
