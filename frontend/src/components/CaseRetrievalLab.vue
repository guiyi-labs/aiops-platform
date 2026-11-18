<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ArrowRight, Download, GitCompareArrows, Play, Search, Shuffle } from 'lucide-vue-next'
import { inspectKnowledge } from '../api/knowledge'
import type { KnowledgeEntry, RetrievalTrace } from '../api/knowledge'
import { useAuthStore } from '../stores/auth'
import { diagnosisRuleName } from '../utils/diagnosis-labels'
import { downloadJSON } from '../utils/download'

const props = defineProps<{ reference: KnowledgeEntry | null; entries: KnowledgeEntry[]; libraryLoading: boolean; libraryError: string }>()
const emit = defineEmits<{ select: [entry: KnowledgeEntry]; reload: [] }>()
const auth = useAuthStore()
const query = ref('')
const rule = ref('')
const kind = ref('')
const trace = ref<RetrievalTrace | null>(null)
const aligned = ref<RetrievalTrace | null>(null)
const loading = ref(false)
const error = ref('')
const frozenReference = ref<KnowledgeEntry | null>(null)
const completedAt = ref('')
const advanced = ref(false)
let sequence = 0
function clearResults() { sequence++; loading.value = false; trace.value = null; aligned.value = null; frozenReference.value = null; error.value = ''; completedAt.value = '' }
watch(() => props.reference, value => {
  clearResults()
  if (value) { query.value = value.summary; rule.value = value.rule_id; kind.value = value.resource_kind }
}, { immediate: true })
watch(() => props.entries, entries => {
  if (!props.reference && entries.length) emit('select', entries.find(item => ['high', 'critical'].includes(item.severity)) ?? entries[0]!)
}, { immediate: true })
onBeforeUnmount(() => { sequence++ })
function selectReference(event: Event) {
  const entry = props.entries.find(item => item.id === Number((event.target as HTMLSelectElement).value))
  if (entry) emit('select', entry)
}
function restoreReference() {
  if (!props.reference) return
  clearResults(); query.value = props.reference.summary; rule.value = props.reference.rule_id; kind.value = props.reference.resource_kind
}
const choices = computed(() => props.reference && !props.entries.some(item => item.id === props.reference?.id) ? [props.reference, ...props.entries] : props.entries)
const baseline = computed(() => trace.value?.structured.slice(0, trace.value.result_limit) ?? [])
const submittedChanged = computed(() => !!trace.value && (query.value.trim() !== trace.value.query.summary_hint || rule.value.trim() !== trace.value.query.rule_id || kind.value !== trace.value.query.resource_kind))
const rank = (items: KnowledgeEntry[], id?: number) => id ? items.findIndex(item => item.id === id) + 1 : 0
const rankIn = (items: KnowledgeEntry[]) => {
  if (!frozenReference.value) return '未指定'
  const position = rank(items, frozenReference.value.id)
  return position ? `第 ${position} 名` : '未进入返回结果'
}
const conclusion = computed(() => {
  if (!trace.value || !frozenReference.value) return '查看两种检索策略的实际结果，并打开来源诊断核对证据。'
  const found = rank(trace.value.results, frozenReference.value.id)
  const baselineFound = rank(baseline.value, frozenReference.value.id)
  if (found && !baselineFound) return `结构化基线未返回参考案例，混合检索将它召回到第 ${found} 名。`
  if (found) return `两种策略均返回参考案例；混合检索位于第 ${found} 名，可继续观察规则更名后的变化。`
  return '参考案例未进入混合检索的最终结果。请检查召回阶段、候选范围与向量状态。'
})
const stageStatus = computed(() => trace.value?.vector_status === 'complete' ? '已完成' : trace.value?.vector_status === 'unavailable' ? '不可用，已降级' : '未启用')
const rows = computed(() => (trace.value?.fused ?? []).map((entry, index) => {
  const structuredRank = rank(trace.value?.structured ?? [], entry.id)
  const vectorRank = rank(trace.value?.vector ?? [], entry.id)
  const constant = trace.value?.rank_constant ?? 60
  const contribution = (position: number) => position ? `1/(${constant}+${position})` : '—'
  return { entry, rank: index + 1, structuredRank, vectorRank, structuredContribution: contribution(structuredRank), vectorContribution: contribution(vectorRank), final: index < (trace.value?.result_limit ?? 3) }
}))
async function run(paired = false) {
  if (!query.value.trim() || (paired && !rule.value.trim())) return
  clearResults()
  const request = sequence
  const reference = props.reference
  const input = { q: query.value.trim(), rule_id: rule.value.trim(), resource_kind: kind.value }
  const originalRule = input.rule_id.replace(/\.renamed$/, '')
  if (paired) input.rule_id = originalRule + '.renamed'
  loading.value = true
  try {
    // Both requests use actual library data. Preserve both submitted snapshots.
    const results = paired
      ? await Promise.all([inspectKnowledge(auth.accessToken, { ...input, rule_id: originalRule }), inspectKnowledge(auth.accessToken, input)])
      : [await inspectKnowledge(auth.accessToken, input)]
    if (request !== sequence) return
    trace.value = results[paired ? 1 : 0]!
    aligned.value = paired ? results[0]! : null
    frozenReference.value = reference
    completedAt.value = new Date().toISOString()
    if (paired) rule.value = input.rule_id
  } catch (err) { if (request === sequence) error.value = err instanceof Error ? err.message : '检索过程读取失败' }
  finally { if (request === sequence) loading.value = false }
}
function renameRule() { if (rule.value && !rule.value.endsWith('.renamed')) rule.value += '.renamed' }
function exportTrace() {
  if (trace.value) downloadJSON('case-retrieval-comparison.json', {
    schema: 'aiops.retrieval-comparison/v2', observed_at: completedAt.value,
    reference_id: frozenReference.value?.id ?? null, trace: trace.value, aligned_trace: aligned.value,
  })
}
</script>

<template>
  <section class="retrieval-lab" aria-label="案例检索对比">
    <header class="retrieval-hero"><span>CASE MEMORY / RETRIEVAL TRACE</span><h2><GitCompareArrows :size="22" />观察案例如何被召回</h2><p>选择一条真实历史案例，一键比较规则更名前后的检索结果，再沿来源诊断查看证据。</p></header>
    <section class="panel demo-selector" aria-label="案例演示准备">
      <ol class="demo-steps"><li><b>1</b>选择历史案例</li><li><b>2</b>运行更名前后对比</li><li><b>3</b>核对召回与证据</li></ol>
      <div class="demo-choice"><label for="retrieval-reference">参考案例</label><select id="retrieval-reference" :value="reference?.id ?? ''" :disabled="libraryLoading || loading || !choices.length" @change="selectReference"><option v-if="!choices.length" value="">{{ libraryLoading ? '正在读取案例库…' : '暂无可用历史案例' }}</option><option v-for="entry in choices" :key="entry.id" :value="entry.id">#{{ entry.id }} · {{ diagnosisRuleName(entry.rule_id) }} · {{ entry.resource_kind }}/{{ entry.resource_name }}</option></select><button class="primary-button" :disabled="loading || !query.trim() || !rule.trim()" @click="run(true)"><Play :size="15" />{{ loading ? '正在运行…' : '一键演示更名前后对比' }}</button></div>
      <p v-if="reference" class="demo-reference">{{ reference.summary }} <RouterLink :to="{ path: '/diagnoses', query: { id: reference.source_diagnosis_id } }">查看来源证据 →</RouterLink></p>
      <p v-if="libraryError" class="error-message" role="alert">{{ libraryError }} <button class="secondary-button" @click="emit('reload')">重试读取案例库</button></p>
      <p v-else-if="!libraryLoading && !choices.length" class="demo-reference">先在“故障诊断”中记录处理结果并将诊断标记为已解决，再返回这里选择归档案例。<RouterLink to="/diagnoses">前往诊断记录 →</RouterLink></p>
    </section>
    <details class="panel retrieval-custom" :open="advanced" @toggle="advanced = ($event.target as HTMLDetailsElement).open"><summary>自定义检索参数</summary>
      <form class="retrieval-form" @submit.prevent="run()"><label class="retrieval-query"><span>当前故障描述</span><textarea v-model="query" aria-label="检索对比故障描述" maxlength="500" rows="3" required placeholder="输入要检索的故障现象" :disabled="loading" /></label><div class="retrieval-fields"><label><span>规则 ID（精确匹配提示）</span><input v-model="rule" aria-label="检索对比规则 ID" maxlength="200" :disabled="loading" /></label><label><span>资源类型提示</span><select v-model="kind" aria-label="检索对比资源类型" :disabled="loading"><option value="">不限资源类型</option><option v-for="value in ['Pod', 'Node', 'Deployment', 'Service', 'Ingress', 'PersistentVolumeClaim', 'HorizontalPodAutoscaler']" :key="value">{{ value }}</option></select></label><button type="button" class="secondary-button" :disabled="!rule || loading" @click="renameRule"><Shuffle :size="14" />模拟规则更名</button><button class="primary-button" :disabled="loading || !query.trim()"><Search :size="14" />运行检索对比</button><button type="button" class="secondary-button" :disabled="!reference || loading" @click="restoreReference">恢复参考案例参数</button></div><p>更名仅修改查询提示，案例库中的规则与数据保持原样。</p></form>
    </details>
    <p v-if="error" role="alert" class="error-message">{{ error }}，可重试本次操作。</p>
    <p v-if="loading" class="panel retrieval-empty" role="status">正在读取当前案例库并计算检索阶段…</p>
    <section v-else-if="!trace" class="panel retrieval-empty"><GitCompareArrows :size="32" /><h3>已准备好真实案例，点击上方开始对比</h3><p>先验证原规则可否返回参考案例，再仅更名规则提示，比较结构化检索与混合检索的差异。</p><span>使用当前案例库 · 两次实际查询 · 不修改规则与案例</span></section>
    <template v-if="trace">
      <section class="panel retrieval-conclusion" aria-live="polite"><span>{{ aligned ? '更名前后对比已完成' : '本次检索已完成' }}</span><h3>{{ conclusion }}</h3><p v-if="aligned">下方保留两次运行结果；详细阶段展示更名后的查询。</p><p v-if="submittedChanged" class="retrieval-limit">参数已修改，下方仍是上次提交的结果。重新运行后更新。</p></section>
      <section v-if="aligned" class="panel rename-comparison"><header><h3>同一故障描述 · 仅改变规则提示</h3><span>参考案例 #{{ frozenReference?.id ?? '未指定' }}</span></header><div class="retrieval-table-scroll" tabindex="0" role="region" aria-label="更名前后结果，可横向滚动"><table data-responsive="scroll"><caption class="sr-only">原规则与更名规则两次实际检索的参考案例名次</caption><thead><tr><th scope="col">查询条件</th><th scope="col">结构化基线</th><th scope="col">混合检索</th></tr></thead><tbody><tr><th scope="row">原规则<code>{{ aligned.query.rule_id }}</code></th><td>{{ rankIn(aligned.structured.slice(0, aligned.result_limit)) }}</td><td>{{ rankIn(aligned.results) }}</td></tr><tr><th scope="row">更名规则<code>{{ trace.query.rule_id }}</code></th><td>{{ rankIn(baseline) }}</td><td>{{ rankIn(trace.results) }}</td></tr></tbody></table></div></section>
      <header class="retrieval-run-heading"><p>结果对应：<strong>{{ trace.query.summary_hint }}</strong><code>规则 {{ trace.query.rule_id || '不限' }} · {{ trace.query.resource_kind || '不限资源类型' }}</code></p><button class="secondary-button" @click="exportTrace"><Download :size="14" />导出对比</button></header>
      <div class="retrieval-stages" aria-label="实际检索阶段"><article class="panel"><span>01 / 结构化短名单</span><strong>{{ trace.structured.length }}</strong><small>{{ trace.query.rule_id || trace.query.resource_kind ? '按所填规则、类型字段筛选' : '未提供字段条件，读取默认短名单' }} · 最多 {{ trace.shortlist_limit }} 条</small></article><ArrowRight :size="16" aria-hidden="true" /><article class="panel"><span>02 / 向量召回</span><strong>{{ trace.vector.length }}</strong><small>{{ stageStatus }} · 已读取候选池 {{ trace.pool_count }} 条</small></article><ArrowRight :size="16" aria-hidden="true" /><article class="panel"><span>03 / {{ trace.vector.length ? 'RRF 排序融合' : '结构化降级结果' }}</span><strong>{{ trace.fused.length }}</strong><small>去重候选 · 最终取前 {{ trace.result_limit }} 条</small></article></div>
      <p v-if="trace.pool_truncated || trace.structured_truncated" class="retrieval-limit">{{ trace.pool_truncated ? '向量候选池达到读取上限，可能还有未参与检索的案例。' : '' }}{{ trace.structured_truncated ? '结构化短名单达到上限。' : '' }}</p>
      <p v-if="trace.vector_status !== 'complete'" class="retrieval-limit">向量阶段{{ trace.vector_status === 'disabled' ? '未启用' : '暂不可用' }}，本次保留结构化检索结果。</p>
      <div class="retrieval-comparison"><section class="panel"><header><h3>结构化基线</h3><span>前 {{ trace.result_limit }} 条</span></header><p v-if="frozenReference" class="reference-rank">参考案例 #{{ frozenReference.id }}：{{ rankIn(baseline) }}</p><ol><li v-for="entry in baseline" :key="entry.id"><RouterLink :to="{ path: '/diagnoses', query: { id: entry.source_diagnosis_id } }">{{ entry.summary }}</RouterLink><code>#{{ entry.id }} · {{ entry.rule_id }}</code></li></ol><p v-if="!baseline.length" class="retrieval-no-match">当前字段条件未召回案例。</p></section><section class="panel"><header><h3>混合检索结果</h3><span>前 {{ trace.result_limit }} 条</span></header><p v-if="frozenReference" class="reference-rank">参考案例 #{{ frozenReference.id }}：{{ rankIn(trace.results) }}</p><ol><li v-for="entry in trace.results" :key="entry.id"><RouterLink :to="{ path: '/diagnoses', query: { id: entry.source_diagnosis_id } }">{{ entry.summary }}</RouterLink><code>#{{ entry.id }} · {{ entry.rule_id }}</code></li></ol><p v-if="!trace.results.length" class="retrieval-no-match">当前候选范围内没有返回案例。</p></section></div>
      <details class="panel retrieval-ranking"><summary>查看融合排序明细与计算说明</summary><header><h3>融合排序明细</h3><span>RRF = Σ 1 / ({{ trace.rank_constant }} + 阶段名次)</span></header><p class="retrieval-boundary">{{ trace.embedder || '未配置向量提供者' }}{{ trace.dimension ? ` · ${trace.dimension} 维` : '' }}；高严重度候选池。词法嵌入用于文本特征召回；排序分数不代表诊断置信度。本次关闭模型重排。</p><div class="retrieval-table-scroll" tabindex="0" role="region" aria-label="融合排序明细，可横向滚动"><table data-responsive="scroll"><caption class="sr-only">候选案例的阶段名次、排序贡献及最终结果</caption><thead><tr><th scope="col">候选案例</th><th scope="col">结构化名次</th><th scope="col">向量名次</th><th scope="col">结构化贡献</th><th scope="col">向量贡献</th><th scope="col">融合分数</th><th scope="col">返回</th></tr></thead><tbody><tr v-for="row in rows" :key="row.entry.id"><th scope="row"><span>#{{ row.entry.id }} {{ row.entry.summary }}</span><code>{{ row.entry.rule_id }}</code></th><td>{{ row.structuredRank || '—' }}</td><td>{{ row.vectorRank || '—' }}</td><td>{{ trace.vector.length ? row.structuredContribution : '—' }}</td><td>{{ trace.vector.length ? row.vectorContribution : '—' }}</td><td>{{ trace.vector.length ? (row.entry.score ?? 0).toFixed(5) : '未融合' }}</td><td>{{ row.final ? `第 ${row.rank} 名` : '候选' }}</td></tr><tr v-if="!rows.length"><td colspan="7">暂无候选案例。</td></tr></tbody></table></div></details>
    </template>
  </section>
</template>

<style scoped>
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; border: 0; }
table[data-responsive="scroll"] { min-width: 900px; }
.retrieval-hero { padding: 8px 0 18px; }
.retrieval-hero > span { color: var(--accent-strong); font: 10px var(--font-mono); letter-spacing: 1.3px; }
.retrieval-hero h2 { font-size: 21px; display: flex; align-items: center; gap: 8px; margin: 12px 0; }
.retrieval-hero p { font-size: 12px; color: var(--text-secondary); line-height: 1.8; margin: 0; }
.retrieval-form { padding: 22px; }
.retrieval-form label { display: grid; gap: 8px; min-width: 0; }
.retrieval-form label > span { font-size: 11px; color: var(--text-secondary); }
.retrieval-query textarea { width: 100%; box-sizing: border-box; resize: vertical; font-size: 13px; line-height: 1.8; }
.retrieval-fields { display: flex; gap: 12px; align-items: end; flex-wrap: wrap; margin-top: 15px; }
.retrieval-fields label:first-child { flex: 1; min-width: 200px; }
.retrieval-fields input, .retrieval-fields select { min-height: 37px; font-size: 12px; width: 100%; }
.retrieval-fields button, .retrieval-run-heading button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 37px; }
.retrieval-form > p { font-size: 11px; line-height: 1.7; color: var(--text-secondary); margin-bottom: 0; }
.retrieval-form > .reference-context { padding: 12px 14px; border-left: 2px solid var(--accent-strong); background: var(--accent-subtle); color: var(--text-primary); }
.reference-context span { display: block; margin-top: 5px; color: var(--text-secondary); }
.retrieval-empty { padding: 42px 24px; text-align: center; margin-top: 20px; }
.retrieval-empty > svg { color: var(--accent-strong); }
.retrieval-empty h3 { font-size: 16px; margin: 16px 0 12px; }
.retrieval-empty p { font-size: 12px; line-height: 1.8; color: var(--text-secondary); }
.retrieval-empty > span { font-size: 11px; color: var(--text-secondary); }
.retrieval-run-heading { display: flex; justify-content: space-between; gap: 14px; align-items: center; margin: 20px 0 15px; }
.retrieval-run-heading p { font-size: 12px; color: var(--text-secondary); line-height: 1.7; margin: 0; }
.retrieval-run-heading strong { font-weight: 500; color: var(--text-primary); }
.retrieval-run-heading code { display: block; font-size: 10px; margin-top: 5px; overflow-wrap: anywhere; }
.retrieval-run-heading button { flex-shrink: 0; }
.retrieval-stages { display: grid; grid-template-columns: 1fr auto 1fr auto 1fr; gap: 12px; align-items: center; }
.retrieval-stages article { padding: 20px; align-self: stretch; }
.retrieval-stages article > span { display: block; font-size: 12px; color: var(--text-secondary); }
.retrieval-stages strong { display: block; font: 600 30px var(--font-mono); margin: 13px 0; }
.retrieval-stages small { font-size: 10px; line-height: 1.8; color: var(--text-secondary); }
.retrieval-stages > svg { color: var(--text-secondary); }
.retrieval-boundary { font-size: 11px; line-height: 1.9; color: var(--text-secondary); margin: 14px 0; }
.retrieval-limit { padding: 12px 16px; font-size: 12px; line-height: 1.8; color: var(--status-warning); background: var(--warning-bg); border-radius: 7px; }
.retrieval-comparison { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.retrieval-comparison section { padding: 20px; min-width: 0; }
.retrieval-comparison header, .retrieval-ranking header { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
h3 { font-size: 14px; margin: 0; }
.retrieval-comparison header span, .retrieval-ranking header span { font-size: 10px; color: var(--text-secondary); }
.reference-rank { font-size: 11px; padding: 10px 12px; background: var(--accent-subtle); border-radius: 6px; color: var(--accent-strong); }
.retrieval-comparison ol { margin: 14px 0 0; padding-left: 20px; font-size: 12px; }
.retrieval-comparison li { padding: 8px 0; line-height: 1.8; }
.retrieval-comparison a { color: var(--text-primary); text-decoration: none; }
.retrieval-comparison a:hover { color: var(--accent-strong); }
.retrieval-comparison code { display: block; font-size: 10px; color: var(--text-secondary); overflow-wrap: anywhere; }
.retrieval-no-match { color: var(--text-secondary); font-size: 12px; margin: 30px 0; text-align: center; }
.retrieval-ranking { margin-top: 18px; overflow: hidden; }
.retrieval-ranking header { padding: 20px; flex-wrap: wrap; }
.retrieval-table-scroll { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: 11px; }
th, td { padding: 12px 14px; text-align: right; border-top: 1px solid var(--border-subtle); white-space: nowrap; }
thead th { font-weight: 500; background: var(--bg-secondary); color: var(--text-secondary); }
th:first-child { text-align: left; }
tbody th { font-weight: 400; }
tbody th > span { display: block; max-width: 300px; white-space: normal; line-height: 1.7; }
tbody code { display: block; color: var(--text-secondary); margin-top: 6px; font-size: 10px; }
@media (max-width: 700px) { .retrieval-stages { grid-template-columns: 1fr; gap: 8px; } .retrieval-stages > svg { display: none; } .retrieval-stages article { padding: 16px; display: grid; grid-template-columns: 1fr auto; gap: 5px; align-items: center; } .retrieval-stages strong { margin: 0; font-size: 25px; } .retrieval-stages small { grid-column: 1 / -1; } .retrieval-comparison { grid-template-columns: 1fr; } .retrieval-form { padding: 18px; } .retrieval-fields > label, .retrieval-fields > button { flex: 1; } .retrieval-fields label:first-child { flex-basis: 100%; min-width: 0; } .retrieval-fields select { max-width: 100%; } .retrieval-run-heading { align-items: start; } .retrieval-run-heading button { padding: 8px; font-size: 11px; } }

.demo-selector { padding: 22px; }
.demo-steps { display: flex; gap: 24px; margin: 0 0 22px; padding: 0; list-style: none; flex-wrap: wrap; }
.demo-steps li { display: flex; align-items: center; gap: 8px; color: var(--text-secondary); font-size: 12px; }
.demo-steps b { background: var(--accent-subtle); color: var(--accent-strong); border-radius: 50%; display: grid; place-items: center; width: 26px; height: 26px; }
.demo-choice { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.demo-choice label { font-size: 12px; }
.demo-choice select { flex: 1; min-width: 0; max-width: 100%; font-size: 12px; }
.demo-choice button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; }
.demo-reference { font-size: 12px; line-height: 1.9; color: var(--text-secondary); margin: 14px 0 0; }
.demo-reference a { color: var(--accent-strong); display: inline-block; margin-left: 12px; }
.retrieval-custom { margin-top: 14px; }
.retrieval-custom > summary, .retrieval-ranking > summary { padding: 16px 20px; font-size: 12px; cursor: pointer; color: var(--text-primary); }
.retrieval-conclusion { border-left: 3px solid var(--accent-strong); padding: 18px 22px; margin-top: 18px; }
.retrieval-conclusion > span { display: block; color: var(--accent-strong); font-size: 11px; margin-bottom: 9px; }
.retrieval-conclusion h3 { line-height: 1.8; }
.retrieval-conclusion p { font-size: 12px; color: var(--text-secondary); margin-bottom: 0; }
.rename-comparison { margin-top: 16px; overflow: hidden; }
.rename-comparison > header { padding: 18px 20px; display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.rename-comparison header span { font-size: 11px; color: var(--text-secondary); }
.rename-comparison table { min-width: 600px; }
.retrieval-ranking .retrieval-boundary { padding: 0 20px; }
@media (max-width: 700px) { .demo-choice { align-items: stretch; } .demo-choice label { flex-basis: 100%; } .demo-choice select { flex-basis: 100%; width: 100%; } .demo-choice button { width: 100%; } .demo-steps { gap: 10px; } .demo-selector { padding: 18px; } }
</style>
