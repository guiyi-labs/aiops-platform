# ci-format-typegen-fix：gofmt 格式化 + OpenAPI typegen 同步

- Date: 2026-08-30
- Status: Complete
- Scope: 修复 CI run 217 中 Backend Check formatting 与 Frontend Contract typegen sync 两个门禁失败

## Context

`da04df1`（P2d 诊断聚合 + 侧栏竞态 + DustField）push 后触发 CI run 217，两项门禁红灯：
1. Backend `Check formatting`：`gofmt -l` 报告 `service.go` 与 `federation_test.go` 格式不一致。
2. Frontend `Contract typegen sync (W10)`：P2d 新增两条路由 + 四套 schema 后 `openapi.d.ts` 未重新生成，与 `openapi.yaml` 不同步。

## What Changed

- `backend/internal/federation/service.go`：`gofmt -w` 自动对齐（空白/换行）。
- `backend/internal/httpserver/federation_test.go`：同上。
- `frontend/src/api/openapi.d.ts`：`pnpm run typegen` 重新生成，新增 `FederationDiagnosisRow` / `FederationDiagnosisList` / `FederationDiagnosisStats` / `FederationDiagnosisClusterCount` schema。

## Verification

- `gofmt -l backend/` 输出为空。
- `pnpm typecheck` 零错误。
- `pnpm run typegen` 成功，`git diff --exit-code -- src/api/openapi.d.ts` 通过。

## Risks / Notes

- 纯格式/生成文件同步，无语义变化。
