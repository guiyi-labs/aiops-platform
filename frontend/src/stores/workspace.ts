import { ref } from 'vue'
import { defineStore } from 'pinia'

import * as workspaceAPI from '../api/workspace'
import type { Workspace, WorkspaceScope } from '../types/workspace'

const SCOPE_STORAGE_KEY = 'workspace.scope'
const CURRENT_WORKSPACE_STORAGE_KEY = 'workspace.currentId'

function readScope(): WorkspaceScope {
  const stored = localStorage.getItem(SCOPE_STORAGE_KEY)
  if (stored === 'platform' || stored === 'workspace' || stored === 'cluster') {
    return stored
  }
  return 'platform'
}

function readCurrentWorkspaceId(): number | null {
  const stored = localStorage.getItem(CURRENT_WORKSPACE_STORAGE_KEY)
  if (stored === null || stored === '') return null
  const parsed = Number(stored)
  return Number.isFinite(parsed) ? parsed : null
}

export const useWorkspaceStore = defineStore('workspace', () => {
  const scope = ref<WorkspaceScope>(readScope())
  const currentWorkspaceId = ref<number | null>(readCurrentWorkspaceId())
  const workspaces = ref<Workspace[]>([])

  function persistScope(value: WorkspaceScope) {
    localStorage.setItem(SCOPE_STORAGE_KEY, value)
  }

  function persistCurrentWorkspaceId(value: number | null) {
    if (value === null) {
      localStorage.removeItem(CURRENT_WORKSPACE_STORAGE_KEY)
    } else {
      localStorage.setItem(CURRENT_WORKSPACE_STORAGE_KEY, String(value))
    }
  }

  async function init(token: string) {
    const result = await workspaceAPI.listWorkspaces(token)
    workspaces.value = result.items
  }

  function setScope(next: WorkspaceScope) {
    scope.value = next
    persistScope(next)
  }

  function setCurrentWorkspace(id: number | null) {
    currentWorkspaceId.value = id
    persistCurrentWorkspaceId(id)
    if (id !== null) {
      setScope('workspace')
    }
  }

  function clearWorkspace() {
    currentWorkspaceId.value = null
    persistCurrentWorkspaceId(null)
    setScope('platform')
  }

  return { scope, currentWorkspaceId, workspaces, init, setScope, setCurrentWorkspace, clearWorkspace }
})
