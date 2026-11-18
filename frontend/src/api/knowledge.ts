import { authorizedRequest } from './client'

export interface KnowledgeEntry {
  id: number
  source_diagnosis_id: number
  rule_id: string
  severity: string
  resource_kind: string
  resource_namespace?: string
  resource_name: string
  summary: string
  root_causes: string[]
  recommendations: string[]
  noted_at: string
}
export interface KnowledgeResponse { items: KnowledgeEntry[]; total: number; truncated?: boolean }

export function listKnowledge(token: string, filters: { q?: string; rule_id?: string; resource_kind?: string } = {}) {
  const query = new URLSearchParams({ limit: '100' })
  for (const [key, value] of Object.entries(filters)) if (value?.trim()) query.set(key, value.trim())
  return authorizedRequest<KnowledgeResponse>(`/api/v1/aiops/knowledge?${query}`, token)
}
export function getKnowledgeStats(token: string) {
  return authorizedRequest<{ total: number }>('/api/v1/aiops/knowledge/stats', token)
}
