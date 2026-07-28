import { APIError } from './auth'
import type { APIErrorBody } from '../types/auth'

export async function authorizedRequest<T>(path: string, accessToken: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${accessToken}`,
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...init.headers,
    },
  })
  if (response.ok) {
    const text = await response.text()
    if (!text) return undefined as T
    return JSON.parse(text) as T
  }
  const body = await response.json().catch(() => undefined) as APIErrorBody | undefined
  throw new APIError(response.status, body?.code ?? 'REQUEST_FAILED', body?.message ?? `Request failed with status ${response.status}`, body?.request_id)
}
