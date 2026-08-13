<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ArrowRight,
  Boxes,
  Check,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  ShieldCheck,
  UserRound,
} from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'

import { APIError } from '../api/auth'
import ParticleNetwork from '../components/ParticleNetwork.vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const showPassword = ref(false)
const focusedField = ref<'username' | 'password' | null>(null)
const interactionState = ref<'idle' | 'submitting' | 'success' | 'error'>('idle')
const activeCapability = ref<'governance' | 'diagnosis' | 'audit' | null>(null)
const passwordChanged = computed(() => route.query.password_changed === '1')
const authPhase = computed(() => {
  if (interactionState.value !== 'idle') return interactionState.value
  return focusedField.value ?? 'idle'
})

function prefersReducedMotion() {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
}

function waitForSuccessTransition() {
  if (prefersReducedMotion()) return Promise.resolve()
  return new Promise<void>((resolve) => window.setTimeout(resolve, 520))
}

function focusField(field: 'username' | 'password') {
  focusedField.value = field
  if (interactionState.value === 'error') {
    interactionState.value = 'idle'
    errorMessage.value = ''
  }
}

function clearFieldFocus(field: 'username' | 'password') {
  if (focusedField.value === field) focusedField.value = null
}

function clearError() {
  if (interactionState.value !== 'error') return
  interactionState.value = 'idle'
  errorMessage.value = ''
}

async function submit() {
  if (submitting.value) return
  submitting.value = true
  interactionState.value = 'submitting'
  errorMessage.value = ''
  try {
    await auth.login(username.value, password.value)
    interactionState.value = 'success'
    await waitForSuccessTransition()
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (error) {
    interactionState.value = 'error'
    if (error instanceof APIError && error.code === 'INVALID_CREDENTIALS') {
      errorMessage.value = '用户名或密码不正确'
    } else if (error instanceof APIError && error.code === 'USER_DISABLED') {
      errorMessage.value = '该账号已被停用，请联系系统管理员'
    } else {
      errorMessage.value = '暂时无法登录，请检查服务状态后重试'
    }
  } finally {
    submitting.value = false
    if (interactionState.value === 'submitting') interactionState.value = 'idle'
  }
}
</script>

<template>
  <main :class="['login-page', `login-page--${authPhase}`]" :data-auth-phase="authPhase">
    <section class="login-intro">
      <ParticleNetwork :phase="authPhase" />
      <div class="login-brand"><span><Boxes :size="22" /></span>K8s AIOps</div>

      <div class="login-radar" aria-hidden="true">
        <svg viewBox="0 0 360 360" fill="none" focusable="false">
          <g class="radar-rings">
            <circle cx="180" cy="180" r="52" />
            <circle cx="180" cy="180" r="96" />
            <circle cx="180" cy="180" r="140" />
            <circle cx="180" cy="180" r="176" />
          </g>
          <g class="radar-cross">
            <line x1="4" y1="180" x2="356" y2="180" />
            <line x1="180" y1="4" x2="180" y2="356" />
            <line x1="66" y1="66" x2="294" y2="294" />
            <line x1="294" y1="66" x2="66" y2="294" />
          </g>
          <g class="radar-sweep-g">
            <path class="radar-wedge" d="M180 180 L180 8 A172 172 0 0 1 322.5 83 Z" />
            <line class="radar-sweep-line" x1="180" y1="180" x2="180" y2="8" />
          </g>
          <g class="radar-blips">
            <circle class="blip blip-a" cx="252" cy="92" r="3.5" />
            <circle class="blip blip-b" cx="118" cy="268" r="3" />
            <circle class="blip blip-c" cx="292" cy="238" r="2.6" />
          </g>
          <circle class="radar-center" cx="180" cy="180" r="5" />
        </svg>
      </div>

      <div class="login-copy">
        <p class="login-eyebrow">MULTI-CLUSTER OPERATIONS</p>
        <h1>让每一次故障判断<br /><em>都有证据可追溯</em></h1>
        <p class="login-description">统一接入 Kubernetes 集群，关联资源状态、事件与日志，以规则诊断为主、AI 解释为辅。</p>
        <ul class="login-features" role="list">
          <li><i aria-hidden="true"></i>RULES-DRIVEN</li>
          <li><i aria-hidden="true"></i>EVIDENCE-FIRST</li>
          <li><i aria-hidden="true"></i>AUDIT-CLOSED</li>
        </ul>
      </div>

      <div class="login-visual" :data-capability="activeCapability || undefined">
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

          <g class="topo-node tone-a node-governance" transform="translate(96 52)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradA)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b node-diagnosis" transform="translate(470 60)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradB)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-a node-governance" transform="translate(60 205)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradA)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b node-diagnosis" transform="translate(500 226)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="url(#nodeGradB)" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-c node-audit" transform="translate(186 258)">
            <circle class="node-halo" r="16" />
            <circle class="node-core" r="9" fill="#f59e0b" />
            <circle class="node-dot" r="2.8" />
          </g>
          <g class="topo-node tone-b node-audit" transform="translate(376 258)">
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

        <div class="login-capabilities" role="list" aria-label="平台核心能力">
          <div
            :class="['login-capability', { 'is-active': activeCapability === 'governance' }]"
            role="listitem"
            @mouseenter="activeCapability = 'governance'"
            @mouseleave="activeCapability = null"
          >
            <i class="capability-pip pip-green" aria-hidden="true"></i>
            <span>多集群治理</span>
            <strong>统一视图</strong>
          </div>
          <div
            :class="['login-capability', { 'is-active': activeCapability === 'diagnosis' }]"
            role="listitem"
            @mouseenter="activeCapability = 'diagnosis'"
            @mouseleave="activeCapability = null"
          >
            <i class="capability-pip pip-teal" aria-hidden="true"></i>
            <span>诊断链路</span>
            <strong>证据优先</strong>
          </div>
          <div
            :class="['login-capability', { 'is-active': activeCapability === 'audit' }]"
            role="listitem"
            @mouseenter="activeCapability = 'audit'"
            @mouseleave="activeCapability = null"
          >
            <i class="capability-pip pip-violet" aria-hidden="true"></i>
            <span>变更控制</span>
            <strong>审计闭环</strong>
          </div>
        </div>

        <footer class="login-footer" aria-hidden="true">
          <span class="login-footer-copy">© 2026 K8s AIOps · Evidence-first Operations</span>
          <span class="login-footer-tags">
            <i aria-hidden="true"></i>SIGNAL-DRIVEN
            <i aria-hidden="true"></i>AUDIT-CLOSED
          </span>
        </footer>
      </div>
    </section>

    <section class="login-form-panel">
      <form class="login-card" :aria-busy="submitting" @submit.prevent="submit">
        <div class="login-card-rail" aria-hidden="true"><i></i></div>
        <span class="login-icon"><LockKeyhole :size="22" /></span>
        <p class="context-label">安全访问</p>
        <h2>登录运维控制台</h2>
        <p class="form-hint">使用平台账号继续</p>
        <p v-if="passwordChanged" class="login-success">密码已更新，请使用新密码重新登录。</p>

        <label for="username">用户名</label>
        <div :class="['login-field', { 'is-focused': focusedField === 'username' }]">
          <UserRound :size="17" aria-hidden="true" />
          <input
            id="username"
            v-model="username"
            name="username"
            autocomplete="username"
            minlength="3"
            maxlength="64"
            required
            autofocus
            @focus="focusField('username')"
            @blur="clearFieldFocus('username')"
            @input="clearError"
          />
        </div>
        <label for="password">密码</label>
        <div :class="['login-field', { 'is-focused': focusedField === 'password' }]">
          <KeyRound :size="17" aria-hidden="true" />
          <input
            id="password"
            v-model="password"
            name="password"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="current-password"
            minlength="8"
            maxlength="128"
            required
            @focus="focusField('password')"
            @blur="clearFieldFocus('password')"
            @input="clearError"
          />
          <button
            class="password-visibility"
            type="button"
            :aria-label="showPassword ? '隐藏密码' : '显示密码'"
            :title="showPassword ? '隐藏密码' : '显示密码'"
            @click="showPassword = !showPassword"
          >
            <EyeOff v-if="showPassword" :size="17" aria-hidden="true" />
            <Eye v-else :size="17" aria-hidden="true" />
          </button>
        </div>

        <p v-if="errorMessage" class="login-error" role="alert">{{ errorMessage }}</p>
        <button class="login-submit" type="submit" :disabled="submitting">
          <span class="login-submit-progress" aria-hidden="true"></span>
          <LoaderCircle v-if="authPhase === 'submitting'" class="login-submit-spinner" :size="17" aria-hidden="true" />
          <Check v-else-if="authPhase === 'success'" :size="17" aria-hidden="true" />
          <ArrowRight v-else :size="17" aria-hidden="true" />
          <span class="login-submit-label" aria-live="polite">
            {{ authPhase === 'success' ? '认证通过' : authPhase === 'submitting' ? '正在建立安全会话' : '进入控制台' }}
          </span>
        </button>
        <p class="login-security-status"><ShieldCheck :size="15" aria-hidden="true" />安全会话保护已启用</p>
      </form>
    </section>
  </main>
</template>
