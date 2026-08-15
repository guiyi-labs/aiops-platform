# M115-1r：automation approve 处理器成功路径 + 状态分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十八片：automation approvePlan（27.8% → 80%+）。

## Context

automationHandler.approvePlan 仅测试了非法 UUID（400）；成功路径与
ErrNotPreviewed 分支（409）完全未覆盖。

## What Changed

`internal/httpserver/automation_test.go` 新增：

- `approveRepoStub`（内嵌 NopRepository + plan map + Approve 状态转移）。
- `TestAutomationHandler_ApproveSuccess`（single 审批，approved 200，含
  ClusterID>0 audit 分支）。
- `TestAutomationHandler_ApproveNotPreviewed`（draft → 409）。
- `int32Ptr` helper。

## Verification

- `go test ./internal/httpserver/`：全绿。
- approvePlan 成功/未预览分支补齐。

## Risks / Notes

- 审批成功需 RequestedByUserID != ApproverID（single 模式无四眼限制，
  2048 仍走一样路径）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
