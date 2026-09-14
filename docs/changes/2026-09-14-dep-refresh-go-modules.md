# 后端依赖组升级（Dependabot #23）：Go 模块六项同步

- Date: 2026-09-14
- Status: Complete
- Scope: 合入 Dependabot 分组 PR #23，升级 `backend/` 下 6 个 Go 模块依赖。

## Context

Dependabot 在 2026-08-24 打开 PR #23（`go-modules` 分组），提出 `backend/`
目录下 6 个 Go 模块的版本升级。该 PR 的其余 CI 作业全部通过，唯独
`Change-record archive gate` 失败——Dependabot 只改 `go.mod` / `go.sum`，
不会附带 `docs/changes/` 归档件。

本记录为该 PR 补的归档件，使其按仓库 AGENTS.md §1 铁律合入，而非绕过门禁。

## What Changed

### backend（Go 模块版本）

- `backend/go.mod` / `backend/go.sum`：以下 6 处升级
  - `github.com/lib/pq` 1.10.9 → 1.12.3
  - `github.com/stretchr/testify` 1.11.1 → 1.12.1
  - `golang.org/x/crypto` 0.54.0 → 0.57.0
  - `gorm.io/driver/postgres` 1.6.0 → 1.6.2
  - `k8s.io/apimachinery` 0.36.3 → 0.37.0
  - `k8s.io/client-go` 0.36.3 → 0.37.0

- 本记录 `docs/changes/2026-09-14-dep-refresh-go-modules.md`：新增。

未涉及业务源码改动。`client-go` / `apimachinery` 成对升级，版本保持一致。

## Verification

- PR #23 CI（run 34828430949）：其余作业全部 pass，包括 `Backend`、
  `Backend race`、`Backend image`、`Frontend`、`Compose runtime`、
  `Manifests`、`Dependency & supply chain`。
  唯一失败项为 `Change-record archive gate`（本记录即为其修复）。
- 这两个作业覆盖了 `go build` + 全量测试 + race 检测，是 client-go 升级
  兼容性的直接证据。
- 补记录后重跑 CI：见 PR #23 的 checks（本记录推送后触发）。
- 本地：`bash .git/info/redline-guard.sh audit` → 全库 0 命中。

## Risks / Notes

- `k8s.io/client-go` 与 `k8s.io/apimachinery` 从 0.36.3 跨到 0.37.0，属
  **0.x 系列次版本**，按语义化版本惯例 0.x 的次版本允许破坏性变更。本次依据是
  CI 的 `Backend` 与 `Backend race` 双作业通过（编译 + 全量测试 + 数据竞争检测），
  并未逐条核对 upstream 变更日志。若后续出现 client-go 相关运行时异常，
  优先怀疑此处，回退方式为 revert 本 PR 的 squash 提交。
- `golang.org/x/crypto` 升级可能影响凭据加密路径，CI 中的 `Credential drill`
  与 `Identity readiness drill` 已覆盖该路径。
- 同类分组 PR #26（frontend-packages）另行归档；
  #25（GitHub Actions 分组）因 `TestCIWorkflowContractsAreParseableAndBounded`
  失败而**未合入**——该测试校验 CI 工作流契约，bump
  `docker/setup-qemu-action` / `docker/setup-buildx-action` 会打破契约断言，
  需独立修复后再合。
