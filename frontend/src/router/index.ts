import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import type { PlatformRole } from '../types/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
    { path: '/workloads', name: 'workloads', component: () => import('../views/WorkloadsView.vue') },
    { path: '/clusters/:clusterId/resources/:kind/:namespace/:name', name: 'resource-detail', component: () => import('../views/ResourceDetailView.vue') },
    { path: '/diagnoses', name: 'diagnoses', component: () => import('../views/DiagnosisWorkspaceView.vue') },
    { path: '/cases', name: 'cases', component: () => import('../views/CasesView.vue') },
    { path: '/settings', name: 'settings', component: () => import('../views/SettingsView.vue') },
    { path: '/settings/clusters', name: 'settings-clusters', component: () => import('../views/SettingsView.vue'), meta: { tab: 'clusters' } },
    { path: '/settings/model', name: 'settings-model', component: () => import('../views/SettingsView.vue'), meta: { tab: 'model', roles: ['system_admin'] } },
    { path: '/settings/users', name: 'settings-users', component: () => import('../views/SettingsView.vue'), meta: { tab: 'users', roles: ['system_admin'] } },
    { path: '/settings/audit', name: 'settings-audit', component: () => import('../views/SettingsView.vue'), meta: { tab: 'audit', roles: ['system_admin', 'security_auditor'] } },
    { path: '/settings/tools', name: 'settings-tools', component: () => import('../views/SettingsView.vue'), meta: { tab: 'tools' } },
    { path: '/settings/account', name: 'settings-account', component: () => import('../views/SettingsView.vue'), meta: { tab: 'account' } },
    // Preserve existing links only for features merged into a retained work area.
    { path: '/clusters', redirect: '/settings/clusters' },
    { path: '/users', redirect: '/settings/users' },
    { path: '/audit-logs', redirect: '/settings/audit' },
    { path: '/account/security', redirect: '/settings/account' },
    { path: '/topology', redirect: to => ({ path: '/diagnoses', query: { ...to.query, view: 'topology' } }) },
    { path: '/aiops/correlation', redirect: to => ({ path: '/diagnoses', query: { ...to.query, view: 'correlation' } }) },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('../views/NotFoundView.vue') },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  // Only recover from cookie on the very first navigation after page load.
  // After that, auth state is fully in-memory; probing the network on every
  // sidebar click can block navigation and — on failure — clobber the fresh
  // login session (reported as "sidebar doesn't navigate after first login").
  if (!auth.initialized) {
    await auth.restore()
  }
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
