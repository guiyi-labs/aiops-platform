<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { listClusters } from '../api/clusters'
import { listNodes, listPods, listEvents } from '../api/kubernetes'
import { listDiagnoses } from '../api/diagnosis'
import { getReadiness } from '../api/health'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { DiagnosisRecord } from '../types/diagnosis'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const selected = ref(0)
const loading = ref(false)
const error = ref('')
const ready = ref<boolean | null>(null)
const nodeCount = ref<number | null>(null)
const podCount = ref<number | null>(null)
const warningCount = ref<number | null>(null)
const diagnosisCount = ref<number | null>(null)
const warningsTruncated = ref(false)
const recent = ref<DiagnosisRecord[]>([])
const current = computed(() => clusters.value.find(item => item.id === selected.value))
let sequence = 0
async function load() {
  const request = ++sequence
  if (!selected.value) return
  loading.value = true
  error.value = ''
  nodeCount.value = podCount.value = warningCount.value = diagnosisCount.value = null
  recent.value = []
  const results = await Promise.allSettled([
    listNodes(auth.accessToken, selected.value),
    listPods(auth.accessToken, selected.value),
    listEvents(auth.accessToken, selected.value),
    listDiagnoses(auth.accessToken, { clusterID: selected.value, limit: 5 }),
  ])
  if (request !== sequence) return
  const [nodes, pods, events, diagnoses] = results
  if (nodes.status === 'fulfilled') nodeCount.value = nodes.value.total
  if (pods.status === 'fulfilled') podCount.value = pods.value.total
  if (events.status === 'fulfilled') {
    warningCount.value = events.value.items.filter(item => item.type === 'Warning').length
    warningsTruncated.value = events.value.remaining > 0
  }
  if (diagnoses.status === 'fulfilled') {
    recent.value = diagnoses.value.items
    diagnosisCount.value = diagnoses.value.total
  }
  if (results.some(result => result.status === 'rejected')) error.value = '部分数据暂不可用，请检查集群连接和读取权限后刷新。'
  loading.value = false
}
async function initialize() {
  void getReadiness().then(value => { ready.value = value.status === 'ready' || value.status === 'ok' }).catch(() => { ready.value = false })
  try {
    clusters.value = (await listClusters(auth.accessToken)).items
    selected.value = clusters.value.find(item => item.enabled)?.id ?? clusters.value[0]?.id ?? 0
    await load()
  } catch { error.value = '无法读取集群配置，请稍后重试。' }
}
onMounted(initialize)
</script>

<template>
  <ConsoleLayout eyebrow="K8s 故障诊断" title="系统概览">
    <section class="page-toolbar">
      <div><strong>{{ ready === null ? '连接中' : ready ? '服务就绪' : '服务暂不可用' }}</strong><span> · {{ current?.name ?? '尚未接入集群' }}</span></div>
      <div class="toolbar-actions"><select v-if="clusters.length" v-model="selected" aria-label="当前诊断集群" @change="load"><option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option></select><button class="secondary-button" :disabled="loading || !selected" @click="load">刷新</button></div>
    </section>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <section v-if="!clusters.length" class="panel overview-panel"><h2>接入实验集群</h2><p>配置集群连接后，即可查看资源、分析故障并积累历史案例。</p><RouterLink to="/settings/clusters">配置集群连接 →</RouterLink></section>
    <template v-else>
      <section class="overview-grid" :aria-busy="loading">
        <article class="panel overview-stat"><span>节点</span><strong>{{ nodeCount ?? '—' }}</strong></article>
        <article class="panel overview-stat"><span>Pod</span><strong>{{ podCount ?? '—' }}</strong></article>
        <article class="panel overview-stat"><span>当前事件样本中的警告</span><strong>{{ warningCount ?? '—' }}{{ warningsTruncated ? '+' : '' }}</strong></article>
        <article class="panel overview-stat"><span>诊断记录</span><strong>{{ diagnosisCount ?? '—' }}</strong></article>
      </section>
      <section class="panel overview-panel"><h2>排查故障</h2><p>选择异常资源运行诊断，查看规则结论、原始证据与处理建议；人工处理完成后结案，供下次检索复用。</p><div class="overview-links"><RouterLink :to="{ path: '/workloads', query: { cluster: selected } }">查看资源并诊断 →</RouterLink><RouterLink :to="{ path: '/diagnoses', query: { cluster: selected } }">查看诊断记录 →</RouterLink><RouterLink to="/cases">检索历史案例 →</RouterLink></div></section>
      <section class="panel overview-panel"><h2>最近诊断</h2><p v-if="!recent.length">暂无诊断记录。</p><ul v-else class="recent-diagnoses"><li v-for="record in recent" :key="record.id"><RouterLink :to="{ path: '/diagnoses', query: { id: record.id, cluster: selected } }">{{ record.summary }}</RouterLink><span>{{ record.resource.kind }}/{{ record.resource.name }} · {{ record.status }}</span></li></ul></section>
    </template>
  </ConsoleLayout>
</template>

<style scoped>
.overview-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; margin: 20px 0; }
.overview-stat { display: flex; flex-direction: column; gap: 12px; padding: 20px; }
.overview-stat strong { font-size: 32px; }
.overview-panel { padding: 24px; margin: 20px 0; }
.overview-panel h2 { font-size: 18px; margin-top: 0; }
.overview-links { display: flex; flex-wrap: wrap; gap: 20px; margin-top: 20px; }
.recent-diagnoses { list-style: none; padding: 0; }
.recent-diagnoses li { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; padding: 14px 0; border-bottom: 1px solid var(--border-subtle); }
.recent-diagnoses span { color: var(--text-secondary); }
@media (max-width: 800px) { .overview-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 420px) { .overview-grid { grid-template-columns: 1fr; } }
</style>
