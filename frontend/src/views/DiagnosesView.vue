<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Boxes, CheckCircle2, Download, FileSearch, MessageSquareText, Pause, Play, RefreshCw, RotateCcw, SearchCheck, SkipBack, SkipForward, Sparkles, Stethoscope, X, XCircle } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { addAIExplanationFeedback, addDiagnosisFeedback, generateDiagnosisExplanation, getAIQualitySummary, getAIRuntimeStatus, getDiagnosis, getDiagnosisReplay, listDiagnoses, listDiagnosisExplanations, transitionDiagnosis } from '../api/diagnosis'
import { APIError } from '../api/auth'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import FindingEvidencePanel from '../components/FindingEvidencePanel.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { AIExplanationFeedbackVerdict, AIQualitySummary, AIRuntimeStatus, DiagnosisAIExplanation, DiagnosisRecord, DiagnosisReplayView, DiagnosisStatus, FeedbackVerdict } from '../types/diagnosis'

import { useDiagnosisReplay } from '../composables/useDiagnosisReplay'
import { fromDiagnosis } from '../utils/finding-detail'
import { downloadJSON } from '../utils/download'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref(0)
const statusFilter = ref<DiagnosisStatus | ''>('')

const records = ref<DiagnosisRecord[]>([])
const detail = ref<DiagnosisRecord | null>(null)
const loading = ref(false)
const detailLoading = ref(false)
// 诊断列表游标分页状态。total 由后端 COUNT 得出（此前后端拿本页条数冒充，
// 界面上的「共 N 条」其实是「这一页有几条」）。
const recordsTotal = ref(0)
const recordsCursor = ref<number | null>(null)
const loadingMore = ref(false)
const errorMessage = ref('')
const transitionComment = ref('')
const feedbackVerdict = ref<FeedbackVerdict>('accurate')
const feedbackComment = ref('')
const saving = ref(false)

const explanations = ref<DiagnosisAIExplanation[]>([])
const aiLoading = ref(false)
const aiError = ref('')
const aiStatus = ref<AIRuntimeStatus | null>(null)
const aiQuality = ref<AIQualitySummary | null>(null)
const aiFeedbackComments = ref<Record<number, string>>({})
const aiFeedbackSaving = ref(0)

const replay = ref<DiagnosisReplayView | null>(null)
const replayLoadError = ref('')
const replayControls = reactive(useDiagnosisReplay(() => replay.value?.steps ?? []))

const clusterNames = computed(() => new Map(clusters.value.map((item) => [item.id, item.name])))
const canManage = computed(() => auth.user?.roles.some((role) => role === 'system_admin' || role === 'operations_admin') ?? false)
const aiGenerationBlocked = computed(() => aiStatus.value ? !aiStatus.value.enabled || aiStatus.value.remaining_tokens === 0 : false)

function exportEvidence() {
  if (!detail.value) return
  downloadJSON(`diagnosis-${detail.value.id}-evidence.json`, {
    schema: 'aiops.diagnosis-evidence/v1', exported_at: new Date().toISOString(),
    record: detail.value, explanations: explanations.value,
    replay: replay.value, replay_available: replay.value !== null,
  })
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

// ─── 证据内容结构化展示 ───
// 把 evidence.content 的扁平键值对渲染为标签式清单；嵌套对象/数组递归展开。
// 时间戳字段单独格式化；长文本截断并保留可展开。
const EVIDENCE_TIMESTAMP_KEYS = new Set(['first_timestamp', 'last_timestamp', 'observed_at', 'occurred_at', 'started_at', 'finished_at', 'created_at', 'last_transition_time'])

function isTimestampField(key: string): boolean { return EVIDENCE_TIMESTAMP_KEYS.has(key) }

function formatEvidenceValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'number') return String(value)
  if (typeof value === 'object') {
    try { return JSON.stringify(value) } catch { return String(value) }
  }
  return String(value)
}

function evidenceEntries(record: Record<string, unknown>): { key: string; value: string; isTimestamp: boolean; isObject: boolean }[] {
  return Object.entries(record).map(([key, raw]) => ({
    key,
    value: isTimestampField(key) ? formatTimestamp(raw) : formatEvidenceValue(raw),
    isTimestamp: isTimestampField(key),
    isObject: typeof raw === 'object' && raw !== null,
  }))
}

function formatTimestamp(raw: unknown): string {
  if (typeof raw !== 'string' || !raw) return '—'
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? raw : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(date)
}

async function loadRecords() {
  loading.value = true; errorMessage.value = ''
  try {
    const page = await listDiagnoses(auth.accessToken, { clusterID: selectedClusterID.value || undefined, status: statusFilter.value })
    records.value = page.items
    recordsTotal.value = page.total
    // 只有还剩记录时才给游标，否则底部会一直显示「加载更多」。
    // 用后端给的 next_before_id 判断是否还有下一页。remaining 是
    // total - 本页条数，游标分页下每页都差不多大，最后一页也仍大于 0。
    recordsCursor.value = page.next_before_id ?? null
  }
  catch { errorMessage.value = '无法加载诊断历史' }
  finally { loading.value = false }
}

// 加载更多：按 id 游标追加下一页。按 id 去重兜底，避免将来换实现时重复渲染。
async function loadMoreRecords() {
  if (recordsCursor.value === null || loadingMore.value) return
  loadingMore.value = true
  errorMessage.value = ''
  try {
    const page = await listDiagnoses(auth.accessToken, {
      clusterID: selectedClusterID.value || undefined,
      status: statusFilter.value,

      beforeID: recordsCursor.value,
    })
    const seen = new Set(records.value.map((item) => item.id))
    records.value = [...records.value, ...page.items.filter((item) => !seen.has(item.id))]
    recordsTotal.value = page.total
    recordsCursor.value = page.next_before_id ?? null
  } catch {
    errorMessage.value = '加载更多诊断失败，请稍后重试'
  } finally {
    loadingMore.value = false
  }
}

async function loadReplay(diagnosisID: number) {
  replayLoadError.value = ''
  try {
    replay.value = await getDiagnosisReplay(auth.accessToken, diagnosisID)
    replayControls.reset()
  } catch {
    replay.value = null
    replayLoadError.value = '回放模式不可用（当前后端可能未启用诊断回放）'
  }
}

async function openDetail(record: DiagnosisRecord) {
  detailLoading.value = true; errorMessage.value = ''
  try {
    const [recordDetail, explanationResponse] = await Promise.all([getDiagnosis(auth.accessToken, record.id), listDiagnosisExplanations(auth.accessToken, record.id)])
    detail.value = recordDetail
    explanations.value = explanationResponse.items

    aiError.value = ''

    loadReplay(record.id)
  }
  catch { errorMessage.value = '无法加载诊断证据详情' }
  finally { detailLoading.value = false }
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
  // P1 knowledge base: historical case references (historical:1..N) are
  // framed as verified past outcomes, not live evidence.
  if (id.startsWith('historical:')) return '历史案例'
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
    aiError.value = describeAIError(error)
  }
  finally { aiLoading.value = false; await loadAIStatus() }
}

// describeAIError 把后端的失败原因翻译成「下一步该动哪里」。
//
// 后端在 message 里已经给出具体原因（截断、未知证据、密钥、限流），早先这里一律
// 覆盖成「AI 解释暂不可用，请检查 Provider 配置」，等于把唯一有用的信息扔掉——
// 用户在界面上永远不知道该把 max_output_tokens 调大还是该换密钥。
function describeAIError(error: unknown): string {
  if (!(error instanceof APIError)) return 'AI 解释暂不可用，规则结论和原始证据仍然有效。'
  const raw = error.message || ''
  switch (error.code) {
    case 'AI_DISABLED':
      return '模型接入尚未启用：到「系统设置 → 模型接入」打开开关后即可生成解释。'
    case 'AI_BUDGET_EXCEEDED':
      return '今日剩余 AI token 预算不足以生成本次解释，规则结论和原始证据仍然有效。'
    case 'AI_BUSY':
      return 'AI 解释并发已满，请稍后重试；规则诊断不受影响。'
    case 'AI_EVIDENCE_REQUIRED':
      return '该诊断没有可引用的证据，无法生成带引用的解释。'
    case 'AI_INVALID_OUTPUT':
      if (/truncated|max_output_tokens/i.test(raw)) {
        return '模型输出被单次上限截断：到「模型接入」把「单次最大输出 token」提到 4000 以上再试。'
      }
      if (/unknown evidence|citation/i.test(raw)) {
        return `模型输出未通过引用校验：${raw}`
      }
      return `模型输出未通过校验：${raw}`
    case 'AI_PROVIDER_ERROR':
      return `模型提供方调用失败：${raw}`
    default:
      return raw || 'AI 解释暂不可用，规则结论和原始证据仍然有效。'
  }
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

async function initialize() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items
    const requested = Number(route.query.cluster)
    selectedClusterID.value = clusters.value.find(item => item.id === requested)?.id ?? clusters.value.find(item => item.enabled)?.id ?? clusters.value[0]?.id ?? 0
  } catch { errorMessage.value = '无法加载集群列表' }
  await Promise.all([loadAIStatus(), loadAIQuality(), loadRecords()])
  const id = Number(route.query.id)
  if (Number.isSafeInteger(id) && id > 0) await openDetail({ id } as DiagnosisRecord)
}

watch([selectedClusterID, statusFilter], loadRecords)
onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="故障分析" title="智能诊断">
    <section class="diagnosis-toolbar">
      <div><strong>诊断历史</strong><span>规则结论与原始证据分离保存，可追溯每次判断依据。</span></div>
      <div class="toolbar-actions">
        <select v-model="selectedClusterID" aria-label="按集群筛选"><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select><select v-model="statusFilter" aria-label="按状态筛选"><option value="">全部状态</option><option value="open">open</option><option value="confirmed">confirmed</option><option value="resolved">resolved</option><option value="dismissed">dismissed</option></select>
        <button class="secondary-button" type="button" :disabled="loading" @click="loadRecords"><RefreshCw :size="15" :class="{ spinning: loading }" />刷新</button>
      </div>
    </section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <section class="diagnosis-history">
      <div v-if="!loading && records.length === 0" class="resource-empty"><Stethoscope :size="30" /><strong>暂无诊断记录</strong><span>请在工作负载页面对异常 Pod 或 Service 发起诊断。</span></div>
      <button v-for="record in records" :key="record.id" type="button" class="diagnosis-history-row" :disabled="detailLoading" @click="openDetail(record)">
        <span class="diagnosis-rule-icon"><FileSearch :size="18" /></span>
        <span class="diagnosis-history-main"><strong>{{ record.summary }}</strong><small>{{ record.rule_id }} · {{ record.resource.kind }}/{{ record.resource.namespace }}/{{ record.resource.name }}</small></span>
        <span class="diagnosis-cluster">{{ clusterNames.get(record.cluster_id) || `集群 #${record.cluster_id}` }}</span>
        <span class="diagnosis-list-badges"><span class="severity-badge">{{ record.severity }}</span><span class="workflow-status" :class="record.status">{{ record.status }}</span></span>
        <time>{{ formatTime(record.observed_at) }}</time>
      </button>

      <div v-if="records.length > 0" class="diagnosis-pager">
        <span class="muted small">已加载 {{ records.length }} / {{ recordsTotal }} 条</span>
        <button
          v-if="recordsCursor !== null"
          type="button"
          class="secondary-button"
          :disabled="loadingMore"
          @click="loadMoreRecords"
        >
          <RefreshCw :size="14" :class="{ spinning: loadingMore }" />
          {{ loadingMore ? '加载中…' : '加载更多' }}
        </button>
        <span v-else class="muted small">已到底</span>
      </div>
    </section>

    <div v-if="detail" class="log-overlay" @click.self="detail = null"><section class="diagnosis-drawer"><header><div><p class="context-label">DIAGNOSIS #{{ detail.id }}</p><h2>{{ detail.rule_id }}</h2></div><button class="icon-button" aria-label="关闭诊断" @click="detail = null"><X :size="18" /></button></header><div class="diagnosis-detail-badges"><span class="severity-badge">{{ detail.severity }}</span><span class="workflow-status" :class="detail.status">{{ detail.status }}</span></div><div class="diagnosis-export"><button class="secondary-button" @click="exportEvidence"><Download :size="14" />导出诊断证据 JSON</button><span>包含当前已读取的结论、证据、解释和回放快照</span></div><p class="diagnosis-summary">{{ detail.summary }}</p><p class="diagnosis-resource-ref">{{ detail.resource.kind }} · {{ detail.resource.namespace }}/{{ detail.resource.name }} · 观察 {{ formatTime(detail.observed_at) }}</p>
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

      <section v-if="canManage" class="workflow-panel"><h3>人工处理与结案</h3><textarea v-model="transitionComment" maxlength="2000" placeholder="记录复现情况、处置结果或驳回原因（可选）" /><div class="workflow-actions"><button v-if="detail.status === 'open'" class="primary-button" :disabled="saving" @click="changeStatus('confirmed')"><CheckCircle2 :size="14" />确认异常</button><button v-if="detail.status === 'confirmed'" class="primary-button" :disabled="saving" @click="changeStatus('resolved')"><CheckCircle2 :size="14" />标记已解决</button><button v-if="['open', 'confirmed'].includes(detail.status)" class="secondary-button" :disabled="saving" @click="changeStatus('dismissed')"><XCircle :size="14" />驳回</button><button v-if="['resolved', 'dismissed'].includes(detail.status)" class="secondary-button" :disabled="saving" @click="changeStatus('open')"><RotateCcw :size="14" />重新打开</button></div></section>

      <h3>可能根因</h3><ol><li v-for="item in detail.root_causes" :key="item">{{ item }}</li></ol><h3>处理建议</h3><ol><li v-for="item in detail.recommendations" :key="item">{{ item }}</li></ol>

      <section class="deep-links" aria-label="关联入口">
        <h3>关联入口</h3>
        <div class="deep-link-row">
          <button v-if="resourceDetailHref(detail)" type="button" class="secondary-button deep-link" @click="openDeepLink(resourceDetailHref(detail)!)"><FileSearch :size="14" />资源详情</button>
          <button type="button" class="secondary-button deep-link" @click="openDeepLink(workloadsHref(detail))"><Boxes :size="14" />工作负载与相关事件</button>
          <button v-if="canViewAudit" type="button" class="secondary-button deep-link" @click="openDeepLink('/audit-logs')"><SearchCheck :size="14" />审计记录</button>
        </div>
        <small class="deep-link-hint">从诊断出发直达资源、拓扑工作台、相关事件与审计入口。</small>
      </section>
      <section v-if="replay" class="replay-panel">
        <h3>回放模式 · {{ replay.steps.length }} 步 <small class="replay-schema">{{ replay.schema }}</small></h3>
        <div class="replay-controls">
          <button type="button" class="secondary-button" :disabled="replayControls.total === 0" @click="replayControls.prev()"><SkipBack :size="14" />上一步</button>
          <button type="button" class="primary-button replay-play" :disabled="replayControls.total === 0" @click="replayControls.toggle()">{{ replayControls.playing ? '暂停' : '播放' }}<component :is="replayControls.playing ? Pause : Play" :size="14" /></button>
          <button type="button" class="secondary-button" :disabled="replayControls.total === 0" @click="replayControls.next()">下一步<SkipForward :size="14" /></button>
          <input v-if="replayControls.total > 0" class="replay-scrubber" type="range" min="1" :max="replayControls.total" :value="replayControls.progress" @input="replayControls.seek(Number(($event.target as HTMLInputElement).value) - 1)" aria-label="回放进度" />
        </div>
        <div class="replay-stage-chips">
          <button v-for="stage in replay.stages" :key="stage.stage" type="button" class="replay-chip" :class="{ active: replayControls.activeStage === stage.stage }" @click="replayControls.setStage(replayControls.activeStage === stage.stage ? '' : stage.stage)">{{ stage.label }} · {{ stage.count }}</button>
          <span v-if="replayControls.total > 0" class="replay-progress">{{ replayControls.progress }} / {{ replayControls.total }}</span>
        </div>
        <article v-if="replayControls.current" class="replay-step">
          <span class="timeline-category" :class="replayControls.current.category || replayControls.current.stage">{{ replayControls.current.stage }}</span>
          <div class="timeline-entry-body">
            <strong>{{ replayControls.current.summary }}</strong>
            <small>{{ replayControls.current.type }} · {{ replayControls.current.ref }}</small>
            <p v-if="replayControls.current.missing" class="timeline-missing">证据缺失 · {{ replayControls.current.missing_reason || '对象未报告该状态' }}</p>
            <pre v-if="replayControls.current.detail && Object.keys(replayControls.current.detail).length" class="replay-detail">{{ JSON.stringify(replayControls.current.detail, null, 2) }}</pre>
          </div>
          <time>{{ replayControls.current.occurred_at ? formatTime(replayControls.current.occurred_at) : '时间未知' }}</time>
        </article>
        <p v-else-if="replayControls.total === 0" class="compact-empty">该诊断暂无回放步骤</p>
        <p v-else class="compact-empty">按 ▶ 播放，或使用上一步/下一步/进度条开始回放</p>
      </section>
      <p v-else-if="replayLoadError" class="replay-error">{{ replayLoadError }}</p>
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
          <summary>原始证据（{{ detail.evidence.length }} 条 · 可追溯）</summary>
          <article v-for="item in detail.evidence" :key="`${item.type}-${item.source}`" class="evidence-card">
            <header class="evidence-card-head">
              <span class="evidence-type">{{ item.type }}</span>
              <code class="evidence-source">{{ item.source }}</code>
            </header>
            <template v-if="item.content && typeof item.content === 'object'">
              <dl class="evidence-fields">
                <template v-for="entry in evidenceEntries(item.content as Record<string, unknown>)" :key="entry.key">
                  <dt>{{ entry.key }}</dt>
                  <dd :class="{ 'is-timestamp': entry.isTimestamp }">{{ entry.value }}<span v-if="entry.isObject">（结构化）</span></dd>
                </template>
              </dl>
            </template>
            <details v-else class="evidence-raw-collapse">
              <summary>查看原始内容</summary>
              <pre>{{ JSON.stringify(item.content, null, 2) }}</pre>
            </details>
          </article>
        </details>
      </section>
      <template v-else>
        <h3>持久化证据 · {{ detail.evidence.length }}</h3>
        <article v-for="item in detail.evidence" :key="`${item.type}-${item.source}`" class="evidence-card">
          <header class="evidence-card-head">
            <span class="evidence-type">{{ item.type }}</span>
            <code class="evidence-source">{{ item.source }}</code>
          </header>
          <template v-if="item.content && typeof item.content === 'object'">
            <dl class="evidence-fields">
              <template v-for="entry in evidenceEntries(item.content as Record<string, unknown>)" :key="entry.key">
                <dt>{{ entry.key }}</dt>
                <dd :class="{ 'is-timestamp': entry.isTimestamp }">{{ entry.value }}</dd>
              </template>
            </dl>
          </template>
          <pre v-else>{{ JSON.stringify(item.content, null, 2) }}</pre>
        </article>
      </template>
      <section class="workflow-timeline"><h3>处置记录 · {{ detail.activities?.length ?? 0 }}</h3><article v-for="item in detail.activities" :key="item.id"><span>{{ item.from_status }} → {{ item.to_status }}</span><strong>{{ item.actor.name }}</strong><time>{{ formatTime(item.created_at) }}</time><p v-if="item.comment">{{ item.comment }}</p></article><p v-if="!detail.activities?.length" class="compact-empty">尚未开始人工处置</p></section>

      <section v-if="canManage" class="feedback-panel"><h3>规则准确性反馈</h3><div><select v-model="feedbackVerdict" aria-label="反馈结论"><option value="accurate">准确</option><option value="inaccurate">不准确</option><option value="uncertain">暂不确定</option></select><input v-model="feedbackComment" maxlength="2000" placeholder="补充判断依据（可选）" /><button class="secondary-button" :disabled="saving" @click="submitFeedback"><MessageSquareText :size="14" />提交反馈</button></div></section><section class="feedback-history"><article v-for="item in detail.feedback" :key="item.id"><span class="feedback-verdict" :class="item.verdict">{{ item.verdict }}</span><strong>{{ item.actor.name }}</strong><time>{{ formatTime(item.created_at) }}</time><p v-if="item.comment">{{ item.comment }}</p></article></section>
    </section></div>
  </ConsoleLayout>
</template>

<style scoped>
.diagnosis-export { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin: 16px 0; }
.diagnosis-export button { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; }
.diagnosis-export span { color: var(--text-secondary); font-size: 11px; }
.evidence-card {
  display: block;
}
.evidence-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.evidence-type {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 5px;
  background: var(--gray-1, #f1f1f1);
  color: var(--text-secondary, #444);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: none;
}
.evidence-source {
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 11px;
  color: var(--text-tertiary, #888);
  overflow-wrap: anywhere;
}
.evidence-fields {
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
}
.evidence-fields dt {
  color: var(--text-secondary, #555);
  font-size: 12px;
  font-weight: 500;
}
.evidence-fields dd {
  margin: 0;
  font-size: 12px;
  color: var(--text-primary, #222);
  overflow-wrap: anywhere;
}
.evidence-fields dd.is-timestamp {
  color: var(--text-tertiary, #888);
}
.evidence-raw-collapse {
  margin-top: 8px;
}
.evidence-raw-collapse summary {
  font-size: 12px;
  color: var(--text-tertiary, #888);
  cursor: pointer;
}
.evidence-raw-collapse pre {
  max-height: 240px;
  overflow: auto;
  font-size: 11px;
  line-height: 1.5;
}
</style>
