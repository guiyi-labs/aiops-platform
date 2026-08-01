export interface EventSummary {
  uid: string
  name: string
  namespace: string
  kind: string
  type: string
  reason: string
  message: string
  count: number
  last_timestamp: string
  first_timestamp: string
  cluster_id: number
  occurred_at: string
}

export interface InhibitView {
  id: number
  source_cluster_id?: number
  source_rule_name: string
  source_severity: string
  target_cluster_id?: number
  target_rule_name: string
  target_severity: string
  reason: string
  enabled: boolean
  creator_id: number
}

export interface AlertDeliveryView {
  id: number
  route_id: number
  receiver_id: number
  alert_instance_id: number
  event_type: string
  dedupe_key: string
  status: string
  attempts: number
  next_attempt_at: string
  delivered_at?: string
  last_error?: string
  created_at: string
  updated_at: string
}
