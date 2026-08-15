# M115-1v：config capability/signal/alert-route env 分支

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十二片：config.Load 子加载器 60-80% 分支补齐。

## Context

loadCapabilityConfig（76.9%）、loadSignalConfig（66.7%）、loadAlertRouteConfig
（72.7%）的大量 env 校验/解析错误分支未测。

## What Changed

`internal/config/config_test.go` 新增：

- `TestLoadCapabilityConfigBranches`（缺 endpoint、dev http 合法、prod 强制 https、
  userinfo 非法 URL、bad bool、bad duration）。
- `TestLoadSignalConfigBranches`（合法 + bad bool + bad duration）。
- `TestLoadAlertRouteConfigBranches`（合法 + bad bool + bad poll interval）。

## Verification

- `go test ./internal/config/`：全绿。
- CapabilityConfig.validate 的 https-in-production/userinfo 分支、三个
  loader 的解析错误分支全覆盖。

## Risks / Notes

- t.Setenv 自动恢复环境变量；各 loader 都被 Load() 间接调用但此处直接测。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
