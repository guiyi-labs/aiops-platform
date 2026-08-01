export interface RepositoryView {
  id: number
  name: string
  display_name: string
  url: string
  has_auth: boolean
  created_at: string
  updated_at: string
}

export interface ChartSummary {
  name: string
  version: string
  description?: string
  icon?: string
  home?: string
  app_version?: string
}

export interface ChartVersion {
  version: string
  app_version: string
  created: string
  digest: string
}

export interface ChartDetail {
  name: string
  description?: string
  icon?: string
  home?: string
  maintainers?: string[]
  versions: ChartVersion[]
}

export interface AppCatalogPlan {
  id: string
  status: string
  repo_id: number
  chart_name: string
  chart_version: string
  target_cluster_id: number
  target_namespace: string
  release_name: string
  values_yaml?: string
  chart_metadata?: Record<string, unknown>
  deploy_diff?: Record<string, unknown>
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
}

export interface CreateRepoRequest {
  name: string
  display_name: string
  url: string
  username?: string
  password?: string
}

export interface DeployPreviewRequest {
  repo_id: number
  chart_name: string
  chart_version: string
  target_cluster_id: number
  target_namespace: string
  release_name: string
  values_yaml?: string
}
