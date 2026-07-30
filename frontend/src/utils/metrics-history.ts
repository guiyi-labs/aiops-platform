import type { MetricHistoryCoverage, MetricHistoryPoint, MetricHistoryRangeHours, MetricUnit } from '../types/metrics-history'

export interface MetricChartSegment {
  path: string
  pointCount: number
}

export interface MetricChartModel {
  segments: MetricChartSegment[]
  points: Array<{ x: number; y: number; value: number; collectedAt: string }>
  maxValue: number
}

export const METRIC_CHART = { width: 720, height: 190, left: 12, right: 12, top: 14, bottom: 20 } as const

export function metricHistoryWindow(now: Date, hours: MetricHistoryRangeHours): { from: string; to: string } {
  return { from: new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString(), to: now.toISOString() }
}

export function metricDisplayUnit(unit: MetricUnit): string {
  return unit === 'nanocores' ? 'mCPU' : 'MiB'
}

export function metricDisplayValue(value: number, unit: MetricUnit): number {
  return unit === 'nanocores' ? value / 1_000_000 : value / 1_048_576
}

export function formatMetricValue(value: number, unit: MetricUnit): string {
  const display = metricDisplayValue(value, unit)
  const digits = display >= 100 ? 0 : display >= 10 ? 1 : 2
  return `${display.toFixed(digits)} ${metricDisplayUnit(unit)}`
}

export function metricCoveragePercent(coverage: MetricHistoryCoverage): number | null {
  if (coverage.collections <= 0) return null
  return Math.min(100, Math.round((coverage.points / coverage.collections) * 100))
}

export function metricCoverageIssues(coverage: MetricHistoryCoverage): string[] {
  const issues: string[] = []
  if (coverage.missing > 0) issues.push(`缺样 ${coverage.missing}`)
  if (coverage.partial > 0) issues.push(`部分成功 ${coverage.partial}`)
  if (coverage.unavailable > 0) issues.push(`不可用 ${coverage.unavailable}`)
  if (coverage.timed_out > 0) issues.push(`超时 ${coverage.timed_out}`)
  if (coverage.failed > 0) issues.push(`失败 ${coverage.failed}`)
  return issues
}

function collectionInterval(points: MetricHistoryPoint[], fromMs: number, toMs: number, collections: number): number {
  const deltas = points.slice(1).map((point, index) => Date.parse(point.collected_at) - Date.parse(points[index].collected_at)).filter((delta) => delta > 0).sort((a, b) => a - b)
  const median = deltas.length ? deltas[Math.floor(deltas.length / 2)] : Number.POSITIVE_INFINITY
  const coverageInterval = collections > 0 ? (toMs - fromMs) / collections : Number.POSITIVE_INFINITY
  const interval = Math.min(median, coverageInterval)
  return Number.isFinite(interval) && interval > 0 ? interval : 60_000
}

export function buildMetricChart(points: MetricHistoryPoint[], from: string, to: string, unit: MetricUnit, collections: number, disconnectAll = false): MetricChartModel {
  const fromMs = Date.parse(from)
  const toMs = Date.parse(to)
  const sorted = [...points].filter((point) => Number.isFinite(Date.parse(point.collected_at)) && point.value >= 0).sort((a, b) => Date.parse(a.collected_at) - Date.parse(b.collected_at))
  const displayValues = sorted.map((point) => metricDisplayValue(point.value, unit))
  const maxValue = Math.max(1, ...displayValues) * 1.08
  const plotWidth = METRIC_CHART.width - METRIC_CHART.left - METRIC_CHART.right
  const plotHeight = METRIC_CHART.height - METRIC_CHART.top - METRIC_CHART.bottom
  const duration = Math.max(1, toMs - fromMs)
  const chartPoints = sorted.map((point, index) => ({
    x: METRIC_CHART.left + Math.max(0, Math.min(1, (Date.parse(point.collected_at) - fromMs) / duration)) * plotWidth,
    y: METRIC_CHART.top + (1 - displayValues[index] / maxValue) * plotHeight,
    value: point.value,
    collectedAt: point.collected_at,
  }))
  const gapThreshold = Math.max(2_000, collectionInterval(sorted, fromMs, toMs, collections) * 1.75)
  const groups: typeof chartPoints[] = []
  chartPoints.forEach((point) => {
    const group = groups.at(-1)
    const previous = group?.at(-1)
    if (!group || disconnectAll || (previous && Date.parse(point.collectedAt) - Date.parse(previous.collectedAt) > gapThreshold)) groups.push([point])
    else group.push(point)
  })
  return {
    segments: groups.map((group) => ({ path: group.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' '), pointCount: group.length })),
    points: chartPoints,
    maxValue,
  }
}
