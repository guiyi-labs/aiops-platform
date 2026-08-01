<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Boxes, ChevronRight, ExternalLink, Eye, Package, Plus, RefreshCw, Rocket, Send, Trash2, X } from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import { listRepositories, createRepository, deleteRepository, listCharts, getChart, listAppCatalogPlans, previewDeploy, executeDeploy } from '../api/appcatalog'
import { listClusters } from '../api/clusters'
import type { RepositoryView, ChartSummary, ChartDetail, AppCatalogPlan } from '../types/appcatalog'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const repos = ref<RepositoryView[]>([])
const plans = ref<AppCatalogPlan[]>([])

const loadingRepos = ref(true)
const loadingClusters = ref(false)
const loadingCharts = ref(false)
const loadingDetail = ref(false)
const loadingPlans = ref(false)
const creating = ref(false)
const previewing = ref(false)
const executing = ref(false)
const deletingRepoId = ref<number | null>(null)

const errorMessage = ref('')
const notice = ref('')

const canManage = computed(() => (auth.user?.roles.includes('system_admin') || auth.user?.roles.includes('operations_admin')) ?? false)

const showCreateForm = ref(false)
const newName = ref('')
const newDisplayName = ref('')
const newUrl = ref('')
const newUsername = ref('')
const newPassword = ref('')

const selectedRepo = ref<RepositoryView | null>(null)
const charts = ref<ChartSummary[]>([])
const selectedChart = ref<ChartDetail | null>(null)
const selectedChartName = ref<string | null>(null)

const showDeployForm = ref(false)
const deployVersion = ref('')
const deployClusterId = ref<number | null>(null)
const deployNamespace = ref('default')
const deployReleaseName = ref('')
const deployValues = ref('')

const plan = ref<AppCatalogPlan | null>(null)

const planStatusLabels: Record<string, string> = {
  awaiting_confirmation: '待确认',
  executing: '执行中',
  succeeded: '成功',
  failed: '失败',
  expired: '已过期',
}

const createFormValid = computed(() =>
  newName.value.trim() !== ''
  && newUrl.value.trim() !== ''
  && newDisplayName.value.trim() !== '',
)

const deployFormValid = computed(() =>
  deployVersion.value !== ''
  && deployClusterId.value !== null
  && deployNamespace.value.trim() !== ''
  && deployReleaseName.value.trim() !== '',
)

async function loadClusters() {
  loadingClusters.value = true
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((c) => c.enabled)
  } catch {
    // 集群加载失败不阻塞主流程
  } finally {
    loadingClusters.value = false
  }
}

async function loadRepos() {
  loadingRepos.value = true
  errorMessage.value = ''
  try {
    repos.value = (await listRepositories(auth.accessToken)).items
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载仓库列表'
  } finally {
    loadingRepos.value = false
  }
}

async function loadPlans() {
  loadingPlans.value = true
  try {
    plans.value = (await listAppCatalogPlans(auth.accessToken)).items
  } catch {
    // 计划加载失败不阻塞
  } finally {
    loadingPlans.value = false
  }
}

async function submitCreate() {
  if (!createFormValid.value) return
  creating.value = true
  errorMessage.value = ''
  try {
    await createRepository(auth.accessToken, {
      name: newName.value.trim(),
      display_name: newDisplayName.value.trim(),
      url: newUrl.value.trim(),
      username: newUsername.value.trim() || undefined,
      password: newPassword.value.trim() || undefined,
    })
    notice.value = '仓库已添加'
    newName.value = ''
    newDisplayName.value = ''
    newUrl.value = ''
    newUsername.value = ''
    newPassword.value = ''
    showCreateForm.value = false
    await loadRepos()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '添加仓库失败'
  } finally {
    creating.value = false
  }
}

async function removeRepo(repo: RepositoryView) {
  deletingRepoId.value = repo.id
  errorMessage.value = ''
  try {
    await deleteRepository(auth.accessToken, repo.id)
    notice.value = `仓库 ${repo.name} 已删除`
    if (selectedRepo.value?.id === repo.id) {
      selectedRepo.value = null
      charts.value = []
      selectedChart.value = null
    }
    await loadRepos()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '删除仓库失败'
  } finally {
    deletingRepoId.value = null
  }
}

async function selectRepo(repo: RepositoryView) {
  selectedRepo.value = repo
  selectedChart.value = null
  selectedChartName.value = null
  charts.value = []
  loadingCharts.value = true
  errorMessage.value = ''
  try {
    charts.value = (await listCharts(auth.accessToken, repo.id)).items
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载 Chart 列表'
  } finally {
    loadingCharts.value = false
  }
}

async function selectChart(name: string) {
  if (!selectedRepo.value) return
  selectedChartName.value = name
  selectedChart.value = null
  loadingDetail.value = true
  errorMessage.value = ''
  try {
    const detail = await getChart(auth.accessToken, selectedRepo.value.id, name)
    selectedChart.value = detail
    deployVersion.value = detail.versions[0]?.version ?? ''
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载 Chart 详情'
  } finally {
    loadingDetail.value = false
  }
}

function openDeployForm() {
  if (!selectedChart.value) return
  showDeployForm.value = true
  plan.value = null
  deployReleaseName.value = selectedChart.value.name
}

function closeDeployForm() {
  showDeployForm.value = false
  plan.value = null
}

async function runPreview() {
  if (!selectedRepo.value || !selectedChartName.value || !deployFormValid.value) return
  previewing.value = true
  errorMessage.value = ''
  try {
    plan.value = await previewDeploy(auth.accessToken, {
      repo_id: selectedRepo.value.id,
      chart_name: selectedChartName.value,
      chart_version: deployVersion.value,
      target_cluster_id: deployClusterId.value as number,
      target_namespace: deployNamespace.value.trim(),
      release_name: deployReleaseName.value.trim(),
      values_yaml: deployValues.value.trim() || undefined,
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '预检失败，请检查参数与集群状态'
  } finally {
    previewing.value = false
  }
}

async function runExecute() {
  if (!plan.value?.id || !plan.value?.confirmation_token) return
  executing.value = true
  errorMessage.value = ''
  try {
    plan.value = await executeDeploy(auth.accessToken, plan.value.id, plan.value.confirmation_token, crypto.randomUUID())
    notice.value = '部署已执行完成'
    await loadPlans()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '执行失败，Kubernetes 可能拒绝了变更'
  } finally {
    executing.value = false
  }
}

function clusterName(id: number | null | undefined): string {
  if (id === null || id === undefined) return '—'
  return clusters.value.find((c) => c.id === id)?.name ?? String(id)
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

const diffText = computed(() => (plan.value?.deploy_diff ? JSON.stringify(plan.value.deploy_diff, null, 2) : ''))

onMounted(async () => {
  await Promise.all([loadClusters(), loadRepos(), loadPlans()])
})
</script>

<template>
  <ConsoleLayout eyebrow="交付与运维" title="Helm 应用目录">
    <section class="page-toolbar">
      <div><strong>{{ repos.length }}</strong><span> 个仓库 · {{ plans.length }} 个部署计划</span></div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="loadingRepos" @click="loadRepos"><RefreshCw :size="16" :class="{ spinning: loadingRepos }" />刷新</button>
        <button v-if="canManage" class="primary-button" type="button" @click="showCreateForm = !showCreateForm"><Plus :size="16" />添加仓库</button>
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="notice" class="user-notice">{{ notice }}</p>

    <!-- 仓库管理 -->
    <section class="catalog-panel">
      <header class="panel-header">
        <div class="panel-title"><Boxes :size="16" /><strong>仓库管理</strong></div>
      </header>

      <div v-if="showCreateForm && canManage" class="create-repo-form">
        <div class="form-grid">
          <div class="form-field">
            <label for="repo-name">名称</label>
            <input id="repo-name" v-model="newName" placeholder="例如 bitnami" maxlength="63" />
          </div>
          <div class="form-field">
            <label for="repo-display">显示名</label>
            <input id="repo-display" v-model="newDisplayName" placeholder="例如 Bitnami Charts" />
          </div>
          <div class="form-field form-field-wide">
            <label for="repo-url">URL</label>
            <input id="repo-url" v-model="newUrl" placeholder="https://charts.bitnami.com/bitnami" />
          </div>
          <div class="form-field">
            <label for="repo-user">用户名（可选）</label>
            <input id="repo-user" v-model="newUsername" placeholder="私有仓库可填写" autocomplete="off" />
          </div>
          <div class="form-field">
            <label for="repo-pass">密码（可选）</label>
            <input id="repo-pass" v-model="newPassword" type="password" placeholder="私有仓库可填写" autocomplete="new-password" />
          </div>
        </div>
        <div class="form-actions">
          <button class="secondary-button" type="button" @click="showCreateForm = false">取消</button>
          <button class="primary-button" type="button" :disabled="!createFormValid || creating" @click="submitCreate">
            <Plus :size="16" /> {{ creating ? '添加中…' : '添加' }}
          </button>
        </div>
      </div>

      <div v-if="loadingRepos" class="empty-hint">正在加载仓库…</div>
      <table v-else-if="repos.length > 0" class="data-table">
        <thead>
          <tr><th>名称</th><th>显示名</th><th>URL</th><th>认证</th><th>创建时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="repo in repos" :key="repo.id" :class="{ 'row-selected': selectedRepo?.id === repo.id }">
            <td>
              <button class="repo-name-btn" type="button" @click="selectRepo(repo)">
                <ChevronRight :size="14" :class="{ rotated: selectedRepo?.id === repo.id }" />
                <strong>{{ repo.name }}</strong>
              </button>
            </td>
            <td>{{ repo.display_name }}</td>
            <td class="truncate-cell">{{ repo.url }}</td>
            <td><span class="auth-badge" :class="{ on: repo.has_auth }">{{ repo.has_auth ? '已认证' : '公开' }}</span></td>
            <td class="muted-cell">{{ formatTime(repo.created_at) }}</td>
            <td>
              <button v-if="canManage" class="danger-button" type="button" title="删除仓库" :disabled="deletingRepoId === repo.id" @click="removeRepo(repo)">
                <Trash2 :size="14" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-hint">尚未添加任何仓库。</p>
    </section>

    <!-- Chart 浏览 + 部署 -->
    <section v-if="selectedRepo" class="catalog-panel">
      <header class="panel-header">
        <div class="panel-title"><Package :size="16" /><strong>{{ selectedRepo.display_name }} 的 Charts</strong></div>
        <span class="muted-cell">{{ charts.length }} 个</span>
      </header>

      <div v-if="loadingCharts" class="empty-hint">正在加载 Charts…</div>
      <div v-else-if="charts.length === 0" class="empty-hint">该仓库暂无可用 Chart。</div>
      <div v-else class="chart-grid">
        <article
          v-for="chart in charts"
          :key="chart.name"
          class="chart-card"
          :class="{ active: selectedChartName === chart.name }"
          tabindex="0"
          @click="selectChart(chart.name)"
          @keydown.enter="selectChart(chart.name)"
        >
          <div class="chart-card-head">
            <img v-if="chart.icon" :src="chart.icon" alt="" class="chart-icon" @error="(e) => (e.target as HTMLImageElement).style.display = 'none'" />
            <span v-else class="chart-icon-placeholder"><Package :size="18" /></span>
            <div class="chart-card-title">
              <strong>{{ chart.name }}</strong>
              <small>v{{ chart.version }}</small>
            </div>
          </div>
          <p class="chart-desc">{{ chart.description || '暂无描述' }}</p>
          <span v-if="chart.app_version" class="chart-app-version">app: {{ chart.app_version }}</span>
        </article>
      </div>

      <!-- Chart 详情 -->
      <div v-if="selectedChartName" class="chart-detail">
        <div class="section-heading">
          <div><p class="context-label">Chart 详情</p><h2>{{ selectedChartName }}</h2></div>
        </div>
        <div v-if="loadingDetail" class="empty-hint">正在加载详情…</div>
        <template v-else-if="selectedChart">
          <dl class="chart-meta">
            <div><dt>描述</dt><dd>{{ selectedChart.description || '—' }}</dd></div>
            <div><dt>主页</dt><dd>
              <a v-if="selectedChart.home" :href="selectedChart.home" target="_blank" rel="noopener" class="home-link">{{ selectedChart.home }}<ExternalLink :size="12" /></a>
              <span v-else>—</span>
            </dd></div>
            <div><dt>维护者</dt><dd>{{ selectedChart.maintainers?.join('、') || '—' }}</dd></div>
          </dl>
          <h3 class="versions-heading">可用版本 ({{ selectedChart.versions.length }})</h3>
          <table class="data-table">
            <thead><tr><th>版本</th><th>App 版本</th><th>创建时间</th><th>Digest</th></tr></thead>
            <tbody>
              <tr v-for="v in selectedChart.versions" :key="v.version">
                <td>{{ v.version }}</td>
                <td>{{ v.app_version || '—' }}</td>
                <td class="muted-cell">{{ formatTime(v.created) }}</td>
                <td class="muted-cell digest-cell">{{ v.digest.slice(0, 19) }}…</td>
              </tr>
            </tbody>
          </table>
          <div class="form-actions">
            <button class="primary-button" type="button" @click="openDeployForm"><Rocket :size="16" />部署</button>
          </div>
        </template>
      </div>
    </section>

    <!-- 部署表单 -->
    <section v-if="showDeployForm && selectedChart" class="catalog-panel">
      <header class="panel-header">
        <div class="panel-title"><Rocket :size="16" /><strong>部署 {{ selectedChartName }}</strong></div>
        <button class="icon-button" type="button" title="关闭" @click="closeDeployForm"><X :size="16" /></button>
      </header>

      <div class="form-grid">
        <div class="form-field">
          <label for="deploy-version">Chart 版本</label>
          <select id="deploy-version" v-model="deployVersion">
            <option value="" disabled>选择版本…</option>
            <option v-for="v in selectedChart.versions" :key="v.version" :value="v.version">{{ v.version }} (app {{ v.app_version || '—' }})</option>
          </select>
        </div>
        <div class="form-field">
          <label for="deploy-cluster">目标集群</label>
          <select id="deploy-cluster" v-model="deployClusterId" :disabled="loadingClusters">
            <option :value="null" disabled>选择集群…</option>
            <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </div>
        <div class="form-field">
          <label for="deploy-ns">目标命名空间</label>
          <input id="deploy-ns" v-model="deployNamespace" placeholder="default" maxlength="63" />
        </div>
        <div class="form-field">
          <label for="deploy-release">Release 名称</label>
          <input id="deploy-release" v-model="deployReleaseName" placeholder="例如 my-app" maxlength="63" />
        </div>
        <div class="form-field form-field-wide">
          <label for="deploy-values">Values YAML（可选）</label>
          <textarea id="deploy-values" v-model="deployValues" rows="6" placeholder="# 覆盖默认值" />
        </div>
      </div>

      <div class="form-actions">
        <button class="secondary-button" type="button" :disabled="!deployFormValid || previewing" @click="runPreview">
          <Eye :size="16" /> {{ previewing ? '预检中…' : '预检 (Preview)' }}
        </button>
        <button v-if="plan" class="primary-button" type="button" :disabled="executing || !canManage" @click="runExecute">
          <Send :size="16" /> {{ executing ? '执行中…' : '确认执行' }}
        </button>
      </div>

      <!-- 预检结果 -->
      <div v-if="plan" class="plan-preview">
        <div class="plan-summary">
          <div><span>计划 ID</span><strong>{{ plan.id.slice(0, 8) }}…</strong></div>
          <div><span>状态</span><strong>{{ planStatusLabels[plan.status] ?? plan.status }}</strong></div>
          <div><span>Chart</span><strong>{{ plan.chart_name }} v{{ plan.chart_version }}</strong></div>
          <div><span>目标</span><strong>{{ clusterName(plan.target_cluster_id) }} / {{ plan.target_namespace }}</strong></div>
          <div><span>Release</span><strong>{{ plan.release_name }}</strong></div>
          <div><span>过期时间</span><strong>{{ formatTime(plan.expires_at) }}</strong></div>
        </div>
        <div v-if="diffText" class="diff-block">
          <p class="context-label">部署差异 (deploy_diff)</p>
          <pre>{{ diffText }}</pre>
        </div>
        <div class="confirmation-warning">
          <p>确认令牌仅显示一次。执行后无法撤回，请确认预检通过。</p>
        </div>
      </div>
    </section>

    <!-- 部署计划 -->
    <section class="catalog-panel">
      <header class="panel-header">
        <div class="panel-title"><Package :size="16" /><strong>部署计划</strong></div>
        <button class="secondary-button" type="button" :disabled="loadingPlans" @click="loadPlans"><RefreshCw :size="14" :class="{ spinning: loadingPlans }" />刷新</button>
      </header>
      <div v-if="loadingPlans" class="empty-hint">正在加载计划…</div>
      <table v-else-if="plans.length > 0" class="data-table">
        <thead>
          <tr><th>状态</th><th>Chart</th><th>版本</th><th>目标集群</th><th>Release</th><th>创建时间</th><th>过期时间</th></tr>
        </thead>
        <tbody>
          <tr v-for="p in plans" :key="p.id">
            <td><span class="plan-status" :class="p.status">{{ planStatusLabels[p.status] ?? p.status }}</span></td>
            <td>{{ p.chart_name }}</td>
            <td>{{ p.chart_version }}</td>
            <td>{{ clusterName(p.target_cluster_id) }}</td>
            <td>{{ p.release_name }}</td>
            <td class="muted-cell">{{ formatTime(p.created_at) }}</td>
            <td class="muted-cell">{{ formatTime(p.expires_at) }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-hint">暂无部署计划。</p>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.catalog-panel {
  margin-top: 18px;
  padding: 0;
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
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-selected td {
  background: var(--accent-subtle);
}

.repo-name-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--accent-primary);
  background: transparent;
  border: 0;
  font-size: 12px;
  font-weight: 600;
  padding: 0;
}

.repo-name-btn svg {
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}

.repo-name-btn svg.rotated {
  transform: rotate(90deg);
}

.auth-badge {
  display: inline-flex;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
}

.auth-badge.on {
  color: var(--status-success);
  background: var(--success-bg);
}

.create-repo-form {
  padding: 16px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.form-field-wide {
  grid-column: 1 / -1;
}

.form-field label {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 600;
}

.form-field input,
.form-field select,
.form-field textarea {
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
.form-field select:focus,
.form-field textarea:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.form-field textarea {
  resize: vertical;
  font-family: var(--font-mono);
  line-height: 1.5;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
  margin-top: 14px;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.data-table th {
  text-align: left;
  padding: 9px 14px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-weight: 600;
  border-bottom: 1px solid var(--border-subtle);
}

.data-table td {
  padding: 10px 14px;
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
  padding: 18px 16px;
  color: var(--text-tertiary);
  font-size: 12px;
}

.chart-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  padding: 16px;
}

.chart-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  cursor: pointer;
  outline: none;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.chart-card:hover,
.chart-card:focus-visible {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.chart-card.active {
  border-color: var(--accent-primary);
  background: var(--accent-subtle);
}

.chart-card-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.chart-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-sm);
}

.chart-icon-placeholder {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.chart-card-title {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.chart-card-title strong {
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chart-card-title small {
  color: var(--text-tertiary);
  font-size: 11px;
}

.chart-desc {
  margin: 0;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

.chart-app-version {
  color: var(--text-tertiary);
  font-size: 11px;
}

.chart-detail {
  padding: 16px;
  border-top: 1px solid var(--border-subtle);
}

.section-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 40px;
}

.chart-meta {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  margin: 14px 0 0;
  background: var(--border-subtle);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.chart-meta > div {
  display: grid;
  gap: 4px;
  padding: 12px;
  background: var(--bg-elevated);
}

.chart-meta dt {
  color: var(--text-tertiary);
  font-size: 11px;
}

.chart-meta dd {
  margin: 0;
  color: var(--text-primary);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.home-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--accent-primary);
  text-decoration: none;
}

.home-link:hover {
  text-decoration: underline;
}

.versions-heading {
  margin: 18px 0 8px;
  color: var(--text-secondary);
  font-size: 13px;
}

.digest-cell {
  font-family: var(--font-mono);
  font-size: 11px;
}

.plan-preview {
  margin-top: 16px;
  padding-top: 16px;
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

.diff-block {
  margin-top: 14px;
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

.plan-status {
  display: inline-flex;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
}

.plan-status.awaiting_confirmation { color: var(--status-warning); background: var(--warning-bg); }
.plan-status.executing { color: var(--status-info); background: var(--info-bg); }
.plan-status.succeeded { color: var(--status-success); background: var(--success-bg); }
.plan-status.failed { color: var(--status-danger); background: var(--danger-bg); }
.plan-status.expired { color: var(--text-secondary); background: var(--bg-tertiary); }

@media (max-width: 900px) {
  .form-grid { grid-template-columns: 1fr; }
  .chart-meta { grid-template-columns: 1fr; }
  .plan-summary { grid-template-columns: 1fr; }
}
</style>
