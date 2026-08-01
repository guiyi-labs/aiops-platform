import { authorizedRequest } from './client'
import type { GitOpsCapability, GitOpsApplication } from '../types/gitops'

export function getGitOpsCapability(token: string, clusterId: number): Promise<GitOpsCapability> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/gitops/capability`, token)
}

export function listGitOpsApplications(token: string, clusterId: number): Promise<{ items: GitOpsApplication[]; total: number; remaining: number }> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/gitops/applications`, token)
}

export function getGitOpsApplication(token: string, clusterId: number, name: string): Promise<GitOpsApplication> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/gitops/applications/${name}`, token)
}
