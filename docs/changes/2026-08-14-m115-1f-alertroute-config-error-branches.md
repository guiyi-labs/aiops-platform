# M115-1f：alertroute 配置函数与错误分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第六片：补齐 alertroute 配置 API（WithCipher/ConfigureDelivery）
  与 CRUD 错误分支的 0%/低覆盖率函数，消除 service.go 三个 0% 函数。

## Context

alertroute service.go 有 3 个 0% 函数（WithCipher/ConfigureDelivery）和多个
60-80% 错误分支；仓库已有完善 mockRepository 基建（54 测试），补分支成本极低。

## What Changed

`backend/internal/alertroute/service_test.go` 新增：

- `TestWithCipher`：设置 cipher 并返回自身。
- `TestConfigureDeliveryAppliesPositiveValues` / `TestConfigureDeliveryIgnoresZeroValues`。
- `TestDeleteReceiverPropagatesRepoError`、`TestDeleteRoutePropagatesRepoError`。
- `TestUpdateRouteRejectsPriorityAboveRange` / `TestUpdateRouteRejectsPriorityBelowRange` /
  `TestUpdateRouteRejectsInvalidGroupInterval` / `TestUpdateRouteRejectsInvalidRepeatInterval` /
  `TestUpdateRoutePropagatesRepoError`。
- `TestCreateReceiverMaskingSecret`：明文 secret 不得落库。

## Verification

- `go test ./internal/alertroute/`：全绿。
- service.go 三个 0% 函数清零；`deleteReceiver`/`updateRoute`/`deleteRoute`
  错误分支达 100%。

## Risks / Notes

- SecretCipher 接口签名是 Encrypt([]byte) / Decrypt([]byte, string)；测试 stub
  镜像该签名，cipher 具体实现（AES-GCM）已有独立测试。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
