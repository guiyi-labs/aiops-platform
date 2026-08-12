# CI 修复：go.mod 工具链升级 1.26.0 → 1.26.5（stdlib 漏洞）

- Date: 2026-08-13
- Status: Complete
- Scope: 修复 CI govulncheck 门禁——go1.26.0 标准库含 9+ 个新入库漏洞

## Context

上一提交让 govulncheck 失败输出可诊断化后，CI（run 31620096256）暴露真实根因：
CI 使用 go.mod 指令 `go 1.26.0` 安装工具链，其**标准库**存在 2026-08-12 新入库的
9+ 个漏洞（GO-2026-5856/5039/5038/5037/4986/4977/4971/4947/4946 等），路径全部可达
（tls.Conn.Handshake、x509.Certificate.Verify、net.Dialer、mail.ParseAddress、
mime.WordDecoder 等）。本地 go1.26.5 扫描 0 affected（stdlib 已修复），因此本地无法复现。
修复方式：把 go.mod 的 go 指令升级到 `1.26.5`，使 CI setup-go 与本地工具链一致。

## What Changed

- `backend/go.mod`：`go 1.26.0` → `go 1.26.5`（`go mod edit -go=1.26.5`；依赖无变化，
  go.sum 不变）。

## Verification

- `go build ./...`、`go test ./internal/diagnosis/ ./internal/httpserver/` 通过。
- 本地 `govulncheck -mode=source ./...`（go1.26.5，linux/amd64 目标）：0 affected，
  仅 1 个 module-only（GO-2026-5932 x/crypto openpgp，未被调用）。
- 推送后等待 CI：govulncheck 步骤应转绿（stdlib 漏洞随 1.26.5 修复）。
