<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import ClustersView from './ClustersView.vue'
import ModelAccessView from './ModelAccessView.vue'
import UsersView from './UsersView.vue'
import AuditLogsView from './AuditLogsView.vue'
import AccountSecurityView from './AccountSecurityView.vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const tabs = computed(() => [
  { id: 'clusters', label: '集群接入', path: '/settings/clusters', component: ClustersView },
  { id: 'model', label: '模型配置', path: '/settings/model', component: ModelAccessView, roles: ['system_admin'] },
  { id: 'users', label: '用户管理', path: '/settings/users', component: UsersView, roles: ['system_admin'] },
  { id: 'audit', label: '审计记录', path: '/settings/audit', component: AuditLogsView, roles: ['system_admin', 'security_auditor'] },
  { id: 'account', label: '账号安全', path: '/settings/account', component: AccountSecurityView },
].filter(tab => !tab.roles || auth.user?.roles.some(role => tab.roles!.includes(role))))
const active = computed(() => tabs.value.find(tab => tab.id === route.meta.tab) ?? tabs.value[0]!)
</script>

<template>
  <ConsoleLayout eyebrow="系统设置" title="系统设置">
    <nav class="workspace-tabs" aria-label="系统设置">
      <RouterLink v-for="tab in tabs" :key="tab.id" :to="tab.path" :class="{ active: active.id === tab.id }" :aria-current="active.id === tab.id ? 'page' : undefined">{{ tab.label }}</RouterLink>
    </nav>
    <component :is="active.component" />
  </ConsoleLayout>
</template>
