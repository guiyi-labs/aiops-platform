<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ChevronDown, ChevronRight, Clock, Gauge, PlayCircle, RefreshCw, Target } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { evaluateSLO, getSLOBurnSummary, listSLITemplates, listSLODefinitions, listSLOEvaluations } from '../api/aiops'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { EvaluationState, SLOBurnSummaryItem, SLITemplate, SLODefinition, SLOEvaluation } from '../types/aiops'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)

const definitions = ref<SLODefinition[]>([])
const templates = ref<SLITemplate[]>([])
const latestEvals = ref<Map<number, SLOEvaluation>>(new Map())
const evaluationHistory = ref<Map<number, SLOEvaluation[]>>(new Map())
const expandedSLOIDs = ref<Set<number>>(new Set())
const evaluating = ref<Set<number>>(new Set())
const loadingHistory = ref<Set<number>>(new Set())
const burnSummary = ref<SLOBurnSummaryItem[]>([])
const burnSummaryLoading = ref(false)

const loading = ref(false)
const errorMessage = ref('')

const hasDefinitions = computed(() => definitions.value.length > 0)

function formatWindow(seconds: number): string {
  if (!seconds || seconds <= 0 || Number.isNaN(seconds)) return '--'
  if (seconds % 86400 === 0) return `${seconds / 86400}d`
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

function formatPercent(ratio: number): string {
  if (ratio == null || Number.isNaN(ratio)) return '--'
  const pct = ratio * 100
  return `${pct.toFixed(3).replace(/\.?0+$/, '')}%`
}

function stateColor(state: EvaluationState): string {
  switch (state) {
    case 'healthy': return 'var(--status-success)'
    case 'burning_slow': return 'var(--status-warning)'
    case 'burning_fast': return '#ea580c'
    case 'breached': return 'var(--status-danger)'
    case 'unavailable':
    default: return 'var(--text-muted)'
  }
}

function stateLabel(state: EvaluationState): string {
  return ({
    healthy: '健康',
    burning_slow: '慢燃',
    burning_fast: '快燃',
    breached: '已突破',
    unavailable: '不可用',
  } as const)[state]
}

function burnStatusLabel(status: SLOBurnSummaryItem['status']): string {
  return ({ burning: '烧燃中', healthy: '健康', unavailable: '不可用', no_data: '无数据' } as const)[status]
}

function coverageLabel(coverage: string): string {
  return ({
    complete: '完整',
    partial: '部分样本',
    missing: '缺失',
    unavailable: '无数据',
    truncated: '截断',
  } as Record<string, string>)[coverage] ?? coverage
}

function coverageTone(coverage: string): string {
  switch (coverage) {
    case 'complete': return 'var(--status-success)'
    case 'partial':
    case 'truncated': return 'var(--status-warning)'
    case 'unavailable': return 'var(--text-muted)'
    default: return 'var(--status-danger)'
  }
}

function formatTime(value: string): string {
  if (!value) return '--'
  try {
    return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
  } catch {
    return value
  }
}

function windowDurationLabel(item: SLOEvaluation): string {
  const start = new Date(item.window_start).getTime()
  const end = new Date(item.window_end).getTime()
  if (Number.isNaN(start) || Number.isNaN(end)) return '--'
  return formatWindow(Math.round((end - start) / 1000))
}

function evalLatencyLabel(item: SLOEvaluation | undefined): string {
  if (!item) return '--'
  const end = new Date(item.window_end).getTime()
  const at = new Date(item.evaluated_at).getTime()
  if (Number.isNaN(end) || Number.isNaN(at) || !end) return '--'
  const ms = Math.max(0, at - end)
  if (ms < 60000) return `${Math.round(ms / 1000)}s`
  if (ms < 3600000) return `${Math.round(ms / 60000)}m`
  return `${(ms / 3600000).toFixed(1)}h`
}

function latestEval(slo: SLODefinition): SLOEvaluation | undefined {
  return latestEvals.value.get(slo.id)
}

function budgetRemainingPercent(item: SLOEvaluation | undefined): number {
  if (!item) return 0
  const value = item.remaining_budget
  if (value == null || Number.isNaN(value) || !Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.round(value * 100)))
}

function burnRateLabel(item: SLOEvaluation | undefined): string {
  if (!item || item.burn_rate == null || Number.isNaN(item.burn_rate)) return '--'
  return `${item.burn_rate.toFixed(2)}×`
}

function evalState(slo: SLODefinition): EvaluationState {
  return latestEvals.value.get(slo.id)?.state ?? 'unavailable'
}

function toggleHistory(slo: SLODefinition) {
  if (expandedSLOIDs.value.has(slo.id)) {
    expandedSLOIDs.value.delete(slo.id)
    return
  }
  expandedSLOIDs.value.add(slo.id)
  if (!evaluationHistory.value.has(slo.id)) {
    void loadHistory(slo)
  }
}

async function loadHistory(slo: SLODefinition) {
  loadingHistory.value.add(slo.id)
  try {
    const resp = await listSLOEvaluations(auth.accessToken, slo.id, { limit: 20 })
    evaluationHistory.value.set(slo.id, resp.items)
  } catch {
    evaluationHistory.value.set(slo.id, [])
  } finally {
    loadingHistory.value.delete(slo.id)
  }
}

async function runEvaluate(slo: SLODefinition) {
  evaluating.value.add(slo.id)
  try {
    const result = await evaluateSLO(auth.accessToken, slo.id)
    latestEvals.value.set(slo.id, result)
    evaluationHistory.value.delete(slo.id)
    if (expandedSLOIDs.value.has(slo.id)) {
      await loadHistory(slo)
    }
  } catch {
    // 评估失败时不覆盖已有结果，错误通过现有 UI 状态呈现
  } finally {
    evaluating.value.delete(slo.id)
  }
}

async function loadDefinitions() {
  if (!selectedClusterID.value) {
    definitions.value = []
    latestEvals.value = new Map()
    evaluationHistory.value = new Map()
    expandedSLOIDs.value = new Set()
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const resp = await listSLODefinitions(auth.accessToken, { cluster_id: selectedClusterID.value })
    definitions.value = resp.items
    latestEvals.value = new Map()
    evaluationHistory.value = new Map()
    expandedSLOIDs.value = new Set()
    await Promise.all(resp.items.map(async (slo) => {
      try {
        const hist = await listSLOEvaluations(auth.accessToken, slo.id, { limit: 1 })
        if (hist.items.length > 0) {
          latestEvals.value.set(slo.id, hist.items[0])
        }
      } catch {
        // 单个 SLO 历史读取失败不影响整体加载
      }
    }))
  } catch {
    errorMessage.value = '无法加载 SLO 定义列表'
  } finally {
    loading.value = false
  }
}

async function loadBurnSummary() {
  if (!selectedClusterID.value) {
    burnSummary.value = []
    return
  }
  burnSummaryLoading.value = true
  try {
    const resp = await getSLOBurnSummary(auth.accessToken, { cluster_id: selectedClusterID.value, limit: 100 })
    burnSummary.value = resp.items
  } catch {
    burnSummary.value = []
  } finally {
    burnSummaryLoading.value = false
  }
}

async function changeCluster() {
  await Promise.all([loadDefinitions(), loadBurnSummary()])
}

async function initialize() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch {
    errorMessage.value = '无法加载集群列表'
  }
  try {
    templates.value = (await listSLITemplates(auth.accessToken)).items
  } catch {
    templates.value = []
  }
  await Promise.all([loadDefinitions(), loadBurnSummary()])
}

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="SLO 仪表盘">
    <template #actions>
      <button class="icon-button" type="button" title="刷新 SLO" aria-label="刷新 SLO 列表" :disabled="loading || !selectedClusterID" @click="loadDefinitions">
        <RefreshCw :size="18" :class="{ spinning: loading }" />
      </button>
    </template>

    <section class="slo-toolbar" aria-label="SLO 筛选">
      <label class="field">
        <span>集群</span>
        <select v-model="selectedClusterID" aria-label="选择集群" :disabled="clusters.length === 0" @change="changeCluster">
          <option :value="null" disabled>选择已启用集群</option>
          <option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
      </label>
      <span class="metric-count">SLO 定义 · {{ definitions.length }}</span>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <section v-if="burnSummary.length > 0" class="burn-summary-strip" aria-label="SLO Burn 总览">
      <div class="burn-summary-header">
        <h3>Burn 总览 · {{ burnSummary.length }} 个 SLO</h3>
        <div v-if="burnSummaryLoading" class="burn-summary-loading"><RefreshCw class="spinning" :size="14" /></div>
      </div>
      <div class="burn-summary-grid">
        <div v-for="item in burnSummary" :key="item.slo_id" class="burn-card" :class="`burn-${item.status}`">
          <div class="burn-card-header">
            <strong>{{ item.service.name }}</strong>
            <span class="burn-status-badge" :class="item.status">{{ burnStatusLabel(item.status) }}</span>
          </div>
          <small>{{ item.service.namespace }} · {{ item.service.kind }}</small>
          <div class="burn-card-metrics">
            <span v-if="item.burn_rate != null">Burn ×{{ item.burn_rate.toFixed(2) }}</span>
            <span v-if="item.ratio != null">Ratio {{ formatPercent(item.ratio) }}</span>
            <span v-if="item.error_budget_remaining != null">Budget {{ formatPercent(item.error_budget_remaining) }}</span>
          </div>
        </div>
      </div>
    </section>

    <div v-if="!loading && clusters.length === 0" class="resource-empty">
      <Gauge :size="30" />
      <strong>没有已启用的集群</strong>
      <span>接入集群后可定义并评估 SLO。</span>
    </div>

    <div v-else-if="!loading && !hasDefinitions" class="resource-empty">
      <Target :size="30" />
      <strong>暂无 SLO 定义</strong>
      <span>当前集群尚未配置 SLO；通过模板创建后将在此展示评估结果与错误预算。</span>
    </div>

    <section v-else class="slo-grid" :class="{ loading }" aria-label="SLO 定义卡片">
      <article v-for="slo in definitions" :key="slo.id" class="slo-card">
        <header class="slo-card-header">
          <div class="slo-service">
            <span class="slo-glyph"><Gauge :size="16" /></span>
            <div>
              <strong>{{ slo.service.name }}</strong>
              <small>{{ slo.service.namespace }} · {{ slo.service.kind }} · 集群 #{{ slo.cluster_id }}</small>
            </div>
          </div>
          <span class="template-badge" :title="`模板 ${slo.template} v${slo.template_version}`">
            {{ slo.template }}<em>v{{ slo.template_version }}</em>
          </span>
        </header>

        <div class="slo-metrics">
          <div class="slo-metric">
            <span>目标</span>
            <strong>{{ formatPercent(slo.objective) }}</strong>
          </div>
          <div class="slo-metric">
            <span>滚动窗口</span>
            <strong><Clock :size="13" />{{ formatWindow(slo.rolling_window_seconds) }}</strong>
          </div>
          <div class="slo-metric">
            <span>启用状态</span>
            <span class="enabled-badge" :class="{ on: slo.enabled, off: !slo.enabled }">{{ slo.enabled ? '已启用' : '已停用' }}</span>
          </div>
        </div>

        <div class="slo-eval">
          <div class="slo-eval-state">
            <span class="state-label">最新评估</span>
            <span class="state-pill" :style="{ color: stateColor(evalState(slo)), borderColor: stateColor(evalState(slo)) }">
              <i class="state-dot" :style="{ background: stateColor(evalState(slo)) }" />
              {{ stateLabel(evalState(slo)) }}
            </span>
          </div>
          <div class="slo-eval-bars">
            <div class="bar-row">
              <span>消耗速率</span>
              <strong>{{ burnRateLabel(latestEvals.get(slo.id)) }}</strong>
            </div>
            <div class="bar-row">
              <span>错误预算剩余</span>
              <div class="budget-track" :title="`${budgetRemainingPercent(latestEvals.get(slo.id))}%`">
                <span :style="{ width: `${budgetRemainingPercent(latestEvals.get(slo.id))}%`, background: stateColor(evalState(slo)) }" />
              </div>
              <small>{{ budgetRemainingPercent(latestEvals.get(slo.id)) }}%</small>
            </div>
          </div>
          <div class="slo-eval-meta">
            <span class="coverage-badge" :style="{ color: coverageTone(latestEval(slo)?.coverage ?? ''), borderColor: coverageTone(latestEval(slo)?.coverage ?? '') }">
              数据覆盖 {{ coverageLabel(latestEval(slo)?.coverage ?? '') }}
            </span>
            <span title="评估时间与窗口结束时间的差">评估延迟 {{ evalLatencyLabel(latestEval(slo)) }}</span>
            <span v-if="latestEval(slo)?.state === 'unavailable'" class="no-data-hint">无样本窗口不视为健康</span>
          </div>
        </div>

        <footer class="slo-card-footer">
          <button class="secondary-button" type="button" :disabled="evaluating.has(slo.id) || !slo.enabled" @click="runEvaluate(slo)">
            <PlayCircle :size="14" :class="{ spinning: evaluating.has(slo.id) }" />{{ evaluating.has(slo.id) ? '评估中' : '立即评估' }}
          </button>
          <button class="history-toggle" type="button" @click="toggleHistory(slo)">
            <component :is="expandedSLOIDs.has(slo.id) ? ChevronDown : ChevronRight" :size="14" />评估历史
          </button>
        </footer>

        <section v-if="expandedSLOIDs.has(slo.id)" class="slo-history">
          <p v-if="loadingHistory.has(slo.id)" class="compact-empty">加载中…</p>
          <p v-else-if="(evaluationHistory.get(slo.id) || []).length === 0" class="compact-empty">暂无评估历史</p>
          <table v-else class="history-table">
            <thead>
              <tr><th>状态</th><th>覆盖度</th><th>窗口</th><th>达成率</th><th>消耗</th><th>剩余预算</th><th>延迟</th><th>评估时间</th></tr>
            </thead>
            <tbody>
              <tr v-for="item in evaluationHistory.get(slo.id)" :key="item.id">
                <td>
                  <span class="state-pill small" :style="{ color: stateColor(item.state), borderColor: stateColor(item.state) }">
                    <i class="state-dot" :style="{ background: stateColor(item.state) }" />{{ stateLabel(item.state) }}
                  </span>
                </td>
                <td>
                  <span class="coverage-badge" :style="{ color: coverageTone(item.coverage), borderColor: coverageTone(item.coverage) }">
                    {{ coverageLabel(item.coverage) }}
                  </span>
                </td>
                <td>{{ windowDurationLabel(item) }}</td>
                <td>{{ formatPercent(item.ratio) }}</td>
                <td>{{ item.burn_rate == null ? '--' : `${item.burn_rate.toFixed(2)}×` }}</td>
                <td>{{ budgetRemainingPercent(item) }}%</td>
                <td>{{ evalLatencyLabel(item) }}</td>
                <td>{{ formatTime(item.evaluated_at) }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </article>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.slo-toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 20px;
  padding: 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.field {
  display: grid;
  gap: 4px;
  color: var(--text-secondary);
  font-size: 12px;
}
.field select {
  min-width: 220px;
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}
.field select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}
.metric-count {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.slo-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 14px;
  margin-top: 14px;
}
.slo-grid.loading {
  opacity: 0.65;
}
.slo-card {
  display: grid;
  gap: 14px;
  padding: 16px 18px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.slo-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.slo-service {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.slo-service > div {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.slo-service strong {
  overflow: hidden;
  color: var(--text-primary);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.slo-service small {
  color: var(--text-secondary);
  font-size: 11px;
}
.slo-glyph {
  display: inline-flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  color: var(--accent-primary);
  background: var(--accent-subtle);
  border-radius: var(--radius-md);
  flex: 0 0 auto;
}
.template-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 9px;
  color: var(--accent-primary);
  font-size: 11px;
  font-weight: 600;
  background: var(--accent-subtle);
  border: 1px solid var(--accent-soft);
  border-radius: var(--radius-full);
  white-space: nowrap;
}
.template-badge em {
  color: var(--text-secondary);
  font-style: normal;
  font-weight: 500;
}
.slo-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.slo-metric {
  display: grid;
  gap: 5px;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.slo-metric > span {
  color: var(--text-secondary);
  font-size: 11px;
}
.slo-metric strong {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 600;
}
.enabled-badge {
  display: inline-flex;
  width: fit-content;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  border-radius: var(--radius-full);
}
.enabled-badge.on {
  color: var(--status-success);
  background: var(--success-bg);
}
.enabled-badge.off {
  color: var(--text-muted);
  background: var(--bg-tertiary);
}
.slo-eval {
  display: grid;
  gap: 10px;
  padding: 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.slo-eval-state {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.state-label {
  color: var(--text-secondary);
  font-size: 11px;
}
.state-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 600;
  border: 1px solid currentColor;
  border-radius: var(--radius-full);
  background: var(--bg-elevated);
}
.slo-eval-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding-top: 8px;
  border-top: 1px dashed var(--border-subtle);
  color: var(--text-secondary);
  font-size: 12px;
}
.coverage-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  border: 1px solid currentColor;
  border-radius: var(--radius-full);
  background: var(--bg-elevated);
  white-space: nowrap;
}
.no-data-hint {
  color: var(--text-muted);
  font-size: 11px;
}
.state-pill.small {
  padding: 2px 7px;
  font-size: 10px;
}
.state-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.slo-eval-bars {
  display: grid;
  gap: 8px;
}
.bar-row {
  display: grid;
  grid-template-columns: 90px 1fr 38px;
  align-items: center;
  gap: 10px;
}
.bar-row > span {
  color: var(--text-secondary);
  font-size: 11px;
}
.bar-row > strong {
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 600;
}
.bar-row > small {
  color: var(--text-secondary);
  font-size: 11px;
  text-align: right;
}
.budget-track {
  height: 6px;
  background: var(--border-soft);
  border-radius: var(--radius-full);
  overflow: hidden;
}
.budget-track span {
  display: block;
  height: 100%;
  border-radius: var(--radius-full);
  transition: width var(--transition-base);
}
.slo-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.history-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 32px;
  padding: 0 8px;
  background: transparent;
  border: 0;
  color: var(--text-secondary);
  font-size: 12px;
}
.history-toggle:hover:not(:disabled) {
  color: var(--accent-primary);
}
.slo-history {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.history-table {
  width: 100%;
  border-collapse: collapse;
}
.history-table th {
  padding: 6px 8px;
  color: var(--text-secondary);
  font-size: 10px;
  font-weight: 700;
  text-align: left;
  border-bottom: 1px solid var(--border-subtle);
}
.history-table td {
  padding: 7px 8px;
  color: var(--text-primary);
  font-size: 11px;
  border-bottom: 1px solid var(--border-subtle);
}
.history-table tbody tr:last-child td {
  border-bottom: 0;
}
.compact-empty {
  margin: 0;
  padding: 20px 0;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}

@media (max-width: 720px) {
  .slo-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .bar-row {
    grid-template-columns: 80px 1fr 38px;
  }
}

.burn-summary-strip {
  margin: 6px 0 24px;
  padding: 16px 18px;
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface);
}
.burn-summary-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.burn-summary-header h3 {
  margin: 0;
  font-size: 14px;
  color: var(--text-primary);
}
.burn-summary-loading {
  display: flex;
  align-items: center;
  color: var(--text-muted);
}
.burn-summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(215px, 1fr));
  gap: 12px;
}
.burn-card {
  padding: 12px 13px;
  border: 1px solid var(--border-soft);
  border-radius: 9px;
  background: var(--gray-0);
}
.burn-card.burn-burning {
  border-color: var(--status-danger);
  background: var(--danger-bg);
}
.burn-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.burn-card-header strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-primary);
}
.burn-card > small {
  display: block;
  margin-top: 2px;
  color: var(--text-muted);
  font-size: 11px;
}
.burn-status-badge {
  padding: 2px 8px;
  font-size: 10px;
  font-weight: 700;
  border-radius: 10px;
  white-space: nowrap;
  text-transform: uppercase;
}
.burn-status-badge.burning { color: var(--status-danger); background: #fbeae6; }
.burn-status-badge.healthy { color: var(--status-success); background: #e5f3ea; }
.burn-status-badge.unavailable { color: var(--status-warning); background: #fdf3e3; }
.burn-status-badge.no_data { color: var(--text-muted); background: var(--border-subtle); }
.burn-card-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  margin-top: 9px;
  color: var(--text-secondary);
  font-size: 11px;
}
</style>
