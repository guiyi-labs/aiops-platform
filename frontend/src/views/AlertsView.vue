<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Bell, BellOff, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { createAlertRule, deleteAlertRule, listAlertInstances, listAlertRules, patchAlertRule } from '../api/alert'
import { APIError } from '../api/auth'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { AlertRule, AlertRuleCreate, AlertInstance, AlertInstanceState } from '../types/alert'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref(0)
const rules = ref<AlertRule[]>([])
const instances = ref<AlertInstance[]>([])
const loading = ref(false)
const errorMessage = ref('')
const showCreateDialog = ref(false)
const stateFilter = ref<AlertInstanceState | ''>('firing')

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

async function refresh() {
  await Promise.all([loadRules(), loadInstances()])
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

watch(selectedClusterID, () => refresh())
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
