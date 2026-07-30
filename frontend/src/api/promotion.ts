import { authorizedRequest } from './client'
import type { PromotionPlan, PromotionPreviewRequest, PromotionListResponse } from '../types/promotion'

export function previewPromotion(accessToken: string, request: PromotionPreviewRequest): Promise<PromotionPlan> {
  return authorizedRequest('/api/v1/promotions/preview', accessToken, {
    method: 'POST', body: JSON.stringify(request),
  })
}

export function executePromotion(accessToken: string, promotionID: string, confirmationToken: string, idempotencyKey: string): Promise<PromotionPlan> {
  return authorizedRequest(`/api/v1/promotions/${promotionID}/execute`, accessToken, {
    method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ confirmation_token: confirmationToken }),
  })
}

export function getPromotion(accessToken: string, promotionID: string): Promise<PromotionPlan> {
  return authorizedRequest(`/api/v1/promotions/${promotionID}`, accessToken)
}

export function listPromotions(accessToken: string, sourceClusterID: number, namespace?: string): Promise<PromotionListResponse> {
  const query = new URLSearchParams({ source_cluster_id: String(sourceClusterID) })
  if (namespace) query.set('namespace', namespace)
  return authorizedRequest(`/api/v1/promotions?${query}`, accessToken)
}
