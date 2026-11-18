<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { getAllPodLogs, listPodContainers } from '../api/kubernetes'
import { useAuthStore } from '../stores/auth'
import type { PodContainerInfo, PodContainerLog } from '../types/kubernetes'
const props = defineProps<{ clusterId: number; namespace: string; name: string }>()
const auth = useAuthStore()
const containers = ref<PodContainerInfo[]>([])
const logs = ref<PodContainerLog[]>([])
const loading = ref(false)
const error = ref('')
const previous = ref(false)
const searchQuery = ref('')
const activeContainer = ref('')
const activeLog = computed(() => logs.value.find(item => item.container === activeContainer.value))
const filteredLines = computed(() => (activeLog.value?.lines ?? []).filter(line => line.message.toLowerCase().includes(searchQuery.value.trim().toLowerCase())))
const content = computed(() => filteredLines.value.map(line => [line.timestamp, line.message].filter(Boolean).join(' ')).join('\n'))
let sequence = 0
onBeforeUnmount(() => { sequence++ })
async function load() {
  const request = ++sequence
  loading.value = true; error.value = ''; logs.value = []
  try {
    const [containerList, allLogs] = await Promise.all([listPodContainers(auth.accessToken, props.clusterId, props.namespace, props.name), getAllPodLogs(auth.accessToken, props.clusterId, props.namespace, props.name, { previous: previous.value, tail_lines: 200 })])
    if (request !== sequence) return
    containers.value = containerList.items; logs.value = allLogs.containers
    if (!containers.value.some(item => item.name === activeContainer.value)) activeContainer.value = containers.value[0]?.name ?? ''
  } catch (err) { if (request === sequence) error.value = err instanceof Error ? err.message : '容器日志读取失败' }
  finally { if (request === sequence) loading.value = false }
}
watch(() => [props.clusterId, props.namespace, props.name], () => { activeContainer.value = ''; searchQuery.value = ''; previous.value = false; void load() }, { immediate: true })
watch(previous, () => { void load() })
function download() {
  if (!activeLog.value) return
  const blob = new Blob([activeLog.value.lines.map(line => `${line.timestamp || ''} ${line.message}`).join('\n')], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a'); link.href = url; link.download = `${props.name}-${activeContainer.value}.log`; link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
</script>
<template>
  <section class="pod-logs-viewer"><header><label><input v-model="previous" type="checkbox" :disabled="loading" />上一次容器日志</label><button class="secondary-button" :disabled="loading" @click="load">刷新日志</button></header><p v-if="loading" role="status">正在读取容器日志…</p><p v-if="error" role="alert" class="error-message">{{ error }}</p><template v-if="!loading && !error"><nav aria-label="日志容器"><button v-for="container in containers" :key="container.name" class="secondary-button" :aria-pressed="activeContainer === container.name" @click="activeContainer = container.name">{{ container.name }} · 重启 {{ container.restart_count }}</button></nav><p v-if="!containers.length">暂无可读取的容器。</p><template v-if="activeContainer"><div class="log-search"><input v-model="searchQuery" aria-label="筛选日志内容" placeholder="输入关键词筛选日志" /><button class="secondary-button" :disabled="!activeLog" @click="download">下载容器日志</button></div><p v-if="activeLog?.truncated">日志已截断：{{ activeLog.truncation_reason || '达到读取上限' }}</p><pre tabindex="0" aria-label="容器日志内容">{{ content || (searchQuery ? '没有匹配的日志行' : '暂无日志内容') }}</pre></template></template></section>
</template>
<style scoped>
header, nav, .log-search { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 14px; }
header { justify-content: space-between; }
label, button, p { font-size: 12px; line-height: 1.8; }
p { color: var(--text-secondary); }
label { display: flex; gap: 8px; align-items: center; }
.log-search input { flex: 1; min-width: 0; }
button[aria-pressed="true"] { color: var(--accent-strong); border-color: var(--accent-strong); background: var(--accent-subtle); }
pre { max-height: 440px; overflow: auto; padding: 16px; background: var(--bg-secondary); color: var(--text-primary); font: 11px/1.8 var(--font-mono); border-radius: 8px; }
</style>
