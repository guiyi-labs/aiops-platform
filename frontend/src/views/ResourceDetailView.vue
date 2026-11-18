<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, RefreshCw, Stethoscope } from 'lucide-vue-next'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import ResourceManifestViewer from '../components/ResourceManifestViewer.vue'
import PodLogsViewer from '../components/PodLogsViewer.vue'
import * as resources from '../api/kubernetes'
import * as diagnosis from '../api/diagnosis'
import { APIError } from '../api/auth'
import { useAuthStore } from '../stores/auth'
import { eventTimestamp, relatedResourceEvents, type EventResourceKind } from '../utils/kubernetes-events'
import type { DiagnosisRecord } from '../types/diagnosis'
import type { KubernetesEvent } from '../types/kubernetes'

const route = useRoute()
const auth = useAuthStore()
const activeTab = ref('overview')
const loading = ref(false)
const error = ref('')
const resource = ref<Record<string, unknown> | null>(null)
const events = ref<KubernetesEvent[]>([])
const eventsError = ref('')
const eventsTruncated = ref(false)
const diagnosisLoading = ref(false)
const diagnosisMessage = ref('')
const diagnosisRecord = ref<DiagnosisRecord | null>(null)
const clusterID = computed(() => Number(route.params.clusterId) || 0)
const aliases: Record<string, string> = { PVC: 'PersistentVolumeClaim', PV: 'PersistentVolume', HPA: 'HorizontalPodAutoscaler', PDB: 'PodDisruptionBudget' }
const kind = computed(() => aliases[String(route.params.kind)] ?? String(route.params.kind))
const namespace = computed(() => route.params.namespace === '_' ? '' : String(route.params.namespace || ''))
const name = computed(() => String(route.params.name || ''))
const clusterScoped = computed(() => ['Node', 'PersistentVolume', 'StorageClass', 'ClusterRole'].includes(kind.value))
const readers: Record<string, () => Promise<unknown>> = {
  Pod: () => resources.getPod(auth.accessToken, clusterID.value, namespace.value, name.value),
  Deployment: () => resources.getDeployment(auth.accessToken, clusterID.value, namespace.value, name.value),
  Service: () => resources.getService(auth.accessToken, clusterID.value, namespace.value, name.value),
  Ingress: () => resources.getIngress(auth.accessToken, clusterID.value, namespace.value, name.value),
  Node: () => resources.getNode(auth.accessToken, clusterID.value, name.value),
  PersistentVolumeClaim: () => resources.getPersistentVolumeClaim(auth.accessToken, clusterID.value, namespace.value, name.value),
  HorizontalPodAutoscaler: () => resources.getHorizontalPodAutoscaler(auth.accessToken, clusterID.value, namespace.value, name.value),
  PersistentVolume: () => resources.getPersistentVolume(auth.accessToken, clusterID.value, name.value),
  PodDisruptionBudget: () => resources.getPodDisruptionBudget(auth.accessToken, clusterID.value, namespace.value, name.value),
  NetworkPolicy: () => resources.getNetworkPolicy(auth.accessToken, clusterID.value, namespace.value, name.value),
  ServiceAccount: () => resources.getServiceAccount(auth.accessToken, clusterID.value, namespace.value, name.value),
  ConfigMap: () => resources.getConfigMap(auth.accessToken, clusterID.value, namespace.value, name.value),
}
const diagnosisRunners: Record<string, () => Promise<DiagnosisRecord>> = {
  Pod: () => diagnosis.diagnosePod(auth.accessToken, clusterID.value, namespace.value, name.value),
  Deployment: () => diagnosis.diagnoseDeployment(auth.accessToken, clusterID.value, namespace.value, name.value),
  Node: () => diagnosis.diagnoseNode(auth.accessToken, clusterID.value, name.value),
  Service: () => diagnosis.diagnoseService(auth.accessToken, clusterID.value, namespace.value, name.value),
  Ingress: () => diagnosis.diagnoseIngress(auth.accessToken, clusterID.value, namespace.value, name.value),
  PersistentVolumeClaim: () => diagnosis.diagnosePersistentVolumeClaim(auth.accessToken, clusterID.value, namespace.value, name.value),
  HorizontalPodAutoscaler: () => diagnosis.diagnoseHorizontalPodAutoscaler(auth.accessToken, clusterID.value, namespace.value, name.value),
}
const canDiagnose = computed(() => !!diagnosisRunners[kind.value] && auth.user?.roles.some(role => ['system_admin', 'operations_admin'].includes(role)))
const workbench = computed(() => ({ path: '/workloads', query: { cluster: String(clusterID.value), kind: ({ PersistentVolumeClaim: 'PVC', HorizontalPodAutoscaler: 'HPA' } as Record<string, string>)[kind.value] ?? kind.value, ...(namespace.value ? { namespace: namespace.value } : {}), name: name.value } }))
const tabs = computed(() => [{ id: 'overview', label: '资源概览' }, { id: 'spec', label: '规格配置' }, { id: 'status', label: '资源状态' }, { id: 'events', label: '相关事件' }, ...(kind.value === 'Pod' ? [{ id: 'logs', label: '容器日志' }] : []), { id: 'manifest', label: '脱敏清单' }])
let sequence = 0
onBeforeUnmount(() => { sequence++ })
async function load() {
  const request = ++sequence
  loading.value = true; error.value = ''; resource.value = null; events.value = []; eventsError.value = ''; eventsTruncated.value = false; diagnosisRecord.value = null; diagnosisMessage.value = ''
  const reader = readers[kind.value]
  if (!reader || !clusterID.value || !name.value) { error.value = '该资源类型暂不支持详情读取，请从集群与资源页面选择资源。'; loading.value = false; return }
  const target = { kind: ({ PersistentVolumeClaim: 'PVC', HorizontalPodAutoscaler: 'HPA', PersistentVolume: 'PV', PodDisruptionBudget: 'PDB' } as Record<string, string>)[kind.value] ?? kind.value, namespace: namespace.value, name: name.value, clusterScoped: clusterScoped.value }
  await Promise.allSettled([
    reader().then(value => { if (request === sequence) resource.value = value as Record<string, unknown> }).catch(err => { if (request === sequence) error.value = err instanceof Error ? err.message : '资源读取失败' }),
    resources.listEvents(auth.accessToken, clusterID.value, clusterScoped.value ? '' : namespace.value, name.value).then(value => { if (request === sequence) { events.value = relatedResourceEvents(value.items, { ...target, kind: target.kind as EventResourceKind }); eventsTruncated.value = value.remaining > 0 } }).catch(() => { if (request === sequence) eventsError.value = '相关事件读取失败；资源信息仍可查看。' }),
  ])
  if (request === sequence) loading.value = false
}
watch([clusterID, kind, namespace, name], () => { activeTab.value = 'overview'; void load() }, { immediate: true })
async function runDiagnosis() {
  const runner = diagnosisRunners[kind.value]
  if (!runner || !canDiagnose.value) return
  const request = sequence
  diagnosisLoading.value = true; diagnosisRecord.value = null; diagnosisMessage.value = ''
  try { const result = await runner(); if (request === sequence) diagnosisRecord.value = result }
  catch (err) { if (request === sequence) diagnosisMessage.value = err instanceof APIError && err.code === 'NO_RULE_MATCH' ? '本次未命中已启用的故障规则，未生成诊断记录。' : err instanceof Error ? err.message : '诊断请求失败，请稍后重试。' }
  finally { diagnosisLoading.value = false }
}
</script>

<template>
  <ConsoleLayout eyebrow="资源与证据" :title="`${kind} · ${name}`">
    <section class="panel resource-page-header"><RouterLink :to="workbench"><ArrowLeft :size="15" />返回集群与资源</RouterLink><div><button class="secondary-button" :disabled="loading" @click="load"><RefreshCw :size="14" />刷新资源</button><button v-if="canDiagnose" class="primary-button" :disabled="loading || !resource || diagnosisLoading" @click="runDiagnosis"><Stethoscope :size="15" />{{ diagnosisLoading ? '正在诊断…' : '运行规则诊断' }}</button></div></section>
    <p v-if="loading" role="status" class="panel detail-message">正在读取资源与相关事件…</p>
    <p v-if="error" role="alert" class="error-message">{{ error }}</p>
    <section v-if="diagnosisRecord" class="panel diagnosis-outcome" role="status"><strong>{{ diagnosisRecord.summary }}</strong><RouterLink :to="{ path: '/diagnoses', query: { id: diagnosisRecord.id } }">查看诊断结论与证据 →</RouterLink></section>
    <p v-if="diagnosisMessage" role="status" class="panel detail-message">{{ diagnosisMessage }}</p>
    <template v-if="resource">
      <section class="panel resource-identity"><h2>{{ kind }} / {{ name }}</h2><p>集群 #{{ clusterID }} · {{ clusterScoped ? '集群级资源' : `命名空间 ${namespace}` }}</p></section>
      <nav class="workspace-tabs" aria-label="资源详情视图"><button v-for="tab in tabs" :key="tab.id" :class="{ active: activeTab === tab.id }" :aria-pressed="activeTab === tab.id" @click="activeTab = tab.id">{{ tab.label }}</button></nav>
      <section class="panel resource-tab-panel">
        <template v-if="activeTab === 'overview'"><h3>资源元数据</h3><pre tabindex="0" aria-label="资源元数据 JSON">{{ JSON.stringify(resource.metadata ?? {}, null, 2) }}</pre></template>
        <template v-else-if="activeTab === 'spec'"><h3>规格配置</h3><pre tabindex="0" aria-label="规格配置 JSON">{{ JSON.stringify(resource.spec ?? {}, null, 2) }}</pre></template>
        <template v-else-if="activeTab === 'status'"><h3>资源状态</h3><pre tabindex="0" aria-label="资源状态 JSON">{{ JSON.stringify(resource.status ?? {}, null, 2) }}</pre></template>
        <template v-else-if="activeTab === 'events'"><h3>相关事件 · {{ events.length }}</h3><p v-if="eventsError" role="alert" class="error-message">{{ eventsError }}</p><p v-else-if="!events.length" class="detail-message">当前返回范围内没有该资源的相关事件。</p><p v-if="eventsTruncated" class="detail-message">事件列表达到读取上限，请在集群与资源中缩小查询范围。</p><article v-for="(event, index) in events" :key="event.metadata.uid || index" class="resource-event"><header><strong>{{ event.reason }}</strong><span>{{ event.type }} · {{ eventTimestamp(event) || '时间未知' }}</span></header><p>{{ event.message }}</p></article></template>
        <PodLogsViewer v-else-if="activeTab === 'logs'" :cluster-id="clusterID" :namespace="namespace" :name="name" />
        <ResourceManifestViewer v-else-if="activeTab === 'manifest'" :cluster-id="clusterID" :kind="kind" :namespace="namespace" :name="name" />
      </section>
    </template>
  </ConsoleLayout>
</template>

<style scoped>
.resource-page-header, .resource-page-header > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.resource-page-header { padding: 16px 20px; }
.resource-page-header a, .resource-page-header button { display: inline-flex; align-items: center; gap: 7px; font-size: 12px; }
.resource-page-header a, .diagnosis-outcome a { color: var(--accent-strong); text-decoration: none; }
.resource-identity { padding: 20px; margin: 18px 0; }
.resource-identity h2 { font-size: 18px; margin: 0 0 10px; overflow-wrap: anywhere; }
.resource-identity p, .detail-message { color: var(--text-secondary); font-size: 12px; line-height: 1.8; }
.detail-message { padding: 18px; }
.resource-tab-panel { padding: 20px; margin-top: 16px; min-width: 0; }
h3 { font-size: 14px; margin-top: 0; }
pre { font: 11px/1.8 var(--font-mono); padding: 16px; background: var(--bg-secondary); color: var(--text-primary); border-radius: 8px; max-height: 520px; overflow: auto; }
.diagnosis-outcome { padding: 18px 20px; display: grid; gap: 10px; font-size: 13px; margin-top: 16px; }
.resource-event { border-top: 1px solid var(--border-subtle); padding: 14px 0; font-size: 12px; overflow-wrap: anywhere; }
.resource-event header { display: flex; gap: 12px; flex-wrap: wrap; }
.resource-event span { color: var(--text-secondary); font-size: 11px; }
.resource-event p { line-height: 1.8; }
</style>
