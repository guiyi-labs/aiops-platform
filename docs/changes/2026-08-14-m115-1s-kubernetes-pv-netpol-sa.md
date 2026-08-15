# M115-1s：kubernetes PersistentVolume/NetworkPolicy/ServiceAccount 0% 清零

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十九片：kubernetes service 6 个 0% 函数清零。

## Context

kubernetes/service.go 的 PersistentVolumes/PersistentVolume/NetworkPolicies/
NetworkPolicy/ServiceAccounts/ServiceAccount 全部 0%（此前未写 list/detail 测试）。

## What Changed

`internal/kubernetes/rollout_test.go` 新增：

- `TestPersistentVolumesAndDetail`（list + path 断言）。
- `TestPersistentVolumeDetail`。
- `TestNetworkPoliciesClusterScopedAndNamespaced`（两种 path 分支）。
- `TestNetworkPolicyDetail`。
- `TestServiceAccountsClusterScopedAndNamespaced`（两种 path 分支）。
- `TestServiceAccountDetail`。

## Verification

- `go test ./internal/kubernetes/`：全绿。
- 6 个 0% 函数清零。

## Risks / Notes

- 无。覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
