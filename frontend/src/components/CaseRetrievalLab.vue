<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRight, Download, GitCompareArrows, Search, Shuffle } from 'lucide-vue-next'
import { inspectKnowledge } from '../api/knowledge'
import type { KnowledgeEntry, RetrievalTrace } from '../api/knowledge'
import { useAuthStore } from '../stores/auth'
import { downloadJSON } from '../utils/download'

const props = defineProps<{ reference: KnowledgeEntry | null }>()
const auth = useAuthStore()
const query = ref('')
const rule = ref('')
const kind = ref('')
const trace = ref<RetrievalTrace | null>(null)
const loading = ref(false)
const error = ref('')
const frozenReference = ref<KnowledgeEntry | null>(null)
let sequence = 0
watch(() => props.reference, value => {
  sequence++
  loading.value = false
  trace.value = null
  frozenReference.value = null
  error.value = ''
  if (value) { query.value = value.summary; rule.value = value.rule_id; kind.value = value.resource_kind }
}, { immediate: true })
const stageStatus = computed(() => trace.value?.vector_status === 'complete' ? '已完成' : trace.value?.vector_status === 'unavailable' ? '不可用，已降级' : '未启用')
const baseline = computed(() => trace.value?.structured.slice(0, trace.value.result_limit) ?? [])
const rankIn = (items: KnowledgeEntry[]) => {
  if (!frozenReference.value) return '未指定'
  const index = items.findIndex(item => item.id === frozenReference.value?.id)
  return index < 0 ? '未进入返回结果' : `第 ${index + 1} 名`
}
const rows = computed(() => (trace.value?.fused ?? []).map((entry, index) => {
  const structuredRank = (trace.value?.structured.findIndex(item => item.id === entry.id) ?? -1) + 1
  const vectorRank = (trace.value?.vector.findIndex(item => item.id === entry.id) ?? -1) + 1
  const constant = trace.value?.rank_constant ?? 60
  const contribution = (rank: number) => rank ? `1/(${constant}+${rank})` : '—'
  return { entry, rank: index + 1, structuredRank, vectorRank, structuredContribution: contribution(structuredRank), vectorContribution: contribution(vectorRank), final: index < (trace.value?.result_limit ?? 3) }
}))
async function run() {
  const request = ++sequence
  trace.value = null
  error.value = ''
  loading.value = true
  const reference = props.reference
  try {
    const response = await inspectKnowledge(auth.accessToken, { q: query.value.trim(), rule_id: rule.value.trim(), resource_kind: kind.value })
    if (request === sequence) { trace.value = response; frozenReference.value = reference }
  } catch (err) { if (request === sequence) error.value = err instanceof Error ? err.message : '检索过程读取失败' }
  finally { if (request === sequence) loading.value = false }
}
function renameRule() { if (rule.value && !rule.value.endsWith('.renamed')) rule.value += '.renamed' }
function exportTrace() { if (trace.value) downloadJSON('case-retrieval-comparison.json', { schema: 'aiops.retrieval-comparison/v1', observed_at: new Date().toISOString(), reference_id: frozenReference.value?.id ?? null, trace: trace.value }) }
</script>

<template>
  <section class="retrieval-lab" aria-label="案例检索对比">
    <header class="retrieval-hero"><span>CASE MEMORY / RETRIEVAL TRACE</span><h2><GitCompareArrows :size="22" />观察案例如何被召回</h2><p>同一次查询并排观察结构化基线和混合检索结果，核对每个案例来自哪一阶段、如何进入最终排序。</p></header>
    <form class="panel retrieval-form" @submit.prevent="run"><label class="retrieval-query"><span>当前故障描述</span><textarea v-model="query" aria-label="检索对比故障描述" maxlength="500" rows="3" required placeholder="例如：Pod 因内存超限被 OOMKilled，容器反复重启" /></label><div class="retrieval-fields"><label><span>规则 ID（精确匹配提示）</span><input v-model="rule" aria-label="检索对比规则 ID" maxlength="200" placeholder="pod.oom_killed.v1" /></label><label><span>资源类型提示</span><select v-model="kind" aria-label="检索对比资源类型"><option value="">不限资源类型</option><option v-for="value in ['Pod', 'Node', 'Deployment', 'Service', 'Ingress', 'PersistentVolumeClaim', 'HorizontalPodAutoscaler']" :key="value">{{ value }}</option></select></label><button type="button" class="secondary-button" :disabled="!rule || loading" @click="renameRule"><Shuffle :size="14" />模拟规则更名</button><button class="primary-button" :disabled="loading || !query.trim()"><Search :size="14" />{{ loading ? '检索中…' : '运行检索对比' }}</button></div><p>更名只修改本次查询的规则提示，用来观察字段不匹配时的召回；案例库中的数据保持原样。</p><p v-if="reference" class="reference-context">参考案例 #{{ reference.id }}：{{ reference.summary }}<span>运行后显示它在两种策略中的返回名次。</span></p></form>
    <p v-if="error" role="alert" class="error-message">{{ error }}</p>
    <section v-if="!trace && !loading" class="panel retrieval-empty"><GitCompareArrows :size="32" /><h3>用一条已归档案例开始对比</h3><p>可在“案例库”中选择“查看检索过程”带入参考案例，先运行原规则查询，再模拟规则更名重试。</p><span>结果来自当前案例库 · 不调用大模型 · 不写入案例</span></section>
    <template v-if="trace">
      <header class="retrieval-run-heading"><p>结果对应：<strong>{{ trace.query.summary_hint }}</strong><code>规则 {{ trace.query.rule_id || '不限' }} · {{ trace.query.resource_kind || '不限资源类型' }}</code></p><button class="secondary-button" @click="exportTrace"><Download :size="14" />导出对比</button></header>
      <div class="retrieval-stages" aria-label="实际检索阶段"><article class="panel"><span>01 / 结构化短名单</span><strong>{{ trace.structured.length }}</strong><small>规则 + 类型精确匹配 · 最多 {{ trace.shortlist_limit }} 条</small></article><ArrowRight :size="16" aria-hidden="true" /><article class="panel"><span>02 / 向量召回</span><strong>{{ trace.vector.length }}</strong><small>{{ stageStatus }} · 已读取候选池 {{ trace.pool_count }} 条</small></article><ArrowRight :size="16" aria-hidden="true" /><article class="panel"><span>03 / {{ trace.vector.length ? 'RRF 排序融合' : '结构化降级结果' }}</span><strong>{{ trace.fused.length }}</strong><small>去重候选 · 最终取前 {{ trace.result_limit }} 条</small></article></div>
      <p class="retrieval-boundary">向量提供者 {{ trace.embedder || '未配置' }}{{ trace.dimension ? ` · ${trace.dimension} 维` : '' }}。当前 lexical-hash-v1 使用词法特征，不代表神经语义理解；召回候选与排序分数均不等于相关性确认或诊断置信度。本次关闭模型重排以单独观察召回与融合。</p>
      <p v-if="trace.pool_truncated || trace.structured_truncated" class="retrieval-limit">{{ trace.pool_truncated ? '向量候选池达到读取上限，可能还有未参与检索的案例。' : '' }}{{ trace.structured_truncated ? '结构化短名单达到上限。' : '' }}</p>
      <p v-if="trace.vector_status !== 'complete'" class="retrieval-limit">向量阶段{{ trace.vector_status === 'disabled' ? '未启用' : '暂不可用' }}，本次保留结构化检索结果。</p>
      <div class="retrieval-comparison"><section class="panel"><header><h3>结构化基线</h3><span>前 {{ trace.result_limit }} 条</span></header><p v-if="frozenReference" class="reference-rank">参考案例 #{{ frozenReference.id }}：{{ rankIn(baseline) }}</p><ol><li v-for="entry in baseline" :key="entry.id"><RouterLink :to="{ path: '/diagnoses', query: { id: entry.source_diagnosis_id } }">{{ entry.summary }}</RouterLink><code>#{{ entry.id }} · {{ entry.rule_id }}</code></li></ol><p v-if="!baseline.length" class="retrieval-no-match">精确字段未召回案例。</p></section><section class="panel"><header><h3>混合检索结果</h3><span>前 {{ trace.result_limit }} 条</span></header><p v-if="frozenReference" class="reference-rank">参考案例 #{{ frozenReference.id }}：{{ rankIn(trace.results) }}</p><ol><li v-for="entry in trace.results" :key="entry.id"><RouterLink :to="{ path: '/diagnoses', query: { id: entry.source_diagnosis_id } }">{{ entry.summary }}</RouterLink><code>#{{ entry.id }} · {{ entry.rule_id }}</code></li></ol><p v-if="!trace.results.length" class="retrieval-no-match">当前候选范围内没有返回案例。</p></section></div>
      <section class="panel retrieval-ranking"><header><h3>融合排序明细</h3><span>RRF = Σ 1 / ({{ trace.rank_constant }} + 阶段名次)</span></header><div class="retrieval-table-scroll" tabindex="0" role="region" aria-label="融合排序明细，可横向滚动"><table data-responsive="scroll"><caption class="sr-only">候选案例的阶段名次、排序贡献及最终结果</caption><thead><tr><th scope="col">候选案例</th><th scope="col">结构化名次</th><th scope="col">向量名次</th><th scope="col">结构化贡献</th><th scope="col">向量贡献</th><th scope="col">融合分数</th><th scope="col">返回</th></tr></thead><tbody><tr v-for="row in rows" :key="row.entry.id"><th scope="row"><span>#{{ row.entry.id }} {{ row.entry.summary }}</span><code>{{ row.entry.rule_id }}</code></th><td>{{ row.structuredRank || '—' }}</td><td>{{ row.vectorRank || '—' }}</td><td>{{ trace.vector.length ? row.structuredContribution : '—' }}</td><td>{{ trace.vector.length ? row.vectorContribution : '—' }}</td><td>{{ trace.vector.length ? (row.entry.score ?? 0).toFixed(5) : '未融合' }}</td><td>{{ row.final ? `第 ${row.rank} 名` : '候选' }}</td></tr><tr v-if="!rows.length"><td colspan="7">暂无候选案例。</td></tr></tbody></table></div></section>
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
</style>
