<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listKnowledge, getKnowledgeStats } from '../api/knowledge'
import type { KnowledgeEntry } from '../api/knowledge'
import CaseRetrievalLab from '../components/CaseRetrievalLab.vue'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import { useCaseComparisonStore } from '../stores/case-comparison'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const view = computed(() => route.query.view === 'compare' ? 'compare' : 'library')
const comparison = useCaseComparisonStore()
const reference = computed(() => comparison.reference)
function inspectCase(item: KnowledgeEntry) { comparison.select(item); void router.push({ path: '/cases', query: { view: 'compare' } }) }
const query = ref('')
const kind = ref('')
const items = ref<KnowledgeEntry[]>([])
const total = ref<number | null>(null)
const truncated = ref(false)
const loading = ref(false)
const error = ref('')
let sequence = 0
async function load() {
  const request = ++sequence
  loading.value = true
  error.value = ''
  items.value = []
  truncated.value = false
  try {
    const response = await listKnowledge(auth.accessToken, { q: query.value, resource_kind: kind.value })
    if (request !== sequence) return
    items.value = response.items
    truncated.value = response.truncated ?? false
  } catch (err) {
    if (request === sequence) error.value = err instanceof Error ? err.message : '案例读取失败'
  } finally {
    if (request === sequence) loading.value = false
  }
}
onMounted(() => {
  void load()
  void getKnowledgeStats(auth.accessToken).then(stats => { total.value = stats.total }).catch(() => { total.value = null })
})
</script>

<template>
  <ConsoleLayout eyebrow="案例记忆" title="历史案例">
    <nav class="workspace-tabs" aria-label="案例视图"><RouterLink to="/cases" :class="{ active: view === 'library' }" :aria-current="view === 'library' ? 'page' : undefined">案例库</RouterLink><RouterLink to="/cases?view=compare" :class="{ active: view === 'compare' }" :aria-current="view === 'compare' ? 'page' : undefined">检索对比</RouterLink></nav>
    <CaseRetrievalLab v-if="view === 'compare'" :reference="reference" :entries="items" :library-loading="loading" :library-error="error" @select="comparison.select" @reload="load" />
    <div v-show="view === 'library'">
    <section class="page-toolbar"><div><strong>{{ total ?? '—' }}</strong><span> 条归档案例 · 已解决的诊断自动归档</span></div></section>
    <form class="case-search" @submit.prevent="load">
      <input v-model="query" maxlength="500" aria-label="故障描述" placeholder="输入故障现象，检索相似历史案例" />
      <select v-model="kind" aria-label="资源类型提示"><option value="">不限资源类型</option><option v-for="value in ['Pod', 'Deployment', 'Node', 'Service', 'Ingress', 'PersistentVolumeClaim', 'HorizontalPodAutoscaler']" :key="value">{{ value }}</option></select>
      <button class="primary-button" :disabled="loading">{{ loading ? '检索中…' : '检索案例' }}</button>
    </form>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <p v-else-if="!loading && !items.length" class="panel">暂无匹配案例。诊断确认后，记录人工处理结果并标记“已解决”，即可积累历史案例。</p>
    <section class="case-list" :aria-busy="loading">
      <article v-for="item in items" :key="item.id" class="panel case-card">
        <header><h2>{{ item.summary }}</h2><span>{{ item.resource_kind }} / {{ item.resource_name }}</span></header>
        <p><code>{{ item.rule_id }}</code> · {{ new Date(item.noted_at).toLocaleString('zh-CN') }}</p>
        <h3>历史根因</h3><ul><li v-for="cause in item.root_causes" :key="cause">{{ cause }}</li></ul>
        <h3>处置建议</h3><ul><li v-for="step in item.recommendations" :key="step">{{ step }}</li></ul>
        <div class="case-actions"><RouterLink :to="{ path: '/diagnoses', query: { id: item.source_diagnosis_id } }">查看来源诊断与证据 →</RouterLink><button class="secondary-button" @click="inspectCase(item)">查看检索过程</button></div>
      </article>
    </section>
    <p v-if="truncated" class="compact-empty">当前结果已截断，请缩小检索范围。</p>
    </div>
  </ConsoleLayout>
</template>

<style scoped>
.case-actions { display: flex; align-items: center; gap: 18px; flex-wrap: wrap; margin-top: 18px; }
.case-actions button { font-size: 12px; }
.case-search { display: flex; flex-wrap: wrap; gap: 12px; margin: 16px 0; }
.case-search input { flex: 1; min-width: 220px; }
.case-list { display: grid; gap: 16px; }
.case-card { padding: 20px; }
.case-card header { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.case-card h2 { font-size: 17px; margin: 0; }
.case-card h3 { font-size: 14px; margin: 16px 0 8px; }
.case-card li { margin: 6px 0; }
</style>
