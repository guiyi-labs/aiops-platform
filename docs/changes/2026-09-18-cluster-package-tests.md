# cluster-package-tests：补齐 cluster 包 hermetic 单元测试

- Date: 2026-09-18
- Status: Complete
- Scope: 为 `internal/cluster` 包补齐与集群注册、连接探测、凭证解析相关的 hermetic 单元测试，提升包语句覆盖率。

## Context

`internal/cluster` 包承载集群注册、凭证加密存储、连接探测（Probe）与 kubeconfig 解析（含 `insecure-skip-tls-verify` 跳过）等核心路径。此前包语句覆盖率仅 47.8%，其中 `Service.Create`（注册集群）、`Service.Probe`（连接探测与条件更新）、`Service.Access`（凭证解密与禁用拦截）、`ParseKubeconfig` 的错误分支均未被单测覆盖，核心方法名虽已测但关键分支缺失。

## What Changed

### 新增测试（均 hermetic，不触达真实集群/数据库）
- `internal/cluster/service_extra_test.go`：
  - `TestServiceCreateRegistersClusterAndEncryptsCredential`：注册集群时解析 kubeconfig、加密凭证后落库，且默认 `Enabled=false`、`Status=disabled`；空名与非 kubeconfig 输入被拒。
  - `TestServiceProbeUpdatesConditionsOnSuccessAndFailure`：探测成功写入 `Ready`/各 Condition `True`；探测失败写入 `Unreachable` 且 `Reachable=False`。
  - `TestServiceAccessRejectsDisabledAndReturnsPlaintext`：禁用集群拒绝访问（`ErrDisabled`），启用后返回明文 kubeconfig。
  - `TestValidatePathRejectsBadPrefix` / `TestTypesPatchTypeMapsStrategicMerge`：覆盖 `validatePath` 与 `typesPatchType` 纯函数边界。
- `internal/cluster/kubeconfig_extra_test.go`：
  - `TestParseKubeconfigRejectsNonHTTPSAndMissingAuth`：非 HTTPS server、缺 token 与证书、非法 CA data 均被拒。
  - `TestParseKubeconfigHonorsInsecureSkipTLSVerify`：断言 `insecure-skip-tls-verify: true` 时 transport 的 `InsecureSkipVerify=true`，验证真实集群接入的 TLS 跳过路径。

## Verification

- `go test -cover ./internal/cluster/...`：包语句覆盖率 **47.8% → 57.5%**。
- `golangci-lint run ./internal/cluster/...`：0 issues。
- `go vet ./internal/cluster/...`：通过。

## Risks / Notes

- 剩余低覆盖集中在 `repository.go`（GORM 实现，需数据库）与 `credential_reencryption.go`（凭证重加密迁移，需数据库），均依赖外部存储、非 hermetic，故未纳入本次补测；其逻辑由集成测试覆盖。
- 本次仅新增测试，未改动任何生产代码逻辑。
