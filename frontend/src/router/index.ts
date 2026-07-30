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
