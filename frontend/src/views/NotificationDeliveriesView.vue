<script setup lang="ts">
import { Bell, RefreshCw, RotateCcw } from 'lucide-vue-next'
import { computed, onMounted, ref } from 'vue'

import { listNotificationDeliveries, retryNotificationDelivery } from '../api/notifications'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { NotificationDelivery, NotificationDeliveryStatus, NotificationEventType } from '../types/notification'

const auth = useAuthStore()
const items = ref<NotificationDelivery[]>([])
const total = ref(0)
const eventType = ref<NotificationEventType | ''>('')
const status = ref<NotificationDeliveryStatus | ''>('')
const diagnosisID = ref<number | undefined>()
const incidentID = ref<number | undefined>()
const escalationLevel = ref<number | undefined>()
const loading = ref(false)
const retryingID = ref(0)
const errorMessage = ref('')
const successMessage = ref('')
const canRetry = computed(() => auth.user?.roles.includes('system_admin') ?? false)

function formatTime(value?: string): string {
  if (!value) return '--'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

async function loadDeliveries() {
  loading.value = true; errorMessage.value = ''; successMessage.value = ''
  try {
    const response = await listNotificationDeliveries(auth.accessToken, { diagnosisID: diagnosisID.value || undefined, incidentID: incidentID.value || undefined, eventType: eventType.value, escalationLevel: escalationLevel.value, status: status.value })
    items.value = response.items; total.value = response.total
  } catch { errorMessage.value = '无法加载通知投递记录，请确认当前账号具有审计权限。' }
  finally { loading.value = false }
}

async function retry(item: NotificationDelivery) {
  retryingID.value = item.id; errorMessage.value = ''; successMessage.value = ''
  try {
    await retryNotificationDelivery(auth.accessToken, item.id)
    successMessage.value = `通知 #${item.id} 已重新进入投递队列。`
    await loadDeliveries()
  } catch { errorMessage.value = '无法重新投递；仅系统管理员可以重试 dead 状态记录，且通知必须已启用。' }
  finally { retryingID.value = 0 }
}

onMounted(loadDeliveries)
</script>

<template>
  <ConsoleLayout eyebrow="运行通知" title="Webhook 投递">
    <section class="notification-toolbar">
      <input v-model.number="diagnosisID" min="1" type="number" placeholder="诊断 ID" aria-label="按诊断 ID 筛选">
      <input v-model.number="incidentID" min="1" type="number" placeholder="事故 ID" aria-label="按事故 ID 筛选">
      <select v-model="eventType" aria-label="按事件类型筛选"><option value="">全部事件</option><option value="diagnosis.created">诊断创建</option><option value="diagnosis.status_changed">状态变更</option><option value="diagnosis.assigned">负责人变更</option><option value="incident.sla_approaching">SLA 临近</option><option value="incident.sla_breached">SLA 逾期</option><option value="incident.sla_escalated">SLA 升级</option></select>
      <select v-model="escalationLevel" aria-label="按升级级别筛选"><option :value="undefined">全部级别</option><option :value="0">基础提醒</option><option :value="1">首次升级</option><option :value="2">最终升级</option></select>
      <select v-model="status" aria-label="按投递状态筛选"><option value="">全部状态</option><option value="pending">等待</option><option value="delivering">投递中</option><option value="delivered">已送达</option><option value="dead">已停止重试</option></select>
      <button class="secondary-button" type="button" :disabled="loading" @click="loadDeliveries"><RefreshCw :size="15" :class="{ spinning: loading }" />查询</button>
    </section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="successMessage" class="audit-export-message">{{ successMessage }}</p>
    <section class="audit-list notification-list">
      <header><div><p class="context-label">SIGNED DIAGNOSIS + INCIDENT WEBHOOKS</p><h2>投递记录 · {{ total }}</h2></div><Bell :size="21" /></header>
      <div v-if="!loading && items.length === 0" class="resource-empty"><Bell :size="30" /><strong>暂无投递记录</strong><span>启用通知后，新诊断和工作流变更会原子写入投递队列。</span></div>
      <article v-for="item in items" :key="item.id" class="notification-row">
        <span class="audit-action">{{ item.event_type }}<small v-if="item.escalation_level">L{{ item.escalation_level }}</small></span>
        <span class="audit-resource"><strong>{{ item.incident_id ? `事故 #${item.incident_id}` : `诊断 #${item.diagnosis_id}` }}</strong><small>投递 #{{ item.id }} · 尝试 {{ item.attempts }} 次</small></span>
        <span class="notification-status" :class="item.status">{{ item.status }}</span>
        <span class="notification-error" :title="item.last_error">{{ item.last_error || '无错误' }}</span>
        <time>{{ formatTime(item.delivered_at || item.next_attempt_at) }}</time>
        <button v-if="item.status === 'dead' && canRetry" class="text-button" type="button" :disabled="retryingID === item.id" @click="retry(item)"><RotateCcw :size="14" />重试</button>
      </article>
    </section>
  </ConsoleLayout>
</template>
