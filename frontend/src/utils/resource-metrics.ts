import type { NodeMetric, NodeResource, PodMetric, ResourceUsage } from '../types/kubernetes'

export interface ResourceMetricTotals { cpuMillicores: number; memoryBytes: number }
export interface PodMetricSummary extends ResourceMetricTotals { name: string; namespace: string; containers: number; timestamp: string }

const decimalMemoryUnits: Record<string, number> = { k: 1e3, K: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18 }
const binaryMemoryUnits: Record<string, number> = { Ki: 2 ** 10, Mi: 2 ** 20, Gi: 2 ** 30, Ti: 2 ** 40, Pi: 2 ** 50, Ei: 2 ** 60 }

function quantity(value: string): { amount: number; suffix: string } | null {
  const match = /^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)([a-zA-Z]*)$/.exec(value.trim())
  if (!match) return null
  const amount = Number(match[1])
  return Number.isFinite(amount) ? { amount, suffix: match[2] ?? '' } : null
}

export function cpuMillicores(value: string): number | null {
  const parsed = quantity(value)
  if (!parsed) return null
  const factors: Record<string, number> = { '': 1000, n: 1e-6, u: 1e-3, m: 1 }
  const factor = factors[parsed.suffix]
  return factor === undefined ? null : parsed.amount * factor
}

export function memoryBytes(value: string): number | null {
  const parsed = quantity(value)
  if (!parsed) return null
  if (parsed.suffix === '') return parsed.amount
  const factor = binaryMemoryUnits[parsed.suffix] ?? decimalMemoryUnits[parsed.suffix]
  return factor === undefined ? null : parsed.amount * factor
}

function addUsage(total: ResourceMetricTotals, usage: ResourceUsage): void {
  total.cpuMillicores += cpuMillicores(usage.cpu) ?? 0
  total.memoryBytes += memoryBytes(usage.memory) ?? 0
}

export function aggregateNodeMetrics(items: NodeMetric[]): ResourceMetricTotals {
  const total = { cpuMillicores: 0, memoryBytes: 0 }
  items.forEach((item) => addUsage(total, item.usage))
  return total
}

export function aggregatePodMetrics(items: PodMetric[]): ResourceMetricTotals {
  const total = { cpuMillicores: 0, memoryBytes: 0 }
  items.forEach((item) => item.containers.forEach((container) => addUsage(total, container.usage)))
  return total
}

export function aggregateNodeAllocatable(nodes: NodeResource[], metrics: NodeMetric[]): ResourceMetricTotals | null {
  if (metrics.length === 0) return null
  const byName = new Map(nodes.map((node) => [node.metadata.name, node]))
  const total = { cpuMillicores: 0, memoryBytes: 0 }
  for (const metric of metrics) {
    const allocatable = byName.get(metric.metadata.name)?.status.allocatable
    const cpu = cpuMillicores(allocatable?.cpu ?? '')
    const memory = memoryBytes(allocatable?.memory ?? '')
    if (cpu === null || memory === null || cpu <= 0 || memory <= 0) return null
    total.cpuMillicores += cpu
    total.memoryBytes += memory
  }
  return total
}

export function utilizationPercent(usage: number, capacity: number): number | null {
  if (!Number.isFinite(usage) || !Number.isFinite(capacity) || usage < 0 || capacity <= 0) return null
  return Math.round((usage / capacity) * 1000) / 10
}

export function rankPodMetrics(items: PodMetric[], mode: 'cpu' | 'memory', limit = 5): PodMetricSummary[] {
  const key = mode === 'cpu' ? 'cpuMillicores' : 'memoryBytes'
  return items
    .map((item) => {
      const total = aggregatePodMetrics([item])
      return { name: item.metadata.name, namespace: item.metadata.namespace ?? '', containers: item.containers.length, timestamp: item.timestamp, ...total }
    })
    .sort((left, right) => right[key] - left[key] || left.namespace.localeCompare(right.namespace) || left.name.localeCompare(right.name))
    .slice(0, Math.max(0, limit))
}

export function formatCPU(millicores: number): string {
  if (millicores >= 1000) return `${(millicores / 1000).toFixed(millicores >= 10000 ? 1 : 2).replace(/\.0+$/, '')} cores`
  return `${Math.round(millicores)}m`
}

export function formatMemory(bytes: number): string {
  const units = [{ label: 'TiB', size: 2 ** 40 }, { label: 'GiB', size: 2 ** 30 }, { label: 'MiB', size: 2 ** 20 }, { label: 'KiB', size: 2 ** 10 }]
  const unit = units.find((candidate) => bytes >= candidate.size) ?? units[units.length - 1]
  return `${(bytes / unit.size).toFixed(bytes >= unit.size * 10 ? 1 : 2).replace(/\.0+$/, '')} ${unit.label}`
}
