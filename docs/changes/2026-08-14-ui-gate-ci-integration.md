# CI 门禁集成：pnpm ui:gate 一键全量校验

- Date: 2026-08-14
- Status: Complete
- Scope: `scripts/ui-gate.mjs` + `frontend/package.json` scripts 注册

## Context

Track A 四件套（CSS tokens / 截图基线 / axe 无障碍 / bundle size）各有独立门禁脚本，
此前需手动按序执行。本次集成一条命令串联全量校验，供 CI 及开发时使用。

## What Changed

- 新增 `scripts/ui-gate.mjs`：按序执行 CSS token 审计 (`--check`) → 截图基线
  `--verify` → axe 审计 → bundle gate，任一步非零即中止。
- `frontend/package.json`：新增 `"ui:gate": "node ../scripts/ui-gate.mjs"` 脚本入口。

## Verification

- `pnpm ui:gate`（在 frontend 目录下运行）：
  1. CSS tokens — `PASS: no exact-value literals outside :root/var()`
  2. Baselines — 62 条 `--verify` 全绿（login 0.004%、users 0.008%/0.035%）
  3. axe a11y — `PASS: 0 critical/serious, 0 app errors`
  4. Bundle — `entry JS gzip: 49.7kB` 门禁通过
  → **`=== UI GATE PASS: 4/4 ===`**，exit 0。

## Risks / Notes

- `ui:gate` 依赖前端已部署到 `AIOPS_BASE_URL`（默认 127.0.0.1:18080），
  CI 中需在 docker cp 之后运行。
- axe 耗时约 2.5 分钟（32 视图 × 2 视口），基线 verify 同理；
  总耗时约 4 分钟，与类型检查/构建门禁并行时不影响流水线节拍。
