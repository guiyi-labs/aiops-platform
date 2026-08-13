import { authorizedRequest } from './client'
import type { NotificationDeliveryListResponse, NotificationDeliveryStatus, NotificationEventType } from '../types/notification'

export type NotificationFilters = {
  diagnosisID?: number
  incidentID?: number
  eventType?: NotificationEventType | ''
  status?: NotificationDeliveryStatus | ''
  limit?: number
}

export function listNotificationDeliveries(token: string, filters: NotificationFilters = {}): Promise<NotificationDeliveryListResponse> {
  const query = new URLSearchParams({ limit: String(filters.limit ?? 100) })
  if (filters.diagnosisID) query.set('diagnosis_id', String(filters.diagnosisID))
  if (filters.incidentID) query.set('incident_id', String(filters.incidentID))
  if (filters.eventType) query.set('event_type', filters.eventType)
  if (filters.status) query.set('status', filters.status)
  return authorizedRequest(`/api/v1/notification-deliveries?${query}`, token)
}

export function retryNotificationDelivery(token: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/notification-deliveries/${id}/retry`, token, { method: 'POST' })
}
