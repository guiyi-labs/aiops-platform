import { authorizedRequest } from './client'
import type { RepositoryView, ChartSummary, ChartDetail, AppCatalogPlan, CreateRepoRequest, DeployPreviewRequest } from '../types/appcatalog'

export function listRepositories(token: string): Promise<{ items: RepositoryView[]; total: number }> {
  return authorizedRequest('/api/v1/app-catalog/repositories', token)
}

export function createRepository(token: string, input: CreateRepoRequest): Promise<RepositoryView> {
  return authorizedRequest('/api/v1/app-catalog/repositories', token, { method: 'POST', body: JSON.stringify(input) })
}

export function getRepository(token: string, repoId: number): Promise<RepositoryView> {
  return authorizedRequest(`/api/v1/app-catalog/repositories/${repoId}`, token)
}

export function deleteRepository(token: string, repoId: number): Promise<void> {
  return authorizedRequest(`/api/v1/app-catalog/repositories/${repoId}`, token, { method: 'DELETE' })
}

export function listCharts(token: string, repoId: number): Promise<{ items: ChartSummary[]; total: number }> {
  return authorizedRequest(`/api/v1/app-catalog/repositories/${repoId}/charts`, token)
}

export function getChart(token: string, repoId: number, chartName: string): Promise<ChartDetail> {
  return authorizedRequest(`/api/v1/app-catalog/repositories/${repoId}/charts/${chartName}`, token)
}

export function listAppCatalogPlans(token: string, params?: { cluster_id?: number; namespace?: string }): Promise<{ items: AppCatalogPlan[]; total: number }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/app-catalog/plans${q ? `?${q}` : ''}`, token)
}

export function previewDeploy(token: string, input: DeployPreviewRequest): Promise<AppCatalogPlan> {
  return authorizedRequest('/api/v1/app-catalog/plans/preview', token, { method: 'POST', body: JSON.stringify(input) })
}

export function getAppCatalogPlan(token: string, planId: string): Promise<AppCatalogPlan> {
  return authorizedRequest(`/api/v1/app-catalog/plans/${planId}`, token)
}

export function executeDeploy(token: string, planId: string, confirmationToken: string, idempotencyKey: string): Promise<AppCatalogPlan> {
  return authorizedRequest(`/api/v1/app-catalog/plans/${planId}/execute`, token, {
    method: 'POST',
    body: JSON.stringify({ confirmation_token: confirmationToken }),
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}
