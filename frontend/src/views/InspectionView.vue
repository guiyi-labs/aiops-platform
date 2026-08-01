<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Boxes,
  ChevronDown,
  ClipboardList,
  Play,
  Plus,
  RefreshCw,
  Trash2,
  X,
} from 'lucide-vue-next'

import {
  createInspectionPlan,
  deleteInspectionPlan,
  listInspectionPlans,
  listInspectionResults,
  listInspectionRulesCatalog,
  listInspectionTasks,
  runInspection,
} from '../api/inspection'
import { listClusters } from '../api/clusters'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { CreateInspectionPlanRequest } from '../types/inspection'
import type { InspectionPlanView, InspectionResultView, InspectionRule, InspectionTaskView } from '../types/inspection'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()

const rules = ref<InspectionRule[]>([])
const plans = ref<InspectionPlanView[]>([])
const tasks = ref<InspectionTaskView[]>([])
const results = ref<InspectionResultView[]>([])
const clusters = ref<Cluster[]>([])

const rulesOpen = ref(false)
const rulesLoading = ref(false)
const rulesError = ref('')

const plansLoading = ref(false)
const plansError = ref('')
const showCreateForm = ref(false)
const creating = ref(false)
const createError = ref('')

const tasksLoading = ref(false)
const tasksError = ref('')
const resultsLoading = ref(false)
const resultsError = ref('')

const selectedTaskId = ref<number | null>(null)
const runningPlanId = ref<number | null>(null)
const deletingPlanId = ref<number | null>(null)

const newPlan = ref<CreateInspectionPlanRequest>({
  name: '',
  cluster_ids: [],
  rule_codes: [],
  cron_spec: '',
  enabled: true,
})

const canCreate = computed(
  () =>
    newPlan.value.name.trim().length > 0 &&
    newPlan.value.cluster_ids.length > 0 &&
    newPlan.value.rule_codes.length > 0,
)

function formatTime(value?: string): string {
  if (!value) return '--'
  try {
    return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
  } catch {
    return value
  }
}

function severityClass(severity: string): string {
  const s = (severity || '').toLowerCase()
  if (s === 'critical') return 'critical'
  if (s === 'warning') return 'warning'
  if (s === 'info') return 'info'
  return 'neutral'
}

function taskStatusClass(status: string): string {
  const s = (status || '').toLowerCase()
  if (s === 'pending') return 'pending'
  if (s === 'running') return 'running'
  if (s === 'completed') return 'completed'
  if (s === 'failed') return 'failed'
  return 'neutral'
}

function resultStateClass(state: string): string {
  const s = (state || '').toLowerCase()
  if (s === 'active') return 'active'
  if (s === 'resolved') return 'resolved'
  if (s === 'stale') return 'stale'
  return 'neutral'
}

function clusterName(id: number): string {
  return clusters.value.find((c) => c.id === id)?.name ?? `集群 #${id}`
}

function clusterNames(ids: number[]): string {
  if (!ids.length) return '--'
  return ids.map(clusterName).join('、')
}

function resetCreateForm() {
  newPlan.value = { name: '', cluster_ids: [], rule_codes: [], cron_spec: '', enabled: true }
  createError.value = ''
}

async function loadClusters() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((c) => c.enabled)
  } catch (err) {
    plansError.value = err instanceof APIError ? err.message : '加载集群列表失败'
  }
}

async function loadRules() {
  rulesLoading.value = true
  rulesError.value = ''
  try {
    rules.value = (await listInspectionRulesCatalog(auth.accessToken)).items
  } catch (err) {
    rulesError.value = err instanceof APIError ? err.message : '加载规则目录失败'
  } finally {
    rulesLoading.value = false
  }
}

async function loadPlans() {
  plansLoading.value = true
  plansError.value = ''
  try {
    plans.value = (await listInspectionPlans(auth.accessToken)).items
  } catch (err) {
    plansError.value = err instanceof APIError ? err.message : '加载巡检计划失败'
  } finally {
    plansLoading.value = false
  }
}

async function loadTasks() {
  tasksLoading.value = true
  tasksError.value = ''
  try {
    const resp = await listInspectionTasks(auth.accessToken, { limit: 100 })
    tasks.value = resp.items
    if (selectedTaskId.value && !tasks.value.some((t) => t.id === selectedTaskId.value)) {
      selectedTaskId.value = null
      results.value = []
    } else if (selectedTaskId.value) {
      void loadResults(selectedTaskId.value)
    }
  } catch (err) {
    tasksError.value = err instanceof APIError ? err.message : '加载巡检任务失败'
  } finally {
    tasksLoading.value = false
  }
}

async function loadResults(taskId: number) {
  resultsLoading.value = true
  resultsError.value = ''
  try {
    const resp = await listInspectionResults(auth.accessToken, { task_id: taskId, limit: 200 })
    results.value = resp.items
  } catch (err) {
    resultsError.value = err instanceof APIError ? err.message : '加载巡检结果失败'
  } finally {
    resultsLoading.value = false
  }
}

function selectTask(task: InspectionTaskView) {
  selectedTaskId.value = task.id
  void loadResults(task.id)
}

function clearTaskFilter() {
  selectedTaskId.value = null
  results.value = []
  resultsError.value = ''
}

async function handleCreate() {
  if (!canCreate.value) return
  creating.value = true
  createError.value = ''
  try {
    await createInspectionPlan(auth.accessToken, newPlan.value)
    showCreateForm.value = false
    resetCreateForm()
    await loadPlans()
  } catch (err) {
    createError.value = err instanceof APIError ? err.message : '创建巡检计划失败'
  } finally {
    creating.value = false
  }
}

async function handleDelete(plan: InspectionPlanView) {
  deletingPlanId.value = plan.id
  plansError.value = ''
  try {
    await deleteInspectionPlan(auth.accessToken, plan.id)
    await loadPlans()
  } catch (err) {
    plansError.value = err instanceof APIError ? err.message : '删除巡检计划失败'
  } finally {
    deletingPlanId.value = null
  }
}

async function handleRunNow(plan: InspectionPlanView) {
  runningPlanId.value = plan.id
  plansError.value = ''
  try {
    await runInspection(auth.accessToken, { cluster_ids: plan.cluster_ids, rule_codes: plan.rule_codes })
    await loadTasks()
  } catch (err) {
    plansError.value = err instanceof APIError ? err.message : '执行巡检失败'
  } finally {
    runningPlanId.value = null
  }
}

async function refreshAll() {
  await Promise.all([loadPlans(), loadTasks()])
}

onMounted(async () => {
  await Promise.all([loadClusters(), loadRules(), loadPlans(), loadTasks()])
})
</script>

<template>
  <ConsoleLayout eyebrow="智能运维" title="智能巡检">
    <template #actions>
      <button
        type="button"
        class="icon-button"
        title="刷新"
        aria-label="刷新巡检数据"
        :disabled="plansLoading || tasksLoading"
        @click="refreshAll"
      >
        <RefreshCw :size="18" :class="{ spinning: plansLoading }" />
      </button>
    </template>

    <!-- 规则目录 -->
    <section class="catalog-panel">
      <button type="button" class="catalog-toggle" :aria-expanded="rulesOpen" @click="rulesOpen = !rulesOpen">
        <ClipboardList :size="18" />
        <span class="catalog-toggle-title">
          <strong>规则目录</strong>
          <small>{{ rules.length }} 条规则</small>
        </span>
        <ChevronDown :size="18" class="catalog-chevron" :class="{ open: rulesOpen }" />
      </button>

      <div v-if="rulesOpen" class="catalog-body">
        <p v-if="rulesError" class="error-message">{{ rulesError }}</p>
        <div v-else-if="rulesLoading" class="panel-empty">加载中…</div>
        <div v-else-if="rules.length === 0" class="panel-empty">暂无巡检规则</div>
        <div v-else class="table-scroll">
          <table class="compact-table">
            <thead>
              <tr>
                <th>规则码</th>
                <th>领域</th>
                <th>默认级别</th>
                <th>信号码</th>
                <th>描述</th>
                <th>修复建议</th>
                <th>超时(秒)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="rule in rules" :key="rule.code">
                <td class="mono">{{ rule.code }}</td>
                <td><span class="domain-badge">{{ rule.domain }}</span></td>
                <td><span class="sev-badge" :class="severityClass(rule.default_severity)">{{ rule.default_severity }}</span></td>
                <td class="mono">{{ rule.signal_code }}</td>
                <td>{{ rule.description }}</td>
                <td class="muted">{{ rule.remediation }}</td>
                <td class="mono">{{ rule.timeout_seconds }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <!-- 巡检计划 -->
    <section class="section-block">
      <header class="section-head">
        <div class="section-head-title">
          <Boxes :size="18" />
          <h2>巡检计划</h2>
          <span class="muted">{{ plans.length }} 个计划</span>
        </div>
        <button type="button" class="primary-button" @click="showCreateForm = !showCreateForm">
          <Plus :size="16" />
          <span>创建计划</span>
        </button>
      </header>

      <p v-if="plansError" class="error-message">{{ plansError }}</p>

      <div v-if="showCreateForm" class="create-panel">
        <header class="create-panel-head">
          <strong>新建巡检计划</strong>
          <button type="button" class="icon-button" title="关闭" aria-label="关闭表单" @click="showCreateForm = false; resetCreateForm()">
            <X :size="16" />
          </button>
        </header>
        <div class="create-grid">
          <label class="form-field">
            <span>计划名称</span>
            <input v-model="newPlan.name" type="text" placeholder="例: 生产集群每日巡检" maxlength="128" />
          </label>
          <label class="form-field">
            <span>Cron 表达式</span>
            <input v-model="newPlan.cron_spec" type="text" placeholder="例: 0 2 * * * (可留空手动触发)" />
          </label>
        </div>

        <div class="multi-grid">
          <div class="multi-block">
            <span class="multi-label">目标集群 · {{ newPlan.cluster_ids.length }}/{{ clusters.length }}</span>
            <div v-if="clusters.length === 0" class="multi-empty muted">没有已启用的集群</div>
            <div v-else class="multi-list">
              <label v-for="c in clusters" :key="c.id" class="check-row">
                <input type="checkbox" :value="c.id" v-model="newPlan.cluster_ids" />
                <span>{{ c.name }}</span>
              </label>
            </div>
          </div>
          <div class="multi-block">
            <span class="multi-label">巡检规则 · {{ newPlan.rule_codes.length }}/{{ rules.length }}</span>
            <div v-if="rules.length === 0" class="multi-empty muted">规则目录为空</div>
            <div v-else class="multi-list">
              <label v-for="rule in rules" :key="rule.code" class="check-row">
                <input type="checkbox" :value="rule.code" v-model="newPlan.rule_codes" />
                <span class="mono">{{ rule.code }}</span>
                <small class="muted">{{ rule.domain }}</small>
              </label>
            </div>
          </div>
        </div>

        <label class="enable-check">
          <input type="checkbox" v-model="newPlan.enabled" />
          <span>启用定时调度</span>
        </label>

        <p v-if="createError" class="error-message">{{ createError }}</p>

        <div class="form-actions">
          <button type="button" class="secondary-button" :disabled="creating" @click="resetCreateForm">重置</button>
          <button type="button" class="primary-button" :disabled="!canCreate || creating" @click="handleCreate">
            <span>{{ creating ? '创建中…' : '创建计划' }}</span>
          </button>
        </div>
      </div>

      <div v-if="plansLoading" class="panel-empty">加载中…</div>
      <div v-else-if="plans.length === 0 && !showCreateForm" class="resource-empty">
        <ClipboardList :size="30" />
        <strong>暂无巡检计划</strong>
        <span>创建计划后可定时或在指定集群上执行巡检规则。</span>
      </div>
      <div v-else class="plan-grid">
        <article v-for="plan in plans" :key="plan.id" class="plan-card">
          <header class="plan-card-head">
            <div class="plan-card-title">
              <strong>{{ plan.name }}</strong>
              <span class="state-badge" :class="plan.enabled ? 'enabled' : 'disabled'">
                {{ plan.enabled ? '已启用' : '已禁用' }}
              </span>
            </div>
            <span class="plan-id muted">#{{ plan.id }}</span>
          </header>
          <dl class="plan-meta">
            <div><dt>目标集群</dt><dd>{{ plan.cluster_ids.length }} 个 · {{ clusterNames(plan.cluster_ids) }}</dd></div>
            <div><dt>巡检规则</dt><dd>{{ plan.rule_codes.length }} 条</dd></div>
            <div><dt>Cron 表达式</dt><dd class="mono">{{ plan.cron_spec || '—' }}</dd></div>
            <div><dt>上次运行</dt><dd>{{ formatTime(plan.last_run_at) }}</dd></div>
            <div><dt>下次运行</dt><dd>{{ formatTime(plan.next_run_at) }}</dd></div>
          </dl>
          <footer class="plan-card-actions">
            <button
              type="button"
              class="secondary-button"
              :disabled="runningPlanId === plan.id"
              :title="plan.cluster_ids.length === 0 ? '计划未关联集群' : '立即执行'"
              @click="handleRunNow(plan)"
            >
              <Play :size="15" :class="{ spinning: runningPlanId === plan.id }" />
              <span>{{ runningPlanId === plan.id ? '执行中…' : '立即执行' }}</span>
            </button>
            <button
              type="button"
              class="danger-button"
              :disabled="deletingPlanId === plan.id"
              title="删除计划"
              aria-label="删除计划"
              @click="handleDelete(plan)"
            >
              <Trash2 :size="15" />
            </button>
          </footer>
        </article>
      </div>
    </section>

    <!-- 任务与结果 -->
    <section class="section-block">
      <header class="section-head">
        <div class="section-head-title">
          <ClipboardList :size="18" />
          <h2>任务与结果</h2>
          <span class="muted">{{ tasks.length }} 个任务</span>
        </div>
        <div v-if="selectedTaskId" class="filter-pill">
          <span>已筛选任务 #{{ selectedTaskId }}</span>
          <button type="button" class="icon-button compact" title="清除筛选" aria-label="清除筛选" @click="clearTaskFilter">
            <X :size="14" />
          </button>
        </div>
      </header>

      <p v-if="tasksError" class="error-message">{{ tasksError }}</p>

      <div v-if="tasksLoading" class="panel-empty">加载中…</div>
      <div v-else-if="tasks.length === 0" class="panel-empty">暂无巡检任务</div>
      <div v-else class="table-scroll">
        <table class="compact-table task-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>计划快照</th>
              <th>触发原因</th>
              <th>状态</th>
              <th>集群进度</th>
              <th>发现数</th>
              <th>开始时间</th>
              <th>结束时间</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="task in tasks"
              :key="task.id"
              class="task-row"
              :class="{ active: selectedTaskId === task.id }"
              @click="selectTask(task)"
            >
              <td class="mono">#{{ task.id }}</td>
              <td>{{ task.plan_name_snapshot }}</td>
              <td>{{ task.trigger_reason }}</td>
              <td><span class="state-badge" :class="taskStatusClass(task.status)">{{ task.status }}</span></td>
              <td>{{ task.completed_clusters }} / {{ task.total_clusters }}</td>
              <td><strong :class="{ 'find-positive': task.finding_count > 0 }">{{ task.finding_count }}</strong></td>
              <td>{{ formatTime(task.started_at) }}</td>
              <td>{{ formatTime(task.finished_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="results-block">
        <header class="results-head">
          <h3>巡检结果</h3>
          <span v-if="selectedTaskId" class="muted">任务 #{{ selectedTaskId }} · {{ results.length }} 条</span>
          <span v-else class="muted">点击上方任务以查看结果</span>
        </header>

        <p v-if="resultsError" class="error-message">{{ resultsError }}</p>
        <div v-else-if="!selectedTaskId" class="panel-empty">选择一个任务以加载其巡检结果</div>
        <div v-else-if="resultsLoading" class="panel-empty">加载中…</div>
        <div v-else-if="results.length === 0" class="panel-empty">该任务暂无巡检结果</div>
        <div v-else class="table-scroll">
          <table class="compact-table result-table">
            <thead>
              <tr>
                <th>规则码</th>
                <th>级别</th>
                <th>状态</th>
                <th>命名空间</th>
                <th>资源</th>
                <th>观测时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in results" :key="item.id">
                <td class="mono">{{ item.rule_code }}</td>
                <td><span class="sev-badge" :class="severityClass(item.severity)">{{ item.severity }}</span></td>
                <td><span class="state-badge" :class="resultStateClass(item.state)">{{ item.state }}</span></td>
                <td>{{ item.namespace || '—' }}</td>
                <td class="mono">{{ item.resource_kind ? `${item.resource_kind}/${item.resource_name || ''}` : '—' }}</td>
                <td>{{ formatTime(item.observed_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.catalog-panel {
  margin-top: 18px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}
.catalog-toggle {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
  color: var(--text-primary);
  background: transparent;
  border: 0;
  text-align: left;
  cursor: pointer;
}
.catalog-toggle:hover {
  background: var(--bg-secondary);
}
.catalog-toggle-title {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.catalog-toggle-title strong {
  font-size: 14px;
}
.catalog-toggle-title small {
  color: var(--text-tertiary);
  font-size: 11px;
}
.catalog-chevron {
  margin-left: auto;
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}
.catalog-chevron.open {
  transform: rotate(180deg);
}
.catalog-body {
  padding: 0 16px 16px;
  border-top: 1px solid var(--border-subtle);
}

.section-block {
  margin-top: 22px;
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.section-head-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-head-title h2 {
  margin: 0;
  font-size: 16px;
}
.section-head-title .muted {
  font-size: 12px;
}

.table-scroll {
  width: 100%;
  overflow-x: auto;
}
.mono {
  font-family: var(--font-mono);
  font-size: 11px;
}
.muted {
  color: var(--text-tertiary);
}

.domain-badge {
  display: inline-flex;
  padding: 2px 8px;
  color: var(--blue-700, #1b4499);
  font-size: 11px;
  font-weight: 600;
  background: var(--blue-50, #eef4ff);
  border-radius: var(--radius-full);
}

.sev-badge {
  display: inline-flex;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: var(--radius-full);
}
.sev-badge.critical {
  color: var(--status-danger);
  background: var(--danger-bg);
}
.sev-badge.warning {
  color: var(--status-warning);
  background: var(--warning-bg);
}
.sev-badge.info {
  color: var(--status-info);
  background: var(--info-bg);
}
.sev-badge.neutral {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}

.state-badge {
  display: inline-flex;
  padding: 2px 9px;
  font-size: 11px;
  font-weight: 600;
  border-radius: var(--radius-full);
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}
.state-badge.enabled {
  color: var(--status-success);
  background: var(--success-bg);
}
.state-badge.disabled {
  color: var(--text-muted);
  background: var(--bg-tertiary);
}
.state-badge.pending {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}
.state-badge.running {
  color: var(--status-warning);
  background: var(--warning-bg);
}
.state-badge.completed {
  color: var(--status-success);
  background: var(--success-bg);
}
.state-badge.failed {
  color: var(--status-danger);
  background: var(--danger-bg);
}
.state-badge.active {
  color: var(--status-danger);
  background: var(--danger-bg);
}
.state-badge.resolved {
  color: var(--status-success);
  background: var(--success-bg);
}
.state-badge.stale {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}

.create-panel {
  margin-bottom: 16px;
  padding: 16px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.create-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.create-panel-head strong {
  font-size: 14px;
}
.create-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}
.form-field {
  display: grid;
  gap: 5px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.form-field input {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font: inherit;
  font-size: var(--text-sm);
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  outline: none;
  box-shadow: var(--shadow-inset);
}
.form-field input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.multi-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 14px;
}
.multi-block {
  display: grid;
  gap: 8px;
}
.multi-label {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.multi-list {
  max-height: 200px;
  overflow-y: auto;
  padding: 8px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.multi-empty {
  padding: 16px;
  text-align: center;
  font-size: 12px;
}
.check-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 4px;
  cursor: pointer;
}
.check-row span {
  font-size: 12px;
  color: var(--text-primary);
}
.check-row small {
  margin-left: auto;
  font-size: 11px;
}
.check-row input {
  margin: 0;
}
.enable-check {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: var(--text-primary);
  font-size: 13px;
  cursor: pointer;
}
.enable-check input {
  margin: 0;
}
.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.plan-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 14px;
}
.plan-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}
.plan-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}
.plan-card-title {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.plan-card-title strong {
  font-size: 14px;
  color: var(--text-primary);
  overflow-wrap: anywhere;
}
.plan-id {
  font-size: 11px;
}
.plan-meta {
  display: grid;
  gap: 8px;
  margin: 0;
}
.plan-meta > div {
  display: grid;
  gap: 2px;
}
.plan-meta dt {
  color: var(--text-tertiary);
  font-size: 10px;
}
.plan-meta dd {
  margin: 0;
  color: var(--text-primary);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.plan-card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}

.task-table .task-row {
  cursor: pointer;
  transition: background var(--transition-fast);
}
.task-table .task-row:hover {
  background: var(--bg-secondary);
}
.task-table .task-row.active {
  background: var(--accent-subtle);
  box-shadow: inset 3px 0 var(--accent-primary);
}
.find-positive {
  color: var(--status-danger);
}

.filter-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px 4px 12px;
  background: var(--accent-subtle);
  border-radius: var(--radius-full);
  font-size: 12px;
  color: var(--accent-primary);
  font-weight: 600;
}
.icon-button.compact {
  width: 26px;
  height: 26px;
}

.results-block {
  margin-top: 18px;
}
.results-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.results-head h3 {
  margin: 0;
  font-size: 14px;
  color: var(--text-primary);
}

@media (max-width: 720px) {
  .create-grid,
  .multi-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
