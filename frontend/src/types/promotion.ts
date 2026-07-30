export type PromotionKind = 'Deployment' | 'Service' | 'Ingress'
export type PromotionDependencyKind = 'ConfigMap' | 'Secret'
export type PromotionStatus = 'awaiting_confirmation' | 'executing' | 'succeeded' | 'failed' | 'partial' | 'expired'
export type PromotionItemStatus = 'pending' | 'applied' | 'failed' | 'skipped'

export interface PromotionBundleItemRequest {
  kind: PromotionKind
  namespace: string
  name: string
}

export interface PromotionDependencyMapping {
  kind: PromotionDependencyKind
  source_namespace: string
  source_name: string
  destination_namespace: string
  destination_name: string
}

export interface PromotionPreviewRequest {
  source_cluster_id: number
  destination_cluster_id: number
  source_namespace: string
  destination_namespace: string
  bundle: PromotionBundleItemRequest[]
  dependency_mappings?: PromotionDependencyMapping[]
}

export interface PromotionBundleItem {
  ordinal: number
  kind: PromotionKind
  source_namespace: string
  source_name: string
  source_uid: string
  source_resource_version: string
  destination_namespace: string
  destination_name: string
  diff: { mode?: string; before?: Record<string, unknown>; after?: Record<string, unknown>; changed_fields?: string[] }
  item_status: PromotionItemStatus
  last_error?: string
}

export interface PromotionDependencyRecord {
  kind: PromotionDependencyKind
  source_namespace: string
  source_name: string
  destination_namespace: string
  destination_name: string
  resolved: boolean
}

export interface PromotionBundleSummary {
  item_count: number
  deployment_count: number
  service_count: number
  ingress_count: number
  pending_count: number
  applied_count: number
  failed_count: number
  skipped_count: number
}

export interface PromotionPlan {
  id: string
  source_cluster_id: number
  destination_cluster_id: number
  source_namespace: string
  destination_namespace: string
  status: PromotionStatus
  bundle_summary: PromotionBundleSummary
  dependency_summary: PromotionDependencyRecord[]
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
  items?: PromotionBundleItem[]
  dependencies?: PromotionDependencyRecord[]
}

export interface PromotionListResponse {
  items: PromotionPlan[]
  total: number
}
