<script setup lang="ts">
import { computed, ref } from 'vue'
import { Boxes, LockKeyhole, ShieldCheck } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'

import { APIError } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const passwordChanged = computed(() => route.query.password_changed === '1')

async function submit() {
  if (submitting.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    await auth.login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (error) {
    if (error instanceof APIError && error.code === 'INVALID_CREDENTIALS') {
      errorMessage.value = '用户名或密码不正确'
    } else if (error instanceof APIError && error.code === 'USER_DISABLED') {
      errorMessage.value = '该账号已被停用，请联系系统管理员'
    } else {
      errorMessage.value = '暂时无法登录，请检查服务状态后重试'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-intro">
      <div class="login-brand"><span><Boxes :size="22" /></span>K8s AIOps</div>
      <div>
        <p class="login-eyebrow">MULTI-CLUSTER OPERATIONS</p>
        <h1>让每一次故障判断<br />都有证据可追溯</h1>
        <p class="login-description">统一接入 Kubernetes 集群，关联资源状态、事件与日志，以规则诊断为主、AI 解释为辅。</p>
      </div>
      <div class="login-boundary"><ShieldCheck :size="18" /><span>AI 仅提供分析建议，不会直接执行集群变更</span></div>
    </section>

    <section class="login-form-panel">
      <form class="login-card" @submit.prevent="submit">
        <span class="login-icon"><LockKeyhole :size="22" /></span>
        <p class="context-label">安全访问</p>
        <h2>登录运维控制台</h2>
        <p class="form-hint">使用平台账号继续</p>
        <p v-if="passwordChanged" class="login-success">密码已更新，请使用新密码重新登录。</p>

        <label for="username">用户名</label>
        <input id="username" v-model="username" name="username" autocomplete="username" minlength="3" maxlength="64" required autofocus />
        <label for="password">密码</label>
        <input id="password" v-model="password" name="password" type="password" autocomplete="current-password" minlength="8" maxlength="128" required />

        <p v-if="errorMessage" class="login-error" role="alert">{{ errorMessage }}</p>
        <button class="login-submit" type="submit" :disabled="submitting">
          {{ submitting ? '正在验证…' : '登录' }}
        </button>
        <p class="security-note">会话凭据通过 HttpOnly Cookie 轮换，访问令牌不会持久化到浏览器存储。</p>
      </form>
    </section>
  </main>
</template>
