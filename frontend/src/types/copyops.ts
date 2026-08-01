export interface CopySummaryItem {
  kind: string
  namespace: string
  name: string
  destination_namespace: string
  destination_name: string
}

export interface CopyOpsPlan {
  id: string
  status: string
  source_cluster_id: number
  source_namespace: string
  source_namespace_uid: string
  source_namespace_resource_version: string
  target_cluster_id: number
  target_namespace: string
  copy_summary?: CopySummaryItem[]
  diff?: Record<string, unknown>
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
}

export interface BundleItemRequest {
  kind: string
  namespace: string
  name: string
}

export interface CopyPreviewRequest {
  target_cluster_id: number
  target_namespace: string
  items: BundleItemRequest[]
}
