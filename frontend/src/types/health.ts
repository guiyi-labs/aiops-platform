export interface HealthResponse {
  status: 'ok' | 'ready' | 'unavailable'
  service: string
  version: string
  checked_at: string
}
