# 修复 CI golangci-lint：receiver name 不统一 + ineffassign

- Date: 2026-08-15
- Status: Complete
- Scope: 修复 CI `Backend golangci-lint` job 两个真实 lint 问题。

## Context

`9720585` 推送后 CI Backend golangci-lint 报两处静态检查错误：

1. `ST1016` @ `backend/internal/operator/types.go`：#`ControlledOperationSpec`
   类型方法 receiver 混用 `s`（IsDryRun）与 `in`（DeepCopyInto/DeepCopy）。
2. `ineffassign` @ `backend/internal/automation/service_test.go#1170`：
   `svc := newTestService(...)` 立即被下一行 `svc = NewService(...)` 覆盖，
   首行赋值无效果。

## What Changed

- `backend/internal/operator/types.go`：`*ControlledOperationSpec` 的
  DeepCopyInto / DeepCopy receiver 统一为 `s`（与 IsDryRun 一致）。
- `backend/internal/automation/service_test.go`：删除被立即覆盖的
  `svc := newTestService(...)` 行，保留带完整 options 的 NewService 赋值。

## Verification

- `golangci-lint v2.12.2 run --config ../.golangci.yml
  ./internal/operator/ ./internal/automation/`（与 CI 同版本同配置）：**0 issues**。
- `gofmt -l`：空。
- `go test -count=1 ./internal/operator/ ./internal/automation/`：全绿。

## Risks / Notes

- receiver 统一为 `s` 是 pure rename，无行为变化；ineffassign 删除一行
  无效赋值，逻辑等价。
