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
      <div class="alert-header">
        <div class="alert-title">
          <Bell :size="24" />
          <h2>告警规则</h2>
        </div>
        <div class="alert-actions">
          <el-select v-model="selectedClusterID" placeholder="选择集群" style="width: 200px">
            <el-option v-for="c in clusters" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
          <el-select v-model="stateFilter" placeholder="告警状态" style="width: 130px" clearable @change="loadInstances">
            <el-option label="触发中" value="firing" />
            <el-option label="已恢复" value="resolved" />
          </el-select>
          <el-button :icon="RefreshCw" circle @click="refresh" :loading="loading" />
          <el-button v-if="canManage" type="primary" :icon="Plus" @click="showCreateDialog = true">创建规则</el-button>
        </div>
      </div>

      <el-alert v-if="errorMessage" :title="errorMessage" type="error" show-icon closable @close="errorMessage = ''" />
      <el-alert v-if="noticeMessage" :title="noticeMessage" type="success" show-icon closable style="margin-bottom: 12px" @close="noticeMessage = ''" />

      <h3>告警规则</h3>
      <el-table :data="rules" v-loading="loading" stripe empty-text="暂无告警规则">
        <el-table-column prop="display_name" label="名称" min-width="140" />
        <el-table-column prop="resource_name" label="节点" min-width="120" />
        <el-table-column label="指标" width="80">
          <template #default="{ row }">{{ metricLabel(row.metric_name) }}</template>
        </el-table-column>
        <el-table-column label="条件" width="180">
          <template #default="{ row }">{{ operatorLabel(row.operator) }} {{ thresholdLabel(row) }} 持续 {{ row.for_seconds }}s</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="评估状态" width="110">
          <template #default="{ row }">
            <el-tag v-if="row.last_evaluation_state" :type="evalStateType(row.last_evaluation_state)" size="small">{{ evalStateLabel(row.last_evaluation_state) }}</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button v-if="canManage" :icon="row.enabled ? BellOff : Bell" circle size="small" @click="toggleEnabled(row)" :title="row.enabled ? '禁用' : '启用'" />
            <el-button v-if="canManage" :icon="Trash2" circle size="small" type="danger" @click="handleDelete(row)" title="删除" />
          </template>
        </el-table-column>
      </el-table>

      <div class="noise-panel" v-loading="overviewLoading">
        <div class="noise-panel-heading">
          <div>
            <h3>告警降噪 · 规则维度聚合</h3>
            <span class="noise-panel-sub">按规则折叠重复实例 · 有界窗口 · 关联 correlation 案例深链</span>
          </div>
          <div class="noise-panel-controls">
            <el-select v-model="overviewWindowMinutes" style="width: 120px" @change="loadOverview">
              <el-option label="1 小时" :value="60" />
              <el-option label="6 小时" :value="360" />
              <el-option label="24 小时" :value="1440" />
              <el-option label="7 天" :value="10080" />
            </el-select>
          </div>
        </div>

        <el-alert v-if="overview?.fail_closed" :title="overview?.empty_note ?? '窗口内没有告警'" type="warning" show-icon :closable="false" style="margin: 12px 0" />
        <template v-else-if="overview">
          <div class="noise-stats">
            <div class="noise-stat"><span>聚合组</span><strong>{{ overview.groups_total }}</strong></div>
            <div class="noise-stat"><span>触发中</span><strong class="danger-text">{{ overview.total_firing }}</strong></div>
            <div class="noise-stat"><span>已恢复</span><strong>{{ overview.total_resolved }}</strong></div>
            <div class="noise-stat"><span>关联案例</span><strong>{{ overview.groups.reduce((acc, g) => acc + (g.related_case_ids?.length ?? 0), 0) }}</strong></div>
          </div>
          <el-table :data="overview.groups" stripe size="small" empty-text="窗口内无告警">
            <el-table-column label="规则" min-width="150">
              <template #default="{ row }"><strong>{{ row.display_name }}</strong><span class="noise-muted"> · {{ row.metric_name }}</span></template>
            </el-table-column>
            <el-table-column label="资源" min-width="130">
              <template #default="{ row }">{{ row.resource_kind }}/{{ row.resource_name }}</template>
            </el-table-column>
            <el-table-column label="触发/恢复" width="110">
              <template #default="{ row }"><el-tag :type="overviewSeverityType(row)" size="small">{{ row.firing_count }}</el-tag> / {{ row.resolved_count }}</template>
            </el-table-column>
            <el-table-column label="首次触发" width="160">
              <template #default="{ row }">{{ formatTime(row.first_fired_at) }}</template>
            </el-table-column>
            <el-table-column label="最近触发" width="160">
              <template #default="{ row }">{{ formatTime(row.last_fired_at) }}</template>
            </el-table-column>
            <el-table-column label="关联案例" width="130">
              <template #default="{ row }">
                <template v-if="row.related_case_ids?.length">
                  <el-tag v-for="caseID in row.related_case_ids" :key="caseID" type="warning" size="small" style="margin-right: 4px">
                    <a :href="`/correlation?case=${caseID}`" @click.prevent="$router.push(`/correlation?case=${caseID}`)">#{{ caseID }}</a>
                  </el-tag>
                </template>
                <span v-else class="noise-muted">-</span>
              </template>
            </el-table-column>
          </el-table>
        </template>
        <el-empty v-else-if="!overviewLoading" description="暂无聚合数据" :image-size="60" />
      </div>

      <h3 style="margin-top: 24px">告警实例</h3>
      <el-table :data="instances" stripe empty-text="暂无告警实例">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="规则" min-width="120">
          <template #default="{ row }">
            {{ rules.find(r => r.id === row.rule_id)?.display_name ?? `规则 ${row.rule_id}` }}
          </template>
        </el-table-column>
        <el-table-column label="关联诊断" width="100">
          <template #default="{ row }">
            <a :href="`/diagnoses?detail=${row.diagnosis_id}`" @click.prevent="$router.push(`/diagnoses?detail=${row.diagnosis_id}`)">#{{ row.diagnosis_id }}</a>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.state === 'firing' ? 'danger' : 'success'" size="small">{{ row.state === 'firing' ? '触发中' : '已恢复' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="首次触发" width="160">
          <template #default="{ row }">{{ formatTime(row.first_fired_at) }}</template>
        </el-table-column>
        <el-table-column label="最近触发" width="160">
          <template #default="{ row }">{{ formatTime(row.last_fired_at) }}</template>
        </el-table-column>
        <el-table-column label="恢复时间" width="160">
          <template #default="{ row }">{{ row.resolved_at ? formatTime(row.resolved_at) : '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button v-if="canManage && row.state === 'firing'" :icon="FilePlus2" circle size="small" type="primary" @click="promoteToIncident(row)" title="创建事故工作区" />
          </template>
        </el-table-column>
      </el-table>

      <el-dialog v-model="showCreateDialog" title="创建告警规则" width="520px" destroy-on-close>
        <el-form label-width="100px" :model="newRule">
          <el-form-item label="规则名称" required>
            <el-input v-model="newRule.display_name" placeholder="例: High CPU Alert" maxlength="128" />
          </el-form-item>
          <el-form-item label="节点名称" required>
            <el-input v-model="newRule.resource_name" placeholder="例: worker-01" maxlength="253" />
          </el-form-item>
          <el-form-item label="指标" required>
            <el-select v-model="newRule.metric_name" style="width: 100%">
              <el-option label="CPU" value="cpu" />
              <el-option label="内存" value="memory" />
            </el-select>
          </el-form-item>
          <el-form-item label="运算符" required>
            <el-select v-model="newRule.operator" style="width: 100%">
              <el-option label=">=" value="gte" />
              <el-option label="<=" value="lte" />
            </el-select>
          </el-form-item>
          <el-form-item label="阈值" required>
            <el-input-number v-model="newRule.threshold" :min="0" :step="1000000000" style="width: 100%" />
            <div class="form-hint">CPU: nanocores (1核心 = 1,000,000,000); 内存: bytes (1GiB = 1,073,741,824)</div>
          </el-form-item>
          <el-form-item label="持续时间(秒)" required>
            <el-input-number v-model="newRule.for_seconds" :min="60" :max="21600" :step="60" style="width: 100%" />
          </el-form-item>
          <el-form-item label="最少采样点" required>
            <el-input-number v-model="newRule.minimum_points" :min="2" :max="360" :step="1" style="width: 100%" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="showCreateDialog = false">取消</el-button>
          <el-button type="primary" @click="handleCreate" :disabled="!newRule.display_name || !newRule.resource_name">创建</el-button>
        </template>
      </el-dialog>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.alert-view {
  padding: 24px;
}
.alert-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  flex-wrap: wrap;
  gap: 12px;
}
.alert-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.alert-title h2 {
  margin: 0;
  font-size: 20px;
}
.alert-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
h3 {
  margin: 0 0 12px 0;
  font-size: 16px;
  color: var(--el-text-color-primary);
}
.form-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}
</style>
