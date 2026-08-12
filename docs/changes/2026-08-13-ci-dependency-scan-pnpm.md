# CI：恢复 main CI 全绿——pnpm、license-scan、ineffassign、SBOM self-test

- Date: 2026-08-13
- Status: Complete
- Scope: 恢复 main CI 全绿——`dependency-scan` 前端 `pnpm audit` 可执行；license allowlist
  对含第三方 LICENSE 子目录的模块（sonic）正确归类；oidc-provider 测试 lint 修复；
  SBOM diff self-test 正确断言 fail-closed

## Context

主框架回归链中连续暴露三个运行前被掩盖的 CI 失败（此前 gofmt/govulncheck 先行失败与
docs-only 跳过路径掩盖了后续步骤）：

1. M100-D 为 `dependency-scan` 接入前端 `pnpm audit --prod`，但该作业只 `setup-go`，
   `command -v pnpm` 失败 → 每次运行必红。
2. `license-scan.sh` 用 `find -iname 'LICENSE*'` 匹配模块目录，会把 `licenses/` 子目录
   当作 license 文件（`tr < 目录` 读取不到文本 → UNKNOWN）。CI 上 `find` 返回顺序随文件系统
   而异，`bytedance/sonic@v1.15.0` 这类带 `licenses/` 第三方子授权的模块被误判 UNKNOWN，
   实际许可证为 Apache-2.0。
3. `oidc-provider/main_test.go` 的 PKCE 缺失断言存在 ineffectual assignment（`badURL`
   被 `authorizeURL(...)` 赋值后立即覆盖），golangci-lint ineffassign 失败。
4. `dependency-scan` 的 SBOM diff self-test 用 `$(...)` 只捕获 stdout，但
   `sbom-diff.mjs` 的 `SBOM diff gate: … fail-closed` 发往 stderr；`grep -q 'fail-closed'`
   永远匹配不上，步骤必失败（M100-D 起即存在的潜在缺陷，被前三步失败掩盖）。

## What Changed

### .github/workflows/ci.yml
- `dependency-scan` 作业新增 `pnpm/action-setup` 与 `actions/setup-node`（复用
  `PNPM_VERSION`/`NODE_VERSION`，含 lockfile 缓存），与 `frontend` 作业一致；`pnpm audit`
  仅读 lockfile，不重复安装 node_modules。

### scripts/license-scan.sh
- 模块许可证发现收敛到模块根（`-maxdepth 1 -type f`），只读取模块自身的 LICENSE/COPYING，
  不再把 `licenses/` 子目录或第三方 LICENSE 文件误当模块许可证，消除环境依赖的 UNKNOWN。

### backend/cmd/oidc-provider/main_test.go
- 移除 PKCE 缺失断言中被立即覆盖的 `badURL := authorizeURL(...)` 首次赋值，保留
  显式构造的（无 code_challenge）URL，修复 ineffassign；断言语义不变。

### .github/workflows/ci.yml
- SBOM diff self-test 改用 `{ node sbom-diff.mjs … || true; } 2>&1` 同时捕获 stdout 与
  stderr，使 `fail-closed` 门线可被断言；保持 identical-inputs 调用必须 exit 0。

## Verification

- `./scripts/license-scan.sh`：`license scan: clean`（含 sonic 判为 Apache-2.0）。
- `go vet ./cmd/oidc-provider/`：通过；`go test ./cmd/oidc-provider/`：`ok`。
- SBOM diff self-test 复用修正后的命令串在本地执行：`SELF-TEST OK`。
- 触发 push 后等待 GitHub Actions 全作业通过（含 dependency、backend、backend race）。

## Risks / Notes

- license 收敛到模块根：个别依赖若真正的 LICENSE 也在根目录则不受影响；若仅存在于更深层
  才可能漏检，但本仓库当前依赖矩阵扫描 clean。
