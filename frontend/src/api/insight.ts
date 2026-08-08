import { authorizedRequest } from './client'
import type { InsightRunbook } from '../types/insight'

// M81 closed-loop insight: resolve the deterministic runbook for a posture
// finding. The backend mapping is pure read-only (no cluster access).
export async function getInsightRunbook(
  token: string,
  params: { clusterId: number; domain: string; code?: string; kind: string; namespace?: string; name: string },
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