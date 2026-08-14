<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AlertTriangle, RefreshCw } from 'lucide-vue-next'

import { evaluateMetricHistory, getMetricHistory, getMetricHistoryArchive } from '../api/metrics-history'
import type { MetricHistoryEvaluationResponse, MetricHistoryRangeHours, MetricHistoryResponse, MetricName, MetricResourceKind } from '../types/metrics-history'
import { buildMetricChart, formatMetricValue, METRIC_CHART, metricCoverageIssues, metricCoveragePercent, metricDisplayUnit, metricHistoryWindow } from '../utils/metrics-history'

const props = withDefaults(defineProps<{
  accessToken: string
  clusterId: number
  resourceKind: MetricResourceKind
  namespace?: string
  name: string
  containers?: string[]
}>(), { namespace: '', containers: () => [] })

const metric = ref<MetricName>('cpu')
const rangeHours = ref<MetricHistoryRangeHours>(1)
const container = ref(props.containers[0] ?? '')
const response = ref<MetricHistoryResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const evaluationThreshold = ref('500')
const evaluationForSeconds = ref(300)
const evaluation = ref<MetricHistoryEvaluationResponse | null>(null)
const evaluationLoading = ref(false)
const evaluationError = ref('')
let requestSequence = 0
let evaluationSequence = 0

const chart = computed(() => response.value ? buildMetricChart(
  response.value.points,
  response.value.from,
  response.value.to,
  response.value.series.unit,
  response.value.coverage.collections,
  response.value.coverage.missing > 0 || response.value.coverage.unavailable > 0 || response.value.coverage.timed_out > 0 || response.value.coverage.failed > 0,
) : null)
const coveragePercent = computed(() => response.value ? metricCoveragePercent(response.value.coverage) : null)
const coverageIssues = computed(() => response.value ? metricCoverageIssues(response.value.coverage) : [])
const displayUnit = computed(() => response.value ? metricDisplayUnit(response.value.series.unit) : metric.value === 'cpu' ? 'mCPU' : 'MiB')
const periodLabel = computed(() => {
  if (!response.value) return ''
  const suffix = rangeHours.value >= 168 ? ' · 下采样(小时档)' : ''
  return `${formatTimestamp(response.value.from)} — ${formatTimestamp(response.value.to)}${suffix}`
})
const canQuery = computed(() => props.resourceKind === 'Node' || Boolean(container.value))
const thresholdUnit = computed(() => metric.value === 'cpu' ? 'mCPU' : 'MiB')

function formatTimestamp(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

async function loadHistory(): Promise<void> {
  const sequence = ++requestSequence
  response.value = null
  errorMessage.value = ''
  if (!canQuery.value) return
  loading.value = true
  const window = metricHistoryWindow(new Date(), rangeHours.value)
  const query = {
    resourceKind: props.resourceKind,
    namespace: props.resourceKind === 'Pod' ? props.namespace : undefined,
    name: props.name,
    container: props.resourceKind === 'Pod' ? container.value : undefined,
    metric: metric.value,
    ...window,
    limit: 1440,
  }
  try {
    // M114-3: 7d/30d ranges read the downsampled hourly archive tier
    // (bounded to 1440 points); shorter ranges read precise samples.
    const result = rangeHours.value >= 168
      ? await getMetricHistoryArchive(props.accessToken, props.clusterId, query)
      : await getMetricHistory(props.accessToken, props.clusterId, query)
    if (sequence === requestSequence) response.value = result
  } catch (error) {
    if (sequence === requestSequence) errorMessage.value = error instanceof Error ? error.message : '历史指标暂时不可用'
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

async function evaluateWindow(): Promise<void> {
  const sequence = ++evaluationSequence
  evaluation.value = null
  evaluationError.value = ''
  const displayThreshold = Number(evaluationThreshold.value)
  if (!Number.isFinite(displayThreshold) || displayThreshold < 0) {
    evaluationError.value = '阈值必须是非负数'
    return
  }
  const threshold = Math.round(displayThreshold * (metric.value === 'cpu' ? 1_000_000 : 1_048_576))
  if (!Number.isSafeInteger(threshold)) {
    evaluationError.value = '阈值超出安全范围'
    return
  }
  const to = new Date()
  const from = new Date(to.getTime() - evaluationForSeconds.value * 1000)
  evaluationLoading.value = true
  try {
    const result = await evaluateMetricHistory(props.accessToken, props.clusterId, {
      resourceKind: props.resourceKind,
      namespace: props.resourceKind === 'Pod' ? props.namespace : undefined,
      name: props.name,
      container: props.resourceKind === 'Pod' ? container.value : undefined,
      metric: metric.value,
      from: from.toISOString(),
      to: to.toISOString(),
      operator: 'gte',
      threshold,
      forSeconds: evaluationForSeconds.value,
      minimumPoints: 2,
    })
    if (sequence === evaluationSequence) evaluation.value = result
  } catch (error) {
    if (sequence === evaluationSequence) evaluationError.value = error instanceof Error ? error.message : '持续窗口评估暂时不可用'
  } finally {
    if (sequence === evaluationSequence) evaluationLoading.value = false
  }
}

watch(() => props.containers, (containers) => {
  if (!containers.includes(container.value)) container.value = containers[0] ?? ''
}, { deep: true })

watch(() => [props.clusterId, props.resourceKind, props.namespace, props.name, container.value, metric.value] as const, () => {
  evaluationSequence++
  evaluationLoading.value = false
  evaluation.value = null
  evaluationError.value = ''
})

watch(() => [props.accessToken, props.clusterId, props.resourceKind, props.namespace, props.name, container.value, metric.value, rangeHours.value] as const, loadHistory, { immediate: true })
</script>

<template>
  <section class="metrics-history-panel" aria-label="历史资源趋势">
    <header class="metrics-history-header">
      <div>
        <span class="eyebrow">METRICS HISTORY</span>
        <h3>资源趋势</h3>
        <small>精确序列 · 缺失采集不会补零</small>
      </div>
      <button class="history-refresh" type="button" title="刷新历史指标" aria-label="刷新历史指标" :disabled="loading || !canQuery" @click="loadHistory">
        <RefreshCw :size="15" :class="{ spinning: loading }" />
      </button>
    </header>

    <div class="history-controls">
      <div class="segmented-control" aria-label="指标">
        <button type="button" :class="{ active: metric === 'cpu' }" @click="metric = 'cpu'">CPU</button>
        <button type="button" :class="{ active: metric === 'memory' }" @click="metric = 'memory'">内存</button>
      </div>
      <label v-if="resourceKind === 'Pod'" class="container-select">
        <span>容器</span>
        <select v-model="container" aria-label="容器">
          <option v-for="item in containers" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <div class="segmented-control range-control" aria-label="时间范围">
        <button v-for="option in ([{ h: 1, label: '1h' }, { h: 6, label: '6h' }, { h: 24, label: '24h' }, { h: 168, label: '7d' }, { h: 720, label: '30d' }] as const)" :key="option.h" type="button" :class="{ active: rangeHours === option.h }" @click="rangeHours = option.h">{{ option.label }}</button>
      </div>
    </div>

    <section class="evaluation-card" aria-label="持续窗口评估">
      <div class="evaluation-copy">
        <strong>持续窗口评估</strong>
        <small>只读证据，不会自动执行运维操作</small>
      </div>
      <label class="evaluation-input"><span>≥ 阈值</span><input v-model="evaluationThreshold" type="number" min="0" step="1" :aria-label="`阈值 ${thresholdUnit}`"><em>{{ thresholdUnit }}</em></label>
      <label class="evaluation-input"><span>持续</span><select v-model="evaluationForSeconds" aria-label="持续时间"><option :value="300">5 分钟</option><option :value="900">15 分钟</option><option :value="3600">1 小时</option></select></label>
      <button class="evaluation-button" type="button" :disabled="evaluationLoading || !canQuery" @click="evaluateWindow">{{ evaluationLoading ? '评估中…' : '评估当前窗口' }}</button>
      <div v-if="evaluation" class="evaluation-result" :class="evaluation.state">
        <strong>{{ evaluation.state === 'firing' ? '持续超过阈值' : evaluation.state === 'normal' ? '未形成持续窗口' : '数据不足' }}</strong>
        <span>{{ evaluation.breaching_points }}/{{ evaluation.points_evaluated }} 个尾部违约点 · 已观察 {{ evaluation.observed_span_seconds }} 秒</span>
      </div>
      <div v-else-if="evaluationError" class="evaluation-result error"><strong>评估失败</strong><span>{{ evaluationError }}</span></div>
    </section>

    <div v-if="!canQuery" class="history-state empty">当前 Pod 没有可查询的容器</div>
    <div v-else-if="loading" class="history-state"><RefreshCw :size="20" class="spinning" /><span>正在读取历史序列…</span></div>
    <div v-else-if="errorMessage" class="history-state error">
      <AlertTriangle :size="20" /><span><strong>趋势暂不可用</strong>{{ errorMessage }}</span>
      <button type="button" @click="loadHistory">重试</button>
    </div>

    <template v-else-if="response">
      <div class="coverage-row">
        <span class="coverage-primary" :class="{ degraded: response.coverage.missing > 0 || coverageIssues.length > 0 }">
          覆盖率 {{ coveragePercent === null ? '--' : `${coveragePercent}%` }}
          <small>{{ response.coverage.points }}/{{ response.coverage.collections }} 次采集</small>
        </span>
        <span v-for="issue in coverageIssues" :key="issue" class="coverage-issue">{{ issue }}</span>
        <span v-if="response.truncated" class="coverage-issue warning">已截断至 {{ response.limits.max_points }} 点</span>
        <span class="unit-label">单位 {{ displayUnit }}</span>
      </div>

      <div v-if="response.points.length === 0" class="history-state empty">
        <strong>该时间范围没有样本</strong>
        <span v-if="response.coverage.collections > 0">已记录 {{ response.coverage.collections }} 次采集；请结合上方缺样与失败状态判断。</span>
        <span v-else>该范围尚未发生采集。</span>
      </div>

      <div v-else-if="chart" class="chart-wrap">
        <div class="chart-value-label"><strong>{{ formatMetricValue(Math.max(...response.points.map((point) => point.value)), response.series.unit) }}</strong><span>峰值</span></div>
        <svg class="history-chart" :viewBox="`0 0 ${METRIC_CHART.width} ${METRIC_CHART.height}`" role="img" :aria-label="`${metric === 'cpu' ? 'CPU' : '内存'} 历史趋势，${response.points.length} 个样本`" preserveAspectRatio="none">
          <line v-for="row in [0, 1, 2, 3]" :key="row" :x1="METRIC_CHART.left" :x2="METRIC_CHART.width - METRIC_CHART.right" :y1="METRIC_CHART.top + row * 48" :y2="METRIC_CHART.top + row * 48" class="chart-grid" />
          <path v-for="(segment, index) in chart.segments" :key="index" :d="segment.path" class="chart-line" />
          <circle v-for="point in chart.points" :key="`${point.collectedAt}-${point.value}`" :cx="point.x" :cy="point.y" r="3.5" class="chart-point">
            <title>{{ formatMetricValue(point.value, response.series.unit) }} · {{ formatTimestamp(point.collectedAt) }}</title>
          </circle>
        </svg>
        <div class="chart-axis"><span>{{ formatTimestamp(response.from) }}</span><span>{{ formatTimestamp(response.to) }}</span></div>
        <p v-if="response.coverage.missing > 0" class="gap-note"><span />图线断开处表示采集缺口，不代表数值为零。</p>
        <p class="period-label">{{ periodLabel }}</p>
      </div>
    </template>
  </section>
</template>

<style scoped>
.metrics-history-panel { min-width: 0; margin-top: 24px; overflow: hidden; border: 1px solid #dbe5e4; border-radius: 16px; background: linear-gradient(145deg, #fbfdfd, #f4f8f7); padding: 16px; }
.metrics-history-header, .history-controls, .coverage-row, .chart-axis { display: flex; align-items: center; }
.metrics-history-header { justify-content: space-between; gap: 12px; }
.metrics-history-header h3 { margin: 2px 0; color: #193b38; font-size: 16px; }
.metrics-history-header small { color: #70827f; }
.eyebrow { color: #238177; font-size: 10px; font-weight: 800; letter-spacing: .12em; }
.history-refresh { display: grid; width: 32px; height: 32px; place-items: center; border: 1px solid #cfdcda; border-radius: 9px; color: #36635e; background: #fff; cursor: pointer; }
.history-refresh:disabled { opacity: .5; cursor: default; }
.history-controls { flex-wrap: wrap; gap: 9px; margin: 14px 0 12px; }
.segmented-control { display: inline-flex; padding: 3px; border: 1px solid #d6e1df; border-radius: 10px; background: #edf3f2; }
.segmented-control button { min-height: 28px; padding: 4px 11px; border: 0; border-radius: 7px; color: #60716e; background: transparent; font: inherit; font-size: 12px; cursor: pointer; }
.segmented-control button.active { color: #fff; background: #237b72; box-shadow: 0 2px 6px rgb(35 123 114 / 20%); }
.range-control { margin-left: auto; }
.container-select { display: flex; min-width: 0; align-items: center; gap: 6px; color: #61726f; font-size: 12px; }
.container-select select { max-width: 180px; min-height: 34px; border: 1px solid #d6e1df; border-radius: 9px; color: #294b47; background: #fff; padding: 0 28px 0 9px; }
.evaluation-card { display: flex; min-width: 0; flex-wrap: wrap; align-items: end; gap: 8px; margin: 0 0 12px; border: 1px solid #d9e4e2; border-radius: 12px; background: #fff; padding: 10px; }
.evaluation-copy { display: flex; min-width: 150px; flex: 1; flex-direction: column; color: #244b47; }
.evaluation-copy small { margin-top: 2px; color: #778985; font-size: 10px; }
.evaluation-input { display: grid; grid-template-columns: auto minmax(58px, 90px) auto; align-items: center; gap: 5px; color: #657773; font-size: 11px; }
.evaluation-input input, .evaluation-input select { min-width: 0; height: 31px; border: 1px solid #d4dfdd; border-radius: 8px; color: #294b47; background: #fff; padding: 0 7px; }
.evaluation-input em { color: #758783; font-style: normal; }
.evaluation-button { min-height: 31px; border: 0; border-radius: 8px; color: #fff; background: #2c746d; padding: 0 10px; cursor: pointer; }
.evaluation-button:disabled { opacity: .5; cursor: default; }
.evaluation-result { display: flex; width: 100%; min-width: 0; align-items: center; justify-content: space-between; gap: 8px; border-radius: 8px; padding: 7px 9px; color: #315f59; background: #e8f3f1; font-size: 11px; }
.evaluation-result.firing { color: #8b3329; background: #ffe5e1; }
.evaluation-result.insufficient_data, .evaluation-result.error { color: #815b18; background: #fff0cc; }
.evaluation-result span { overflow-wrap: anywhere; text-align: right; }
.coverage-row { min-width: 0; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.coverage-primary, .coverage-issue, .unit-label { border-radius: 999px; padding: 4px 8px; font-size: 11px; }
.coverage-primary { color: #17665e; background: #dff3ef; font-weight: 700; }
.coverage-primary.degraded { color: #825b11; background: #fff0c9; }
.coverage-primary small { margin-left: 4px; font-weight: 500; opacity: .8; }
.coverage-issue { color: #8b5220; background: #fff0df; }
.coverage-issue.warning { color: #8b3a31; background: #ffe5e1; }
.unit-label { margin-left: auto; color: #536966; background: #e8efee; }
.history-state { display: flex; min-height: 150px; align-items: center; justify-content: center; gap: 9px; border: 1px dashed #cfdddb; border-radius: 12px; color: #667b77; text-align: center; }
.history-state.empty { flex-direction: column; }
.history-state.error { color: #97443a; background: #fff8f7; }
.history-state.error span { display: flex; flex-direction: column; text-align: left; }
.history-state.error button { border: 1px solid #ddb9b4; border-radius: 8px; color: #884039; background: #fff; padding: 5px 9px; cursor: pointer; }
.chart-wrap { position: relative; min-width: 0; }
.chart-value-label { display: flex; align-items: baseline; gap: 6px; color: #214945; }
.chart-value-label span { color: #758783; font-size: 11px; }
.history-chart { display: block; width: 100%; height: 190px; overflow: visible; }
.chart-grid { stroke: #dce7e5; stroke-width: 1; vector-effect: non-scaling-stroke; }
.chart-line { fill: none; stroke: #238177; stroke-width: 2.5; stroke-linecap: round; stroke-linejoin: round; vector-effect: non-scaling-stroke; }
.chart-point { fill: #fff; stroke: #238177; stroke-width: 2; vector-effect: non-scaling-stroke; }
.chart-axis { justify-content: space-between; gap: 12px; color: #768784; font-size: 10px; }
.gap-note, .period-label { margin: 8px 0 0; color: #697d79; font-size: 11px; }
.gap-note span { display: inline-block; width: 15px; margin-right: 5px; border-top: 2px dashed #c17b2b; vertical-align: middle; }
.period-label { overflow-wrap: anywhere; opacity: .75; }
.spinning { animation: history-spin .9s linear infinite; }
@keyframes history-spin { to { transform: rotate(360deg); } }
@media (max-width: 600px) {
  .metrics-history-panel { padding: 13px; border-radius: 13px; }
  .history-controls { align-items: stretch; }
  .container-select { order: 3; width: 100%; }
  .container-select select { width: 100%; max-width: none; }
  .range-control { margin-left: 0; }
  .evaluation-card { align-items: stretch; }
  .evaluation-copy, .evaluation-input, .evaluation-button { width: 100%; }
  .evaluation-input { grid-template-columns: 48px minmax(0, 1fr) auto; }
  .evaluation-result { align-items: flex-start; flex-direction: column; }
  .evaluation-result span { text-align: left; }
  .unit-label { margin-left: 0; }
  .history-chart { height: 160px; }
  .chart-axis { font-size: 9px; }
}
</style>
