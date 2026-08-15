# M115-1k：kubernetes 分页/补丁解码/指标路径分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十一片：kubernetes 列表分页、patch 解码错误、metrics
  路径与容器归一化等低覆盖率分支；包覆盖率 72.5% → 73.1%。

## Context

kubernetes/service.go 大部分函数已 100%；剩余 60-87% 的是分页适配（
filterAndPage/countNamed/remaining）与 patch 解码错误分支。

## What Changed

`backend/internal/kubernetes/rollout_test.go` 新增：

- `TestNamespacesFilterAndPaginate`：Name 过滤 + Page/Limit 分页 + Remaining。
- `TestNodesPageResponseBranches`、`TestNodeMetricsQueryPath`、
  `TestPodMetricsNormalizesContainersAndNamespacePath`（namespace 路径 +
  containers nil → 空切片）。
- `TestPatchDeploymentDecodeError` / `TestPatchCronJobDecodeError` /
  `TestPatchNodeDecodeError`（网关返回非法 JSON → decode 错误）。
- `TestPatchDeploymentGatewaysError`（disabled cluster + 网关错误）。

## Verification

- `go test ./internal/kubernetes/`：全绿。
- 包覆盖率 72.5% → 73.1%。

## Risks / Notes

- apiquery.ListQuery 的 Limit 默认 0 → filterAndPage 返回空切片；测试必须
  显式给 Page/Limit。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
