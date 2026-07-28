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
