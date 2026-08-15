# M115-1x：alertroute 接收器/路由上限 + 解密错误分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十四片：CreateReceiver/CreateRoute 上限分支、
  ListReceivers 解密错误。

## Context

alertroute/service.go 的 MaxReceiversPerUser/MaxRoutesPerUser 上限分支与
ListReceivers 的 decrypt 错误路径未测。

## What Changed

`internal/alertroute/service_test.go` 新增：

- `TestCreateReceiverLimit`（20 个后 → ErrReceiverLimit）。
- `TestListReceiversDecryptError`（篡改 enc: 信封 → 解密错误）。
- `TestCreateRouteLimit`（50 个后 → ErrRouteLimit）。

## Verification

- `go test ./internal/alertroute/`：全绿。
- CreateReceiver/CreateRoute 上限分支、ListReceivers 解密错误分支补齐。

## Risks / Notes

- validateRoute 要求 DedupeKey 非空；Route 字面量须携带 DedupeKey。
- decryptValue 的 SplitN 信封检查对单段 "enc:broken" 直接报错（不依赖 cipher）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
