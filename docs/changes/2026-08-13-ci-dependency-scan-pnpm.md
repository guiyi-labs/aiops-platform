# CI：Dependency 扫描作业补全 pnpm，修复 `pnpm not found` 失败

- Date: 2026-08-13
- Status: Complete
- Scope: CI `dependency-scan`（Dependency & supply chain）作业可正常执行前端 `pnpm audit --prod`

## Context

M100-D（`scripts/dependency-vuln-scan.sh` + `dependency-scan` CI 作业）新增前端
`pnpm audit --prod` 门禁，但该作业只 `setup-go` 而未安装 pnpm/Node，导致每次运行时
脚本命中 `command -v pnpm` 失败并输出 `pnpm not found`，作业必然失败。

此前该缺陷被掩盖：gofmt 与 govulncheck 主分支失败先行暴露、之后的 runtime 作业走了
docs-only 跳过路径，`dependency-scan` 从未在完整 runtime 下真正通过。本次 gofmt 与
go.mod toolchain（1.26.5）修复后，`dependency-scan` 成为首个暴露此回归的作业。

## What Changed

### .github/workflows/ci.yml
- `dependency-scan` 作业新增两步：`pnpm/action-setup`（复用顶层 `PNPM_VERSION`）
  与 `actions/setup-node`（复用 `NODE_VERSION`，含 `frontend/pnpm-lock.yaml` 缓存），
  与 `frontend` 作业配置保持一致。`pnpm audit` 仅读取 lockfile，无需 `pnpm install`，
  避免在依赖扫描作业重复安装 node_modules。

## Verification

- 本地 `bash -n .github/workflows/ci.yml` YAML 语法校验。
- 触发 `fix(ci)` push 后等待 GitHub Actions `dependency-scan` 作业通过（govulncheck 0 affected、
  `pnpm audit --prod` 使用真实 lockfile）。

## Risks / Notes

- `pnpm audit` 依赖 lockfile 与允许的 registry 可达；若 registry 不可达会失败为 gate，
  属预期的 fail-closed 行为。
