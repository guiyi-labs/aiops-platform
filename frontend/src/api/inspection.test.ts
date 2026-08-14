import { afterEach, describe, expect, it, vi } from 'vitest'

import { getInspectionCoverage } from './inspection'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

describe('inspection API client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('requests coverage without a window (server default 30 days)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      scope: 'inspection:plans+tasks+results:30d',
      observed_at: '2026-08-14T10:00:00Z',
      window_days: 30,
      plan_total: 2,
      plan_enabled: 1,
      task_total: 3,
      task_completed: 2,
      task_failed: 1,
      task_scheduled: 2,
      task_manual: 1,
      finding_total: 3,
      distinct_rule_codes: 2,
      by_severity: { critical: 2, warning: 1 },
      rule_coverage: 0.2,
      trend: [
        { day: '2026-08-13', tasks: 2, findings: 2 },
        { day: '2026-08-14', tasks: 1, findings: 1 },
      ],
      fail_closed: false,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const coverage = await getInspectionCoverage('token')

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/inspection/coverage')
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token' })
    expect(coverage.finding_total).toBe(3)
    expect(coverage.distinct_rule_codes).toBe(2)
    expect(coverage.rule_coverage).toBeCloseTo(0.2)
    expect(coverage.fail_closed).toBe(false)
    expect(coverage.trend).toHaveLength(2)
  })

  it('passes window_days when given', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      scope: 'inspection:plans+tasks+results:7d',
      window_days: 7,
      plan_total: 0,
      plan_enabled: 0,
      task_total: 0,
      task_completed: 0,
      task_failed: 0,
      task_scheduled: 0,
      task_manual: 0,
      finding_total: 0,
      distinct_rule_codes: 0,
      by_severity: {},
      rule_coverage: 0,
      trend: [],
      fail_closed: true,
      empty_note: 'window contains no inspection findings (fail-closed)',
    }))
    vi.stubGlobal('fetch', fetchMock)

    await getInspectionCoverage('token', { window_days: 7 })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/inspection/coverage?window_days=7')
  })
})