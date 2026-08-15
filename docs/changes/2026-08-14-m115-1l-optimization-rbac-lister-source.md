# M115-1l：optimization RBAC/kubernetesLister/NodeUsageSource 分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十二片：补齐 optimization collector 的 collectRBAC、
  NewKubernetesLister.List（0%）、NewNodeUsageSource（0%）分支。

## Context

optimization/collector.go 多数路径已覆盖；collectRBAC（65.9%）、
kubernetesLister.List 与 NodeUsageSource（各 0%）是剩余杠杆。

## What Changed

`backend/internal/optimization/collector_test.go` 新增：

- `TestCollectRBAC_PopulatesClusterAndNamespacedBindings`：cluster/namespaced
  binding 全部字段（RoleRules 解析、Subjects）。
- `TestCollectRBAC_PropagatesListFailure`。
- `TestNewKubernetesListerListBranches`：credential/gateway/decode 三错误 +
  happy path（items 解析）。
- `TestNewNodeUsageSource` / `TestNodeUsageSeries_QueryError`（errSeriesRepo）。

## Verification

- `go test ./internal/optimization/`：全绿。
- NewKubernetesLister/NewNodeUsageSource 0% → 100%；collectRBAC、NodeUsageSeries
  分支补齐；包覆盖率 82.5%。

## Risks / Notes

- metricsHistoryUsageSource.Query 对 nil svc 会 panic，需真实 Service +
  errSeriesRepo。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
