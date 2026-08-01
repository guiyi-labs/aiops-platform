<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowRight, Boxes, Copy, Eye, History, Plus, RefreshCw, Send, XCircle } from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import { previewCopyPlan, listCopyPlansByCluster, executeCopyPlan, listMyCopyPlans } from '../api/copyops'
import { listClusters } from '../api/clusters'
import type { CopyOpsPlan, BundleItemRequest } from '../types/copyops'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])

const sourceClusterId = ref<number | null>(null)
const targetClusterId = ref<number | null>(null)
const targetNamespace = ref('default')

const items = ref<BundleItemRequest[]>([])
const newItemKind = ref<string>('Deployment')
const newItemNamespace = ref('')
const newItemName = ref('')

const plan = ref<CopyOpsPlan | null>(null)
const previewing = ref(false)
const executing = ref(false)

const historyMode = ref<'mine' | 'cluster'>('mine')
const historyClusterId = ref<number | null>(null)
const historyPlans = ref<CopyOpsPlan[]>([])
const loadingHistory = ref(false)
const loadingClusters = ref(true)

const errorMessage = ref('')
const notice = ref('')

const allowedKinds = ['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Service', 'Ingress', 'ServiceAccount', 'ConfigMap', 'Secret']

const canManage = computed(() => (auth.user?.roles.includes('system_admin') || auth.user?.roles.includes('operations_admin')) ?? false)

const planStatusLabels: Record<string, string> = {
  awaiting_confirmation: '待确认',
  executing: '执行中',
  succeeded: '成功',
  failed: '失败',
  expired: '已过期',
}

const createFormValid = computed(() =>
  sourceClusterId.value !== null
  && targetClusterId.value !== null
  && sourceClusterId.value !== targetClusterId.value
  && targetNamespace.value.trim() !== ''
  && items.value.length > 0,
)

const newItemValid = computed(() =>
  newItemKind.value !== ''
  && newItemNamespace.value.trim() !== ''
  && newItemName.value.trim() !== '',
)

const diffText = computed(() => (plan.value?.diff ? JSON.stringify(plan.value.diff, null, 2) : ''))

function clusterName(id: number | null | undefined): string {
  if (id === null || id === undefined) return '—'
  return clusters.value.find((c) => c.id === id)?.name ?? String(id)
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

function shortId(id: string): string {
  return id.length > 10 ? `${id.slice(0, 8)}…` : id
}

function addItem() {
  if (!newItemValid.value) return
  const kind = newItemKind.value
  const namespace = newItemNamespace.value.trim()
  const name = newItemName.value.trim()
  const exists = items.value.some((it) => it.kind === kind && it.namespace === namespace && it.name === name)
  if (exists) {
    errorMessage.value = '该资源已添加'
    return
  }
  items.value.push({ kind, namespace, name })
  newItemName.value = ''
  errorMessage.value = ''
}

function removeItem(index: number) {
  items.value.splice(index, 1)
}

async function loadClusters() {
  loadingClusters.value = true
  errorMessage.value = ''
  try {
    const enabled = (await listClusters(auth.accessToken)).items.filter((c) => c.enabled)
    clusters.value = enabled
    if (historyClusterId.value === null && enabled.length > 0) {
      historyClusterId.value = enabled[0].id
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载集群列表'
  } finally {
    loadingClusters.value = false
  }
}

async function runPreview() {
  if (!createFormValid.value || sourceClusterId.value === null) return
  previewing.value = true
  errorMessage.value = ''
  try {
    plan.value = await previewCopyPlan(auth.accessToken, sourceClusterId.value, {
      target_cluster_id: targetClusterId.value as number,
      target_namespace: targetNamespace.value.trim(),
      items: items.value,
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '预检失败，请检查资源与集群状态'
  } finally {
    previewing.value = false
  }
}

async function runExecute() {
  if (!plan.value?.id) return
  executing.value = true
  errorMessage.value = ''
  try {
    plan.value = await executeCopyPlan(auth.accessToken, plan.value.id, crypto.randomUUID())
    notice.value = '复制已执行完成'
    await loadHistory()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '执行失败，Kubernetes 可能拒绝了变更'
  } finally {
    executing.value = false
  }
}

function resetCreate() {
  plan.value = null
  notice.value = ''
  errorMessage.value = ''
}

async function loadHistory() {
  loadingHistory.value = true
  errorMessage.value = ''
  try {
    if (historyMode.value === 'mine') {
      historyPlans.value = (await listMyCopyPlans(auth.accessToken)).items
    } else if (historyClusterId.value !== null) {
      historyPlans.value = (await listCopyPlansByCluster(auth.accessToken, historyClusterId.value)).items
    } else {
      historyPlans.value = []
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载复制计划历史'
  } finally {
    loadingHistory.value = false
  }
}

watch(historyMode, () => loadHistory())
watch(historyClusterId, () => {
  if (historyMode.value === 'cluster') loadHistory()
})

onMounted(async () => {
  await loadClusters()
  await loadHistory()
})
</script>

<template>
  <ConsoleLayout eyebrow="交付与运维" title="跨集群复制">
    <section class="page-toolbar">
      <div><strong>{{ items.length }}</strong><span> 个待复制资源 · {{ historyPlans.length }} 个历史计划</span></div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="loadingClusters" @click="loadClusters"><RefreshCw :size="16" :class="{ spinning: loadingClusters }" />刷新集群</button>
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="notice" class="user-notice">{{ notice }}</p>

    <!-- 新建复制 -->
    <section class="copy-panel">
      <header class="panel-header">
        <div class="panel-title"><Copy :size="16" /><strong>新建复制</strong></div>
      </header>

      <div class="copy-body">
        <div class="form-grid">
          <div class="form-field">
            <label for="source-cluster">源集群</label>
            <select id="source-cluster" v-model="sourceClusterId" :disabled="loadingClusters">
              <option :value="null" disabled>选择源集群…</option>
              <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
          </div>
          <div class="form-field">
            <label for="target-cluster">目标集群</label>
            <select id="target-cluster" v-model="targetClusterId" :disabled="loadingClusters">
              <option :value="null" disabled>选择目标集群…</option>
              <option v-for="c in clusters" :key="c.id" :value="c.id" :disabled="c.id === sourceClusterId">{{ c.name }}</option>
            </select>
          </div>
          <div class="form-field">
            <label for="target-ns">目标命名空间</label>
            <input id="target-ns" v-model="targetNamespace" placeholder="default" maxlength="63" />
          </div>
        </div>

        <div class="items-section">
          <p class="context-label">资源项</p>
          <div class="item-add-form">
            <select v-model="newItemKind" class="kind-select">
              <option v-for="k in allowedKinds" :key="k" :value="k">{{ k }}</option>
            </select>
            <input v-model="newItemNamespace" placeholder="命名空间" maxlength="63" class="ns-input" @keydown.enter="addItem" />
            <input v-model="newItemName" placeholder="资源名称" maxlength="253" class="name-input" @keydown.enter="addItem" />
            <button class="secondary-button" type="button" :disabled="!newItemValid" @click="addItem"><Plus :size="14" />添加</button>
          </div>

          <table v-if="items.length > 0" class="data-table">
            <thead>
              <tr><th>#</th><th>Kind</th><th>命名空间</th><th>名称</th><th></th></tr>
            </thead>
            <tbody>
              <tr v-for="(it, i) in items" :key="`${it.kind}-${it.namespace}-${it.name}`">
                <td>{{ i + 1 }}</td>
                <td>{{ it.kind }}</td>
                <td>{{ it.namespace }}</td>
                <td>{{ it.name }}</td>
                <td><button class="icon-button compact" type="button" title="移除" @click="removeItem(i)"><XCircle :size="15" /></button></td>
              </tr>
            </tbody>
          </table>
          <p v-else class="empty-hint">尚未添加任何资源项。</p>
        </div>

        <div v-if="!canManage" class="permission-hint">仅系统管理员 / 运维管理员可创建并执行复制。</div>

        <div class="form-actions">
          <button class="secondary-button" type="button" :disabled="!createFormValid || previewing || !canManage" @click="runPreview">
            <Eye :size="16" /> {{ previewing ? '预检中…' : '预检 (Preview)' }}
          </button>
          <button v-if="plan" class="primary-button" type="button" :disabled="executing || !canManage" @click="runExecute">
            <Send :size="16" /> {{ executing ? '执行中…' : '确认执行' }}
          </button>
          <button v-if="plan" class="text-button" type="button" @click="resetCreate">重置</button>
        </div>

        <!-- 预检结果 -->
        <div v-if="plan" class="plan-preview">
          <div class="plan-summary">
            <div><span>计划 ID</span><strong>{{ shortId(plan.id) }}</strong></div>
            <div><span>状态</span><strong>{{ planStatusLabels[plan.status] ?? plan.status }}</strong></div>
            <div><span>源</span><strong>{{ clusterName(plan.source_cluster_id) }} / {{ plan.source_namespace }}</strong></div>
            <div><span>目标</span><strong>{{ clusterName(plan.target_cluster_id) }} / {{ plan.target_namespace }}</strong></div>
            <div><span>过期时间</span><strong>{{ formatTime(plan.expires_at) }}</strong></div>
          </div>

          <div v-if="plan.copy_summary && plan.copy_summary.length > 0" class="summary-block">
            <p class="context-label">复制摘要 (copy_summary)</p>
            <table class="data-table">
              <thead><tr><th>Kind</th><th>源</th><th>目标</th></tr></thead>
              <tbody>
                <tr v-for="(s, i) in plan.copy_summary" :key="i">
                  <td>{{ s.kind }}</td>
                  <td>{{ s.namespace }}/{{ s.name }}</td>
                  <td>{{ s.destination_namespace }}/{{ s.destination_name }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="diffText" class="diff-block">
            <p class="context-label">差异 (diff)</p>
            <pre>{{ diffText }}</pre>
          </div>

          <div class="confirmation-warning">
            <p>确认令牌仅显示一次。执行后无法撤回，请确认预检通过。</p>
          </div>
        </div>
      </div>
    </section>

    <!-- 复制计划历史 -->
    <section class="copy-panel">
      <header class="panel-header">
        <div class="panel-title"><History :size="16" /><strong>复制计划历史</strong></div>
        <button class="secondary-button" type="button" :disabled="loadingHistory" @click="loadHistory"><RefreshCw :size="14" :class="{ spinning: loadingHistory }" />刷新</button>
      </header>

      <div class="history-tabs">
        <button type="button" class="history-tab" :class="{ active: historyMode === 'mine' }" @click="historyMode = 'mine'">
          <Boxes :size="14" /> 我的计划
        </button>
        <button type="button" class="history-tab" :class="{ active: historyMode === 'cluster' }" @click="historyMode = 'cluster'">
          <ArrowRight :size="14" /> 按集群
        </button>
        <select v-if="historyMode === 'cluster'" v-model="historyClusterId" class="history-cluster-select" :disabled="loadingClusters">
          <option :value="null" disabled>选择集群…</option>
          <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </div>

      <div v-if="loadingHistory" class="empty-hint">正在加载计划…</div>
      <div v-else-if="historyPlans.length === 0" class="empty-state">
        <History :size="36" />
        <strong>暂无复制计划</strong>
        <span>{{ historyMode === 'mine' ? '你还没有创建过复制计划。' : '所选集群暂无复制计划。' }}</span>
      </div>
      <div v-else class="history-table-wrap">
        <table class="data-table history-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>状态</th>
              <th>源 → 目标集群</th>
              <th>源 → 目标命名空间</th>
              <th>创建时间</th>
              <th>执行时间</th>
              <th>错误</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in historyPlans" :key="p.id">
              <td class="id-cell" :title="p.id">{{ shortId(p.id) }}</td>
              <td><span class="plan-status" :class="p.status">{{ planStatusLabels[p.status] ?? p.status }}</span></td>
              <td>{{ clusterName(p.source_cluster_id) }} <ArrowRight :size="12" class="arrow-icon" /> {{ clusterName(p.target_cluster_id) }}</td>
              <td>{{ p.source_namespace }} <ArrowRight :size="12" class="arrow-icon" /> {{ p.target_namespace }}</td>
              <td class="muted-cell">{{ formatTime(p.created_at) }}</td>
              <td class="muted-cell">{{ formatTime(p.executed_at) }}</td>
              <td class="error-cell" :title="p.last_error || ''">{{ p.last_error || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.copy-panel {
  margin-top: 18px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}

.panel-header {
  display: flex;
  min-height: 56px;
  padding: 12px 16px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 700;
}

.copy-body {
  padding: 18px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.form-field label {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 600;
}

.form-field input,
.form-field select {
  padding: 8px 10px;
  color: var(--text-primary);
  font: inherit;
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  outline: none;
}

.form-field input:focus,
.form-field select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.items-section {
  margin-top: 20px;
}

.item-add-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.item-add-form select,
.item-add-form input {
  padding: 7px 10px;
  color: var(--text-primary);
  font: inherit;
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  outline: none;
}

.item-add-form select:focus,
.item-add-form input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.kind-select {
  min-width: 140px;
}

.ns-input,
.name-input {
  min-width: 140px;
}

.name-input {
  flex: 1;
  min-width: 180px;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  margin-top: 10px;
}

.data-table th {
  text-align: left;
  padding: 8px 12px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-weight: 600;
  border-bottom: 1px solid var(--border-subtle);
}

.data-table td {
  padding: 9px 12px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

.data-table tbody tr:last-child td {
  border-bottom: 0;
}

.data-table tbody tr:hover td {
  background: var(--bg-secondary);
}

.empty-hint {
  padding: 14px 0;
  color: var(--text-tertiary);
  font-size: 12px;
}

.permission-hint {
  margin-top: 14px;
  padding: 9px 12px;
  color: var(--text-tertiary);
  font-size: 12px;
  background: var(--bg-secondary);
  border-left: 3px solid var(--border-strong);
  border-radius: var(--radius-sm);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
  margin-top: 18px;
}

.plan-preview {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid var(--border-subtle);
}

.plan-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  padding: 14px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
}

.plan-summary > div {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.plan-summary span {
  font-size: 11px;
  color: var(--text-tertiary);
}

.plan-summary strong {
  font-size: 13px;
  color: var(--text-primary);
}

.summary-block {
  margin-top: 16px;
}

.diff-block {
  margin-top: 16px;
}

.diff-block pre {
  margin: 6px 0 0;
  padding: 12px;
  max-height: 280px;
  overflow: auto;
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.5;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  white-space: pre-wrap;
}

.confirmation-warning {
  margin-top: 14px;
  padding: 10px 12px;
  background: var(--warning-bg);
  border-left: 3px solid var(--status-warning);
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--status-warning);
}

.confirmation-warning p {
  margin: 0;
}

.history-tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.history-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 12px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}

.history-tab:hover:not(:disabled) {
  border-color: var(--border-strong);
}

.history-tab.active {
  color: #ffffff;
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.history-cluster-select {
  height: 32px;
  min-width: 180px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}

.history-cluster-select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}

.history-table-wrap {
  overflow-x: auto;
}

.history-table {
  min-width: 880px;
}

.id-cell {
  font-family: var(--font-mono);
  font-size: 11px;
}

.muted-cell {
  color: var(--text-tertiary);
  font-size: 11px;
  white-space: nowrap;
}

.arrow-icon {
  color: var(--text-tertiary);
  vertical-align: -2px;
  margin: 0 3px;
}

.error-cell {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--status-danger);
  font-size: 11px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  padding: 24px;
  color: var(--text-tertiary);
  text-align: center;
}

.empty-state svg {
  color: var(--text-muted);
}

.empty-state strong {
  margin-top: 12px;
  color: var(--text-secondary);
  font-size: 14px;
}

.empty-state span {
  margin-top: 6px;
  font-size: 12px;
}

.plan-status {
  display: inline-flex;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  white-space: nowrap;
}

.plan-status.awaiting_confirmation { color: var(--status-warning); background: var(--warning-bg); }
.plan-status.executing { color: var(--status-info); background: var(--info-bg); }
.plan-status.succeeded { color: var(--status-success); background: var(--success-bg); }
.plan-status.failed { color: var(--status-danger); background: var(--danger-bg); }
.plan-status.expired { color: var(--text-secondary); background: var(--bg-tertiary); }

.icon-button.compact {
  width: 28px;
  height: 28px;
  color: var(--text-tertiary);
}

.icon-button.compact:hover:not(:disabled) {
  color: var(--status-danger);
}

@media (max-width: 900px) {
  .form-grid { grid-template-columns: 1fr; }
  .plan-summary { grid-template-columns: 1fr; }
  .item-add-form { flex-direction: column; align-items: stretch; }
  .item-add-form .name-input { min-width: 0; }
}
</style>
