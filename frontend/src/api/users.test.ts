import { afterEach, describe, expect, it, vi } from 'vitest'

import { createUser, listAssignableUsers, listUsers, resetUserPassword, updateUser } from './users'

describe('users API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists users eligible for diagnosis assignment', async () => {
    const response = { items: [{ id: 1, username: 'admin', display_name: 'Administrator', roles: ['system_admin'] }], total: 1, remaining: 0 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listAssignableUsers('token')).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/users/assignable', expect.any(Object))
  })

  it('lists, creates and safely updates managed users', async () => {
    const managed = { id: 2, username: 'operator', display_name: 'Operator', roles: ['operations_admin'], status: 'active' }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [managed], total: 1, remaining: 0 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(managed), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...managed, status: 'disabled' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await listUsers('token')
    await createUser('token', { username: 'operator', password: 'secure-password', display_name: 'Operator', roles: ['operations_admin'] })
    await updateUser('token', 2, { status: 'disabled' })
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/users?limit=100', expect.any(Object))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/users', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/users/2', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ status: 'disabled' }) }))
  })

  it('resets a managed user password without exposing it in the URL', async () => {
    const managed = { id: 2, username: 'operator', display_name: 'Operator', roles: ['operations_admin'], status: 'active' }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(managed), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await resetUserPassword('token', 2, 'replacement-password')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/users/2/password-reset', expect.objectContaining({ method: 'POST', body: JSON.stringify({ password: 'replacement-password' }) }))
  })
})
