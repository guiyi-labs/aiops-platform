<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  CheckCircle2,
  Clock,
  Cpu,
  FlaskConical,
  Minus,
  Play,
  RefreshCw,
  TrendingUp,
  XCircle,
} from 'lucide-vue-next'

import { getQualityReport, runQualityReplay } from '../api/aiops'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { QualityReport, ScenarioDelta } from '../types/aiops'

const auth = useAuthStore()

const loading = ref(true)
const error = ref('')
const report = ref<QualityReport | null>(null)
const replayLoading = ref(false)
const replayMessage = ref('')
const replayError = ref('')

const summary = computed(() => report.value?.summary ?? null)

const regressedScenarios = computed(() =>
  report.value?.scenario_results.filter((s) => s.delta === 'regressed') ?? [],
)

const improvedScenarios = computed(() =>
  report.value?.scenario_results.filter((s) => s.delta === 'improved') ?? [],
)

const engineChanged = computed(() => {
  if (!report.value) return []
  const before = report.value.engine_versions_before
  const after = report.value.engine_versions_after
  const keys: (keyof typeof before)[] = [
    'signal_version', 'topology_version', 'slo_version',
    'correlation_version', 'investigator_version', 'automation_version', 'verifier_version',
  ]
  return keys.filter((k) => before[k] !== after[k])
})

function deltaIcon(delta: ScenarioDelta) {
  switch (delta) {
    case 'improved': return ArrowUpRight
    case 'regressed': return ArrowDownRight
    case 'preserved': return CheckCircle2
    case 'unchanged': return Minus
  }
}

function deltaColor(delta: ScenarioDelta): string {
  switch (delta) {
    case 'improved': return 'var(--status-success)'
    case 'regressed': return 'var(--status-danger)'
    case 'preserved': return 'var(--status-info)'
    case 'unchanged': return 'var(--text-muted)'
  }
}

function deltaLabel(delta: ScenarioDelta): string {
  switch (delta) {
    case 'improved': return '改善'
    case 'regressed': return '回归'
    case 'preserved': return '保持'
    case 'unchanged': return '未变'
  }
}

function formatDateTime(iso: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return iso
  }
}

async function loadReport() {
  loading.value = true
  error.value = ''
  try {
    report.value = await getQualityReport(auth.accessToken)
  } catch (e) {
    if (e instanceof APIError && e.status === 404) {
      error.value = '质量报告尚未生成。点击"运行回放"生成最新报告。'
    } else if (e instanceof APIError) {
      error.value = e.message
    } else {
      error.value = '加载质量报告失败'
    }
  } finally {
    loading.value = false
  }
}

async function handleReplay() {
  replayLoading.value = true
  replayError.value = ''
  replayMessage.value = ''
  try {
    const result = await runQualityReplay(auth.accessToken)
    replayMessage.value = result.message || '回放任务已启动，请稍后刷新查看结果。'
    setTimeout(() => loadReport(), 3000)
  } catch (e) {
    replayError.value = e instanceof APIError ? e.message : '运行回放失败'
  } finally {
    replayLoading.value = false
  }
}

onMounted(loadReport)
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="质量仪表盘">
    <template #actions>
      <button
        class="secondary-button"
        :disabled="replayLoading"
        @click="handleReplay"
      >
        <Play v-if="!replayLoading" :size="14" />
        <RefreshCw v-else :size="14" class="spinning" />
        {{ replayLoading ? '回放中…' : '运行回放' }}
      </button>
      <button class="secondary-button" :disabled="loading" @click="loadReport">
        <RefreshCw :size="14" :class="{ spinning: loading }" />
        刷新
      </button>
    </template>

    <div class="quality-content">
      <!-- Loading -->
      <div v-if="loading" class="loading-state">
        <RefreshCw :size="24" class="spinning" />
        <span>加载质量报告中…</span>
      </div>

      <!-- Error / Empty -->
      <div v-else-if="error" class="empty-state">
        <FlaskConical :size="40" />
        <p>{{ error }}</p>
      </div>

      <!-- Report -->
      <template v-else-if="report">
        <!-- Replay feedback -->
        <div v-if="replayMessage" class="alert alert-success">
          <CheckCircle2 :size="16" />
          <span>{{ replayMessage }}</span>
        </div>
        <div v-if="replayError" class="alert alert-error">
          <AlertTriangle :size="16" />
          <span>{{ replayError }}</span>
        </div>

        <!-- Summary cards -->
        <div v-if="summary" class="summary-grid">
          <div class="summary-card">
            <div class="summary-value">{{ summary.total_scenarios }}</div>
            <div class="summary-label">场景总数</div>
          </div>
          <div class="summary-card success">
            <div class="summary-value">{{ summary.passed_after }}</div>
            <div class="summary-label">通过（变更后）</div>
          </div>
          <div class="summary-card success-muted">
            <div class="summary-value">{{ summary.passed_before }}</div>
            <div class="summary-label">通过（变更前）</div>
          </div>
          <div class="summary-card improved">
            <TrendingUp :size="18" />
            <div class="summary-value">{{ summary.improved }}</div>
            <div class="summary-label">改善</div>
          </div>
          <div class="summary-card regressed">
            <AlertTriangle :size="18" />
            <div class="summary-value">{{ summary.regressed }}</div>
            <div class="summary-label">回归</div>
          </div>
        </div>

        <!-- Engine version comparison -->
        <section v-if="engineChanged.length > 0" class="quality-card">
          <h3 class="card-title">引擎版本变更</h3>
          <table class="data-table">
            <thead>
              <tr>
                <th>引擎</th>
                <th>变更前</th>
                <th>变更后</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="key in engineChanged" :key="key">
                <td class="engine-name">{{ key.replace('_version', '') }}</td>
                <td class="mono">{{ report.engine_versions_before[key] }}</td>
                <td class="mono">{{ report.engine_versions_after[key] }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- Changed components -->
        <section v-if="report.changed_components.length > 0" class="quality-card">
          <h3 class="card-title">变更组件</h3>
          <div class="tag-list">
            <span v-for="comp in report.changed_components" :key="comp" class="component-tag">
              {{ comp }}
            </span>
          </div>
        </section>

        <!-- Regressed scenarios (highlight first) -->
        <section v-if="regressedScenarios.length > 0" class="quality-card alert-card">
          <h3 class="card-title">
            <AlertTriangle :size="18" />
            回归场景（{{ regressedScenarios.length }}）
          </h3>
          <table class="data-table">
            <thead>
              <tr>
                <th>场景 ID</th>
                <th>变更前</th>
                <th>变更后</th>
                <th>步骤</th>
                <th>备注</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in regressedScenarios" :key="s.scenario_id">
                <td class="mono">{{ s.scenario_id }}</td>
                <td>
                  <CheckCircle2 v-if="s.passed_before" :size="14" style="color: var(--status-success)" />
                  <XCircle v-else :size="14" style="color: var(--status-danger)" />
                </td>
                <td>
                  <CheckCircle2 v-if="s.passed_after" :size="14" style="color: var(--status-success)" />
                  <XCircle v-else :size="14" style="color: var(--status-danger)" />
                </td>
                <td>{{ s.steps_passed_after }}/{{ s.steps_total }}</td>
                <td>{{ s.notes || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- Improved scenarios -->
        <section v-if="improvedScenarios.length > 0" class="quality-card">
          <h3 class="card-title">
            <TrendingUp :size="18" />
            改善场景（{{ improvedScenarios.length }}）
          </h3>
          <table class="data-table">
            <thead>
              <tr>
                <th>场景 ID</th>
                <th>变更前</th>
                <th>变更后</th>
                <th>步骤</th>
                <th>备注</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in improvedScenarios" :key="s.scenario_id">
                <td class="mono">{{ s.scenario_id }}</td>
                <td>
                  <CheckCircle2 v-if="s.passed_before" :size="14" style="color: var(--status-success)" />
                  <XCircle v-else :size="14" style="color: var(--status-danger)" />
                </td>
                <td>
                  <CheckCircle2 v-if="s.passed_after" :size="14" style="color: var(--status-success)" />
                  <XCircle v-else :size="14" style="color: var(--status-danger)" />
                </td>
                <td>{{ s.steps_passed_after }}/{{ s.steps_total }}</td>
                <td>{{ s.notes || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- All scenarios -->
        <section class="quality-card">
          <h3 class="card-title">全部场景结果</h3>
          <table class="data-table">
            <thead>
              <tr>
                <th>场景 ID</th>
                <th>结果</th>
                <th>变化</th>
                <th>步骤（前→后 / 总）</th>
                <th>备注</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in report.scenario_results" :key="s.scenario_id">
                <td class="mono">{{ s.scenario_id }}</td>
                <td>
                  <CheckCircle2 v-if="s.passed_after" :size="14" style="color: var(--status-success)" />
                  <XCircle v-else :size="14" style="color: var(--status-danger)" />
                </td>
                <td>
                  <span class="delta-badge" :style="{ color: deltaColor(s.delta) }">
                    <component :is="deltaIcon(s.delta)" :size="12" />
                    {{ deltaLabel(s.delta) }}
                  </span>
                </td>
                <td>{{ s.steps_passed_before }} → {{ s.steps_passed_after }} / {{ s.steps_total }}</td>
                <td>{{ s.notes || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- Report metadata -->
        <section class="quality-card meta-card">
          <div class="meta-row">
            <span class="meta-label"><Cpu :size="14" /> 报告版本</span>
            <span class="mono">{{ report.report_version }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label"><FlaskConical :size="14" /> 数据集版本</span>
            <span class="mono">{{ report.dataset_version_before }} → {{ report.dataset_version_after }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label"><Clock :size="14" /> 生成时间</span>
            <span>{{ formatDateTime(report.generated_at) }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label"><CheckCircle2 :size="14" /> 审批状态</span>
            <span class="badge" :class="report.approved ? 'badge-success' : 'badge-muted'">
              {{ report.approved ? '已审批' : '未审批' }}
            </span>
            <span v-if="report.reviewer" class="meta-reviewer">审批人: {{ report.reviewer }}</span>
          </div>
        </section>
      </template>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.quality-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 60px 20px;
  color: var(--text-secondary);
  font-size: 14px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 60px 20px;
  color: var(--text-muted);
  text-align: center;
}

.empty-state svg {
  opacity: 0.4;
}

.alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: var(--radius-lg);
  font-size: 13px;
}

.alert-success {
  background: var(--success-bg);
  color: var(--status-success);
  border: 1px solid #bbf7d0;
}

.alert-error {
  background: var(--danger-bg);
  color: var(--status-danger);
  border: 1px solid #fecaca;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}

.summary-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.summary-card.success { border-left: 3px solid var(--status-success); }
.summary-card.success-muted { border-left: 3px solid var(--text-muted); }
.summary-card.improved { border-left: 3px solid var(--status-success); }
.summary-card.regressed { border-left: 3px solid var(--status-danger); }

.summary-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1;
}

.summary-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.quality-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: 20px;
}

.quality-card.alert-card {
  border-color: #fecaca;
  background: #fff5f5;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 16px 0;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.data-table th {
  text-align: left;
  padding: 8px 12px;
  font-weight: 500;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-default);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

.data-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}

.data-table td.mono, .data-table th.mono {
  font-family: 'SF Mono', 'Cascadia Code', monospace;
  font-size: 12px;
}

.engine-name {
  font-weight: 500;
  text-transform: capitalize;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.component-tag {
  padding: 4px 12px;
  background: var(--accent-subtle);
  color: var(--accent-primary);
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
}

.delta-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  font-weight: 500;
}

.meta-card {
  padding: 16px 20px;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  font-size: 13px;
  color: var(--text-secondary);
}

.meta-row + .meta-row {
  border-top: 1px solid var(--border-subtle);
}

.meta-label {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 140px;
  color: var(--text-muted);
}

.meta-reviewer {
  margin-left: auto;
  color: var(--text-muted);
}

.badge {
  padding: 2px 10px;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
}

.badge-success {
  background: var(--success-bg);
  color: var(--status-success);
}

.badge-muted {
  background: var(--gray-100);
  color: var(--text-muted);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.spinning {
  animation: spin 0.8s linear infinite;
}
</style>
