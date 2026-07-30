import type { KubernetesEvent } from '../types/kubernetes'

export type EventResourceKind = 'Pod' | 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'ReplicaSet' | 'Job' | 'CronJob' | 'Node' | 'Service' | 'Ingress' | 'PVC' | 'PV' | 'StorageClass' | 'HPA' | 'ResourceQuota' | 'LimitRange' | 'ConfigMap' | 'Secret' | 'NetworkPolicy' | 'PDB' | 'ServiceAccount'

const involvedObjectKinds: Record<EventResourceKind, string> = {
  Pod: 'Pod',
  Deployment: 'Deployment',
  StatefulSet: 'StatefulSet',
  DaemonSet: 'DaemonSet',
  ReplicaSet: 'ReplicaSet',
  Job: 'Job',
  CronJob: 'CronJob',
  Node: 'Node',
  Service: 'Service',
  Ingress: 'Ingress',
  PVC: 'PersistentVolumeClaim',
  PV: 'PersistentVolume',
  StorageClass: 'StorageClass',
  HPA: 'HorizontalPodAutoscaler',
  ResourceQuota: 'ResourceQuota',
  LimitRange: 'LimitRange',
  ConfigMap: 'ConfigMap',
  Secret: 'Secret',
  NetworkPolicy: 'NetworkPolicy',
  PDB: 'PodDisruptionBudget',
  ServiceAccount: 'ServiceAccount',
}

export function eventTimestamp(event: KubernetesEvent): string {
  return event.series?.lastObservedTime || event.eventTime || event.lastTimestamp || event.firstTimestamp || event.metadata.creationTimestamp || ''
}

export function relatedResourceEvents(events: KubernetesEvent[], target: { kind: EventResourceKind; namespace: string; name: string; clusterScoped: boolean }): KubernetesEvent[] {
  return events
    .filter((event) => event.involvedObject.name === target.name
      && event.involvedObject.kind === involvedObjectKinds[target.kind]
      && (target.clusterScoped || event.involvedObject.namespace === target.namespace))
    .sort((left, right) => eventTimestamp(right).localeCompare(eventTimestamp(left)))
}
