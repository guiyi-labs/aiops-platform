import { afterEach, describe, expect, it, vi } from 'vitest'

import { getAlertOverview } from './alert'

describe('alert API client (M114-1)', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('requests an alert overview with correct params', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      scope: 'alerts:overview',
      groups: [],
      total_firing: 0,
      total_resolved: 0,
      fail_closed: true,
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await getAlertOverview('token', 9, { window_minutes: 360, max_groups: 30 })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toContain('/clusters/9/alerts/overview')
    expect(path).toContain('window_minutes=360')
    expect(path).toContain('max_groups=30')
  })

  it('sends bearer auth with the alert overview request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ groups: [], fail_closed: true }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await getAlertOverview('tok-abc', 5)

    const [, init] = fetchMock.mock.calls[0] ?? []
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer tok-abc' })
  })
})