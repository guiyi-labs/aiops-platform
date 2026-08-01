import { authorizedRequest } from './client'
import type { Workspace, WorkspaceMembership, WorkspaceQuota, WorkspaceRoleBinding } from '../types/workspace'

export function listWorkspaces(token: string): Promise<{ items: Workspace[]; total: number }> {
  return authorizedRequest('/api/v1/workspaces', token)
}

export function createWorkspace(token: string, input: { name: string; display_name: string; metadata?: Record<string, unknown> }): Promise<Workspace> {
  return authorizedRequest('/api/v1/workspaces', token, { method: 'POST', body: JSON.stringify(input) })
}

export function getWorkspace(token: string, id: number): Promise<Workspace> {
  return authorizedRequest(`/api/v1/workspaces/${id}`, token)
}

export function patchWorkspace(token: string, id: number, patch: { display_name?: string; metadata?: Record<string, unknown> }): Promise<Workspace> {
  return authorizedRequest(`/api/v1/workspaces/${id}`, token, { method: 'PATCH', body: JSON.stringify(patch) })
}

export function deleteWorkspace(token: string, id: number): Promise<void> {
  return authorizedRequest(`/api/v1/workspaces/${id}`, token, { method: 'DELETE' })
}

export function listWorkspaceMemberships(token: string, workspaceId: number): Promise<{ items: WorkspaceMembership[] }> {
  return authorizedRequest(`/api/v1/workspaces/${workspaceId}/memberships`, token)
}

export function listWorkspaceQuota(token: string, workspaceId: number): Promise<WorkspaceQuota> {
  return authorizedRequest(`/api/v1/workspaces/${workspaceId}/quota`, token)
}

export function listWorkspaceRoleBindings(token: string, workspaceId: number): Promise<{ items: WorkspaceRoleBinding[] }> {
  return authorizedRequest(`/api/v1/workspaces/${workspaceId}/role-bindings`, token)
}
