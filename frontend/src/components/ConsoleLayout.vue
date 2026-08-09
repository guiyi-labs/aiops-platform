<script setup lang="ts">
import {
  Activity,
  Bell,
  BellRing,
  Boxes,
  BoxIcon,
  Brain,
  ChevronRight,
  FileClock,
  FlaskConical,
  Gauge,
  GitBranch,
  Globe,
  Hammer,
  KeyRound,
  LayoutGrid,
  LifeBuoy,
  Link2,
  LogOut,
  Network,
  Package,
  Radar,
  Search,
  Send,
  ShieldCheck,
  Stethoscope,
  Shuffle,
  Target,
  Users,
  Wallet,
  Workflow,
} from 'lucide-vue-next'
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '../stores/auth'

defineProps<{ eyebrow: string; title: string }>()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const navigationGroups = computed(() => [
  {
    label: '工作台',
    items: [
      { label: '总览', icon: Gauge, route: '/' },
      { label: '集群', icon: Boxes, route: '/clusters' },
    ],
  },
  {
    label: '资源观察',
    items: [
      { label: '全局搜索', icon: Search, route: '/search' },
      { label: '资源工作台', icon: BoxIcon, route: '/workloads' },
      { label: '资源拓扑', icon: Network, route: '/topology' },
      { label: '事件中心', icon: Bell, route: '/events' },
      { label: '监控大盘', icon: Gauge, route: '/monitoring' },
      { label: '事件流与告警', icon: Activity, route: '/event-stream' },
      { label: '服务网格', icon: Globe, route: '/service-mesh' },
    ],
  },
  {
    label: '智能运维',
    items: [
      { label: 'AIOps 概览', icon: Activity, route: '/aiops/overview' },
      { label: 'SLO 仪表盘', icon: Target, route: '/aiops/slo' },
      { label: '关联案例', icon: Link2, route: '/aiops/correlation' },
      { label: 'AI 调查', icon: Brain, route: '/aiops/investigator' },
      { label: '自动化控制台', icon: Workflow, route: '/aiops/automation', adminOnly: true },
      { label: '智能巡检', icon: Radar, route: '/inspection' },
      { label: '质量仪表盘', icon: FlaskConical, route: '/aiops/quality' },
    ],
  },
  {
    label: '分析与治理',
    items: [
      { label: '智能诊断', icon: Stethoscope, route: '/diagnoses' },
      { label: '告警规则', icon: BellRing, route: '/alerts' },
      { label: '命名空间治理', icon: LayoutGrid, route: '/namespace-posture' },
      { label: '治理态势', icon: ShieldCheck, route: '/posture' },
      { label: '优化中心', icon: Wallet, route: '/optimization' },
      { label: '跨集群 Promotion', icon: Shuffle, route: '/promotions', adminOnly: true },
      { label: '工作负载保护', icon: ShieldCheck, route: '/workload-protection' },
      { label: '节点维护', icon: Hammer, route: '/node-maintenance' },
      { label: '恢复演练', icon: LifeBuoy, route: '/restore-rehearsal' },
      { label: '通知投递', icon: Send, route: '/notifications', auditOnly: true },
      { label: '审计日志', icon: FileClock, route: '/audit-logs', auditOnly: true },
      { label: '用户管理', icon: Users, route: '/users', adminOnly: true },
    ],
  },
  {
    label: '交付与运维',
    items: [
      { label: 'Helm 应用目录', icon: Package, route: '/app-catalog' },
      { label: 'GitOps 应用', icon: GitBranch, route: '/gitops' },
      { label: '跨集群复制', icon: Shuffle, route: '/copyops', adminOnly: true },
    ],
  },
].map((group) => ({
  ...group,
  items: group.items.filter((item) =>
    (!item.auditOnly || auth.user?.roles.some((role) => role === 'system_admin' || role === 'security_auditor'))
    && (!item.adminOnly || auth.user?.roles.includes('system_admin'))),
})).filter((group) => group.items.length > 0))

const roleLabel = computed(() => {
  const role = auth.user?.roles[0]
  return ({ system_admin: '系统管理员', operations_admin: '运维管理员', viewer: '只读观察员', security_auditor: '安全审计员' } as Record<string, string>)[role ?? ''] ?? '平台用户'
})

async function logout() {
  await auth.logout()
  await router.push({ name: 'login' })
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark"><Boxes :size="20" /></span>
        <span class="brand-copy"><strong>K8s AIOps</strong><small>Operations Console</small></span>
      </div>
      <div class="workspace-selector">
        <span class="environment-dot" />
        <div><strong>本地开发环境</strong><span>API connected</span></div>
        <ChevronRight :size="14" />
      </div>
      <nav class="navigation" aria-label="主导航">
        <div v-for="group in navigationGroups" :key="group.label" class="nav-group">
          <p class="nav-group-label">{{ group.label }}</p>
          <button
            v-for="item in group.items"
            :key="item.label"
            type="button"
            class="nav-item"
            :class="{ active: item.route === route.path }"
            :title="item.label"
            @click="router.push(item.route)"
          >
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
          </button>
        </div>
      </nav>
    </aside>
    <main class="main-content">
      <header class="topbar">
        <div class="topbar-title"><p class="context-label">{{ eyebrow }}</p><h1>{{ title }}</h1></div>
        <div class="topbar-actions">
          <slot name="actions" />
          <div class="topbar-user"><strong>{{ auth.user?.display_name }}</strong><span>{{ roleLabel }}</span></div>
          <span class="avatar" aria-label="当前用户">{{ auth.userInitials }}</span>
          <button type="button" class="icon-button" title="账号安全" aria-label="账号安全" @click="router.push('/account/security')"><KeyRound :size="18" /></button>
          <button type="button" class="icon-button" title="退出登录" aria-label="退出登录" @click="logout"><LogOut :size="18" /></button>
        </div>
      </header>
      <div class="page-content"><slot /></div>
    </main>
  </div>
</template>
