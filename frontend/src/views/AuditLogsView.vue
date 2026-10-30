<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Download, FileClock, RefreshCw, ShieldCheck, X } from 'lucide-vue-next'

import { exportAuditLogs, listAuditLogs } from '../api/audit'
import { listClusters } from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { AuditEntry, AuditResult } from '../types/audit'
import type { Cluster } from '../types/cluster'

const actions = ['auth.login', 'auth.refresh', 'auth.logout', 'cluster.create', 'cluster.enabled.update', 'cluster.probe', 'cluster.delete', 'diagnosis.run', 'diagnosis.status.update', 'diagnosis.feedback.create', 'diagnosis.assignment.update', 'diagnosis.ai_explanation.create', 'ai_explanation.feedback.create', 'audit.export']
const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const clusterID = ref(0)
const action = ref('')
const result = ref<AuditResult | ''>('')
const entries = ref<AuditEntry[]>([])
const total = ref(0)
const loading = ref(false)
const errorMessage = ref('')
const detail = ref<AuditEntry | null>(null)
const exporting = ref(false)
const exportMessage = ref('')
// 游标分页状态：nextBeforeID 由后端在「取满一页」时给出，缺省表示已到底。
const nextBeforeID = ref<number | null>(null)
const loadingMore = ref(false)

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

function resourceLabel(entry: AuditEntry): string {
  const path = [entry.resource.namespace, entry.resource.name].filter(Boolean).join('/')
  return [entry.resource.type, path].filter(Boolean).join(' · ') || '--'
}

async function loadEntries() {
  loading.value = true; errorMessage.value = ''
  try {
    const response = await listAuditLogs(auth.accessToken, { clusterID: clusterID.value || undefined, action: action.value, result: result.value })
    entries.value = response.items; total.value = response.total
    nextBeforeID.value = response.next_before_id ?? null
  } catch { errorMessage.value = '无法加载审计日志，请确认当前账号具有审计权限' }
  finally { loading.value = false }
}

// 加载更多：按游标追加下一页。审计表只追加，用 id 游标而不是页码，
// 避免翻页期间新记录插到最前导致相邻页重复或遗漏。
async function loadMore() {
  if (nextBeforeID.value === null || loadingMore.value) return
  loadingMore.value = true
  errorMessage.value = ''
  try {
    const response = await listAuditLogs(auth.accessToken, {
      clusterID: clusterID.value || undefined,
      action: action.value,
      result: result.value,
      beforeID: nextBeforeID.value,
    })
    // 去重兜底：极端情况下（同一毫秒多条）后端游标是开区间 id <，不会重复，
    // 但前端仍按 id 过滤一次，避免将来换实现时出现重复渲染。
    const seen = new Set(entries.value.map((item) => item.id))
    entries.value = [...entries.value, ...response.items.filter((item) => !seen.has(item.id))]
    total.value = response.total
    nextBeforeID.value = response.next_before_id ?? null
  } catch {
    errorMessage.value = '加载更多审计记录失败，请稍后重试'
  } finally {
    loadingMore.value = false
  }
}

async function initialize() {
  try { clusters.value = (await listClusters(auth.accessToken)).items }
  catch { errorMessage.value = '无法加载集群筛选项' }
  await loadEntries()
}

async function downloadExport() {
  exporting.value = true; errorMessage.value = ''; exportMessage.value = ''
  try {
    const exported = await exportAuditLogs(auth.accessToken, { clusterID: clusterID.value || undefined, action: action.value, result: result.value })
    const url = URL.createObjectURL(exported.blob)
    const link = document.createElement('a')
    link.href = url; link.download = exported.filename; link.click()
    URL.revokeObjectURL(url)
    exportMessage.value = exported.truncated ? `已导出前 ${exported.rows} 条，共 ${exported.total} 条；请增加筛选条件后再次导出。` : `已导出 ${exported.rows} 条审计记录。`
    await loadEntries()
  } catch { errorMessage.value = '审计 CSV 导出失败，请确认权限或稍后重试' }
  finally { exporting.value = false }
}

onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="安全与合规" title="审计日志">
    <section class="audit-toolbar">
      <select v-model="clusterID" aria-label="按集群筛选"><option :value="0">全部集群</option><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select>
      <select v-model="action" aria-label="按操作筛选"><option value="">全部操作</option><option v-for="item in actions" :key="item" :value="item">{{ item }}</option></select>
      <select v-model="result" aria-label="按结果筛选"><option value="">全部结果</option><option value="success">成功</option><option value="failure">失败</option><option value="denied">拒绝</option></select>
      <button class="secondary-button" type="button" :disabled="loading" @click="loadEntries"><RefreshCw :size="15" :class="{ spinning: loading }" />查询</button>
      <button class="secondary-button" type="button" :disabled="loading || exporting" @click="downloadExport"><Download :size="15" />{{ exporting ? '导出中' : '导出 CSV' }}</button>
    </section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="exportMessage" class="audit-export-message">{{ exportMessage }}</p>
    <section class="audit-list">
      <header><div><p class="context-label">IMMUTABLE AUDIT TRAIL</p><h2>操作记录 · {{ total }}</h2></div><ShieldCheck :size="21" /></header>
      <div v-if="!loading && entries.length === 0" class="resource-empty"><FileClock :size="30" /><strong>暂无审计记录</strong><span>平台关键写操作将在此处追加记录。</span></div>
      <button v-for="entry in entries" :key="entry.id" type="button" class="audit-row" @click="detail = entry"><span class="audit-action">{{ entry.action }}</span><span class="audit-resource"><strong>{{ resourceLabel(entry) }}</strong><small>{{ entry.actor.name || '匿名会话' }} · {{ entry.request_id }}</small></span><span class="audit-result" :class="entry.result">{{ entry.result }}</span><span class="audit-status">HTTP {{ entry.status_code }}</span><time>{{ formatTime(entry.created_at) }}</time></button>

      <div v-if="entries.length > 0" class="audit-pager">
        <span class="muted small">
          已加载 {{ entries.length }} / {{ total }} 条
        </span>
        <button
          v-if="nextBeforeID !== null"
          type="button"
          class="secondary-button"
          :disabled="loadingMore"
          @click="loadMore"
        >
          <RefreshCw :size="14" :class="{ spinning: loadingMore }" />
          {{ loadingMore ? '加载中…' : '加载更多' }}
        </button>
        <span v-else class="muted small">已到底</span>
      </div>
    </section>

    <div v-if="detail" class="log-overlay" @click.self="detail = null"><section class="audit-drawer"><header><div><p class="context-label">AUDIT #{{ detail.id }}</p><h2>{{ detail.action }}</h2></div><button class="icon-button" aria-label="关闭审计详情" @click="detail = null"><X :size="18" /></button></header><div class="audit-detail-summary"><span class="audit-result" :class="detail.result">{{ detail.result }}</span><strong>{{ resourceLabel(detail) }}</strong><time>{{ formatTime(detail.created_at) }}</time></div><dl><dt>操作者</dt><dd>{{ detail.actor.name || '匿名会话' }}<span v-if="detail.actor.id">#{{ detail.actor.id }}</span></dd><dt>请求 ID</dt><dd>{{ detail.request_id }}</dd><dt>HTTP 状态</dt><dd>{{ detail.status_code }}</dd><dt>来源地址</dt><dd>{{ detail.ip_address || '--' }}</dd><dt>User-Agent</dt><dd>{{ detail.user_agent || '--' }}</dd><dt>集群快照</dt><dd>{{ detail.details.cluster_id || detail.cluster_id || '--' }}</dd></dl><h3>非敏感详情</h3><pre>{{ JSON.stringify(detail.details, null, 2) }}</pre></section></div>
  </ConsoleLayout>
</template>
