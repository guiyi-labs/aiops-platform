# M115-1w：automation NopCaseReader + CreatePlan 错误分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十三片：NopCaseReader 两个 0% 方法清零 +
  CreatePlan EligibleActionCodes 错误传播 + nil-repo disabled。

## Context

automation/service.go 的 NopCaseReader.GetCase（0%）/EligibleActionCodes（0%）
未测；CreatePlan 的 EligibleActionCodes 错误分支、nil repo 分支也未覆盖。

## What Changed

`internal/automation/service_test.go` 新增：

- `TestNopCaseReaderMethods`（GetCase → ErrCaseNotFound；EligibleActionCodes → nil,nil）。
- `TestCreatePlanRejectsWhenEligibleCodesError`（codesErr 传播）。
- `TestCreatePlanDisabledWhenNilRepo`（NewService(nil,nil,nil) → ErrDisabled）。

## Verification

- `go test ./internal/automation/`：全绿。
- NopCaseReader 0% → 100%；CreatePlan 错误分支补齐。

## Risks / Notes

- 无。覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
