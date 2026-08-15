# M115-1p：copyops Execute 校验 + failPlan 错误分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十六片：Execute 校验（57.1%→更高）、failPlan
  错误分支（NSIdentity/destNsExists/createRes）。

## Context

copyops/service.go Execute（66.7%）有多个 failPlan 路径未被触达；
validateExecuteRequest（57.1%）缺少非法输入路径。

## What Changed

`internal/copyops/service_test.go` 新增：

- `TestExecuteValidationBranches`（短 planID、空 token、空 idempotency）。
- `TestExecute_ClaimNotFound`（不存在 plan → ErrNotFound）。
- `TestExecute_NSIdentityError`（第二次调用 nsIdentity 返回 error → failPlan "identity read failed"）。
- `TestExecute_DestNamespaceDeleted`（nsExists 第二次返回 false → "deleted between"）。
- `TestExecute_CreateResourceError`（dryRun 成功、实际 create 失败 → StatusFailed）。

## Verification

- `go test ./internal/copyops/`：全绿。
- validateExecuteRequest/Execute 失败分支补齐；包覆盖率65.3%。

## Risks / Notes

- failPlan 仅在 appliedCount>0 时设置 LastError；全部失败时 LastError 为空
  （现有行为，非回归）。
- nsExists/nsIdentity 在 Preview 阶段也被调用，测试须用 first-call-succeeds
  模式让 Preview 通过。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
