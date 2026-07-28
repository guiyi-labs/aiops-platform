export type AuditResult = 'success' | 'failure' | 'denied'
export interface AuditEntry {
  id: number
  actor: { id?: number; name: string }
  cluster_id?: number
  action: string
  resource: { type?: string; namespace?: string; name?: string }
  result: AuditResult
  request_id: string
  status_code: number
  ip_address: string
  user_agent: string
  details: Record<string, unknown>
  created_at: string
}
export interface AuditListResponse { items: AuditEntry[]; total: number; remaining: number }
