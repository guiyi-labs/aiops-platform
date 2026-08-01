<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import {
  Activity,
  BellRing,
  BoxIcon,
  ChevronDown,
  Inbox,
  Plus,
  Radio,
  RefreshCw,
  ShieldAlert,
  Square,
  Trash2,
} from 'lucide-vue-next'

import { buildEventStreamUrl, createInhibit, deleteInhibit, listAlertDeliveries, listInhibits } from '../api/eventstream'
import { listClusters } from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { AlertDeliveryView, EventSummary, InhibitView } from '../types/eventstream'
import type { Cluster } from '../types/cluster'

type SseStatus = 'disconnected' | 'connected' | 'error'
type TabKey = 'stream' | 'routes'

const auth = useAuthStore()
const activeTab = ref<TabKey>('stream')

// Cluster context
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)

// Event stream state
const namespace = ref('')
const eventSource = ref<EventSource | null>(null)
const sseStatus = ref<SseStatus>('disconnected')
const events = ref<EventSummary[]>([])
const sseHint = ref('')

// Alert routes state
const inhibits = ref<InhibitView[]>([])
const inhibitsLoading = ref(false)
const inhibitError = ref('')
const showCreateForm = ref(false)
const creatingInhibit = ref(false)
const newInhibit = ref({
  source_rule_name: '',
  source_severity: 'warning',
  target_rule_name: '',
  target_severity: 'critical',
  reason: '',
  enabled: true,
})

const deliveries = ref<AlertDeliveryView[]>([])
const deliveriesLoading = ref(false)
const deliveryError = ref('')
const deliveryStatus = ref('')

function formatTime(value?: string): string {
  if (!value) return '--'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(d)
}

function eventTypeClass(type: string): string {
  if (type === 'Warning') return 'ev-warning'
  if (type === 'Normal') return 'ev-normal'
  return 'ev-other'
}

function deliveryStatusClass(status: string): string {
  if (status === 'pending') return 'dlv-pending'
  if (status === 'delivered') return 'dlv-delivered'
  if (status === 'failed') return 'dlv-failed'
  return 'dlv-other'
}

function connect() {
  disconnect()
  const clusterID = selectedClusterID.value
  if (!clusterID || !auth.accessToken) {
    sseHint.value = '请先选择集群'
    return
  }
  sseHint.value = ''
  const url = buildEventStreamUrl(clusterID, auth.accessToken, { namespace: namespace.value.trim() || undefined })
  const source = new EventSource(url)
  eventSource.value = source
  source.onopen = () => {
    sseStatus.value = 'connected'
  }
  source.onerror = () => {
    sseStatus.value = 'error'
    source.close()
    eventSource.value = null
  }
  source.onmessage = (ev: MessageEvent) => {
    try {
      const summary = JSON.parse(ev.data) as EventSummary
      events.value.unshift(summary)
      if (events.value.length > 200) {
        events.value.length = 200
      }
    } catch {
      // ignore unparseable payloads
    }
  }
}

function disconnect() {
  if (eventSource.value) {
    eventSource.value.close()
    eventSource.value = null
  }
  sseStatus.value = 'disconnected'
}

async function loadClusters() {
  try {
    const result = await listClusters(auth.accessToken)
    clusters.value = result.items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch {
    sseHint.value = '无法加载集群列表'
  }
}

async function loadInhibits() {
  inhibitsLoading.value = true
  inhibitError.value = ''
  try {
    const resp = await listInhibits(auth.accessToken)
    inhibits.value = resp.items ?? []
  } catch (err) {
    inhibits.value = []
    inhibitError.value = err instanceof Error ? err.message : '无法加载告警抑制规则'
  } finally {
    inhibitsLoading.value = false
  }
}

async function handleCreateInhibit() {
  if (!newInhibit.value.source_rule_name || !newInhibit.value.target_rule_name) return
  creatingInhibit.value = true
  inhibitError.value = ''
  try {
    await createInhibit(auth.accessToken, {
      source_rule_name: newInhibit.value.source_rule_name.trim(),
      source_severity: newInhibit.value.source_severity.trim(),
      target_rule_name: newInhibit.value.target_rule_name.trim(),
      target_severity: newInhibit.value.target_severity.trim(),
      reason: newInhibit.value.reason.trim(),
      enabled: newInhibit.value.enabled,
    })
    showCreateForm.value = false
    newInhibit.value = { source_rule_name: '', source_severity: 'warning', target_rule_name: '', target_severity: 'critical', reason: '', enabled: true }
    await loadInhibits()
  } catch (err) {
    inhibitError.value = err instanceof Error ? err.message : '创建抑制规则失败'
  } finally {
    creatingInhibit.value = false
  }
}

async function handleDeleteInhibit(inhibit: InhibitView) {
  inhibitError.value = ''
  try {
    await deleteInhibit(auth.accessToken, inhibit.id)
    await loadInhibits()
  } catch (err) {
    inhibitError.value = err instanceof Error ? err.message : '删除抑制规则失败'
  }
}

async function loadDeliveries() {
  deliveriesLoading.value = true
  deliveryError.value = ''
  try {
    const resp = await listAlertDeliveries(auth.accessToken, { status: deliveryStatus.value || undefined, limit: 100 })
    deliveries.value = resp.items ?? []
  } catch (err) {
    deliveries.value = []
    deliveryError.value = err instanceof Error ? err.message : '无法加载告警投递记录'
  } finally {
    deliveriesLoading.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadInhibits(), loadDeliveries()])
}

// Cluster change tears down the stream; namespace change reconnects if live.
watch(selectedClusterID, () => {
  disconnect()
})

watch(namespace, () => {
  if (eventSource.value) connect()
})

onMounted(() => {
  void loadClusters()
  void loadInhibits()
  void loadDeliveries()
})

onUnmounted(disconnect)
</script>

<template>
  <ConsoleLayout eyebrow="可观测性" title="事件流与告警">
    <template #actions>
      <button type="button" class="secondary-button" :disabled="inhibitsLoading || deliveriesLoading" @click="refreshAll">
        <RefreshCw :size="15" :class="{ spinning: inhibitsLoading || deliveriesLoading }" />刷新
      </button>
    </template>

    <nav class="eventstream-tabs" aria-label="视图切换">
      <button type="button" :class="{ active: activeTab === 'stream' }" @click="activeTab = 'stream'">
        <Radio :size="15" /><span>实时事件流</span>
      </button>
      <button type="button" :class="{ active: activeTab === 'routes' }" @click="activeTab = 'routes'">
        <BellRing :size="15" /><span>告警路由</span>
      </button>
    </nav>

    <!-- Tab 1: Event Stream -->
    <section v-if="activeTab === 'stream'" class="eventstream-tab">
      <section class="resource-toolbar eventstream-toolbar">
        <select v-model="selectedClusterID" aria-label="选择集群">
          <option :value="null" disabled>选择已启用集群</option>
          <option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
        <input v-model="namespace" type="text" placeholder="Namespace 过滤（可选）" aria-label="Namespace 过滤" />
        <span class="sse-status" :class="sseStatus">
          <span class="sse-dot" />
          {{ sseStatus === 'connected' ? '已连接' : sseStatus === 'error' ? '连接错误' : '未连接' }}
        </span>
        <button v-if="sseStatus !== 'connected'" type="button" class="primary-button" :disabled="!selectedClusterID" @click="connect">
          <Radio :size="15" />连接
        </button>
        <button v-else type="button" class="secondary-button" @click="disconnect">
          <Square :size="15" />断开
        </button>
      </section>

      <p v-if="sseHint" class="error-message">{{ sseHint }}</p>

      <div v-if="clusters.length === 0" class="resource-empty">
        <BoxIcon :size="30" />
        <strong>没有已启用的集群</strong>
        <span>请先接入并启用集群，再订阅实时事件流。</span>
      </div>

      <section v-else class="resource-panel eventstream-panel">
        <div class="section-heading">
          <div>
            <p class="context-label">LIVE EVENTS</p>
            <h2>事件流 · {{ events.length }}</h2>
          </div>
          <span class="eventstream-meta">最多保留最近 200 条 · 新事件置顶</span>
        </div>

        <div v-if="events.length === 0" class="empty-state">
          <Activity :size="28" />
          <strong>暂无实时事件</strong>
          <span>选择集群并点击「连接」开始订阅 Kubernetes 事件。</span>
        </div>
        <div v-else class="eventstream-list">
          <article v-for="event in events" :key="event.uid || `${event.kind}-${event.name}-${event.occurred_at}`" class="eventstream-row">
            <span class="event-type-badge" :class="eventTypeClass(event.type)">{{ event.type || 'Normal' }}</span>
            <div class="eventstream-row-main">
              <div class="eventstream-row-head">
                <strong>{{ event.kind }}/{{ event.name }}</strong>
                <span class="eventstream-reason">{{ event.reason || '—' }}</span>
              </div>
              <p class="eventstream-message">{{ event.message || '--' }}</p>
              <div class="eventstream-row-meta">
                <span>{{ event.namespace || 'cluster-scoped' }}</span>
                <span>×{{ event.count || 1 }}</span>
                <time>{{ formatTime(event.last_timestamp || event.occurred_at) }}</time>
              </div>
            </div>
          </article>
        </div>
      </section>
    </section>

    <!-- Tab 2: Alert Routes -->
    <section v-else class="eventstream-tab">
      <!-- Inhibits -->
      <section class="resource-panel inhibit-panel">
        <div class="section-heading">
          <div>
            <p class="context-label">ALERT INHIBITS</p>
            <h2>告警抑制 · {{ inhibits.length }}</h2>
          </div>
          <button type="button" class="secondary-button" :disabled="inhibitsLoading" @click="loadInhibits">
            <RefreshCw :size="15" :class="{ spinning: inhibitsLoading }" />刷新
          </button>
        </div>

        <p v-if="inhibitError" class="error-message">{{ inhibitError }}</p>

        <div class="inhibit-actions">
          <button type="button" class="primary-button" @click="showCreateForm = !showCreateForm">
            <Plus :size="15" />{{ showCreateForm ? '收起表单' : '创建抑制规则' }}
            <ChevronDown :size="14" :class="{ rotated: showCreateForm }" />
          </button>
        </div>

        <form v-if="showCreateForm" class="inhibit-form" @submit.prevent="handleCreateInhibit">
          <label class="inhibit-field">
            <span>源规则名称 <em>*</em></span>
            <input v-model="newInhibit.source_rule_name" type="text" placeholder="如 HighCPU" required />
          </label>
          <label class="inhibit-field">
            <span>源严重级别</span>
            <input v-model="newInhibit.source_severity" type="text" placeholder="warning" />
          </label>
          <label class="inhibit-field">
            <span>目标规则名称 <em>*</em></span>
            <input v-model="newInhibit.target_rule_name" type="text" placeholder="如 NodeNotReady" required />
          </label>
          <label class="inhibit-field">
            <span>目标严重级别</span>
            <input v-model="newInhibit.target_severity" type="text" placeholder="critical" />
          </label>
          <label class="inhibit-field inhibit-field-reason">
            <span>原因</span>
            <input v-model="newInhibit.reason" type="text" placeholder="说明抑制理由" />
          </label>
          <button type="submit" class="primary-button" :disabled="creatingInhibit || !newInhibit.source_rule_name || !newInhibit.target_rule_name">
            {{ creatingInhibit ? '创建中…' : '创建' }}
          </button>
        </form>

        <div v-if="inhibits.length === 0" class="empty-state inhibit-empty">
          <ShieldAlert :size="26" />
          <strong>暂无抑制规则</strong>
          <span>创建抑制规则可在源告警触发时静默目标告警。</span>
        </div>
        <div v-else class="inhibit-list">
          <article v-for="inhibit in inhibits" :key="inhibit.id" class="inhibit-row">
            <div class="inhibit-flow">
              <strong>{{ inhibit.source_rule_name }}</strong>
              <span class="inhibit-severity">{{ inhibit.source_severity || '—' }}</span>
              <span class="inhibit-arrow">→</span>
              <strong>{{ inhibit.target_rule_name }}</strong>
              <span class="inhibit-severity">{{ inhibit.target_severity || '—' }}</span>
            </div>
            <p v-if="inhibit.reason" class="inhibit-reason">{{ inhibit.reason }}</p>
            <div class="inhibit-row-actions">
              <span class="toggle" :class="{ on: inhibit.enabled }" :title="inhibit.enabled ? '已启用' : '已禁用'">
                <span class="toggle-knob" />
              </span>
              <button type="button" class="danger-button" :title="'删除'" @click="handleDeleteInhibit(inhibit)">
                <Trash2 :size="14" />
              </button>
            </div>
          </article>
        </div>
      </section>

      <!-- Deliveries -->
      <section class="resource-panel delivery-panel">
        <div class="section-heading">
          <div>
            <p class="context-label">ALERT DELIVERIES</p>
            <h2>告警投递 · {{ deliveries.length }}</h2>
          </div>
          <div class="delivery-toolbar">
            <select v-model="deliveryStatus" aria-label="按投递状态筛选" @change="loadDeliveries">
              <option value="">全部状态</option>
              <option value="pending">pending</option>
              <option value="delivered">delivered</option>
              <option value="failed">failed</option>
            </select>
            <button type="button" class="secondary-button" :disabled="deliveriesLoading" @click="loadDeliveries">
              <RefreshCw :size="15" :class="{ spinning: deliveriesLoading }" />刷新
            </button>
          </div>
        </div>

        <p v-if="deliveryError" class="error-message">{{ deliveryError }}</p>

        <div v-if="deliveriesLoading" class="empty-state"><RefreshCw class="spinning" :size="24" /><span>正在加载投递记录</span></div>
        <div v-else-if="deliveries.length === 0" class="empty-state">
          <Inbox :size="28" />
          <strong>暂无投递记录</strong>
          <span>调整状态筛选或等待告警路由产生新的投递。</span>
        </div>
        <div v-else class="delivery-table-scroll">
          <table class="compact-table delivery-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>事件类型</th>
                <th>状态</th>
                <th>去重键</th>
                <th>尝试</th>
                <th>下次尝试</th>
                <th>送达时间</th>
                <th>最近错误</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in deliveries" :key="item.id">
                <td>{{ item.id }}</td>
                <td>{{ item.event_type }}</td>
                <td><span class="delivery-status" :class="deliveryStatusClass(item.status)">{{ item.status }}</span></td>
                <td class="mono">{{ item.dedupe_key || '—' }}</td>
                <td>{{ item.attempts }}</td>
                <td>{{ formatTime(item.next_attempt_at) }}</td>
                <td>{{ formatTime(item.delivered_at) }}</td>
                <td class="delivery-error" :title="item.last_error || ''">{{ item.last_error || '—' }}</td>
                <td>{{ formatTime(item.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.eventstream-tabs {
  display: flex;
  gap: 4px;
  margin-top: 18px;
  padding: 4px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
  width: fit-content;
}

.eventstream-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 0 16px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
}

.eventstream-tabs button:hover {
  color: var(--text-primary);
}

.eventstream-tabs button.active {
  color: var(--text-primary);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-sm);
}

.eventstream-toolbar {
  grid-template-columns: minmax(180px, 240px) minmax(180px, 1fr) auto auto;
  align-items: center;
}

.sse-status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 12px;
  height: 36px;
  font-size: 12px;
  font-weight: 600;
  border-radius: var(--radius-md);
  white-space: nowrap;
}

.sse-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}

.sse-status.connected {
  color: var(--status-success);
  background: var(--success-bg);
}

.sse-status.connected .sse-dot {
  background: var(--status-success);
}

.sse-status.disconnected {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}

.sse-status.error {
  color: var(--status-danger);
  background: var(--danger-bg);
}

.sse-status.error .sse-dot {
  background: var(--status-danger);
}

.eventstream-meta {
  color: var(--text-tertiary);
  font-size: 11px;
  white-space: nowrap;
}

.eventstream-panel {
  margin-top: 14px;
  margin-bottom: 30px;
}

.eventstream-list {
  margin-top: 12px;
  max-height: 560px;
  overflow-y: auto;
}

.eventstream-row {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  gap: 14px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.eventstream-row:last-child {
  border-bottom: 0;
}

.event-type-badge {
  display: inline-flex;
  width: fit-content;
  height: 24px;
  padding: 0 9px;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-sm);
}

.ev-warning {
  color: var(--status-warning);
  background: var(--warning-bg);
}

.ev-normal {
  color: var(--status-success);
  background: var(--success-bg);
}

.ev-other {
  color: var(--status-info);
  background: var(--info-bg);
}

.eventstream-row-main {
  display: grid;
  gap: 5px;
  min-width: 0;
}

.eventstream-row-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.eventstream-row-head strong {
  color: var(--text-primary);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.eventstream-reason {
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.eventstream-message {
  margin: 0;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.eventstream-row-meta {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  color: var(--text-tertiary);
  font-size: 11px;
}

.eventstream-row-meta time {
  color: var(--text-muted);
}

/* Inhibits */
.inhibit-panel {
  margin-top: 14px;
  margin-bottom: 18px;
}

.inhibit-actions {
  margin-top: 12px;
}

.inhibit-actions .primary-button .rotated,
.inhibit-actions .rotated {
  transform: rotate(180deg);
}

.inhibit-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr)) minmax(0, 1.4fr) auto;
  gap: 10px;
  align-items: end;
  margin-top: 12px;
  padding: 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.inhibit-field {
  display: grid;
  gap: 5px;
  min-width: 0;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.inhibit-field em {
  color: var(--status-danger);
  font-style: normal;
}

.inhibit-field-reason {
  grid-column: span 2;
}

.inhibit-field input {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-inset);
  outline: none;
}

.inhibit-field input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.inhibit-empty {
  margin-top: 14px;
}

.inhibit-list {
  display: grid;
  gap: 8px;
  margin-top: 14px;
}

.inhibit-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  padding: 12px 14px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  align-items: center;
}

.inhibit-flow {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.inhibit-flow strong {
  color: var(--text-primary);
  font-size: 12px;
}

.inhibit-severity {
  padding: 2px 7px;
  color: var(--text-secondary);
  font-size: 10px;
  font-weight: 600;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
}

.inhibit-arrow {
  color: var(--text-muted);
  font-size: 12px;
}

.inhibit-reason {
  grid-column: 1;
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.5;
}

.inhibit-row-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.toggle {
  display: inline-flex;
  width: 34px;
  height: 18px;
  padding: 2px;
  align-items: center;
  background: var(--border-default);
  border-radius: var(--radius-full);
  transition: background var(--transition-fast);
}

.toggle.on {
  background: var(--status-success);
}

.toggle-knob {
  display: block;
  width: 14px;
  height: 14px;
  background: #ffffff;
  border-radius: 50%;
  transition: transform var(--transition-fast);
}

.toggle.on .toggle-knob {
  transform: translateX(16px);
}

/* Deliveries */
.delivery-panel {
  margin-bottom: 30px;
}

.delivery-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.delivery-toolbar select {
  height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  font-size: 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
}

.delivery-table-scroll {
  margin-top: 12px;
  overflow-x: auto;
}

.delivery-table {
  min-width: 980px;
}

.delivery-status {
  display: inline-flex;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 700;
  border-radius: var(--radius-full);
}

.dlv-pending {
  color: var(--status-warning);
  background: var(--warning-bg);
}

.dlv-delivered {
  color: var(--status-success);
  background: var(--success-bg);
}

.dlv-failed {
  color: var(--status-danger);
  background: var(--danger-bg);
}

.dlv-other {
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}

.delivery-error {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: var(--font-mono);
}

@media (max-width: 1000px) {
  .inhibit-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .inhibit-field-reason {
    grid-column: span 2;
  }
  .inhibit-form .primary-button {
    grid-column: 1 / -1;
  }
}

@media (max-width: 720px) {
  .eventstream-toolbar {
    grid-template-columns: 1fr;
  }
  .eventstream-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .inhibit-form {
    grid-template-columns: 1fr;
  }
  .inhibit-field-reason {
    grid-column: span 1;
  }
  .inhibit-row {
    grid-template-columns: 1fr;
  }
}
</style>
