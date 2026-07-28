import { authorizedRequest } from './client'
import type { ManagedUser, PlatformRole, UserProfile } from '../types/auth'

export function listAssignableUsers(token: string): Promise<{ items: UserProfile[]; total: number; remaining: number }> {
  return authorizedRequest('/api/v1/users/assignable', token)
}

export function listUsers(token: string): Promise<{ items: ManagedUser[]; total: number; remaining: number }> {
  return authorizedRequest('/api/v1/users?limit=100', token)
}

export function createUser(token: string, input: { username: string; password: string; display_name: string; roles: PlatformRole[] }): Promise<ManagedUser> {
  return authorizedRequest('/api/v1/users', token, { method: 'POST', body: JSON.stringify(input) })
}

export function updateUser(token: string, userID: number, input: { display_name?: string; status?: 'active' | 'disabled'; roles?: PlatformRole[] }): Promise<ManagedUser> {
  return authorizedRequest(`/api/v1/users/${userID}`, token, { method: 'PATCH', body: JSON.stringify(input) })
}

export function resetUserPassword(token: string, userID: number, password: string): Promise<ManagedUser> {
  return authorizedRequest(`/api/v1/users/${userID}/password-reset`, token, { method: 'POST', body: JSON.stringify({ password }) })
}
