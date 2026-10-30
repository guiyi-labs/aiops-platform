<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Bell, BellOff, FilePlus2, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { createAlertRule, deleteAlertRule, getAlertOverview, listAlertInstances, listAlertRules, patchAlertRule } from '../api/alert'
import { createIncident } from '../api/incidents'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { AlertOverviewResponse, AlertRule, AlertRuleCreate, AlertInstance, AlertInstanceState } from '../types/alert'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref(0)
const rules = ref<AlertRule[]>([])
const instances = ref<AlertInstance[]>([])
const loading = ref(false)
const errorMessage = ref('')
const noticeMessage = ref('')
const showCreateDialog = ref(false)
const stateFilter = ref<AlertInstanceState | ''>('firing')
const overview = ref<AlertOverviewResponse | null>(null)
const overviewLoading = ref(false)
const overviewWindowMinutes = ref(1440)

const newRule = ref<AlertRuleCreate>({
  display_name: '',
  resource_kind: 'Node',
  resource_name: '',
  metric_name: 'cpu',
  operator: 'gte',
  threshold: 3000000000,
  for_seconds: 300,
  minimum_points: 5,
})

const canManage = computed(() => auth.user?.roles.some((role) => role === 'system_admin' || role === 'operations_admin') ?? false)

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function evalStateLabel(state: string): string {
  switch (state) {
    case 'firing': return '触发中'
    case 'normal': return '正常'
    case 'insufficient_data': return '数据不足'
    case 'error': return '错误'
    default: return '-'
  }
}

function evalStateType(state: string): string {
  switch (state) {
    case 'firing': return 'danger'
    case 'normal': return 'success'
    case 'insufficient_data': return 'warning'
    case 'error': return 'danger'
    default: return 'info'
  }
}

function metricLabel(m: string): string {
  return m === 'cpu' ? 'CPU' : '内存'
}

function operatorLabel(op: string): string {
  return op === 'gte' ? '>=' : '<='
}

function thresholdLabel(rule: AlertRule): string {
  if (rule.metric_name === 'cpu') return `${(rule.threshold / 1e9).toFixed(1)} 核心`
  return `${(rule.threshold / 1e9).toFixed(1)} GiB`
}

async function loadClusters() {
  try {
    const result = await listClusters(auth.accessToken!)
    clusters.value = result.items
    if (clusters.value.length > 0 && !selectedClusterID.value) {
      selectedClusterID.value = clusters.value[0].id
    }
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '加载集群失败'
  }
}

async function loadRules() {
  if (!selectedClusterID.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    rules.value = await listAlertRules(auth.accessToken!, selectedClusterID.value)
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '加载规则失败'
  } finally {
    loading.value = false
  }
}

async function loadInstances() {
  if (!selectedClusterID.value) return
  try {
    const filters: { state?: string; limit?: number } = { limit: 100 }
    if (stateFilter.value) filters.state = stateFilter.value
    instances.value = await listAlertInstances(auth.accessToken!, selectedClusterID.value, filters)
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '加载告警失败'
  }
}

async function loadOverview() {
  if (!selectedClusterID.value) return
  overviewLoading.value = true
  try {
    overview.value = await getAlertOverview(auth.accessToken!, selectedClusterID.value, { window_minutes: overviewWindowMinutes.value })
  } catch {
    overview.value = null
  } finally {
    overviewLoading.value = false
  }
}

function overviewSeverityType(group: AlertOverviewResponse['groups'][number]): 'danger' | 'success' {
  return group.firing_count > 0 ? 'danger' : 'success'
}

async function refresh() {
  await Promise.all([loadRules(), loadInstances(), loadOverview()])
}

async function handleCreate() {
  try {
    await createAlertRule(auth.accessToken!, selectedClusterID.value, newRule.value)
    showCreateDialog.value = false
    newRule.value = { display_name: '', resource_kind: 'Node', resource_name: '', metric_name: 'cpu', operator: 'gte', threshold: 3000000000, for_seconds: 300, minimum_points: 5 }
    await loadRules()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '创建规则失败'
  }
}

async function toggleEnabled(rule: AlertRule) {
  try {
    await patchAlertRule(auth.accessToken!, selectedClusterID.value, rule.id, { enabled: !rule.enabled })
    await loadRules()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '更新规则失败'
  }
}

async function handleDelete(rule: AlertRule) {
  try {
    await deleteAlertRule(auth.accessToken!, selectedClusterID.value, rule.id)
    await loadRules()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '删除规则失败'
  }
}

async function promoteToIncident(inst: AlertInstance) {
  errorMessage.value = ''
  noticeMessage.value = ''
  if (inst.state !== 'firing') {
    noticeMessage.value = '仅触发中的告警实例可创建事故工作区'
    return
  }
  try {
    const incident = await createIncident(auth.accessToken!, {
      source_type: 'alert',
      source_ref: `alert:${inst.id}`,
      cluster_id: selectedClusterID.value,
      title: '告警关联事故',
      severity: 'high',
      summary: `从告警实例 #${inst.id} 提升的事故工作区`,
      resource: { kind: 'Node', name: rules.value.find(r => r.id === inst.rule_id)?.resource_name ?? '' },
    })
    noticeMessage.value = `已创建事故工作区 ${incident.number}`
  } catch (err) {
    if (err instanceof APIError && err.code === 'SOURCE_ALREADY_USED') {
      noticeMessage.value = '该告警实例已存在关联的事故工作区'
    } else {
      errorMessage.value = err instanceof APIError ? err.message : '创建事故工作区失败'
    }
  }
}

watch(selectedClusterID, () => refresh())
watch(overviewWindowMinutes, () => loadOverview())
onMounted(() => loadClusters())
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="告警规则">
    <div class="alert-view">
      <header class="alert-header">
        <div class="alert-title">
          <Bell :size="24" />
          <div>
            <h2>告警规则</h2>
            <p class="alert-sub">节点级指标阈值告警 · 持续时间 + 最少采样点双约束 · 可提升为事故工作区</p>
          </div>
        </div>
        <div class="alert-actions">
          <select v-model.number="selectedClusterID" aria-label="选择集群">
            <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
          <select v-model="stateFilter" aria-label="告警状态" @change="loadInstances">
            <option value="">全部状态</option>
            <option value="firing">触发中</option>
            <option value="resolved">已恢复</option>
          </select>
          <button class="icon-button" type="button" title="刷新" aria-label="刷新" :disabled="loading" @click="refresh">
            <RefreshCw :size="16" />
          </button>
          <button v-if="canManage" class="primary-button" type="button" @click="showCreateDialog = true">
            <Plus :size="15" /> 创建规则
          </button>
        </div>
      </header>

      <p v-if="errorMessage" class="alert-banner is-error">{{ errorMessage }}</p>
      <p v-if="noticeMessage" class="alert-banner is-success">{{ noticeMessage }}</p>

      <section class="alert-panel">
        <div class="panel-heading">
          <h3>规则列表</h3>
          <span class="panel-count">{{ rules.length }} 条</span>
        </div>
        <div class="alert-table-scroll">
          <table class="compact-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>节点</th>
                <th>指标</th>
                <th>条件</th>
                <th>启用</th>
                <th>评估状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="rule in rules" :key="rule.id">
                <td><strong>{{ rule.display_name }}</strong></td>
                <td class="mono">{{ rule.resource_name }}</td>
                <td>{{ metricLabel(rule.metric_name) }}</td>
                <td>
                  <span class="mono">{{ operatorLabel(rule.operator) }} {{ thresholdLabel(rule) }}</span>
                  <span class="muted-extra">持续 {{ rule.for_seconds }}s · ≥{{ rule.minimum_points }} 点</span>
                </td>
                <td>
                  <span class="alert-badge" :class="rule.enabled ? 'is-success' : 'is-muted'">{{ rule.enabled ? '启用' : '禁用' }}</span>
                </td>
                <td>
                  <span v-if="rule.last_evaluation_state" class="alert-badge" :class="'is-' + evalStateType(rule.last_evaluation_state)">
                    {{ evalStateLabel(rule.last_evaluation_state) }}
                  </span>
                  <span v-else class="muted-extra">-</span>
                </td>
                <td>
                  <button
                    v-if="canManage"
                    class="icon-button compact"
                    type="button"
                    :title="rule.enabled ? '禁用' : '启用'"
                    :aria-label="rule.enabled ? '禁用' : '启用'"
                    @click="toggleEnabled(rule)"
                  >
                    <BellOff v-if="rule.enabled" :size="14" />
                    <Bell v-else :size="14" />
                  </button>
                  <button
                    v-if="canManage"
                    class="icon-button compact is-danger"
                    type="button"
                    title="删除"
                    aria-label="删除"
                    @click="handleDelete(rule)"
                  >
                    <Trash2 :size="14" />
                  </button>
                </td>
              </tr>
              <tr v-if="!loading && rules.length === 0">
                <td colspan="7">
                  <div class="resource-empty">
                    <Bell :size="28" />
                    <strong>暂无告警规则</strong>
                    <span>创建节点级 CPU / 内存阈值规则后，评估结果会显示在这一列。</span>
                  </div>
                </td>
              </tr>
              <tr v-if="loading">
                <td colspan="7" class="loading-row">加载中…</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="alert-panel">
        <div class="panel-heading">
          <div>
            <h3>告警降噪 · 规则维度聚合</h3>
            <p class="alert-sub">按规则折叠重复实例 · 有界窗口 · 关联 correlation 案例深链</p>
          </div>
          <select v-model.number="overviewWindowMinutes" aria-label="聚合窗口" @change="loadOverview">
            <option :value="60">1 小时</option>
            <option :value="360">6 小时</option>
            <option :value="1440">24 小时</option>
            <option :value="10080">7 天</option>
          </select>
        </div>

        <p v-if="overview?.fail_closed" class="alert-banner is-warning">{{ overview?.empty_note ?? '窗口内没有告警' }}</p>
        <template v-else-if="overview">
          <div class="noise-stats">
            <div class="noise-stat"><span>聚合组</span><strong>{{ overview.groups_total }}</strong></div>
            <div class="noise-stat"><span>触发中</span><strong class="is-danger-text">{{ overview.total_firing }}</strong></div>
            <div class="noise-stat"><span>已恢复</span><strong>{{ overview.total_resolved }}</strong></div>
            <div class="noise-stat">
              <span>关联案例</span>
              <strong>{{ overview.groups.reduce((acc, g) => acc + (g.related_case_ids?.length ?? 0), 0) }}</strong>
            </div>
          </div>
          <div class="alert-table-scroll">
            <table class="compact-table">
              <thead>
                <tr>
                  <th>规则</th>
                  <th>资源</th>
                  <th>触发 / 恢复</th>
                  <th>首次触发</th>
                  <th>最近触发</th>
                  <th>关联案例</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="group in overview.groups" :key="group.rule_id">
                  <td>
                    <strong>{{ group.display_name }}</strong>
                    <span class="muted-extra">{{ group.metric_name }}</span>
                  </td>
                  <td class="mono">{{ group.resource_kind }}/{{ group.resource_name }}</td>
                  <td>
                    <span class="alert-badge" :class="'is-' + overviewSeverityType(group)">{{ group.firing_count }}</span>
                    / {{ group.resolved_count }}
                  </td>
                  <td>{{ formatTime(group.first_fired_at) }}</td>
                  <td>{{ formatTime(group.last_fired_at) }}</td>
                  <td>
                    <template v-if="group.related_case_ids?.length">
                      <button
                        v-for="caseID in group.related_case_ids"
                        :key="caseID"
                        class="case-chip"
                        type="button"
                        @click="$router.push(`/aiops/correlation?case=${caseID}`)"
                      >
                        #{{ caseID }}
                      </button>
                    </template>
                    <span v-else class="muted-extra">-</span>
                  </td>
                </tr>
                <tr v-if="overview.groups.length === 0">
                  <td colspan="6"><div class="resource-empty"><span>窗口内无告警</span></div></td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
        <div v-else-if="!overviewLoading" class="resource-empty">
          <span>暂无聚合数据</span>
        </div>
      </section>

      <section class="alert-panel">
        <div class="panel-heading">
          <h3>告警实例</h3>
          <span class="panel-count">{{ instances.length }} 条</span>
        </div>
        <div class="alert-table-scroll">
          <table class="compact-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>规则</th>
                <th>关联诊断</th>
                <th>状态</th>
                <th>首次触发</th>
                <th>最近触发</th>
                <th>恢复时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="inst in instances" :key="inst.id">
                <td class="mono">#{{ inst.id }}</td>
                <td>{{ rules.find(r => r.id === inst.rule_id)?.display_name ?? `规则 ${inst.rule_id}` }}</td>
                <td>
                  <button class="link-button" type="button" @click="$router.push(`/diagnoses?detail=${inst.diagnosis_id}`)">
                    #{{ inst.diagnosis_id }}
                  </button>
                </td>
                <td>
                  <span class="alert-badge" :class="inst.state === 'firing' ? 'is-danger' : 'is-success'">
                    {{ inst.state === 'firing' ? '触发中' : '已恢复' }}
                  </span>
                </td>
                <td>{{ formatTime(inst.first_fired_at) }}</td>
                <td>{{ formatTime(inst.last_fired_at) }}</td>
                <td>{{ inst.resolved_at ? formatTime(inst.resolved_at) : '-' }}</td>
                <td>
                  <button
                    v-if="canManage && inst.state === 'firing'"
                    class="icon-button compact"
                    type="button"
                    title="创建事故工作区"
                    aria-label="创建事故工作区"
                    @click="promoteToIncident(inst)"
                  >
                    <FilePlus2 :size="14" />
                  </button>
                </td>
              </tr>
              <tr v-if="instances.length === 0">
                <td colspan="8">
                  <div class="resource-empty">
                    <span>当前筛选条件下没有告警实例</span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div v-if="showCreateDialog" class="alert-overlay" @click.self="showCreateDialog = false">
        <section class="alert-dialog">
          <header>
            <div>
              <p class="context-label">NEW ALERT RULE</p>
              <h2>创建告警规则</h2>
            </div>
            <button class="icon-button" type="button" aria-label="关闭" @click="showCreateDialog = false">×</button>
          </header>

          <label class="form-field">
            <span>规则名称</span>
            <input v-model="newRule.display_name" type="text" maxlength="128" placeholder="例：node01 CPU 持续高水位" />
          </label>
          <label class="form-field">
            <span>节点名称</span>
            <input v-model="newRule.resource_name" type="text" maxlength="253" placeholder="例：k8s-node01" />
          </label>
          <div class="form-row">
            <label class="form-field">
              <span>指标</span>
              <select v-model="newRule.metric_name">
                <option value="cpu">CPU</option>
                <option value="memory">内存</option>
              </select>
            </label>
            <label class="form-field">
              <span>运算符</span>
              <select v-model="newRule.operator">
                <option value="gte">&gt;=</option>
                <option value="lte">&lt;=</option>
              </select>
            </label>
          </div>
          <label class="form-field">
            <span>阈值（纳核 / 字节）</span>
            <input v-model.number="newRule.threshold" type="number" min="0" step="1000000000" />
            <small>CPU: nanocores（1 核 = 1,000,000,000）；内存: bytes（1 GiB = 1,073,741,824）</small>
          </label>
          <div class="form-row">
            <label class="form-field">
              <span>持续时间（秒）</span>
              <input v-model.number="newRule.for_seconds" type="number" min="60" max="21600" step="60" />
            </label>
            <label class="form-field">
              <span>最少采样点</span>
              <input v-model.number="newRule.minimum_points" type="number" min="2" max="360" step="1" />
            </label>
          </div>

          <footer>
            <button class="secondary-button" type="button" @click="showCreateDialog = false">取消</button>
            <button class="primary-button" type="button" :disabled="!newRule.display_name || !newRule.resource_name" @click="handleCreate">
              创建
            </button>
          </footer>
        </section>
      </div>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.alert-view {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.alert-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.alert-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.alert-title h2 {
  margin: 0;
  font-size: 18px;
  color: var(--text-primary);
}

.alert-sub {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.alert-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.alert-actions select,
.alert-panel select,
.form-field select {
  height: 34px;
  padding: 0 10px;
  font-size: 13px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--gray-300);
  border-radius: 6px;
}

.alert-panel {
  padding: 16px 18px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
}

.panel-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.panel-heading h3 {
  margin: 0;
  font-size: 14px;
  color: var(--text-primary);
}

.panel-count {
  font-size: 12px;
  color: var(--text-tertiary);
}

.alert-table-scroll {
  overflow-x: auto;
}

.alert-badge {
  display: inline-block;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 999px;
  white-space: nowrap;
}

.alert-badge.is-danger { color: var(--status-danger); background: var(--danger-bg); }
.alert-badge.is-success { color: var(--status-success-strong); background: var(--accent-subtle); }
.alert-badge.is-warning { color: var(--status-warning); background: var(--warning-bg); }
.alert-badge.is-info { color: #475467; background: var(--bg-primary); }
.alert-badge.is-muted { color: var(--text-tertiary); background: var(--bg-primary); }

.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.muted-extra { display: block; margin-top: 3px; font-size: 11px; color: var(--text-tertiary); }
.loading-row { text-align: center; color: var(--text-tertiary); font-size: 12px; }

.alert-banner {
  margin: 0;
  padding: 10px 14px;
  font-size: 13px;
  border-radius: 8px;
  border: 1px solid transparent;
}

.alert-banner.is-error { color: var(--status-danger); background: var(--danger-bg); border-color: #fecdca; }
.alert-banner.is-success { color: var(--status-success-strong); background: var(--accent-subtle); border-color: #abefc6; }
.alert-banner.is-warning { color: var(--status-warning); background: var(--warning-bg); border-color: #fedf89; }

.noise-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.noise-stat {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.noise-stat span { display: block; font-size: 11px; color: var(--text-tertiary); }
.noise-stat strong { display: block; margin-top: 4px; font-size: 18px; color: var(--text-primary); }
.is-danger-text { color: var(--status-danger); }

.case-chip {
  margin-right: 4px;
  padding: 1px 8px;
  font-size: 11px;
  color: var(--status-warning);
  background: var(--warning-bg);
  border: 1px solid #fedf89;
  border-radius: 999px;
  cursor: pointer;
}

.link-button {
  padding: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  color: var(--brand-600);
  background: none;
  border: 0;
  cursor: pointer;
  text-decoration: underline;
}

.icon-button.is-danger { color: var(--status-danger); }
.icon-button.compact { width: 26px; height: 26px; }

.alert-overlay {
  position: fixed;
  inset: 0;
  z-index: 60;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(17, 24, 28, 0.45);
}

.alert-dialog {
  width: min(520px, 100%);
  max-height: 86vh;
  overflow-y: auto;
  padding: 20px 22px;
  background: var(--bg-elevated);
  border-radius: 12px;
  box-shadow: 0 24px 60px rgba(16, 24, 40, 0.24);
}

.alert-dialog header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.alert-dialog h2 { margin: 4px 0 0; font-size: 17px; color: var(--text-primary); }

.form-field {
  display: block;
  margin-bottom: 14px;
}

.form-field > span {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-field input {
  width: 100%;
  height: 34px;
  padding: 0 10px;
  font-size: 13px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--gray-300);
  border-radius: 6px;
}

.form-field small { display: block; margin-top: 6px; font-size: 11px; color: var(--text-tertiary); }

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.alert-dialog footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 8px;
}
</style>
