export interface GitOpsCapability {
  installed: boolean
  version?: string
}

export interface GitOpsApplication {
  name: string
  uid: string
  resource_version: string
  project: string
  sync_status: string
  sync_revision?: string
  health_status: string
  health_message?: string
  source_repo_url: string
  source_target_revision?: string
  source_path?: string
  destination_server?: string
  destination_namespace: string
  operation_state_phase?: string
  operation_started_at?: string
  last_sync_started_at?: string
  last_sync_finished_at?: string
  conditions?: string[]
  created_at: string
}
