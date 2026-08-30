# fleet-diagnoses-view：舰队诊断前端视图

- Date: 2026-08-30
- Status: Complete
- Scope: P2d 前端交付——新增「舰队诊断」页面，展示跨集群聚合诊断记录与统计

## Context

P2d 后端 `GET /api/v1/federation/diagnoses` + `/diagnoses/stats` 端点已在 `da04df1` 完成，
但前端缺少对应的可视化入口。本次补建「舰队诊断」页面，让用户在控制台侧栏直接查看
全舰队诊断记录（表格 + 统计 + 深链到单诊断详情），完成 P2d 四方向的最后一块。

## What Changed

### frontend/src/views/FleetDiagnosesView.vue（新建）
- 标题区：`Globe` 图标 + "舰队诊断" + 右侧汇总数字 `total / open / resolved`
  （从 `GET /api/v1/federation/diagnoses/stats` 读取）。
- 过滤区：status 下拉（open/confirmed/resolved/dismissed/all）、severity 下拉
  （critical/high/medium/low/info/all）、limit 输入（1–200，默认 50）。
- 表格：集群名（`cluster_name`，兜底 `集群 #id`）| 规则 `rule_id` | 严重度（`severity-badge`）
  | 资源 `resource_kind/resource_name`（含 namespace 小字）| 状态 `workflow-status`
  | 观测时间（相对时间：刚刚/分钟前/小时前/天前，>7天回落短日期）。
- 点击行 → `router.push(/diagnoses/:id)`；空状态 "当前舰队无诊断记录"。
- `fetch` 直接调用（带 `Authorization: Bearer`），不引入额外 API 层。
- 样式走 `console-theme.css` 变量，表格 `compact-table`/`fleet-table`；小屏 ≤720px 卡片式
  `data-label` 布局无水平滚动；`prefers-reduced-motion` 下行无动画。

### frontend/src/router/index.ts
- 新增路由 `{ path: '/fleet-diagnoses', name: 'fleet-diagnoses',
  component: () => import('../views/FleetDiagnosesView.vue'), meta: { requiresAuth: true } }`。

### frontend/src/components/ConsoleLayout.vue
- 侧栏「分析与治理」分组新增 `{ label: '舰队诊断', icon: Globe, route: '/fleet-diagnoses' }`
  （顶部引入 `Globe` from `lucide-vue-next`）。

## Verification

- `pnpm typecheck`（`vue-tsc -b`）零错误。
- `pnpm build` 成功（1818 modules），产物含 `FleetDiagnosesView-BURUAflT.css` + `FleetDiagnosesView-T8BC1vV6.js`。

## Risks / Notes

- 依赖后端 P2d 端点 `GET /federation/diagnoses` + `/diagnoses/stats`，未认证时返回 401。
- 与现有 `DiagnosesView.vue`（单集群诊断列表）互补：Fleet 视图跨集群聚合，Diagnoses 视图
  针对单集群详情。
