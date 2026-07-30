<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowRight, CheckCircle2, Layers, RefreshCw, Send, XCircle } from 'lucide-vue-next'

import * as clusterAPI from '../api/clusters'
import * as promotionAPI from '../api/promotion'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { PromotionBundleItemRequest, PromotionDependencyMapping, PromotionKind, PromotionPlan } from '../types/promotion'

const auth = useAuthStore()
const clusters = ref<Cluster[]>([])
const loading = ref(true)
const errorMessage = ref('')
const notice = ref('')
const step = ref<1 | 2 | 3 | 4>(1)

const sourceClusterID = ref<number | null>(null)
const destinationClusterID = ref<number | null>(null)
const sourceNamespace = ref('')
const destinationNamespace = ref('')

const bundle = ref<PromotionBundleItemRequest[]>([])
const newBundleKind = ref<PromotionKind>('Deployment')
const newBundleName = ref('')

const dependencyMappings = ref<PromotionDependencyMapping[]>([])
const newDepKind = ref<'ConfigMap' | 'Secret'>('ConfigMap')
const newDepSourceName = ref('')
const newDepDestinationName = ref('')

const previewing = ref(false)
const executing = ref(false)
const plan = ref<PromotionPlan | null>(null)

const canManage = computed(() => (auth.user?.roles.includes('system_admin') || auth.user?.roles.includes('operations_admin')) ?? false)

const sourceCluster = computed(() => clusters.value.find((c) => c.id === sourceClusterID.value) ?? null)
const destinationCluster = computed(() => clusters.value.find((c) => c.id === destinationClusterID.value) ?? null)

const step1Valid = computed(() =>
  sourceClusterID.value !== null
  && destinationClusterID.value !== null
  && sourceClusterID.value !== destinationClusterID.value
  && sourceNamespace.value.trim() !== ''
  && destinationNamespace.value.trim() !== '',
)

const step2Valid = computed(() => bundle.value.length > 0)

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    clusters.value = (await clusterAPI.listClusters(auth.accessToken)).items.filter((c) => c.enabled)
  } catch {
    errorMessage.value = '无法加载集群列表'
  } finally {
    loading.value = false
  }
}

function addBundleItem() {
  const name = newBundleName.value.trim()
  if (!name) return
  const exists = bundle.value.some((item) => item.kind === newBundleKind.value && item.name === name && item.namespace === sourceNamespace.value.trim())
  if (exists) {
    errorMessage.value = '该资源已添加到 bundle'
    return
  }
  bundle.value.push({ kind: newBundleKind.value, namespace: sourceNamespace.value.trim(), name })
  newBundleName.value = ''
  errorMessage.value = ''
}

function removeBundleItem(index: number) {
  bundle.value.splice(index, 1)
}

function addDependencyMapping() {
  const sourceName = newDepSourceName.value.trim()
  const destName = newDepDestinationName.value.trim()
  if (!sourceName || !destName) return
  const exists = dependencyMappings.value.some((m) => m.kind === newDepKind.value && m.source_name === sourceName)
  if (exists) {
    errorMessage.value = '该依赖映射已存在'
    return
  }
  dependencyMappings.value.push({
    kind: newDepKind.value,
    source_namespace: sourceNamespace.value.trim(),
    source_name: sourceName,
    destination_namespace: destinationNamespace.value.trim(),
    destination_name: destName,
  })
  newDepSourceName.value = ''
  newDepDestinationName.value = ''
  errorMessage.value = ''
}

function removeDependencyMapping(index: number) {
  dependencyMappings.value.splice(index, 1)
}

async function runPreview() {
  if (!sourceClusterID.value || !destinationClusterID.value) return
  previewing.value = true
  errorMessage.value = ''
  try {
    plan.value = await promotionAPI.previewPromotion(auth.accessToken, {
      source_cluster_id: sourceClusterID.value,
      destination_cluster_id: destinationClusterID.value,
      source_namespace: sourceNamespace.value.trim(),
      destination_namespace: destinationNamespace.value.trim(),
      bundle: bundle.value,
      dependency_mappings: dependencyMappings.value.length > 0 ? dependencyMappings.value : undefined,
    })
    step.value = 3
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '预检失败，请检查集群状态与资源冲突'
  } finally {
    previewing.value = false
  }
}

async function runExecute() {
  if (!plan.value?.id || !plan.value?.confirmation_token) return
  executing.value = true
  errorMessage.value = ''
  try {
    const idempotencyKey = `promotion-${plan.value.id}-${Date.now()}`
    plan.value = await promotionAPI.executePromotion(auth.accessToken, plan.value.id, plan.value.confirmation_token, idempotencyKey)
    notice.value = 'Promotion 已执行完成'
    step.value = 4
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '执行失败，Kubernetes 可能拒绝了变更'
  } finally {
    executing.value = false
  }
}

function resetWizard() {
  step.value = 1
  sourceClusterID.value = null
  destinationClusterID.value = null
  sourceNamespace.value = ''
  destinationNamespace.value = ''
  bundle.value = []
  dependencyMappings.value = []
  plan.value = null
  errorMessage.value = ''
  notice.value = ''
}

const statusLabels: Record<string, string> = {
  awaiting_confirmation: '待确认',
  executing: '执行中',
  succeeded: '成功',
  failed: '失败',
  partial: '部分成功',
  expired: '已过期',
}

const itemStatusLabels: Record<string, string> = {
  pending: '待执行',
  applied: '已应用',
  failed: '失败',
  skipped: '已跳过',
}

onMounted(load)
</script>

<template>
  <ConsoleLayout eyebrow="跨集群治理" title="Promotion 向导">
    <section class="page-toolbar">
      <div><strong>{{ bundle.length }}</strong><span> 个 bundle 项</span></div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新集群</button>
      </div>
    </section>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="notice" class="user-notice">{{ notice }}</p>

    <ol class="wizard-steps">
      <li :class="{ active: step === 1, done: step > 1 }">1. 集群与命名空间</li>
      <li :class="{ active: step === 2, done: step > 2 }">2. Bundle 组装</li>
      <li :class="{ active: step === 3, done: step > 3 }">3. 预检确认</li>
      <li :class="{ active: step === 4 }">4. 执行结果</li>
    </ol>

    <!-- Step 1: Clusters & Namespaces -->
    <section v-if="step === 1" class="wizard-panel">
      <div class="form-grid">
        <div class="form-field">
          <label for="source-cluster">源集群</label>
          <select id="source-cluster" v-model="sourceClusterID" :disabled="loading">
            <option :value="null" disabled>选择源集群…</option>
            <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }} ({{ c.status }})</option>
          </select>
        </div>
        <div class="form-field">
          <label for="source-namespace">源命名空间</label>
          <input id="source-namespace" v-model="sourceNamespace" placeholder="例如 demo" maxlength="63" />
        </div>
        <div class="form-field">
          <label for="dest-cluster">目标集群</label>
          <select id="dest-cluster" v-model="destinationClusterID" :disabled="loading">
            <option :value="null" disabled>选择目标集群…</option>
            <option v-for="c in clusters" :key="c.id" :value="c.id" :disabled="c.id === sourceClusterID">{{ c.name }} ({{ c.status }})</option>
          </select>
        </div>
        <div class="form-field">
          <label for="dest-namespace">目标命名空间</label>
          <input id="dest-namespace" v-model="destinationNamespace" placeholder="例如 staging" maxlength="63" />
        </div>
      </div>
      <div class="form-actions">
        <button class="primary-button" type="button" :disabled="!step1Valid" @click="step = 2">下一步 <ArrowRight :size="16" /></button>
      </div>
    </section>

    <!-- Step 2: Bundle Assembly -->
    <section v-else-if="step === 2" class="wizard-panel">
      <div class="section-heading">
        <div><p class="context-label">Bundle 组装</p><h2>选择需要 promote 的资源</h2></div>
      </div>
      <div class="bundle-add-form">
        <select v-model="newBundleKind">
          <option value="Deployment">Deployment</option>
          <option value="Service">Service</option>
          <option value="Ingress">Ingress</option>
        </select>
        <input v-model="newBundleName" placeholder="资源名称（例如 api）" maxlength="253" @keydown.enter="addBundleItem" />
        <button class="secondary-button" type="button" @click="addBundleItem">添加</button>
      </div>
      <table v-if="bundle.length > 0" class="data-table">
        <thead><tr><th>#</th><th>Kind</th><th>Namespace</th><th>Name</th><th></th></tr></thead>
        <tbody>
          <tr v-for="(item, i) in bundle" :key="i">
            <td>{{ i + 1 }}</td>
            <td>{{ item.kind }}</td>
            <td>{{ item.namespace }}</td>
            <td>{{ item.name }}</td>
            <td><button class="icon-button" type="button" title="移除" @click="removeBundleItem(i)"><XCircle :size="16" /></button></td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-hint">尚未添加任何资源。源命名空间为 <strong>{{ sourceNamespace }}</strong>。</p>

      <details class="dependency-section">
        <summary>依赖映射（ConfigMap / Secret）</summary>
        <div class="bundle-add-form">
          <select v-model="newDepKind">
            <option value="ConfigMap">ConfigMap</option>
            <option value="Secret">Secret</option>
          </select>
          <input v-model="newDepSourceName" placeholder="源名称" maxlength="253" />
          <input v-model="newDepDestinationName" placeholder="目标名称" maxlength="253" />
          <button class="secondary-button" type="button" @click="addDependencyMapping">添加映射</button>
        </div>
        <table v-if="dependencyMappings.length > 0" class="data-table">
          <thead><tr><th>Kind</th><th>源</th><th>目标</th><th></th></tr></thead>
          <tbody>
            <tr v-for="(m, i) in dependencyMappings" :key="i">
              <td>{{ m.kind }}</td>
              <td>{{ m.source_namespace }}/{{ m.source_name }}</td>
              <td>{{ m.destination_namespace }}/{{ m.destination_name }}</td>
              <td><button class="icon-button" type="button" title="移除" @click="removeDependencyMapping(i)"><XCircle :size="16" /></button></td>
            </tr>
          </tbody>
        </table>
        <p v-else class="empty-hint">如果 Deployment 引用了 ConfigMap 或 Secret，需要提供映射以验证目标集群上存在同名对象。</p>
      </details>

      <div class="form-actions">
        <button class="secondary-button" type="button" @click="step = 1">上一步</button>
        <button class="primary-button" type="button" :disabled="!step2Valid || previewing" @click="runPreview">
          <Layers :size="16" /> {{ previewing ? '预检中…' : '预检 (Dry-Run)' }}
        </button>
      </div>
    </section>

    <!-- Step 3: Preview Confirmation -->
    <section v-else-if="step === 3 && plan" class="wizard-panel">
      <div class="section-heading">
        <div><p class="context-label">预检结果</p><h2>Plan {{ plan.id.slice(0, 8) }}…</h2></div>
      </div>
      <div class="plan-summary">
        <div><span>状态</span><strong>{{ statusLabels[plan.status] ?? plan.status }}</strong></div>
        <div><span>源</span><strong>{{ sourceCluster?.name }} / {{ plan.source_namespace }}</strong></div>
        <div><span>目标</span><strong>{{ destinationCluster?.name }} / {{ plan.destination_namespace }}</strong></div>
        <div><span>过期时间</span><strong>{{ new Date(plan.expires_at).toLocaleString() }}</strong></div>
      </div>

      <h3>Bundle 项 ({{ plan.items?.length ?? 0 }})</h3>
      <table class="data-table">
        <thead><tr><th>#</th><th>Kind</th><th>源</th><th>目标</th><th>状态</th></tr></thead>
        <tbody>
          <tr v-for="(item, i) in plan.items" :key="i">
            <td>{{ i + 1 }}</td>
            <td>{{ item.kind }}</td>
            <td>{{ item.source_namespace }}/{{ item.source_name }}</td>
            <td>{{ item.destination_namespace }}/{{ item.destination_name }}</td>
            <td>{{ itemStatusLabels[item.item_status] ?? item.item_status }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="plan.dependencies && plan.dependencies.length > 0">
        <h3>依赖映射 ({{ plan.dependencies.length }})</h3>
        <table class="data-table">
          <thead><tr><th>Kind</th><th>源</th><th>目标</th><th>已解析</th></tr></thead>
          <tbody>
            <tr v-for="(dep, i) in plan.dependencies" :key="i">
              <td>{{ dep.kind }}</td>
              <td>{{ dep.source_namespace }}/{{ dep.source_name }}</td>
              <td>{{ dep.destination_namespace }}/{{ dep.destination_name }}</td>
              <td>{{ dep.resolved ? '是' : '否' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="confirmation-warning">
        <p>确认令牌仅显示一次。执行后将无法撤回，请确认 dry-run 预检通过。</p>
      </div>

      <div class="form-actions">
        <button class="secondary-button" type="button" @click="resetWizard">取消</button>
        <button class="primary-button" type="button" :disabled="executing || !canManage" @click="runExecute">
          <Send :size="16" /> {{ executing ? '执行中…' : '确认执行' }}
        </button>
      </div>
    </section>

    <!-- Step 4: Execution Result -->
    <section v-else-if="step === 4 && plan" class="wizard-panel">
      <div class="result-banner" :class="plan.status">
        <CheckCircle2 v-if="plan.status === 'succeeded'" :size="32" />
        <XCircle v-else :size="32" />
        <div>
          <h2>{{ statusLabels[plan.status] ?? plan.status }}</h2>
          <p v-if="plan.last_error">{{ plan.last_error }}</p>
        </div>
      </div>

      <h3>Bundle 项执行结果</h3>
      <table class="data-table">
        <thead><tr><th>#</th><th>Kind</th><th>目标</th><th>状态</th><th>错误</th></tr></thead>
        <tbody>
          <tr v-for="(item, i) in plan.items" :key="i">
            <td>{{ i + 1 }}</td>
            <td>{{ item.kind }}</td>
            <td>{{ item.destination_namespace }}/{{ item.destination_name }}</td>
            <td>{{ itemStatusLabels[item.item_status] ?? item.item_status }}</td>
            <td>{{ item.last_error || '—' }}</td>
          </tr>
        </tbody>
      </table>

      <div class="form-actions">
        <button class="primary-button" type="button" @click="resetWizard">新建 Promotion</button>
      </div>
    </section>
  </ConsoleLayout>
</template>
