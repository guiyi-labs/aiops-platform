import { afterEach, describe, expect, it, vi } from 'vitest'

import { exportIncidentPostmortem, getIncidentContext, getIncidentMetrics, getIncidentRunbook, listIncidentResponseCatalog } from './incidents'

describe('incident response catalog API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('loads templates and the configured severity matrix', async () => {
    const catalog = { templates: [{ id: 'generic' }], severity_matrix: [{ severity: 'critical', target_minutes: 60 }] }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(catalog), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listIncidentResponseCatalog('token')).resolves.toEqual(catalog)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/templates', expect.any(Object))
  })
})

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

describe('incident context cockpit API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('loads the aggregated M112-1 cockpit with the resource-context contract', async () => {
    const response = {
      resource_context: { scope: { cluster_id: 3, kind: 'Deployment', name: 'api' }, observed_at: '2026-08-14T00:00:00Z', source: 'incident_snapshot', freshness: { age_seconds: 300, as_of: '2026-08-14T00:00:00Z' }, empty_sample: { count: 0, bounded: true, semantic: 'fail_closed' } },
      incident: { id: 7, number: 'INC-000007', title: 'deployment unavailable' },
      sla: { due_at: '2026-08-14T04:00:00Z', overdue: false, remaining: '2小时', deadline_text: '剩余 2小时' },
      health: { status: 'confirmed', overdue: false, evidence_available: true, runbook_available: true, note_count: 1, system_event_count: 2 },
      evidence_sources: [{ source_type: 'diagnosis', count: 1, deep_link: '/diagnoses' }],
      recent_events: [],
      recommended_actions: [{ action: 'deployment.rollout_restart', target_kind: 'Deployment', dry_run_first: true, summary: 'Preview rollout restart' }],
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentContext('token', 7)).resolves.toMatchObject({ resource_context: { empty_sample: { semantic: 'fail_closed' } } })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/7/context', expect.any(Object))
  })

  it('keeps a missing cockpit fail-closed (no runbook claimed)', async () => {
    const response = {
      resource_context: { scope: { cluster_id: 3 }, observed_at: '2026-08-14T00:00:00Z', source: 'incident_snapshot', freshness: { age_seconds: 0, as_of: '2026-08-14T00:00:00Z' }, empty_sample: { count: 0, bounded: true, semantic: 'fail_closed' } },
      incident: { id: 8, number: 'INC-000008', title: 'missing source' },
      sla: { due_at: '2026-08-14T00:00:00Z', overdue: false, remaining: '--', deadline_text: '未设置' },
      health: { status: 'open', overdue: false, evidence_available: true, runbook_available: false, note_count: 0, system_event_count: 1 },
      evidence_sources: [],
      recent_events: [],
      recommended_actions: [],
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(getIncidentContext('token', 8)).resolves.toMatchObject({ health: { runbook_available: false } })
  })
})

describe('incident postmortem export API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('downloads the Markdown filename provided by the API', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('# INC-000007\n', {
      status: 200,
      headers: { 'Content-Type': 'text/markdown', 'Content-Disposition': 'attachment; filename="incident-INC-000007-postmortem.md"' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await exportIncidentPostmortem('token', 7)
    expect(result.filename).toBe('incident-INC-000007-postmortem.md')
    await expect(result.blob.text()).resolves.toContain('INC-000007')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/incidents/7/postmortem/export', expect.objectContaining({ headers: expect.objectContaining({ Accept: 'text/markdown' }) }))
  })
})
