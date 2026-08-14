import { afterEach, describe, expect, it, vi } from 'vitest'

import { evaluateMetricHistory, getMetricHistory, getMetricHistoryArchive } from './metrics-history'

describe('metrics history API client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('sends an exact Pod container series with an inclusive/exclusive bounded window', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ points: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await getMetricHistory('token', 17, {
      resourceKind: 'Pod', namespace: 'team one', name: 'api/canary', container: 'web sidecar', metric: 'cpu',
      from: '2026-07-29T00:00:00.000Z', to: '2026-07-29T06:00:00.000Z',
    })

    const target = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://console.test')
    expect(target.pathname).toBe('/api/v1/clusters/17/metrics/history')
    expect(Object.fromEntries(target.searchParams)).toEqual({
      resource_kind: 'Pod', name: 'api/canary', metric: 'cpu', from: '2026-07-29T00:00:00.000Z',
      to: '2026-07-29T06:00:00.000Z', limit: '1440', namespace: 'team one', container: 'web sidecar',
    })
    expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ headers: expect.objectContaining({ Authorization: 'Bearer token' }) }))
  })

  it('omits Pod-only selectors for a Node and preserves stable API errors', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ points: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 'METRICS_HISTORY_QUERY_FAILED', message: 'unable to query metric history' }), { status: 500 }))
    vi.stubGlobal('fetch', fetchMock)
    const query = { resourceKind: 'Node' as const, name: 'worker-1', metric: 'memory' as const, from: '2026-07-29T00:00:00Z', to: '2026-07-29T01:00:00Z', limit: 61 }

    await getMetricHistory('token', 9, query)
    const target = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://console.test')
    expect(target.searchParams.get('resource_kind')).toBe('Node')
    expect(target.searchParams.get('namespace')).toBeNull()
    expect(target.searchParams.get('container')).toBeNull()
    expect(target.searchParams.get('limit')).toBe('61')
    await expect(getMetricHistory('token', 9, query)).rejects.toMatchObject({ status: 500, code: 'METRICS_HISTORY_QUERY_FAILED', message: 'unable to query metric history' })
  })

  it('routes 7d/30d windows to the downsampled archive endpoint (M114-3)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ points: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await getMetricHistoryArchive('token', 17, {
      resourceKind: 'Node', name: 'worker-1', metric: 'cpu',
      from: '2026-07-15T00:00:00.000Z', to: '2026-08-14T00:00:00.000Z',
    })

    const target = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://console.test')
    expect(target.pathname).toBe('/api/v1/clusters/17/metrics/history/archive')
    expect(Object.fromEntries(target.searchParams)).toEqual({
      resource_kind: 'Node', name: 'worker-1', metric: 'cpu', from: '2026-07-15T00:00:00.000Z',
      to: '2026-08-14T00:00:00.000Z', limit: '1440',
    })
    expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ headers: expect.objectContaining({ Authorization: 'Bearer token' }) }))
  })

  it('sends one fixed sustained-window evaluation without a query language', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ state: 'normal' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await evaluateMetricHistory('token', 9, {
      resourceKind: 'Node', name: 'worker-1', metric: 'cpu', from: '2026-07-29T00:00:00Z', to: '2026-07-29T00:05:00Z',
      operator: 'gte', threshold: 500_000_000, forSeconds: 300,
    })
    const target = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://console.test')
    expect(target.pathname).toBe('/api/v1/clusters/9/metrics/history/evaluate')
    expect(Object.fromEntries(target.searchParams)).toMatchObject({ operator: 'gte', threshold: '500000000', for_seconds: '300', minimum_points: '2' })
    expect(target.searchParams.has('limit')).toBe(false)
  })
})
