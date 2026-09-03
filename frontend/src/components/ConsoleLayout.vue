<script setup lang="ts">
import {
  Activity,
  BarChart2,
  Bell,
  BellRing,
  Boxes,
  BoxIcon,
  Brain,
  ChevronDown,
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
  Moon,
  MessageSquareText,
  Network,
  Package,
  PanelLeftClose,
  PanelLeftOpen,
  Pin,
  PinOff,
  Radar,
  Search,
  Send,
  ShieldCheck,
  Sun,
  Star,
  Stethoscope,
  Shuffle,
  Target,
  Users,
  Wallet,
  Workflow,
  X,
} from 'lucide-vue-next'
import { computed, inject, provide, ref, watch, nextTick, onMounted, onUnmounted, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '../stores/auth'
import { useTheme } from '../composables/useTheme'
import AppToast from './AppToast.vue'

const props = defineProps<{ eyebrow?: string; title?: string; shell?: boolean }>()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { theme, toggle: toggleTheme } = useTheme()

// Resolve the routed component directly from the matched route. route.matched
// is proven-reactive (the sidebar .active highlight and the topbar title both
// track it), so rendering it via <component :is> makes the console view swap
// on every client-side navigation — without depending on <RouterView>'s
// scoped-slot prop, which is what previously froze the view after login.
const resolvedComponent = computed<Component | undefined>(() => {
  const matched = route.matched
  const last = matched[matched.length - 1]
  return (last?.components?.default) as Component | undefined
})

const shellContextKey = Symbol.for('aiops.console-shell')
type ShellContext = { eyebrow: typeof shellEyebrow; title: typeof shellTitle }
const shellEyebrow = ref(props.eyebrow ?? '')
const shellTitle = ref(props.title ?? '')
const parentShellContext = inject<ShellContext | null>(shellContextKey, null)
if (props.shell) {
  provide(shellContextKey, { eyebrow: shellEyebrow, title: shellTitle })
} else if (parentShellContext) {
  watch(() => [props.eyebrow, props.title] as const, ([eyebrow, title]) => {
    parentShellContext.eyebrow.value = eyebrow ?? ''
    parentShellContext.title.value = title ?? ''
  }, { immediate: true })
}

// ─── Sidebar collapse state ───
const sidebarCollapsed = ref(window.localStorage.getItem('aiops.sidebar.collapsed') === '1')

watch(sidebarCollapsed, (collapsed) => {
  window.localStorage.setItem('aiops.sidebar.collapsed', collapsed ? '1' : '0')
})

// ─── Accordion state (which groups are expanded) ───
const ACCORDION_KEY = 'aiops.sidebar.accordion'
function loadAccordionState(): Record<string, boolean> {
  try {
    return JSON.parse(window.localStorage.getItem(ACCORDION_KEY) || '{}')
  } catch { return {} }
}
const accordionState = ref<Record<string, boolean>>(loadAccordionState())
// All groups default to expanded; only explicitly collapsed ones are false
function isGroupExpanded(label: string): boolean {
  return accordionState.value[label] !== false
}
function toggleGroup(label: string) {
  accordionState.value = { ...accordionState.value, [label]: !isGroupExpanded(label) }
  window.localStorage.setItem(ACCORDION_KEY, JSON.stringify(accordionState.value))
}

// ─── Pinned / favorites ───
const PINS_KEY = 'aiops.sidebar.pins'
function loadPins(): string[] {
  try {
    return JSON.parse(window.localStorage.getItem(PINS_KEY) || '[]')
  } catch { return [] }
}
const pinnedRoutes = ref<string[]>(loadPins())

watch(pinnedRoutes, (pins) => {
  window.localStorage.setItem(PINS_KEY, JSON.stringify(pins))
}, { deep: true })

function isPinned(routePath: string): boolean {
  return pinnedRoutes.value.includes(routePath)
}
function togglePin(routePath: string) {
  if (isPinned(routePath)) {
    pinnedRoutes.value = pinnedRoutes.value.filter(r => r !== routePath)
  } else {
    pinnedRoutes.value = [...pinnedRoutes.value, routePath]
  }
}

// ─── Navigation groups ───
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
      { label: '解释覆盖率', icon: BarChart2, route: '/aiops/ai-coverage' },
    ],
  },
  {
    label: '分析与治理',
    items: [
      { label: '智能诊断', icon: Stethoscope, route: '/diagnoses' },
      { label: '舰队诊断', icon: Globe, route: '/fleet-diagnoses' },
      { label: '事故工作空间', icon: MessageSquareText, route: '/incidents' },
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

// Build a flat list for command palette search
const allNavItems = computed(() => {
  const items: { label: string; icon: Component; route: string; group: string }[] = []
  for (const group of navigationGroups.value) {
    for (const item of group.items) {
      items.push({ label: item.label, icon: item.icon, route: item.route, group: group.label })
    }
  }
  return items
})

// Pinned items resolved from flat nav list
const pinnedItems = computed(() => {
  return pinnedRoutes.value
    .map(r => allNavItems.value.find(item => item.route === r))
    .filter(Boolean) as { label: string; icon: Component; route: string; group: string }[]
})

const roleLabel = computed(() => {
  const role = auth.user?.roles[0]
  return ({ system_admin: '系统管理员', operations_admin: '运维管理员', viewer: '只读观察员', security_auditor: '安全审计员' } as Record<string, string>)[role ?? ''] ?? '平台用户'
})

// ─── Collapsed hover tooltip ───
const hoveredItem = ref<{ label: string; group: string; route: string } | null>(null)
const tooltipPos = ref({ top: 0, left: 0 })
function onNavHover(e: MouseEvent, label: string, group: string, routePath: string) {
  if (!sidebarCollapsed.value) return
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  tooltipPos.value = { top: rect.top + rect.height / 2, left: rect.right + 8 }
  hoveredItem.value = { label, group, route: routePath }
}
function onNavLeave() {
  hoveredItem.value = null
}

// ─── Command palette (Ctrl+K) ───
const commandPaletteOpen = ref(false)
const commandQuery = ref('')
const commandInputRef = ref<HTMLInputElement | null>(null)

const commandResults = computed(() => {
  const q = commandQuery.value.toLowerCase().trim()
  if (!q) return allNavItems.value
  return allNavItems.value.filter(item =>
    item.label.toLowerCase().includes(q) || item.group.toLowerCase().includes(q)
  )
})

function openCommandPalette() {
  commandPaletteOpen.value = true
  commandQuery.value = ''
  nextTick(() => commandInputRef.value?.focus())
}
function closeCommandPalette() {
  commandPaletteOpen.value = false
  commandQuery.value = ''
}
function commandNavigate(path: string) {
  closeCommandPalette()
  navigate(path)
}

function onGlobalKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault()
    if (commandPaletteOpen.value) {
      closeCommandPalette()
    } else {
      openCommandPalette()
    }
  }
  if (e.key === 'Escape' && commandPaletteOpen.value) {
    closeCommandPalette()
  }
}

onMounted(() => {
  document.addEventListener('keydown', onGlobalKeydown)
})
onUnmounted(() => {
  document.removeEventListener('keydown', onGlobalKeydown)
})

async function logout() {
  await auth.logout()
  await router.push({ name: 'login' })
}

async function navigate(path: string) {
  if (route.path === path) return
  await router.push(path)
}
</script>

<template>
  <template v-if="!props.shell">
    <Teleport v-if="parentShellContext" to="#console-topbar-actions" defer>
      <slot name="actions" />
    </Teleport>
    <slot />
  </template>
  <div v-else :class="['app-shell', { 'sidebar-collapsed': sidebarCollapsed }]" data-testid="console-shell">
    <aside id="primary-sidebar" class="sidebar">
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
        <!-- Pinned / Favorites section -->
        <div v-if="pinnedItems.length > 0" class="nav-group nav-group--pinned">
          <p class="nav-group-label nav-group-label--pinned">
            <Star :size="11" />
            <span>收藏</span>
          </p>
          <button
            v-for="item in pinnedItems"
            :key="'pin-' + item.route"
            type="button"
            class="nav-item"
            :class="{ active: item.route === route.path }"
            :title="item.label"
            :aria-label="sidebarCollapsed ? item.label : undefined"
            @click="navigate(item.route)"
            @mouseenter="onNavHover($event, item.label, item.group, item.route)"
            @mouseleave="onNavLeave"
          >
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
          </button>
        </div>

        <!-- Regular nav groups with accordion -->
        <div v-for="group in navigationGroups" :key="group.label" class="nav-group">
          <button
            type="button"
            class="nav-group-header"
            :class="{ collapsed: !isGroupExpanded(group.label) }"
            :title="sidebarCollapsed ? group.label : undefined"
            :aria-label="isGroupExpanded(group.label) ? `收起${group.label}` : `展开${group.label}`"
            :aria-expanded="isGroupExpanded(group.label)"
            @click="sidebarCollapsed ? undefined : toggleGroup(group.label)"
          >
            <p class="nav-group-label">{{ group.label }}</p>
            <ChevronDown
              v-if="!sidebarCollapsed"
              :size="14"
              class="nav-group-chevron"
              :class="{ rotated: !isGroupExpanded(group.label) }"
            />
          </button>
          <div class="nav-group-items" :class="{ hidden: !isGroupExpanded(group.label) && !sidebarCollapsed }">
            <div
              v-for="item in group.items"
              :key="item.label"
              class="nav-row"
            >
              <button
                type="button"
                class="nav-item"
                :class="{ active: item.route === route.path }"
                :title="item.label"
                :aria-label="sidebarCollapsed ? item.label : undefined"
                @click="navigate(item.route)"
                @mouseenter="onNavHover($event, item.label, group.label, item.route)"
                @mouseleave="onNavLeave"
              >
                <component :is="item.icon" :size="18" />
                <span>{{ item.label }}</span>
              </button>
              <button
                v-if="!sidebarCollapsed"
                type="button"
                class="pin-toggle"
                :class="{ pinned: isPinned(item.route) }"
                :title="isPinned(item.route) ? '取消收藏' : '收藏此页面'"
                :aria-label="isPinned(item.route) ? '取消收藏' : '收藏此页面'"
                @click="togglePin(item.route)"
              >
                <PinOff v-if="isPinned(item.route)" :size="12" />
                <Pin v-else :size="12" />
              </button>
            </div>
          </div>
        </div>
      </nav>
    </aside>

    <!-- Collapsed hover tooltip -->
    <Teleport to="body">
      <div
        v-if="hoveredItem && sidebarCollapsed"
        class="sidebar-tooltip"
        :style="{ top: tooltipPos.top + 'px', left: tooltipPos.left + 'px' }"
      >
        <span class="sidebar-tooltip-group">{{ hoveredItem.group }}</span>
        <span class="sidebar-tooltip-label">{{ hoveredItem.label }}</span>
      </div>
    </Teleport>

    <!-- Command palette overlay -->
    <Teleport to="body">
      <div v-if="commandPaletteOpen" class="command-palette-backdrop" @click="closeCommandPalette">
        <div class="command-palette" @click.stop>
          <div class="command-palette-header">
            <Search :size="16" class="command-palette-icon" />
            <input
              ref="commandInputRef"
              v-model="commandQuery"
              type="text"
              class="command-palette-input"
              placeholder="搜索导航项... (Ctrl+K)"
              @keydown.escape="closeCommandPalette"
            />
            <button type="button" class="command-palette-close" @click="closeCommandPalette">
              <X :size="14" />
            </button>
          </div>
          <div class="command-palette-results">
            <button
              v-for="item in commandResults"
              :key="item.route"
              type="button"
              class="command-palette-item"
              :class="{ active: item.route === route.path }"
              @click="commandNavigate(item.route)"
            >
              <component :is="item.icon" :size="16" />
              <span class="command-palette-item-label">{{ item.label }}</span>
              <span class="command-palette-item-group">{{ item.group }}</span>
            </button>
            <p v-if="commandResults.length === 0" class="command-palette-empty">无匹配结果</p>
          </div>
        </div>
      </div>
    </Teleport>

    <main class="main-content">
      <header class="topbar">
        <div class="topbar-left">
          <button
            type="button"
            class="icon-button sidebar-toggle"
            :title="sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
            :aria-label="sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
            :aria-expanded="!sidebarCollapsed"
            aria-controls="primary-sidebar"
            @click="sidebarCollapsed = !sidebarCollapsed"
          >
            <PanelLeftOpen v-if="sidebarCollapsed" :size="18" />
            <PanelLeftClose v-else :size="18" />
          </button>
          <button
            type="button"
            class="icon-button command-palette-trigger"
            title="快速搜索 (Ctrl+K)"
            aria-label="快速搜索"
            @click="openCommandPalette"
          >
            <Search :size="16" />
          </button>
        </div>
        <div class="topbar-title"><p class="context-label">{{ shellEyebrow }}</p><h1>{{ shellTitle }}</h1></div>
        <div id="console-topbar-actions" class="topbar-actions">
          <slot name="actions" />
          <div class="topbar-user"><strong>{{ auth.user?.display_name }}</strong><span>{{ roleLabel }}</span></div>
          <span class="avatar" aria-label="当前用户">{{ auth.userInitials }}</span>
          <button type="button" class="icon-button theme-toggle" :title="theme === 'dark' ? '切换亮色主题' : '切换暗色主题'" :aria-label="theme === 'dark' ? '切换亮色主题' : '切换暗色主题'" @click="toggleTheme">
            <Moon v-if="theme === 'light'" :size="18" />
            <Sun v-else :size="18" />
          </button>
          <button type="button" class="icon-button" title="账号安全" aria-label="账号安全" @click="router.push('/account/security')"><KeyRound :size="18" /></button>
          <button type="button" class="icon-button" title="退出登录" aria-label="退出登录" @click="logout"><LogOut :size="18" /></button>
        </div>
      </header>
      <div class="page-content">
        <component :is="resolvedComponent" v-if="resolvedComponent" :key="route.fullPath" />
      </div>
    </main>
    <AppToast />
  </div>
</template>
