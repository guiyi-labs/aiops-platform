<script setup lang="ts">import { computed, ref, watch } from 'vue';
import { Container, Download, Loader2, Search } from 'lucide-vue-next';
import { getAllPodLogs, listPodContainers } from '../api/kubernetes';
import { useAuthStore } from '../stores/auth';
import type { PodContainerInfo, PodContainerLog } from '../types/kubernetes';
const props = defineProps<{
 clusterId: number;
 namespace: string;
 name: string;
}>();
const auth = useAuthStore();
const containers = ref<PodContainerInfo[]>([]);
const logs = ref<PodContainerLog[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);
const previous = ref(false);
const searchQuery = ref('');
const activeContainer = ref<string>('');
const logMap = computed(() => {
 const map = new Map<string, PodContainerLog>();
 for (const l of logs.value) {
 map.set(l.container, l);
 }
 return map;
});
const activeLog = computed(() => {
 if (!activeContainer.value)
 return null;
 return logMap.value.get(activeContainer.value) || null;
});
const filteredLines = computed(() => {
 if (!activeLog.value)
 return [];
 if (!searchQuery.value.trim())
 return activeLog.value.lines;
 const q = searchQuery.value.toLowerCase();
 return activeLog.value.lines.filter((l) => l.message.toLowerCase().includes(q));
});
async function load() {
 if (!props.clusterId || !props.name)
 return;
 loading.value = true;
 error.value = null;
 try {
 const [containerList, allLogs] = await Promise.all([
 listPodContainers(auth.accessToken, props.clusterId, props.namespace, props.name),
 getAllPodLogs(auth.accessToken, props.clusterId, props.namespace, props.name, { previous: previous.value, tail_lines: 200 }),
 ]);
 containers.value = containerList.items;
 logs.value = allLogs.containers;
 if (!activeContainer.value && containers.value.length > 0) {
 activeContainer.value = containers.value[0].name;
 }
 }
 catch (err) {
 error.value = err instanceof Error ? err.message : 'Failed to load logs';
 }
 finally {
 loading.value = false;
 }
}
watch(() => [props.clusterId, props.namespace, props.name], () => {
 load();
}, { immediate: true });
watch(previous, () => {
 load();
});
function downloadLogs() {
 if (!activeLog.value)
 return;
 const content = activeLog.value.lines.map((l) => `${l.timestamp} ${l.message}`).join('\n');
 const blob = new Blob([content], { type: 'text/plain' });
 const url = URL.createObjectURL(blob);
 const a = document.createElement('a');
 a.href = url;
 a.download = `${props.name}-${activeContainer.value}.log`;
 a.click();
 URL.revokeObjectURL(url);
}
</script>

<template>
  <div class="pod-logs-viewer">
    <div v-if="loading" class="flex items-center gap-2 text-slate-500 text-sm p-4">
      <Loader2 class="h-4 w-4 animate-spin" />
      Loading container logs...
    </div>
    <div v-else-if="error" class="text-red-600 text-sm p-4">{{ error }}</div>
    <div v-else class="space-y-3">
      <div class="flex items-center gap-3 flex-wrap">
        <div class="flex items-center gap-1 text-sm">
          <span class="text-slate-500">Previous:</span>
          <button
            :class="[
              'px-2 py-0.5 rounded text-xs',
              previous ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-600',
            ]"
            @click="previous = !previous"
          >{{ previous ? 'On' : 'Off' }}</button>
        </div>
        <div class="flex items-center gap-1 flex-wrap">
          <button
            v-for="c in containers"
            :key="c.name"
            :class="[
              'flex items-center gap-1 px-2 py-1 rounded text-xs border',
              activeContainer === c.name
                ? 'bg-indigo-50 border-indigo-200 text-indigo-700'
                : 'bg-white border-slate-200 text-slate-600 hover:bg-slate-50',
            ]"
            @click="activeContainer = c.name"
          >
            <Container class="h-3 w-3" />
            {{ c.name }}
            <span v-if="c.restart_count > 0" class="text-amber-600">({{ c.restart_count }})</span>
          </button>
        </div>
      </div>

      <div v-if="activeContainer" class="space-y-2">
        <div class="flex items-center gap-2">
          <div class="relative flex-1">
            <Search class="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-slate-400" />
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Search logs..."
              class="w-full pl-7 pr-2 py-1 text-xs border border-slate-200 rounded focus:outline-none focus:ring-1 focus:ring-indigo-300"
            />
          </div>
          <button
            v-if="activeLog"
            class="flex items-center gap-1 px-2 py-1 text-xs text-slate-500 hover:text-slate-700 border border-slate-200 rounded hover:bg-slate-50"
            @click="downloadLogs"
          >
            <Download class="h-3 w-3" />
            Download
          </button>
        </div>

        <div
          v-if="activeLog?.truncated"
          class="text-xs text-amber-600 bg-amber-50 border border-amber-200 rounded px-2 py-1"
        >
          Logs truncated: {{ activeLog.truncation_reason || 'body limit reached' }}
        </div>

        <pre class="bg-slate-900 text-slate-100 rounded p-3 text-xs overflow-auto max-h-80 font-mono leading-relaxed">
          <template v-if="filteredLines.length === 0">
            <span class="text-slate-500">{{ searchQuery ? 'No matching log lines' : 'No log lines' }}</span>
          </template>
          <template v-else>
            <span v-for="(line, i) in filteredLines" :key="i">
              <span class="text-slate-500">{{ line.timestamp }}</span>
              <span :class="searchQuery && line.message.toLowerCase().includes(searchQuery.toLowerCase()) ? 'bg-yellow-500/30' : ''">{{ line.message }}</span>
              {'\n'}
            </span>
          </template>
        </pre>
      </div>
    </div>
  </div>
</template>
