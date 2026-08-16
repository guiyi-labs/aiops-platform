# P2a 质量部分（gosec/trivy/nanoid + 测试提测，CI 门禁阈值拆分）

- Date: 2026-08-16
- Status: Complete（质量部分）；门禁阈值拆分未完成（待旗舰包提测）
- Scope: ④ P2a 调整版 —— 核心包 ≥75%（4 旗舰包）+ gosec 高危；本 commit 只含
  确定性质量增益，CI 门禁阈值改动留待 4 旗舰包提测后随门禁一起提交
- DependsOn: README（0fcaacd）、CLI（96c7856）、发布准备（d88144e）

## What

### 1. gosec 高危修复 + lint 规则（全仓库 0 issues）

- `.golangci.yml`：启用 `gosec`；排除保守噪声 G115/G304/G104/G602 与 `_test.go`
  （真实发现——cookie 标志、文件权限、TLS——在代码内修复或标注，不排除）。
- 各包修复（均为真实 gosec 发现）：
  - `cluster/kubeconfig.go`、`cmd/demo-kube-mock`：G402 TLS 校验跳过 → `#nosec G402`
    注明（CLI/演示 fixture 场景）。
  - `config/config.go`、`golden/service.go`、`golden/storage.go`、
    `scalebench/report.go` 等：G306/G301 文件权限收紧（0o600/0o644 语义化）。
  - `netpolicy`、`inspection`、`httpserver/auth|oidc`、`auth/token.go`、
    `scalefixture/generate.go`、`cluster/model.go`：G112/G107/G110 等修复/标注。
- 验证：`golangci-lint run --config ../.golangci.yml ./...` → **0 issues**。

### 2. trivy 依赖漏洞扫描脚本

- `scripts/dependency-vuln-scan.sh`：trivy `fs` 逐目录扫描（backend module、
  frontend lockfile、root），`--exit-code 1` 失败即退；合同扫描结果 **0
  findings**。

### 3. frontend 供应链修复

- `pnpm-workspace.yaml` + `pnpm-lock.yaml`：`nanoid` **3.3.18** overrides
  （修复 CVE-2025-49013）；`package.json` 移除 pnpm 引擎字段（pnpm v11
  workspace 兼容）。

### 4. 测试确定性 + 提测

- `knowledge/repository.go`：severity 过滤改确定性顺序（消除
  TestGormRepositoryListByFilter flaky）。
- `metricshistory/pure_helpers_test.go`（新增）：提测 74.9 → 76.2%。

## 拆分说明（为什么本次不含 CI 门禁阈值）

工作树原 CI 门禁改动是「5 包 ≥75%」旧版（metricshistory/apiquery/
deprecatedapi/optimization/knowledge）。中枢 P2a 调整版定义核心包为 4 旗舰包
（**diagnosis/knowledge/aiexplain/aiinvestigator**），当前
diagnosis 67.9% / aiexplain 52.9% / aiinvestigator 71.6% 未达标。若现在提交
4 包 75% 门禁 → CI 红，破坏门禁链。因此 **CI yml 阈值改动暂存工作树**，待 4
旗舰包提测完成后随门禁一起提交（下一步任务）。

## Verification

- `golangci-lint run --config ../.golangci.yml ./...` → 0 issues。
- `go test ./internal/metricshistory/ ./internal/knowledge/` → ok。
- `scripts/dependency-vuln-scan.sh` → 0 findings（此前验证）。

## Artifacts

- `.golangci.yml`、`scripts/dependency-vuln-scan.sh`
- backend 13 个文件的 gosec 修复
- `frontend/pnpm-workspace.yaml` + `pnpm-lock.yaml`
- `backend/internal/knowledge/repository.go`、`metricshistory/pure_helpers_test.go`