export type NotificationEventType = 'diagnosis.created' | 'diagnosis.status_changed' | 'diagnosis.assigned' | 'incident.sla_approaching' | 'incident.sla_breached' | 'incident.sla_escalated'
export type NotificationDeliveryStatus = 'pending' | 'delivering' | 'delivered' | 'dead'

export interface NotificationDelivery {
  id: number
  diagnosis_id?: number
  incident_id?: number
  event_type: NotificationEventType
  escalation_level?: number
  status: NotificationDeliveryStatus
  attempts: number
  next_attempt_at: string
  delivered_at?: string
  last_error?: string
  created_at: string
  updated_at: string
}

export interface NotificationDeliveryListResponse {
  items: NotificationDelivery[]
  total: number
  remaining: number
}
