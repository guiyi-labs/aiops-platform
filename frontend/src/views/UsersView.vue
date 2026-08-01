<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Check, KeyRound, Network, Pencil, Plus, RefreshCw, ShieldCheck, Trash2, UserRoundCheck, UserRoundX, X } from 'lucide-vue-next'

import { APIError } from '../api/auth'
import {
  createClusterGrant,
  createNamespaceGrant,
  deleteClusterGrant,
  deleteNamespaceGrant,
  listClusterGrants,
  listNamespaceGrants,
} from '../api/grants'
import { listClusters } from '../api/clusters'
import { createUser, listUsers, resetUserPassword, updateUser } from '../api/users'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { Cluster } from '../types/cluster'
import type { ClusterGrant, NamespaceGrant } from '../types/grants'
import type { ManagedUser, PlatformRole } from '../types/auth'

const roleOptions: Array<{ code: PlatformRole; label: string }> = [
  { code: 'system_admin', label: '系统管理员' },
  { code: 'operations_admin', label: '运维管理员' },
  { code: 'security_auditor', label: '安全审计员' },
  { code: 'viewer', label: '只读用户' },
]
const auth = useAuthStore()
const canManageGrants = computed(() => Boolean(auth.user?.roles?.includes('system_admin')))
const users = ref<ManagedUser[]>([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const notice = ref('')
const showCreate = ref(false)
const createForm = ref<{ username: string; display_name: string; password: string; roles: PlatformRole[] }>({ username: '', display_name: '', password: '', roles: ['viewer'] })
const editingID = ref(0)
const editDisplayName = ref('')
const editRoles = ref<PlatformRole[]>([])
const resettingID = ref(0)
const replacementPassword = ref('')

// Per-user access grants state. Keys are user IDs.
const expandedGrants = ref<Record<number, boolean>>({})
const grantsLoading = ref<Record<number, boolean>>({})
const grantsError = ref<Record<number, string>>({})
const clusterGrantsByUser = ref<Record<number, ClusterGrant[]>>({})
const namespaceGrantsByUser = ref<Record<number, NamespaceGrant[]>>({})
const clusters = ref<Cluster[]>([])
const clustersLoaded = ref(false)
// Form state per user
const newClusterID = ref<Record<number, number | ''>>({})
const newNsClusterID = ref<Record<number, number | ''>>({})
const newNamespace = ref<Record<number, string>>({})
const grantSaving = ref<Record<number, boolean>>({})

function formatTime(value?: string): string {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '从未登录'
}

function roleLabel(role: PlatformRole): string {
  return roleOptions.find((item) => item.code === role)?.label || role
}

function clusterName(clusterID: number): string {
  const cluster = clusters.value.find((c) => c.id === clusterID)
  return cluster ? cluster.name : `集群 #${clusterID}`
}

function managementError(error: unknown, fallback: string): string {
  if (!(error instanceof APIError)) return fallback
  if (error.code === 'USERNAME_EXISTS') return '用户名已存在。'
  if (error.code === 'SELF_PROTECTION') return '不能停用当前账号或修改自己的角色。'
  if (error.code === 'LAST_SYSTEM_ADMIN') return '必须保留至少一个活跃的系统管理员。'
  if (error.code === 'INVALID_USER') return '请检查用户名、显示名、密码长度和角色。'
  if (error.code === 'INVALID_PASSWORD') return '新密码必须为 12–128 个字符。'
  if (error.code === 'SELF_PASSWORD_RESET') return '请通过后续个人密码修改流程更新当前账号。'
  if (error.code === 'GRANT_EXISTS') return '该授权已存在，无需重复添加。'
  if (error.code === 'GRANT_NOT_FOUND') return '目标授权不存在，可能已被删除。'
  return fallback
}

async function loadUsers() {
  loading.value = true; errorMessage.value = ''
  try { const response = await listUsers(auth.accessToken); users.value = response.items; total.value = response.total }
  catch { errorMessage.value = '无法加载用户列表，请确认系统管理员权限。' }
  finally { loading.value = false }
}

async function loadClustersIfNeeded() {
  if (clustersLoaded.value) return
  try {
    const response = await listClusters(auth.accessToken)
    clusters.value = response.items
    clustersLoaded.value = true
  } catch {
    // Fall back to numeric IDs if clusters cannot be listed; the dropdown will
    // be empty but the panel still shows the existing grants with numeric IDs.
  }
}

async function toggleGrantsPanel(user: ManagedUser) {
  const nowOpen = !expandedGrants.value[user.id]
  expandedGrants.value[user.id] = nowOpen
  if (!nowOpen) return
  grantsLoading.value[user.id] = true; grantsError.value[user.id] = ''
  try {
    await loadClustersIfNeeded()
    const [clusterResp, nsResp] = await Promise.all([
      listClusterGrants(auth.accessToken, user.id),
      listNamespaceGrants(auth.accessToken, user.id),
    ])
    clusterGrantsByUser.value[user.id] = clusterResp.items
    namespaceGrantsByUser.value[user.id] = nsResp.items
    if (!(user.id in newClusterID.value)) newClusterID.value[user.id] = ''
    if (!(user.id in newNsClusterID.value)) newNsClusterID.value[user.id] = ''
    if (!(user.id in newNamespace.value)) newNamespace.value[user.id] = ''
  } catch {
    grantsError.value[user.id] = '无法加载授权列表，请确认系统管理员权限。'
  } finally {
    grantsLoading.value[user.id] = false
  }
}

async function submitAddClusterGrant(user: ManagedUser) {
  const id = Number(newClusterID.value[user.id])
  if (!id) return
  grantSaving.value[user.id] = true; grantsError.value[user.id] = ''
  try {
    await createClusterGrant(auth.accessToken, user.id, id)
    newClusterID.value[user.id] = ''
    const response = await listClusterGrants(auth.accessToken, user.id)
    clusterGrantsByUser.value[user.id] = response.items
    notice.value = `已授予对 ${clusterName(id)} 的集群级访问。`
  } catch (error) { grantsError.value[user.id] = managementError(error, '新增集群授权失败。') }
  finally { grantSaving.value[user.id] = false }
}

async function submitDeleteClusterGrant(user: ManagedUser, grant: ClusterGrant) {
  grantSaving.value[user.id] = true; grantsError.value[user.id] = ''
  try {
    await deleteClusterGrant(auth.accessToken, user.id, grant.cluster_id)
    const response = await listClusterGrants(auth.accessToken, user.id)
    clusterGrantsByUser.value[user.id] = response.items
    notice.value = `已撤销对 ${clusterName(grant.cluster_id)} 的集群级访问。`
  } catch (error) { grantsError.value[user.id] = managementError(error, '撤销集群授权失败。') }
  finally { grantSaving.value[user.id] = false }
}

async function submitAddNamespaceGrant(user: ManagedUser) {
  const id = Number(newNsClusterID.value[user.id])
  const ns = newNamespace.value[user.id]?.trim()
  if (!id || !ns) return
  grantSaving.value[user.id] = true; grantsError.value[user.id] = ''
  try {
    await createNamespaceGrant(auth.accessToken, user.id, id, ns)
    newNsClusterID.value[user.id] = ''
    newNamespace.value[user.id] = ''
    const response = await listNamespaceGrants(auth.accessToken, user.id)
    namespaceGrantsByUser.value[user.id] = response.items
    notice.value = `已授予对 ${clusterName(id)} · ${ns} 的命名空间访问。`
  } catch (error) { grantsError.value[user.id] = managementError(error, '新增命名空间授权失败。') }
  finally { grantSaving.value[user.id] = false }
}

async function submitDeleteNamespaceGrant(user: ManagedUser, grant: NamespaceGrant) {
  grantSaving.value[user.id] = true; grantsError.value[user.id] = ''
  try {
    await deleteNamespaceGrant(auth.accessToken, user.id, grant.cluster_id, grant.namespace)
    const response = await listNamespaceGrants(auth.accessToken, user.id)
    namespaceGrantsByUser.value[user.id] = response.items
    notice.value = `已撤销对 ${clusterName(grant.cluster_id)} · ${grant.namespace} 的命名空间访问。`
  } catch (error) { grantsError.value[user.id] = managementError(error, '撤销命名空间授权失败。') }
  finally { grantSaving.value[user.id] = false }
}

async function submitCreate() {
  saving.value = true; errorMessage.value = ''; notice.value = ''
  try {
    await createUser(auth.accessToken, { ...createForm.value, username: createForm.value.username.trim(), display_name: createForm.value.display_name.trim() })
    createForm.value = { username: '', display_name: '', password: '', roles: ['viewer'] }
    showCreate.value = false; notice.value = '账号已创建。'; await loadUsers()
  } catch (error) { errorMessage.value = managementError(error, '账号创建失败。') }
  finally { saving.value = false }
}

function beginEdit(user: ManagedUser) {
  resettingID.value = 0; replacementPassword.value = ''; editingID.value = user.id; editDisplayName.value = user.display_name; editRoles.value = [...user.roles]
}

function beginPasswordReset(user: ManagedUser) {
  editingID.value = 0; resettingID.value = user.id; replacementPassword.value = ''; errorMessage.value = ''; notice.value = ''
}

async function submitPasswordReset(user: ManagedUser) {
  saving.value = true; errorMessage.value = ''; notice.value = ''
  try {
    await resetUserPassword(auth.accessToken, user.id, replacementPassword.value)
    resettingID.value = 0; replacementPassword.value = ''; notice.value = `已重置 ${user.username} 的密码，所有旧会话已失效。`
  } catch (error) { errorMessage.value = managementError(error, '密码重置失败。') }
  finally { saving.value = false }
}

async function saveEdit(user: ManagedUser) {
  saving.value = true; errorMessage.value = ''; notice.value = ''
  try {
    const input: { display_name: string; roles?: PlatformRole[] } = { display_name: editDisplayName.value.trim() }
    if (user.id !== auth.user?.id) input.roles = editRoles.value
    await updateUser(auth.accessToken, user.id, input)
    editingID.value = 0; notice.value = '账号资料已更新。'; await loadUsers()
  } catch (error) { errorMessage.value = managementError(error, '账号更新失败。') }
  finally { saving.value = false }
}

async function toggleStatus(user: ManagedUser) {
  saving.value = true; errorMessage.value = ''; notice.value = ''
  try {
    const status = user.status === 'active' ? 'disabled' : 'active'
    await updateUser(auth.accessToken, user.id, { status })
    notice.value = status === 'active' ? '账号已启用。' : '账号已停用，刷新会话已撤销。'
    await loadUsers()
  } catch (error) { errorMessage.value = managementError(error, '账号状态更新失败。') }
  finally { saving.value = false }
}

onMounted(loadUsers)
</script>

<template>
  <ConsoleLayout eyebrow="访问控制" title="用户管理">
    <section class="user-toolbar"><div><strong>平台账号 · {{ total }}</strong><span>创建账号、分配角色、管理登录状态以及分配集群与命名空间授权。</span></div><div><button class="secondary-button" :disabled="loading" @click="loadUsers"><RefreshCw :size="15" :class="{ spinning: loading }" />刷新</button><button class="primary-button" @click="showCreate = !showCreate"><Plus :size="15" />创建账号</button></div></section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p><p v-if="notice" class="user-notice">{{ notice }}</p>
    <section v-if="showCreate" class="user-create-panel"><header><div><ShieldCheck :size="18" /><strong>创建平台账号</strong></div><button class="icon-button" aria-label="关闭创建账号" @click="showCreate = false"><X :size="17" /></button></header><div class="user-form-grid"><label>用户名<input v-model="createForm.username" maxlength="64" placeholder="lowercase.username" /></label><label>显示名<input v-model="createForm.display_name" maxlength="128" placeholder="值班运维" /></label><label>初始密码<input v-model="createForm.password" type="password" maxlength="128" placeholder="至少 12 个字符" /></label></div><div class="role-checks"><label v-for="role in roleOptions" :key="role.code"><input v-model="createForm.roles" type="checkbox" :value="role.code" />{{ role.label }}</label></div><button class="primary-button" :disabled="saving || !createForm.username || !createForm.display_name || createForm.password.length < 12 || !createForm.roles.length" @click="submitCreate"><Check :size="15" />确认创建</button></section>
    <section class="user-list">
      <article v-for="user in users" :key="user.id" class="user-card">
        <div class="user-avatar">{{ user.display_name.slice(0, 2).toUpperCase() }}</div>
        <div class="user-main">
          <div><strong>{{ user.display_name }}</strong><span>@{{ user.username }}</span><em :class="user.status">{{ user.status }}</em></div>
          <small>上次登录：{{ formatTime(user.last_login_at) }} · 创建：{{ formatTime(user.created_at) }}</small>
          <div class="user-role-badges"><span v-for="role in user.roles" :key="role">{{ roleLabel(role) }}</span></div>
          <div v-if="editingID === user.id" class="user-edit-panel">
            <input v-model="editDisplayName" maxlength="128" aria-label="编辑显示名" />
            <div v-if="user.id !== auth.user?.id" class="role-checks"><label v-for="role in roleOptions" :key="role.code"><input v-model="editRoles" type="checkbox" :value="role.code" />{{ role.label }}</label></div>
            <small v-else>为保护当前会话，只能修改自己的显示名。</small>
            <div><button class="primary-button" :disabled="saving || !editDisplayName.trim() || (user.id !== auth.user?.id && !editRoles.length)" @click="saveEdit(user)"><Check :size="14" />保存</button><button class="secondary-button" @click="editingID = 0">取消</button></div>
          </div>
          <div v-if="resettingID === user.id" class="user-edit-panel">
            <input v-model="replacementPassword" type="password" maxlength="128" aria-label="新密码" placeholder="输入 12–128 个字符" />
            <small>提交后该账号的访问令牌和刷新会话立即失效。</small>
            <div><button class="primary-button" :disabled="saving || replacementPassword.length < 12" @click="submitPasswordReset(user)"><KeyRound :size="14" />确认重置</button><button class="secondary-button" @click="resettingID = 0">取消</button></div>
          </div>
          <div v-if="canManageGrants">
            <button class="secondary-button grants-toggle" :disabled="grantsLoading[user.id]" @click="toggleGrantsPanel(user)">
              <Network :size="14" />
              {{ expandedGrants[user.id] ? '收起授权管理' : '授权管理' }}
            </button>
            <div v-if="expandedGrants[user.id]" class="grants-panel">
              <p v-if="grantsLoading[user.id]" class="muted">正在加载授权信息…</p>
              <p v-else-if="grantsError[user.id]" class="error-message compact">{{ grantsError[user.id] }}</p>
              <template v-else>
                <section class="grants-subsection">
                  <header><strong>集群级授权</strong><small>获得集群级授权后，该用户可以访问集群内所有命名空间的资源。</small></header>
                  <ul class="grants-list" v-if="clusterGrantsByUser[user.id]?.length">
                    <li v-for="grant in clusterGrantsByUser[user.id]" :key="grant.id">
                      <span class="grants-item"><em>集群</em><strong>{{ clusterName(grant.cluster_id) }}</strong><small>{{ formatTime(grant.created_at) }}</small></span>
                      <button class="icon-button danger" :disabled="grantSaving[user.id]" aria-label="撤销集群授权" @click="submitDeleteClusterGrant(user, grant)"><Trash2 :size="14" /></button>
                    </li>
                  </ul>
                  <p v-else class="muted compact">暂无集群级授权。</p>
                  <div class="grants-add-row">
                    <select v-if="clusters.length" :value="newClusterID[user.id]" @change="(event: Event) => newClusterID[user.id] = Number((event.target as HTMLSelectElement).value) || ''">
                      <option value="" disabled>选择要授权的集群…</option>
                      <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
                    </select>
                    <input v-else type="number" min="1" :value="newClusterID[user.id]" @input="(event: Event) => newClusterID[user.id] = Number((event.target as HTMLInputElement).value) || ''" placeholder="集群 ID" />
                    <button class="primary-button compact" :disabled="!newClusterID[user.id] || grantSaving[user.id]" @click="submitAddClusterGrant(user)"><Plus :size="13" />添加</button>
                  </div>
                </section>
                <section class="grants-subsection">
                  <header><strong>命名空间级授权</strong><small>仅允许访问指定集群内的单个命名空间；无法枚举集群其他内容。</small></header>
                  <ul class="grants-list" v-if="namespaceGrantsByUser[user.id]?.length">
                    <li v-for="grant in namespaceGrantsByUser[user.id]" :key="grant.id">
                      <span class="grants-item"><em>命名空间</em><strong>{{ clusterName(grant.cluster_id) }} · {{ grant.namespace }}</strong><small>{{ formatTime(grant.created_at) }}</small></span>
                      <button class="icon-button danger" :disabled="grantSaving[user.id]" aria-label="撤销命名空间授权" @click="submitDeleteNamespaceGrant(user, grant)"><Trash2 :size="14" /></button>
                    </li>
                  </ul>
                  <p v-else class="muted compact">暂无命名空间级授权。</p>
                  <div class="grants-add-row namespace-row">
                    <select v-if="clusters.length" :value="newNsClusterID[user.id]" @change="(event: Event) => newNsClusterID[user.id] = Number((event.target as HTMLSelectElement).value) || ''">
                      <option value="" disabled>选择集群…</option>
                      <option v-for="c in clusters" :key="c.id" :value="c.id">{{ c.name }}</option>
                    </select>
                    <input v-else type="number" min="1" :value="newNsClusterID[user.id]" @input="(event: Event) => newNsClusterID[user.id] = Number((event.target as HTMLInputElement).value) || ''" placeholder="集群 ID" />
                    <input v-model="newNamespace[user.id]" maxlength="253" placeholder="命名空间名称，例如 default 或 team-a" />
                    <button class="primary-button compact" :disabled="!newNsClusterID[user.id] || !newNamespace[user.id]?.trim() || grantSaving[user.id]" @click="submitAddNamespaceGrant(user)"><Plus :size="13" />添加</button>
                  </div>
                </section>
              </template>
            </div>
          </div>
        </div>
        <div class="user-actions">
          <button class="secondary-button" :disabled="saving" @click="beginEdit(user)"><Pencil :size="14" />编辑</button>
          <button class="secondary-button" :disabled="saving || user.id === auth.user?.id" @click="beginPasswordReset(user)"><KeyRound :size="14" />重置密码</button>
          <button class="secondary-button" :disabled="saving || user.id === auth.user?.id" @click="toggleStatus(user)"><UserRoundX v-if="user.status === 'active'" :size="14" /><UserRoundCheck v-else :size="14" />{{ user.status === 'active' ? '停用' : '启用' }}</button>
        </div>
      </article>
      <div v-if="!loading && users.length === 0" class="resource-empty">暂无平台账号</div>
    </section>
  </ConsoleLayout>
</template>

<style scoped>
.grants-toggle {
  margin-top: 0.75rem;
}
.grants-panel {
  display: grid;
  gap: 0.75rem;
  margin-top: 0.75rem;
  padding: 0.75rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--surface-2);
}
.grants-subsection header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.75rem;
  margin-bottom: 0.35rem;
}
.grants-subsection header small {
  color: var(--muted);
}
.grants-list {
  list-style: none;
  padding: 0;
  margin: 0 0 0.5rem;
  display: grid;
  gap: 0.35rem;
}
.grants-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.45rem 0.6rem;
  background: var(--surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}
.grants-item {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.grants-item em {
  font-style: normal;
  font-size: 0.75rem;
  padding: 0.08rem 0.4rem;
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--muted);
}
.grants-item small {
  color: var(--muted);
}
.grants-add-row {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}
.grants-add-row.namespace-row input:nth-of-type(2) {
  flex: 1 1 220px;
}
.grants-add-row select,
.grants-add-row input {
  flex: 1 1 160px;
  padding: 0.45rem 0.55rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-subtle);
  background: var(--surface);
  color: var(--text);
}
.muted.compact,
.error-message.compact {
  padding: 0.25rem 0.1rem;
  margin: 0;
}
.icon-button.danger {
  color: var(--danger);
}
.icon-button.danger:hover {
  background: color-mix(in srgb, var(--danger) 12%, transparent);
}
</style>
