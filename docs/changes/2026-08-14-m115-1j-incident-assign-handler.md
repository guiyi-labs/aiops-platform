# M115-1j：incident assign/batch-assign 处理器分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十片：补齐 incident assign 处理器（此前 0%）与
  batch-assign/evidence 分支；assign 400/404/409 错误路径全覆盖。

## Context

`incidentHandler.assign`（PATCH /incidents/:incident_id/assignment）此前 0%：
测试引擎未注册该路由。仓库已有成熟 incidentRepoStub + newIncidentTestEngine。

## What Changed

`backend/internal/httpserver/incidents_test.go`：

- 测试引擎注册 `POST /incidents/:incident_id/assignment`。
- `TestIncidentHandler_AssignSuccessAndErrors`：seed + 非法 body 400 +
  成功 200（含 expected_version 命中）+ 版本冲突 409 + 缺失 404。
- `TestIncidentHandler_AssignInvalidID`（非数字 ID 400）。
- `TestIncidentHandler_BatchAssignTooMany`（超 50 上限 400）。
- `TestIncidentHandler_EvidenceForMissingIncident`（缺失 999 非 200）。

## Verification

- `go test ./internal/httpserver/`：全绿。
- incidents.go assign 0% → 100%；batchAssign 验证分支补齐。

## Risks / Notes

- `expected_version` 有 `binding:"required"`，0 值会被 gin binding 拒绝为 400；
  测试必须用 seed 记录的实际 Version。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
