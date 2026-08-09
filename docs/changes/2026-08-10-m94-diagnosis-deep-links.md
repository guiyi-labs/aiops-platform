# M94（第三步）：诊断深链（资源 / 工作台 / 相关事件 / 审计）

- Date: 2026-08-10
- Status: Complete（M94 第三步；回放模式仍为后续增量）
- Scope: 诊断详情抽屉新增“关联入口”，从诊断直达到资源详情、工作负载与相关事件、审计记录。

## Context

M94 的第一步（根因卡 + 证据时间线）与第二步（行动区）已交付。本步完成 M94 范围的“深链”：
从诊断详情直接导航到相关上下文，减少确认根因前的跳转成本。深链仅做只读导航，不新增任何
写路径或任意命令。

## What Changed

### 前端（frontend/src/views/DiagnosesView.vue）

- 新增“关联入口”区块（aria-label=`关联入口`）：
  - **资源详情**：对 ResourceDetail 视图支持的类型（Pod / Service / Node / Deployment /
    Ingress / PersistentVolumeClaim）跳转 `/clusters/:id/resources/:kind/:ns/:name`
    （Node 用 `_` 占位 namespace）；
  - **工作负载与相关事件**：跳转 `/workloads?cluster&kind&namespace&name`，由 WorkloadsView 的
    查询路由加载资源详情与相关事件；
  - **审计记录**：`security_auditor` / `system_admin` 可见，跳转 `/audit-logs`。
- 新增 `workloadsKindMap`、`resourceDetailKinds`、`resourceDetailHref`、`workloadsHref`、
  `canViewAudit`、`openDeepLink` 辅助函数。
- `frontend/src/styles/base.css`：`.deep-links` 区块、按钮行与提示样式。

### 浏览器旅程

- `frontend/e2e/diagnosis-timeline.spec.ts`：新增 2 条深链旅程（资源详情 URL 断言、
  工作负载查询 URL 断言），Desktop + Mobile 各 2 条；总诊断旅程 8/8，浏览器回归 50/50。

## Verification

- `pnpm typecheck`、`pnpm lint`：通过。
- `npx playwright test`：50/50 通过（原 46/46 + 新增 4/4 深链旅程，Desktop+Mobile）。
- `git diff --check`：通过。
- 后端无改动（纯前端只读导航）；此前 `go test ./internal/diagnosis/` 等仍通过。

## Risks / Notes

- 资源详情深链仅在 ResourceDetail 视图支持的类型上出现（排除 HorizontalPodAutoscaler），
  其余诊断仍提供工作负载/相关事件入口，保证链路真实可用而非 404。
- 事件深链复用资源详情抽屉中的既有相关事件能力，避免新增无查询过滤的弱链接。
- M94 剩余增量：回放模式（按事件时间重放 M81 insight 链路），将在新的 change-record 中归档。