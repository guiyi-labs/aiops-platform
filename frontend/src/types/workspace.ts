export interface Workspace {
  id: number
  name: string
  display_name: string
  owner_user_id: number
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface WorkspaceMembership {
  workspace_id: number
  cluster_id: number
  namespace: string
  created_at: string
}

export interface WorkspaceQuota {
  hard_cpu_cores: number
  hard_memory_mib: number
  hard_pod_count: number
  hard_namespace_count: number
}

export type WorkspaceRole = 'workspace_admin' | 'workspace_editor' | 'workspace_viewer'

export interface WorkspaceRoleBinding {
  user_id: number
  user_name: string
  role: WorkspaceRole
  created_at: string
}

export type WorkspaceScope = 'platform' | 'workspace' | 'cluster'
