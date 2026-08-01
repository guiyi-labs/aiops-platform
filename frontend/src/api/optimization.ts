import { authorizedRequest } from './client'
import type { CISStatus, CostRate, DeprecatedAPIStatus, FinOpsWasteSummary } from '../types/optimization'

// Client for the read-only optimization analyzers (M61-M65).
//
// Each request sends only `cluster_id` (plus the target version for the
// deprecated-API check), which makes the server auto-collect the observation
// bundle from the live cluster and run the pure analyzer. Callers may still
// supply their own bundle out-of-band; the console always uses auto-collection.
//
// Go marshals a nil slice as `null`, so every list is normalised to an array
// here. That keeps the contract honest at the boundary instead of forcing
// every view to guard against null.

export async function analyzeCIS(token: string, clusterId: number): Promise<CISStatus> {
  const status = await authorizedRequest<CISStatus>('/api/v1/optimization/cis/analyze', token, {
    method: 'POST',
    body: JSON.stringify({ cluster_id: clusterId }),
  })
  return { ...status, findings: status.findings ?? [], by_severity: status.by_severity ?? {}, by_family: status.by_family ?? {} }
}

export async function analyzeFinOps(token: string, clusterId: number, rate?: CostRate): Promise<FinOpsWasteSummary> {
  const summary = await authorizedRequest<FinOpsWasteSummary>('/api/v1/optimization/finops/analyze', token, {
    method: 'POST',
    body: JSON.stringify({ cluster_id: clusterId, ...(rate ? { rate } : {}) }),
  })
  return { ...summary, recommendations: summary.recommendations ?? [] }
}

export async function analyzeDeprecatedAPI(token: string, clusterId: number, targetVersion: string): Promise<DeprecatedAPIStatus> {
  const status = await authorizedRequest<DeprecatedAPIStatus>('/api/v1/optimization/deprecated-api/analyze', token, {
    method: 'POST',
    body: JSON.stringify({ cluster_id: clusterId, target_version: targetVersion }),
  })
  return { ...status, findings: status.findings ?? [] }
}
