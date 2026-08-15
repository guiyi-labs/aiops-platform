# M115-1i：promotion Get/List 查询面测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第九片：promotion Service.Get/List（此前 0%）与其校验分支。

## Context

promotion/service.go 的 Get/List 与 List 的非法 clusterID 校验此前 0%；仓库有
完善 kubeStub + repoStub 基建（18 既有测试）。

## What Changed

`backend/internal/promotion/service_test.go` 新增：

- `previewForGetTest` helper（Deployment 清单 + Preview 建 plan）。
- `TestGetReturnsPlan` / `TestGetReturnsErrNotFound`。
- `TestListReturnsPlans` / `TestListRejectsInvalidClusterID`。

## Verification

- `go test ./internal/promotion/`：全绿。
- promotion Get/List 100%；全局覆盖率 ~69.0%。

## Risks / Notes

- BundleItemRequest 的 Namespace 必须等于 SourceNamespace（validateRequest），
  测试 mirror 该约束。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
