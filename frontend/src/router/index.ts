import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '../stores/auth'
import type { PlatformRole } from '../types/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      name: 'dashboard',
      component: () => import('../views/DashboardView.vue'),
    },
    {
      path: '/clusters',
      name: 'clusters',
      component: () => import('../views/ClustersView.vue'),
    },
    {
      path: '/workloads',
      name: 'workloads',
      component: () => import('../views/WorkloadsView.vue'),
    },
    {
      path: '/clusters/:clusterId/resources/:kind/:namespace/:name',
      name: 'resource-detail',
      component: () => import('../views/ResourceDetailView.vue'),
    },
    {
      path: '/search',
      name: 'global-search',
      component: () => import('../views/GlobalSearchView.vue'),
    },
    {
      path: '/topology',
      name: 'topology',
      component: () => import('../views/TopologyView.vue'),
    },
    {
      path: '/events',
      name: 'events',
      component: () => import('../views/EventsView.vue'),
    },
    {
      path: '/diagnoses',
      name: 'diagnoses',
      component: () => import('../views/DiagnosesView.vue'),
    },
    {
      path: '/incidents',
      name: 'incidents',
      component: () => import('../views/IncidentsView.vue'),
    },
    {
      path: '/promotions',
      name: 'promotions',
      component: () => import('../views/PromotionView.vue'),
      meta: { roles: ['system_admin', 'operations_admin'] },
    },
    {
      path: '/workload-protection',
      name: 'workload-protection',
      component: () => import('../views/WorkloadProtectionView.vue'),
    },
    {
      path: '/alerts',
      name: 'alerts',
      component: () => import('../views/AlertsView.vue'),
    },
    {
      path: '/namespace-posture',
      name: 'namespace-posture',
      component: () => import('../views/NamespacePostureView.vue'),
    },
    // M66: 优化中心（M61-M65 只读分析器的控制台入口）
    {
      path: '/optimization',
      name: 'optimization',
      component: () => import('../views/OptimizationView.vue'),
    },
    // M80: 聚合治理态势（全部 M61-M78 分析器的统一只读总览）
    {
      path: '/posture',
      name: 'posture',
      component: () => import('../views/PostureView.vue'),
    },
    {
      path: '/node-maintenance',
      name: 'node-maintenance',
      component: () => import('../views/NodeMaintenanceView.vue'),
      meta: { roles: ['system_admin', 'operations_admin'] },
    },
    {
      path: '/restore-rehearsal',
      name: 'restore-rehearsal',
      component: () => import('../views/RestoreRehearsalView.vue'),
      meta: { roles: ['system_admin', 'operations_admin'] },
    },
    {
      path: '/audit-logs',
      name: 'audit-logs',
      component: () => import('../views/AuditLogsView.vue'),
      meta: { roles: ['system_admin', 'security_auditor'] },
    },
    {
      path: '/notifications',
      name: 'notifications',
      component: () => import('../views/NotificationDeliveriesView.vue'),
      meta: { roles: ['system_admin', 'security_auditor'] },
    },
    {
      path: '/users',
      name: 'users',
      component: () => import('../views/UsersView.vue'),
      meta: { roles: ['system_admin'] },
    },
    {
      path: '/account/security',
      name: 'account-security',
      component: () => import('../views/AccountSecurityView.vue'),
    },
    // M53: AIOps 概览 + 信号 + 拓扑
    {
      path: '/aiops/overview',
      name: 'aiops-overview',
      component: () => import('../views/AIOpsOverviewView.vue'),
    },
    // M54: SLO 仪表盘 + 关联案例
    {
      path: '/aiops/slo',
      name: 'aiops-slo',
      component: () => import('../views/SLODashboardView.vue'),
    },
    {
      path: '/aiops/correlation',
      name: 'aiops-correlation',
      component: () => import('../views/CorrelationCasesView.vue'),
    },
    // M55: AI 调查 + 自动化控制台
    {
      path: '/aiops/investigator',
      name: 'aiops-investigator',
      component: () => import('../views/AIInvestigatorView.vue'),
    },
    {
      path: '/aiops/automation',
      name: 'aiops-automation',
      component: () => import('../views/AutomationConsoleView.vue'),
      meta: { roles: ['system_admin', 'operations_admin'] },
    },
    // M56: 质量仪表盘
    {
      path: '/aiops/quality',
      name: 'aiops-quality',
      component: () => import('../views/QualityDashboardView.vue'),
    },
    // M50: 监控大盘
    {
      path: '/monitoring',
      name: 'monitoring',
      component: () => import('../views/MonitoringDashboardView.vue'),
    },
    // M51: 事件流与告警
    {
      path: '/event-stream',
      name: 'event-stream',
      component: () => import('../views/EventStreamView.vue'),
    },
    // M52: 智能巡检
    {
      path: '/inspection',
      name: 'inspection',
      component: () => import('../views/InspectionView.vue'),
    },
    // M52: 服务网格
    {
      path: '/service-mesh',
      name: 'service-mesh',
      component: () => import('../views/ServiceMeshView.vue'),
    },
    // M57: Helm 应用目录
    {
      path: '/app-catalog',
      name: 'app-catalog',
      component: () => import('../views/AppCatalogView.vue'),
    },
    // M58: GitOps 应用
    {
      path: '/gitops',
      name: 'gitops',
      component: () => import('../views/GitOpsView.vue'),
    },
    // M58: 跨集群复制
    {
      path: '/copyops',
      name: 'copyops',
      component: () => import('../views/CopyOpsView.vue'),
      meta: { roles: ['system_admin', 'operations_admin'] },
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.restore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  const requiredRoles = to.meta.roles as PlatformRole[] | undefined
  if (requiredRoles && !auth.user?.roles.some((role) => requiredRoles.includes(role))) {
    return { name: 'dashboard' }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'dashboard' }
  }
})

export default router
