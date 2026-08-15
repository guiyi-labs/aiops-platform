# M115-1y：alertroute 服务级 ListDeliveries + IsInhibited 错误分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十五片：service.ListDeliveries 0% 清零、IsInhibited repo 错误分支。

## Context

alertroute/service.go 的服务级 ListDeliveries（0%）与 IsInhibited 的
ListEnabledInhibits 错误分支未测。

## What Changed

`internal/alertroute/service_test.go` 新增：

- `TestServiceListDeliveries`（全量 + receiver/status 过滤 + 无匹配）。
- `TestIsInhibitedListErrorReturnsFalse`（listEnabledInhibitsErr → false,nil）。
- mockRepository 新增 `listEnabledInhibitsErr` 注入字段。

## Verification

- `go test ./internal/alertroute/`：全绿。
- ListDeliveries 0% → 100%；IsInhibited 错误分支补齐。

## Risks / Notes

- 无。覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
