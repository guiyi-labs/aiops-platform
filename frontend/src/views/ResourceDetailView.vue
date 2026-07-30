<script setup lang="ts">import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, FileText, Activity, AlertTriangle, ListChecks, Wrench } from 'lucide-vue-next';
import ConsoleLayout from '../components/ConsoleLayout.vue';
import ResourceManifestViewer from '../components/ResourceManifestViewer.vue';
import PodLogsViewer from '../components/PodLogsViewer.vue';
import { getConfigMap, getPod, getPersistentVolume, getPersistentVolumeClaim, getPodDisruptionBudget, getNetworkPolicy, getServiceAccount, getService, getDeployment, getIngress, getNode, listEvents } from '../api/kubernetes';
import { diagnosePod, diagnoseDeployment, diagnoseNode, diagnoseService, diagnoseIngress, diagnosePersistentVolumeClaim, diagnoseHorizontalPodAutoscaler, getRolloutHistory, getRolloutStatus } from '../api/diagnosis';
import { useAuthStore } from '../stores/auth';
import type { KubernetesEvent } from '../types/kubernetes';
import type { RolloutHistory, RolloutStatus } from '../types/diagnosis';
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const activeTab = ref<'overview' | 'spec' | 'status' | 'events' | 'logs' | 'manifest' | 'tasks' | 'rollout'>('overview');
const loading = ref(false);
const error = ref<string | null>(null);
const resource = ref<Record<string, unknown> | null>(null);
const events = ref<KubernetesEvent[]>([]);
const diagnosisLoading = ref(false);
const diagnosisResult = ref<string | null>(null);
const rolloutHistory = ref<RolloutHistory | null>(null);
const rolloutStatus = ref<RolloutStatus | null>(null);
const rolloutLoading = ref(false);
const rolloutError = ref<string | null>(null);
const clusterId = computed(() => Number(route.params.clusterId) || 0);
const kind = computed(() => String(route.params.kind || ''));
const namespace = computed(() => String(route.params.namespace || ''));
const name = computed(() => String(route.params.name || ''));
const isPod = computed(() => kind.value === 'Pod');
const isLoggable = computed(() => isPod.value);
const showManifest = computed(() => ['Pod', 'Deployment', 'Service', 'Ingress', 'PersistentVolumeClaim', 'PersistentVolume', 'PodDisruptionBudget', 'NetworkPolicy', 'ServiceAccount', 'Role', 'ClusterRole'].includes(kind.value));
const showTasks = computed(() => ['Pod', 'Deployment', 'Node', 'Service', 'Ingress', 'PersistentVolumeClaim', 'HPA'].includes(kind.value));
const showRollout = computed(() => kind.value === 'Deployment');
async function loadResource() {
 if (!clusterId.value || !kind.value || !name.value)
 return;
 loading.value = true;
 error.value = null;
 try {
 const token = auth.accessToken;
 let result: unknown;
 switch (kind.value) {
 case 'Pod':
 result = await getPod(token, clusterId.value, namespace.value, name.value);
 break;
 case 'Deployment':
 result = await getDeployment(token, clusterId.value, namespace.value, name.value);
 break;
 case 'Service':
 result = await getService(token, clusterId.value, namespace.value, name.value);
 break;
 case 'Ingress':
 result = await getIngress(token, clusterId.value, namespace.value, name.value);
 break;
 case 'Node':
 result = await getNode(token, clusterId.value, name.value);
 break;
 case 'PersistentVolumeClaim':
 result = await getPersistentVolumeClaim(token, clusterId.value, namespace.value, name.value);
 break;
 case 'PersistentVolume':
 result = await getPersistentVolume(token, clusterId.value, name.value);
 break;
 case 'PodDisruptionBudget':
 result = await getPodDisruptionBudget(token, clusterId.value, namespace.value, name.value);
 break;
 case 'NetworkPolicy':
 result = await getNetworkPolicy(token, clusterId.value, namespace.value, name.value);
 break;
 case 'ServiceAccount':
 result = await getServiceAccount(token, clusterId.value, namespace.value, name.value);
 break;
 case 'ConfigMap':
 result = await getConfigMap(token, clusterId.value, namespace.value, name.value);
 break;
 default:
 result = null;
 }
 resource.value = result as Record<string, unknown> | null;
 const eventsResp = await listEvents(token, clusterId.value, namespace.value);
 events.value = eventsResp.items;
 }
 catch (err) {
 error.value = err instanceof Error ? err.message : 'Failed to load resource';
 }
 finally {
 loading.value = false;
 }
}
async function loadRollout() {
  if (!clusterId.value || !namespace.value || !name.value) return;
  rolloutLoading.value = true;
  rolloutError.value = null;
  try {
    const token = auth.accessToken;
    const [history, status] = await Promise.allSettled([
      getRolloutHistory(token, clusterId.value, namespace.value, name.value),
      getRolloutStatus(token, clusterId.value, namespace.value, name.value),
    ]);
    rolloutHistory.value = history.status === 'fulfilled' ? history.value : null;
    rolloutStatus.value = status.status === 'fulfilled' ? status.value : null;
    if (history.status === 'rejected' && status.status === 'rejected') {
      rolloutError.value = '无法读取发布历史或状态';
    }
  } finally {
    rolloutLoading.value = false;
  }
}
watch([clusterId, kind, namespace, name], () => {
 loadResource();
}, { immediate: true });
watch(activeTab, (tab) => {
  if (tab === 'rollout' && showRollout.value && !rolloutHistory.value && !rolloutStatus.value && !rolloutLoading.value) {
    loadRollout();
  }
});
async function runDiagnosis() {
 if (!clusterId.value || !kind.value)
 return;
 diagnosisLoading.value = true;
 diagnosisResult.value = null;
 try {
 const token = auth.accessToken;
 let diagnosisFn: typeof diagnosePod | null = null;
 switch (kind.value) {
 case 'Pod': diagnosisFn = diagnosePod; break;
 case 'Deployment': diagnosisFn = diagnoseDeployment; break;
 case 'Node': diagnosisFn = diagnoseNode; break;
 case 'Service': diagnosisFn = diagnoseService; break;
 case 'Ingress': diagnosisFn = diagnoseIngress; break;
 case 'PersistentVolumeClaim': diagnosisFn = diagnosePersistentVolumeClaim; break;
 case 'HPA': diagnosisFn = diagnoseHorizontalPodAutoscaler; break;
 }
 if (diagnosisFn) {
 const result = await diagnosisFn(token, clusterId.value, namespace.value, name.value);
 diagnosisResult.value = `Diagnosis completed: ${result.status}`;
 }
 }
 catch (err) {
 diagnosisResult.value = `Diagnosis failed: ${err instanceof Error ? err.message : 'unknown error'}`;
 }
 finally {
 diagnosisLoading.value = false;
 }
}
function goBack() {
 router.push(`/workloads?cluster=${clusterId.value}`);
}
</script>

<template>
  <ConsoleLayout eyebrow="Kubernetes" :title="`${kind}: ${name}`">
    <div class="resource-detail-view p-6 max-w-6xl mx-auto">
      <div class="flex items-center gap-3 mb-6">
        <button
          class="flex items-center gap-1 text-sm text-slate-600 hover:text-slate-900"
          @click="goBack"
        >
          <ArrowLeft class="h-4 w-4" />
          Back to workbench
        </button>
      </div>

      <div v-if="loading" class="flex items-center gap-2 text-slate-500 p-8 justify-center">
        Loading resource...
      </div>

      <div v-else-if="error" class="text-red-600 p-4">{{ error }}</div>

      <div v-else-if="resource" class="space-y-6">
        <div class="flex items-center gap-3">
          <FileText class="h-5 w-5 text-indigo-500" />
          <h1 class="text-xl font-semibold">{{ kind }}: {{ name }}</h1>
          <span v-if="namespace" class="text-sm text-slate-500">({{ namespace }})</span>
        </div>

        <div class="flex items-center gap-1 border-b border-slate-200">
          <button
            v-for="tab in (['overview', 'spec', 'status', 'events'] as const)"
            :key="tab"
            :class="[
              'px-3 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === tab
                ? 'border-indigo-500 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-700',
            ]"
            @click="activeTab = tab"
          >{{ tab.charAt(0).toUpperCase() + tab.slice(1) }}</button>
          <button
            v-if="isLoggable"
            :class="[
              'px-3 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'logs'
                ? 'border-indigo-500 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-700',
            ]"
            @click="activeTab = 'logs'"
          >Logs</button>
          <button
            v-if="showManifest"
            :class="[
              'px-3 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'manifest'
                ? 'border-indigo-500 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-700',
            ]"
            @click="activeTab = 'manifest'"
          >Manifest</button>
          <button
            v-if="showTasks"
            :class="[
              'px-3 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'tasks'
                ? 'border-indigo-500 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-700',
            ]"
            @click="activeTab = 'tasks'"
          >Tasks</button>
          <button
            v-if="showRollout"
            :class="[
              'px-3 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'rollout'
                ? 'border-indigo-500 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-700',
            ]"
            @click="activeTab = 'rollout'"
          >Rollout</button>
        </div>

        <div class="tab-content">
          <div v-if="activeTab === 'overview'" class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="bg-white border border-slate-200 rounded p-4">
              <h3 class="text-sm font-medium text-slate-700 mb-2">Metadata</h3>
              <pre class="text-xs font-mono text-slate-600 overflow-auto">{{ JSON.stringify(resource.metadata || {}, null, 2) }}</pre>
            </div>
            <div class="bg-white border border-slate-200 rounded p-4">
              <h3 class="text-sm font-medium text-slate-700 mb-2">Quick Info</h3>
              <div class="text-xs text-slate-600 space-y-1">
                <div>Kind: <span class="font-mono">{{ kind }}</span></div>
                <div>Name: <span class="font-mono">{{ name }}</span></div>
                <div v-if="namespace">Namespace: <span class="font-mono">{{ namespace }}</span></div>
                <div>Cluster: #{{ clusterId }}</div>
              </div>
            </div>
          </div>

          <div v-else-if="activeTab === 'spec'" class="bg-white border border-slate-200 rounded p-4">
            <h3 class="text-sm font-medium text-slate-700 mb-2">Spec</h3>
            <pre class="text-xs font-mono text-slate-600 overflow-auto max-h-96">{{ JSON.stringify(resource.spec || {}, null, 2) }}</pre>
          </div>

          <div v-else-if="activeTab === 'status'" class="bg-white border border-slate-200 rounded p-4">
            <h3 class="text-sm font-medium text-slate-700 mb-2">Status</h3>
            <pre class="text-xs font-mono text-slate-600 overflow-auto max-h-96">{{ JSON.stringify(resource.status || {}, null, 2) }}</pre>
          </div>

          <div v-else-if="activeTab === 'events'" class="bg-white border border-slate-200 rounded p-4">
            <h3 class="text-sm font-medium text-slate-700 mb-2 flex items-center gap-2">
              <ListChecks class="h-4 w-4" />
              Related Events
            </h3>
            <div v-if="events.length === 0" class="text-sm text-slate-500 p-4">No related events</div>
            <div v-else class="space-y-2">
              <div
                v-for="(event, i) in events.filter((e) => e.involvedObject?.name === name).slice(0, 20)"
                :key="i"
                class="border border-slate-200 rounded p-2 text-xs"
              >
                <div class="flex items-center gap-2">
                  <span :class="event.type === 'Warning' ? 'text-amber-600' : 'text-slate-600'" class="font-medium">{{ event.type }}</span>
                  <span class="font-mono">{{ event.reason }}</span>
                </div>
                <div class="text-slate-600 mt-1">{{ event.message }}</div>
              </div>
            </div>
          </div>

          <div v-else-if="activeTab === 'logs' && isPod" class="bg-white border border-slate-200 rounded p-4">
            <PodLogsViewer :cluster-id="clusterId" :namespace="namespace" :name="name" />
          </div>

          <div v-else-if="activeTab === 'manifest' && showManifest" class="bg-white border border-slate-200 rounded p-4">
            <ResourceManifestViewer :cluster-id="clusterId" :kind="kind" :namespace="namespace" :name="name" />
          </div>

          <div v-else-if="activeTab === 'tasks' && showTasks" class="bg-white border border-slate-200 rounded p-4 space-y-3">
            <h3 class="text-sm font-medium text-slate-700 flex items-center gap-2">
              <Wrench class="h-4 w-4" />
              Available Actions
            </h3>
            <div class="flex items-center gap-2">
              <button
                :disabled="diagnosisLoading"
                class="flex items-center gap-1 px-3 py-1.5 bg-indigo-600 text-white text-xs rounded hover:bg-indigo-700 disabled:opacity-50"
                @click="runDiagnosis"
              >
                <Activity class="h-3 w-3" />
                {{ diagnosisLoading ? 'Running...' : 'Run Diagnosis' }}
              </button>
            </div>
            <div v-if="diagnosisResult" class="text-xs text-slate-600 bg-slate-50 border border-slate-200 rounded p-2">
              {{ diagnosisResult }}
            </div>
            <div class="text-xs text-slate-500">
              <AlertTriangle class="inline h-3 w-3 mr-1" />
              Actions are read-only. No write operations are performed from this view.
            </div>
          </div>

          <div v-else-if="activeTab === 'rollout' && showRollout" class="bg-white border border-slate-200 rounded p-4 space-y-4">
            <div v-if="rolloutLoading" class="text-sm text-slate-500">正在读取发布状态...</div>
            <div v-else-if="rolloutError" class="text-sm text-red-600">{{ rolloutError }}</div>
            <template v-else>
              <section v-if="rolloutStatus" class="space-y-2">
                <h3 class="text-sm font-medium text-slate-700">Rollout Status</h3>
                <div class="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
                  <div class="bg-slate-50 border border-slate-200 rounded p-2"><div class="text-slate-500">Phase</div><div class="font-mono">{{ rolloutStatus.phase }}</div></div>
                  <div class="bg-slate-50 border border-slate-200 rounded p-2"><div class="text-slate-500">Current Revision</div><div class="font-mono">{{ rolloutStatus.current_revision }}</div></div>
                  <div class="bg-slate-50 border border-slate-200 rounded p-2"><div class="text-slate-500">Desired / Ready</div><div class="font-mono">{{ rolloutStatus.desired_replicas }} / {{ rolloutStatus.ready_replicas }}</div></div>
                  <div class="bg-slate-50 border border-slate-200 rounded p-2"><div class="text-slate-500">Updated / Available</div><div class="font-mono">{{ rolloutStatus.updated_replicas }} / {{ rolloutStatus.available_replicas }}</div></div>
                </div>
                <div v-if="rolloutStatus.unavailable_replicas > 0" class="text-xs text-amber-600">Unavailable: {{ rolloutStatus.unavailable_replicas }}</div>
                <div v-if="rolloutStatus.reason || rolloutStatus.message" class="text-xs text-slate-600 border border-slate-200 rounded p-2">
                  <span class="font-mono">{{ rolloutStatus.reason || '--' }}</span>: {{ rolloutStatus.message || '--' }}
                </div>
                <div v-if="rolloutStatus.conditions?.length" class="space-y-1">
                  <div class="text-xs text-slate-500">Conditions</div>
                  <div v-for="cond in rolloutStatus.conditions" :key="cond.type" class="text-xs border border-slate-200 rounded p-2 flex items-center gap-2">
                    <span :class="['h-2 w-2 rounded-full', cond.status === 'True' ? 'bg-emerald-500' : 'bg-slate-300']" />
                    <span class="font-mono">{{ cond.type }} · {{ cond.status }}</span>
                    <span class="text-slate-500">{{ cond.reason || '' }}</span>
                  </div>
                </div>
              </section>
              <section v-if="rolloutHistory" class="space-y-2">
                <h3 class="text-sm font-medium text-slate-700">Revision History</h3>
                <div v-if="!rolloutHistory.revisions.length" class="text-xs text-slate-500">没有历史版本</div>
                <div v-else class="overflow-x-auto">
                  <table class="w-full text-xs border border-slate-200">
                    <thead class="bg-slate-50">
                      <tr>
                        <th class="text-left p-2 font-medium text-slate-600">Revision</th>
                        <th class="text-left p-2 font-medium text-slate-600">ReplicaSet</th>
                        <th class="text-left p-2 font-medium text-slate-600">Images</th>
                        <th class="text-left p-2 font-medium text-slate-600">Replicas</th>
                        <th class="text-left p-2 font-medium text-slate-600">Created</th>
                        <th class="text-left p-2 font-medium text-slate-600">State</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="rev in rolloutHistory.revisions" :key="rev.revision" :class="rev.current ? 'bg-emerald-50' : ''">
                        <td class="p-2 font-mono">{{ rev.revision }}</td>
                        <td class="p-2 font-mono">{{ rev.replicaset_name }}</td>
                        <td class="p-2 font-mono break-all">{{ rev.images.join(', ') || '--' }}</td>
                        <td class="p-2 font-mono">{{ rev.ready_replicas }}/{{ rev.replicas }}</td>
                        <td class="p-2 font-mono">{{ rev.created_at || '--' }}</td>
                        <td class="p-2"><span v-if="rev.current" class="px-1.5 py-0.5 text-emerald-700 bg-emerald-100 rounded">当前</span><span v-else class="text-slate-400">历史</span></td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section>
            </template>
          </div>
        </div>
      </div>
    </div>
  </ConsoleLayout>
</template>
