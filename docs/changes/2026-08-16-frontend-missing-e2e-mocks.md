# Frontend E2E 补缺失 mock：修复 M112/M113 渲染回归

- Date: 2026-08-16
- Status: Complete
- Scope: CI `Frontend Browser regression (M96 shell)` 4 个稳定失败

## Context

`af670de`（operator 覆盖率过线）后 CI 唯一失败为 Frontend 4 个测试
（`incidents.spec.ts` workflow + `finding-evidence.spec.ts` inspection，
Desktop/Mobile 双浏览器）。深查（见 docs/enhancement-frontend-analysis.md）
确认：**operator 增强零间接影响**，根因是 M112-1/M112-3/M113-3 前端新增
驾驶舱/覆盖率渲染，e2e mock 未同步补路由 → 请求落 api-fixtures 兜底空响应
（缺结构字段）→ Vue render function 抛 TypeError → 组件 DOM 更新中断 →
断言目标停留在旧状态。

## What Changed（纯测试 mock 修正，零业务逻辑变更）

- `frontend/e2e/incidents.spec.ts`：
  - 新增 `cockpitContext` fixture（合法 IncidentContextCockpit，含
    resource_context.scope/freshness/health/sla/evidence_sources 等）
  - beforeEach 注册 `GET /api/v1/incidents/7/context` 路由
  - 新增 `incidentSummary` fixture（合法 IncidentSummaryResponse，含
    next_steps/citations 数组）并注册 `GET /api/v1/incidents/7/summary` 路由
- `frontend/e2e/finding-evidence.spec.ts`：
  - 新增 `inspectionCoverage` fixture（合法 InspectionCoverageSummary，含
    by_severity/fail_closed/trend/rule_coverage）
  - beforeEach 注册 `**/api/v1/aiops/inspection/coverage**` 路由

## Verification

- `npx playwright test e2e/incidents.spec.ts e2e/finding-evidence.spec.ts`：
  **14/14 通过**（含此前失败的 4 个）
- 全量 `npx playwright test`：**76/76 通过**
- `pnpm typecheck` / `pnpm lint`（eslint）/ `pnpm test`（vitest 160）全绿

## Risks / Notes

- 仅补测试数据；api-fixtures 兜底 fail-closed 长期项另行评估。
- 这 4 个失败自 M112 起隐匿（b8d2374 的 Frontend job 为 change-scope
  skipped 假绿），本次修复让 Frontend E2E 恢复真实验证能力。
