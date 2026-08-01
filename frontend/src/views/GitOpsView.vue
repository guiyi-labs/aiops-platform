<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { AlertTriangle, ChevronDown, GitBranch, RefreshCw } from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import { getGitOpsCapability, listGitOpsApplications } from '../api/gitops'
import { listClusters } from '../api/clusters'
import type { GitOpsCapability, GitOpsApplication } from '../types/gitops'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterId = ref<number | null>(null)
const capability = ref<GitOpsCapability | null>(null)
const apps = ref<GitOpsApplication[]>([])
const expandedName = ref<string | null>(null)

const loadingClusters = ref(true)
const loadingCapability = ref(false)
const loadingApps = ref(false)

const errorMessage = ref('')

const syncStatusLabels: Record<string, string> = {
  Synced: '已同步',
  OutOfSync: '未同步',
  Unknown: '未知',
}

const healthStatusLabels: Record<string, string> = {
  Healthy: '健康',
  Progressing: '进行中',
  Degraded: '降级',
  Suspended: '已挂起',
  Missing: '缺失',
  Unknown: '未知',
}

const selectedCluster = computed(() => clusters.value.find((c) => c.id === selectedClusterId.value) ?? null)
const installed = computed(() => capability.value?.installed !== false)

function syncStatusClass(status: string): string {
  const s = status.toLowerCase()
  if (s === 'synced') return 'green'
  if (s === 'outofsync') return 'amber'
  return 'gray'
}

function healthStatusClass(status: string): string {
  const s = status.toLowerCase()
  if (s === 'healthy') return 'green'
  if (s === 'progressing') return 'blue'
  if (s === 'degraded') return 'red'
  return 'gray'
}

function toggleExpand(name: string) {
  expandedName.value = expandedName.value === name ? null : name
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

async function loadClusters() {
  loadingClusters.value = true
  errorMessage.value = ''
  try {
    const enabled = (await listClusters(auth.accessToken)).items.filter((c) => c.enabled)
    clusters.value = enabled
    if (selectedClusterId.value === null && enabled.length > 0) {
      selectedClusterId.value = enabled[0].id
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载集群列表'
  } finally {
    loadingClusters.value = false
  }
}

async function loadCapability(clusterId: number) {
  loadingCapability.value = true
  try {
    capability.value = await getGitOpsCapability(auth.accessToken, clusterId)
  } catch (error) {
    capability.value = { installed: false }
    errorMessage.value = error instanceof Error ? error.message : '无法检测 ArgoCD 能力'
  } finally {
    loadingCapability.value = false
  }
}

async function loadApps(clusterId: number) {
  loadingApps.value = true
  errorMessage.value = ''
  apps.value = []
  expandedName.value = null
  try {
    apps.value = (await listGitOpsApplications(auth.accessToken, clusterId)).items
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载 GitOps 应用列表'
  } finally {
    loadingApps.value = false
  }
}

async function refresh() {
  if (selectedClusterId.value === null) return
  await Promise.all([loadCapability(selectedClusterId.value), loadApps(selectedClusterId.value)])
}

watch(selectedClusterId, (id) => {
  if (id !== null) refresh()
})

onMounted(async () => {
  await loadClusters()
  if (selectedClusterId.value !== null) await refresh()
})
</script>

<template>
  <ConsoleLayout eyebrow="交付与运维" title="GitOps 应用">
    <section class="page-toolbar">
      <div>
        <label for="gitops-cluster" class="cluster-select-label">集群</label>
        <select id="gitops-cluster" v-model="selectedClusterId" :disabled="loadingClusters">
          <option :value="null" disabled>选择集群…</option>
          <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="selectedClusterId === null || loadingApps" @click="refresh">
          <RefreshCw :size="16" :class="{ spinning: loadingApps }" />刷新
        </button>
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <!-- 能力指示器 -->
    <div v-if="selectedClusterId !== null && !loadingCapability && !installed" class="capability-banner">
      <AlertTriangle :size="18" />
      <div>
        <strong>ArgoCD 未安装</strong>
        <p>集群 <strong>{{ selectedCluster?.name }}</strong> 未安装 ArgoCD，无法展示 GitOps 应用。请联系管理员安装 ArgoCD。</p>
      </div>
    </div>
    <div v-else-if="selectedClusterId !== null && capability?.installed" class="capability-ok">
      <GitBranch :size="16" />
      <span>ArgoCD 已安装<span v-if="capability.version"> · 版本 {{ capability.version }}</span></span>
    </div>

    <!-- 应用列表 -->
    <section v-if="selectedClusterId !== null && installed" class="gitops-panel">
      <header class="panel-header">
        <div class="panel-title"><GitBranch :size="16" /><strong>GitOps 应用</strong></div>
        <span class="muted-cell">{{ apps.length }} 个</span>
      </header>

      <div v-if="loadingApps" class="empty-hint">正在加载应用…</div>
      <div v-else-if="apps.length === 0" class="empty-state">
        <GitBranch :size="36" />
        <strong>暂无 GitOps 应用</strong>
        <span>该集群上未发现 ArgoCD Application 资源。</span>
      </div>
      <div v-else class="apps-table-wrap">
        <table class="apps-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>项目</th>
              <th>同步状态</th>
              <th>健康状态</th>
              <th>源仓库</th>
              <th>目标命名空间</th>
              <th>最近同步开始</th>
              <th>最近同步完成</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <template v-for="app in apps" :key="app.name">
              <tr class="app-row" :class="{ expanded: expandedName === app.name }" @click="toggleExpand(app.name)">
                <td><strong>{{ app.name }}</strong></td>
                <td>{{ app.project || '—' }}</td>
                <td><span class="status-badge" :class="syncStatusClass(app.sync_status)">{{ syncStatusLabels[app.sync_status] ?? app.sync_status }}</span></td>
                <td><span class="status-badge" :class="healthStatusClass(app.health_status)">{{ healthStatusLabels[app.health_status] ?? app.health_status }}</span></td>
                <td class="truncate-cell">{{ app.source_repo_url || '—' }}</td>
                <td>{{ app.destination_namespace || '—' }}</td>
                <td class="muted-cell">{{ formatTime(app.last_sync_started_at) }}</td>
                <td class="muted-cell">{{ formatTime(app.last_sync_finished_at) }}</td>
                <td><ChevronDown :size="16" class="expand-icon" :class="{ rotated: expandedName === app.name }" /></td>
              </tr>
              <tr v-if="expandedName === app.name" class="app-detail-row">
                <td colspan="9">
                  <div class="app-detail">
                    <dl class="detail-grid">
                      <div><dt>同步 Revision</dt><dd>{{ app.sync_revision || '—' }}</dd></div>
                      <div><dt>源 Target Revision</dt><dd>{{ app.source_target_revision || '—' }}</dd></div>
                      <div><dt>源 Path</dt><dd>{{ app.source_path || '—' }}</dd></div>
                      <div><dt>目标 Server</dt><dd>{{ app.destination_server || '—' }}</dd></div>
                      <div><dt>操作阶段</dt><dd>{{ app.operation_state_phase || '—' }}</dd></div>
                      <div><dt>健康消息</dt><dd>{{ app.health_message || '—' }}</dd></div>
                    </dl>
                    <div v-if="app.conditions && app.conditions.length > 0" class="conditions-block">
                      <p class="context-label">Conditions</p>
                      <ul class="conditions-list">
                        <li v-for="(cond, i) in app.conditions" :key="i">{{ cond }}</li>
                      </ul>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.cluster-select-label {
  margin-right: 8px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.page-toolbar > div:first-child {
  display: flex;
  align-items: center;
}

.page-toolbar select {
  height: 36px;
  min-width: 200px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}

.page-toolbar select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}

.capability-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-top: 16px;
  padding: 14px 16px;
  color: var(--status-warning);
  background: var(--warning-bg);
  border: 1px solid #ead7a8;
  border-left: 3px solid var(--status-warning);
  border-radius: var(--radius-md);
}

.capability-banner svg {
  flex: 0 0 auto;
  margin-top: 1px;
}

.capability-banner strong {
  color: var(--status-warning);
  font-size: 13px;
}

.capability-banner p {
  margin: 4px 0 0;
  color: #8a6d10;
  font-size: 12px;
  line-height: 1.5;
}

.capability-banner p strong {
  color: #6b5410;
}

.capability-ok {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
  padding: 9px 12px;
  color: var(--status-success);
  font-size: 12px;
  background: var(--success-bg);
  border-radius: var(--radius-md);
}

.capability-ok span {
  color: #1d7a4f;
}

.gitops-panel {
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

.muted-cell {
  color: var(--text-tertiary);
  font-size: 11px;
}

.truncate-cell {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-hint {
  padding: 22px 16px;
  color: var(--text-tertiary);
  font-size: 12px;
  text-align: center;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 240px;
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

.apps-table-wrap {
  overflow-x: auto;
}

.apps-table {
  width: 100%;
  min-width: 980px;
  border-collapse: collapse;
  font-size: 12px;
}

.apps-table th {
  text-align: left;
  padding: 10px 14px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-weight: 600;
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
}

.apps-table td {
  padding: 11px 14px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
  vertical-align: top;
}

.apps-table td strong {
  color: var(--text-primary);
  font-size: 13px;
}

.app-row {
  cursor: pointer;
  transition: background var(--transition-fast);
}

.app-row:hover td {
  background: var(--bg-secondary);
}

.app-row.expanded td {
  background: var(--accent-subtle);
}

.expand-icon {
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}

.expand-icon.rotated {
  transform: rotate(180deg);
}

.app-detail-row td {
  padding: 0;
  background: var(--bg-secondary);
}

.app-detail {
  padding: 16px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  margin: 0;
  background: var(--border-subtle);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.detail-grid > div {
  display: grid;
  gap: 4px;
  padding: 11px 13px;
  background: var(--bg-elevated);
}

.detail-grid dt {
  color: var(--text-tertiary);
  font-size: 11px;
}

.detail-grid dd {
  margin: 0;
  color: var(--text-primary);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.conditions-block {
  margin-top: 14px;
}

.conditions-list {
  margin: 6px 0 0;
  padding: 12px 14px 12px 28px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.7;
}

.status-badge {
  display: inline-flex;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  border-radius: var(--radius-full);
  white-space: nowrap;
}

.status-badge.green { color: var(--status-success); background: var(--success-bg); }
.status-badge.amber { color: var(--status-warning); background: var(--warning-bg); }
.status-badge.blue { color: var(--status-info); background: var(--info-bg); }
.status-badge.red { color: var(--status-danger); background: var(--danger-bg); }
.status-badge.gray { color: var(--text-secondary); background: var(--bg-tertiary); }

@media (max-width: 900px) {
  .detail-grid { grid-template-columns: 1fr; }
}
</style>
