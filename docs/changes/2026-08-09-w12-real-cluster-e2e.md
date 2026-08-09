# W12: 真实集群 E2E 证据收口 + 前端构建修复

- Date: 2026-08-09
- Status: Complete（本地 Compose + kind 验证通过；产物已修复）
- Scope: 前端类型契约修复 + E2E 断言修正 + 三个真实集群 E2E 证据归档

## Context

补齐长远计划 P0 的核心证据缺口：Compose（postgres + backend + frontend）就绪后，
跑通 `e2e-diagnosis-kind.ps1`、`e2e-fleet-kind.ps1`、`e2e-global-search-kind.ps1`，
证据 JSON 存入 `.artifacts/<suite>-e2e/`。同时修复两个此前在干净 checkout 上
即失败的**前端构建缺陷**，以及一个与产品受控变更契约冲突的 E2E 断言。

## What Changed

### 修复前端构建（干净 checkout 后 `pnpm build` 失败，现已全绿）

- `frontend/src/api/openapi.ts`：`OperationResponse<Op, Status>` 默认参数
  `Status extends keyof operations[Op][`responses`] = 200` 在 TS 约束检查时会对
  全部 272 个操作的 `responses` 键求交集，而其中仅 115 个操作含 `200` 响应，
  导致交集为 `never`、默认值非法。改为 `Status extends number` 并用
  `Status & keyof operations[Op][`responses`]` 取值。
- `frontend/src/api/insight.ts`：`InsightRunbook` 由纯别名改为
`Omit<InsightRunbookContract, `operations`> & { operations: NonNullable<...> }`，
  与 `@/types/insight` 的必填形态一致（OpenAPI 中 `operations` 可选，运行时已 `?? []` 归一）。
- 验证：`pnpm typecheck` / `pnpm build` / `pnpm vitest run`（22 files · 124 tests）全绿。

### 修正 E2E RBAC 断言

- `scripts/e2e-diagnosis-kind.ps1`：原断言 `patch_nodes == no` 与产品契约冲突。
  observer 的 Node `patch`、Pod eviction `create`、Velero Backup/Restore `create`、
  隔离 Namespace/ResourceQuota/NetworkPolicy `create` 是受控变更集，由
  `backend/internal/deployment/managed_cluster_test.go` 与 README 明确定义。
  改为 `patch_nodes == yes`，保留 `patch_deployments == no`（Deployment 仅授权给
  namespace 级 remediator 角色）。

## 真实集群验收证据（kind v0.30.0 / k8s v1.34.0）

- `.artifacts/diagnosis-e2e/*.json`：Node `not_ready` + Deployment `replicas_unavailable`
  确定性诊断通过；RBAC 断言符合受控变更契约；清理全绿。
- `.artifacts/fleet-e2e/*.json`：双成员联邦注册 + probe；超时/不可用隔离到单集群、
  其余集群 degrade 不雪崩；恢复后计数还原。
- `.artifacts/search-e2e/*.json`：跨集群资源搜索 baseline、稳定排序、kind 过滤、
  per-cluster 超时隔离、query failure 隔离、恢复后 0 失败。

## Notes

- Compose 主栈在 E2E 期间保持 healthy（postgres/backend/frontend）。
- `.artifacts/` 已 .gitignore，证据仅本地归档，change-record 提供链接与摘要。
