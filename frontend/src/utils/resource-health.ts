import type { Deployment, Pod } from '../types/kubernetes'

export type ResourceHealth = 'healthy' | 'warning' | 'critical' | 'unknown'

export function labelsMatch(selector?: Record<string, string>, labels?: Record<string, string>): boolean {
  const entries = Object.entries(selector ?? {})
  return entries.length > 0 && entries.every(([key, value]) => labels?.[key] === value)
}

export function selectsPod(namespace: string | undefined, selector: Record<string, string> | undefined, pod: Pod): boolean {
  return (namespace ?? '') === (pod.metadata.namespace ?? '') && labelsMatch(selector, pod.metadata.labels)
}

export function podHealth(pod: Pod): ResourceHealth {
  if (pod.status.phase === 'Failed') return 'critical'
  const statuses = pod.status.containerStatuses ?? []
  if (statuses.some((item) => ['ImagePullBackOff', 'ErrImagePull', 'CrashLoopBackOff'].includes(item.state.waiting?.reason ?? ''))) return 'critical'
  if (pod.status.phase === 'Pending' || pod.status.phase === 'Unknown') return 'warning'
  if (pod.status.phase === 'Succeeded') return 'healthy'
  if (pod.status.phase === 'Running') return statuses.length > 0 && statuses.every((item) => item.ready) ? 'healthy' : 'warning'
  return 'unknown'
}

export function deploymentHealth(deployment: Deployment): ResourceHealth {
  const desired = deployment.spec.replicas ?? 1
  if (desired === 0) return 'healthy'
  if (deployment.status.readyReplicas === 0) return 'critical'
  if (deployment.status.readyReplicas < desired || deployment.status.unavailableReplicas > 0) return 'warning'
  return 'healthy'
}
