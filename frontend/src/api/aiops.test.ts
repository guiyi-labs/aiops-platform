import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  approveAutomationPlan,
  createAutomationPlan,
  createSLODefinition,
  evaluateSLO,
  executeAutomationPlan,
  generateInvestigation,
  getAIOpsOverview,
  getCorrelationCase,
  getInvestigation,
  getTopologyGraph,
  getQualityReport,
  listAutomationPlans,
  listCorrelationCases,
  listInvestigations,
  listSignals,
  listSLODefinitions,
  listSLITemplates,
  listTopologyChanges,
  previewAutomationPlan,
  runQualityReplay,
  verifyAutomationPlan,
} from './aiops'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

describe('aiops API client (M39-M45/M56)', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('fetches the AIOps overview with bearer auth', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      active_diagnoses: 3,
      partial: false,
      generated_at: '2026-08-02T10:00:00Z',
    }))
    vi.stubGlobal('fetch', fetchMock)

    const overview = await getAIOpsOverview('token')

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/overview')
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token' })
    expect(overview.active_diagnoses).toBe(3)
  })

  it('builds the signals query from the supplied filters only', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0, next_cursor: '' }))
    vi.stubGlobal('fetch', fetchMock)

    await listSignals('token', { cluster_id: 7, severity: 'critical', limit: 50, namespace: '' })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/signals?cluster_id=7&severity=critical&limit=50')
    // Empty namespace must be dropped, not serialized as an empty value.
    expect(path).not.toContain('namespace')
  })

  it('serializes topology graph params', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ edges: [], nodes: [] }))
    vi.stubGlobal('fetch', fetchMock)

    await getTopologyGraph('token', { cluster_id: 9, namespace: 'default' })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/topology/graph?cluster_id=9&namespace=default')
  })

  it('lists topology changes without params', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetchMock)

    await listTopologyChanges('token')

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/topology/changes')
  })

  it('creates an SLO definition via POST with a JSON body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 11, name: 'web-availability' }))
    vi.stubGlobal('fetch', fetchMock)

    const slo = await createSLODefinition('token', {
      cluster_id: 7,
      service: { cluster_id: 7, kind: 'Deployment', namespace: 'prod', name: 'web' },
      template: 'availability',
      objective: 0.99,
      rolling_window_seconds: 604800,
    })

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/slos')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toMatchObject({
      cluster_id: 7,
      template: 'availability',
      objective: 0.99,
    })
    expect((init as RequestInit).headers).toMatchObject({ 'Content-Type': 'application/json' })
    expect(slo.id).toBe(11)
  })

  it('lists SLO definitions with cluster filter', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetchMock)

    await listSLODefinitions('token', { cluster_id: 7, enabled: true })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/slos?cluster_id=7&enabled=true')
  })

  it('fetches SLI templates', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], template_version: 'v1' }))
    vi.stubGlobal('fetch', fetchMock)

    await listSLITemplates('token')

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/slos/templates')
  })

  it('evaluates an SLO via POST', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 5, slo_id: 11 }))
    vi.stubGlobal('fetch', fetchMock)

    await evaluateSLO('token', 11)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/slos/11/evaluate')
    expect(init).toMatchObject({ method: 'POST' })
  })

  it('lists correlation cases with filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetchMock)

    await listCorrelationCases('token', { status: 'open', limit: 25 })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/correlation/cases?status=open&limit=25')
  })

  it('fetches a correlation case detail', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 3, status: 'open' }))
    vi.stubGlobal('fetch', fetchMock)

    await getCorrelationCase('token', 3)

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/correlation/cases/3')
  })

  it('generates an investigation with optional runbook query', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 8, status: 'running' }))
    vi.stubGlobal('fetch', fetchMock)

    await generateInvestigation('token', 3, { runbook_id: 'rb-1' })

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/investigator/cases/3/investigations?runbook_id=rb-1')
    expect(init).toMatchObject({ method: 'POST' })
  })

  it('lists investigations for a case', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetchMock)

    await listInvestigations('token', 3)

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/investigator/cases/3/investigations')
  })

  it('fetches a single investigation', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 8, status: 'succeeded' }))
    vi.stubGlobal('fetch', fetchMock)

    await getInvestigation('token', 8)

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/investigator/investigations/8')
  })

  it('lists automation plans with status filter', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetchMock)

    await listAutomationPlans('token', { status: 'pending' })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans?status=pending')
  })

  it('walks the automation plan lifecycle endpoints', async () => {
    const respond = vi.fn()
    vi.stubGlobal('fetch', vi.fn((...args: unknown[]) => {
      respond(...args)
      return Promise.resolve(jsonResponse({ id: 'plan-1', status: 'running' }))
    }))

    await createAutomationPlan('token', {
      action_code: 'restart_workload',
      cluster_id: 7,
      target: { cluster_id: 7, kind: 'Deployment', namespace: 'prod', name: 'web' },
    })
    let [path, init] = respond.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans')
    expect(init).toMatchObject({ method: 'POST' })

    await previewAutomationPlan('token', 'plan-1')
    ;[path, init] = respond.mock.calls[1] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans/plan-1/preview')
    expect(init).toMatchObject({ method: 'POST' })

    await approveAutomationPlan('token', 'plan-1')
    ;[path, init] = respond.mock.calls[2] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans/plan-1/approve')
    expect(init).toMatchObject({ method: 'POST' })

    await executeAutomationPlan('token', 'plan-1', { confirmation_token: 'tok' })
    ;[path, init] = respond.mock.calls[3] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans/plan-1/execute')
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ confirmation_token: 'tok' })

    await verifyAutomationPlan('token', 'plan-1')
    ;[path, init] = respond.mock.calls[4] ?? []
    expect(path).toBe('/api/v1/aiops/automation/plans/plan-1/verify')
    expect(init).toMatchObject({ method: 'POST' })
  })

  it('fetches the quality report and triggers a replay', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ report_version: '2026.08.1', summary: {} }))
      .mockResolvedValueOnce(jsonResponse({ task_id: 'task-1', status: 'running', message: 'ok' }))
    vi.stubGlobal('fetch', fetchMock)

    const report = await getQualityReport('token')
    expect(report.report_version).toBe('2026.08.1')

    const replay = await runQualityReplay('token')
    expect(replay.task_id).toBe('task-1')
    expect(replay.status).toBe('running')

    const [path, init] = fetchMock.mock.calls[1] ?? []
    expect(path).toBe('/api/v1/aiops/quality-report/run')
    expect(init).toMatchObject({ method: 'POST' })
  })

  it('surfaces a failed AIOps request as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'AI_OPS_UNAVAILABLE', message: 'service disabled' }, 503,
    )))

    await expect(getAIOpsOverview('token')).rejects.toMatchObject({
      status: 503, code: 'AI_OPS_UNAVAILABLE', message: 'service disabled',
    })
  })
})
