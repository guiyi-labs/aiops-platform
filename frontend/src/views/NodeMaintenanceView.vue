<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  Hammer,
  ListTree,
  RefreshCw,
  Server,
  ShieldAlert,
  Wrench,
} from 'lucide-vue-next'

import * as k8sAPI from '../api/kubernetes'
import * as clusterAPI from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { MaintenanceAction, MaintenancePlan } from '../types/kubernetes'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const plans = ref<MaintenancePlan[]>([])
const loading = ref(true)
const errorMessage = ref('')

// Preview form state
const action = ref<MaintenanceAction>('cordon')
const nodeName = ref('')
const previewing = ref(false)
const previewError = ref('')
const currentPlan = ref<MaintenancePlan | null>(null)

// Execute state
const executing = ref(false)
const executeError = ref('')
const executeResult = ref<MaintenancePlan | null>(null)

const actionLabels: Record<MaintenanceAction, string> = {
  cordon: 'Cordon',
  uncordon: 'Uncordon',
  drain: 'Drain',
}

const actionIcons: Record<MaintenanceAction, typeof Hammer> = {
  cordon: Hammer,
  uncordon: Wrench,
  drain: ListTree,
}

const statusLabels: Record<string, string> = {
  awaiting_confirmation: '待确认',
  executing: '执行中',
  succeeded: '成功',
  failed: '失败',
  expired: '已过期',
}

const statusClasses: Record<string, string> = {
  awaiting_confirmation: 'pending',
  executing: 'pending',
  succeeded: 'running',
  failed: 'failed',
  expired: 'unknown',
}

const classLabels: Record<string, string> = {
  retained: '保留',
  evictable: '可驱逐',
  blocking: '阻断',
}

const classClasses: Record<string, string> = {
  retained: 'running',
  evictable: 'pending',
  blocking: 'failed',
}

const filteredPlans = computed(() => plans.value.slice(0, 30))

function phaseLabelClass(status: string): string {
  return statusClasses[status] ?? 'unknown'
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

function genIdempotencyKey(): string {
  const ts = Date.now().toString(36)
  const rand = Math.random().toString(36).slice(2, 10)
  return `m30-${ts}-${rand}`
}

async function initialize() {
  loading.value = true
  errorMessage.value = ''
  try {
    const list = await clusterAPI.listClusters(auth.accessToken)
    clusters.value = list.items.filter((c) => c.enabled)
    if (clusters.value.length > 0) selectedClusterID.value = clusters.value[0].id
  } catch {
    errorMessage.value = '无法加载集群列表'
  } finally {
    loading.value = false
  }
}

async function loadPlans() {
  if (!selectedClusterID.value) {
    plans.value = []
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const resp = await k8sAPI.listMaintenancePlans(auth.accessToken, selectedClusterID.value)
    plans.value = resp.items ?? []
  } catch (error) {
    plans.value = []
    errorMessage.value = error instanceof Error ? error.message : '无法加载维护计划'
  } finally {
    loading.value = false
  }
}

async function submitPreview() {
  if (!selectedClusterID.value || !nodeName.value) return
  previewing.value = true
  previewError.value = ''
  currentPlan.value = null
  executeResult.value = null
  try {
    currentPlan.value = await k8sAPI.previewMaintenancePlan(
      auth.accessToken,
      selectedClusterID.value,
      action.value,
      nodeName.value,
    )
  } catch (error) {
    previewError.value = error instanceof Error ? error.message : '预览失败'
  } finally {
    previewing.value = false
  }
}

async function submitExecute() {
  if (!currentPlan.value?.confirmation_token) return
  executing.value = true
  executeError.value = ''
  executeResult.value = null
  try {
    executeResult.value = await k8sAPI.executeMaintenancePlan(
      auth.accessToken,
      currentPlan.value.id,
      currentPlan.value.confirmation_token,
      genIdempotencyKey(),
    )
    currentPlan.value = null
    void loadPlans()
  } catch (error) {
    executeError.value = error instanceof Error ? error.message : '执行失败'
  } finally {
    executing.value = false
  }
}

function cancelPreview() {
  currentPlan.value = null
  previewError.value = ''
  executeError.value = ''
  executeResult.value = null
}

function selectPlan(plan: MaintenancePlan) {
  executeResult.value = plan
  currentPlan.value = null
}

watch(selectedClusterID, () => {
  currentPlan.value = null
  executeResult.value = null
  void loadPlans()
})
onMounted(() => void initialize().then(() => void loadPlans()))
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="节点维护">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="loading || !selectedClusterID" @click="loadPlans">
        <RefreshCw :size="16" :class="{ spin: loading }" />
        <span>刷新</span>
      </button>
      <select v-model="selectedClusterID" class="cluster-select" aria-label="选择集群" :disabled="clusters.length === 0">
        <option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option>
      </select>
    </template>

    <div class="two-column">
      <!-- Left: Preview form + plan history -->
      <section class="panel">
        <header class="panel-header">
          <div class="panel-title">
            <Server :size="18" />
            <strong>维护操作</strong>
          </div>
        </header>

        <div class="preview-form">
          <label class="form-label">
            <span>操作类型</span>
            <select v-model="action" class="compact-input" :disabled="previewing || executing">
              <option value="cordon">Cordon（标记不可调度）</option>
              <option value="uncordon">Uncordon（恢复调度）</option>
              <option value="drain">Drain（驱逐并cordon）</option>
            </select>
          </label>
          <label class="form-label">
            <span>节点名称</span>
            <input
              v-model="nodeName"
              type="text"
              class="compact-input"
              placeholder="worker-node-name"
              :disabled="previewing || executing"
              @keyup.enter="submitPreview"
            />
          </label>
          <button
            type="button"
            class="primary-button"
            :disabled="!nodeName || !selectedClusterID || previewing"
            @click="submitPreview"
          >
            <component :is="actionIcons[action]" :size="16" />
            <span>{{ previewing ? '预览中…' : `预览 ${actionLabels[action]}` }}</span>
          </button>
          <p v-if="previewError" class="error-text">{{ previewError }}</p>
        </div>

        <div class="plan-history">
          <h4>历史计划</h4>
          <div v-if="loading" class="panel-empty">加载中…</div>
          <div v-else-if="errorMessage" class="panel-empty error">{{ errorMessage }}</div>
          <div v-else-if="filteredPlans.length === 0" class="panel-empty muted">暂无维护计划</div>
          <div v-else class="plan-list">
            <button
              v-for="plan in filteredPlans"
              :key="plan.id"
              type="button"
              class="plan-row"
              @click="selectPlan(plan)"
            >
              <div class="plan-row-main">
                <strong>{{ actionLabels[plan.action] }}</strong>
                <span class="mono small">{{ plan.node_name }}</span>
              </div>
              <span :class="['phase-badge', phaseLabelClass(plan.status)]">{{ statusLabels[plan.status] }}</span>
            </button>
          </div>
        </div>
      </section>

      <!-- Right: Preview detail / Execution result -->
      <section class="panel detail-panel">
        <header class="panel-header sticky">
          <div class="panel-title">
            <ShieldAlert :size="18" />
            <strong>{{ executeResult ? '执行结果' : (currentPlan ? '预览确认' : '维护详情') }}</strong>
          </div>
        </header>

        <!-- Empty state -->
        <div v-if="!currentPlan && !executeResult && !executeError" class="panel-empty muted">
          选择左侧操作类型和节点，点击预览开始
        </div>

        <!-- Execute error -->
        <div v-else-if="executeError && !executeResult" class="panel-empty error">
          {{ executeError }}
        </div>

        <!-- Execution result -->
        <div v-else-if="executeResult" class="result-detail">
          <div class="result-meta">
            <div class="meta-row">
              <span class="muted">操作</span>
              <strong>{{ actionLabels[executeResult.action] }}</strong>
            </div>
            <div class="meta-row">
              <span class="muted">节点</span>
              <span class="mono">{{ executeResult.node_name }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">状态</span>
              <span :class="['phase-badge', phaseLabelClass(executeResult.status)]">{{ statusLabels[executeResult.status] }}</span>
            </div>
            <div class="meta-row" v-if="executeResult.executed_at">
              <span class="muted">执行时间</span>
              <span>{{ formatTimestamp(executeResult.executed_at) }}</span>
            </div>
            <div class="meta-row" v-if="executeResult.last_error">
              <span class="muted">错误</span>
              <span class="error-text">{{ executeResult.last_error }}</span>
            </div>
          </div>

          <section v-if="executeResult.execution_result" class="posture-section">
            <div class="section-head">
              <CheckCircle2 :size="16" />
              <h3>执行结果</h3>
            </div>
            <div class="summary-grid">
              <div class="summary-card">
                <p class="muted">Node Patched</p>
                <strong>{{ executeResult.execution_result.node_patched ? '是' : '否' }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">不可调度</p>
                <strong>{{ executeResult.execution_result.unschedulable_now ? '是' : '否' }}</strong>
              </div>
              <div class="summary-card" v-if="executeResult.action === 'drain'">
                <p class="muted">已驱逐</p>
                <strong>{{ executeResult.execution_result.evicted_count }}</strong>
              </div>
              <div class="summary-card" v-if="executeResult.action === 'drain'">
                <p class="muted">失败</p>
                <strong :class="{ 'error-text': executeResult.execution_result.failed_count > 0 }">
                  {{ executeResult.execution_result.failed_count }}
                </strong>
              </div>
            </div>
            <div v-if="executeResult.execution_result.partial" class="warning-banner">
              <AlertTriangle :size="16" />
              <span>部分驱逐失败，节点保持 cordoned 状态</span>
            </div>
            <table v-if="executeResult.execution_result.pod_outcomes?.length" class="compact-table">
              <thead><tr><th>Pod</th><th>命名空间</th><th>结果</th><th>详情</th></tr></thead>
              <tbody>
                <tr v-for="outcome in executeResult.execution_result.pod_outcomes" :key="`${outcome.namespace}-${outcome.name}`">
                  <td class="mono">{{ outcome.name }}</td>
                  <td class="mono">{{ outcome.namespace }}</td>
                  <td>
                    <span :class="['phase-badge', outcome.outcome === 'evicted' ? 'running' : 'failed']">{{ outcome.outcome }}</span>
                  </td>
                  <td class="small">{{ outcome.detail ?? '—' }}</td>
                </tr>
              </tbody>
            </table>
          </section>
        </div>

        <!-- Preview confirmation -->
        <div v-else-if="currentPlan" class="preview-detail">
          <div class="warning-banner" v-if="currentPlan.action === 'drain'">
            <AlertTriangle :size="16" />
            <span>Drain 是破坏性操作，将驱逐符合条件的 Pod。请仔细审查以下预览。</span>
          </div>

          <div class="result-meta">
            <div class="meta-row">
              <span class="muted">操作</span>
              <strong>{{ actionLabels[currentPlan.action] }}</strong>
            </div>
            <div class="meta-row">
              <span class="muted">节点</span>
              <span class="mono">{{ currentPlan.node_name }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">当前不可调度</span>
              <strong>{{ currentPlan.preview_evidence.node_unschedulable ? '是' : '否' }}</strong>
            </div>
            <div class="meta-row">
              <span class="muted">过期时间</span>
              <span>{{ formatTimestamp(currentPlan.expires_at) }}</span>
            </div>
          </div>

          <section v-if="currentPlan.action === 'drain'" class="posture-section">
            <div class="section-head">
              <ListTree :size="16" />
              <h3>Pod 分类</h3>
            </div>
            <div class="summary-grid">
              <div class="summary-card">
                <p class="muted">保留（DaemonSet/mirror）</p>
                <strong>{{ currentPlan.preview_evidence.retained_count }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">可驱逐</p>
                <strong>{{ currentPlan.preview_evidence.evictable_count }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">阻断</p>
                <strong :class="{ 'error-text': currentPlan.preview_evidence.blocking_count > 0 }">
                  {{ currentPlan.preview_evidence.blocking_count }}
                </strong>
              </div>
            </div>
            <table v-if="currentPlan.preview_evidence.pods.length" class="compact-table">
              <thead><tr><th>Pod</th><th>命名空间</th><th>Owner</th><th>分类</th><th>emptyDir</th><th>PDB</th></tr></thead>
              <tbody>
                <tr v-for="pod in currentPlan.preview_evidence.pods" :key="`${pod.namespace}-${pod.name}`">
                  <td class="mono">{{ pod.name }}</td>
                  <td class="mono">{{ pod.namespace }}</td>
                  <td>{{ pod.owner_kind || '—' }}</td>
                  <td><span :class="['phase-badge', classClasses[pod.classification]]">{{ classLabels[pod.classification] }}</span></td>
                  <td>{{ pod.has_empty_dir ? '是' : '否' }}</td>
                  <td class="small">{{ pod.pdb_name ?? '—' }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <div class="confirm-section">
            <p class="muted small">
              确认令牌仅显示一次。执行需要此令牌和幂等键。
            </p>
            <div class="confirm-token">
              <Clock :size="14" />
              <code>{{ currentPlan.confirmation_token }}</code>
            </div>
            <div class="confirm-actions">
              <button type="button" class="secondary-button" :disabled="executing" @click="cancelPreview">取消</button>
              <button
                type="button"
                class="danger-button"
                :disabled="executing"
                @click="submitExecute"
              >
                <ShieldAlert :size="16" />
                <span>{{ executing ? '执行中…' : `确认执行 ${actionLabels[currentPlan.action]}` }}</span>
              </button>
            </div>
            <p v-if="executeError" class="error-text">{{ executeError }}</p>
          </div>
        </div>
      </section>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.preview-form { display: flex; flex-direction: column; gap: 12px; padding: 14px; border-bottom: 1px solid var(--border-soft); }
.form-label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-muted); }
.form-label select, .form-label input { font-size: 14px; }

.plan-history { padding: 14px; }
.plan-history h4 { font-size: 12px; color: var(--text-muted); margin: 0 0 8px; }
.plan-list { display: flex; flex-direction: column; gap: 6px; }
.plan-row {
  display: flex; align-items: center; justify-content: space-between;
  width: 100%; text-align: left; padding: 8px 12px;
  background: var(--surface-2); border: 1px solid var(--border-soft);
  border-radius: 8px; cursor: pointer; transition: background 0.15s;
}
.plan-row:hover { background: var(--surface-3); }
.plan-row-main { display: flex; align-items: center; gap: 8px; }

.result-detail, .preview-detail { display: flex; flex-direction: column; gap: 18px; padding: 8px 4px; }
.result-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; padding: 12px; background: var(--surface-2); border-radius: 10px; }
.meta-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }

.posture-section { border: 1px solid var(--border-soft); border-radius: 10px; padding: 14px; display: flex; flex-direction: column; gap: 12px; }
.section-head { display: flex; align-items: center; gap: 8px; }
.section-head h3 { font-size: 14px; margin: 0; }

.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.summary-card { padding: 10px 12px; background: var(--surface-2); border-radius: 8px; }
.summary-card p { margin: 0 0 4px; font-size: 12px; }

.warning-banner {
  display: flex; align-items: center; gap: 8px; padding: 10px 12px;
  background: rgba(234,179,8,0.12); border: 1px solid rgba(234,179,8,0.3);
  border-radius: 8px; color: #eab308; font-size: 13px;
}

.confirm-section { display: flex; flex-direction: column; gap: 12px; padding: 14px; border: 1px solid var(--border-soft); border-radius: 10px; }
.confirm-token {
  display: flex; align-items: center; gap: 6px; padding: 8px 12px;
  background: var(--surface-2); border-radius: 6px; font-family: var(--font-mono);
  font-size: 12px; word-break: break-all;
}
.confirm-actions { display: flex; gap: 8px; }

.danger-button {
  display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px;
  background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.3);
  color: #ef4444; border-radius: 8px; cursor: pointer; font-size: 14px;
  transition: background 0.15s;
}
.danger-button:hover:not(:disabled) { background: rgba(239,68,68,0.2); }
.danger-button:disabled { opacity: 0.5; cursor: not-allowed; }

.error-text { color: #ef4444; font-size: 13px; }
.mono { font-family: var(--font-mono); }
.small { font-size: 12px; }
</style>
