<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Activity } from 'lucide-vue-next'
import { getMetricHistory } from '../api/metrics-history'
import type { MetricHistoryRangeHours, MetricHistoryResponse, MetricName } from '../types/metrics-history'
import { buildMetricChart, formatMetricValue, metricCoverageIssues, metricCoveragePercent, metricDisplayUnit, metricHistoryWindow } from '../utils/metrics-history'

const props = defineProps<{ token: string; clusterId: number; nodes: string[]; refreshKey: number }>()
const node = ref('')
const metric = ref<MetricName>('cpu')
const hours = ref<MetricHistoryRangeHours>(1)
const response = ref<MetricHistoryResponse | null>(null)
const loading = ref(false)
const error = ref('')
let sequence = 0
watch(() => props.nodes, names => { if (!names.includes(node.value)) node.value = names[0] ?? '' }, { immediate: true })
const chart = computed(() => {
  const value = response.value
  return value ? buildMetricChart(value.points, value.from, value.to, value.series.unit, value.coverage.collections, metricCoverageIssues(value.coverage).length > 0) : null
})
const coverage = computed(() => response.value ? metricCoveragePercent(response.value.coverage) : null)
const peak = computed(() => response.value?.points.length ? formatMetricValue(Math.max(...response.value.points.map(point => point.value)), response.value.series.unit) : '—')
function time(value: string) { return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }
async function load() {
  const request = ++sequence
  response.value = null
  error.value = ''
  loading.value = false
  if (!node.value) return
  loading.value = true
  try {
    const result = await getMetricHistory(props.token, props.clusterId, { resourceKind: 'Node', name: node.value, metric: metric.value, ...metricHistoryWindow(new Date(), hours.value), limit: 720 })
    if (request === sequence) response.value = result
  } catch { if (request === sequence) error.value = '历史指标暂不可用，请稍后刷新。' }
  finally { if (request === sequence) loading.value = false }
}
watch(() => [props.clusterId, props.token, node.value, metric.value, hours.value, props.refreshKey], load, { immediate: true })
</script>

<template>
  <section class="dash-panel trend-panel" aria-labelledby="trend-title" :aria-busy="loading">
    <header class="dash-panel-heading"><div><span class="dash-kicker">RESOURCE TREND</span><h2 id="trend-title"><Activity :size="16" />节点资源趋势</h2></div><span class="dash-caption">峰值 <strong>{{ peak }}</strong></span></header>
    <div class="trend-controls">
      <select v-model="node" aria-label="趋势节点" :disabled="!nodes.length"><option v-if="!nodes.length" value="">暂无节点</option><option v-for="name in nodes" :key="name" :value="name">{{ name }}</option></select>
      <div class="dash-segments" role="group" aria-label="趋势指标"><button v-for="item in (['cpu', 'memory'] as const)" :key="item" :aria-pressed="metric === item" @click="metric = item">{{ item === 'cpu' ? 'CPU' : '内存' }}</button></div>
      <select v-model="hours" aria-label="趋势时间范围"><option :value="1">最近 1 小时</option><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option></select>
    </div>
    <div v-if="loading || !response?.points.length" class="trend-empty"><Activity :size="28" /><strong>{{ loading ? '正在读取采集序列' : error || '暂无历史采集样本' }}</strong><span>{{ node ? '按所选节点展示，缺失采集不会补零。' : '接入节点后展示真实资源趋势。' }}</span></div>
    <template v-else-if="chart && response">
      <div class="trend-chart-wrap"><span class="trend-axis">{{ chart.maxValue.toFixed(0) }} {{ metricDisplayUnit(response.series.unit) }}</span>
        <svg viewBox="0 0 720 190" class="trend-chart" role="img" :aria-label="`${node} ${metric === 'cpu' ? 'CPU' : '内存'}趋势，${response.points.length} 个样本，峰值 ${peak}`" preserveAspectRatio="none">
          <defs><linearGradient id="dashboard-trend-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="var(--dash-cyan)" stop-opacity=".24" /><stop offset="100%" stop-color="var(--dash-cyan)" stop-opacity="0" /></linearGradient></defs>
          <line v-for="y in [14, 66, 118, 170]" :key="y" x1="12" x2="708" :y1="y" :y2="y" class="trend-grid" />
          <g v-for="(segment, index) in chart.segments" :key="index"><path v-if="segment.pointCount > 1" :d="`${segment.path} L ${segment.path.split(' ').at(-2)} 170 L ${segment.path.split(' ')[1]} 170 Z`" fill="url(#dashboard-trend-fill)" /><path :d="segment.path" class="trend-line" /></g>
          <circle v-for="(point, index) in chart.points" :key="index" :cx="point.x" :cy="point.y" :r="chart.points.length < 60 ? 2.5 : 1.4" class="trend-dot"><title>{{ time(point.collectedAt) }} · {{ formatMetricValue(point.value, response.series.unit) }}</title></circle>
        </svg>
        <div class="trend-times"><span>{{ time(response.from) }}</span><span>{{ time(response.to) }}</span></div>
      </div>
    </template>
    <footer class="dash-footnote">{{ response ? `采集覆盖 ${coverage === null ? '—' : coverage + '%'} · ${response.points.length} 个样本` : '单节点时间序列' }}<span v-if="response">{{ metricCoverageIssues(response.coverage).join(' · ') || '以采集时间为横轴' }}{{ response.truncated ? ' · 已截断' : '' }}</span></footer>
  </section>
</template>

<style scoped>
.trend-controls { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: 16px; }
.trend-controls select { min-width: 0; max-width: 190px; flex: 1; }
.trend-controls select:last-child { max-width: 145px; }
.trend-chart-wrap { margin-top: 12px; }
.trend-axis { color: var(--text-secondary); font: 10px var(--font-mono); }
.trend-chart { display: block; width: 100%; height: 140px; overflow: visible; }
.trend-grid { stroke: var(--border-subtle); stroke-dasharray: 4 5; }
.trend-line { stroke: var(--dash-cyan); fill: none; stroke-width: 2.5; vector-effect: non-scaling-stroke; }
.trend-dot { fill: var(--dash-cyan); }
.trend-times { display: flex; justify-content: space-between; color: var(--text-secondary); font: 10px var(--font-mono); }
.trend-empty { min-height: 182px; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 10px; color: var(--text-secondary); text-align: center; }
.trend-empty svg { color: var(--dash-cyan); opacity: .6; }
.trend-empty strong { font-size: 13px; font-weight: 500; }
.trend-empty span { font-size: 11px; }
@media (max-width: 520px) { .trend-controls { gap: 6px; } .trend-controls select:first-child { flex-basis: 100%; max-width: 100%; } }
</style>
