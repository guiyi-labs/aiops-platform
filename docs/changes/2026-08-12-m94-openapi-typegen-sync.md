# M94 契约同步：openapi.d.ts typegen 产物刷新

- Date: 2026-08-12
- Status: Complete
- Scope: 重新生成 OpenAPI typegen 产物，修复 M94 引入的契约不同步

## Context

M94 回放模式在 `docs/api/openapi.yaml` 新增了 3 个 schema（`DiagnosisReplayView`、
`DiagnosisReplayStage`、`DiagnosisReplayStep`）与 `GET /diagnoses/:diagnosis_id/replay`
路径，但未同步重新生成前端契约类型 `frontend/src/api/openapi.d.ts`。CI 的
`Contract typegen sync (W10)` 门禁（`pnpm typegen && git diff --exit-code -- src/api/openapi.d.ts`）
会因此失败，阻塞推送与发布。本回合补齐同步。

## What Changed

- `frontend/src/api/openapi.d.ts`：由 `pnpm typegen`（openapi-typescript 7.13.0）
  重新生成，新增 97 行——replay 路径 + 3 个 schema + `getDiagnosisReplay` operation；
  无其他意外变更。

## Verification

- `pnpm typegen && git diff --exit-code -- src/api/openapi.d.ts`：无 diff（产物与 OpenAPI 同步）。
- `pnpm typecheck`、`pnpm build`、`pnpm lint` 全绿（`src/api/openapi.ts` import 该产物）。
- `scripts/scan-sensitive-fields.sh`：clean。

## Risks / Notes

- 该产物由 `docs/api/openapi.yaml` 生成，后续 OpenAPI 变更必须同步执行 `pnpm typegen`
  并保持无 diff；CI 门禁已覆盖，本地复验一次确认闭环。
