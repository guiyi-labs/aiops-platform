import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIError, changePassword, listSessions, login, refreshSession, revokeOtherSessions, revokeSession } from './auth'

describe('auth API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('returns a login session and requests cookie credentials', async () => {
    const session = {
      access_token: 'access',
      token_type: 'Bearer' as const,
      expires_in: 900,
      user: { id: 1, username: 'admin', display_name: 'Administrator', roles: ['system_admin'] },
    }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(session), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(login('admin', 'change_me_now')).resolves.toEqual(session)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/auth/login', expect.objectContaining({
      method: 'POST',
      credentials: 'same-origin',
    }))
  })

  it('preserves the server error code', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 'REFRESH_TOKEN_INVALID',
      message: 'refresh session is invalid or expired',
      request_id: 'request-1',
    }), { status: 401 })))

    await expect(refreshSession()).rejects.toEqual(expect.objectContaining<Partial<APIError>>({
      status: 401,
      code: 'REFRESH_TOKEN_INVALID',
      requestId: 'request-1',
    }))
  })

  it('changes password with the access token and body-only credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await changePassword('access-token', 'current-password', 'replacement-password')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/auth/password-change', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ current_password: 'current-password', new_password: 'replacement-password' }),
      headers: expect.objectContaining({ Authorization: 'Bearer access-token' }),
    }))
  })

  it('lists and revokes only the selected refresh sessions', async () => {
    const payload = { items: [{ id: 4, user_agent: 'browser', ip_address: '127.0.0.1', current: true, created_at: '2026-07-17T00:00:00Z', expires_at: '2026-07-24T00:00:00Z' }], total: 1, remaining: 0 }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(payload), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ revoked: 2 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listSessions('token')).resolves.toEqual(payload)
    await revokeSession('token', 5)
    await expect(revokeOtherSessions('token')).resolves.toEqual({ revoked: 2 })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/auth/sessions/5', expect.objectContaining({ method: 'DELETE' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/auth/sessions/revoke-others', expect.objectContaining({ method: 'POST' }))
  })
})
