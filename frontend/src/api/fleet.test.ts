import { afterEach, describe, expect, it, vi } from 'vitest'

import { getFleetHealth } from './fleet'

describe('fleet API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('requests the bounded authenticated health comparison', async () => {
    const payload = { items: [], total: 0, remaining: 0, checked_at: '2026-07-27T12:00:00Z', limits: { max_clusters: 20, max_concurrent_clusters: 4, per_cluster_timeout_ms: 4000, resource_sample_limit: 100 } }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(getFleetHealth('access-token', 12)).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/fleet/health?limit=12', expect.objectContaining({
      headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
    }))
  })
})
