<script setup lang="ts">
import { computed, ref } from 'vue'
import { Boxes, LockKeyhole } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'

import { APIError } from '../api/auth'
import { useCountUp } from '../composables/useCountUp'
import ParticleNetwork from '../components/ParticleNetwork.vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const passwordChanged = computed(() => route.query.password_changed === '1')

const statClusters = useCountUp(0, ref(12), { duration: 1400 })
const statNodes = useCountUp(0, ref(186), { duration: 1700 })
const statAccuracy = useCountUp(0, ref(99), { duration: 1500 })

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
      <ParticleNetwork />
      <div class="login-brand"><span><Boxes :size="22" /></span>K8s AIOps</div>

      <div class="login-copy">
        <p class="login-eyebrow">MULTI-CLUSTER OPERATIONS</p>
        <h1>让每一次故障判断<br /><em>都有证据可追溯</em></h1>
        <p class="login-description">统一接入 Kubernetes 集群，关联资源状态、事件与日志，以规则诊断为主、AI 解释为辅。</p>
      </div>

      <div class="login-visual">
        <div class="login-grid" aria-hidden="true"></div>
        <svg class="login-topology" viewBox="0 0 560 300" fill="none" aria-hidden="true" focusable="false">
          <defs>
            <linearGradient id="hubGrad" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stop-color="#5eead4" />
              <stop offset="1" stop-color="#818cf8" />
            </linearGradient>
            <linearGradient id="nodeGradA" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stop-color="#34d399" />
              <stop offset="1" stop-color="#2dd4bf" />
            </linearGradient>
            <linearGradient id="nodeGradB" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stop-color="#818cf8" />
              <stop offset="1" stop-color="#6366f1" />
            </linearGradient>
          </defs>

          <g class="topology-mesh">
            <line x1="96" y1="52" x2="470" y2="60" />
            <line x1="470" y1="60" x2="500" y2="226" />
            <line x1="500" y1="226" x2="376" y2="258" />
            <line x1="376" y1="258" x2="186" y2="258" />
            <line x1="186" y1="258" x2="60" y2="205" />
            <line x1="60" y1="205" x2="96" y2="52" />
          </g>

          <g class="topology-links">
            <line class="flow flow-a" x1="280" y1="150" x2="96" y2="52" />
            <line class="flow flow-b" x1="280" y1="150" x2="470" y2="60" />
            <line class="flow flow-c" x1="280" y1="150" x2="60" y2="205" />
            <line class="flow flow-d" x1="280" y1="150" x2="500" y2="226" />
            <line class="flow flow-e" x1="280" y1="150" x2="186" y2="258" />
            <line class="flow flow-f" x1="280" y1="150" x2="376" y2="258" />
          </g>

          <g class="topo-node tone-a" transform="translate(96 52)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradA)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b" transform="translate(470 60)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradB)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-a" transform="translate(60 205)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradA)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b" transform="translate(500 226)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradB)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-c" transform="translate(186 258)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="#f59e0b" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b" transform="translate(376 258)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradB)" />
            <circle class="node-dot" r="2.8" />
          </g>

          <g class="topo-hub" transform="translate(280 150)">
            <circle class="hub-wave" r="30" />
            <circle class="hub-ring" r="21" />
            <circle class="hub-core" r="15" fill="url(#hubGrad)" />
            <circle class="hub-dot" r="4.5" />
          </g>
        </svg>

        <div class="login-stats" role="group" aria-label="平台实时概况">
          <div class="login-stat">
            <i class="stat-pip pip-green"></i>
            <span>已接入集群</span>
            <strong>{{ statClusters.value }}<em> 套</em></strong>
          </div>
          <div class="login-stat">
            <i class="stat-pip pip-teal"></i>
            <span>在线节点</span>
            <strong>{{ statNodes.value }}<em> 个</em></strong>
          </div>
          <div class="login-stat">
            <i class="stat-pip pip-violet"></i>
            <span>诊断准确率</span>
            <strong>{{ statAccuracy.value }}<em> %</em></strong>
          </div>
        </div>
      </div>
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
