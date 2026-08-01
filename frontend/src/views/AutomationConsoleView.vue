<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  AlertTriangle,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  Cpu,
  GitCompare,
  Play,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Stethoscope,
  XCircle,
} from 'lucide-vue-next'

import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import {
  approveAutomationPlan,
  cancelAutomationPlan,
  executeAutomationPlan,
  getAutomationPlan,
  getAutomationVerification,
  listAutomationPlans,
  listAutomationRunbooks,
  previewAutomationPlan,
  verifyAutomationPlan,
} from '../api/aiops'
import { listClusters } from '../api/clusters'
import type {
  ActionPlanResponse,
  ActionVerification,
  EvidenceComparison,
  GateStatus,
  PlanLevel,
  PlanStatus,
  PolicyGate,
  Runbook,
  VerificationStatus,
} from '../types/aiops'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()

// Filters & plan list
const clusters = ref<Cluster[]>([])
const plans = ref<ActionPlanResponse[]>([])
const plansLoading = ref(false)
const plansError = ref('')
const statusFilter = ref<PlanStatus | ''>('')
const clusterFilter = ref<number | ''>('')

// Selected plan detail
const selectedPlanId = ref<string | null>(null)
const selectedPlan = ref<ActionPlanResponse | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

// Lifecycle action state
const actionLoading = ref(false)
const actionMessage = ref('')
const actionError = ref('')

// Execute form
const confirmationToken = ref('')
const idempotencyKey = ref('')

// Verification
const verification = ref<ActionVerification | null>(null)
const verificationLoading = ref(false)
const verificationError = ref('')

// Runbook catalog
const runbooks = ref<Runbook[]>([])
const automationVersion = ref('')
const runbooksLoading = ref(true)
const runbooksError = ref('')

const statusLabels: Record<PlanStatus, string> = {
  draft: '草稿',
  previewed: '已预览',
  approved: '已批准',
  executing: '执行中',
  succeeded: '成功',
  failed: '失败',
  expired: '已过期',
  cancelled: '已取消',
  verified: '已验证',
}

function statusClass(status: PlanStatus): string {
  switch (status) {
    case 'draft': return 'st-gray'
    case 'previewed': return 'st-blue'
    case 'approved': return 'st-blue'
    case 'executing': return 'st-amber'
    case 'succeeded': return 'st-green'
    case 'failed': return 'st-red'
    case 'expired': return 'st-gray'
    case 'cancelled': return 'st-gray'
    case 'verified': return 'st-green'
  }
}

const statusOptions: { value: PlanStatus | ''; label: string }[] = [
  { value: '', label: '全部状态' },
  { value: 'draft', label: '草稿' },
  { value: 'previewed', label: '已预览' },
  { value: 'approved', label: '已批准' },
  { value: 'executing', label: '执行中' },
  { value: 'succeeded', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'expired', label: '已过期' },
  { value: 'cancelled', label: '已取消' },
  { value: 'verified', label: '已验证' },
]

function levelClass(level: PlanLevel): string {
  switch (level) {
    case 'L0': return 'lv-green'
    case 'L1': return 'lv-blue'
    case 'L2': return 'lv-amber'
    case 'L3': return 'lv-red'
  }
}

const gateLabels: Record<GateStatus, string> = { passed: '通过', failed: '失败', skipped: '跳过' }

function gateClass(status: GateStatus): string {
  switch (status) {
    case 'passed': return 'gate-green'
    case 'failed': return 'gate-red'
    case 'skipped': return 'gate-gray'
  }
}

const verificationLabels: Record<VerificationStatus, string> = {
  pending: '待定',
  effective: '有效',
  ineffective: '无效',
  failed: '失败',
  unknown: '未知',
}

function verificationClass(status: VerificationStatus): string {
  switch (status) {
    case 'effective': return 'vrf-green'
    case 'pending': return 'vrf-amber'
    case 'ineffective': return 'vrf-red'
    case 'failed': return 'vrf-red'
    case 'unknown': return 'vrf-gray'
  }
}

const comparisonLabels: Record<EvidenceComparison, string> = {
  improved: '改善',
  unchanged: '未变',
  worse: '恶化',
  insufficient: '证据不足',
}

function comparisonClass(cmp: EvidenceComparison): string {
  switch (cmp) {
    case 'improved': return 'cmp-green'
    case 'unchanged': return 'cmp-gray'
    case 'worse': return 'cmp-red'
    case 'insufficient': return 'cmp-amber'
  }
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return value.replace('T', ' ').replace(/Z$/, ' UTC')
}

function clusterName(clusterId: number): string {
  return clusters.value.find((c) => c.id === clusterId)?.name ?? `集群 #${clusterId}`
}

function targetLabel(plan: ActionPlanResponse): string {
  return `${plan.target.kind}/${plan.target.name}`
}

function parametersJson(plan: ActionPlanResponse | null): string {
  if (!plan) return '{}'
  try {
    return JSON.stringify(plan.parameters ?? {}, null, 2)
  } catch {
    return '{}'
  }
}

function snapshotSignals(snap: { signals: string[] }): string {
  return snap.signals?.length ? snap.signals.join(', ') : '—'
}

const filteredPlans = computed(() => {
  let list = plans.value
  if (statusFilter.value !== '') {
    list = list.filter((p) => p.status === statusFilter.value)
  }
  if (clusterFilter.value !== '') {
    list = list.filter((p) => p.cluster_id === clusterFilter.value)
  }
  return list.slice(0, 50)
})

const canCancel = computed(() => {
  const s = selectedPlan.value?.status
  return s === 'draft' || s === 'previewed' || s === 'approved'
})

const failedGateCount = computed(() => {
  const gates = selectedPlan.value?.policy_gates ?? []
  return gates.filter((g: PolicyGate) => g.status === 'failed').length
})

async function loadClusters() {
  try {
    const resp = await listClusters(auth.accessToken)
    clusters.value = resp.items ?? []
  } catch {
    clusters.value = []
  }
}

async function loadPlans() {
  plansLoading.value = true
  plansError.value = ''
  try {
    const resp = await listAutomationPlans(auth.accessToken, { limit: 100 })
    plans.value = resp.items ?? []
  } catch (error) {
    plans.value = []
    plansError.value = error instanceof Error ? error.message : '无法加载自动化计划列表'
  } finally {
    plansLoading.value = false
  }
}

async function loadRunbooks() {
  runbooksLoading.value = true
  runbooksError.value = ''
  try {
    const resp = await listAutomationRunbooks(auth.accessToken)
    runbooks.value = resp.items ?? []
    automationVersion.value = resp.automation_version
  } catch (error) {
    runbooksError.value = error instanceof Error ? error.message : '无法加载 Runbook 目录'
  } finally {
    runbooksLoading.value = false
  }
}

async function selectPlan(plan: ActionPlanResponse) {
  selectedPlanId.value = plan.id
  actionMessage.value = ''
  actionError.value = ''
  verification.value = null
  verificationError.value = ''
  detailLoading.value = true
  detailError.value = ''
  selectedPlan.value = plan
  try {
    selectedPlan.value = await getAutomationPlan(auth.accessToken, plan.id)
    confirmationToken.value = selectedPlan.value.confirmation_token ?? ''
    if (selectedPlan.value.verification_id) {
      void loadVerification(plan.id)
    }
  } catch (error) {
    detailError.value = error instanceof Error ? error.message : '无法加载计划详情'
  } finally {
    detailLoading.value = false
  }
}

async function loadVerification(planId: string) {
  verificationLoading.value = true
  verificationError.value = ''
  try {
    verification.value = await getAutomationVerification(auth.accessToken, planId)
  } catch (error) {
    verification.value = null
    verificationError.value = error instanceof Error ? error.message : '无法加载验证结果'
  } finally {
    verificationLoading.value = false
  }
}

function resetActionFeedback() {
  actionMessage.value = ''
  actionError.value = ''
}

async function runPreview() {
  if (!selectedPlan.value) return
  actionLoading.value = true
  resetActionFeedback()
  try {
    const updated = await previewAutomationPlan(auth.accessToken, selectedPlan.value.id)
    selectedPlan.value = updated
    confirmationToken.value = updated.confirmation_token ?? ''
    actionMessage.value = '计划已预览，策略门禁已校验'
    void loadPlans()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : '预览失败'
  } finally {
    actionLoading.value = false
  }
}

async function runApprove() {
  if (!selectedPlan.value) return
  actionLoading.value = true
  resetActionFeedback()
  try {
    const updated = await approveAutomationPlan(auth.accessToken, selectedPlan.value.id)
    selectedPlan.value = updated
    confirmationToken.value = updated.confirmation_token ?? ''
    actionMessage.value = '计划已批准，可执行'
    void loadPlans()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : '审批失败'
  } finally {
    actionLoading.value = false
  }
}

async function runExecute() {
  if (!selectedPlan.value) return
  if (!confirmationToken.value) {
    actionError.value = '请输入确认令牌'
    return
  }
  actionLoading.value = true
  resetActionFeedback()
  try {
    const key = idempotencyKey.value || crypto.randomUUID()
    const updated = await executeAutomationPlan(auth.accessToken, selectedPlan.value.id, {
      confirmation_token: confirmationToken.value,
      idempotency_key: key,
    })
    selectedPlan.value = updated
    idempotencyKey.value = ''
    actionMessage.value = '计划已开始执行'
    void loadPlans()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : '执行失败'
  } finally {
    actionLoading.value = false
  }
}

async function runVerify() {
  if (!selectedPlan.value) return
  actionLoading.value = true
  resetActionFeedback()
  verificationError.value = ''
  try {
    const result = await verifyAutomationPlan(auth.accessToken, selectedPlan.value.id)
    verification.value = result
    actionMessage.value = '验证已完成'
    const refreshed = await getAutomationPlan(auth.accessToken, selectedPlan.value.id)
    selectedPlan.value = refreshed
    void loadPlans()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : '验证失败'
  } finally {
    actionLoading.value = false
  }
}

async function runCancel() {
  if (!selectedPlan.value) return
  actionLoading.value = true
  resetActionFeedback()
  try {
    const updated = await cancelAutomationPlan(auth.accessToken, selectedPlan.value.id)
    selectedPlan.value = updated
    actionMessage.value = '计划已取消'
    void loadPlans()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : '取消失败'
  } finally {
    actionLoading.value = false
  }
}

onMounted(() => {
  void loadClusters()
  void loadPlans()
  void loadRunbooks()
})
</script>

<template>
  <ConsoleLayout eyebrow="AIOps" title="自动化控制台">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="plansLoading" @click="loadPlans">
        <RefreshCw :size="16" :class="{ spinning: plansLoading }" />
        <span>刷新</span>
      </button>
    </template>

    <div class="two-column">
      <!-- Left: plan list with filters -->
      <section class="panel">
        <header class="panel-header">
          <div class="panel-title">
            <Cpu :size="18" />
            <strong>自动化计划</strong>
          </div>
        </header>

        <div class="plan-filters">
          <label class="filter-field">
            <span>状态</span>
            <select v-model="statusFilter" class="filter-select">
              <option v-for="opt in statusOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </label>
          <label class="filter-field">
            <span>集群</span>
            <select v-model.number="clusterFilter" class="filter-select">
              <option value="">全部集群</option>
              <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
          </label>
        </div>

        <div class="plan-list">
          <div v-if="plansLoading" class="panel-empty">加载中…</div>
          <div v-else-if="plansError" class="panel-empty error">{{ plansError }}</div>
          <div v-else-if="filteredPlans.length === 0" class="panel-empty muted">暂无匹配的自动化计划</div>
          <div v-else class="plan-items">
            <button
              v-for="plan in filteredPlans"
              :key="plan.id"
              type="button"
              class="plan-row"
              :class="{ active: selectedPlanId === plan.id }"
              @click="selectPlan(plan)"
            >
              <div class="plan-row-main">
                <div class="plan-row-head">
                  <strong class="mono">{{ plan.action_code }}</strong>
                  <span :class="['st-badge', statusClass(plan.status)]">{{ statusLabels[plan.status] }}</span>
                  <span :class="['lv-badge', levelClass(plan.level)]">{{ plan.level }}</span>
                </div>
                <span class="plan-target">{{ targetLabel(plan) }}</span>
                <span class="muted small">{{ clusterName(plan.cluster_id) }} · {{ formatTimestamp(plan.created_at) }}</span>
              </div>
              <ChevronRight :size="16" class="plan-chevron" />
            </button>
          </div>
        </div>
      </section>

      <!-- Right: plan detail -->
      <section class="panel detail-panel">
        <header class="panel-header sticky">
          <div class="panel-title">
            <ShieldCheck :size="18" />
            <strong>计划详情</strong>
          </div>
        </header>

        <!-- Empty state -->
        <div v-if="!selectedPlan" class="panel-empty muted">选择左侧计划查看详情</div>

        <div v-else class="plan-detail">
          <p v-if="actionMessage" class="success-banner">{{ actionMessage }}</p>
          <p v-if="actionError" class="error-text">{{ actionError }}</p>
          <p v-if="detailError" class="error-text">{{ detailError }}</p>

          <div v-if="detailLoading" class="detail-loading">加载详情中…</div>

          <template v-if="selectedPlan">
            <!-- Header meta -->
            <div class="detail-meta">
              <div class="meta-row">
                <span class="muted">动作编码</span>
                <strong class="mono">{{ selectedPlan.action_code }}</strong>
              </div>
              <div class="meta-row">
                <span class="muted">目标</span>
                <span class="mono">{{ selectedPlan.target.kind }}/{{ selectedPlan.target.name }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">命名空间</span>
                <span class="mono">{{ selectedPlan.target.namespace || '—' }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">集群</span>
                <span>{{ clusterName(selectedPlan.cluster_id) }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">状态</span>
                <span :class="['st-badge', statusClass(selectedPlan.status)]">{{ statusLabels[selectedPlan.status] }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">等级</span>
                <span :class="['lv-badge', levelClass(selectedPlan.level)]">{{ selectedPlan.level }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">审批类型</span>
                <span>{{ selectedPlan.approval_type === 'four_eyes' ? '四眼审批' : '单审批' }}</span>
              </div>
              <div class="meta-row">
                <span class="muted">创建时间</span>
                <span>{{ formatTimestamp(selectedPlan.created_at) }}</span>
              </div>
              <div class="meta-row" v-if="selectedPlan.approved_at">
                <span class="muted">批准时间</span>
                <span>{{ formatTimestamp(selectedPlan.approved_at) }}</span>
              </div>
              <div class="meta-row" v-if="selectedPlan.executed_at">
                <span class="muted">执行时间</span>
                <span>{{ formatTimestamp(selectedPlan.executed_at) }}</span>
              </div>
              <div class="meta-row" v-if="selectedPlan.expires_at">
                <span class="muted">过期时间</span>
                <span>{{ formatTimestamp(selectedPlan.expires_at) }}</span>
              </div>
              <div class="meta-row" v-if="selectedPlan.last_error">
                <span class="muted">最近错误</span>
                <span class="error-text">{{ selectedPlan.last_error }}</span>
              </div>
            </div>

            <!-- Policy gates -->
            <section class="detail-section">
              <div class="section-head">
                <ShieldCheck :size="16" />
                <h3>策略门禁 ({{ selectedPlan.policy_gates.length }})<span v-if="failedGateCount" class="gate-fail-count">{{ failedGateCount }} 失败</span></h3>
              </div>
              <div v-if="selectedPlan.policy_gates.length === 0" class="muted small">暂无策略门禁</div>
              <table v-else class="compact-table">
                <thead><tr><th>编码</th><th>状态</th><th>原因</th><th>检查时间</th><th>重检</th></tr></thead>
                <tbody>
                  <tr v-for="(gate, idx) in selectedPlan.policy_gates" :key="`gate-${idx}`">
                    <td class="mono">{{ gate.code }}</td>
                    <td><span :class="['gate-badge', gateClass(gate.status)]">{{ gateLabels[gate.status] }}</span></td>
                    <td>{{ gate.reason || '—' }}</td>
                    <td>{{ formatTimestamp(gate.checked_at) }}</td>
                    <td>{{ gate.rechecked ? '是' : '否' }}</td>
                  </tr>
                </tbody>
              </table>
            </section>

            <!-- Parameters -->
            <section class="detail-section">
              <div class="section-head"><Cpu :size="16" /><h3>操作参数</h3></div>
              <pre class="json-block">{{ parametersJson(selectedPlan) }}</pre>
            </section>

            <!-- Change info -->
            <section class="detail-section" v-if="selectedPlan.change">
              <div class="section-head"><GitCompare :size="16" /><h3>变更信息</h3></div>
              <div class="change-box">
                <div class="meta-row">
                  <span class="muted">变更类型</span>
                  <span class="mono">{{ selectedPlan.change.kind }}</span>
                </div>
                <div class="meta-row">
                  <span class="muted">变更目标</span>
                  <span class="mono">{{ selectedPlan.change.target.kind }}/{{ selectedPlan.change.target.name }}</span>
                </div>
                <div class="meta-row" v-if="selectedPlan.change.diff_summary">
                  <span class="muted">差异摘要</span>
                  <span>{{ selectedPlan.change.diff_summary }}</span>
                </div>
                <div class="meta-row">
                  <span class="muted">安全标记</span>
                  <span :class="['safe-badge', selectedPlan.change.safe ? 'safe-yes' : 'safe-no']">
                    {{ selectedPlan.change.safe ? '安全' : '不安全' }}
                  </span>
                </div>
              </div>
            </section>

            <!-- Confirmation token (approved state) -->
            <section class="detail-section" v-if="selectedPlan.status === 'approved'">
              <div class="section-head"><Sparkles :size="16" /><h3>执行确认</h3></div>
              <div class="execute-form">
                <label class="filter-field">
                  <span>确认令牌</span>
                  <input v-model="confirmationToken" type="text" class="filter-input" placeholder="输入确认令牌" />
                </label>
                <label class="filter-field">
                  <span>幂等键（可选）</span>
                  <input v-model="idempotencyKey" type="text" class="filter-input" placeholder="留空将自动生成" />
                </label>
              </div>
            </section>

            <!-- Lifecycle actions -->
            <section class="detail-section">
              <div class="section-head"><Play :size="16" /><h3>生命周期操作</h3></div>
              <div class="lifecycle-actions">
                <button
                  v-if="selectedPlan.status === 'draft'"
                  type="button"
                  class="primary-button"
                  :disabled="actionLoading"
                  @click="runPreview"
                >
                  <RefreshCw :size="16" />
                  <span>预览</span>
                </button>
                <button
                  v-if="selectedPlan.status === 'previewed'"
                  type="button"
                  class="primary-button"
                  :disabled="actionLoading"
                  @click="runApprove"
                >
                  <CheckCircle2 :size="16" />
                  <span>批准</span>
                </button>
                <button
                  v-if="selectedPlan.status === 'approved'"
                  type="button"
                  class="primary-button"
                  :disabled="actionLoading || !confirmationToken"
                  @click="runExecute"
                >
                  <Play :size="16" />
                  <span>{{ actionLoading ? '执行中…' : '执行' }}</span>
                </button>
                <button
                  v-if="selectedPlan.status === 'executing' || selectedPlan.status === 'succeeded'"
                  type="button"
                  class="secondary-button"
                  :disabled="actionLoading"
                  @click="runVerify"
                >
                  <Stethoscope :size="16" />
                  <span>{{ actionLoading ? '验证中…' : '验证' }}</span>
                </button>
                <button
                  v-if="canCancel"
                  type="button"
                  class="danger-button"
                  :disabled="actionLoading"
                  @click="runCancel"
                >
                  <XCircle :size="16" />
                  <span>取消</span>
                </button>
              </div>
            </section>

            <!-- Verification result -->
            <section class="detail-section" v-if="verification || verificationLoading || verificationError">
              <div class="section-head"><Stethoscope :size="16" /><h3>验证结果</h3></div>
              <div v-if="verificationLoading" class="detail-loading">加载验证结果中…</div>
              <p v-else-if="verificationError" class="error-text">{{ verificationError }}</p>
              <div v-else-if="verification" class="verification-box">
                <div class="meta-row">
                  <span class="muted">验证状态</span>
                  <span :class="['vrf-badge', verificationClass(verification.status)]">{{ verificationLabels[verification.status] }}</span>
                </div>
                <div class="meta-row">
                  <span class="muted">证据对比</span>
                  <span :class="['cmp-badge', comparisonClass(verification.evidence_comparison)]">{{ comparisonLabels[verification.evidence_comparison] }}</span>
                </div>
                <div class="meta-row" v-if="verification.reason">
                  <span class="muted">原因</span>
                  <span>{{ verification.reason }}</span>
                </div>
                <div class="meta-row">
                  <span class="muted">缺失证据</span>
                  <span>{{ verification.missing_evidence ? '是' : '否' }}</span>
                </div>
                <div class="meta-row" v-if="verification.verified_at">
                  <span class="muted">验证时间</span>
                  <span>{{ formatTimestamp(verification.verified_at) }}</span>
                </div>

                <!-- Snapshot comparison -->
                <div class="snapshot-grid">
                  <div class="snapshot-card">
                    <p class="muted small">执行前快照</p>
                    <span class="small">Signals: {{ snapshotSignals(verification.pre_snapshot) }}</span>
                    <span class="small" v-if="verification.pre_snapshot.slo_state">SLO: {{ verification.pre_snapshot.slo_state }}</span>
                    <span class="small" v-if="verification.pre_snapshot.topology_version">拓扑版本: {{ verification.pre_snapshot.topology_version }}</span>
                    <span class="muted small">{{ formatTimestamp(verification.pre_snapshot.captured_at) }}</span>
                  </div>
                  <div class="snapshot-card">
                    <p class="muted small">执行后快照</p>
                    <span class="small">Signals: {{ snapshotSignals(verification.post_snapshot) }}</span>
                    <span class="small" v-if="verification.post_snapshot.slo_state">SLO: {{ verification.post_snapshot.slo_state }}</span>
                    <span class="small" v-if="verification.post_snapshot.topology_version">拓扑版本: {{ verification.post_snapshot.topology_version }}</span>
                    <span class="muted small">{{ formatTimestamp(verification.post_snapshot.captured_at) }}</span>
                  </div>
                </div>

                <!-- Rollback info -->
                <div class="rollback-banner" v-if="verification.rollback_triggered">
                  <AlertTriangle :size="16" />
                  <span>已触发回滚<template v-if="verification.rollback_plan_id"> · 回滚计划: <strong class="mono">{{ verification.rollback_plan_id }}</strong></template></span>
                </div>
              </div>
            </section>
          </template>
        </div>
      </section>
    </div>

    <!-- Runbook catalog -->
    <section class="panel runbook-panel">
      <header class="panel-header">
        <div class="panel-title">
          <BookOpen :size="18" />
          <strong>自动化 Runbook 目录</strong>
          <span v-if="automationVersion" class="muted small">v{{ automationVersion }}</span>
        </div>
      </header>

      <div v-if="runbooksLoading" class="panel-empty">加载中…</div>
      <div v-else-if="runbooksError" class="panel-empty error">{{ runbooksError }}</div>
      <div v-else-if="runbooks.length === 0" class="panel-empty muted">暂无可用 Runbook</div>
      <div v-else class="runbook-grid">
        <article v-for="rb in runbooks" :key="rb.id" class="runbook-card">
          <div class="runbook-card-head">
            <strong>{{ rb.display_name }}</strong>
            <span :class="['lv-badge', levelClass(rb.level as PlanLevel)]">{{ rb.level }}</span>
          </div>
          <p class="runbook-desc">{{ rb.description }}</p>
          <div class="runbook-block">
            <p class="muted small">动作编码</p>
            <div class="chip-list">
              <span v-for="code in rb.action_codes" :key="code" class="chip">{{ code }}</span>
              <span v-if="rb.action_codes.length === 0" class="muted small">—</span>
            </div>
          </div>
          <div class="runbook-block">
            <p class="muted small">前置条件</p>
            <ul class="runbook-list">
              <li v-for="(pre, idx) in rb.prerequisites" :key="`pre-${idx}`">{{ pre }}</li>
              <li v-if="rb.prerequisites.length === 0" class="muted small">—</li>
            </ul>
          </div>
          <div class="runbook-block">
            <p class="muted small">回滚策略</p>
            <p class="runbook-rollback">{{ rb.rollback_strategy || '—' }}</p>
          </div>
        </article>
      </div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
/* Filters */
.plan-filters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  padding: 14px;
  border-bottom: 1px solid var(--border-soft);
}
.filter-field { display: flex; flex-direction: column; gap: 5px; font-size: 12px; color: var(--text-muted); }
.filter-select, .filter-input {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-inset);
}
.filter-input { font-family: var(--font-mono); }
.filter-select:focus, .filter-input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}

/* Plan list */
.plan-list { padding: 8px; }
.plan-items { display: flex; flex-direction: column; gap: 6px; }
.plan-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  text-align: left;
  padding: 11px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}
.plan-row:hover { background: var(--bg-tertiary); }
.plan-row.active { border-color: var(--accent-primary); box-shadow: 0 0 0 2px var(--accent-soft); }
.plan-row-main { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.plan-row-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.plan-row-head strong { color: var(--text-primary); font-size: 13px; }
.plan-target { color: var(--text-secondary); font-size: 12px; font-family: var(--font-mono); }
.plan-chevron { color: var(--text-muted); flex: 0 0 auto; margin-top: 2px; }

/* Status / level badges */
.st-badge, .lv-badge, .gate-badge, .vrf-badge, .cmp-badge, .safe-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-full);
}
.st-gray { color: var(--text-secondary); background: var(--bg-tertiary); }
.st-blue { color: var(--status-info); background: var(--info-bg); }
.st-amber { color: var(--status-warning); background: var(--warning-bg); }
.st-green { color: var(--status-success); background: var(--success-bg); }
.st-red { color: var(--status-danger); background: var(--danger-bg); }

.lv-green { color: var(--status-success); background: var(--success-bg); }
.lv-blue { color: var(--status-info); background: var(--info-bg); }
.lv-amber { color: var(--status-warning); background: var(--warning-bg); }
.lv-red { color: var(--status-danger); background: var(--danger-bg); }

.gate-green { color: var(--status-success); background: var(--success-bg); }
.gate-red { color: var(--status-danger); background: var(--danger-bg); }
.gate-gray { color: var(--text-secondary); background: var(--bg-tertiary); }

.gate-fail-count {
  margin-left: 8px;
  padding: 1px 7px;
  color: var(--status-danger);
  font-size: 11px;
  font-weight: 700;
  background: var(--danger-bg);
  border-radius: var(--radius-full);
}

.vrf-green { color: var(--status-success); background: var(--success-bg); }
.vrf-amber { color: var(--status-warning); background: var(--warning-bg); }
.vrf-red { color: var(--status-danger); background: var(--danger-bg); }
.vrf-gray { color: var(--text-secondary); background: var(--bg-tertiary); }

.cmp-green { color: var(--status-success); background: var(--success-bg); }
.cmp-gray { color: var(--text-secondary); background: var(--bg-tertiary); }
.cmp-red { color: var(--status-danger); background: var(--danger-bg); }
.cmp-amber { color: var(--status-warning); background: var(--warning-bg); }

.safe-yes { color: var(--status-success); background: var(--success-bg); }
.safe-no { color: var(--status-danger); background: var(--danger-bg); }

/* Detail panel */
.plan-detail { display: flex; flex-direction: column; gap: 18px; padding: 16px; }

.detail-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 18px;
  padding: 14px;
  background: var(--bg-secondary);
  border-radius: var(--radius-lg);
}
.meta-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }

.detail-section { display: flex; flex-direction: column; gap: 8px; }
.section-head { display: flex; align-items: center; gap: 8px; color: var(--text-primary); }
.section-head h3 { margin: 0; font-size: 14px; }

.json-block {
  margin: 0;
  padding: 14px;
  overflow-x: auto;
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}

.change-box, .verification-box {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.execute-form { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }

.lifecycle-actions { display: flex; flex-wrap: wrap; gap: 8px; }

.snapshot-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 6px; }
.snapshot-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}

.rollback-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  color: var(--status-warning);
  font-size: 13px;
  background: var(--warning-bg);
  border-left: 3px solid var(--status-warning);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.success-banner {
  margin: 0;
  padding: 9px 12px;
  color: var(--status-success);
  font-size: 12px;
  background: var(--success-bg);
  border-left: 3px solid var(--status-success);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.error-text {
  margin: 0;
  padding: 9px 12px;
  color: var(--status-danger);
  font-size: 12px;
  background: var(--danger-bg);
  border-left: 3px solid var(--status-danger);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}

.detail-loading { color: var(--text-secondary); font-size: 12px; }

/* Runbook catalog */
.runbook-panel { margin-top: 18px; }
.runbook-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
  padding: 16px;
}
.runbook-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}
.runbook-card-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.runbook-card-head strong { color: var(--text-primary); font-size: 14px; }
.runbook-desc { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.55; }
.runbook-block { display: flex; flex-direction: column; gap: 5px; }
.chip-list { display: flex; flex-wrap: wrap; gap: 6px; }
.chip {
  padding: 2px 8px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}
.runbook-list { margin: 0; padding-left: 18px; color: var(--text-secondary); font-size: 12px; line-height: 1.6; }
.runbook-rollback { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.55; }

.mono { font-family: var(--font-mono); }
.small { font-size: 12px; }

@media (max-width: 900px) {
  .detail-meta { grid-template-columns: 1fr; }
  .plan-filters { grid-template-columns: 1fr; }
  .execute-form { grid-template-columns: 1fr; }
  .snapshot-grid { grid-template-columns: 1fr; }
}
</style>
