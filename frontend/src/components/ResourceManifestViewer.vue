<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { getResourceManifest } from '../api/kubernetes'
import { useAuthStore } from '../stores/auth'
import { downloadJSON } from '../utils/download'
const props = defineProps<{ clusterId: number; kind: string; namespace: string; name: string }>()
const auth = useAuthStore()
const loading = ref(false)
const error = ref('')
const manifest = ref<Record<string, unknown> | null>(null)
const notice = ref('')
let sequence = 0
onBeforeUnmount(() => { sequence++ })
async function load() {
  const request = ++sequence
  loading.value = true; error.value = ''; manifest.value = null; notice.value = ''
  try { const value = await getResourceManifest(auth.accessToken, props.clusterId, props.kind, props.namespace || '_', props.name); if (request === sequence) manifest.value = value }
  catch (err) { if (request === sequence) error.value = err instanceof Error ? err.message : '脱敏清单读取失败' }
  finally { if (request === sequence) loading.value = false }
}
watch(() => [props.clusterId, props.kind, props.namespace, props.name], () => { void load() }, { immediate: true })
async function copy() {
  try { await navigator.clipboard.writeText(JSON.stringify(manifest.value, null, 2)); notice.value = '已复制脱敏清单' }
  catch { notice.value = '无法访问剪贴板，可使用下载按钮保存。' }
}
</script>
<template>
  <section class="manifest-viewer"><p v-if="loading" role="status">正在读取脱敏资源清单…</p><p v-if="error" role="alert" class="error-message">{{ error }} <button class="secondary-button" @click="load">重试</button></p><template v-if="manifest"><header><p>敏感字段由服务端替换为 &lt;redacted&gt;。</p><div><button class="secondary-button" @click="copy">复制清单</button><button class="secondary-button" @click="downloadJSON(`${name}-manifest.json`, manifest)">下载清单</button></div></header><p v-if="notice" role="status">{{ notice }}</p><pre tabindex="0" aria-label="脱敏资源清单 JSON">{{ JSON.stringify(manifest, null, 2) }}</pre></template></section>
</template>
<style scoped>
header, header > div { display: flex; align-items: center; gap: 12px; justify-content: space-between; flex-wrap: wrap; }
p, button { font-size: 12px; line-height: 1.8; }
p { color: var(--text-secondary); }
pre { max-height: 520px; overflow: auto; padding: 16px; background: var(--bg-secondary); color: var(--text-primary); font: 11px/1.8 var(--font-mono); border-radius: 8px; }
</style>
