<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RefreshCw, ShieldCheck, ShieldAlert, Server, X, AlertTriangle, CheckCircle2, Clock, Archive } from 'lucide-vue-next'

import * as k8sAPI from '../api/kubernetes'
import * as clusterAPI from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { VeleroBackup, VeleroCapability } from '../types/kubernetes'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const capability = ref<VeleroCapability | null>(null)
const namespace = ref('')
const name = ref('')
const backups = ref<VeleroBackup[]>([])
const loading = ref(true)
const errorMessage = ref('')
const selectedBackup = ref<VeleroBackup | null>(null)
let loadSequence = 0

const phaseLabels: Record<string, string> = {
  Completed: '已完成',
  PartiallyFailed: '部分失败',
  Failed: '失败',
  InProgress: '进行中',
  New: '待处理',
  Deleting: '删除中',
}

const phaseClasses: Record<string, string> = {
  Completed: 'running',
  PartiallyFailed: 'unknown',
  Failed: 'failed',
  InProgress: 'pending',
  New: 'pending',
  Deleting: 'pending',
}

const filteredBackups = computed(() => {
  if (!name.value) return backups.value
  const lower = name.value.toLowerCase()
  return backups.value.filter((b) => b.name.toLowerCase().includes(lower))
})

const completedCount = computed(() => backups.value.filter((b) => b.phase === 'Completed').length)
const failedCount = computed(() => backups.value.filter((b) => b.phase === 'Failed' || b.phase === 'PartiallyFailed').length)
const inProgressCount = computed(() => backups.value.filter((b) => b.phase === 'InProgress' || b.phase === 'New').length)

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

async function loadCapability() {
  if (!selectedClusterID.value) return
  capability.value = null
  try {
    capability.value = await k8sAPI.getVeleroCapability(auth.accessToken, selectedClusterID.value)
  } catch {
    capability.value = { installed: false }
  }
}

async function loadBackups() {
  if (!selectedClusterID.value || !capability.value?.installed) {
    backups.value = []
    return
  }
  loading.value = true
  errorMessage.value = ''
  const sequence = ++loadSequence
  try {
    const response = await k8sAPI.listBackups(auth.accessToken, selectedClusterID.value, namespace.value)
    if (sequence !== loadSequence) return
    backups.value = response.items
  } catch (error) {
    if (sequence !== loadSequence) return
    backups.value = []
    errorMessage.value = error instanceof Error ? error.message : '无法加载备份清单'
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function refresh() {
  await loadCapability()
  await loadBackups()
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

function openDetail(backup: VeleroBackup) {
  selectedBackup.value = backup
}

watch(selectedClusterID, () => {
  namespace.value = ''
  name.value = ''
  backups.value = []
  selectedBackup.value = null
  refresh()
})

watch(namespace, () => loadBackups())

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="跨集群治理" title="工作负载保护">
    <section class="page-toolbar">
      <div><strong>{{ backups.length }}</strong><span> 个备份记录</span></div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="loading || !selectedClusterID" @click="refresh"><RefreshCw :size="16" />刷新</button>
      </div>
    </section>

    <section class="resource-toolbar workload-protection-toolbar">
      <select v-model="selectedClusterID" :disabled="loading || clusters.length === 0" aria-label="选择集群">
        <option :value="null" disabled>选择集群…</option>
        <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <select v-model="namespace" :disabled="loading || !capability?.installed" aria-label="命名空间筛选">
        <option value="">全部命名空间</option>
        <option value="velero">velero</option>
        <option value="default">default</option>
      </select>
      <div class="search-field">
        <input v-model="name" placeholder="按备份名称搜索…" aria-label="搜索备份" />
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>

    <div v-if="clusters.length === 0 && !loading" class="resource-empty">
      <Server :size="30" />
      <strong>尚无可用集群</strong>
      <span>请先接入并启用一个 Kubernetes 集群</span>
    </div>

    <template v-else-if="selectedClusterID">
      <!-- Velero capability banner -->
      <section v-if="capability && !capability.installed" class="resource-empty velero-unavailable">
        <ShieldAlert :size="32" />
        <strong>Velero 未安装</strong>
        <span>该集群未检测到 Velero API（velero.io/v1）。安装 Velero 后即可查看备份清单。</span>
        <span class="velero-hint">平台不会将 Velero 列为核心依赖；未安装时此页面保持只读空态。</span>
      </section>

      <template v-else-if="capability?.installed">
        <!-- Summary cards -->
        <section class="resource-summary-grid">
          <article>
            <span>备份总数</span>
            <strong>{{ backups.length }}</strong>
          </article>
          <article>
            <span>已完成</span>
            <strong>{{ completedCount }}</strong>
          </article>
          <article>
            <span>失败/部分失败</span>
            <strong>{{ failedCount }}</strong>
          </article>
          <article>
            <span>进行中/待处理</span>
            <strong>{{ inProgressCount }}</strong>
          </article>
        </section>

        <!-- Backup inventory table -->
        <section class="resource-panel">
          <div class="section-heading">
            <div>
              <p class="context-label">Velero Backup 清单</p>
              <h2>备份记录</h2>
            </div>
            <ShieldCheck :size="20" />
          </div>

          <div v-if="loading" class="empty-state"><RefreshCw class="spinning" :size="24" /><span>正在加载备份清单</span></div>
          <div v-else-if="filteredBackups.length === 0" class="empty-state"><Archive :size="28" /><strong>暂无备份记录</strong><span>该集群/命名空间下未发现 Velero Backup CR</span></div>

          <div v-else class="pod-table-wrap">
            <table class="pod-table backup-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>命名空间</th>
                  <th>阶段</th>
                  <th>范围</th>
                  <th>存储位置</th>
                  <th>错误/警告</th>
                  <th>开始时间</th>
                  <th>过期时间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="backup in filteredBackups" :key="backup.namespace + '/' + backup.name" class="resource-row" @click="openDetail(backup)">
                  <td><strong>{{ backup.name }}</strong></td>
                  <td><span>{{ backup.namespace }}</span></td>
                  <td><span class="resource-status" :class="phaseClasses[backup.phase] || 'unknown'">{{ phaseLabels[backup.phase] || backup.phase }}</span></td>
                  <td><span>{{ backup.included_namespaces?.join(', ') || '—' }}</span></td>
                  <td><span>{{ backup.storage_location || '—' }}</span></td>
                  <td>
                    <span v-if="backup.errors > 0" class="backup-error-count">{{ backup.errors }} 错误</span>
                    <span v-if="backup.warnings > 0" class="backup-warning-count">{{ backup.warnings }} 警告</span>
                    <span v-if="backup.errors === 0 && backup.warnings === 0">—</span>
                  </td>
                  <td><span>{{ formatTimestamp(backup.started_at) }}</span></td>
                  <td><span>{{ formatTimestamp(backup.expiration) }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>
    </template>

    <!-- Backup detail drawer -->
    <div v-if="selectedBackup" class="log-overlay" @click.self="selectedBackup = null">
      <section class="resource-detail-drawer backup-detail-drawer">
        <header class="detail-header">
          <div>
            <p class="context-label">Velero Backup 详情</p>
            <h2>{{ selectedBackup.name }}</h2>
            <span>{{ selectedBackup.namespace }}</span>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" @click="selectedBackup = null"><X :size="18" /></button>
        </header>

        <div class="detail-body">
          <div class="detail-status-row">
            <span class="resource-status" :class="phaseClasses[selectedBackup.phase] || 'unknown'">{{ phaseLabels[selectedBackup.phase] || selectedBackup.phase }}</span>
            <span v-if="selectedBackup.failure_reason" class="detail-failure-reason"><AlertTriangle :size="14" />{{ selectedBackup.failure_reason }}</span>
          </div>

          <div class="detail-grid">
            <div class="detail-field">
              <label>范围 (Included Namespaces)</label>
              <span>{{ selectedBackup.included_namespaces?.join(', ') || '—' }}</span>
            </div>
            <div class="detail-field">
              <label>存储位置</label>
              <span>{{ selectedBackup.storage_location || '—' }}</span>
            </div>
            <div class="detail-field">
              <label>TTL</label>
              <span>{{ selectedBackup.ttl || '—' }}</span>
            </div>
            <div class="detail-field">
              <label>创建时间</label>
              <span>{{ formatTimestamp(selectedBackup.created_at) }}</span>
            </div>
            <div class="detail-field">
              <label>开始时间</label>
              <span><Clock :size="13" /> {{ formatTimestamp(selectedBackup.started_at) }}</span>
            </div>
            <div class="detail-field">
              <label>完成时间</label>
              <span><CheckCircle2 :size="13" /> {{ formatTimestamp(selectedBackup.completed_at) }}</span>
            </div>
            <div class="detail-field">
              <label>过期时间</label>
              <span>{{ formatTimestamp(selectedBackup.expiration) }}</span>
            </div>
            <div class="detail-field">
              <label>错误 / 警告</label>
              <span><strong>{{ selectedBackup.errors }}</strong> / <strong>{{ selectedBackup.warnings }}</strong></span>
            </div>
          </div>

          <div class="detail-notice">
            <ShieldCheck :size="16" />
            <span>此页面为只读视图。受控备份创建需在只读清单和 real-kind 兼容性验证通过后另行启用。恢复操作目前保持禁用。</span>
          </div>
        </div>
      </section>
    </div>
  </ConsoleLayout>
</template>
