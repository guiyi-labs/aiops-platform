<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Bell, BoxIcon, Clock3, Info, RefreshCw, Search, TriangleAlert, X } from 'lucide-vue-next'

import { listClusters } from '../api/clusters'
import { getEventCockpit, listEvents, listNamespaces } from '../api/kubernetes'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { EventCockpitResponse, KubernetesEvent, Namespace } from '../types/kubernetes'

type EventTypeFilter = 'all' | 'Warning' | 'Normal'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selectedClusterID = ref<number | null>(null)
const namespaces = ref<Namespace[]>([])
const namespace = ref('')
const eventType = ref<EventTypeFilter>('all')
const resourceKind = ref('')
const searchText = ref('')
const appliedSearch = ref('')
const events = ref<KubernetesEvent[]>([])
const total = ref(0)
const loading = ref(false)
const errorMessage = ref('')
const selectedEvent = ref<KubernetesEvent | null>(null)
const lastSyncedAt = ref<Date | null>(null)
const cockpit = ref<EventCockpitResponse | null>(null)
const cockpitLoading = ref(false)
const cockpitWindowMinutes = ref(1440)
let loadSequence = 0

const resourceKinds = computed(() => [...new Set(events.value.map((item) => item.involvedObject.kind).filter(Boolean))].sort())
const filteredEvents = computed(() => events.value
  .filter((item) => eventType.value === 'all' || item.type === eventType.value)
  .filter((item) => !resourceKind.value || item.involvedObject.kind === resourceKind.value)
  .sort((left, right) => eventTimeValue(right) - eventTimeValue(left)))
const cockpitWarningCount = computed(() => cockpit.value?.groups.filter((g) => g.severity === 'warning').length ?? 0)
const cockpitNormalCount = computed(() => cockpit.value?.groups.filter((g) => g.severity === 'info').length ?? 0)
const lastSyncedLabel = computed(() => lastSyncedAt.value
  ? new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(lastSyncedAt.value)
  : '--')

function eventTimestamp(event: KubernetesEvent): string | undefined {
  return event.series?.lastObservedTime || event.eventTime || event.lastTimestamp || event.firstTimestamp || event.metadata.creationTimestamp
}

function eventTimeValue(event: KubernetesEvent): number {
  const value = eventTimestamp(event)
  return value ? new Date(value).getTime() || 0 : 0
}

function formatTime(value?: string): string {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(date)
}

function occurrenceCount(event: KubernetesEvent): number {
  return event.series?.count || event.count || 1
}

async function initialize() {
  try {
    clusters.value = (await listClusters(auth.accessToken)).items.filter((item) => item.enabled)
    selectedClusterID.value = clusters.value[0]?.id ?? null
  } catch {
    errorMessage.value = '无法加载集群列表'
  }
}

async function loadClusterContext() {
  const clusterID = selectedClusterID.value
  if (!clusterID) {
    namespaces.value = []
    events.value = []
    total.value = 0
    return
  }
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const [namespaceResponse, eventResponse] = await Promise.all([
      listNamespaces(auth.accessToken, clusterID),
      listEvents(auth.accessToken, clusterID, namespace.value, appliedSearch.value),
    ])
    if (sequence !== loadSequence) return
    namespaces.value = namespaceResponse.items
    events.value = eventResponse.items
    total.value = eventResponse.total
    lastSyncedAt.value = new Date()
    if (resourceKind.value && !resourceKinds.value.includes(resourceKind.value)) resourceKind.value = ''
  } catch {
    if (sequence === loadSequence) errorMessage.value = '事件读取失败，请确认集群可达且观察账号具有 events 读取权限'
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

function applySearch() {
  appliedSearch.value = searchText.value.trim()
  void loadClusterContext()
}

async function loadCockpit() {
  const clusterID = selectedClusterID.value
  if (!clusterID) return
  cockpitLoading.value = true
  try {
    cockpit.value = await getEventCockpit(auth.accessToken, clusterID, { window_minutes: cockpitWindowMinutes.value })
  } catch {
    cockpit.value = null
  } finally {
    cockpitLoading.value = false
  }
}

function cockpitSeverityClass(sev: string): string {
  return sev === 'warning' ? 'warning' : 'normal'
}

function trendBarHeight(events: number): string {
  const max = cockpit.value?.trend.reduce((acc, p) => Math.max(acc, p.events), 0) ?? 1
  const pct = max > 0 ? Math.max((events / max) * 100, 6) : 6
  return `${pct}%`
}

watch(selectedClusterID, () => {
  namespace.value = ''
  resourceKind.value = ''
  appliedSearch.value = ''
  searchText.value = ''
  void loadClusterContext()
  void loadCockpit()
})
watch(namespace, () => {
  resourceKind.value = ''
  void loadClusterContext()
})
watch(cockpitWindowMinutes, () => void loadCockpit())
onMounted(async () => {
  await initialize()
  void loadCockpit()
})
</script>

<template>
  <ConsoleLayout eyebrow="可观测性" title="事件中心">
    <section class="resource-toolbar event-toolbar" aria-label="事件筛选">
      <select v-model="selectedClusterID" aria-label="选择集群"><option :value="null" disabled>选择已启用集群</option><option v-for="item in clusters" :key="item.id" :value="item.id">{{ item.name }}</option></select>
      <select v-model="namespace" aria-label="Namespace"><option value="">全部 Namespace</option><option v-for="item in namespaces" :key="item.metadata.name" :value="item.metadata.name">{{ item.metadata.name }}</option></select>
      <select v-model="eventType" aria-label="事件类型"><option value="all">全部类型</option><option value="Warning">Warning</option><option value="Normal">Normal</option></select>
      <select v-model="resourceKind" aria-label="资源类型"><option value="">全部资源</option><option v-for="kind in resourceKinds" :key="kind" :value="kind">{{ kind }}</option></select>
      <label class="search-field"><Search :size="15" /><input v-model="searchText" placeholder="按资源名称筛选" @keyup.enter="applySearch" /></label>
      <button class="secondary-button" type="button" :disabled="loading || !selectedClusterID" @click="applySearch"><RefreshCw :size="15" :class="{ spinning: loading }" />查询</button>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <div v-if="clusters.length === 0" class="resource-empty"><BoxIcon :size="30" /><strong>没有已启用的集群</strong><span>请先接入并启用集群，再查看 Kubernetes 原生事件。</span></div>
    <template v-else>
      <section class="resource-summary-grid event-summary-grid" aria-label="事件摘要">
        <article><span>当前范围</span><strong>{{ total }}</strong><small>最多展示最近 100 条</small></article>
        <article class="warning-summary"><span>Warning 组</span><strong>{{ cockpitWarningCount }}</strong><small>重复事件聚合组</small></article>
        <article><span>Normal 组</span><strong>{{ cockpitNormalCount }}</strong><small>重复事件聚合组</small></article>
        <article><span>原始事件</span><strong>{{ cockpit?.total_events ?? 0 }}</strong><small>窗口内去重折叠前</small></article>
      </section>

      <section class="event-cockpit-panel" aria-label="事件驾驶舱">
        <div class="section-heading cockpit-heading">
          <div><p class="context-label">EVENT COCKPIT · {{ cockpit?.window_minutes ?? 1440 }} MIN</p><h2>事件驾驶舱</h2></div>
          <div class="cockpit-controls">
            <label class="window-select">
              <span>窗口</span>
              <select v-model.number="cockpitWindowMinutes" aria-label="聚合窗口">
                <option :value="60">1 小时</option>
                <option :value="360">6 小时</option>
                <option :value="1440">24 小时</option>
                <option :value="10080">7 天</option>
              </select>
            </label>
            <button class="icon-button" type="button" title="刷新驾驶舱" aria-label="刷新驾驶舱" :disabled="cockpitLoading || !selectedClusterID" @click="loadCockpit"><RefreshCw :size="15" :class="{ spinning: cockpitLoading }" /></button>
          </div>
        </div>

        <p v-if="cockpit?.fail_closed" class="cockpit-failclosed">{{ cockpit.empty_note }}</p>
        <div v-if="cockpitLoading" class="empty-state event-loading"><RefreshCw class="spinning" :size="22" /><span>正在聚合事件</span></div>
        <div v-else-if="cockpit && !cockpit.fail_closed">
          <div class="cockpit-stats">
            <article><span>聚合组</span><strong>{{ cockpit.groups_total }}</strong></article>
            <article><span>原始事件</span><strong>{{ cockpit.total_events }}</strong></article>
            <article><span>累计次数</span><strong>{{ cockpit.total_raw_count }}</strong></article>
          </div>

          <div class="cockpit-layout">
            <div class="cockpit-groups">
              <table>
                <thead><tr><th>级别</th><th>原因</th><th>资源</th><th>时间窗</th><th>次数</th></tr></thead>
                <tbody>
                  <tr v-for="(group, index) in cockpit.groups" :key="`${group.namespace}-${group.kind}-${group.resource_name}-${group.reason}-${index}`">
                    <td><span class="event-type-badge" :class="cockpitSeverityClass(group.severity)"><TriangleAlert v-if="group.severity === 'warning'" :size="13" /><Info v-else :size="13" />{{ group.severity }}</span></td>
                    <td class="cockpit-reason"><strong>{{ group.reason }}</strong><small class="muted">{{ group.namespace || 'cluster-scoped' }}</small></td>
                    <td class="cockpit-resource">{{ group.kind }}/{{ group.resource_name }}</td>
                    <td class="mono">{{ formatTime(group.first_seen) }}<small class="muted">→ {{ formatTime(group.last_seen) }}</small></td>
                    <td><span class="cockpit-count">×{{ group.raw_count }}</span><small class="muted">{{ group.event_count }} 条折叠</small></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="cockpit-trend">
              <h3>按天趋势（事件数）</h3>
              <div v-if="cockpit.trend.length === 0" class="empty-state event-loading"><Bell :size="22" /><span>窗口内无趋势数据</span></div>
              <div v-else class="trend-bars">
                <div v-for="point in cockpit.trend" :key="point.day" class="trend-bar-col">
                  <div class="trend-bar-track"><div class="trend-bar" :style="{ height: trendBarHeight(point.events) }" :title="`${point.day}: ${point.events} 事件 / ${point.groups} 组`"></div></div>
                  <span class="trend-bar-day">{{ point.day.slice(5) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="resource-panel event-center-panel">
        <div class="section-heading"><div><p class="context-label">KUBERNETES EVENTS</p><h2>事件流 · {{ filteredEvents.length }}</h2></div><span class="event-sync-time"><Clock3 :size="14" />更新于 {{ lastSyncedLabel }}</span></div>
        <div class="event-list-header" aria-hidden="true"><span>级别 / 原因</span><span>涉及资源</span><span>消息</span><span>次数</span><span>最后出现</span></div>
        <div v-if="loading" class="empty-state event-loading"><RefreshCw class="spinning" :size="24" /><span>正在读取集群事件</span></div>
        <div v-else-if="filteredEvents.length === 0" class="empty-state event-loading"><Bell :size="28" /><strong>当前筛选范围没有事件</strong><span>调整 Namespace、类型或资源名称后重新查询。</span></div>
        <button v-for="event in filteredEvents" v-else :key="event.metadata.uid || `${event.involvedObject.uid}-${event.reason}-${eventTimestamp(event)}`" class="event-center-row" type="button" @click="selectedEvent = event">
          <span class="event-reason"><span class="event-type-badge" :class="event.type.toLowerCase()"><TriangleAlert v-if="event.type === 'Warning'" :size="13" /><Info v-else :size="13" />{{ event.type || 'Normal' }}</span><strong>{{ event.reason || 'Unknown' }}</strong></span>
          <span class="event-object"><strong>{{ event.involvedObject.kind }}/{{ event.involvedObject.name }}</strong><small>{{ event.involvedObject.namespace || 'cluster-scoped' }}</small></span>
          <span class="event-message">{{ event.message || '--' }}</span>
          <span class="event-count">×{{ occurrenceCount(event) }}</span>
          <time>{{ formatTime(eventTimestamp(event)) }}</time>
        </button>
      </section>
    </template>

    <div v-if="selectedEvent" class="log-overlay" @click.self="selectedEvent = null"><section class="event-detail-drawer">
      <header><div><p class="context-label">EVENT DETAIL</p><h2>{{ selectedEvent.reason || 'Kubernetes Event' }}</h2></div><button class="icon-button" type="button" aria-label="关闭事件详情" @click="selectedEvent = null"><X :size="18" /></button></header>
      <span class="event-type-badge detail-badge" :class="selectedEvent.type.toLowerCase()"><TriangleAlert v-if="selectedEvent.type === 'Warning'" :size="14" /><Info v-else :size="14" />{{ selectedEvent.type || 'Normal' }}</span>
      <p class="event-detail-message">{{ selectedEvent.message }}</p>
      <dl class="event-detail-grid">
        <div><dt>涉及资源</dt><dd>{{ selectedEvent.involvedObject.kind }}/{{ selectedEvent.involvedObject.name }}</dd></div>
        <div><dt>Namespace</dt><dd>{{ selectedEvent.involvedObject.namespace || 'cluster-scoped' }}</dd></div>
        <div><dt>首次出现</dt><dd>{{ formatTime(selectedEvent.firstTimestamp || selectedEvent.eventTime || selectedEvent.metadata.creationTimestamp) }}</dd></div>
        <div><dt>最后出现</dt><dd>{{ formatTime(eventTimestamp(selectedEvent)) }}</dd></div>
        <div><dt>累计次数</dt><dd>{{ occurrenceCount(selectedEvent) }}</dd></div>
        <div><dt>动作</dt><dd>{{ selectedEvent.action || '--' }}</dd></div>
        <div><dt>上报组件</dt><dd>{{ selectedEvent.reportingComponent || '--' }}</dd></div>
        <div><dt>上报实例</dt><dd>{{ selectedEvent.reportingInstance || '--' }}</dd></div>
      </dl>
    </section></div>
  </ConsoleLayout>
</template>
