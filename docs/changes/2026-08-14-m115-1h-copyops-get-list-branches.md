# M115-1h：copyops Get/ListByUser/ListByCluster 分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第八片：补齐 copyops 服务查询面（Get/ListByUser/ListByCluster）
  与其校验分支；服务层 0% 函数清零。

## Context

copyops/service.go 的 Get/ListByUser/ListByCluster 此前 0% 覆盖（仓库有完善
inmemRepo 基建）。查询面是用户可见 API，补齐成本低。

## What Changed

`backend/internal/copyops/service_test.go` 新增：

- `previewK8sFake` 共享 fake（nsIdentity/nsExists/rawResource/resExists/dry-run
  createRes）。
- `TestServiceGetReturnsPlan` / `TestServiceGetRejectsEmptyID`。
- `TestServiceListByUser`（多用户筛选）/ `TestServiceListByUserRejectsInvalid`。
- `TestServiceListByCluster` / `TestServiceListByClusterRejectsInvalid`。
- `TestServiceNewServiceDefaults`。

## Verification

- `go test ./internal/copyops/`：全绿。
- copyops 服务 0% 函数清零（Get/ListByUser/ListByCluster 100%）。

## Risks / Notes

- 两个 Preview 用递增 rand 避免 plan ID 冲突覆盖（staticRand 同构 → 同 ID）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
