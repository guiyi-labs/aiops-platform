# M115-1u：optimization finops/no-inputs/rate/auto-collect 分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十一片：finopsAnalyze（42.1%→高）、cis/deprecatedAPI 缺输入分支。

## Context

优化 handler 的 no-inputs 400、finops rate override、deprecated-API cluster 校验
分支未测。

## What Changed

`internal/httpserver/optimization_test.go` 新增：

- `TestOptimizationFinOpsNoInputsNoCollector400`（无 collector → 400）。
- `TestOptimizationFinOpsRateOverride`（显式 rate 覆盖 DefaultCostRate）。
- `TestOptimizationCISNoInputsNoCollector400`。
- `TestOptimizationDeprecatedAPIRequiresCluster`（缺 cluster_id → 400）。

## Verification

- `go test ./internal/httpserver/`：全绿。
- finopsAnalyze/相关分支补齐。

## Risks / Notes

- 无。覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
