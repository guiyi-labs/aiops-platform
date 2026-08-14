import { afterEach, describe, expect, it, vi } from 'vitest'

import { getIncidentMetrics } from './incidents'

describe('incident metrics API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('serializes the bounded metrics window and cluster filter', async () => {
    const metrics = { window_days: 14, cluster_id: 3, sample_limit: 200, sampled: 2, truncated: false, assigned: 1, acknowledged: 2, resolved: 1, overdue: 0, sla_evaluated: 1, sla_compliant: 1, sla_compliance_rate: 1, first_assigned_seconds: 20, mtta_seconds: 40, mttr_seconds: 60 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(metrics), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentMetrics('token', { clusterID: 3, days: 14 })).resolves.toEqual(metrics)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/metrics?cluster_id=3&days=14', expect.any(Object))
  })

  it('keeps missing lifecycle samples as null', async () => {
    const metrics = { window_days: 30, cluster_id: 0, sample_limit: 200, sampled: 0, truncated: false, assigned: 0, acknowledged: 0, resolved: 0, overdue: 0, sla_evaluated: 0, sla_compliant: 0, sla_compliance_rate: null, first_assigned_seconds: null, mtta_seconds: null, mttr_seconds: null }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(metrics), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentMetrics('token')).resolves.toMatchObject({ mtta_seconds: null, mttr_seconds: null })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/metrics', expect.any(Object))
  })
})
