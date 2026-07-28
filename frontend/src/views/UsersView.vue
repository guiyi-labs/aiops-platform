<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Check, KeyRound, Pencil, Plus, RefreshCw, ShieldCheck, UserRoundCheck, UserRoundX, X } from 'lucide-vue-next'

import { APIError } from '../api/auth'
import { createUser, listUsers, resetUserPassword, updateUser } from '../api/users'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { ManagedUser, PlatformRole } from '../types/auth'

const roleOptions: Array<{ code: PlatformRole; label: string }> = [
  { code: 'system_admin', label: '系统管理员' },
  { code: 'operations_admin', label: '运维管理员' },
  { code: 'security_auditor', label: '安全审计员' },
  { code: 'viewer', label: '只读用户' },
]
const auth = useAuthStore()
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

function formatTime(value?: string): string {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '从未登录'
}

function roleLabel(role: PlatformRole): string {
  return roleOptions.find((item) => item.code === role)?.label || role
}

function managementError(error: unknown, fallback: string): string {
  if (!(error instanceof APIError)) return fallback
  if (error.code === 'USERNAME_EXISTS') return '用户名已存在。'
  if (error.code === 'SELF_PROTECTION') return '不能停用当前账号或修改自己的角色。'
  if (error.code === 'LAST_SYSTEM_ADMIN') return '必须保留至少一个活跃的系统管理员。'
  if (error.code === 'INVALID_USER') return '请检查用户名、显示名、密码长度和角色。'
  if (error.code === 'INVALID_PASSWORD') return '新密码必须为 12–128 个字符。'
  if (error.code === 'SELF_PASSWORD_RESET') return '请通过后续个人密码修改流程更新当前账号。'
  return fallback
}

async function loadUsers() {
  loading.value = true; errorMessage.value = ''
  try { const response = await listUsers(auth.accessToken); users.value = response.items; total.value = response.total }
  catch { errorMessage.value = '无法加载用户列表，请确认系统管理员权限。' }
  finally { loading.value = false }
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
    <section class="user-toolbar"><div><strong>平台账号 · {{ total }}</strong><span>创建账号、分配角色并管理登录状态。</span></div><div><button class="secondary-button" :disabled="loading" @click="loadUsers"><RefreshCw :size="15" :class="{ spinning: loading }" />刷新</button><button class="primary-button" @click="showCreate = !showCreate"><Plus :size="15" />创建账号</button></div></section>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p><p v-if="notice" class="user-notice">{{ notice }}</p>
    <section v-if="showCreate" class="user-create-panel"><header><div><ShieldCheck :size="18" /><strong>创建平台账号</strong></div><button class="icon-button" aria-label="关闭创建账号" @click="showCreate = false"><X :size="17" /></button></header><div class="user-form-grid"><label>用户名<input v-model="createForm.username" maxlength="64" placeholder="lowercase.username" /></label><label>显示名<input v-model="createForm.display_name" maxlength="128" placeholder="值班运维" /></label><label>初始密码<input v-model="createForm.password" type="password" maxlength="128" placeholder="至少 12 个字符" /></label></div><div class="role-checks"><label v-for="role in roleOptions" :key="role.code"><input v-model="createForm.roles" type="checkbox" :value="role.code" />{{ role.label }}</label></div><button class="primary-button" :disabled="saving || !createForm.username || !createForm.display_name || createForm.password.length < 12 || !createForm.roles.length" @click="submitCreate"><Check :size="15" />确认创建</button></section>
    <section class="user-list"><article v-for="user in users" :key="user.id" class="user-card"><div class="user-avatar">{{ user.display_name.slice(0, 2).toUpperCase() }}</div><div class="user-main"><div><strong>{{ user.display_name }}</strong><span>@{{ user.username }}</span><em :class="user.status">{{ user.status }}</em></div><small>上次登录：{{ formatTime(user.last_login_at) }} · 创建：{{ formatTime(user.created_at) }}</small><div class="user-role-badges"><span v-for="role in user.roles" :key="role">{{ roleLabel(role) }}</span></div><div v-if="editingID === user.id" class="user-edit-panel"><input v-model="editDisplayName" maxlength="128" aria-label="编辑显示名" /><div v-if="user.id !== auth.user?.id" class="role-checks"><label v-for="role in roleOptions" :key="role.code"><input v-model="editRoles" type="checkbox" :value="role.code" />{{ role.label }}</label></div><small v-else>为保护当前会话，只能修改自己的显示名。</small><div><button class="primary-button" :disabled="saving || !editDisplayName.trim() || (user.id !== auth.user?.id && !editRoles.length)" @click="saveEdit(user)"><Check :size="14" />保存</button><button class="secondary-button" @click="editingID = 0">取消</button></div></div><div v-if="resettingID === user.id" class="user-edit-panel"><input v-model="replacementPassword" type="password" maxlength="128" aria-label="新密码" placeholder="输入 12–128 个字符" /><small>提交后该账号的访问令牌和刷新会话立即失效。</small><div><button class="primary-button" :disabled="saving || replacementPassword.length < 12" @click="submitPasswordReset(user)"><KeyRound :size="14" />确认重置</button><button class="secondary-button" @click="resettingID = 0">取消</button></div></div></div><div class="user-actions"><button class="secondary-button" :disabled="saving" @click="beginEdit(user)"><Pencil :size="14" />编辑</button><button class="secondary-button" :disabled="saving || user.id === auth.user?.id" @click="beginPasswordReset(user)"><KeyRound :size="14" />重置密码</button><button class="secondary-button" :disabled="saving || user.id === auth.user?.id" @click="toggleStatus(user)"><UserRoundX v-if="user.status === 'active'" :size="14" /><UserRoundCheck v-else :size="14" />{{ user.status === 'active' ? '停用' : '启用' }}</button></div></article><div v-if="!loading && users.length === 0" class="resource-empty">暂无平台账号</div></section>
  </ConsoleLayout>
</template>
