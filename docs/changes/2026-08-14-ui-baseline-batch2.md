# UI 基线/axe/响应式扩展·第二批（Track A）

- Date: 2026-08-14
- Status: Complete
- Scope: `scripts/capture-ui-baselines.mjs` / `scripts/audit-a11y-axe.mjs` + 基线产物

## Context

Track A 收口目标是把截图/axe/响应式三件套铺到关键视图。第一批（登录 + `/`、
`/clusters`、`/workloads`、`/diagnoses`）已落地。本改动扩展第二批 6 个高价值视图：
事件中心、资源拓扑、事故工作空间、优化中心、告警规则、AIOps 概览。

## What Changed

- `capture-ui-baselines.mjs` 与 `audit-a11y-axe.mjs` 的 `views` 各新增 6 条
  （readyText 取自线上 H1，settle 时间按页面异步加载调整，拓扑略长 1800ms）。
- 基线产物新增 12 张 PNG（`docs/ui-baselines/images/**-desktop-1440x900.png` /
  `**-mobile-375x812.png`），manifest 扩至 **22 条**。
- 375px 响应式审计第二批（含可滚动 tab 容器的可达性区分）。

## Verification

- **截图基线**：`--verify` 22/22 确定性通过（login desktop diff 0.000%，其余全部
  IDENTICAL，含新增 topology/incidents/optimization/alerts/events/aiops-overview）。
- **axe 双视口**：新增 6 页 12 组合格，critical/serious 全 0、app 错误全 0。
- **375px 响应式**：新增 6 页全部 `overflowX=false`；优化中心分析器 Tab 按钮为
  可横向滚动容器内元素（`overflow-x:auto` + Tab 设计），可达，非裁切（审计按
  "可滚动祖先"区分）。
- 构建：本轮仅改 `scripts/*.mjs` 与 PNG/manifest，未动前端源码；`--verify` 本身即
  全链路回归证据。

## Follow-up

- 剩余视图（全局搜索、监控大盘、服务网格、SLO、关联、命名空间治理、审计日志、
  用户管理、Helm/GitOps 等）按同模式追加 `views`。
- 可将二阶审计（response-batch 脚本）固化进 `scripts/`，与 `--verify` 合并为
  全量 `pnpm ui:gate`。
