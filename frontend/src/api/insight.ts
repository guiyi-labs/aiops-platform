import { authorizedRequest } from './client'
import type { InsightRunbookContract, InsightRunbookQuery } from './openapi'

// M81 closed-loop insight: resolve the deterministic runbook for a posture
// finding. The backend mapping is pure read-only (no cluster access).
//
// W10: request query and response payload types are consumed from the
// openapi-typescript generated contract (src/api/openapi.d.ts ←
// docs/api/openapi.yaml); runtime behavior is unchanged.
export type InsightRunbook = InsightRunbookContract
export type InsightRunbookParams = Omit<InsightRunbookQuery, 'cluster_id' | 'namespace'> & {
  clusterId: number
  namespace?: string
}

export async function getInsightRunbook(
  token: string,
  params: InsightRunbookParams,
): Promise<InsightRunbook> {
  const query = new URLSearchParams({
    cluster_id: String(params.clusterId),
    domain: params.domain,
    kind: params.kind,
    name: params.name,
  })
  if (params.code) query.set('code', params.code)
  if (params.namespace) query.set('namespace', params.namespace)
  const runbook = await authorizedRequest<InsightRunbook>(`/api/v1/aiops/insight?${query.toString()}`, token)
  return {
    ...runbook,
    diagnoses: runbook.diagnoses ?? [],
    inspection: runbook.inspection ?? [],
    operations: runbook.operations ?? [],
  }
}