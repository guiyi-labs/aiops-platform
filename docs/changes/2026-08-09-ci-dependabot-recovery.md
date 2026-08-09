# CI 故障恢复：同步 main 修复 7 条 dependabot PR 的 Typecheck 失败

- Date: 2026-08-09
- Status: Complete
- Scope: 7 条 dependabot 分支合并 `main`（W12 类型修复）并重跑 CI 全绿，未改任何依赖版本。

## Context

远端 `main` 最新 CI 全绿（commit `181da6f`），但 7 条 dependabot 升级 PR 的
Typecheck 阶段失败。失败根因一致：这些 PR 基于旧 base（`a0874e3`），未包含
W12 已落地的 `frontend/src/api/openapi.ts` / `frontend/src/api/insight.ts`
类型契约修复，导致 `InsightRunbook` 不匹配与 `OperationResponse Status` 约束错误。
属旧基线漂移，非真实依赖回归。

## What Changed

- 通过 GitHub Merges API 将 `main`（含 W12 修复）合并进以下分支并触发 CI 重跑：
  - `dependabot/npm_and_yarn/frontend/vue-router-5.2.0`（PR #9）
  - `dependabot/npm_and_yarn/frontend/types/node-26.1.2`（PR #10）
  - `dependabot/npm_and_yarn/frontend/typescript-6.0.3`（PR #11）
  - `dependabot/npm_and_yarn/frontend/vitest-4.1.10`（PR #12）
  - `dependabot/npm_and_yarn/frontend/frontend-packages-15e6b046f9`（PR #14）
  - `dependabot/go_modules/backend/go-modules-9850a54ddd`（PR #7）
  - `dependabot/github_actions/sigstore/cosign-installer-4.1.2`（PR #13）
- 未对任何依赖版本做额外改动；合并提交仅带入 `main` 已有代码。

## Verification

- 7 条分支最新 CI 运行全部 `success`（04:10–04:20 触发）。
- 全部 PR `mergeable` 且 `mergeStateStatus = CLEAN`（所需检查全部通过）。
- `main` 保持绿色（`181da6f`），无失败/取消运行残留。

## Risks / Notes

- 这些 PR 为 dependabot 自动创建，尚未合并进 `main`；CI 已绿，可安排合并并核对产物。
- 若依赖升级引入新的破坏性类型变更（如 vue-router 5 迁移），需在本地分支真修后重跑。