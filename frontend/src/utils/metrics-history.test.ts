import { describe, expect, it } from 'vitest'

import type { MetricHistoryCoverage, MetricHistoryPoint } from '../types/metrics-history'
import { buildMetricChart, formatMetricValue, metricCoverageIssues, metricCoveragePercent, metricDisplayUnit, metricHistoryWindow } from './metrics-history'

const point = (minute: number, value = 250_000_000): MetricHistoryPoint => ({
  value, source_timestamp: `2026-07-29T00:${String(minute).padStart(2, '0')}:00Z`, window_milliseconds: 15_000,
  collected_at: `2026-07-29T00:${String(minute).padStart(2, '0')}:00Z`,
})

const coverage = (overrides: Partial<MetricHistoryCoverage> = {}): MetricHistoryCoverage => ({
  collections: 4, succeeded: 4, partial: 0, unavailable: 0, timed_out: 0, failed: 0, points: 4, missing: 0, ...overrides,
})

describe('metrics history presentation', () => {
  it('builds separate SVG paths across sparse collection gaps instead of filling or connecting them', () => {
    const chart = buildMetricChart([point(0), point(1), point(3)], '2026-07-29T00:00:00Z', '2026-07-29T00:04:00Z', 'nanocores', 4)
    expect(chart.points).toHaveLength(3)
    expect(chart.segments).toHaveLength(2)
    expect(chart.segments.map((segment) => segment.pointCount)).toEqual([2, 1])
    expect(chart.segments[0].path).not.toContain('NaN')

    const unknownGapLocations = buildMetricChart([point(0), point(1), point(2)], '2026-07-29T00:00:00Z', '2026-07-29T01:00:00Z', 'nanocores', 4, true)
    expect(unknownGapLocations.segments.map((segment) => segment.pointCount)).toEqual([1, 1, 1])
  })

  it('uses explicit human-readable CPU and memory units', () => {
    expect(metricDisplayUnit('nanocores')).toBe('mCPU')
    expect(formatMetricValue(250_000_000, 'nanocores')).toBe('250 mCPU')
    expect(metricDisplayUnit('bytes')).toBe('MiB')
    expect(formatMetricValue(134_217_728, 'bytes')).toBe('128 MiB')
  })

  it('summarizes coverage loss and collection failure states independently', () => {
    const degraded = coverage({ collections: 10, points: 6, missing: 4, partial: 1, unavailable: 1, timed_out: 1, failed: 1 })
    expect(metricCoveragePercent(degraded)).toBe(60)
    expect(metricCoverageIssues(degraded)).toEqual(['缺样 4', '部分成功 1', '不可用 1', '超时 1', '失败 1'])
    expect(metricCoveragePercent(coverage({ collections: 0, points: 0 }))).toBeNull()
  })

  it('creates exact 1h, 6h and 24h RFC3339 query windows', () => {
    const now = new Date('2026-07-29T12:00:00.000Z')
    expect(metricHistoryWindow(now, 1).from).toBe('2026-07-29T11:00:00.000Z')
    expect(metricHistoryWindow(now, 6).from).toBe('2026-07-29T06:00:00.000Z')
    expect(metricHistoryWindow(now, 24).from).toBe('2026-07-28T12:00:00.000Z')
  })
})
