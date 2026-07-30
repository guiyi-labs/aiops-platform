<script setup lang="ts">import { ref, watch } from 'vue';
import { Loader2 } from 'lucide-vue-next';
import { getResourceManifest } from '../api/kubernetes';
import { useAuthStore } from '../stores/auth';
const props = defineProps<{
 clusterId: number;
 kind: string;
 namespace: string;
 name: string;
}>();
const emit = defineEmits<{
 (e: 'loaded'): void;
}>();
const auth = useAuthStore();
const loading = ref(false);
const error = ref<string | null>(null);
const manifest = ref<Record<string, unknown> | null>(null);
const formatted = ref('');
async function load() {
 loading.value = true;
 error.value = null;
 try {
 manifest.value = await getResourceManifest(auth.accessToken, props.clusterId, props.kind, props.namespace, props.name);
 formatted.value = JSON.stringify(manifest.value, null, 2);
 emit('loaded');
 }
 catch (err) {
 error.value = err instanceof Error ? err.message : 'Failed to load manifest';
 }
 finally {
 loading.value = false;
 }
}
watch(() => [props.clusterId, props.kind, props.namespace, props.name], () => {
 if (props.clusterId && props.kind && props.name) {
 load();
 }
}, { immediate: true });
async function copyToClipboard() {
 try {
 await navigator.clipboard.writeText(formatted.value);
 }
 catch {
 // ignore
 }
}
</script>

<template>
  <div class="manifest-viewer">
    <div v-if="loading" class="flex items-center gap-2 text-slate-500 text-sm">
      <Loader2 class="h-4 w-4 animate-spin" />
      Loading manifest...
    </div>
    <div v-else-if="error" class="text-red-600 text-sm">{{ error }}</div>
    <div v-else-if="manifest" class="relative">
      <div class="flex items-center justify-between mb-2">
        <span class="text-xs text-slate-500">Redacted manifest — sensitive fields are replaced with <code class="bg-slate-100 px-1 rounded">&lt;redacted&gt;</code></span>
        <button
          class="text-xs text-slate-500 hover:text-slate-700"
          @click="copyToClipboard"
        >Copy</button>
      </div>
      <pre class="bg-slate-50 border border-slate-200 rounded p-3 text-xs overflow-auto max-h-96 font-mono whitespace-pre">{{ formatted }}</pre>
    </div>
  </div>
</template>
