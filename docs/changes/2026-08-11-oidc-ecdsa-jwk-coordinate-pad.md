# Fix Flaky EC JWK Coordinate Padding in OIDC Test

- Date: 2026-08-11
- Status: Complete
- Scope: 修复 `backend/internal/oidc` 的 `TestJWKECPublicKey` 因 P-256 坐标零填充不足而导致的间歇性 CI 失败。

## Context

CI run `31407745388`（推送 `a47c8de`）的 Backend job 在
`TestJWKECPublicKey` 失败：

```
keyset_test.go:82: publicKey error = oidc: unsupported or malformed signing key
```

根因：测试辅助函数 `ecJWK` 通过 `key.X.Bytes()` 编码 P-256 坐标，该方法
会去掉前导零字节。约 0.8% 的随机生成密钥的 X 或 Y 坐标包含前导零，导致
base64 编码后长度 < 32 字节，被生产代码 `ecdsaPublicKey` 的长度校验拒绝。
该失败与本次释放身份修复无关，但阻塞 CI gate。

## What Changed

- `backend/internal/oidc/keyset_test.go`：新增 `fixedWidthBytes` 辅助函数，
  将 `ecJWK` 中的 X、Y 坐标填充到固定 32 字节（P-256 域大小），与
  `ecdsaPublicKey` 的严格长度校验一致，并符合 RFC 7518 §6.2.1.2。

## Verification

- `go test -count=100 -run TestJWKECPublicKey ./internal/oidc/`（backend 模块）：100 次连续通过，此前
  在相同条件下载体失败率约 0.8%/次。
- `go test -count=1 ./internal/oidc/`：全包通过。
- `git diff --check`：无空白错误。

## Risks / Notes

- 改动仅影响测试辅助函数，生产代码 `ecdsaPublicKey` 不变，行为无回归。
- 该问题自首次引入（`ad82377`）时即存在，因 P-256 坐标前导零概率仅
  约 0.8%/次，之前恰好未被触发。
