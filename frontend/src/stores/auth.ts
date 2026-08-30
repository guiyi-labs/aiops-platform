import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import * as authAPI from '../api/auth'
import type { UserProfile } from '../types/auth'

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref('')
  const user = ref<UserProfile | null>(null)
  const initialized = ref(false)

  const isAuthenticated = computed(() => Boolean(accessToken.value && user.value))
  const userInitials = computed(() => {
    const label = user.value?.display_name || user.value?.username || ''
    return label.slice(0, 2).toUpperCase() || '--'
  })

  function applySession(session: Awaited<ReturnType<typeof authAPI.login>>) {
    accessToken.value = session.access_token
    user.value = session.user
  }

  async function login(username: string, password: string) {
    applySession(await authAPI.login(username.trim(), password))
    // Mark session as initialized so the router guard does not immediately
    // call restore() -> refreshSession(), which would race with the login
    // response and potentially clear the freshly-set auth state.
    initialized.value = true
  }

  async function restore() {
    if (initialized.value) return
    try {
      applySession(await authAPI.refreshSession())
    } catch {
      accessToken.value = ''
      user.value = null
    } finally {
      initialized.value = true
    }
  }

  async function logout() {
    try {
      await authAPI.logout()
    } finally {
      accessToken.value = ''
      user.value = null
    }
  }

  return { accessToken, user, initialized, isAuthenticated, userInitials, login, restore, logout }
})
