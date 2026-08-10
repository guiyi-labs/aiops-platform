<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Boxes, CheckCircle2, FileSearch, MessageSquareText, RefreshCw, RotateCcw, SearchCheck, Sparkles, Stethoscope, UserCheck, X, XCircle } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { addAIExplanationFeedback, addDiagnosisFeedback, assignDiagnosis, executeRemediation, generateDiagnosisExplanation, getAIQualitySummary, getAIRuntimeStatus, getDiagnosis, listDiagnoses, listDiagnosisExplanations, listRemediationPlans, previewRemediation, transitionDiagnosis } from '../api/diagnosis'
import { APIError } from '../api/auth'
import { listAssignableUsers } from '../api/users'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import FindingEvidencePanel from '../components/FindingEvidencePanel.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { AIExplanationFeedbackVerdict, AIQualitySummary, AIRuntimeStatus, DiagnosisActionItem, DiagnosisAIExplanation, DiagnosisRecord, DiagnosisStatus, FeedbackVerdict, RemediationPlan } from '../types/diagnosis'
import type { UserProfile } from '../types/auth'
import { fromDiagnosis } from '../utils/finding-detail'

const auth = useAuthStore()
const router = useRouter()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref(0)
const statusFilter = ref<DiagnosisStatus | ''>('')
const overdueOnly = ref(false)
const records = ref<DiagnosisRecord[]>([])
const detail = ref<DiagnosisRecord | null>(null)
const loading = ref(false)
const detailLoading = ref(false)
const errorMessage = ref('')
const transitionComment = ref('')
const feedbackVerdict = ref<FeedbackVerdict>('accurate')
const feedbackComment = ref('')
const saving = ref(false)
const assignableUsers = ref<UserProfile[]>([])
const assignmentUserID = ref(0)
const assignmentComment = ref('')
const explanations = ref<DiagnosisAIExplanation[]>([])
const aiLoading = ref(false)
const aiError = ref('')
const aiStatus = ref<AIRuntimeStatus | null>(null)
const aiQuality = ref<AIQualitySummary | null>(null)
const aiFeedbackComments = ref<Record<number, string>>({})
const aiFeedbackSaving = ref(0)
const remediationPlans = ref<RemediationPlan[]>([])
const remediationTarget = ref('')
const pendingRemediation = ref<RemediationPlan | null>(null)
const remediationToken = ref('')
const remediationIdempotencyKey = ref('')
const remediationConfirmed = ref(false)
const remediationBusy = ref(false)
const remediationError = ref('')

const clusterNames = computed(() => new Map(clusters.value.map((item) => [item.id, item.name])))
const canManage = computed(() => auth.user?.roles.some((role) => role === 'system_admin' || role === 'operations_admin') ?? false)
const aiGenerationBlocked = computed(() => aiStatus.value ? !aiStatus.value.enabled || aiStatus.value.remaining_tokens === 0 : false)

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function slaLabel(record: DiagnosisRecord): string {
  if (record.status === 'resolved' || record.status === 'dismissed') return 'SLA 已关闭'
  const milliseconds = new Date(record.sla_due_at).getTime() - Date.now()
  const absoluteMinutes = Math.max(1, Math.round(Math.abs(milliseconds) / 60000))
  const hours = Math.floor(absoluteMinutes / 60)
  const minutes = absoluteMinutes % 60
  const duration = hours > 0 ? `${hours}小时${minutes > 0 ? `${minutes}分` : ''}` : `${minutes}分钟`
  return milliseconds < 0 ? `已逾期 ${duration}` : `剩余 ${duration}`
}

async function loadRecords() {
  loading.value = true; errorMessage.value = ''
  try { records.value = (await listDiagnoses(auth.accessToken, { clusterID: selectedClusterID.value || undefined, status: statusFilter.value, overdue: overdueOnly.value ? true : undefined })).items }
  catch { errorMessage.value = '无法加载诊断历史' }
  finally { loading.value = false }
}

async function openDetail(record: DiagnosisRecord) {
  detailLoading.value = true; errorMessage.value = ''
  try {
    const [recordDetail, explanationResponse, remediationResponse] = await Promise.all([getDiagnosis(auth.accessToken, record.id), listDiagnosisExplanations(auth.accessToken, record.id), listRemediationPlans(auth.accessToken, record.id)])
    detail.value = recordDetail
    explanations.value = explanationResponse.items
    remediationPlans.value = remediationResponse.items
    pendingRemediation.value = null; remediationToken.value = ''; remediationConfirmed.value = false; remediationError.value = ''
    aiError.value = ''
    assignmentUserID.value = detail.value.assignee?.id ?? assignableUsers.value[0]?.id ?? 0
  }
  catch { errorMessage.value = '无法加载诊断证据详情' }
  finally { detailLoading.value = false }
}

async function createRemediationPreview() {
  if (!detail.value || !remediationTarget.value.trim()) return
  remediationBusy.value = true; remediationError.value = ''
  try {
    const plan = await previewRemediation(auth.accessToken, detail.value.id, 'deployment.rollout_restart', remediationTarget.value.trim())
    pendingRemediation.value = plan
    remediationToken.value = plan.confirmation_token || ''
    remediationIdempotencyKey.value = `remediation-${crypto.randomUUID()}`
    remediationConfirmed.value = false
    remediationPlans.value = [plan, ...remediationPlans.value]
  } catch (error) {
    remediationError.value = error instanceof APIError && error.code === 'REMEDIATION_TARGET_MISMATCH'
      ? '目标 Deployment 的 selector 与当前诊断 Pod 不匹配。'
      : 'dry-run 未通过；未创建可执行修复计划。'
  } finally { remediationBusy.value = false }
}

async function confirmRemediation() {
  if (!pendingRemediation.value || !remediationToken.value || !remediationConfirmed.value) return
  remediationBusy.value = true; remediationError.value = ''
  try {
    const result = await executeRemediation(auth.accessToken, pendingRemediation.value.id, remediationToken.value, remediationIdempotencyKey.value)
    remediationPlans.value = remediationPlans.value.map((item) => item.id === result.id ? result : item)
    pendingRemediation.value = null; remediationToken.value = ''; remediationConfirmed.value = false
  } catch (error) {
    remediationError.value = error instanceof APIError ? `执行失败：${error.message}` : '执行失败，请检查集群状态和审计记录。'
    if (detail.value) remediationPlans.value = (await listRemediationPlans(auth.accessToken, detail.value.id).catch(() => ({ items: remediationPlans.value, total: 0, remaining: 0 }))).items
  } finally { remediationBusy.value = false }
}

function remediationStatusLabel(status: RemediationPlan['status']): string {
  return ({ awaiting_confirmation: '等待确认', executing: '执行中', succeeded: '成功', failed: '失败', expired: '已过期' } as const)[status]
}

function actionKindLabel(kind: DiagnosisActionItem['kind']): string {
  return kind === 'advisory' ? '只读建议' : '受控动作'
}

function actionAreaHint(detail: DiagnosisRecord): { level: 'advisory' | 'unavailable' | 'permission' | 'dependency'; text: string } {
  const hasControlled = detail.actions?.some((item) => item.kind === 'controlled_action') ?? false
  if (!hasControlled) return { level: 'advisory', text: '当前诊断仅提供只读建议，无受控动作。' }
  if (!canManage.value) return { level: 'permission', text: '受控动作需要 system_admin / operations_admin 权限。' }
  if (detail.status !== 'confirmed') return { level: 'dependency', text: '受控动作要求诊断处于 confirmed 状态，且集群 dry-run 可用。' }
  return { level: 'advisory', text: '' }
}

const workloadsKindMap: Record<string, string> = {
  Pod: 'Pod',
  Service: 'Service',
  Node: 'Node',
  Deployment: 'Deployment',
  Ingress: 'Ingress',
  PersistentVolumeClaim: 'PVC',
  HorizontalPodAutoscaler: 'HPA',
}

const resourceDetailKinds = new Set(['Pod', 'Service', 'Node', 'Deployment', 'Ingress', 'PersistentVolumeClaim'])

function workloadsHref(record: DiagnosisRecord): string {
  const query = new URLSearchParams({
    cluster: String(record.cluster_id),
    kind: workloadsKindMap[record.resource.kind] ?? record.resource.kind,
  })
  if (record.resource.namespace) query.set('namespace', record.resource.namespace)
  query.set('name', record.resource.name)
  return `/workloads?${query.toString()}`
}

function resourceDetailHref(record: DiagnosisRecord): string | null {
  if (!resourceDetailKinds.has(record.resource.kind)) return null
  const ns = record.resource.kind === 'Node' ? '_' : (record.resource.namespace || '_')
  return `/clusters/${record.cluster_id}/resources/${record.resource.kind}/${ns}/${record.resource.name}`
}

const canViewAudit = computed(() => auth.user?.roles.some((role) => role === 'security_auditor' || role === 'system_admin') ?? false)

function openDeepLink(href: string): void {
  void router.push(href)
}

function evidenceLabel(id: string): string {
  if (!detail.value) return id
  const index = Number(id.slice(1)) - 1
  const evidence = detail.value.evidence[index]
  return evidence ? `${id} · ${evidence.type} · ${evidence.source}` : id
}

function aiFeedbackLabel(verdict: AIExplanationFeedbackVerdict): string {
  if (verdict === 'helpful') return '有帮助'
  if (verdict === 'partially_helpful') return '部分有帮助'
  return '无帮助'
}

function percent(value: number): string {
  return `${Math.round(value * 100)}%`
}

async function generateExplanation() {
  if (!detail.value) return
  aiLoading.value = true; aiError.value = ''
  try {
    const item = await generateDiagnosisExplanation(auth.accessToken, detail.value.id)
    explanations.value = [item, ...explanations.value]
  } catch (error) {
    if (error instanceof APIError && error.code === 'AI_BUDGET_EXCEEDED') aiError.value = '今日剩余 AI token 预算不足以生成本次解释，规则结论和原始证据仍然有效。'
    else if (error instanceof APIError && error.code === 'AI_BUSY') aiError.value = 'AI 解释并发已满，请稍后重试；规则诊断不受影响。'
    else aiError.value = 'AI 解释暂不可用，规则结论和原始证据仍然有效。请检查 Provider 配置或稍后重试。'
  }
  finally { aiLoading.value = false; await loadAIStatus() }
}

async function loadAIStatus() {
  try { aiStatus.value = await getAIRuntimeStatus(auth.accessToken) }
  catch { aiStatus.value = null }
}

async function loadAIQuality() {
  try { aiQuality.value = await getAIQualitySummary(auth.accessToken) }
  catch { aiQuality.value = null }
}

async function submitAIExplanationFeedback(item: DiagnosisAIExplanation, verdict: AIExplanationFeedbackVerdict) {
  if (item.my_feedback) return
  aiFeedbackSaving.value = item.id; aiError.value = ''
  try {
    const result = await addAIExplanationFeedback(auth.accessToken, item.id, verdict, (aiFeedbackComments.value[item.id] || '').trim())
    item.my_feedback = result.feedback
    item.feedback_summary = result.summary
    delete aiFeedbackComments.value[item.id]
    await loadAIQuality()
  } catch (error) {
    aiError.value = error instanceof APIError && error.code === 'AI_FEEDBACK_EXISTS'
      ? '你已经评价过这条解释。'
      : 'AI 解释质量反馈保存失败，请稍后重试。'
  } finally { aiFeedbackSaving.value = 0 }
}

async function changeStatus(status: DiagnosisStatus) {
  if (!detail.value) return
  saving.value = true; errorMessage.value = ''
  try {
    detail.value = await transitionDiagnosis(auth.accessToken, detail.value.id, status, transitionComment.value.trim())
    transitionComment.value = ''
    await loadRecords()
  } catch { errorMessage.value = '状态更新失败，请检查当前状态与操作权限' }
  finally { saving.value = false }
}

async function submitFeedback() {
  if (!detail.value) return
  saving.value = true; errorMessage.value = ''
  try {
    detail.value = await addDiagnosisFeedback(auth.accessToken, detail.value.id, feedbackVerdict.value, feedbackComment.value.trim())
    feedbackComment.value = ''
  } catch { errorMessage.value = '人工反馈保存失败' }
  finally { saving.value = false }
}

async function transferAssignment() {
  if (!detail.value || !assignmentUserID.value) return
  saving.value = true; errorMessage.value = ''
  try {
    detail.value = await assignDiagnosis(auth.accessToken, detail.value.id, assignmentUserID.value, assignmentComment.value.trim())
    assignmentComment.value = ''
    await loadRecords()
  } catch { errorMessage.value = '负责人转派失败，请确认目标账号可用且不是当前负责人' }
  finally { saving.value = false }
}

async function initialize() {
  try {
    const [clusterResponse, userResponse] = await Promise.all([listClusters(auth.accessToken), canManage.value ? listAssignableUsers(auth.accessToken) : Promise.resolve({ items: [], total: 0, remaining: 0 })])
    clusters.value = clusterResponse.items; assignableUsers.value = userResponse.items
  }
  catch { errorMessage.value = '无法加载集群列表' }
  await Promise.all([loadAIStatus(), loadAIQuality()])
  await loadRecords()
}

watch([selectedClusterID, statusFilter, overdueOnly], loadRecords)
onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="故障分析" title="智能诊断">
    <section class="diagnosis-toolbar">
      <div><strong>诊断历史</strong><span>规则结论与原始证据分离保存，可追溯每次判断依据。</span></div>
      <div class="toolbar-actions">
        <select v-model="selectedClusterID" aria-label="按集群筛选"><option :value="0">全部集群</option><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select><select v-model="statusFilter" aria-label="按状态筛选"><option value="">全部状态</option><option value="open">open</option><option value="confirmed">confirmed</option><option value="resolved">resolved</option><option value="dismissed">dismissed</option></select><label class="overdue-filter"><input v-model="overdueOnly" type="checkbox" />仅看逾期</label>
        <button class="secondary-button" type="button" :disabled="loading" @click="loadRecords"><RefreshCw :size="15" :class="{ spinning: loading }" />刷新</button>
      </div>
    </section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <section class="diagnosis-history">
      <div v-if="!loading && records.length === 0" class="resource-empty"><Stethoscope :size="30" /><strong>暂无诊断记录</strong><span>请在工作负载页面对异常 Pod 或 Service 发起诊断。</span></div>
      <button v-for="record in records" :key="record.id" type="button" class="diagnosis-history-row" :disabled="detailLoading" @click="openDetail(record)">
        <span class="diagnosis-rule-icon"><FileSearch :size="18" /></span>
        <span class="diagnosis-history-main"><strong>{{ record.summary }}</strong><small>{{ record.rule_id }} · {{ record.resource.kind }}/{{ record.resource.namespace }}/{{ record.resource.name }} · {{ slaLabel(record) }}</small></span>
        <span class="diagnosis-cluster">{{ clusterNames.get(record.cluster_id) || `集群 #${record.cluster_id}` }}</span>
        <span class="diagnosis-list-badges"><span class="severity-badge">{{ record.severity }}</span><span class="workflow-status" :class="record.status">{{ record.status }}</span></span>
        <time>{{ formatTime(record.observed_at) }}</time>
      </button>
    </section>

    <div v-if="detail" class="log-overlay" @click.self="detail = null"><section class="diagnosis-drawer"><header><div><p class="context-label">DIAGNOSIS #{{ detail.id }}</p><h2>{{ detail.rule_id }}</h2></div><button class="icon-button" aria-label="关闭诊断" @click="detail = null"><X :size="18" /></button></header><div class="diagnosis-detail-badges"><span class="severity-badge">{{ detail.severity }}</span><span class="workflow-status" :class="detail.status">{{ detail.status }}</span><span v-if="detail.assignee" class="assignee-badge"><UserCheck :size="13" />{{ detail.assignee.name }}</span><span class="sla-badge" :class="{ overdue: detail.overdue }">{{ slaLabel(detail) }}</span></div><p class="diagnosis-summary">{{ detail.summary }}</p><p class="diagnosis-resource-ref">{{ detail.resource.kind }} · {{ detail.resource.namespace }}/{{ detail.resource.name }} · 观察 {{ formatTime(detail.observed_at) }} · 截止 {{ formatTime(detail.sla_due_at) }}</p>
      <FindingEvidencePanel :finding="fromDiagnosis(detail)" />
      <section v-if="detail.root_cause_card" class="root-cause-card">
        <header><span class="root-cause-label">根因结论</span><span class="root-cause-confidence">{{ detail.root_cause_card.confidence }}</span></header>
        <p class="root-cause-conclusion">{{ detail.root_cause_card.conclusion }}</p>
        <dl class="root-cause-meta">
          <div><dt>首次观察</dt><dd>{{ formatTime(detail.root_cause_card.first_observed_at) }}</dd></div>
          <div><dt>置信依据</dt><dd>{{ detail.root_cause_card.confidence_source }}</dd></div>
          <div><dt>关键证据</dt><dd><code v-for="ref in detail.root_cause_card.key_evidence_refs" :key="ref">{{ evidenceLabel(ref) }}</code></dd></div>
        </dl>
      </section>
      <section class="ai-explanation-panel">
        <div class="ai-panel-heading"><div><span><Sparkles :size="15" />引用式 AI 解释</span><small>AI 只解释下方规则证据，不执行集群操作。</small></div><button v-if="canManage" class="secondary-button" :disabled="aiLoading || aiGenerationBlocked" @click="generateExplanation"><Sparkles :size="14" :class="{ spinning: aiLoading }" />{{ explanations.length ? '重新生成' : '生成解释' }}</button></div>
        <div v-if="aiStatus" class="ai-runtime-status"><span :class="{ available: aiStatus.available }">{{ !aiStatus.enabled ? 'Provider 已禁用' : aiStatus.available ? 'Provider 可用' : 'Provider 受限' }}</span><small>{{ aiStatus.model }} · 今日 {{ aiStatus.used_tokens_today }} / {{ aiStatus.daily_token_budget || '不限' }} tokens · 并发 {{ aiStatus.active_requests }}/{{ aiStatus.max_concurrent_requests }}</small><small v-if="aiQuality">质量反馈 {{ aiQuality.total_feedback }} 条 · 有帮助率 {{ percent(aiQuality.helpful_rate) }}</small></div>
        <p v-if="aiError" class="ai-fallback-message">{{ aiError }}</p><p v-if="!explanations.length && !aiError" class="compact-empty">尚未生成 AI 解释，确定性规则结论不受影响。</p>
        <article v-for="item in explanations" :key="item.id" class="ai-explanation-card">
          <header><strong>{{ item.summary }}</strong><span>{{ item.model }} · {{ formatTime(item.created_at) }}</span></header><p>{{ item.analysis }}</p>
          <h4>建议操作</h4><ol><li v-for="action in item.recommended_actions" :key="action.action"><span class="ai-priority" :class="action.priority">{{ action.priority }}</span>{{ action.action }} <small v-if="action.evidence_ids.length">[{{ action.evidence_ids.join(', ') }}]</small></li></ol>
          <h4>证据引用</h4><ul><li v-for="citation in item.citations" :key="`${citation.evidence_id}-${citation.claim}`"><strong>{{ evidenceLabel(citation.evidence_id) }}</strong><span>{{ citation.claim }}</span></li></ul>
          <footer>{{ item.actor.name }} · 输入 {{ item.input_tokens }} / 输出 {{ item.output_tokens }} tokens</footer>
          <div class="ai-feedback-box">
            <div><strong>解释质量</strong><small>{{ item.feedback_summary?.total || 0 }} 条反馈<span v-if="item.feedback_summary?.total"> · 有帮助率 {{ percent(item.feedback_summary.helpful_rate) }}</span></small></div>
            <p v-if="item.my_feedback" class="ai-feedback-recorded">已反馈：{{ aiFeedbackLabel(item.my_feedback.verdict) }}<span v-if="item.my_feedback.comment"> · {{ item.my_feedback.comment }}</span></p>
            <template v-else>
              <input v-model="aiFeedbackComments[item.id]" maxlength="1000" placeholder="补充评价原因（可选）" :aria-label="`解释 ${item.id} 的反馈备注`" />
              <div class="ai-feedback-actions"><button :disabled="aiFeedbackSaving === item.id" @click="submitAIExplanationFeedback(item, 'helpful')">有帮助</button><button :disabled="aiFeedbackSaving === item.id" @click="submitAIExplanationFeedback(item, 'partially_helpful')">部分有帮助</button><button :disabled="aiFeedbackSaving === item.id" @click="submitAIExplanationFeedback(item, 'not_helpful')">无帮助</button></div>
            </template>
          </div>
        </article>
      </section>
      <section v-if="canManage" class="assignment-panel"><h3>负责人转派</h3><div><select v-model="assignmentUserID" aria-label="选择负责人"><option v-for="item in assignableUsers" :key="item.id" :value="item.id">{{ item.display_name }} · {{ item.username }}</option></select><input v-model="assignmentComment" maxlength="2000" placeholder="转派原因（可选）" /><button class="secondary-button" :disabled="saving || !assignmentUserID || assignmentUserID === detail.assignee?.id" @click="transferAssignment"><UserCheck :size="14" />转派</button></div></section>
      <section v-if="canManage" class="workflow-panel"><h3>处置操作</h3><textarea v-model="transitionComment" maxlength="2000" placeholder="记录复现情况、处置结果或驳回原因（可选）" /><div class="workflow-actions"><button v-if="detail.status === 'open'" class="primary-button" :disabled="saving" @click="changeStatus('confirmed')"><CheckCircle2 :size="14" />确认异常</button><button v-if="detail.status === 'confirmed'" class="primary-button" :disabled="saving" @click="changeStatus('resolved')"><CheckCircle2 :size="14" />标记已解决</button><button v-if="['open', 'confirmed'].includes(detail.status)" class="secondary-button" :disabled="saving" @click="changeStatus('dismissed')"><XCircle :size="14" />驳回</button><button v-if="['resolved', 'dismissed'].includes(detail.status)" class="secondary-button" :disabled="saving" @click="changeStatus('open')"><RotateCcw :size="14" />重新打开</button></div></section>
      <section class="remediation-panel">
        <div class="remediation-heading"><div><h3>受控修复</h3><small>仅允许对匹配当前 Pod 的 Deployment 执行 rollout restart；先由 Kubernetes dry-run 验证。</small></div></div>
        <div v-if="canManage && detail.status === 'confirmed' && detail.resource.kind === 'Pod'" class="remediation-preview-form"><input v-model="remediationTarget" maxlength="253" placeholder="目标 Deployment 名称" aria-label="目标 Deployment 名称" /><button class="secondary-button" :disabled="remediationBusy || !remediationTarget.trim()" @click="createRemediationPreview">生成 dry-run 计划</button></div>
        <p v-else-if="canManage" class="compact-empty">只有 confirmed 状态的 Pod 诊断可以生成修复计划。</p>
        <p v-if="remediationError" class="error-message">{{ remediationError }}</p>
        <div v-if="pendingRemediation" class="remediation-confirmation"><strong>dry-run 已通过：{{ pendingRemediation.target.kind }}/{{ pendingRemediation.target.namespace }}/{{ pendingRemediation.target.name }}</strong><small>计划 {{ pendingRemediation.id }} · 资源版本 {{ pendingRemediation.target.resource_version }} · {{ formatTime(pendingRemediation.expires_at) }} 前有效</small><label><input v-model="remediationConfirmed" type="checkbox" />我确认对上述固定资源执行 rollout restart，并理解这会创建新的 Pod。</label><button class="primary-button" :disabled="remediationBusy || !remediationConfirmed" @click="confirmRemediation">确认并执行</button></div>
        <div class="remediation-history"><article v-for="plan in remediationPlans" :key="plan.id"><span class="remediation-status" :class="plan.status">{{ remediationStatusLabel(plan.status) }}</span><strong>{{ plan.target.kind }}/{{ plan.target.namespace }}/{{ plan.target.name }}</strong><time>{{ formatTime(plan.created_at) }}</time><small>{{ plan.requested_by.name }} · {{ plan.id }}</small><p v-if="plan.last_error">{{ plan.last_error }}</p></article><p v-if="!remediationPlans.length" class="compact-empty">暂无受控修复计划。</p></div>
      </section>
      <h3>可能根因</h3><ol><li v-for="item in detail.root_causes" :key="item">{{ item }}</li></ol><h3>处理建议</h3><ol><li v-for="item in detail.recommendations" :key="item">{{ item }}</li></ol>
      <section class="action-area">
        <h3>行动区 · {{ detail.actions?.length ?? 0 }}</h3>
        <div v-if="detail.actions?.length" class="action-area-list">
          <article v-for="item in detail.actions" :key="item.title" :class="['action-item', item.kind]">
            <span class="action-kind" :class="item.kind">{{ actionKindLabel(item.kind) }}</span>
            <div class="action-body">
              <strong>{{ item.title }}</strong>
              <p v-if="item.detail">{{ item.detail }}</p>
              <small v-if="item.requires_dry_run">先由 Kubernetes dry-run 验证；确认后才会执行。</small>
            </div>
          </article>
        </div>
        <div v-if="detail.actions?.some((item) => item.kind === 'controlled_action') && !(canManage && detail.status === 'confirmed')" class="action-area-note">
          <span class="notice-badge">{{ actionAreaHint(detail).level }}</span>
          {{ actionAreaHint(detail).text }}
        </div>
      </section>
      <section class="deep-links" aria-label="关联入口">
        <h3>关联入口</h3>
        <div class="deep-link-row">
          <button v-if="resourceDetailHref(detail)" type="button" class="secondary-button deep-link" @click="openDeepLink(resourceDetailHref(detail)!)"><FileSearch :size="14" />资源详情</button>
          <button type="button" class="secondary-button deep-link" @click="openDeepLink(workloadsHref(detail))"><Boxes :size="14" />工作负载与相关事件</button>
          <button v-if="canViewAudit" type="button" class="secondary-button deep-link" @click="openDeepLink('/audit-logs')"><SearchCheck :size="14" />审计记录</button>
        </div>
        <small class="deep-link-hint">从诊断出发直达资源、拓扑工作台、相关事件与审计入口。</small>
      </section>
      <section v-if="detail.timeline?.length" class="evidence-timeline">
        <h3>证据时间线 · {{ detail.timeline.length }}</h3>
        <article v-for="item in detail.timeline" :key="item.ref">
          <span class="timeline-category" :class="item.category">{{ item.category }}</span>
          <div class="timeline-entry-body">
            <strong>{{ item.summary || item.type }}</strong>
            <small>{{ item.type }} · {{ item.source }}</small>
            <p v-if="item.missing" class="timeline-missing">证据缺失 · {{ item.missing_reason || '对象未报告该状态' }}</p>
          </div>
          <time>{{ item.occurred_at ? formatTime(item.occurred_at) : '时间未知' }}</time>
        </article>
        <details class="raw-evidence">
          <summary>原始证据 JSON（可追溯）</summary>
          <article v-for="item in detail.evidence" :key="`${item.type}-${item.source}`" class="evidence-card"><strong>{{ item.type }} · {{ item.source }}</strong><pre>{{ JSON.stringify(item.content, null, 2) }}</pre></article>
        </details>
      </section>
      <template v-else>
        <h3>持久化证据 · {{ detail.evidence.length }}</h3>
        <article v-for="item in detail.evidence" :key="`${item.type}-${item.source}`" class="evidence-card"><strong>{{ item.type }} · {{ item.source }}</strong><pre>{{ JSON.stringify(item.content, null, 2) }}</pre></article>
      </template>
      <section class="workflow-timeline"><h3>处置记录 · {{ detail.activities?.length ?? 0 }}</h3><article v-for="item in detail.activities" :key="item.id"><span>{{ item.from_status }} → {{ item.to_status }}</span><strong>{{ item.actor.name }}</strong><time>{{ formatTime(item.created_at) }}</time><p v-if="item.comment">{{ item.comment }}</p></article><p v-if="!detail.activities?.length" class="compact-empty">尚未开始人工处置</p></section>
      <section class="assignment-history"><h3>转派记录 · {{ detail.assignments?.length ?? 0 }}</h3><article v-for="item in detail.assignments" :key="item.id"><span>{{ item.from_assignee?.name || '未分配' }} → {{ item.to_assignee.name }}</span><strong>{{ item.actor.name }}</strong><time>{{ formatTime(item.created_at) }}</time><p v-if="item.comment">{{ item.comment }}</p></article><p v-if="!detail.assignments?.length" class="compact-empty">暂无负责人转派</p></section>
      <section v-if="canManage" class="feedback-panel"><h3>规则准确性反馈</h3><div><select v-model="feedbackVerdict" aria-label="反馈结论"><option value="accurate">准确</option><option value="inaccurate">不准确</option><option value="uncertain">暂不确定</option></select><input v-model="feedbackComment" maxlength="2000" placeholder="补充判断依据（可选）" /><button class="secondary-button" :disabled="saving" @click="submitFeedback"><MessageSquareText :size="14" />提交反馈</button></div></section><section class="feedback-history"><article v-for="item in detail.feedback" :key="item.id"><span class="feedback-verdict" :class="item.verdict">{{ item.verdict }}</span><strong>{{ item.actor.name }}</strong><time>{{ formatTime(item.created_at) }}</time><p v-if="item.comment">{{ item.comment }}</p></article></section>
    </section></div>
  </ConsoleLayout>
</template>
