<script setup lang="ts">
import { computed, ref } from 'vue'
import { CheckCircle2, Download, FlaskConical, Play, Search, ShieldCheck, XCircle } from 'lucide-vue-next'
import { getDiagnosisValidation } from '../api/validation'
import type { ValidationReport } from '../api/validation'
import { useAuthStore } from '../stores/auth'
import { diagnosisRuleName } from '../utils/diagnosis-labels'
import { downloadJSON } from '../utils/download'

const auth = useAuthStore()
const report = ref<ValidationReport | null>(null)
const loading = ref(false)
const error = ref('')
const rule = ref('')
const resultFilter = ref('')
const search = ref('')
const activeID = ref('')
const outcomes = computed(() => (report.value?.outcomes ?? []).filter(item =>
  (!rule.value || item.rule_id === rule.value) &&
  (!resultFilter.value || (resultFilter.value === 'failed' ? !item.passed || (item.kind === 'Pod' && !item.pipeline_passed) : resultFilter.value === 'positive' ? item.expected : !item.expected)) &&
  `${item.id} ${item.note} ${diagnosisRuleName(item.rule_id)}`.toLowerCase().includes(search.value.toLowerCase()),
))
const active = computed(() => report.value?.outcomes.find(item => item.id === activeID.value))
const passed = computed(() => report.value?.outcomes.filter(item => item.passed && (item.kind !== 'Pod' || item.pipeline_passed)).length ?? 0)
const format = (value: number) => value.toFixed(3)
async function run() {
  loading.value = true
  error.value = ''
  // A new run clears the previous report so failures cannot look like fresh results.
  report.value = null
  activeID.value = ''
  try { report.value = await getDiagnosisValidation(auth.accessToken) }
  catch (err) { error.value = err instanceof Error ? err.message : '规则验证暂不可用' }
  finally { loading.value = false }
}
function selectRule(id: string) { rule.value = rule.value === id ? '' : id; activeID.value = '' }
</script>

<template>
  <section class="validation-workspace" aria-label="规则验证">
    <header class="validation-hero"><div><span class="validation-eyebrow">DETERMINISTIC VALIDATION</span><h2><FlaskConical :size="22" />规则验证与场景回放</h2><p>用标注快照复现诊断推理，逐项核对目标规则、正反例与证据。验证不会向集群注入故障，也不会生成诊断记录。</p></div><div class="validation-actions"><button class="primary-button" :disabled="loading" @click="run"><Play :size="15" />{{ loading ? '正在验证…' : report ? '重新验证' : '运行固定场景验证' }}</button><button v-if="report" class="secondary-button" @click="downloadJSON('diagnosis-validation.json', report)"><Download :size="15" />导出结果</button></div></header>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <div v-if="!report" class="panel validation-intro" :aria-busy="loading"><ShieldCheck :size="34" /><h3>{{ loading ? '正在运行规则引擎' : '从输入快照验证输出结论' }}</h3><p>固定语料包含故障正例、健康负例和容易混淆的相邻状态。Pod 场景另外验证「内存溢出 → 镜像拉取 → 崩溃循环 → 挂起」的优先级链。</p><ol><li>读取人工标注快照</li><li>执行实际诊断规则</li><li>计算混淆矩阵与 F1</li><li>查看场景原文和命中证据</li></ol></div>
    <template v-else>
      <div class="validation-metrics"><article class="panel"><span>场景通过</span><strong>{{ passed }}<small> / {{ report.total_scenarios }}</small></strong></article><article class="panel"><span>规则覆盖</span><strong>{{ report.per_rule.length }}<small> 条规则</small></strong></article><article class="panel"><span>Micro / Macro F1</span><strong>{{ format(report.micro_f1) }}<small> / {{ format(report.macro_f1) }}</small></strong></article><article class="panel"><span>Pod 优先级链</span><strong>{{ report.pipeline.correct_top1 }}<small> / {{ report.pipeline.total_pod_scenarios }}</small></strong></article></div>
      <p class="validation-boundary">这是固定标注语料上的规格一致性验证，结果不代表生产环境准确率。语料 v{{ report.corpus_version }} · 本次运行 {{ new Date(report.generated_at).toLocaleString('zh-CN') }}</p>
      <section class="panel rule-matrix"><header><h3>规则覆盖矩阵</h3><span>点击规则筛选下方场景 · TP 真正例 / FP 误报 / FN 漏报 / TN 真负例</span></header><div class="validation-table-scroll" tabindex="0" role="region" aria-label="规则覆盖矩阵，可横向滚动"><table data-responsive="scroll"><caption class="sr-only">每条规则的标注场景数、混淆矩阵与评估指标</caption><thead><tr><th scope="col">规则</th><th scope="col">场景</th><th scope="col">TP</th><th scope="col">FP</th><th scope="col">FN</th><th scope="col">TN</th><th scope="col">精确率</th><th scope="col">召回率</th><th scope="col">F1</th></tr></thead><tbody><tr v-for="item in report.per_rule" :key="item.rule_id" :class="{ selected: rule === item.rule_id }"><th scope="row"><button :aria-pressed="rule === item.rule_id" @click="selectRule(item.rule_id)">{{ diagnosisRuleName(item.rule_id) }}<code>{{ item.rule_id }}</code></button></th><td>{{ item.scenarios }}</td><td>{{ item.true_positives }}</td><td :class="{ mismatch: item.false_positives > 0 }">{{ item.false_positives }}</td><td :class="{ mismatch: item.false_negatives > 0 }">{{ item.false_negatives }}</td><td>{{ item.true_negatives }}</td><td>{{ format(item.precision) }}</td><td>{{ format(item.recall) }}</td><td>{{ format(item.f1) }}</td></tr></tbody></table></div></section>
      <section class="panel scenario-panel"><header class="scenario-header"><div><h3>场景检查</h3><span>{{ outcomes.length }} 个匹配场景</span></div><div class="scenario-filters"><label><Search :size="14" /><input v-model="search" aria-label="搜索验证场景" placeholder="搜索场景或规则" /></label><select v-model="rule" aria-label="验证规则"><option value="">全部规则</option><option v-for="item in report.per_rule" :key="item.rule_id" :value="item.rule_id">{{ diagnosisRuleName(item.rule_id) }}</option></select><select v-model="resultFilter" aria-label="场景类型"><option value="">全部场景</option><option value="positive">故障正例</option><option value="negative">近邻负例</option><option value="failed">验证未通过</option></select></div></header>
        <div class="scenario-layout"><ul class="scenario-list"><li v-for="item in outcomes" :key="item.id"><button :aria-pressed="activeID === item.id" @click="activeID = item.id"><component :is="item.passed && (item.kind !== 'Pod' || item.pipeline_passed) ? CheckCircle2 : XCircle" :size="16" :class="item.passed && (item.kind !== 'Pod' || item.pipeline_passed) ? 'passed' : 'mismatch'" /><span><strong>{{ item.id }}</strong><small>{{ diagnosisRuleName(item.rule_id) }} · {{ item.expected ? '故障正例' : '近邻负例' }}</small></span></button></li><li v-if="!outcomes.length" class="scenario-empty">没有符合筛选条件的场景。</li></ul>
          <article v-if="active && outcomes.some(item => item.id === activeID)" class="scenario-detail"><span class="validation-eyebrow">SCENARIO / {{ active.kind }}</span><h3>{{ active.id }}</h3><p class="scenario-note">{{ active.note }}</p><dl class="scenario-result"><div><dt>人工标注</dt><dd>{{ active.expected ? '目标规则应命中' : '目标规则不应命中' }}</dd></div><div><dt>本次结果</dt><dd :class="active.passed ? 'passed' : 'mismatch'">{{ active.matched ? '命中' : '未命中' }} · {{ active.passed ? '一致' : '不一致' }}</dd></div></dl><div v-if="active.kind === 'Pod'" class="pipeline-result"><strong>Pod 优先级链 {{ active.pipeline_passed ? '通过' : '不一致' }}</strong><span>预期：{{ active.pipeline_expected || '不命中规则' }}</span><span>实际：{{ active.pipeline_actual || '不命中规则' }}</span></div><template v-if="active.record"><h4>规则结论</h4><p>{{ active.record.summary }}</p><h4>候选根因</h4><ul><li v-for="cause in active.record.root_causes" :key="cause">{{ cause }}</li></ul><h4>命中证据 · {{ active.record.evidence.length }}</h4><details v-for="(evidence, index) in active.record.evidence" :key="index"><summary>{{ evidence.type }} · {{ evidence.source }}</summary><pre tabindex="0" aria-label="命中证据 JSON">{{ JSON.stringify(evidence.content, null, 2) }}</pre></details></template><p v-else class="negative-explanation">此场景未命中目标规则。负例用来检查健康状态或其他故障是否会被误报，Pod 优先级链结果单独核对。</p><details class="scenario-input"><summary>查看原始输入与人工标注</summary><pre tabindex="0" aria-label="场景原始输入 JSON">{{ JSON.stringify(active.input, null, 2) }}</pre></details></article><div v-else class="scenario-detail scenario-empty">选择左侧场景，核对输入、结论和证据。</div>
        </div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; border: 0; }
table[data-responsive="scroll"] { min-width: 800px; }
.validation-workspace { min-width: 0; }
.validation-hero { display: flex; justify-content: space-between; align-items: center; gap: 24px; padding: 22px 24px; border: 1px solid var(--border-subtle); border-radius: 14px; background: linear-gradient(115deg, var(--accent-subtle), var(--bg-elevated)); }
.validation-eyebrow { color: var(--accent-strong); font: 10px var(--font-mono); letter-spacing: 1.2px; }
.validation-hero h2 { display: flex; align-items: center; gap: 9px; font-size: 21px; margin: 10px 0; }
.validation-hero p { color: var(--text-secondary); max-width: 640px; font-size: 12px; line-height: 1.8; margin-bottom: 0; }
.validation-actions { display: flex; flex-direction: column; gap: 8px; flex-shrink: 0; }
.validation-actions button { display: flex; align-items: center; justify-content: center; gap: 7px; }
.validation-intro { padding: 36px; margin-top: 20px; text-align: center; }
.validation-intro > svg { color: var(--accent-strong); }
.validation-intro h3 { margin: 15px 0 10px; }
.validation-intro p { max-width: 650px; margin: 0 auto; font-size: 13px; line-height: 1.8; color: var(--text-secondary); }
.validation-intro ol { display: flex; justify-content: center; gap: 34px; margin: 26px 0 0; padding: 0; font-size: 12px; list-style-position: inside; color: var(--text-secondary); flex-wrap: wrap; }
.validation-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; margin-top: 18px; }
.validation-metrics article { padding: 19px; }
.validation-metrics span { display: block; color: var(--text-secondary); font-size: 12px; }
.validation-metrics strong { display: block; margin-top: 12px; font: 600 28px var(--font-mono); }
.validation-metrics small { font-size: 11px; color: var(--text-secondary); }
.validation-boundary { font-size: 11px; line-height: 1.7; color: var(--text-secondary); margin: 14px 1px 18px; }
.rule-matrix, .scenario-panel { overflow: hidden; margin-top: 18px; }
.rule-matrix > header { padding: 20px; display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
h3 { font-size: 15px; margin: 0; }
.rule-matrix header span { font-size: 11px; color: var(--text-secondary); }
.validation-table-scroll { overflow-x: auto; }
table { border-collapse: collapse; width: 100%; font-size: 12px; }
th, td { padding: 11px 14px; text-align: right; border-top: 1px solid var(--border-subtle); white-space: nowrap; }
thead th { font-weight: 500; color: var(--text-secondary); background: var(--bg-secondary); }
th:first-child { text-align: left; }
tbody th { font-weight: 400; }
tbody button { text-align: left; padding: 0; border: 0; color: var(--text-primary); background: transparent; cursor: pointer; font-size: 12px; }
tbody code { display: block; font-size: 10px; color: var(--text-secondary); margin-top: 5px; }
tr.selected { background: var(--accent-subtle); }
.scenario-header { display: flex; justify-content: space-between; gap: 15px; align-items: center; padding: 20px; flex-wrap: wrap; }
.scenario-header > div:first-child { display: flex; flex-direction: column; gap: 6px; }
.scenario-header span { font-size: 11px; color: var(--text-secondary); }
.scenario-filters { display: flex; gap: 8px; flex-wrap: wrap; }
.scenario-filters label { position: relative; display: flex; align-items: center; }
.scenario-filters label svg { position: absolute; left: 9px; color: var(--text-secondary); }
.scenario-filters input { padding-left: 30px; width: 180px; }
.scenario-filters input, .scenario-filters select { font-size: 11px; min-height: 34px; max-width: 100%; }
.scenario-layout { display: grid; grid-template-columns: minmax(210px, .8fr) minmax(0, 1.6fr); border-top: 1px solid var(--border-subtle); }
.scenario-list { list-style: none; margin: 0; padding: 10px; max-height: 620px; overflow-y: auto; border-right: 1px solid var(--border-subtle); }
.scenario-list button { display: flex; align-items: center; gap: 9px; width: 100%; border: 1px solid transparent; border-radius: 6px; background: transparent; color: var(--text-primary); padding: 12px 10px; cursor: pointer; text-align: left; }
.scenario-list button[aria-pressed="true"] { background: var(--accent-subtle); border-color: var(--accent-soft); }
.scenario-list button > svg { flex-shrink: 0; }
.scenario-list strong { font: 11px var(--font-mono); overflow-wrap: anywhere; }
.scenario-list small { display: block; color: var(--text-secondary); font-size: 10px; margin-top: 7px; }
.scenario-detail { padding: 22px; max-height: 620px; overflow: auto; font-size: 12px; line-height: 1.8; }
.scenario-detail h3 { margin: 8px 0 12px; font-family: var(--font-mono); overflow-wrap: anywhere; }
.scenario-note, .negative-explanation { color: var(--text-secondary); }
.scenario-result { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin: 18px 0; }
.scenario-result > div { padding: 12px; background: var(--bg-secondary); border-radius: 8px; }
.scenario-result dt { font-size: 11px; color: var(--text-secondary); }
.scenario-result dd { margin: 4px 0 0; font-weight: 600; }
.pipeline-result { display: grid; gap: 4px; padding: 12px 14px; border-left: 2px solid var(--accent-strong); background: var(--accent-subtle); overflow-wrap: anywhere; }
.pipeline-result span { font: 10px/1.7 var(--font-mono); }
.scenario-detail h4 { margin: 18px 0 8px; font-size: 12px; }
.scenario-detail ul { padding-left: 20px; }
.scenario-detail details { border: 1px solid var(--border-subtle); border-radius: 7px; margin-top: 9px; padding: 10px 12px; }
.scenario-detail summary { cursor: pointer; font-size: 11px; overflow-wrap: anywhere; }
.scenario-detail pre { font: 11px/1.8 var(--font-mono); max-width: 100%; overflow-x: auto; padding: 12px; background: var(--bg-secondary); border-radius: 6px; }
.scenario-empty { color: var(--text-secondary); font-size: 12px; padding: 30px; text-align: center; }
.passed { color: var(--status-success-strong, var(--status-success)); }
.mismatch { color: var(--status-danger); }
@media (max-width: 950px) { .validation-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .scenario-layout { grid-template-columns: 1fr; } .scenario-list { max-height: 240px; border-right: 0; border-bottom: 1px solid var(--border-subtle); } }
@media (max-width: 600px) { .validation-hero { flex-direction: column; align-items: stretch; padding: 20px; } .validation-hero h2 { font-size: 18px; } .validation-metrics { gap: 10px; } .validation-metrics article { padding: 15px; } .validation-metrics strong { font-size: 22px; } .scenario-filters { width: 100%; } .scenario-filters label, .scenario-filters input { width: 100%; } .scenario-filters select { flex: 1; min-width: 0; } .validation-intro { padding: 24px; } .validation-intro ol { gap: 15px; } .scenario-detail { padding: 18px; } }
</style>
