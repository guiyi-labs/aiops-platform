<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  BadgeCheck,
  BrainCircuit,
  CircleAlert,
  Gauge,
  KeyRound,
  PlugZap,
  Power,
  RefreshCw,
  Save,
  ShieldCheck,
} from 'lucide-vue-next'

import {
  getAISettings,
  testAISettings,
  updateAISettings,
  type AIProtocol,
  type AISettings,
} from '../api/aiSettings'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const errorMessage = ref('')
const notice = ref('')
const testResult = ref<{ ok: boolean; detail: string; latency?: number } | null>(null)
const current = ref<AISettings | null>(null)

// 表单状态。apiKey 为空 = 沿用已保存的密钥（界面不显示明文）。
const form = reactive({
  enabled: false,
  protocol: 'responses' as AIProtocol,
  baseUrl: '',
  model: '',
  apiKey: '',
  timeoutSeconds: 30,
  maxOutputTokens: 4096,
  dailyTokenBudget: 100000,
  maxConcurrentRequests: 2,
})

const runtime = computed(() => current.value?.runtime ?? null)
const apiKeyConfigured = computed(() => current.value?.api_key_set ?? false)

// 配置齐全但开关关着是最容易漏的一步：保存能成功、页面不报错，但 AI 全程不生效。
// 与其让人自己去对照「运行状态」卡，不如直接把这种状态拎到最上面并给一键启用。
const readyButDisabled = computed(() => {
  const s = current.value
  if (!s) return false
  return !s.enabled && s.base_url !== '' && s.model !== '' && s.api_key_set
})

const protocolHint = computed(() =>
  form.protocol === 'responses'
    ? 'OpenAI 官方与支持 Responses API 的网关。请求打到 {base_url}/responses。'
    : '绝大多数 OpenAI 兼容端点（DeepSeek、通义、Kimi、vLLM、Ollama 等）。请求打到 {base_url}/chat/completions。',
)

function applySettings(settings: AISettings) {
  current.value = settings
  form.enabled = settings.enabled
  form.protocol = settings.protocol === 'chat_completions' ? 'chat_completions' : 'responses'
  form.baseUrl = settings.base_url
  form.model = settings.model
  form.apiKey = ''
  form.timeoutSeconds = settings.request_timeout_seconds || 30
  form.maxOutputTokens = settings.max_output_tokens || 4096
  form.dailyTokenBudget = settings.daily_token_budget ?? 100000
  form.maxConcurrentRequests = settings.max_concurrent_requests || 2
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    applySettings(await getAISettings(auth.accessToken))
  } catch (error) {
    errorMessage.value = describe(error, '无法读取模型接入配置')
  } finally {
    loading.value = false
  }
}

async function runTest() {
  testing.value = true
  errorMessage.value = ''
  notice.value = ''
  testResult.value = null
  try {
    const result = await testAISettings(auth.accessToken, {
      protocol: form.protocol,
      base_url: form.baseUrl,
      model: form.model,
      // 留空时后端回落到已保存的密钥，避免为一次测试重复粘贴。
      api_key: form.apiKey,
      request_timeout_seconds: form.timeoutSeconds,
    })
    testResult.value = {
      ok: result.ok,
      detail: result.detail,
      latency: result.probe?.latency_ms,
    }
  } catch (error) {
    if (error instanceof APIError && error.status === 502) {
      testResult.value = { ok: false, detail: error.message }
    } else {
      testResult.value = { ok: false, detail: describe(error, '连通性测试失败') }
    }
  } finally {
    testing.value = false
  }
}

// 清除已保存的密钥：仅在关闭状态下允许（否则会留下必然失败的配置）。
async function clearKey() {
  saving.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    await updateAISettings(auth.accessToken, {
      enabled: form.enabled,
      protocol: form.protocol,
      base_url: form.baseUrl,
      model: form.model,
      api_key: '',
      clear_api_key: true,
      request_timeout_seconds: form.timeoutSeconds,
      max_output_tokens: form.maxOutputTokens,
      daily_token_budget: form.dailyTokenBudget,
      max_concurrent_requests: form.maxConcurrentRequests,
    })
    await load()
    notice.value = '已清除保存的 API 密钥。'
  } catch (error) {
    errorMessage.value = describe(error, '清除密钥失败')
  } finally {
    saving.value = false
  }
}

async function save() {
  saving.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const saved = await updateAISettings(auth.accessToken, {
      enabled: form.enabled,
      protocol: form.protocol,
      base_url: form.baseUrl,
      model: form.model,
      api_key: form.apiKey,
      request_timeout_seconds: form.timeoutSeconds,
      max_output_tokens: form.maxOutputTokens,
      daily_token_budget: form.dailyTokenBudget,
      max_concurrent_requests: form.maxConcurrentRequests,
    })
    // 保存后重新拉一次，拿到最新的 runtime 与 api_key_hint。
    await load()
    testResult.value = null
    notice.value = saved.enabled
      ? '配置已保存并立即生效（无需重启）。'
      : '配置已保存，但开关处于关闭状态——此状态下不会调用模型。'
  } catch (error) {
    errorMessage.value = describe(error, '保存失败')
  } finally {
    saving.value = false
  }
}

// 一键启用：沿用在用的配置，只把开关翻到 true 并立即保存。
async function enableNow() {
  form.enabled = true
  await save()
}

function describe(error: unknown, fallback: string): string {
  if (error instanceof APIError) {
    if (error.code === 'INVALID_BASE_URL') return '接入地址必须是完整的 http(s) URL。'
    if (error.code === 'INVALID_PROTOCOL') return '协议取值非法。'
    if (error.code === 'MISSING_MODEL') return '开启接入时必须填写模型名称。'
    if (error.code === 'MISSING_API_KEY') return '开启接入时必须填写 API 密钥。'
    return error.message || fallback
  }
  return error instanceof Error ? error.message : fallback
}

onMounted(load)
</script>

<template>
  <ConsoleLayout eyebrow="系统设置" title="模型接入">
    <template #actions>
      <button class="icon-button" type="button" aria-label="刷新配置" :disabled="loading" @click="load">
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </template>

    <div v-if="readyButDisabled" class="ai-enable-banner">
      <div>
        <strong>配置已就绪，但接入开关是关闭的</strong>
        <p class="muted small">
          地址、模型与密钥都已保存，当前不会调用模型——诊断详情页会回退到确定性诊断。
        </p>
      </div>
      <button class="primary-button" type="button" :disabled="saving" @click="enableNow">
        <Power :size="15" />立即启用
      </button>
    </div>

    <section class="ai-settings-status">
      <article class="ai-status-card">
        <header><BrainCircuit :size="16" /><span>运行状态</span></header>
        <strong :class="runtime?.available ? 'tone-ok' : 'tone-off'">
          {{ runtime?.enabled ? (runtime?.available ? '可用于解释' : '已启用但当前不可用') : '未启用' }}
        </strong>
        <p class="muted small">
          提供方 {{ runtime?.provider || '未配置' }} · 模型 {{ runtime?.model || '未配置' }}
        </p>
      </article>
      <article class="ai-status-card">
        <header><Gauge :size="16" /><span>今日用量</span></header>
        <strong>{{ runtime?.used_tokens_today ?? 0 }} / {{ runtime?.daily_token_budget ?? 0 }} token</strong>
        <p class="muted small">
          预留 {{ runtime?.reserved_tokens ?? 0 }} · 解释 {{ runtime?.explanation_count_today ?? 0 }} 次
        </p>
      </article>
      <article class="ai-status-card">
        <header><ShieldCheck :size="16" /><span>密钥</span></header>
        <strong>{{ apiKeyConfigured ? '已配置' : '未配置' }}</strong>
        <p class="muted small">只写不回显，界面仅显示末四位 {{ current?.api_key_hint || '——' }}</p>
      </article>
    </section>

    <section class="ai-settings-card">
      <header class="ai-settings-head">
        <div>
          <strong>模型接入配置</strong>
          <p class="muted small">
            保存后立即生效，无需重启服务。环境变量只在数据库为空时作为初值播种。
          </p>
        </div>
        <label class="ai-switch">
          <input v-model="form.enabled" type="checkbox" />
          <span>{{ form.enabled ? '已启用' : '已关闭' }}</span>
        </label>
      </header>

      <form class="ai-settings-form" @submit.prevent="save">
        <label class="field-wide">
          请求协议
          <select v-model="form.protocol">
            <option value="chat_completions">Chat Completions（OpenAI 兼容，推荐）</option>
            <option value="responses">Responses API（OpenAI 官方）</option>
          </select>
          <span class="field-hint">{{ protocolHint }}</span>
        </label>

        <label class="field-wide">
          接入地址（base_url）
          <input v-model="form.baseUrl" type="url" placeholder="https://api.example.com/v1" autocomplete="off" />
          <span class="field-hint">只写到 <code>/v1</code> 这一层，程序会自动拼接 <code>/chat/completions</code> 或 <code>/responses</code>。</span>
        </label>

        <label>
          模型名称
          <input v-model="form.model" type="text" placeholder="deepseek-chat / gpt-4o-mini" autocomplete="off" />
        </label>

        <label>
          API 密钥
          <input
            v-model="form.apiKey"
            type="password"
            autocomplete="new-password"
            :placeholder="apiKeyConfigured ? `已配置 ${current?.api_key_hint || ''}，留空表示不修改` : 'sk-...'"
          />
          <span class="field-hint">留空即保持原密钥不变；密钥以 AES-256-GCM 加密入库，接口永不回显。</span>
        </label>

        <label>
          请求超时（秒）
          <input v-model.number="form.timeoutSeconds" type="number" min="1" max="600" />
        </label>

        <label>
          单次最大输出 token
          <input v-model.number="form.maxOutputTokens" type="number" min="1" max="32768" />
          <span class="field-hint">
            中文模型的合规解释实测需 2400~2600 token，建议不低于 4000；填小会被截断并报
            「output truncated」。同时不能大于下面的日预算。
          </span>
        </label>

        <label>
          日 token 预算
          <input v-model.number="form.dailyTokenBudget" type="number" min="0" step="1000" />
          <span class="field-hint">超出预算后当天停止调用并回退到确定性诊断。</span>
        </label>

        <label>
          并发上限
          <input v-model.number="form.maxConcurrentRequests" type="number" min="1" max="64" />
        </label>

        <div class="ai-settings-actions">
          <button class="secondary-button" type="button" :disabled="testing || !form.baseUrl || !form.model" @click="runTest">
            <PlugZap :size="15" />{{ testing ? '正在测试…' : '测试连接' }}
          </button>
          <button
            v-if="apiKeyConfigured"
            class="danger-button"
            type="button"
            :disabled="saving || form.enabled"
            :title="form.enabled ? '请先关闭接入再清除密钥' : '清除已保存的密钥'"
            @click="clearKey"
          >
            清除密钥
          </button>
          <button class="primary-button" type="submit" :disabled="saving || loading">
            <Save :size="15" />{{ saving ? '保存中…' : '保存配置' }}
          </button>
        </div>
      </form>

      <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
      <p v-if="notice" class="user-notice">{{ notice }}</p>

      <div v-if="testResult" :class="['ai-probe-result', testResult.ok ? 'probe-ok' : 'probe-fail']">
        <component :is="testResult.ok ? BadgeCheck : CircleAlert" :size="16" />
        <div>
          <strong>{{ testResult.ok ? '连接成功' : '连接失败' }}</strong>
          <span>{{ testResult.detail }}</span>
          <span v-if="testResult.latency !== undefined" class="muted small">耗时 {{ testResult.latency }} ms</span>
        </div>
      </div>

      <div class="ai-settings-notes">
        <h4><KeyRound :size="14" />密钥与约束</h4>
        <ul>
          <li>密钥与集群 kubeconfig 复用同一套加密器（AES-256-GCM + 密钥版本轮换），接口只回末四位。</li>
          <li>模型输出受四重约束：证据引用强制校验、JSON Schema 严格模式、日 token 预算与并发准入、失败回退确定性诊断。</li>
          <li>关闭接入后，解释与调查链路回退到确定性结果，不会报错，也不会调用外部服务。</li>
          <li>本页为系统管理员专属；读取、保存、测试均写入审计日志。</li>
        </ul>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
/* 配置齐全但开关关闭时的横幅。用左侧色条而非整块高饱和底色：
   这是「提醒待办」不是「错误」，不该和真正的报错抢视觉权重。 */
.ai-enable-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 16px;
  margin-bottom: 16px;
  border: 1px solid var(--border-subtle);
  border-left: 3px solid var(--status-warning, #b45309);
  border-radius: 8px;
  background: var(--surface-raised);
}
.ai-enable-banner strong { display: block; margin-bottom: 2px; }
.ai-enable-banner p { margin: 0; }

.ai-settings-status {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.ai-status-card {
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: var(--surface-raised);
  padding: 14px 16px;
}
.ai-status-card header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 12px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  margin-bottom: 8px;
}
.ai-status-card strong {
  display: block;
  font-size: 18px;
  letter-spacing: -0.01em;
}
.tone-ok { color: var(--status-success, #0f766e); }
.tone-off { color: var(--text-secondary); }

.ai-settings-card {
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: var(--surface-raised);
  padding: 18px 20px;
}
.ai-settings-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.ai-switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  white-space: nowrap;
}
.ai-settings-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 14px 18px;
}
.ai-settings-form label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-secondary);
}
.ai-settings-form .field-wide { grid-column: 1 / -1; }
.ai-settings-form input,
.ai-settings-form select {
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--surface-base);
  color: var(--text-primary);
  font-size: 13px;
}
.field-hint {
  font-size: 12px;
  color: var(--text-tertiary, var(--text-secondary));
  line-height: 1.5;
}
.ai-settings-actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  margin-top: 4px;
}
.ai-probe-result {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin-top: 14px;
  padding: 12px 14px;
  border-radius: 8px;
  font-size: 13px;
}
.ai-probe-result strong { display: block; margin-bottom: 2px; }
.ai-probe-result span { display: block; }
.probe-ok { background: var(--status-success-bg, #ecfdf5); color: var(--status-success-text, #065f46); }
.probe-fail { background: var(--status-danger-bg, #fef2f2); color: var(--status-danger-text, #991b1b); }
.ai-settings-notes {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-subtle);
  font-size: 13px;
  color: var(--text-secondary);
}
.ai-settings-notes h4 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  margin-bottom: 8px;
}
.ai-settings-notes ul { margin: 0; padding-left: 18px; display: grid; gap: 5px; }
</style>
