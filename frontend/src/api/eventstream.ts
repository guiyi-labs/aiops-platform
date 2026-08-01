import { authorizedRequest } from './client'
import type { InhibitView, AlertDeliveryView } from '../types/eventstream'

// SSE event stream — returns EventSource URL builder, not a fetch call
export function buildEventStreamUrl(clusterId: number, accessToken: string, params?: { namespace?: string; field_selector?: string }): string {
  const sp = new URLSearchParams()
  if (params?.namespace) sp.set('namespace', params.namespace)
  if (params?.field_selector) sp.set('field_selector', params.field_selector)
  const q = sp.toString()
  // Note: EventSource doesn't support Authorization headers, so token goes as query param
  // The backend should accept ?token= for SSE connections
  return `/api/v1/clusters/${clusterId}/events/stream${q ? `?${q}&token=${accessToken}` : `?token=${accessToken}`}`
}

export function listInhibits(token: string): Promise<{ items: InhibitView[] }> {
  return authorizedRequest('/api/v1/alert-routes/inhibits', token)
}

export function createInhibit(token: string, input: Omit<InhibitView, 'id' | 'creator_id'>): Promise<InhibitView> {
  return authorizedRequest('/api/v1/alert-routes/inhibits', token, { method: 'POST', body: JSON.stringify(input) })
}

export function deleteInhibit(token: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/alert-routes/inhibits/${id}`, token, { method: 'DELETE' })
}

export function listAlertDeliveries(token: string, params?: { status?: string; limit?: number }): Promise<{ items: AlertDeliveryView[]; total: number }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/alert-routes/deliveries${q ? `?${q}` : ''}`, token)
}
