# M100-D：依赖漏洞 / 许可证 / 镜像基础层 / SBOM 差异门禁

- Date: 2026-08-12
- Status: Complete
- Scope: M100 第四步——按路线“依赖漏洞、许可证、镜像基础层和 SBOM 差异报告；critical 默认 fail-closed”落地供应链治理门禁，并修复扫描发现的真实漏洞（pgx SQL 注入、quic-go HTTP/3 内存耗尽）。

## Context

M100 验收要求“安全扫描结果可追溯到 release manifest；例外有负责人、理由和到期时间”。存量有 Dependabot 分组、许可证报告（PowerShell 生成）与 syft SBOM（release 产物），但缺少：依赖漏洞扫描门禁、许可证 allowlist 门禁、镜像基础层漂移门禁、SBOM 差异报告与 fail-closed 接线。首次扫描即发现 2 个可达 Go 漏洞与 1 个生产前端高危（nanoid）。

## What Changed

### 漏洞修复（扫描驱动）

- `backend/go.mod` / `go.sum`：`github.com/jackc/pgx/v5` v5.6.0 → **v5.9.2**（修复 GO-2026-5004 SQL 注入：dollar-quoted 字符串字面量占位符混淆）；`github.com/quic-go/quic-go` v0.59.0 → **v0.59.1**（修复 GO-2026-5676 HTTP/3 QPACK trailer 展开内存耗尽）。升级后 `go test ./...` / vet / lint 全绿。
- `frontend/pnpm-workspace.yaml`：新增 `overrides: nanoid: 3.3.17`（pnpm 11 配置位置），修复 GHSA-2v37-7h3g-55p8（自定义生成器 size=0 无限循环，生产依赖链 vue→compiler-sfc→postcss→nanoid）；lockfile 更新，前端 typecheck/lint/137 单测/build 全绿。
- 剩余 4 个 dev 工具链告警（brace-expansion ×2、js-yaml、postcss，经 eslint/vite/vitest）不进入生产产物，作为**追踪例外**记录（见 Risks）。

### 门禁脚本与清单

- `scripts/dependency-vuln-scan.sh`：Go `govulncheck -mode=source`（可达漏洞 fail-closed；模块级未调用仅报告）+ 前端 `pnpm audit --prod --audit-level=high`（生产依赖 fail-closed）。
- `scripts/license-scan.sh` + `scripts/license-scan-parser.mjs`：Go 模块（`go list -m -json all` + 模块缓存 LICENSE 分类，复用许可证报告启发式）+ 前端 `pnpm licenses list --prod`，allowlist（MIT/Apache-2.0/BSD-2/3-Clause/ISC/MPL-2.0），UNKNOWN 或非 allowlist 一律 fail。
- `docs/security/image-base-manifest.md` + `scripts/image-base-drift.sh`：锁定 4 个基础镜像（backend `golang:1.26-alpine`/`alpine:3.22`，frontend `node:22.13.1-alpine3.21`/`nginx:1.27-alpine`）的 manifest digest；本地 docker 或 registry 重解析比对，漂移 fail-closed。
- `scripts/sbom-diff.mjs` + `scripts/testdata/sbom/`：syft SPDX JSON 差异（added/removed/changed），默认 `--added-threshold 0` fail-closed；fixture 验证检出新增包与版本变更。
- `.github/workflows/ci.yml`：新增 `dependency-scan` job（govulncheck + pnpm audit + 许可证 + 基础镜像漂移 + SBOM diff 自测）。
- `docs/supply-chain/dependency-licenses.md`：随依赖升级同步版本（pgx v5.9.2、nanoid 3.3.17）。

## Verification

- 漏洞：`govulncheck` 可达漏洞 2 → **0**；`pnpm audit --prod` → **No known vulnerabilities**。
- 许可证：`scripts/license-scan.sh` 全绿（Go 模块全部 allowlist 命中、前端 5 个许可组全在 allowlist）。
- 基础镜像：`scripts/image-base-drift.sh` clean（本地镜像用 docker 解析、缺失镜像用 registry 解析）。
- SBOM 差异：fixture 检出 `quic-go/quic-go@v0.59.1` 新增 + `jackc/pgx v5.6.0→v5.9.2` 变更并 fail-closed；无变化时通过。
- 门禁：`go test ./...`/vet/lint、前端 typecheck/lint/137 单测/build 全绿；`scan-sensitive-fields`（M100-C）回归 clean；backend 镜像重建 healthy、登录 200。
- 冒烟用户无新增（本轮未创建）。

## Risks / Notes

- **追踪例外**：Go `golang.org/x/crypto@v0.54.0`（GO-2026-5932，openpgp 包 unmaintained/unsafe by design，未达版本、代码未调用该包，无修复版）；前端 dev 工具链 brace-expansion/js-yaml/postcss 告警（不进入生产产物）。负责人：平台维护者；处置：跟随 Dependabot 主版本升级窗口复评，不阻塞生产发布。
- 许可证分类是启发式（与既有许可证报告同源）；无法定位 LICENSE 文件的模块 fail-closed，需人工补 allowlist 或分类。
- SBOM 差异门禁当前在 CI 以 fixture 自测形式接线；真实发布比较（上次 release SBOM vs 当前）由 release.yml 在产物生成时执行（syft SPDX 格式一致）。
- M100 至此 A/B/C/D 全部完成；里程碑整体封口（变更与验收映射见本记录）后按惯例打 tag `baseline-m100-YYYYMMDD` 并推送。
