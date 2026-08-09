import { afterEach, describe, expect, it, vi } from 'vitest'

import { getInsightRunbook } from './insight'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

describe('insight API client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('builds the closed-loop query from a finding and normalises empty lists', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 7,
      domain: 'network',
      finding_code: 'NET-EXPOSE',
      kind: 'Deployment',
      namespace: 'prod',
      name: 'api',
      diagnoses: null,
      inspection: null,
      operations: null,
      read_only: true,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const runbook = await getInsightRunbook('token', {
      clusterId: 7,
      domain: 'network',
      code: 'NET-EXPOSE',
      kind: 'Deployment',
      namespace: 'prod',
      name: 'api',
    })

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/insight?cluster_id=7&domain=network&kind=Deployment&name=api&code=NET-EXPOSE&namespace=prod')
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token' })
    expect(runbook.diagnoses).toEqual([])
    expect(runbook.inspection).toEqual([])
    expect(runbook.operations).toEqual([])
    expect(runbook.read_only).toBe(true)
  })

  it('drops empty optional params from the query string', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 1, domain: 'cis', kind: 'Node', name: 'worker-1', diagnoses: [], inspection: [], operations: [], read_only: true,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await getInsightRunbook('token', { clusterId: 1, domain: 'cis', kind: 'Node', name: 'worker-1' })

    const [path] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/aiops/insight?cluster_id=1&domain=cis&kind=Node&name=worker-1')
    expect(path).not.toContain('code=')
    expect(path).not.toContain('namespace=')
  })
})