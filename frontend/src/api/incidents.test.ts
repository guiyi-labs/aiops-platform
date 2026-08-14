import { afterEach, describe, expect, it, vi } from 'vitest'

import { getIncidentMetrics, getIncidentRunbook } from './incidents'

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

describe('incident runbook API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reads the incident-scoped runbook without adding query guesses', async () => {
    const response = { incident_id: 7, available: true, domain: 'network', finding_code: 'NET-EXPOSE', runbook: { read_only: true } }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentRunbook('token', 7)).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/7/runbook', expect.any(Object))
  })

  it('preserves fail-closed unavailable responses', async () => {
    const response = { incident_id: 8, available: false, reason: 'domain_unavailable' }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentRunbook('token', 8)).resolves.toMatchObject(response)
  })
})
