import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  createClusterGrant,
  createNamespaceGrant,
  deleteClusterGrant,
  deleteNamespaceGrant,
  getMyGrants,
  listClusterGrants,
  listNamespaceGrants,
} from './grants'

describe('grants API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists cluster grants under /users/:user_id/cluster-grants', async () => {
    const payload = {
      items: [{ id: 1, user_id: 7, cluster_id: 11, created_at: '2026-08-01T00:00:00Z' }],
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listClusterGrants('access-token', 7)).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/users/7/cluster-grants',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
      }),
    )
  })

  it('creates a cluster grant with cluster_id in the body', async () => {
    const payload = { id: 2, user_id: 7, cluster_id: 42, created_at: '2026-08-01T00:00:00Z' }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    await createClusterGrant('access-token', 7, 42)
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ cluster_id: 42 }))
  })

  it('deletes a cluster grant via DELETE path', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await deleteClusterGrant('access-token', 7, 42)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/users/7/cluster-grants/42',
      expect.objectContaining({ method: 'DELETE' }),
    )
  })

  it('lists namespace grants under /users/:user_id/namespace-grants', async () => {
    const payload = {
      items: [{ id: 5, user_id: 7, cluster_id: 11, namespace: 'prod', created_at: '2026-08-01T00:00:00Z' }],
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listNamespaceGrants('access-token', 7)).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/users/7/namespace-grants',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
      }),
    )
  })

  it('creates a namespace grant posting cluster_id+namespace', async () => {
    const payload = { id: 6, user_id: 7, cluster_id: 11, namespace: 'staging', created_at: '2026-08-01T00:00:00Z' }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    await createNamespaceGrant('access-token', 7, 11, 'staging')
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ cluster_id: 11, namespace: 'staging' }))
  })

  it('deletes a namespace grant via path with URL-encoded namespace', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await deleteNamespaceGrant('access-token', 7, 11, 'prod-app')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/users/7/namespace-grants/11/prod-app',
      expect.objectContaining({ method: 'DELETE' }),
    )
  })

  it('deletes a namespace grant containing special characters safely', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await deleteNamespaceGrant('access-token', 7, 11, 'ns/with slash')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/users/7/namespace-grants/11/ns%2Fwith%20slash',
      expect.objectContaining({ method: 'DELETE' }),
    )
  })

  it('fetches own grants via the /auth/me/grants shortcut endpoint', async () => {
    const payload = {
      cluster_grants: [{ id: 1, user_id: 1, cluster_id: 11, created_at: '2026-08-01T00:00:00Z' }],
      namespace_grants: [{ id: 5, user_id: 1, cluster_id: 11, namespace: 'dev', created_at: '2026-08-01T00:00:00Z' }],
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(getMyGrants('access-token')).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/me/grants',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
      }),
    )
  })
})
