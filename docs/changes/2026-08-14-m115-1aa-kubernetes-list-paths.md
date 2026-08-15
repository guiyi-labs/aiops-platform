# M115-1aa：kubernetes ResourceQuotas/LimitRanges/Secrets/Services 双向 path

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十七片：4 个 list 函数的 namespaced 与 cluster-wide
  path 分支补齐。

## Context

kubernetes/service.go 的 ResourceQuotas（66.7%）、LimitRanges（62.5%）、
Secrets（66.7%）、Services（75%）只测了错误分支，成功 path 分支（namespace
是否为空）未覆盖。

## What Changed

`internal/kubernetes/rollout_test.go` 新增：

- `newListTestService`（body `{"items":[]}` 的 gatewayStub）。
- `TestResourceQuotasListPaths` / `TestLimitRangesListPaths` /
  `TestSecretsListPaths` / `TestServicesListPaths`（各测 namespaced + "" 两种 path）。

## Verification

- `go test ./internal/kubernetes/`：全绿。
- 四个函数 success 路径全覆盖。

## Risks / Notes

- gatewayStub 需 body `{"items":[]}` 否则 JSON 解码失败。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
