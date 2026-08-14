<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Download, FileText, MessageSquareText, Plus, RefreshCw, UserCheck, UserPlus, X } from 'lucide-vue-next'

import { addIncidentFollower, addIncidentNote, assignIncident, batchAssignIncidents, createIncident, exportIncidentPostmortem, getIncident, getIncidentContext, getIncidentEvidence, getIncidentRunbook, getIncidentSummary, listIncidentResponseCatalog, listIncidents, removeIncidentFollower, setIncidentPostmortem, severityLabels, statusLabels, transitionIncident } from '../api/incidents'
import { APIError } from '../api/auth'
import { listAssignableUsers } from '../api/users'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { UserProfile } from '../types/auth'
import type { Incident, IncidentContextCockpit, IncidentCreateInput, IncidentEvidenceItem, IncidentResourceRef, IncidentResponseCatalog, IncidentRunbookResponse, IncidentSeverity, IncidentSourceType, IncidentStatus, IncidentSummary } from '../types/incident'

interface IncidentCreateForm extends Omit<IncidentCreateInput, 'resource'> {
  resource: IncidentResourceRef
}

const auth = useAuthStore()
const incidents = ref<Incident[]>([])
const summary = ref<IncidentSummary | null>(null)
const statusFilter = ref<IncidentStatus | ''>('')
const loading = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const selected = ref<number[]>([])
const batchAssigneeID = ref(0)
const batchComment = ref('')
const timelineFilter = ref<'all' | 'note' | 'system'>('all')
const evidenceFilter = ref<IncidentSourceType | 'all'>('all')

const detail = ref<Incident | null>(null)
const detailLoading = ref(false)
const evidence = ref<IncidentEvidenceItem[]>([])
const evidenceLoading = ref(false)
const runbook = ref<IncidentRunbookResponse | null>(null)
const runbookLoading = ref(false)
const cockpit = ref<IncidentContextCockpit | null>(null)
const cockpitLoading = ref(false)
const showCreate = ref(false)
const saving = ref(false)
const responseCatalog = ref<IncidentResponseCatalog | null>(null)

const newIncident = ref<IncidentCreateForm>({
  source_type: 'finding',
  source_ref: '',
  template_id: 'generic',
  cluster_id: 0,
  title: '',
  severity: 'warning',
  summary: '',
  resource: { kind: '', namespace: '', name: '' },
})

const assignableUsers = ref<UserProfile[]>([])
const assignmentUserID = ref(0)
const assignmentComment = ref('')
const transitionStatus = ref<IncidentStatus>('confirmed')
const transitionComment = ref('')
const followerUserID = ref(0)
const noteContent = ref('')
const postmortemContent = ref('')

const canManage = computed(() => auth.user?.roles.some((role) => role === 'system_admin' || role === 'operations_admin') ?? false)
const allSelected = computed(() => incidents.value.length > 0 && incidents.value.every((incident) => selected.value.includes(incident.id)))

const noteCount = computed(() => (detail.value?.timeline ?? []).filter((event) => event.event_type === 'note').length)
const systemCount = computed(() => (detail.value?.timeline ?? []).filter((event) => event.event_type === 'system').length)
const filteredTimeline = computed(() => {
  const events = detail.value?.timeline ?? []
  if (timelineFilter.value === 'all') return events
  return events.filter((event) => event.event_type === timelineFilter.value)
})
const evidenceSourceCounts = computed<Partial<Record<IncidentSourceType, number>>>(() => {
  const counts: Partial<Record<IncidentSourceType, number>> = {}
  for (const item of evidence.value) counts[item.source_type] = (counts[item.source_type] ?? 0) + 1
  return counts
})
const evidenceSourceCount = computed(() => Object.keys(evidenceSourceCounts.value).length)
const filteredEvidence = computed(() => {
  if (evidenceFilter.value === 'all') return evidence.value
  return evidence.value.filter((item) => item.source_type === evidenceFilter.value)
})
const resolutionDuration = computed(() => {
  if (!detail.value?.resolved_at) return '--'
  const milliseconds = new Date(detail.value.resolved_at).getTime() - new Date(detail.value.observed_at).getTime()
  if (Number.isNaN(milliseconds) || milliseconds < 0) return '--'
  const minutes = Math.round(milliseconds / 60000)
  const hours = Math.floor(minutes / 60)
  return hours > 0 ? `${hours}小时${minutes % 60}分` : `${minutes}分钟`
})
const linkedRunbook = computed(() => (runbook.value?.available ? runbook.value.runbook ?? null : null))
const cockpitFormatAge = computed(() => {
  const seconds = cockpit.value?.resource_context.freshness.age_seconds ?? 0
  if (seconds < 60) return `${seconds}秒`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes}分钟`
  const hours = Math.floor(minutes / 60)
  return `${hours}小时${minutes % 60}分`
})
const compatibleTemplates = computed(() => responseCatalog.value?.templates.filter((template) => template.source_types.includes(newIncident.value.source_type)) ?? [])
const selectedTemplate = computed(() => compatibleTemplates.value.find((template) => template.id === newIncident.value.template_id) ?? compatibleTemplates.value[0])
const selectedSlaTarget = computed(() => responseCatalog.value?.severity_matrix.find((target) => target.severity === newIncident.value.severity)?.target_minutes ?? null)

function toggleSelect(id: number) {
  selected.value = selected.value.includes(id) ? selected.value.filter((item) => item !== id) : [...selected.value, id]
}

function toggleSelectAll() {
  selected.value = allSelected.value ? selected.value.filter((id) => !incidents.value.some((incident) => incident.id === id)) : [...selected.value, ...incidents.value.map((incident) => incident.id)]
}

function clearSelection() {
  selected.value = []
  batchAssigneeID.value = 0
  batchComment.value = ''
}

function openCreate() {
  showCreate.value = true
  if (!newIncident.value.template_id && compatibleTemplates.value.length > 0) {
    newIncident.value.template_id = compatibleTemplates.value[0].id
  }
}

function applyTemplateDefaults() {
  const template = selectedTemplate.value
  if (!template) return
  newIncident.value.template_id = template.id
  newIncident.value.title = template.default_title
  newIncident.value.summary = template.default_summary
  newIncident.value.severity = template.default_severity
}

function ensureTemplateForSource() {
  if (compatibleTemplates.value.some((template) => template.id === newIncident.value.template_id)) return
  newIncident.value.template_id = compatibleTemplates.value[0]?.id ?? ''
}
const sourceRefPlaceholder = computed(() => {
  switch (newIncident.value.source_type) {
    case 'diagnosis': return 'diagnosis:<id>'
    case 'alert': return 'alert:<告警实例ID>'
    case 'inspection': return 'inspection:<巡检结果ID>'
    case 'signal': return 'signal:<信号实例ID>'
    default: return 'finding:<cluster>:<code>:<kind>:<ns>:<name>'
  }
})

function severityTone(severity: IncidentSeverity): string {
  return { info: 'info', warning: 'warning', high: 'danger', critical: 'danger' }[severity]
}

function sourceTypeLabel(sourceType: Incident['source_type']): string {
  return { diagnosis: '诊断记录', finding: '人工上报', alert: '告警实例', inspection: '巡检结果', signal: '信号实例', correlation: '关联案例' }[sourceType]
}

function runbookReasonLabel(reason?: IncidentRunbookResponse['reason']): string {
  return {
    source_resolver_unavailable: '当前环境未启用来源解析器',
    source_unavailable: '原始来源已不可读取，已停止推断',
    domain_unavailable: '原始来源没有可信的故障域信息',
  }[reason ?? 'domain_unavailable']
}

function evidenceSourceLabel(sourceType: IncidentEvidenceItem['source_type']): string {
  return { diagnosis: '诊断记录', finding: '人工上报', alert: '告警实例', inspection: '巡检结果', signal: '信号实例', correlation: '关联案例' }[sourceType]
}

function formatTime(value?: string): string {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date)
}

const SLA_APPROACHING_WINDOW_MS = 15 * 60 * 1000

function slaLabel(incident: Incident): string {
  if (incident.status === 'resolved' || incident.status === 'dismissed') return 'SLA 已关闭'
  const milliseconds = new Date(incident.sla_due_at).getTime() - Date.now()
  const absoluteMinutes = Math.max(1, Math.round(Math.abs(milliseconds) / 60000))
  const hours = Math.floor(absoluteMinutes / 60)
  const minutes = absoluteMinutes % 60
  const duration = hours > 0 ? `${hours}小时${minutes > 0 ? `${minutes}分` : ''}` : `${minutes}分钟`
  return milliseconds < 0 ? `已逾期 ${duration}` : `剩余 ${duration}`
}

function slaTone(incident: Incident): string {
  if (incident.status === 'resolved' || incident.status === 'dismissed') return ''
  const remaining = new Date(incident.sla_due_at).getTime() - Date.now()
  if (remaining < 0) return 'overdue'
  if (remaining <= SLA_APPROACHING_WINDOW_MS) return 'approaching'
  return ''
}

const allowedTransitions = computed<IncidentStatus[]>(() => {
  if (!detail.value) return []
  switch (detail.value.status) {
    case 'open': return ['confirmed', 'dismissed']
    case 'confirmed': return ['resolved', 'dismissed']
    case 'resolved': return ['open']
    case 'dismissed': return ['open']
    default: return []
  }
})

async function loadAll() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [list, board] = await Promise.all([
      listIncidents(auth.accessToken, { status: statusFilter.value }),
      getIncidentSummary(auth.accessToken),
    ])
    incidents.value = list.items
    summary.value = board
    if (canManage.value && assignableUsers.value.length === 0) {
      assignableUsers.value = (await listAssignableUsers(auth.accessToken)).items
    }
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '加载事故工作空间失败'
  } finally {
    loading.value = false
  }
}

async function loadResponseCatalog() {
  try {
    responseCatalog.value = await listIncidentResponseCatalog(auth.accessToken)
    applyTemplateDefaults()
  } catch {
    responseCatalog.value = null
  }
}

async function openDetail(incident: Incident) {
  detailLoading.value = true
  errorMessage.value = ''
  try {
    detail.value = await getIncident(auth.accessToken, incident.id)
    timelineFilter.value = 'all'
    evidenceFilter.value = 'all'
    transitionStatus.value = allowedTransitions.value[0] ?? 'confirmed'
    transitionComment.value = ''
    noteContent.value = ''
    postmortemContent.value = detail.value.postmortem ?? ''
    runbook.value = null
    cockpit.value = null
    if (canManage.value && assignableUsers.value.length === 0) {
      assignableUsers.value = (await listAssignableUsers(auth.accessToken)).items
    }
    void loadEvidence(detail.value.id)
    void loadRunbook(detail.value.id)
    void loadCockpit(detail.value.id)
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '加载事故详情失败'
  } finally {
    detailLoading.value = false
  }
}

async function handleCreate() {
  saving.value = true
  errorMessage.value = ''
  try {
    await createIncident(auth.accessToken, newIncident.value)
    showCreate.value = false
    newIncident.value = { source_type: 'finding', source_ref: '', template_id: 'generic', cluster_id: 0, title: '', severity: 'warning', summary: '', resource: { kind: '', namespace: '', name: '' } }
    await loadAll()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '创建事故失败'
  } finally {
    saving.value = false
  }
}

async function handleTransition() {
  if (!detail.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await transitionIncident(auth.accessToken, detail.value.id, detail.value.version, transitionStatus.value, transitionComment.value.trim())
    transitionComment.value = ''
    transitionStatus.value = allowedTransitions.value[0] ?? 'confirmed'
    await loadAll()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '状态变更失败'
  } finally {
    saving.value = false
  }
}

async function handleAssign() {
  if (!detail.value || assignmentUserID.value <= 0) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await assignIncident(auth.accessToken, detail.value.id, detail.value.version, assignmentUserID.value, assignmentComment.value.trim())
    assignmentComment.value = ''
    await loadAll()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '移交失败'
  } finally {
    saving.value = false
  }
}

async function handleBatchAssign() {
  if (!selected.value.length || batchAssigneeID.value <= 0) return
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const result = await batchAssignIncidents(auth.accessToken, { incident_ids: [...selected.value], assignee_user_id: batchAssigneeID.value, comment: batchComment.value.trim() })
    if (result.failed?.length) {
      errorMessage.value = `批量指派完成：成功 ${result.assigned}/${result.total}，失败 ${result.failed.length} 个（${result.failed.map((item) => `#${item.incident_id}:${item.error}`).join('，')}）`
    } else {
      successMessage.value = `已批量指派 ${result.assigned} 个事故。`
    }
    clearSelection()
    await loadAll()
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '批量指派失败'
  } finally {
    saving.value = false
  }
}

async function handleAddFollower() {
  if (!detail.value || followerUserID.value <= 0) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await addIncidentFollower(auth.accessToken, detail.value.id, followerUserID.value)
    followerUserID.value = 0
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '添加关注失败'
  } finally {
    saving.value = false
  }
}

async function handleRemoveFollower(userID: number) {
  if (!detail.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await removeIncidentFollower(auth.accessToken, detail.value.id, userID)
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '取消关注失败'
  } finally {
    saving.value = false
  }
}

async function handleAddNote() {
  if (!detail.value || !noteContent.value.trim()) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await addIncidentNote(auth.accessToken, detail.value.id, detail.value.version, noteContent.value.trim())
    noteContent.value = ''
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '添加备注失败'
  } finally {
    saving.value = false
  }
}

async function handlePostmortem() {
  if (!detail.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    detail.value = await setIncidentPostmortem(auth.accessToken, detail.value.id, detail.value.version, postmortemContent.value.trim())
  } catch (err) {
    errorMessage.value = err instanceof APIError ? err.message : '保存复盘失败'
  } finally {
    saving.value = false
  }
}

async function handlePostmortemExport() {
  if (!detail.value) return
  try {
    const exported = await exportIncidentPostmortem(auth.accessToken, detail.value.id)
    const url = URL.createObjectURL(exported.blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = exported.filename
    anchor.click()
    URL.revokeObjectURL(url)
    successMessage.value = `已导出 ${detail.value.number} 复盘 Markdown。`
  } catch {
    errorMessage.value = '复盘导出失败'
  }
}

async function handleExport(incident: Incident) {
  try {
    const response = await fetch(`/api/v1/incidents/${incident.id}/export`, { headers: { Authorization: `Bearer ${auth.accessToken}` } })
    if (response.ok) {
      const blob = await response.blob()
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `incident-${incident.number}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
    } else {
      errorMessage.value = '导出失败'
    }
  } catch {
    errorMessage.value = '导出失败'
  }
}

async function loadEvidence(incidentID: number) {
  evidenceLoading.value = true
  try {
    const result = await getIncidentEvidence(auth.accessToken, incidentID)
    evidence.value = result.items
  } catch {
    evidence.value = []
  } finally {
    evidenceLoading.value = false
  }
}

async function loadRunbook(incidentID: number) {
  runbookLoading.value = true
  try {
    const result = await getIncidentRunbook(auth.accessToken, incidentID)
    if (detail.value?.id === incidentID) runbook.value = result
  } catch {
    if (detail.value?.id === incidentID) runbook.value = { incident_id: incidentID, available: false, reason: 'source_unavailable' }
  } finally {
    if (detail.value?.id === incidentID) runbookLoading.value = false
  }
}

async function loadCockpit(incidentID: number) {
  cockpitLoading.value = true
  try {
    const result = await getIncidentContext(auth.accessToken, incidentID)
    if (detail.value?.id === incidentID) cockpit.value = result
  } catch {
    if (detail.value?.id === incidentID) cockpit.value = null
  } finally {
    if (detail.value?.id === incidentID) cockpitLoading.value = false
  }
}

onMounted(() => { void loadAll(); void loadResponseCatalog() })
</script>

<template>
  <ConsoleLayout eyebrow="分析与治理" title="事故工作空间">
    <template #actions>
      <button class="icon-button" type="button" title="刷新" aria-label="刷新事故列表" :disabled="loading" @click="loadAll">
        <RefreshCw :size="18" :class="{ spinning: loading }" />
      </button>
    </template>

    <section class="incident-toolbar">
      <div>
        <strong>事故工作空间</strong>
        <span>围绕诊断或人工上报的事故，进行确认、移交、跟踪与复盘。</span>
      </div>
      <div class="toolbar-actions">
        <select v-model="statusFilter" aria-label="按状态筛选" @change="loadAll">
          <option value="">全部状态</option>
          <option value="open">待确认</option>
          <option value="confirmed">已确认</option>
          <option value="resolved">已解决</option>
          <option value="dismissed">已驳回</option>
        </select>
        <button v-if="canManage" class="primary-button" type="button" :disabled="saving" @click="openCreate">
          <Plus :size="15" />新建事故
        </button>
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="successMessage" class="audit-export-message">{{ successMessage }}</p>

    <section v-if="summary" class="incident-stats" aria-label="事故统计">
      <article class="stat-card"><strong>{{ summary.total }}</strong><span>全部</span></article>
      <article class="stat-card"><strong>{{ summary.open }}</strong><span>待确认</span></article>
      <article class="stat-card"><strong>{{ summary.confirmed }}</strong><span>已确认</span></article>
      <article class="stat-card"><strong>{{ summary.resolved }}</strong><span>已解决</span></article>
      <article class="stat-card"><strong>{{ summary.dismissed }}</strong><span>已驳回</span></article>
      <article class="stat-card stat-overdue"><strong>{{ summary.overdue }}</strong><span>已逾期</span></article>
    </section>

    <section v-if="canManage && selected.length > 0" class="batch-toolbar" aria-label="批量指派">
      <span class="batch-count"><strong>{{ selected.length }}</strong> 个事故已选</span>
      <select v-model.number="batchAssigneeID" aria-label="选择批量负责人">
        <option :value="0" disabled>选择负责人</option>
        <option v-for="u in assignableUsers" :key="u.id" :value="u.id">{{ u.display_name }}</option>
      </select>
      <input v-model="batchComment" maxlength="2000" placeholder="批量移交说明（可选）" />
      <button class="primary-button" type="button" :disabled="saving || batchAssigneeID <= 0" @click="handleBatchAssign">
        <UserCheck :size="14" />批量指派
      </button>
      <button class="text-button" type="button" :disabled="saving" @click="clearSelection">取消选择</button>
    </section>

    <section class="incident-list">
      <div v-if="!loading && incidents.length === 0" class="resource-empty">
        <MessageSquareText :size="30" />
        <strong>暂无事故工作空间</strong>
        <span>事故会从诊断记录或人工上报自动汇集到这里。</span>
      </div>
      <div v-else class="incident-table-scroll">
      <table class="compact-table">
        <thead>
          <tr>
            <th v-if="canManage" class="select-cell">
              <input type="checkbox" aria-label="全选当前页" :checked="allSelected" @change="toggleSelectAll" />
            </th>
            <th>编号</th>
            <th>标题</th>
            <th>级别</th>
            <th>状态</th>
            <th>负责人</th>
            <th>集群</th>
            <th>SLA</th>
            <th>更新时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="incident in incidents" :key="incident.id" :class="{ 'row-selected': selected.includes(incident.id) }">
            <td v-if="canManage" class="select-cell">
              <input type="checkbox" :aria-label="'选择 ' + incident.number" :checked="selected.includes(incident.id)" @change="toggleSelect(incident.id)" />
            </td>
            <td><strong>{{ incident.number }}</strong></td>
            <td>
              <button class="incident-title" type="button" :disabled="detailLoading" @click="openDetail(incident)">{{ incident.title }}</button>
            </td>
            <td><span class="severity-badge" :class="severityTone(incident.severity)">{{ severityLabels[incident.severity] }}</span></td>
            <td><span class="workflow-status" :class="incident.status">{{ statusLabels[incident.status] }}</span></td>
            <td>{{ incident.assignee?.name ?? '未指派' }}</td>
            <td>#{{ incident.cluster_id }}</td>
            <td><span class="sla-badge" :class="slaTone(incident)">{{ slaLabel(incident) }}</span></td>
            <td>{{ formatTime(incident.updated_at) }}</td>
            <td>
              <button class="icon-button compact" type="button" title="导出 CSV" aria-label="导出 CSV" @click="handleExport(incident)"><Download :size="15" /></button>
            </td>
          </tr>
        </tbody>
      </table>
      </div>
    </section>

    <div v-if="detail" class="log-overlay" @click.self="detail = null">
      <section class="incident-drawer">
        <header>
          <div>
            <p class="context-label">INCIDENT {{ detail.number }}</p>
            <h2>{{ detail.title }}</h2>
          </div>
          <button class="icon-button" type="button" aria-label="关闭事故详情" @click="detail = null"><X :size="18" /></button>
        </header>

        <div class="incident-badges">
          <span class="severity-badge" :class="severityTone(detail.severity)">{{ severityLabels[detail.severity] }}</span>
          <span class="workflow-status" :class="detail.status">{{ statusLabels[detail.status] }}</span>
          <span class="sla-badge" :class="slaTone(detail)">{{ slaLabel(detail) }}</span>
          <span class="assignee-badge" v-if="detail.assignee"><UserCheck :size="13" />{{ detail.assignee.name }}</span>
          <span class="version-badge">v{{ detail.version }}</span>
        </div>

        <p class="incident-summary">{{ detail.summary || '暂无摘要' }}</p>

        <dl class="incident-meta">
          <div><dt>来源</dt><dd>{{ sourceTypeLabel(detail.source_type) }} · {{ detail.source_ref }}</dd></div>
          <div><dt>资源</dt><dd>{{ detail.resource.kind }} {{ detail.resource.namespace ? detail.resource.namespace + '/' : '' }}{{ detail.resource.name }}</dd></div>
          <div><dt>SLA 截止</dt><dd>{{ formatTime(detail.sla_due_at) }}</dd></div>
          <div><dt>创建时间</dt><dd>{{ formatTime(detail.created_at) }}</dd></div>
        </dl>

        <h3>上下文驾驶舱</h3>
        <p v-if="cockpitLoading" class="muted">正在组装上下文…</p>
        <div v-else-if="cockpit" class="incident-cockpit">
          <div class="cockpit-contract">
            <span class="cockpit-tag">scope {{ cockpit.resource_context.scope.cluster_id ? '#' + cockpit.resource_context.scope.cluster_id : '-' }}</span>
            <span class="cockpit-tag">数据源 {{ cockpit.resource_context.source }}</span>
            <span class="cockpit-tag">freshness {{ cockpitFormatAge }}</span>
            <span class="cockpit-tag">空样本 fail_closed</span>
          </div>
          <div class="cockpit-health">
            <span><strong>{{ statusLabels[cockpit.health.status] }}</strong><small>状态</small></span>
            <span><strong class="sla-tone" :class="{ overdue: cockpit.health.overdue }">{{ cockpit.sla.deadline_text }}</strong><small>SLA</small></span>
            <span><strong>{{ cockpit.evidence_sources.length }}</strong><small>证据来源</small></span>
            <span><strong>{{ cockpit.health.note_count }}</strong><small>备注</small></span>
            <span><strong>{{ cockpit.health.system_event_count }}</strong><small>系统事件</small></span>
          </div>
          <div v-if="cockpit.evidence_sources.length" class="cockpit-evidence">
            <a v-for="src in cockpit.evidence_sources" :key="src.source_type" class="cockpit-source-link" :href="src.deep_link">
              {{ evidenceSourceLabel(src.source_type) }} × {{ src.count }} ↗
            </a>
          </div>
          <div v-if="cockpit.recommended_actions.length" class="cockpit-actions">
            <strong>建议动作（只读 dry-run）</strong>
            <div v-for="action in cockpit.recommended_actions" :key="action.action" class="cockpit-action">
              <span>{{ action.action }}</span>
              <small>{{ action.target_kind }} · {{ action.dry_run_first ? '必须先预览' : '受控操作' }}</small>
            </div>
          </div>
          <p v-if="!cockpit.health.runbook_available" class="muted">当前事故暂无可信的只读 runbook（来源域缺失或解析器未启用）。</p>
        </div>
        <p v-else class="muted">上下文驾驶舱加载失败（不影响其余区块）。</p>

        <h3>响应 Runbook</h3>
        <p v-if="runbookLoading" class="muted">正在解析响应步骤…</p>
        <div v-else-if="linkedRunbook" class="incident-runbook">
          <div class="runbook-headline">
            <span class="runbook-domain">{{ linkedRunbook.domain }}</span>
            <span v-if="linkedRunbook.finding_code" class="runbook-code">{{ linkedRunbook.finding_code }}</span>
            <span class="runbook-readonly">只读编排</span>
          </div>
          <div v-if="linkedRunbook.diagnoses.length" class="runbook-group">
            <strong>诊断入口</strong>
            <div v-for="route in linkedRunbook.diagnoses" :key="route.resource_kind" class="runbook-item">
              <span>{{ route.resource_kind }}</span>
              <p>{{ route.summary }}</p>
              <small>{{ route.rule_ids.join(' · ') }}</small>
            </div>
          </div>
          <div v-if="linkedRunbook.inspection.length" class="runbook-group">
            <strong>巡检校验</strong>
            <div v-for="rule in linkedRunbook.inspection" :key="rule.rule_code" class="runbook-item">
              <span>{{ rule.rule_code }}</span>
              <p>{{ rule.summary }}</p>
              <small>{{ rule.signal_code }}</small>
            </div>
          </div>
          <div v-if="linkedRunbook.ai_explanation" class="runbook-group">
            <strong>AI 解释入口</strong>
            <p class="runbook-note">{{ linkedRunbook.ai_explanation.summary }}</p>
            <code>{{ linkedRunbook.ai_explanation.endpoint }}</code>
          </div>
          <div v-if="linkedRunbook.operations?.length" class="runbook-group">
            <strong>Dry-run 候选</strong>
            <div v-for="operation in linkedRunbook.operations" :key="operation.action" class="runbook-item">
              <span>{{ operation.action }}</span>
              <p>{{ operation.summary }}</p>
              <small>{{ operation.dry_run_first ? '必须先预览' : '受控操作' }} · {{ operation.target_kind }}</small>
            </div>
          </div>
          <p v-if="!linkedRunbook.diagnoses.length && !linkedRunbook.inspection.length && !linkedRunbook.ai_explanation && !linkedRunbook.operations?.length" class="muted">当前资源没有可用的只读响应步骤。</p>
        </div>
        <p v-else class="runbook-unavailable">{{ runbookReasonLabel(runbook?.reason) }}</p>

        <template v-if="evidenceLoading">
          <h3>证据时间线</h3>
          <p class="muted">正在加载证据…</p>
        </template>
        <template v-else-if="evidence.length">
          <h3>证据时间线</h3>
          <div v-if="evidenceSourceCount > 1" class="filter-tabs" role="tablist" aria-label="证据来源过滤">
            <button type="button" :class="{ active: evidenceFilter === 'all' }" @click="evidenceFilter = 'all'">全部 ({{ evidence.length }})</button>
            <button v-for="(count, source) in evidenceSourceCounts" :key="source" type="button" :class="{ active: evidenceFilter === source }" @click="evidenceFilter = source">{{ evidenceSourceLabel(source) }} ({{ count }})</button>
          </div>
          <div class="incident-evidence">
            <div v-for="item in filteredEvidence" :key="item.source_ref" class="evidence-card">
              <div class="evidence-card-head">
                <span class="evidence-source">{{ evidenceSourceLabel(item.source_type) }}</span>
                <span class="severity-badge" :class="severityTone(item.severity ?? detail.severity)">{{ severityLabels[item.severity ?? detail.severity] }}</span>
                <a v-if="item.deep_link" class="evidence-link" :href="item.deep_link">查看原始证据 ↗</a>
              </div>
              <div class="evidence-title">{{ item.title }}</div>
              <p v-if="item.summary" class="evidence-summary">{{ item.summary }}</p>
              <dl v-if="item.fields?.length" class="evidence-fields">
                <div v-for="f in item.fields" :key="f.label + ':' + f.value">
                  <dt>{{ f.label }}</dt>
                  <dd>{{ f.value }}</dd>
                </div>
              </dl>
              <div v-if="item.resource" class="evidence-resource">{{ item.resource.kind }} {{ item.resource.namespace ? item.resource.namespace + '/' : '' }}{{ item.resource.name }}</div>
              <div class="evidence-src">来源 {{ item.source_ref }}<span v-if="item.observed_at"> · {{ formatTime(item.observed_at) }}</span></div>
            </div>
          </div>
        </template>

        <template v-if="canManage">
          <h3>状态变更</h3>
          <div v-if="allowedTransitions.length" class="action-row">
            <select v-model="transitionStatus" aria-label="目标状态">
              <option v-for="s in allowedTransitions" :key="s" :value="s">{{ statusLabels[s] }}</option>
            </select>
            <input v-model="transitionComment" maxlength="2000" placeholder="变更备注（可选）" />
            <button class="secondary-button" type="button" :disabled="saving" @click="handleTransition">提交</button>
          </div>
          <span v-else class="muted">当前状态无可用变更</span>

          <h3>移交负责人</h3>
          <div class="action-row">
            <select v-model="assignmentUserID" aria-label="选择负责人">
              <option :value="0" disabled>选择负责人…</option>
              <option v-for="u in assignableUsers" :key="u.id" :value="u.id">{{ u.display_name }}</option>
            </select>
            <input v-model="assignmentComment" maxlength="2000" placeholder="移交说明（可选）" />
            <button class="secondary-button" type="button" :disabled="saving || assignmentUserID <= 0" @click="handleAssign">移交</button>
          </div>

          <h3>关注者</h3>
          <div class="action-row">
            <select v-model="followerUserID" aria-label="选择关注用户">
              <option :value="0" disabled>选择用户…</option>
              <option v-for="u in assignableUsers" :key="u.id" :value="u.id">{{ u.display_name }}</option>
            </select>
            <button class="secondary-button" type="button" :disabled="saving || followerUserID <= 0" @click="handleAddFollower"><UserPlus :size="14" />关注</button>
          </div>
          <div v-if="detail.followers?.length" class="follower-list">
            <span v-for="f in detail.followers" :key="f.user_id" class="follower-tag">
              {{ f.name }}
              <button v-if="canManage" type="button" class="follower-remove" :disabled="saving" aria-label="取消关注" @click="handleRemoveFollower(f.user_id)"><X :size="11" /></button>
            </span>
          </div>
          <span v-else class="muted">暂无关注者</span>
        </template>

        <h3>时间线</h3>
        <div class="filter-tabs" role="tablist" aria-label="时间线过滤">
          <button type="button" :class="{ active: timelineFilter === 'all' }" @click="timelineFilter = 'all'">全部 ({{ detail.timeline?.length ?? 0 }})</button>
          <button type="button" :class="{ active: timelineFilter === 'note' }" @click="timelineFilter = 'note'">备注 ({{ noteCount }})</button>
          <button type="button" :class="{ active: timelineFilter === 'system' }" @click="timelineFilter = 'system'">系统 ({{ systemCount }})</button>
        </div>
        <ol class="incident-timeline">
          <li v-for="event in filteredTimeline" :key="event.id">
            <span class="timeline-marker" :class="event.event_type" />
            <div>
              <div class="timeline-head">
                <span class="timeline-kind" :class="event.event_type">{{ event.event_type === 'note' ? '备注' : '系统' }}</span>
                <span class="timeline-actor">{{ event.actor.name || 'system' }}</span>
                <time>{{ formatTime(event.created_at) }}</time>
              </div>
              <p class="timeline-content">{{ event.content }}</p>
            </div>
          </li>
        </ol>

        <div v-if="canManage" class="action-row note-row">
          <textarea v-model="noteContent" rows="2" maxlength="4000" placeholder="记录新的备注…" />
          <button class="secondary-button" type="button" :disabled="saving || !noteContent.trim()" @click="handleAddNote">添加备注</button>
        </div>

        <template v-if="detail.status === 'resolved'">
          <h3>复盘视图</h3>
          <p v-if="detail.postmortem" class="postmortem-view">{{ detail.postmortem }}</p>
          <p v-else class="muted">尚未记录复盘结论。</p>
          <div class="postmortem-metrics">
            <span class="metric"><strong>{{ slaTone(detail) === 'overdue' ? 'SLA 逾期' : 'SLA 达标' }}</strong><small>结果</small></span>
            <span class="metric"><strong>{{ resolutionDuration }}</strong><small>解决耗时</small></span>
            <span class="metric"><strong>{{ systemCount }}</strong><small>系统事件</small></span>
            <span class="metric"><strong>{{ noteCount }}</strong><small>人工备注</small></span>
            <span class="metric"><strong>{{ evidenceSourceCount }}</strong><small>证据来源</small></span>
          </div>
          <textarea v-model="postmortemContent" rows="6" maxlength="10000" placeholder="记录根因、处理过程与后续改进…" />
          <div class="action-row">
            <button v-if="canManage" class="secondary-button" type="button" :disabled="saving" @click="handlePostmortem">保存复盘</button>
            <button class="secondary-button" type="button" :disabled="saving" @click="handlePostmortemExport"><FileText :size="14" />导出 Markdown</button>
          </div>
        </template>
      </section>
    </div>

    <div v-if="showCreate" class="log-overlay" @click.self="showCreate = false">
      <section class="incident-form">
        <header>
          <div>
            <p class="context-label">NEW INCIDENT</p>
            <h2>新建事故工作空间</h2>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" @click="showCreate = false"><X :size="18" /></button>
        </header>
        <label class="form-field">
          <span>来源类型</span>
          <select v-model="newIncident.source_type" aria-label="来源类型" @change="ensureTemplateForSource">
          <option value="finding">人工上报</option>
          <option value="diagnosis">诊断记录</option>
          <option value="alert">告警实例</option>
          <option value="inspection">巡检结果</option>
          <option value="signal">信号实例</option>
          </select>
        </label>
        <label class="form-field">
          <span>来源标识</span>
          <input v-model="newIncident.source_ref" :placeholder="sourceRefPlaceholder" />
        </label>
        <label v-if="compatibleTemplates.length" class="form-field">
          <span>响应模板</span>
          <select v-model="newIncident.template_id" aria-label="响应模板" @change="applyTemplateDefaults">
            <option v-for="template in compatibleTemplates" :key="template.id" :value="template.id">{{ template.name }}</option>
          </select>
          <small v-if="selectedTemplate" class="form-hint">{{ selectedTemplate.description }} · {{ selectedTemplate.steps.join(' · ') }}</small>
        </label>
        <label class="form-field">
          <span>集群 ID</span>
          <input v-model.number="newIncident.cluster_id" type="number" min="1" />
        </label>
        <label class="form-field">
          <span>标题</span>
          <input v-model="newIncident.title" maxlength="500" :disabled="['diagnosis','alert','inspection','signal'].includes(newIncident.source_type)" placeholder="诊断/告警/巡检/信号来源时自动填充" />
        </label>
        <label class="form-field">
          <span>严重级别</span>
          <select v-model="newIncident.severity" :disabled="['diagnosis','alert','inspection','signal'].includes(newIncident.source_type)" aria-label="严重级别">
            <option value="info">信息</option>
            <option value="warning">警告</option>
            <option value="high">高</option>
            <option value="critical">严重</option>
          </select>
          <small v-if="selectedSlaTarget" class="form-hint">当前矩阵目标：{{ selectedSlaTarget >= 1440 ? `${Math.round(selectedSlaTarget / 1440)} 天` : `${selectedSlaTarget} 分钟` }}</small>
        </label>
        <label class="form-field">
          <span>摘要</span>
          <textarea v-model="newIncident.summary" rows="2" maxlength="4000" :disabled="['diagnosis','alert','inspection','signal'].includes(newIncident.source_type)" />
        </label>
        <template v-if="newIncident.source_type === 'finding'">
          <label class="form-field">
            <span>资源类型</span>
            <input v-model="newIncident.resource.kind" placeholder="Pod / Service / Node…" />
          </label>
          <label class="form-field">
            <span>命名空间</span>
            <input v-model="newIncident.resource.namespace" />
          </label>
          <label class="form-field">
            <span>资源名称</span>
            <input v-model="newIncident.resource.name" />
          </label>
        </template>
        <div class="form-actions">
          <button class="secondary-button" type="button" @click="showCreate = false">取消</button>
          <button class="primary-button" type="button" :disabled="saving || !newIncident.source_ref || !newIncident.cluster_id" @click="handleCreate">创建</button>
        </div>
      </section>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.incident-toolbar {
  display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; flex-wrap: wrap;
  margin-bottom: 14px; padding: 14px 16px; background: var(--panel-bg, #ffffff); border: 1px solid #dfe5e8; border-radius: 8px;
}
.incident-toolbar > div:first-child { display: grid; gap: 4px; }
.incident-toolbar strong { color: #344149; font-size: 13px; }
.incident-toolbar span { color: #5a6672; font-size: 11px; }
.incident-toolbar select { min-width: 150px; height: 34px; padding: 0 10px; color: #43515a; background: #ffffff; border: 1px solid #cfd8dc; border-radius: 5px; }
.toolbar-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.primary-button {
  display: inline-flex; align-items: center; gap: 6px; height: 34px; padding: 0 14px; color: #ffffff;
  background: #326ce5; border: 0; border-radius: 5px; font-size: 12px; font-weight: 650; cursor: pointer;
}
.primary-button:disabled { opacity: 0.55; cursor: not-allowed; }
.incident-stats { display: flex; gap: 10px; margin-bottom: 14px; flex-wrap: wrap; }
.stat-card {
  display: flex; flex-direction: column; align-items: center; min-width: 88px; padding: 10px 14px;
  background: #ffffff; border: 1px solid #e3e8ea; border-radius: 8px;
}
.stat-card strong { color: #2f3d45; font-size: 20px; }
.stat-card span { color: #77858d; font-size: 11px; margin-top: 2px; }
.stat-card.stat-overdue { background: #fdf0ee; border-color: #f3c9c2; }
.stat-card.stat-overdue strong { color: #b13a2a; }
.batch-toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 10px 14px; margin: 10px 0; border: 1px dashed #b9c9e8; border-radius: 12px; background: #f2f6fc; }
.batch-toolbar .batch-count { font-size: 13px; color: #35598c; }
.batch-toolbar .batch-count strong { color: #1d4ed8; }
.batch-toolbar input[type='text'] { width: 260px; }
.batch-toolbar select { max-width: 200px; }
.incident-list tr.row-selected td { background: #eef4ff; }
.incident-list .select-cell { width: 34px; text-align: center; }
.incident-list { background: #ffffff; border: 1px solid #e3e8ea; border-radius: 8px; padding: 4px 14px 12px; }
.incident-title { padding: 0; color: #326ce5; background: none; border: 0; font-size: 12px; text-align: left; cursor: pointer; }
.incident-title:hover { text-decoration: underline; }
.incident-title:disabled { cursor: wait; }
.severity-badge.danger { color: #a43b2e; background: #fbeae6; }
.severity-badge.warning { color: #8c6225; background: #fff3d8; }
.severity-badge.info { color: #3b6ea5; background: #e6eef7; }
.workflow-status { display: inline-flex; padding: 3px 8px; border-radius: 10px; font-size: 11px; font-weight: 650; }
.workflow-status.open { color: #8c6225; background: #fff3d8; }
.workflow-status.confirmed { color: #3b6ea5; background: #e6eef7; }
.workflow-status.resolved { color: #2e7867; background: #e3f1ed; }
.workflow-status.dismissed { color: #5a6672; background: #eef1f3; }
.sla-badge { display: inline-flex; padding: 2px 7px; border-radius: 8px; font-size: 11px; color: #4c6c9b; background: #e8eef7; }
.sla-badge.overdue { color: #b13a2a; background: #fbe9e6; }
.sla-badge.approaching { color: #9a6700; background: #fdf3d7; }
.assignee-badge { display: inline-flex; align-items: center; gap: 4px; padding: 3px 8px; border-radius: 10px; font-size: 11px; color: #2e7867; background: #e3f1ed; }
.version-badge { display: inline-flex; padding: 3px 8px; border-radius: 10px; font-size: 11px; color: #69578a; background: #f0ecfa; }
.incident-drawer {
  width: min(640px, 92vw); height: 100%; padding: 22px; overflow-y: auto;
  background: #f8fafb; box-shadow: -12px 0 38px rgba(17, 28, 34, 0.18);
}
.incident-drawer header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.incident-drawer h2 { margin: 0; font-size: 18px; color: #2f3d45; }
.incident-badges { display: flex; align-items: center; gap: 6px; margin-top: 14px; flex-wrap: wrap; }
.incident-summary { margin: 12px 0; color: #46545c; font-size: 13px; line-height: 1.6; }
.incident-meta { margin: 0; display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px 16px; }
.incident-meta div { display: flex; gap: 8px; font-size: 12px; }
.incident-meta dt { color: #77858d; min-width: 68px; }
.incident-runbook { display: grid; gap: 10px; padding: 12px 14px; background: #f1f6fb; border: 1px solid #d8e3ed; border-radius: 8px; }
.runbook-headline { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.runbook-domain { color: #2f5f91; font-size: 12px; font-weight: 700; }
.runbook-code { color: #5d6d78; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 10px; overflow-wrap: anywhere; }
.runbook-readonly { margin-left: auto; padding: 3px 7px; color: #2e7867; background: #e3f1ed; border-radius: 8px; font-size: 10px; font-weight: 650; }
.runbook-group { display: grid; gap: 6px; }
.runbook-group > strong { color: #39474e; font-size: 11px; }
.runbook-item { display: grid; gap: 2px; padding: 8px 10px; background: #ffffff; border: 1px solid #e1e8ed; border-radius: 6px; }
.runbook-item > span { color: #3b6ea5; font-size: 12px; font-weight: 650; }
.runbook-item p, .runbook-note { margin: 0; color: #46545c; font-size: 11px; line-height: 1.5; }
.runbook-item small, .incident-runbook code { color: #77858d; font-size: 10px; overflow-wrap: anywhere; }
.runbook-unavailable { margin: 0; padding: 10px 12px; color: #7a5b25; background: #fff8e7; border: 1px solid #f0dfb5; border-radius: 6px; font-size: 12px; }
.incident-cockpit { display: grid; gap: 10px; padding: 12px 14px; background: #f5f8fc; border: 1px solid #dbe5ef; border-radius: 8px; }
.cockpit-contract { display: flex; gap: 6px; flex-wrap: wrap; }
.cockpit-tag { padding: 2px 8px; border-radius: 8px; font-size: 10px; font-weight: 650; color: #4c6c9b; background: #e8eef7; }
.cockpit-health { display: flex; gap: 10px; flex-wrap: wrap; }
.cockpit-health > span { display: inline-flex; flex-direction: column; gap: 2px; padding: 8px 12px; border-radius: 10px; background: #ffffff; border: 1px solid #e3e8ea; }
.cockpit-health strong { font-size: 14px; color: #2b3a55; }
.cockpit-health strong.sla-tone.overdue { color: #b13a2a; }
.cockpit-health small { font-size: 11px; color: #8593a8; }
.cockpit-evidence { display: flex; gap: 6px; flex-wrap: wrap; }
.cockpit-source-link { padding: 3px 9px; border-radius: 12px; font-size: 11px; color: #1d4ed8; background: #e4ecfc; text-decoration: none; }
.cockpit-source-link:hover { text-decoration: underline; }
.cockpit-actions { display: grid; gap: 6px; }
.cockpit-actions > strong { color: #39474e; font-size: 11px; }
.cockpit-action { display: flex; align-items: center; gap: 8px; padding: 7px 10px; background: #ffffff; border: 1px solid #e1e8ed; border-radius: 6px; }
.cockpit-action span { color: #3b6ea5; font-size: 12px; font-weight: 650; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.cockpit-action small { margin-left: auto; color: #77858d; font-size: 10px; }
.incident-evidence { display: grid; gap: 10px; }
.evidence-card { padding: 12px 14px; background: #f0f4f6; border: 1px solid #dce4e7; border-radius: 8px; }
.evidence-card-head { display: flex; align-items: center; gap: 8px; }
.evidence-source { font-size: 11px; font-weight: 650; color: #2e7867; text-transform: uppercase; }
.evidence-source { color: #3b6ea5; }
.evidence-link { margin-left: auto; font-size: 11px; color: #3b6ea5; text-decoration: none; }
.evidence-link:hover { text-decoration: underline; }
.evidence-title { margin: 8px 0 0; font-size: 14px; font-weight: 650; color: #2f3d45; }
.evidence-summary { margin: 4px 0 0; color: #46545c; font-size: 12px; line-height: 1.6; }
.evidence-fields { margin: 10px 0 0; display: grid; grid-template-columns: repeat(2, 1fr); gap: 4px 16px; }
.evidence-fields div { display: flex; gap: 8px; font-size: 12px; }
.evidence-fields dt { color: #77858d; min-width: 64px; }
.evidence-fields dd { margin: 0; color: #3d4b53; overflow-wrap: anywhere; }
.evidence-resource { margin: 10px 0 0; font-size: 12px; color: #3d4b53; }
.evidence-src { margin-top: 8px; font-size: 11px; color: #99a4ab; overflow-wrap: anywhere; }
.incident-meta dd { margin: 0; color: #3d4b53; overflow-wrap: anywhere; }
.incident-drawer h3 { margin: 22px 0 10px; color: #39474e; font-size: 13px; }
.action-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.action-row select, .action-row input { height: 34px; padding: 0 10px; color: #43515a; background: #ffffff; border: 1px solid #cfd8dc; border-radius: 5px; }
.action-row textarea { width: 100%; padding: 8px 10px; color: #43515a; background: #ffffff; border: 1px solid #cfd8dc; border-radius: 5px; font: inherit; resize: vertical; }
.action-row.note-row { flex-direction: column; align-items: stretch; }
.muted { color: #77858d; font-size: 12px; }
.follower-list { display: flex; gap: 6px; flex-wrap: wrap; }
.follower-tag { display: inline-flex; align-items: center; gap: 4px; padding: 3px 8px; border-radius: 10px; font-size: 11px; color: #2e7867; background: #e3f1ed; }
.follower-remove { display: inline-flex; padding: 0; color: #2e7867; background: none; border: 0; cursor: pointer; }
.incident-timeline { margin: 4px 0 0; padding: 0; list-style: none; }
.incident-timeline li { position: relative; display: flex; gap: 10px; padding: 0 0 14px 0; }
.incident-timeline li::before { content: ''; position: absolute; left: 4px; top: 16px; bottom: -2px; width: 1px; background: #e0e6e9; }
.incident-timeline li:last-child::before { display: none; }
.timeline-marker { position: relative; z-index: 1; width: 9px; height: 9px; margin-top: 5px; border-radius: 50%; background: #3b6ea5; flex: none; }
.timeline-marker.note { background: #d97706; }
.timeline-head { display: flex; align-items: center; gap: 8px; }
.timeline-kind { font-size: 11px; font-weight: 650; }
.timeline-kind.system { color: #3b6ea5; }
.timeline-kind.note { color: #b45309; }
.timeline-actor { color: #77858d; font-size: 11px; }
.timeline-head time { margin-left: auto; color: #99a4ab; font-size: 11px; }
.timeline-content { margin: 4px 0 0; color: #46545c; font-size: 12px; line-height: 1.6; white-space: pre-wrap; }
.postmortem-view { margin: 0 0 10px; padding: 10px 12px; color: #3d4b53; font-size: 13px; line-height: 1.7; white-space: pre-wrap; background: #eef6f3; border: 1px solid #d7e9e2; border-radius: 6px; }
.incident-form { width: min(520px, 92vw); margin-left: auto; margin-right: 24px; height: 100%; padding: 22px 22px 44px; overflow-y: auto; background: #f8fafb; box-shadow: -12px 0 38px rgba(17, 28, 34, 0.18); }
.incident-form header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.incident-form h2 { margin: 0; font-size: 18px; color: #2f3d45; }
.form-field { display: grid; gap: 5px; margin-top: 12px; }
.form-field > span { color: #5a6672; font-size: 11px; font-weight: 650; }
.form-hint { color: #77858d; font-size: 10px; line-height: 1.5; }
.form-field input, .form-field select, .form-field textarea { padding: 8px 10px; color: #43515a; background: #ffffff; border: 1px solid #cfd8dc; border-radius: 5px; font: inherit; }
.form-field textarea { resize: vertical; }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 18px; }
.incident-table-scroll { max-width: 100%; overflow-x: auto; }

@media (max-width: 720px) {
  .incident-drawer { width: 100vw; padding: 16px; }
  .incident-form { width: 100vw; margin-right: 0; padding: 16px 16px 32px; }
  .incident-meta { grid-template-columns: 1fr; }
  .action-row { flex-direction: column; align-items: stretch; }
  .action-row select, .action-row input { width: 100%; }
  .action-row textarea { width: 100%; }
  .form-actions { flex-wrap: wrap; }
  .form-actions .secondary-button, .form-actions .primary-button { flex: 1 1 120px; }
}
.filter-tabs { display: flex; gap: 6px; flex-wrap: wrap; margin: 6px 0 10px; }
.filter-tabs button { padding: 3px 10px; border-radius: 14px; font-size: 11px; color: #5a6b85; background: #eef1f5; border: 1px solid transparent; cursor: pointer; }
.filter-tabs button.active { color: #1d4ed8; background: #e4ecfc; border-color: #b9c9e8; }
.postmortem-metrics { display: flex; gap: 10px; flex-wrap: wrap; margin: 10px 0; }
.postmortem-metrics .metric { display: inline-flex; flex-direction: column; gap: 2px; padding: 8px 12px; border-radius: 10px; background: #f4f7fb; border: 1px solid #e3e8ea; }
.postmortem-metrics .metric strong { font-size: 14px; color: #2b3a55; }
.postmortem-metrics .metric small { font-size: 11px; color: #8593a8; }
</style>
