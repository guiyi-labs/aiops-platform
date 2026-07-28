import { afterEach, describe, expect, it, vi } from 'vitest'

import { getReadiness } from './health'

describe('getReadiness', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the health payload', async () => {
    const payload = {
      status: 'ready' as const,
      service: 'k8s-aiops-api',
      version: 'test',
      checked_at: '2026-07-16T12:00:00Z',
    }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify(payload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ))

    await expect(getReadiness()).resolves.toEqual(payload)
  })

  it('rejects an unavailable response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 503 })))

    await expect(getReadiness()).rejects.toThrow('status 503')
  })
})
