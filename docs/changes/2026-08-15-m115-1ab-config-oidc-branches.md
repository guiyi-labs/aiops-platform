# M115-1ab：config OIDC 加载成功与错误分支测试

- Date: 2026-08-15
- Status: Complete
- Scope: backend/internal/config — loadOIDCConfig 全路径覆盖（成功路径 + 4 条错误分支）

## Context

M115 工程质量冲刺中，`internal/config` 包的 `loadOIDCConfig` 函数存在零覆盖区域：
非 https issuer 拒绝、bool 解析失败、BreakGlass.MaxAccounts 范围校验、int 解析失败四条
错误分支均未被测试覆盖。本切片补齐这些分支，同时新增一条成功路径验证完整字段映射
（Issuer、BreakGlass、GroupToRoles）。

## What Changed

### backend/internal/config
- `config_test.go`：新增 `TestLoadOIDCConfigValid`（完整有效配置加载，验证字段映射）
  和 `TestLoadOIDCConfigBranches`（non-https issuer / bool parse / MaxAccounts 范围 /
  int parse 四条错误分支）；共 58 行。

## Verification

- `go test ./internal/config/ -v -count=1`：全绿，config 包覆盖率维持 ≥83%。
- `go test ./... -count=1 -timeout 180s`：整体套件全绿，无新增失败。
- 证据类型：mock/单测（无真实 OIDC 服务）。

## Risks / Notes

- 所有断言基于 `loadOIDCConfig` 内部校验逻辑（非 https issuer 返回 error、MaxAccounts
  仅接受 1–2），若校验规则变化需同步更新测试。
- 不涉及真实 OIDC Provider；OIDC 集成测试属于后续授权轨（M89/M90）。
