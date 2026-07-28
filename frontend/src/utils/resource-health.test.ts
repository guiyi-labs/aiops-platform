import { describe, expect, it } from 'vitest'

import type { Deployment, Pod } from '../types/kubernetes'
import { deploymentHealth, labelsMatch, podHealth, selectsPod } from './resource-health'

function pod(phase: string, ready = true, waitingReason = ''): Pod {
  return {
    metadata: { name: 'api-1', labels: { app: 'api', track: 'stable' } },
    spec: { containers: [{ name: 'api', image: 'example/api:1' }] },
    status: {
      phase,
      containerStatuses: [{ name: 'api', ready, restartCount: 0, state: waitingReason ? { waiting: { reason: waitingReason } } : {}, lastState: {} }],
    },
  }
}

function deployment(readyReplicas: number, replicas = 2, unavailableReplicas = replicas - readyReplicas): Deployment {
  return {
    metadata: { name: 'api' },
    spec: { replicas, selector: { matchLabels: { app: 'api' } }, template: { spec: { containers: [] } } },
    status: { replicas, readyReplicas, availableReplicas: readyReplicas, updatedReplicas: readyReplicas, unavailableReplicas },
  }
}

describe('resource health', () => {
  it('matches Kubernetes selectors only when every requested label exists', () => {
    expect(labelsMatch({ app: 'api', track: 'stable' }, { app: 'api', track: 'stable', version: '1' })).toBe(true)
    expect(labelsMatch({ app: 'api', track: 'canary' }, { app: 'api', track: 'stable' })).toBe(false)
    expect(labelsMatch({}, { app: 'api' })).toBe(false)
  })

  it('never links a namespaced selector to a pod in another namespace', () => {
    const selectedPod = pod('Running')
    selectedPod.metadata.namespace = 'team-b'
    expect(selectsPod('team-a', { app: 'api' }, selectedPod)).toBe(false)
    expect(selectsPod('team-b', { app: 'api' }, selectedPod)).toBe(true)
  })

  it('treats running ready and completed pods as healthy', () => {
    expect(podHealth(pod('Running'))).toBe('healthy')
    expect(podHealth(pod('Succeeded', false))).toBe('healthy')
  })

  it('prioritizes actionable container failures over the pod phase', () => {
    expect(podHealth(pod('Running', false, 'CrashLoopBackOff'))).toBe('critical')
    expect(podHealth(pod('Pending', false))).toBe('warning')
  })

  it('grades deployment readiness without treating scaled-to-zero as broken', () => {
    expect(deploymentHealth(deployment(2))).toBe('healthy')
    expect(deploymentHealth(deployment(1))).toBe('warning')
    expect(deploymentHealth(deployment(0))).toBe('critical')
    expect(deploymentHealth(deployment(0, 0, 0))).toBe('healthy')
  })
})
