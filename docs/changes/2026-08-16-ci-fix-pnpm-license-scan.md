# CI 修复：pnpm esbuild 构建批准 + license 扫描链（13a486d 两个 job failure）

- Date: 2026-08-16
- Status: Complete
- Scope: 中枢指令 —— 修复 13a486d 的 Dependency & supply chain + Frontend 两个
  CI job failure（Release v0.1.0 关键路径）
- DependsOn: 879ac75（P2a 质量）、13a486d

## CI 失败根因（gh run view 31933125785 实际日志）

### ① Frontend job — `pnpm install --frozen-lockfile` ERR_PNPM_IGNORED_BUILDS

879ac75 改动 frontend/pnpm-workspace.yaml 时**误伤**了 esbuild 构建批准配置：
`allowBuilds.esbuild` 被错改成占位文本 `set this to true or false` 且删除了
`onlyBuiltDependencies: [esbuild]`。pnpm v11 对 esbuild 的 postinstall
（下载/校验平台二进制）默认拦截 → install 以退出码 1 失败。

**修复**（frontend/pnpm-workspace.yaml）：恢复
`allowBuilds.esbuild: true` + `onlyBuiltDependencies: [esbuild]`（pnpm v11
双键保留，前向兼容）；保留 nanoid 3.3.18 overrides 不动。
本地 `pnpm install --frozen-lockfile` 验证：esbuild 构建正常，install 成功。

### ② Dependency & supply chain job — License allowlist scan parse failed

真实日志：`license JSON parse failed: (packages ?? []) is not iterable`。
两处缺陷：
- CI dependency job 没有 `pnpm install` 步骤，`pnpm licenses list --prod
  --json` 在无 node_modules 时输出形态异常，parser 硬解析失败；
- `scripts/license-scan-parser.mjs` 只接受 `{license: [pkg,...]}` 形态，
  对 pnpm 版本漂移（对象/单包/嵌套 `licenses` 键/裸字符串）直接抛错。

**修复**：
- `scripts/license-scan.sh`：frontend 段前置 `pnpm install --frozen-lockfile`
  （no-op 当已装；失败时输出日志并 fail 门禁，绝不扫描空树）；
- `scripts/license-scan-parser.mjs`：结构容错 —— 数组/单对象/嵌套
  `licenses` 桶全部归一化，非包 value 跳过，空流不误杀；非 allowlist 仍
  硬失败（fail-closed 语义不变）。

## gosec 遗漏复核（中枢转述位置逐点审计）

中枢指令列的 7 个位置（oidc-provider:468 ListenAndServeTLS、alert:316
Rows.Next、kubeconfig:111 X509KeyPair、incidentchat:296 http.Client.Do、
demo-kube-mock:44 http.Client.Get、aiops/cases.go:278 http.Get、
cluster/clientprovider.go:46）经双重复验：

1. `golangci-lint v2.12.2 run --config ../.golangci.yml ./...` → **0 issues**
   （与 CI 完全相同的命令与版本）；
2. 独立 `gosec`（全 severity/confidence，排除 G115/G304/G104/G602）扫以上
   包 → **无 G402/G107 输出**。

各位置现状：demo-kube-mock 与 cmd/aiops cases.go 已带 `// #nosec` 注解；
其余位置非 G402/G107 触发模式（无 InsecureSkipVerify、SQL 为常量、URL 为
NewRequest 结构化传入）。**结论：所列位置在当前基线无真实 gosec 发现**，
无需追加标注（追加无意义 #nosec 反而污染代码）。判断依据：CI 失败日志
（gh run view）中 Dependency job 的 Go/lint 链全部通过，失败仅有 license
parse 一处；Backend job 的 golangci-lint 步骤在 13a486d run 中未见失败
（run 级失败仅 Dependency/Frontend/CI result 三 job）。

## Verification

- `cd frontend && pnpm install --frozen-lockfile` → esbuild 构建放行，成功。
- `./scripts/license-scan.sh` → Go 模块 + Frontend prod 全 allowlist，clean。
- parser 边界：单对象 value → exit 0；非 allowlist 许可证 → exit 1（fail-closed）。
- 本地/CI lint（v2.12.2，同命令同版本）0 issues。

## Artifacts

- `frontend/pnpm-workspace.yaml`
- `scripts/license-scan.sh`
- `scripts/license-scan-parser.mjs`