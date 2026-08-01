import { authorizedRequest } from './client'
import type { CopyOpsPlan, CopyPreviewRequest } from '../types/copyops'

export function previewCopyPlan(token: string, clusterId: number, input: CopyPreviewRequest): Promise<CopyOpsPlan> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/copy-plans/preview`, token, { method: 'POST', body: JSON.stringify(input) })
}

export function listCopyPlansByCluster(token: string, clusterId: number, params?: { offset?: number; limit?: number }): Promise<{ items: CopyOpsPlan[]; total: number; offset: number; limit: number }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null) sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterId}/copy-plans${q ? `?${q}` : ''}`, token)
}

export function executeCopyPlan(token: string, planId: string, idempotencyKey: string): Promise<CopyOpsPlan> {
  return authorizedRequest(`/api/v1/copy-plans/${planId}/execute`, token, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

export function getCopyPlan(token: string, planId: string): Promise<CopyOpsPlan> {
  return authorizedRequest(`/api/v1/copy-plans/${planId}`, token)
}

export function listMyCopyPlans(token: string, params?: { offset?: number; limit?: number }): Promise<{ items: CopyOpsPlan[]; total: number; offset: number; limit: number }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null) sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/copy-plans${q ? `?${q}` : ''}`, token)
}
