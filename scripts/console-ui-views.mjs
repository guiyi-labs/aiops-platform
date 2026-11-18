// The same retained routes drive live accessibility and screenshot checks.
export const consoleUIViews = [
  { name: 'login', path: '/login', readyText: '进入控制台', auth: false },
  { name: 'dashboard', path: '/', readyText: '系统概览', auth: true },
  { name: 'workloads', path: '/workloads', readyText: '集群与资源', auth: true },
  { name: 'diagnoses', path: '/diagnoses', readyText: '诊断历史', auth: true },
  { name: 'correlation', path: '/diagnoses?view=correlation', readyText: '关联案例', auth: true },
  { name: 'topology', path: '/diagnoses?view=topology', readyText: '资源拓扑', auth: true },
  { name: 'cases', path: '/cases', readyText: '历史案例', auth: true },
  { name: 'clusters', path: '/settings/clusters', readyText: '集群接入', auth: true },
  { name: 'model', path: '/settings/model', readyText: '模型接入', auth: true },
  { name: 'users', path: '/settings/users', readyText: '用户管理', auth: true },
  { name: 'audit-logs', path: '/settings/audit', readyText: '审计日志', auth: true },
  { name: 'account', path: '/settings/account', readyText: '账号安全', auth: true },
]
