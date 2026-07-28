import { describe, expect, it } from 'vitest'

import type { KubernetesEvent } from '../types/kubernetes'
import { eventTimestamp, relatedResourceEvents } from './kubernetes-events'

function event(name: string, kind: string, namespace: string, time: Partial<KubernetesEvent>): KubernetesEvent {
  return {
    metadata: { name: `${name}.${kind}`, namespace, creationTimestamp: time.metadata?.creationTimestamp },
    type: 'Normal',
    reason: 'Observed',
    message: 'fixture',
    count: 1,
    involvedObject: { name, kind, namespace },
    ...time,
  }
}

describe('related Kubernetes events', () => {
  it('matches resource name, Kubernetes kind and namespace exactly', () => {
    const events = [
      event('cache', 'PersistentVolumeClaim', 'team-a', { eventTime: '2026-07-27T10:00:00Z' }),
      event('cache-copy', 'PersistentVolumeClaim', 'team-a', { eventTime: '2026-07-27T10:01:00Z' }),
      event('cache', 'ConfigMap', 'team-a', { eventTime: '2026-07-27T10:02:00Z' }),
      event('cache', 'PersistentVolumeClaim', 'team-b', { eventTime: '2026-07-27T10:03:00Z' }),
    ]
    const result = relatedResourceEvents(events, { kind: 'PVC', namespace: 'team-a', name: 'cache', clusterScoped: false })
    expect(result).toHaveLength(1)
    expect(result[0]?.involvedObject.kind).toBe('PersistentVolumeClaim')
  })

  it('does not require a namespace for cluster-scoped resources', () => {
    const events = [event('standard', 'StorageClass', '', { eventTime: '2026-07-27T10:00:00Z' })]
    expect(relatedResourceEvents(events, { kind: 'StorageClass', namespace: '', name: 'standard', clusterScoped: true })).toHaveLength(1)
  })

  it('uses modern observation fields and returns newest events first', () => {
    const older = event('api', 'Pod', 'demo', { eventTime: '2026-07-27T10:00:00Z' })
    const newer = event('api', 'Pod', 'demo', { eventTime: '2026-07-27T09:00:00Z', series: { count: 2, lastObservedTime: '2026-07-27T11:00:00Z' } })
    const result = relatedResourceEvents([older, newer], { kind: 'Pod', namespace: 'demo', name: 'api', clusterScoped: false })
    expect(result).toEqual([newer, older])
    expect(eventTimestamp(newer)).toBe('2026-07-27T11:00:00Z')
  })

  it('maps M17 aliases to exact Kubernetes involvedObject kinds', () => {
    const events = [
      event('scale', 'HorizontalPodAutoscaler', 'team-a', { eventTime: '2026-07-27T10:00:00Z' }),
      event('quota', 'ResourceQuota', 'team-a', { eventTime: '2026-07-27T10:01:00Z' }),
      event('runtime', 'Secret', 'team-a', { eventTime: '2026-07-27T10:02:00Z' }),
    ]
    expect(relatedResourceEvents(events, { kind: 'HPA', namespace: 'team-a', name: 'scale', clusterScoped: false })).toHaveLength(1)
    expect(relatedResourceEvents(events, { kind: 'ResourceQuota', namespace: 'team-a', name: 'quota', clusterScoped: false })).toHaveLength(1)
    expect(relatedResourceEvents(events, { kind: 'Secret', namespace: 'team-a', name: 'runtime', clusterScoped: false })).toHaveLength(1)
  })
})
