import { afterEach, describe, expect, it, vi } from 'vitest'

import { createCluster, listClusters, updateClusterCredential } from './clusters'

describe('cluster API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('uses the bearer token and returns the list envelope', async () => {
    const payload = { items: [], total: 0, remaining: 0 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listClusters('access-token')).resolves.toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/clusters', expect.objectContaining({
      headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
    }))
  })

  it('submits kubeconfig only in the create request body', async () => {
    const payload = { id: 1, name: 'dev', api_server: 'https://example.test', enabled: false, status: 'disabled', conditions: [] }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    await createCluster('access-token', 'dev', 'secret-config')
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.body).toBe(JSON.stringify({ name: 'dev', kubeconfig: 'secret-config' }))
    expect(JSON.stringify(payload)).not.toContain('secret-config')
  })

  it('rotates kubeconfig through the credential endpoint body', async () => {
    const payload = { id: 1, name: 'dev', api_server: 'https://new.example.test', enabled: true, status: 'unknown', conditions: [] }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await updateClusterCredential('access-token', 1, 'replacement-config')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/clusters/1/credentials', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ kubeconfig: 'replacement-config' }) }))
  })
})
