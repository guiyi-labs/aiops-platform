<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { AlertTriangle, CheckCircle2, FileText, RefreshCw, ThumbsUp, Users, XCircle } from 'lucide-vue-next'

import { getAICoverage } from '../api/diagnosis'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { AICoverage } from '../types/diagnosis'

const auth = useAuthStore()
const loading = ref(false)
const errorMessage = ref('')
const coverage = ref<AICoverage | null>(null)

const rateCards = computed(() => {
  const c = coverage.value
  if (!c) return []
  return [
    {
      label: '解释可用率',
      value: c.total_explanations > 0 ? `${rate(c.explained_diagnoses, c.total_explanations)}%` : '--',
      sub: `${c.explained_diagnoses} / ${c.total_explanations} 诊断覆盖`,
      icon: CheckCircle2,
      tone: '#1e7b3d',
    },
    {
      label: '引用率',
      value: `${rate(c.with_citations, c.total_explanations)}%`,
      sub: `${c.with_citations} / ${c.total_explanations} 条带引用`,
      icon: FileText,
      tone: '#2d6fb0',
    },
    {
      label: '降级率',
      value: `${rate(c.deterministic_count, c.total_explanations)}%`,
      sub: `${c.deterministic_count} 条确定性降级`,
      icon: AlertTriangle,
      tone: '#b34a4a',
    },
    {
      label: '反馈好评率',
      value: `${rate(c.quality.helpful, c.quality.total_feedback)}%`,
      sub: `${c.quality.total_feedback} 条反馈 · ${c.quality.contributors} 位贡献者`,
      icon: ThumbsUp,
      tone: '#8663b3',
    },
  ]
})

function rate(part: number, total: number): string {
  if (!total) return '0'
  return ((part / total) * 100).toFixed(1)
}

async function loadCoverage() {
  loading.value = true
  errorMessage.value = ''
  try {
    coverage.value = await getAICoverage(auth.accessToken)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '加载解释覆盖率失败'
    coverage.value = null
  } finally {
    loading.value = false
  }
}

onMounted(loadCoverage)
</script>

<template>
  <ConsoleLayout>
    <div class="page">
      <div class="page-header">
        <div>
          <h1 class="page-title">解释覆盖率大盘</h1>
          <p class="page-subtitle">M112-4 · AI 解释可用率 / 引用率 / 降级率（基于 aiexplain quality feedback 基线，只读展示）</p>
        </div>
        <button class="ghost-button" type="button" :disabled="loading" @click="loadCoverage">
          <RefreshCw :size="15" /> 刷新
        </button>
      </div>

      <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
      <p v-else-if="loading" class="muted">正在加载 AI 解释覆盖率…</p>

      <template v-else-if="coverage">
        <div class="rate-grid">
          <div v-for="card in rateCards" :key="card.label" class="rate-card">
            <component :is="card.icon" :size="22" :color="card.tone" />
            <div class="rate-label">{{ card.label }}</div>
            <div class="rate-value">{{ card.value }}</div>
            <div class="rate-sub">{{ card.sub }}</div>
          </div>
        </div>

        <section class="panel">
          <h2 class="panel-title">引用情况</h2>
          <div class="bar-row">
            <span class="bar-label">带引用解释</span>
            <div class="bar-track"><div class="bar-fill" :style="{ width: coverage.total_explanations ? (coverage.with_citations / coverage.total_explanations) * 100 + '%' : '0%' }" /></div>
            <span class="bar-count">{{ coverage.with_citations }} / {{ coverage.total_explanations }}</span>
          </div>
          <div class="bar-row">
            <span class="bar-label">确定性降级</span>
            <div class="bar-track"><div class="bar-fill danger" :style="{ width: coverage.total_explanations ? (coverage.deterministic_count / coverage.total_explanations) * 100 + '%' : '0%' }" /></div>
            <span class="bar-count">{{ coverage.deterministic_count }} / {{ coverage.total_explanations }}</span>
          </div>
          <p class="muted note">{{ coverage.window_note }}</p>
        </section>

        <section class="panel">
          <h2 class="panel-title">质量反馈基线</h2>
          <div class="quality-grid">
            <div><span class="quality-icon"><ThumbsUp :size="15" /></span><div class="quality-value">{{ coverage.quality.helpful }}</div><div class="quality-label">有帮助</div></div>
            <div><span class="quality-icon partial">◐</span><div class="quality-value">{{ coverage.quality.partially_helpful }}</div><div class="quality-label">部分有帮助</div></div>
            <div><span class="quality-icon bad"><XCircle :size="15" /></span><div class="quality-value">{{ coverage.quality.not_helpful }}</div><div class="quality-label">无帮助</div></div>
            <div><span class="quality-icon"><Users :size="15" /></span><div class="quality-value">{{ coverage.quality.contributors }}</div><div class="quality-label">反馈贡献者</div></div>
          </div>
          <div v-if="coverage.quality.by_model.length" class="model-table">
            <h3 class="panel-subtitle">按模型</h3>
            <table>
              <thead><tr><th>模型</th><th>反馈</th><th>帮助</th><th>部分帮助</th><th>无帮助</th><th>好评率</th></tr></thead>
              <tbody>
                <tr v-for="row in coverage.quality.by_model" :key="row.model">
                  <td>{{ row.model }}</td>
                  <td>{{ row.total_feedback }}</td>
                  <td>{{ row.helpful }}</td>
                  <td>{{ row.partially_helpful }}</td>
                  <td>{{ row.not_helpful }}</td>
                  <td>{{ (row.helpful_rate * 100).toFixed(1) }}%</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>
      <p v-else class="muted">暂无解释覆盖率数据（尚未生成 AI 解释或服务不可用）。</p>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.page { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 18px; }
.page-title { margin: 0; font-size: 20px; color: #1e2a3a; }
.page-subtitle { margin: 4px 0 0; color: #6b7686; font-size: 12px; }
.ghost-button { display: inline-flex; align-items: center; gap: 6px; border: 1px solid #cbd5e1; background: #fff; color: #334155; padding: 8px 14px; border-radius: 8px; cursor: pointer; font-size: 13px; }
.ghost-button:hover { background: #f1f5f9; }
.error-message { color: #b34a4a; background: #fdecec; border: 1px solid #f3c2c2; padding: 10px 14px; border-radius: 8px; font-size: 13px; }
.muted { color: #8593a8; font-size: 13px; }
.rate-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; margin-bottom: 18px; }
.rate-card { background: #fff; border: 1px solid #dfe7f0; border-radius: 12px; padding: 16px; display: flex; flex-direction: column; gap: 6px; }
.rate-label { color: #6b7686; font-size: 12px; }
.rate-value { font-size: 26px; font-weight: 700; color: #1e2a3a; }
.rate-sub { color: #8593a8; font-size: 11px; }
.panel { background: #fff; border: 1px solid #dfe7f0; border-radius: 12px; padding: 18px; margin-bottom: 18px; }
.panel-title { margin: 0 0 14px; font-size: 15px; color: #1e2a3a; }
.panel-subtitle { margin: 16px 0 8px; font-size: 13px; color: #334155; }
.bar-row { display: grid; grid-template-columns: 120px 1fr 90px; align-items: center; gap: 12px; margin-bottom: 10px; }
.bar-label { font-size: 12px; color: #52606d; }
.bar-track { height: 10px; background: #e9eef5; border-radius: 5px; overflow: hidden; }
.bar-fill { height: 100%; background: #2d6fb0; border-radius: 5px; }
.bar-fill.danger { background: #b34a4a; }
.bar-count { font-size: 12px; color: #52606d; text-align: right; }
.note { margin-top: 12px; font-size: 11px; }
.quality-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 12px; }
.quality-grid > div { background: #f5f8fc; border: 1px solid #e2eaf3; border-radius: 10px; padding: 12px; text-align: center; }
.quality-icon { display: inline-flex; color: #2d6fb0; justify-content: center; }
.quality-icon.partial { color: #a17a24; font-size: 16px; }
.quality-icon.bad { color: #b34a4a; }
.quality-value { font-size: 22px; font-weight: 700; color: #1e2a3a; margin-top: 4px; }
.quality-label { font-size: 11px; color: #6b7686; margin-top: 2px; }
.model-table { margin-top: 6px; }
.model-table table { width: 100%; border-collapse: collapse; font-size: 12px; }
.model-table th, .model-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #edf1f6; color: #334155; }
.model-table th { color: #6b7686; font-weight: 500; }
</style>