<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { KeyRound, Plus, RefreshCw, Server, Trash2, Wifi, X, Boxes } from 'lucide-vue-next'

import * as clusterAPI from '../api/clusters'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import EmptyState from '../components/EmptyState.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'

const auth = useAuthStore()
const router = useRouter()
const clusters = ref<Cluster[]>([])
const loading = ref(true)
const busyID = ref<number | null>(null)
const showForm = ref(false)
const name = ref('')
const kubeconfig = ref('')
const errorMessage = ref('')
const notice = ref('')
const rotatingID = ref<number | null>(null)
const replacementKubeconfig = ref('')
const canManage = computed(() => auth.user?.roles.includes('system_admin') ?? false)

const statusLabels: Record<Cluster['status'], string> = { disabled: '已停用', unknown: '待探测', ready: '就绪', unreachable: '不可达' }

async function load() {
  loading.value = true
  errorMessage.value = ''
  try { clusters.value = (await clusterAPI.listClusters(auth.accessToken)).items }
  catch { errorMessage.value = '无法加载集群列表' }
  finally { loading.value = false }
}

async function create() {
  errorMessage.value = ''; notice.value = ''
  try {
    await clusterAPI.createCluster(auth.accessToken, name.value.trim(), kubeconfig.value)
    name.value = ''; kubeconfig.value = ''; showForm.value = false
    notice.value = `集群「${name.value || '新集群'}」已登记，正在探测连接...`
    await load()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '集群保存失败'
  }
}

function beginCredentialRotation(item: Cluster) {
  rotatingID.value = item.id; replacementKubeconfig.value = ''; errorMessage.value = ''; notice.value = ''
}

async function rotateCredential(item: Cluster) {
  busyID.value = item.id; errorMessage.value = ''; notice.value = ''
  try {
    await clusterAPI.updateClusterCredential(auth.accessToken, item.id, replacementKubeconfig.value)
    rotatingID.value = null; replacementKubeconfig.value = ''; notice.value = `集群 ${item.name} 的凭据已替换，请重新执行连接探测。`; await load()
  } catch (error) { errorMessage.value = error instanceof Error ? error.message : '凭据替换失败' }
  finally { busyID.value = null }
}

async function probe(item: Cluster) {
  busyID.value = item.id
  try { await clusterAPI.probeCluster(auth.accessToken, item.id); notice.value = `集群「${item.name}」探测完成`; await load() }
  catch { await load(); errorMessage.value = '连接探测失败，请检查 API 地址、证书与网络' }
  finally { busyID.value = null }
}

async function toggle(item: Cluster) {
  busyID.value = item.id
  try { await clusterAPI.setClusterEnabled(auth.accessToken, item.id, !item.enabled); await load() }
  finally { busyID.value = null }
}

async function remove(item: Cluster) {
  if (!window.confirm(`确认移除集群"${item.name}"？凭据将一并删除。`)) return
  busyID.value = item.id
  try { await clusterAPI.deleteCluster(auth.accessToken, item.id); notice.value = `集群「${item.name}」已移除`; await load() }
  finally { busyID.value = null }
}

function openFirstClusterGuide() {
  showForm.value = true
}

onMounted(load)
</script>

<template>
  <ConsoleLayout eyebrow="多集群管理" title="集群接入">
    <section class="page-toolbar">
      <div><strong>{{ clusters.length }}</strong><span> 个已登记集群</span></div>
      <div class="toolbar-actions">
        <button class="secondary-button" type="button" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新</button>
        <button v-if="canManage" class="primary-button" type="button" @click="showForm = !showForm"><Plus :size="16" />接入集群</button>
      </div>
    </section>

    <!-- Prominent onboarding form when triggered -->
    <form v-if="showForm" class="cluster-form" @submit.prevent="create">
      <div class="section-heading"><div><p class="context-label">凭据导入</p><h2>登记 Kubernetes 集群</h2></div></div>
      <label for="cluster-name">集群名称</label>
      <input id="cluster-name" v-model="name" required maxlength="128" placeholder="例如 kind-dev" />
      <label for="kubeconfig">Kubeconfig</label>
      <textarea id="kubeconfig" v-model="kubeconfig" required rows="9" spellcheck="false" placeholder="粘贴包含 current-context 的 kubeconfig；保存后不会回显" />
      <div class="form-actions"><button class="secondary-button" type="button" @click="showForm = false">取消</button><button class="primary-button" type="submit">加密保存</button></div>
    </form>

    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <p v-if="notice" class="user-notice">{{ notice }}</p>

    <section class="cluster-list" aria-label="集群列表">
      <!-- Loading state -->
      <div v-if="loading" class="empty-state">
        <RefreshCw class="spinning" :size="24" />
        <span>正在加载集群</span>
      </div>

      <!-- Empty state: hero variant with prominent CTA -->
      <EmptyState
        v-else-if="clusters.length === 0 && !showForm"
        hero
        :icon="Boxes"
        title="欢迎使用 AIOps 平台"
        description="接入你的第一个 Kubernetes 集群，开始智能运维之旅。支持任何标准 kubeconfig 的集群，包括 Kind、EKS、ACK 等。"
      >
        <button class="primary-button cluster-cta" type="button" @click="openFirstClusterGuide">
          <Plus :size="18" /> 接入第一个集群
        </button>
        <p class="cluster-hint">或前往 <a href="#" @click.prevent="router.push('/clusters?demo=1')">体验演示集群</a></p>
      </EmptyState>

      <!-- Cluster list -->
      <article v-for="item in clusters" v-else :key="item.id" class="cluster-card">
        <div class="cluster-main"><span class="service-icon"><Server :size="19" /></span><div><strong>{{ item.name }}</strong><span>{{ item.api_server }}</span></div></div>
        <span class="status-pill" :class="item.status">{{ statusLabels[item.status] }}</span>
        <div class="cluster-version">{{ item.kubernetes_version || '版本未知' }}</div>
        <div v-if="rotatingID === item.id" class="cluster-credential-form">
          <div><strong>替换连接凭据</strong><button class="icon-button" type="button" aria-label="关闭凭据替换" @click="rotatingID = null"><X :size="16" /></button></div>
          <textarea v-model="replacementKubeconfig" rows="7" spellcheck="false" placeholder="粘贴新的 kubeconfig；提交后旧客户端缓存会失效" />
          <div class="form-actions"><button class="secondary-button" type="button" @click="rotatingID = null">取消</button><button class="primary-button" type="button" :disabled="busyID === item.id || !replacementKubeconfig.trim()" @click="rotateCredential(item)"><KeyRound :size="14" />替换并加密</button></div>
        </div>
        <div class="cluster-actions">
          <button class="text-button" type="button" :disabled="busyID === item.id" @click="probe(item)"><Wifi :size="15" />探测</button>
          <button v-if="canManage" class="text-button" type="button" :disabled="busyID === item.id" @click="beginCredentialRotation(item)"><KeyRound :size="15" />轮换凭据</button>
          <button v-if="canManage" class="text-button" type="button" :disabled="busyID === item.id" @click="toggle(item)">{{ item.enabled ? '停用' : '启用' }}</button>
          <button v-if="canManage" class="danger-button" type="button" :disabled="busyID === item.id" title="移除集群" @click="remove(item)"><Trash2 :size="15" /></button>
        </div>
      </article>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.cluster-cta {
  margin-top: 8px;
  padding: 12px 28px;
  font-size: 15px;
  font-weight: 600;
  border-radius: 8px;
}
.cluster-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--text-tertiary);
}
.cluster-hint a {
  color: var(--accent-primary-active);
  text-decoration: none;
  font-weight: 500;
}
.cluster-hint a:hover { text-decoration: underline; }
</style>
