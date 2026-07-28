import type { APIErrorBody, AuthSession, RefreshSession, UserProfile } from '../types/auth'

export class APIError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId = '',
  ) {
    super(message)
    this.name = 'APIError'
  }
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (response.ok) {
    return response.json() as Promise<T>
  }

  let body: APIErrorBody | undefined
  try {
    body = (await response.json()) as APIErrorBody
  } catch {
    // Keep a stable client-side error when a proxy returns a non-JSON response.
  }
  throw new APIError(
    response.status,
    body?.code ?? 'REQUEST_FAILED',
    body?.message ?? `Request failed with status ${response.status}`,
    body?.request_id,
  )
}

export async function login(username: string, password: string): Promise<AuthSession> {
  const response = await fetch('/api/v1/auth/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  return parseResponse<AuthSession>(response)
}

export async function refreshSession(): Promise<AuthSession> {
  const response = await fetch('/api/v1/auth/refresh', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })
  return parseResponse<AuthSession>(response)
}

export async function getCurrentUser(accessToken: string): Promise<UserProfile> {
  const response = await fetch('/api/v1/auth/me', {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` },
  })
  return parseResponse<UserProfile>(response)
}

export async function logout(): Promise<void> {
  const response = await fetch('/api/v1/auth/logout', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })
  if (!response.ok && response.status !== 401) {
    await parseResponse<never>(response)
  }
}

export async function changePassword(accessToken: string, currentPassword: string, newPassword: string): Promise<void> {
  const response = await fetch('/api/v1/auth/password-change', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json', Authorization: `Bearer ${accessToken}` },
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
  if (!response.ok) await parseResponse<never>(response)
}

export async function listSessions(accessToken: string): Promise<{ items: RefreshSession[]; total: number; remaining: number }> {
  const response = await fetch('/api/v1/auth/sessions', { credentials: 'same-origin', headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` } })
  return parseResponse(response)
}

export async function revokeSession(accessToken: string, sessionID: number): Promise<void> {
  const response = await fetch(`/api/v1/auth/sessions/${sessionID}`, { method: 'DELETE', credentials: 'same-origin', headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` } })
  if (!response.ok) await parseResponse<never>(response)
}

export async function revokeOtherSessions(accessToken: string): Promise<{ revoked: number }> {
  const response = await fetch('/api/v1/auth/sessions/revoke-others', { method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` } })
  return parseResponse(response)
}
