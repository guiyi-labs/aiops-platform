<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { listIntegrationTools, validateIntegrationTool, type IntegrationTool, type ToolValidation } from '../api/integrations'
import { listClusters } from '../api/clusters'
import { getReadiness } from '../api/health'
import { getKnowledgeStats } from '../api/knowledge'
import { getAIRuntimeStatus } from '../api/diagnosis'
import { listPods, listNodes, listEvents, listNodeMetrics } from '../api/kubernetes'
import { useAuthStore } from '../stores/auth'
import { downloadJSON } from '../utils/download'
const auth = useAuthStore()
const tools = ref<IntegrationTool[]>([])
const loading = ref(false)
const error = ref('')
const selectedTool = ref('get_diagnosis')
const argumentsText = ref('{\n  "diagnosis_id": "1"\n}')
const validating = ref(false)
const result = ref<ToolValidation | null>(null)
const validationError = ref('')
const checking = ref(false)
const checkedAt = ref('')
type Check = { name: string; state: 'ready' | 'notice' | 'failed'; detail: string; path: string }
const checks = ref<Check[]>([])
let alive = true
let sequence = 0
const selected = computed(() => tools.value.find(item => item.name === selectedTool.value))
const labels: Record<string, string> = { list_diagnoses: '诊断列表', get_diagnosis: '诊断详情', get_diagnosis_evidence: '诊断证据回放', search_knowledge: '案例查询', knowledge_stats: '案例统计', get_cluster: '集群信息' }
onBeforeUnmount(() => { alive = false; sequence++ })
async function load() {
  loading.value = true; error.value = ''
  try { const response = await listIntegrationTools(auth.accessToken); if (alive) tools.value = response.items }
  catch (err) { if (alive) error.value = err instanceof Error ? err.message : '工具目录读取失败' }
  finally { if (alive) loading.value = false }
}
function selectTool() {
  sequence++; validating.value = false; result.value = null; validationError.value = ''
  argumentsText.value = JSON.stringify(Object.fromEntries((selected.value?.arguments ?? []).filter(arg => arg.required).map(arg => [arg.name, '1'])), null, 2)
}
function preset(mode: 'valid' | 'unknown' | 'path') {
  sequence++; validating.value = false; result.value = null; validationError.value = ''; selectedTool.value = 'get_diagnosis'
  argumentsText.value = JSON.stringify(mode === 'unknown' ? { diagnosis_id: '1', execute: 'true' } : { diagnosis_id: mode === 'path' ? '../../auth' : '1' }, null, 2)
}
async function validate() {
  const request = ++sequence
  result.value = null; validationError.value = ''; validating.value = true
  try {
    const args: unknown = JSON.parse(argumentsText.value)
    if (!args || typeof args !== 'object' || Array.isArray(args) || Object.values(args).some(value => typeof value !== 'string')) throw new Error('参数必须是 JSON 对象，每个参数值须为字符串。')
    const response = await validateIntegrationTool(auth.accessToken, selectedTool.value, args as Record<string, string>)
    if (alive && request === sequence) result.value = response
  } catch (err) { if (alive && request === sequence) validationError.value = err instanceof Error ? err.message : '校验失败，请重试' }
  finally { if (alive && request === sequence) validating.value = false }
}
async function checkEnvironment() {
  if (checking.value) return
  checking.value = true; checks.value = []; checkedAt.value = ''
  const output: Check[] = []
  const check = async (name: string, path: string, action: () => Promise<{ detail: string; state?: Check['state'] }>) => {
    try { const response = await action(); output.push({ name, path, state: response.state ?? 'ready', detail: response.detail }) }
    catch (err) { output.push({ name, path, state: 'failed', detail: err instanceof Error ? err.message : '读取失败' }) }
  }
  await Promise.allSettled([
    check('服务就绪', '/', async () => { const response = await getReadiness(); return { detail: `${response.service} · ${response.status}`, state: ['ok', 'ready'].includes(response.status) ? 'ready' : 'failed' } }),
    check('案例记忆', '/cases', async () => { const response = await getKnowledgeStats(auth.accessToken); return { detail: `${response.total} 条已归档案例`, state: response.total ? 'ready' : 'notice' } }),
    check('可选 AI 解释', '/settings/model', async () => { const response = await getAIRuntimeStatus(auth.accessToken); return { detail: response.enabled ? `${response.model} · ${response.available ? '可用' : '当前受限'}` : '尚未启用，规则诊断可独立演示', state: response.enabled && response.available ? 'ready' : 'notice' } }),
    check('集群与多源采集', '/workloads', async () => {
      const clusters = (await listClusters(auth.accessToken)).items
      const cluster = clusters.find(item => item.enabled)
      if (!cluster) return { detail: '未接入启用的集群，请先完成集群配置', state: 'notice' }
      const sources = await Promise.allSettled([listPods(auth.accessToken, cluster.id), listNodes(auth.accessToken, cluster.id), listEvents(auth.accessToken, cluster.id), listNodeMetrics(auth.accessToken, cluster.id)])
      const names = ['Pod 状态', 'Node 状态', 'Event 事件', '资源指标']
      const detail = sources.map((source, index) => `${names[index]}：${source.status === 'fulfilled' ? `${source.value.items.length} 条` : '读取失败'}`).join('；')
      return { detail: `${cluster.name} · ${detail}`, state: sources.some(item => item.status === 'rejected') ? 'failed' : sources.some(item => item.status === 'fulfilled' && item.value.items.length === 0) ? 'notice' : 'ready' }
    }),
  ])
  if (!alive) return
  checks.value = output.sort((left, right) => left.name.localeCompare(right.name, 'zh-CN')); checkedAt.value = new Date().toISOString(); checking.value = false
}
onMounted(load)
</script>
<template>
  <section class="integration-view">
    <header class="integration-hero"><span>RUNTIME / READ-ONLY TOOLS</span><h2>运行检查与工具接入</h2><p>先确认演示所需的数据和服务，再查看 MCP 网关实际公布的只读能力与参数边界。</p></header>
    <section class="panel environment-check"><header><div><h3>演示环境检查</h3><p>读取当前服务、集群信号、案例库与 AI 状态。</p></div><button class="primary-button" :disabled="checking" @click="checkEnvironment">{{ checking ? '正在检查…' : '检查演示环境' }}</button></header><div v-if="checks.length" class="check-grid"><article v-for="check in checks" :key="check.name" :class="check.state"><span>{{ check.state === 'ready' ? '可用' : check.state === 'notice' ? '需准备' : '需处理' }}</span><h4>{{ check.name }}</h4><p>{{ check.detail }}</p><RouterLink :to="check.path">打开相关页面 →</RouterLink></article></div><footer v-if="checkedAt"><span>检查时间 {{ new Date(checkedAt).toLocaleString('zh-CN') }} · 当前快照</span><button class="secondary-button" @click="downloadJSON('environment-check.json', { checked_at: checkedAt, checks })">导出检查结果</button></footer></section>
    <p v-if="loading" role="status">正在读取工具目录…</p><p v-if="error" role="alert" class="error-message">{{ error }} <button class="secondary-button" @click="load">重试</button></p>
    <section class="panel tool-catalog"><header><h3>只读 MCP 工具目录 · {{ tools.length }}</h3><span>GET · stdio 网关</span></header><p>目录与独立 aiops-mcp 进程共享。此页检查参数规则；客户端连接状态需在对应 MCP 客户端确认。</p><div class="catalog-grid"><article v-for="tool in tools" :key="tool.name"><h4>{{ labels[tool.name] || tool.name }}</h4><code>{{ tool.name }}</code><p>{{ tool.method }} {{ tool.path }}</p><small>参数：{{ tool.arguments.map(arg => arg.name + (arg.required ? '（必填）' : '')).join('、') || '无' }}</small></article></div></section>
    <section class="panel tool-validator"><header><h3>参数校验演示</h3><span>只校验，不执行请求</span></header><div class="preset-buttons"><button class="secondary-button" :disabled="validating" @click="preset('valid')">载入合法参数</button><button class="secondary-button" :disabled="validating" @click="preset('unknown')">载入未声明参数</button><button class="secondary-button" :disabled="validating" @click="preset('path')">载入越界路径</button></div><form @submit.prevent="validate"><label>选择工具<select v-model="selectedTool" :disabled="validating || !tools.length" @change="selectTool"><option v-for="tool in tools" :key="tool.name" :value="tool.name">{{ labels[tool.name] || tool.name }} · {{ tool.name }}</option></select></label><label>调用参数 JSON<textarea v-model="argumentsText" aria-label="工具调用参数 JSON" rows="5" maxlength="4000" :disabled="validating" @input="result = null" /></label><button class="primary-button" :disabled="validating || !tools.length">{{ validating ? '正在校验…' : '校验参数' }}</button></form><p v-if="validationError" role="alert" class="error-message">{{ validationError }}</p><div v-if="result" class="tool-result" role="status"><strong>{{ result.allowed ? '参数校验通过' : '调用已被拦截' }}</strong><p>{{ result.allowed ? '工具名称与参数符合网关声明；未向后端发起工具请求，也未检查目标对象是否存在。' : result.reason }}</p><code v-if="result.path">{{ result.method }} {{ result.path }}</code></div></section>
    <details class="panel integration-config"><summary>MCP 客户端接入方式</summary><p>在部署主机编译 aiops-mcp，客户端通过 stdio 启动它。API 地址填写可访问的后端地址，令牌使用该用户的有效访问令牌。</p><pre tabindex="0" aria-label="MCP 客户端配置示例">{{ JSON.stringify({ mcpServers: { aiops: { command: '/absolute/path/to/aiops-mcp', env: { AIOPS_API_URL: 'http://127.0.0.1:18080', AIOPS_API_TOKEN: '<user-access-token>' } } } }, null, 2) }}</pre></details>
  </section>
</template>
<style scoped>
.integration-hero { margin-bottom: 20px; }
.integration-hero > span { font: 10px var(--font-mono); letter-spacing: 1px; color: var(--accent-strong); }
h2 { font-size: 21px; margin: 12px 0; }
h3 { font-size: 15px; margin: 0; }
h4 { margin: 10px 0; font-size: 13px; }
p { font-size: 12px; line-height: 1.9; color: var(--text-secondary); }
.integration-view > .panel { margin-top: 18px; padding: 22px; }
header, footer { display: flex; justify-content: space-between; gap: 16px; align-items: center; flex-wrap: wrap; }
header > span, footer > span { font-size: 11px; color: var(--text-secondary); }
.check-grid, .catalog-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-top: 18px; }
.check-grid article, .catalog-grid article { padding: 16px; border: 1px solid var(--border-subtle); border-radius: 8px; min-width: 0; }
.check-grid article > span { font-size: 11px; color: var(--status-success); }
.check-grid article.notice > span { color: var(--status-warning); }
.check-grid article.failed > span { color: var(--status-danger); }
a { color: var(--accent-strong); font-size: 12px; }
footer { margin-top: 18px; }
code { font: 11px/1.8 var(--font-mono); overflow-wrap: anywhere; }
small { font-size: 11px; color: var(--text-secondary); }
.preset-buttons { display: flex; gap: 10px; margin: 18px 0; flex-wrap: wrap; }
form { display: grid; gap: 16px; }
form > label { display: grid; gap: 10px; font-size: 12px; min-width: 0; }
select, textarea { width: 100%; max-width: 100%; box-sizing: border-box; font: 12px/1.8 var(--font-mono); }
form > button { justify-self: start; }
.tool-result { margin-top: 18px; padding: 16px; background: var(--accent-subtle); border-left: 2px solid var(--accent-strong); font-size: 13px; }
summary { cursor: pointer; font-size: 13px; }
pre { padding: 16px; max-height: 400px; overflow: auto; font: 11px/1.8 var(--font-mono); background: var(--bg-secondary); border-radius: 8px; }
@media (max-width: 700px) { .check-grid, .catalog-grid { grid-template-columns: 1fr; } .integration-view > .panel { padding: 18px; } }
</style>
