import { describe, expect, it } from 'vitest'

import { aggregateNodeAllocatable, aggregateNodeMetrics, aggregatePodMetrics, cpuMillicores, formatCPU, formatMemory, memoryBytes, rankPodMetrics, utilizationPercent } from './resource-metrics'

describe('Kubernetes resource metrics', () => {
  it('parses CPU quantities without inventing utilization percentages', () => {
    expect(cpuMillicores('250000000n')).toBe(250)
    expect(cpuMillicores('250000u')).toBe(250)
    expect(cpuMillicores('250m')).toBe(250)
    expect(cpuMillicores('1.5')).toBe(1500)
    expect(cpuMillicores('12Mi')).toBeNull()
  })

  it('parses binary and decimal memory quantities', () => {
    expect(memoryBytes('512Ki')).toBe(512 * 2 ** 10)
    expect(memoryBytes('1.5Gi')).toBe(1.5 * 2 ** 30)
    expect(memoryBytes('2G')).toBe(2e9)
    expect(memoryBytes('2048')).toBe(2048)
    expect(memoryBytes('bad')).toBeNull()
  })

  it('aggregates node and every Pod container independently', () => {
    expect(aggregateNodeMetrics([{ metadata: { name: 'worker' }, timestamp: '', window: '30s', usage: { cpu: '1', memory: '2Gi' } }])).toEqual({ cpuMillicores: 1000, memoryBytes: 2 * 2 ** 30 })
    expect(aggregatePodMetrics([{ metadata: { name: 'api', namespace: 'prod' }, timestamp: '', window: '30s', containers: [{ name: 'app', usage: { cpu: '125m', memory: '128Mi' } }, { name: 'sidecar', usage: { cpu: '25m', memory: '32Mi' } }] }])).toEqual({ cpuMillicores: 150, memoryBytes: 160 * 2 ** 20 })
  })

  it('formats absolute usage with explicit units', () => {
    expect(formatCPU(1500)).toBe('1.50 cores')
    expect(formatCPU(250)).toBe('250m')
    expect(formatMemory(1536 * 2 ** 20)).toBe('1.50 GiB')
  })

  it('uses only name-matched real Node allocatable as a utilization denominator', () => {
    const nodes = [{ metadata: { name: 'worker-a' }, spec: {}, status: { nodeInfo: { kubeletVersion: '', osImage: '', containerRuntimeVersion: '' }, addresses: [], conditions: [], allocatable: { cpu: '4', memory: '8Gi' } } }]
    const metrics = [{ metadata: { name: 'worker-a' }, timestamp: '', window: '30s', usage: { cpu: '1', memory: '2Gi' } }]
    expect(aggregateNodeAllocatable(nodes, metrics)).toEqual({ cpuMillicores: 4000, memoryBytes: 8 * 2 ** 30 })
    expect(utilizationPercent(1000, 4000)).toBe(25)
    expect(aggregateNodeAllocatable(nodes, [{ ...metrics[0], metadata: { name: 'missing' } }])).toBeNull()
    expect(utilizationPercent(100, 0)).toBeNull()
  })

  it('ranks bounded Pod samples by the selected real usage signal', () => {
    const items = [
      { metadata: { name: 'api', namespace: 'prod' }, timestamp: '2026-07-27T06:00:00Z', window: '30s', containers: [{ name: 'app', usage: { cpu: '250m', memory: '128Mi' } }] },
      { metadata: { name: 'cache', namespace: 'prod' }, timestamp: '2026-07-27T06:00:00Z', window: '30s', containers: [{ name: 'cache', usage: { cpu: '100m', memory: '512Mi' } }] },
      { metadata: { name: 'worker', namespace: 'jobs' }, timestamp: '2026-07-27T06:00:00Z', window: '30s', containers: [{ name: 'worker', usage: { cpu: '500m', memory: '64Mi' } }] },
    ]
    expect(rankPodMetrics(items, 'cpu', 2).map((item) => item.name)).toEqual(['worker', 'api'])
    expect(rankPodMetrics(items, 'memory', 2).map((item) => item.name)).toEqual(['cache', 'api'])
  })
})
