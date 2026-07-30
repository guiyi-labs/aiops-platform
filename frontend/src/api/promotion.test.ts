import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIError } from './auth'
import { executePromotion, getPromotion, listPromotions, previewPromotion } from './promotion'
import type { PromotionPlan } from '../types/promotion'

const token = 'test-token'

function mockFetchOnce(body: unknown, status = 200): void {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
    json: async () => body,
  }))
}

function lastCall(): { url: string; init: RequestInit } {
  const calls = (fetch as ReturnType<typeof vi.fn>).mock.calls
  const last = calls[calls.length - 1]
  return { url: last[0] as string, init: last[1] as RequestInit }
}

afterEach(() => vi.unstubAllGlobals())

describe('previewPromotion', () => {
  it('POSTs the preview request and returns the plan', async () => {
    const plan: PromotionPlan = {
      id: 'plan-1', source_cluster_id: 1, destination_cluster_id: 2,
      source_namespace: 'demo', destination_namespace: 'staging',
      status: 'awaiting_confirmation', bundle_summary: {} as never,
      dependency_summary: [], expires_at: '2026-07-29T12:00:00Z',
      created_at: '2026-07-29T11:00:00Z', updated_at: '2026-07-29T11:00:00Z',
      confirmation_token: 'token-abc',
    }
    mockFetchOnce(plan, 201)

    const result = await previewPromotion(token, {
      source_cluster_id: 1, destination_cluster_id: 2,
      source_namespace: 'demo', destination_namespace: 'staging',
      bundle: [{ kind: 'Deployment', namespace: 'demo', name: 'api' }],
    })

    expect(result).toEqual(plan)
    const { url, init } = lastCall()
    expect(url).toBe('/api/v1/promotions/preview')
    expect(init.method).toBe('POST')
    expect(init.headers).toMatchObject({ Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' })
  })

  it('throws APIError on validation failure', async () => {
    mockFetchOnce({ code: 'INVALID_PROMOTION', message: 'invalid' }, 400)
    await expect(previewPromotion(token, {
      source_cluster_id: 1, destination_cluster_id: 1,
      source_namespace: 'demo', destination_namespace: 'staging',
      bundle: [],
    })).rejects.toThrow(APIError)
  })
})

describe('executePromotion', () => {
  it('sends confirmation token and idempotency key', async () => {
    const plan: PromotionPlan = {
      id: 'plan-1', source_cluster_id: 1, destination_cluster_id: 2,
      source_namespace: 'demo', destination_namespace: 'staging',
      status: 'succeeded', bundle_summary: {} as never,
      dependency_summary: [], expires_at: '2026-07-29T12:00:00Z',
      created_at: '2026-07-29T11:00:00Z', updated_at: '2026-07-29T11:00:00Z',
    }
    mockFetchOnce(plan)

    const result = await executePromotion(token, 'plan-1', 'confirm-token', 'idem-key-1234')
    expect(result.status).toBe('succeeded')
    const { url, init } = lastCall()
    expect(url).toBe('/api/v1/promotions/plan-1/execute')
    expect(init.method).toBe('POST')
    expect(init.headers).toMatchObject({ 'Idempotency-Key': 'idem-key-1234' })
    expect(JSON.parse(init.body as string)).toEqual({ confirmation_token: 'confirm-token' })
  })
})

describe('getPromotion', () => {
  it('GETs a single plan by id', async () => {
    mockFetchOnce({ id: 'plan-2', status: 'expired' } as PromotionPlan)
    const result = await getPromotion(token, 'plan-2')
    expect(result.id).toBe('plan-2')
    const { url, init } = lastCall()
    expect(url).toBe('/api/v1/promotions/plan-2')
    expect(init.method).toBeUndefined()
  })
})

describe('listPromotions', () => {
  it('builds query with source_cluster_id and namespace', async () => {
    mockFetchOnce({ items: [], total: 0 })
    await listPromotions(token, 5, 'demo')
    const { url } = lastCall()
    expect(url).toContain('source_cluster_id=5')
    expect(url).toContain('namespace=demo')
  })

  it('omits namespace when not provided', async () => {
    mockFetchOnce({ items: [], total: 0 })
    await listPromotions(token, 5)
    const { url } = lastCall()
    expect(url).toContain('source_cluster_id=5')
    expect(url).not.toContain('namespace=')
  })
})
