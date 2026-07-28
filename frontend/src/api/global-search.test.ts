import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  createSavedGlobalSearchFilter,
  deleteSavedGlobalSearchFilter,
  listSavedGlobalSearchFilters,
  searchFleetResources,
  updateSavedGlobalSearchFilter,
} from './global-search'

describe('global search API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('encodes only the fixed bounded search shape', async () => {
    const payload = { query: 'api', kinds: ['Pod', 'Service'], items: [], total: 0, remaining: 0, clusters_total: 2, clusters_searched: 2, clusters_remaining: 0, complete: true, failures: [], checked_at: '2026-07-27T12:00:00Z', limits: { max_clusters: 20, max_concurrent_clusters: 4, per_cluster_timeout_ms: 4000, max_results: 100, per_kind_limit: 100 } }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(searchFleetResources('access-token', { query: 'api', namespace: 'prod', kinds: ['Pod', 'Service'], clusterLimit: 8, limit: 30 })).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/fleet/resources/search?q=api&namespace=prod&kinds=pods%2Cservices&cluster_limit=8&limit=30', expect.objectContaining({
      headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
    }))
  })

  it('manages only the fixed saved-filter resource', async () => {
    const saved = { id: 4, name: 'Production API', query: 'api', namespace: 'prod', kinds: ['Pod'], schema_version: 1, compatible: true, created_at: '2026-07-27T12:00:00Z', updated_at: '2026-07-27T12:00:00Z' }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [saved], total: 1, limit: 20 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(saved), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...saved, name: 'Renamed' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await listSavedGlobalSearchFilters('access-token')
    await createSavedGlobalSearchFilter('access-token', { name: 'Production API', query: 'api', namespace: 'prod', kinds: ['Pod'] })
    await updateSavedGlobalSearchFilter('access-token', 4, { name: 'Renamed' })
    await deleteSavedGlobalSearchFilter('access-token', 4)

    expect(fetchMock.mock.calls.map(([path, init]) => [path, init.method ?? 'GET', init.body])).toEqual([
      ['/api/v1/fleet/resources/search/filters', 'GET', undefined],
      ['/api/v1/fleet/resources/search/filters', 'POST', JSON.stringify({ name: 'Production API', query: 'api', namespace: 'prod', kinds: ['Pod'] })],
      ['/api/v1/fleet/resources/search/filters/4', 'PATCH', JSON.stringify({ name: 'Renamed' })],
      ['/api/v1/fleet/resources/search/filters/4', 'DELETE', undefined],
    ])
  })
})
