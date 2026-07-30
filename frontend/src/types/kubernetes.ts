export interface ObjectMeta { name: string; namespace?: string; uid?: string; creationTimestamp?: string; labels?: Record<string, string>; annotations?: Record<string, string>; resourceVersion?: string }
export interface Namespace { metadata: ObjectMeta; status: { phase: string } }
export interface NodeResource { metadata: ObjectMeta; spec: { unschedulable?: boolean }; status: { nodeInfo: { kubeletVersion: string; osImage: string; containerRuntimeVersion: string }; addresses: Array<{ type: string; address: string }>; conditions: Array<{ type: string; status: string; reason: string; message: string; lastTransitionTime: string }>; capacity?: Record<string, string>; allocatable?: Record<string, string> } }
export interface ResourceUsage { cpu: string; memory: string }
export interface NodeMetric { metadata: ObjectMeta; timestamp: string; window: string; usage: ResourceUsage }
export interface PodMetric { metadata: ObjectMeta; timestamp: string; window: string; containers: Array<{ name: string; usage: ResourceUsage }> }
export interface Deployment { metadata: ObjectMeta; spec: { replicas?: number; selector: { matchLabels?: Record<string, string> }; template: { spec: { containers: Array<{ name: string; image: string }> } } }; status: { replicas: number; readyReplicas: number; availableReplicas: number; updatedReplicas: number; unavailableReplicas: number } }
export interface WorkloadTemplate { spec: { containers: Array<{ name: string; image: string }> } }
export interface WorkloadCondition { type: string; status: string; reason?: string; message?: string; lastTransitionTime?: string }
export interface StatefulSetResource {
  metadata: ObjectMeta
  spec: { replicas?: number; serviceName: string; podManagementPolicy?: string; selector: { matchLabels?: Record<string, string> }; template: WorkloadTemplate; updateStrategy: { type: string } }
  status: { replicas: number; currentReplicas: number; readyReplicas: number; updatedReplicas: number; availableReplicas: number }
}
export interface DaemonSetResource {
  metadata: ObjectMeta
  spec: { selector: { matchLabels?: Record<string, string> }; template: WorkloadTemplate; updateStrategy: { type: string } }
  status: { desiredNumberScheduled: number; currentNumberScheduled: number; numberReady: number; numberAvailable: number; updatedNumberScheduled: number; numberUnavailable: number }
}
export interface ReplicaSetResource {
  metadata: ObjectMeta
  spec: { replicas?: number; selector: { matchLabels?: Record<string, string> }; template: WorkloadTemplate }
  status: { replicas: number; readyReplicas: number; availableReplicas: number; fullyLabeledReplicas: number }
}
export interface JobSpec { parallelism?: number; completions?: number; backoffLimit?: number; suspend?: boolean; template: WorkloadTemplate }
export interface JobResource {
  metadata: ObjectMeta
  spec: JobSpec
  status: { active: number; succeeded: number; failed: number; startTime?: string; completionTime?: string; conditions?: WorkloadCondition[] }
}
export interface CronJobResource {
  metadata: ObjectMeta
  spec: { schedule: string; timeZone?: string; concurrencyPolicy?: string; suspend?: boolean; successfulJobsHistoryLimit?: number; failedJobsHistoryLimit?: number; jobTemplate: { spec: JobSpec } }
  status: { active?: Array<{ kind?: string; namespace?: string; name?: string; uid?: string }>; lastScheduleTime?: string; lastSuccessfulTime?: string }
}
export interface MetricTarget { type: string; value?: string; averageValue?: string; averageUtilization?: number }
export interface HPAMetricSpec {
  type: string
  resource?: { name: string; target: MetricTarget }
  containerResource?: { name: string; container: string; target: MetricTarget }
  pods?: { metric: { name: string }; target: MetricTarget }
  object?: { describedObject: { apiVersion: string; kind: string; name: string }; metric: { name: string }; target: MetricTarget }
  external?: { metric: { name: string }; target: MetricTarget }
}
export interface HorizontalPodAutoscalerResource {
  metadata: ObjectMeta
  spec: { scaleTargetRef: { apiVersion: string; kind: string; name: string }; minReplicas?: number; maxReplicas: number; metrics: HPAMetricSpec[] }
  status: { currentReplicas: number; desiredReplicas: number; lastScaleTime?: string; conditions: WorkloadCondition[] }
}
export interface ResourceQuotaResource { metadata: ObjectMeta; spec: { hard?: Record<string, string> }; status: { hard?: Record<string, string>; used?: Record<string, string> } }
export interface LimitRangeItem { type: string; max?: Record<string, string>; min?: Record<string, string>; default?: Record<string, string>; defaultRequest?: Record<string, string>; maxLimitRequestRatio?: Record<string, string> }
export interface LimitRangeResource { metadata: ObjectMeta; spec: { limits: LimitRangeItem[] } }
export interface SecretResource { metadata: ObjectMeta; immutable?: boolean; type: string; dataKeys: string[] }
export interface ServiceResource { metadata: ObjectMeta; spec: { type: string; clusterIP?: string; externalName?: string; selector?: Record<string, string>; ports: Array<{ name?: string; protocol: string; port: number; targetPort?: string | number; nodePort?: number }> } }
export interface IngressBackend { service?: { name: string; port: { name?: string; number?: number } } }
export interface IngressResource {
  metadata: ObjectMeta
  spec: {
    ingressClassName?: string
    defaultBackend?: IngressBackend
    rules?: Array<{ host?: string; http?: { paths: Array<{ path?: string; pathType?: string; backend: IngressBackend }> } }>
    tls?: Array<{ hosts?: string[]; secretName?: string }>
  }
  status: { loadBalancer: { ingress?: Array<{ ip?: string; hostname?: string }> } }
}
export interface EndpointSliceResource {
  metadata: ObjectMeta
  addressType: string
  serviceName?: string
  ports?: Array<{ name?: string; protocol?: string; port?: number }>
  endpoints: Array<{
    addresses: string[]
    conditions?: { ready?: boolean; serving?: boolean; terminating?: boolean }
    nodeName?: string
    targetRef?: { kind?: string; namespace?: string; name?: string; uid?: string }
  }>
}
export interface PersistentVolumeClaim {
  metadata: ObjectMeta
  spec: { accessModes?: string[]; storageClassName?: string; volumeMode?: string; volumeName?: string; resources: { requests?: Record<string, string> } }
  status: { phase: string; accessModes?: string[]; capacity?: Record<string, string> }
}
export interface StorageClassResource {
  metadata: ObjectMeta
  provisioner: string
  reclaimPolicy?: string
  volumeBindingMode?: string
  allowVolumeExpansion?: boolean
}
export interface ConfigMapResource {
  metadata: ObjectMeta
  immutable?: boolean
  dataKeys: string[]
  binaryDataKeys: string[]
}
export interface PodContainerInfo {
  name: string
  ready: boolean
  restart_count: number
  state: 'running' | 'waiting' | 'terminated'
  is_init: boolean
  image: string
}
export interface PodLogLine {
  timestamp: string
  message: string
}
export interface PodContainerLog {
  container: string
  lines: PodLogLine[]
  truncated: boolean
  truncation_reason?: 'body_limit' | 'fetch_error'
}
export interface PodLogsResponse {
  containers: PodContainerLog[]
  previous: boolean
}
export interface PersistentVolume {
  metadata: ObjectMeta
  spec: { capacity?: Record<string, string>; accessModes?: string[]; persistentVolumeReclaimPolicy?: string; storageClassName?: string; volumeMode?: string; persistentVolumeSource?: Record<string, unknown>; claimRef?: { namespace: string; name: string } }
  status: { phase: string; message?: string }
}
export interface PodDisruptionBudgetResource {
  metadata: ObjectMeta
  spec: { minAvailable?: number | string; maxUnavailable?: number | string; selector?: Record<string, unknown> }
  status: { currentHealthy: number; desiredHealthy: number; disruptionsAllowed: number; expectedPods: number }
}
export interface NetworkPolicyResource {
  metadata: ObjectMeta
  spec: { podSelector?: Record<string, unknown>; policyTypes?: string[]; ingress?: Array<Record<string, unknown>>; egress?: Array<Record<string, unknown>> }
}
export interface ServiceAccountResource {
  metadata: ObjectMeta
  automountServiceAccountToken?: boolean
  imagePullSecrets?: Array<{ name: string }>
  secretRefs?: Array<{ name: string }>
}
export interface ContainerStateDetail { reason?: string; message?: string; exitCode?: number; signal?: number; startedAt?: string; finishedAt?: string }
export interface ContainerStatus { name: string; ready: boolean; restartCount: number; state: { waiting?: ContainerStateDetail; terminated?: ContainerStateDetail }; lastState: { waiting?: ContainerStateDetail; terminated?: ContainerStateDetail } }
export interface Pod {
  metadata: ObjectMeta
  spec: { nodeName?: string; containers: Array<{ name: string; image: string }> }
  status: { phase: string; podIP?: string; hostIP?: string; reason?: string; message?: string; conditions?: Array<{ type: string; status: string; reason?: string; message?: string; lastTransitionTime?: string }>; containerStatuses?: ContainerStatus[] }
}
export interface KubernetesEvent {
  metadata: ObjectMeta
  type: string
  reason: string
  message: string
  count: number
  action?: string
  eventTime?: string
  firstTimestamp?: string
  lastTimestamp?: string
  reportingComponent?: string
  reportingInstance?: string
  series?: { count?: number; lastObservedTime?: string }
  involvedObject: { kind: string; namespace?: string; name: string; uid?: string }
}
export interface ListResponse<T> { items: T[]; total: number; remaining: number }

export interface VeleroCapability { installed: boolean; version?: string }

export interface VeleroBackup {
  name: string
  namespace: string
  phase: string
  included_namespaces?: string[]
  storage_location?: string
  ttl?: string
  expiration?: string
  started_at?: string
  completed_at?: string
  failure_reason?: string
  errors: number
  warnings: number
  created_at: string
}

export interface BackupPlan {
  id: string
  cluster_id: number
  status: string
  backup_name: string
  backup_namespace: string
  included_namespaces: string[]
  storage_location: string
  ttl: string
  include_cluster_resources: boolean
  snapshot_volumes: boolean
  label_selector?: Record<string, string>
  velero_version: string
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
  requested_by: { id: number; name: string }
  parameters: {
    included_namespaces: string[]
    storage_location: string
    ttl: string
    include_cluster_resources: boolean
    snapshot_volumes: boolean
    label_selector?: Record<string, string>
  }
}

export interface BackupStorageLocation {
  name: string
  namespace: string
  phase: string
  provider: string
}

export type SourceStatus = 'complete' | 'partial' | 'truncated' | 'unavailable'

export interface EvidenceCitation {
  api_path: string
  status: SourceStatus
  total: number
  returned: number
  remaining: number
  error?: string
  collected_at: string
}

export interface WorkloadKindCount {
  kind: string
  desired_replicas: number
  ready_replicas: number
  available_replicas: number
  updated_replicas: number
  failed_replicas: number
  count: number
}

export interface WorkloadSummary {
  evidence: EvidenceCitation
  by_kind: WorkloadKindCount[]
  total_count: number
  desired_total: number
  ready_total: number
}

export interface PodPhaseCount { phase: string; count: number }
export interface PodNodeSpread { node_name: string; count: number }
export interface PodSummary {
  evidence: EvidenceCitation
  total: number
  scheduled: number
  by_phase: PodPhaseCount[]
  by_node: PodNodeSpread[]
  unique_node_count: number
}

export interface ResourceQuotaEntry { name: string; hard?: Record<string, string>; used?: Record<string, string> }
export interface ResourceQuotaPosture { evidence: EvidenceCitation; quotas: ResourceQuotaEntry[] }
export interface LimitRangePosture { evidence: EvidenceCitation; ranges: LimitRangeResource[] }
export interface PDBEntry {
  name: string
  min_available?: string
  max_unavailable?: string
  current_healthy: number
  desired_healthy: number
  disruptions_allowed: number
  expected_pods: number
}
export interface PDBPosture { evidence: EvidenceCitation; pdbs: PDBEntry[]; count: number }
export interface NodeCapacityEntry { name: string; capacity?: Record<string, string>; allocatable?: Record<string, string>; schedulable: boolean }
export interface NodeCapacityPosture { evidence: EvidenceCitation; nodes: NodeCapacityEntry[]; count: number }

export interface NamespacePosture {
  name: string
  phase: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  created_at: string
  resource_quotas: ResourceQuotaPosture
  limit_ranges: LimitRangePosture
  workloads: WorkloadSummary
  pods: PodSummary
  pdbs: PDBPosture
  node_capacity: NodeCapacityPosture
  partial_sections: string[]
}

export interface PostureListEntry {
  name: string
  phase: string
  created_at: string
  workload_count: number
  pod_count: number
  quota_count: number
  limit_range_count: number
  pdb_count: number
  partial_sections: string[]
}

// --- Node Maintenance (M30) ---

export type MaintenanceAction = 'cordon' | 'uncordon' | 'drain'
export type MaintenanceStatus = 'awaiting_confirmation' | 'executing' | 'succeeded' | 'failed' | 'expired'
export type PodClassification = 'retained' | 'evictable' | 'blocking'

export interface MaintenancePodEvidence {
  name: string
  namespace: string
  uid: string
  resource_version: string
  owner_kind: string
  owner_name: string
  has_empty_dir: boolean
  classification: PodClassification
  pdb_name?: string
  pdb_disruptions_allowed?: number
}

export interface MaintenancePreviewEvidence {
  node_uid: string
  node_resource_version: string
  node_unschedulable: boolean
  pods: MaintenancePodEvidence[]
  retained_count: number
  evictable_count: number
  blocking_count: number
}

export interface MaintenancePodOutcome {
  name: string
  namespace: string
  outcome: string
  detail?: string
}

export interface MaintenanceExecutionResult {
  node_patched: boolean
  unschedulable_now: boolean
  pod_outcomes?: MaintenancePodOutcome[]
  evicted_count: number
  failed_count: number
  partial: boolean
}

export interface MaintenancePlan {
  id: string
  cluster_id: number
  status: MaintenanceStatus
  action: MaintenanceAction
  node_name: string
  node_uid: string
  node_resource_version: string
  node_unschedulable: boolean
  preview_evidence: MaintenancePreviewEvidence
  execution_result?: MaintenanceExecutionResult
  requested_by: { id: number; name: string }
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
}

// --- Restore Rehearsal (M31) ---

export type RestoreRehearsalStatus = 'awaiting_confirmation' | 'executing' | 'succeeded' | 'failed' | 'expired'

export interface RestoreQuarantineStatus {
  namespace_created: boolean
  namespace_uid?: string
  network_policy_name: string
  network_policy_created: boolean
  resource_quota_name: string
  resource_quota_created: boolean
  dry_run_validated: boolean
}

export interface RestoreRestoredItem {
  kind: string
  name: string
  namespace: string
}

export interface RestoreExecutionResult {
  restore_created: boolean
  restore_phase?: string
  restore_uid?: string
  restored_items?: RestoreRestoredItem[]
  restored_item_count: number
  truncated_items: boolean
  quarantine_established: boolean
  failure_reason?: string
  partial: boolean
}

export interface RestoreSourceSnapshot {
  name: string
  namespace: string
  uid: string
  resource_version: string
  phase: string
  included_namespaces?: string[]
}

export interface RestorePlan {
  id: string
  cluster_id: number
  status: RestoreRehearsalStatus
  source_backup_name: string
  source_backup_namespace: string
  source_backup_uid: string
  source_backup_resource_version: string
  source_backup_phase: string
  destination_namespace: string
  destination_namespace_uid: string
  velero_restore_name: string
  velero_restore_namespace: string
  velero_restore_uid: string
  quarantine_status: RestoreQuarantineStatus
  execution_result?: RestoreExecutionResult
  requested_by: { id: number; name: string }
  allowed_kinds: string[]
  excluded_kinds: string[]
  source_snapshot: RestoreSourceSnapshot
  destination_name: string
  expires_at: string
  executed_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  confirmation_token?: string
}
