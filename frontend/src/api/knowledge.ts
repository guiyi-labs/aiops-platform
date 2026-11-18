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

export interface RetrievalTrace {
  query: { summary_hint: string; rule_id: string; resource_kind: string }
  structured: KnowledgeEntry[]
  structured_truncated: boolean
  vector: (KnowledgeEntry & { score?: number })[]
  fused: (KnowledgeEntry & { score?: number })[]
  results: KnowledgeEntry[]
  vector_status: 'complete' | 'disabled' | 'unavailable'
  embedder: string
  dimension: number
  pool_count: number
  pool_truncated: boolean
  shortlist_limit: number
  vector_limit: number
  result_limit: number
  rank_constant: number
}
export function inspectKnowledge(token: string, input: { q: string; rule_id: string; resource_kind: string }) {
  const query = new URLSearchParams(input)
  return authorizedRequest<RetrievalTrace>(`/api/v1/aiops/knowledge/inspect?${query}`, token)
}
