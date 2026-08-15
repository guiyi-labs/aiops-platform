# M115-1o：remediation List/ListOperations/RolloutHistory/validResourceName/validContainerImage 分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十五片：remediation 包 0% 函数清零 + 错误分支覆盖。

## Context

remediation/service.go 的 List（0%）、ListOperations（0%）以及 RolloutHistory/
RolloutStatus 的校验分支（66.7%）、validResourceName/validContainerImage 边界（66.7%）
均未测试。

## What Changed

`internal/remediation/service_test.go` 新增：

- `TestListValidatesDiagnosisExists`（诊断不存在返回 err，存在返回 plans）。
- `TestListOperationsValidation`（cluster=0、namespace 超长、非法 kind → ErrInvalidOperation；合法返回 plans）。
- `TestRolloutHistoryRejectsInvalidInput`（cluster=0、空 ns → ErrInvalidOperation；kube
  错误传播）。
- `TestValidResourceNameAndContainerImageEdges`（validResourceName 边界 + validContainerImage 空/513 超长）。

## Verification

- `go test ./internal/remediation/`：全绿。
- List/ListOperations 0%→100%；validContainerImage/validResourceName 边界分支补齐。

## Risks / Notes

- NewService 签名为 (diagnoses, kubernetes, repository)；前序测试误写顺序。
- validContainerImage 上限为 512（非 400）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
