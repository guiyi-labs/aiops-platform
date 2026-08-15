# M115-1z：automation handler get/cancel/list 成功路径 + list 校验

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十六片：getPlan/cancelPlan/listPlans 成功路径与
  listPlans 查询参数校验分支。

## Context

httpserver/automation.go 的 getPlan（60%）、cancelPlan（56.2%）、listPlans
（66.7%）成功路径与 listPlans 校验分支未测。

## What Changed

`internal/httpserver/automation_test.go` 新增：

- `TestAutomationHandler_GetPlanSuccess`（approveRepoStub 复用 → 200 含 approved/prod）。
- `TestAutomationHandler_CancelSuccess`（cancelRepoStub → 200 含 cancelled）。
- `TestAutomationHandler_ListPlansValidation`（case_id=abc/0、cluster_id=-1、
  limit=0/201 全 400）。
- `TestAutomationHandler_ListPlansSuccess`（listPlansRepoStub → 200 total 2）。
- 新增 `cancelRepoStub`、`listPlansRepoStub`。

## Verification

- `go test ./internal/httpserver/`：全绿。
- getPlan/cancelPlan/listPlans 成功路径 + listPlans 校验分支补齐。

## Risks / Notes

- ActionPlanResponse 是 camelCase JSON（`"total":2`、`"status":"approved"`）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
