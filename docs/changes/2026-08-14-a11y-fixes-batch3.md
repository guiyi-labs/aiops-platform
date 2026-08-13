# Track A 无障碍/控制台错误修复·第三批 + 全视图响应式审计（axe 32 视图收口）

- Date: 2026-08-14
- Status: Complete
- Scope: 前端样式/语义修复 + `scripts/audit-a11y-axe.mjs` 分类 + 62 基线重建

## Context

第三批 20 视图接入 axe 审计后暴露 6 类问题：2 类对比度（红色 danger、用户页灰色小字）、
3 类 select 可访问名缺失、2 类前端 400 真实缺陷（缺 cluster_id 参数）、以及
后端 feature-gate 路由 404 噪声。本轮全部闭环，axe 32 视图 × 双视口全量 PASS。

## What Changed

### 对比度修复（serious）
- `src/styles/console-theme.css`：`--status-danger #dc2626 → #b91c1c`
  （配合 `--danger-bg #fee2e2` 达 5.01:1，且与 `.status-pill.unavailable` 既有值一致；
  修复 audit-logs `.audit-result.failure` 与 investigator `.error-text`）。
- `src/styles/base.css`：用户页 4 处小字迁 token（axe 逐节点曝露）：
  `.user-toolbar span` `#839097→var(--gray-500)`、`.user-main` small `#89959a→var(--gray-500)`、
  `.user-edit-panel small` `#7e8b87→var(--gray-600)`、`.user-avatar` 底 `#4d887c→#357468`
  （白字 4.09→5.46:1）。

### select 可访问名（critical）
- `NamespacePostureView.vue` / `NodeMaintenanceView.vue` / `RestoreRehearsalView.vue`：
  `.cluster-select` 增加 `aria-label="选择集群"`（与 OptimizationView 既有写法一致）。

### 前端 400 真实缺陷（app errors）
- `AppCatalogView.vue`：`listAppCatalogPlans` 改为必须携带 `cluster_id`；`onMounted`
  先加载集群再按首个集群加载计划，无集群时置空（此前无参调用触发 400）。
- `AIInvestigatorView.vue`：`listCorrelationCases` 先加载集群再带 `cluster_id` 调用，
  无集群时置空（此前 400）。

### 审计脚本
- `scripts/audit-a11y-axe.mjs`：`classify()` 将 `responded with a status of 404` 的网络
  错误与 401 一并归为 probes（后端 feature-gate 路由在本地环境未注册属预期响应，
  不再是 app error）；400 仍算 app error，防止掩盖真实缺陷。

### 基线重建
- `docs/ui-baselines/`：re-capture 62 条，受影响视图（dashboard/investigator/login/users）
  像素更新；manifest 0 mismatch。

## Verification

- **axe 32 视图 × 2 视口**：`PASS: 0 critical/serious, 0 app errors`（404/401 归
  auth/feature probes，count 单独列出）。
- **截图基线 62 条 `--verify`**：59 IDENTICAL + 3 PASS（login diff 0.000%、
  users 0.006%/0.025% 在阈值内），exit 0。
- **375px 响应式全量审计**：31/31 视图 `overflowX=false`、无不可达交互元素。
- **门禁**：`pnpm typecheck` ✓、`vite build` ✓、`bundle-gate` ✓（entry JS gzip 49.7kB）。
- **部署**：`docker cp frontend/dist/. k8s-aiops-frontend-1:/usr/share/nginx/html/`，
  `curl 127.0.0.1:18080/login` 200（非持久，重启需重新 cp）。

## Risks / Notes

- `--status-danger` 全局变暗（`#b91c1c`）影响所有红色状态元素视觉，基线已重建；
  与 base.css 原 `--status-danger`（#c0392b，4.45 仍差 0.05）拉开安全余量。
- users 页 maxΔ=151px（0.025%）阈值内 PASS，仍属候选掩码对象。
- axe 门禁的 404-as-probe 规则依赖错误文本格式，若后端改错误文案需复核。
