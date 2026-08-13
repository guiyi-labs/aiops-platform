<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  DatabaseBackup,
  LifeBuoy,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
} from 'lucide-vue-next'

import * as k8sAPI from '../api/kubernetes'
import * as clusterAPI from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { RestorePlan } from '../types/kubernetes'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const plans = ref<RestorePlan[]>([])
const loading = ref(true)
const errorMessage = ref('')

// Preview form state
const sourceBackupName = ref('')
const sourceBackupNamespace = ref('')
const previewing = ref(false)
const previewError = ref('')
const currentPlan = ref<RestorePlan | null>(null)

// Execute state
const executing = ref(false)
const executeError = ref('')
const executeResult = ref<RestorePlan | null>(null)

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

const filteredPlans = computed(() => plans.value.slice(0, 30))

function phaseLabelClass(status: string): string {
  return statusClasses[status] ?? 'unknown'
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

function genIdempotencyKey(): string {
  return crypto.randomUUID()
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
    const resp = await k8sAPI.listRestorePlans(auth.accessToken, selectedClusterID.value)
    plans.value = resp.items ?? []
  } catch (error) {
    plans.value = []
    errorMessage.value = error instanceof Error ? error.message : '无法加载恢复演练计划'
  } finally {
    loading.value = false
  }
}

async function submitPreview() {
  if (!selectedClusterID.value || !sourceBackupName.value || !sourceBackupNamespace.value) return
  previewing.value = true
  previewError.value = ''
  currentPlan.value = null
  executeResult.value = null
  try {
    currentPlan.value = await k8sAPI.previewRestorePlan(auth.accessToken, selectedClusterID.value, {
      source_backup_name: sourceBackupName.value,
      source_backup_namespace: sourceBackupNamespace.value,
    })
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
    executeResult.value = await k8sAPI.executeRestorePlan(
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

function selectPlan(plan: RestorePlan) {
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
  <ConsoleLayout eyebrow="分析与治理" title="恢复演练">
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
            <LifeBuoy :size="18" />
            <strong>恢复演练</strong>
          </div>
        </header>

        <div class="preview-form">
          <p class="muted small form-intro">
            从已完成的 M28 兼容备份恢复工作负载到隔离命名空间，验证备份可用性。目标命名空间由系统生成，并施加默认拒绝网络策略与零 Pod 配额隔离。
          </p>
          <label class="form-label">
            <span>源备份命名空间</span>
            <input
              v-model="sourceBackupNamespace"
              type="text"
              class="compact-input"
              placeholder="velero"
              :disabled="previewing || executing"
            />
          </label>
          <label class="form-label">
            <span>源备份名称</span>
            <input
              v-model="sourceBackupName"
              type="text"
              class="compact-input"
              placeholder="weekly-backup-20260730"
              :disabled="previewing || executing"
              @keyup.enter="submitPreview"
            />
          </label>
          <button
            type="button"
            class="primary-button"
            :disabled="!sourceBackupName || !sourceBackupNamespace || !selectedClusterID || previewing"
            @click="submitPreview"
          >
            <DatabaseBackup :size="16" />
            <span>{{ previewing ? '预览中…' : '预览恢复演练' }}</span>
          </button>
          <p v-if="previewError" class="error-text">{{ previewError }}</p>
        </div>

        <div class="plan-history">
          <h4>历史计划</h4>
          <div v-if="loading" class="panel-empty">加载中…</div>
          <div v-else-if="errorMessage" class="panel-empty error">{{ errorMessage }}</div>
          <div v-else-if="filteredPlans.length === 0" class="panel-empty muted">暂无恢复演练计划</div>
          <div v-else class="plan-list">
            <button
              v-for="plan in filteredPlans"
              :key="plan.id"
              type="button"
              class="plan-row"
              @click="selectPlan(plan)"
            >
              <div class="plan-row-main">
                <strong class="small">{{ plan.source_backup_name }}</strong>
                <span class="mono small">{{ plan.destination_namespace }}</span>
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
            <strong>{{ executeResult ? '执行结果' : (currentPlan ? '预览确认' : '演练详情') }}</strong>
          </div>
        </header>

        <!-- Empty state -->
        <div v-if="!currentPlan && !executeResult && !executeError" class="panel-empty muted">
          输入源备份信息，点击预览开始隔离恢复演练
        </div>

        <!-- Execute error -->
        <div v-else-if="executeError && !executeResult" class="panel-empty error">
          {{ executeError }}
        </div>

        <!-- Execution result -->
        <div v-else-if="executeResult" class="result-detail">
          <div class="result-meta">
            <div class="meta-row">
              <span class="muted">源备份</span>
              <span class="mono">{{ executeResult.source_backup_name }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">目标命名空间</span>
              <span class="mono">{{ executeResult.destination_namespace }}</span>
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
                <p class="muted">Restore 创建</p>
                <strong>{{ executeResult.execution_result.restore_created ? '是' : '否' }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">Restore 阶段</p>
                <strong>{{ executeResult.execution_result.restore_phase || '—' }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">隔离已建立</p>
                <strong>{{ executeResult.execution_result.quarantine_established ? '是' : '否' }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">恢复资源数</p>
                <strong>{{ executeResult.execution_result.restored_item_count }}</strong>
              </div>
            </div>
            <div v-if="executeResult.execution_result.partial" class="warning-banner">
              <AlertTriangle :size="16" />
              <span>部分恢复失败，隔离命名空间已保留以便排查</span>
            </div>
            <div v-if="executeResult.execution_result.failure_reason" class="warning-banner">
              <AlertTriangle :size="16" />
              <span>{{ executeResult.execution_result.failure_reason }}</span>
            </div>
            <table v-if="executeResult.execution_result.restored_items?.length" class="compact-table">
              <thead><tr><th>Kind</th><th>名称</th><th>命名空间</th></tr></thead>
              <tbody>
                <tr v-for="(item, idx) in executeResult.execution_result.restored_items" :key="`${item.kind}-${item.name}-${idx}`">
                  <td>{{ item.kind }}</td>
                  <td class="mono">{{ item.name }}</td>
                  <td class="mono">{{ item.namespace }}</td>
                </tr>
              </tbody>
            </table>
            <p v-if="executeResult.execution_result.truncated_items" class="muted small">
              恢复资源数超过上限，列表已截断。
            </p>
          </section>
        </div>

        <!-- Preview confirmation -->
        <div v-else-if="currentPlan" class="preview-detail">
          <div class="warning-banner">
            <AlertTriangle :size="16" />
            <span>恢复演练将在隔离命名空间中创建资源。目标命名空间施加默认拒绝网络策略与零 Pod 配额，演练后应清理。</span>
          </div>

          <div class="result-meta">
            <div class="meta-row">
              <span class="muted">源备份</span>
              <span class="mono">{{ currentPlan.source_backup_name }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">源备份命名空间</span>
              <span class="mono">{{ currentPlan.source_backup_namespace }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">源备份阶段</span>
              <strong>{{ currentPlan.source_backup_phase }}</strong>
            </div>
            <div class="meta-row">
              <span class="muted">目标命名空间</span>
              <span class="mono">{{ currentPlan.destination_namespace }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">Velero Restore</span>
              <span class="mono">{{ currentPlan.velero_restore_name }}</span>
            </div>
            <div class="meta-row">
              <span class="muted">过期时间</span>
              <span>{{ formatTimestamp(currentPlan.expires_at) }}</span>
            </div>
          </div>

          <section class="posture-section">
            <div class="section-head">
              <ShieldCheck :size="16" />
              <h3>隔离控制（预检已验证）</h3>
            </div>
            <div class="summary-grid">
              <div class="summary-card">
                <p class="muted">网络策略</p>
                <strong>{{ currentPlan.quarantine_status.network_policy_name }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">资源配额</p>
                <strong>{{ currentPlan.quarantine_status.resource_quota_name }}</strong>
              </div>
              <div class="summary-card">
                <p class="muted">预检 Dry-Run</p>
                <strong>{{ currentPlan.quarantine_status.dry_run_validated ? '通过' : '未验证' }}</strong>
              </div>
            </div>
          </section>

          <section class="posture-section">
            <div class="section-head">
              <DatabaseBackup :size="16" />
              <h3>恢复范围</h3>
            </div>
            <div class="scope-block">
              <p class="muted small">允许恢复的资源类型</p>
              <div class="kind-tags">
                <span v-for="kind in currentPlan.allowed_kinds" :key="kind" class="kind-tag allowed">{{ kind }}</span>
              </div>
            </div>
            <div class="scope-block">
              <p class="muted small">明确排除的资源类型</p>
              <div class="kind-tags">
                <span v-for="kind in currentPlan.excluded_kinds" :key="kind" class="kind-tag excluded">{{ kind }}</span>
              </div>
            </div>
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
                <span>{{ executing ? '执行中…' : '确认执行恢复演练' }}</span>
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
.form-intro { line-height: 1.5; }
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
.plan-row-main { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }

.result-detail, .preview-detail { display: flex; flex-direction: column; gap: 18px; padding: 8px 4px; }
.result-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; padding: 12px; background: var(--surface-2); border-radius: 10px; }
.meta-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }

.posture-section { border: 1px solid var(--border-soft); border-radius: 10px; padding: 14px; display: flex; flex-direction: column; gap: 12px; }
.section-head { display: flex; align-items: center; gap: 8px; }
.section-head h3 { font-size: 14px; margin: 0; }

.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.summary-card { padding: 10px 12px; background: var(--surface-2); border-radius: 8px; }
.summary-card p { margin: 0 0 4px; font-size: 12px; }

.scope-block { display: flex; flex-direction: column; gap: 6px; }
.kind-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.kind-tag { font-size: 12px; padding: 2px 8px; border-radius: 12px; border: 1px solid var(--border-soft); font-family: var(--font-mono); }
.kind-tag.allowed { background: rgba(34,197,94,0.1); border-color: rgba(34,197,94,0.3); color: #22c55e; }
.kind-tag.excluded { background: rgba(239,68,68,0.08); border-color: rgba(239,68,68,0.25); color: #ef4444; }

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
