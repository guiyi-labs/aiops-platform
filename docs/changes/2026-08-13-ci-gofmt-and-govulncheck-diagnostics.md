# CI 修复：gofmt 门禁 + govulncheck 失败输出可诊断化

- Date: 2026-08-13
- Status: Complete
- Scope: 修复推送后远端 CI 两项失败（gofmt / govulncheck），使主分支恢复绿色

## Context

2026-08-12 推送 28 个提交后，远端 CI（run 31612640705）两项失败：
- `Backend / Check formatting`：`gofmt -l` 报 3 个未格式化文件。
- `Dependency & supply chain / Dependency vulnerability scan`：`govulncheck`
  exit 3（发现可达漏洞），但脚本 `set -e` + 命令替换吞掉了输出，CI 日志无任何
  漏洞详情，无法定位。

## What Changed

- `backend/cmd/demo-kube-mock/fixtures.go`、`backend/cmd/demo-kube-mock/handler.go`、
  `backend/internal/httpserver/replay_test.go`：`gofmt -w` 格式化。
- `scripts/dependency-vuln-scan.sh`：govulncheck 运行改为 `set +e` 捕获退出码并
  **总是打印完整输出**；非零退出或命中 "affected by N vulnerabilities" 均判失败。
  消除失败信息被吞的问题，后续门禁失败在 CI 日志可直接看到漏洞清单。

## Verification

- `gofmt -l .` 无输出（本地）。
- `bash -n scripts/dependency-vuln-scan.sh` 通过；本地 `govulncheck -mode=source`
  （go1.26.5 + linux/amd64 目标）均为 0 affected（1 个 module-only：GO-2026-5932）。
- 推送后等待 CI 结果；govulncheck 失败步骤将输出真实漏洞清单以定位根因。
