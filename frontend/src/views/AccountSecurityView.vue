<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { KeyRound, MonitorSmartphone, RefreshCw, ShieldCheck, Trash2 } from 'lucide-vue-next'
import { useRouter } from 'vue-router'

import { APIError, changePassword, listSessions, revokeOtherSessions, revokeSession } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { RefreshSession } from '../types/auth'

const auth = useAuthStore()
const router = useRouter()
const currentPassword = ref('')
const newPassword = ref('')
const confirmation = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const sessions = ref<RefreshSession[]>([])
const sessionsLoading = ref(false)
const sessionNotice = ref('')

async function submit() {
  errorMessage.value = ''
  if (newPassword.value !== confirmation.value) { errorMessage.value = '两次输入的新密码不一致。'; return }
  if (newPassword.value.length < 12) { errorMessage.value = '新密码至少需要 12 个字符。'; return }
  submitting.value = true
  try {
    await changePassword(auth.accessToken, currentPassword.value, newPassword.value)
    await auth.logout()
    await router.replace({ name: 'login', query: { password_changed: '1' } })
  } catch (error) {
    if (error instanceof APIError && error.code === 'CURRENT_PASSWORD_INVALID') errorMessage.value = '当前密码不正确。'
    else if (error instanceof APIError && error.code === 'PASSWORD_UNCHANGED') errorMessage.value = '新密码不能与当前密码相同。'
    else errorMessage.value = '密码修改失败，请稍后重试。'
  } finally { submitting.value = false }
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function loadSessions() {
  sessionsLoading.value = true
  try { sessions.value = (await listSessions(auth.accessToken)).items }
  catch { errorMessage.value = '无法加载登录会话。' }
  finally { sessionsLoading.value = false }
}

async function revoke(item: RefreshSession) {
  if (item.current) return
  try { await revokeSession(auth.accessToken, item.id); sessionNotice.value = '指定会话已撤销。'; await loadSessions() }
  catch { errorMessage.value = '会话撤销失败。' }
}

async function revokeOthers() {
  try { const result = await revokeOtherSessions(auth.accessToken); sessionNotice.value = `已撤销 ${result.revoked} 个其他会话。`; await loadSessions() }
  catch { errorMessage.value = '无法撤销其他会话，请确认当前刷新会话仍有效。' }
}

onMounted(loadSessions)
</script>

<template>
  <ConsoleLayout eyebrow="账号安全" title="修改密码">
    <section class="account-security-card">
      <header><span><ShieldCheck :size="20" /></span><div><strong>更新登录凭据</strong><p>成功后当前访问令牌及全部刷新会话立即失效，需要重新登录。</p></div></header>
      <form @submit.prevent="submit">
        <label>当前密码<input v-model="currentPassword" type="password" autocomplete="current-password" minlength="8" maxlength="128" required /></label>
        <label>新密码<input v-model="newPassword" type="password" autocomplete="new-password" minlength="12" maxlength="128" required /></label>
        <label>确认新密码<input v-model="confirmation" type="password" autocomplete="new-password" minlength="12" maxlength="128" required /></label>
        <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
        <button class="primary-button" type="submit" :disabled="submitting || !currentPassword || newPassword.length < 12 || !confirmation"><KeyRound :size="15" />{{ submitting ? '正在更新…' : '修改密码并退出' }}</button>
      </form>
    </section>
    <section class="account-security-card session-card">
      <header><span><MonitorSmartphone :size="20" /></span><div><strong>登录会话</strong><p>仅展示仍有效的刷新会话，不显示令牌或摘要。</p></div><button class="icon-button" type="button" aria-label="刷新会话列表" :disabled="sessionsLoading" @click="loadSessions"><RefreshCw :size="16" :class="{ spinning: sessionsLoading }" /></button></header>
      <p v-if="sessionNotice" class="user-notice">{{ sessionNotice }}</p>
      <div class="session-toolbar"><span>有效会话 · {{ sessions.length }}</span><button class="secondary-button" type="button" :disabled="sessionsLoading || sessions.filter((item) => !item.current).length === 0" @click="revokeOthers">撤销全部其他会话</button></div>
      <div class="session-list"><article v-for="item in sessions" :key="item.id"><div><strong>{{ item.user_agent || '未知客户端' }}</strong><span>{{ item.ip_address || '未知地址' }} · 登录于 {{ formatTime(item.created_at) }} · 到期 {{ formatTime(item.expires_at) }}</span></div><em v-if="item.current">当前会话</em><button v-else class="danger-button" type="button" aria-label="撤销会话" @click="revoke(item)"><Trash2 :size="14" />撤销</button></article><p v-if="!sessionsLoading && sessions.length === 0" class="resource-empty">暂无有效刷新会话</p></div>
    </section>
  </ConsoleLayout>
</template>
