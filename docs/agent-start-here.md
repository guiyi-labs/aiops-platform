# New Environment Agent Start Here

- Updated: 2026-07-30
- Repository: `https://github.com/guiyi-labs/aiops-platform.git`
- Current code baseline: commit `62320fcac3bbb50b33b7cd6945495264b04b026c`
- Annotated baseline tag: `baseline-m25-20260730`
- Accepted hosted CI: [run 30502322757](https://github.com/guiyi-labs/aiops-platform/actions/runs/30502322757)

这是一份给新设备上接管本项目的 Agent 的首读文档。除非用户明确要求复现历史版本，
开发应从最新 `origin/main` 开始；需要复现 M21-M25 精确代码状态时，再检出
`baseline-m25-20260730`。该标签固定指向 `62320fc`，标签之后允许存在纯文档或后续
里程碑提交。

## Current Baseline

项目是面向中小规模 Kubernetes 环境的多集群 AIOps 与受控运维平台：

- 后端：Go 1.25、Gin、GORM、PostgreSQL 17/pgvector、Kubernetes client-go。
- 前端：Vue 3、TypeScript、Vite、Pinia、Vitest。
- 运行时：Docker Compose；目标集群通过短期 ServiceAccount 凭据接入。
- 交付：GitHub Actions、Kustomize、一次性 kind、脱敏机器证据。

M21-M25 已完成并纳入基线：

| 里程碑 | 已接受能力 |
|---|---|
| M21 | PostgreSQL 稀疏历史、后台 Metrics 采集、精确序列查询、持续窗口评估、趋势 UI、Node 指标诊断证据 |
| M22 | 多容器日志、固定只读资源、服务端脱敏 Manifest、统一资源详情工作台 |
| M23 | Deployment 镜像更新、精确 ReplicaSet 修订回滚、dry-run/确认/幂等/审计、真实 kind 生命周期 |
| M24 | 固定 Deployment/Service/Ingress 跨集群发布、依赖映射、目标预检、逐项执行结果、最小写 RBAC |
| M25 | 可选 Velero 能力探测、只读 Backup 库存与详情、未安装时稳定 424、只读 RBAC |

最新基线通过 271 个 Go `Test*` 入口、17 个 Vitest 文件/73 个用例、生产构建、
三服务 Compose 健康检查，以及 M21/M23/M24/M25 真实 kind 验收。新克隆不会包含
本地 `.artifacts`；应以托管 CI 和变更归档为跨设备证据，以新设备重新生成的
`.artifacts` 为本机证据。

## Read Order

新 Agent 在首次修改代码前按顺序阅读：

1. 本文：当前系统、边界和接管步骤。
2. `docs/new-environment-bootstrap.md`：新设备安装、密钥、启动和验收。
3. `docs/changes/2026-07-30-m21-m25-baseline-alignment.md`：本轮审查修复与证据账本。
4. `docs/project-vision-and-delivery-standards.md`：后续路线、准入和完成标准。
5. `docs/roadmap.md`：里程碑状态与明确非目标。
6. `docs/development-handoff.md`：历史交接细节；顶部 Current Baseline 优先于下方旧阶段记录。
7. 当前任务相关 ADR、OpenAPI 和变更归档，不要无目的读取全部历史 ADR。

## Architecture Map

| 路径 | 责任 | 变更时必须同步 |
|---|---|---|
| `backend/internal/<domain>` | 领域模型、规则、仓储和服务 | 单元/仓储测试、迁移、错误合同 |
| `backend/internal/httpserver` | 鉴权、HTTP 参数、稳定错误、路由 | OpenAPI、路由合同测试 |
| `backend/internal/kubernetes` | 固定 Kubernetes 读写能力和字段裁剪 | fake/loopback 测试、RBAC、真实 kind |
| `backend/migrations` | 仅向前追加的 PostgreSQL schema | down migration、约束测试、真实数据库应用 |
| `frontend/src/api` 与 `types` | 鉴权 API 客户端和类型 | Vitest、OpenAPI 对齐 |
| `frontend/src/views` 与 `components` | 操作员工作流 | typecheck、Vitest、生产构建、响应式检查 |
| `deploy` | 平台、目标集群和一次性验收清单 | Kustomize 渲染、最小 RBAC、清理边界 |
| `scripts` | 快速/完整/真实环境门禁 | PowerShell 5.1 兼容、脱敏证据、finally 清理 |
| `.github/workflows` | 托管和真实 kind 流水线 | 固定 action SHA、无 PR 密钥、合同测试 |
| `docs/adr` / `docs/changes` | 为什么这样设计 / 本阶段做了什么 | 状态、边界、证据、后续项 |

## First 30 Minutes

新 Agent 的第一轮只做审计和恢复，不直接改功能：

```powershell
git remote -v
git fetch --tags --prune origin
git switch main
git pull --ff-only
git status --short --branch
git log -3 --oneline --decorate
git tag --contains 62320fcac3bbb50b33b7cd6945495264b04b026c
```

随后按 `docs/new-environment-bootstrap.md` 完成工具链预检，创建新的 `.env`，启动
Compose，依次运行 `scripts/verify-fast.ps1` 和 `scripts/verify.ps1`。只有这两层通过，
才开始新里程碑。涉及 Kubernetes、数据库写入、RBAC、CI 或故障恢复时，再运行对应
一次性真实验收。

首次汇报至少包含：

- 当前 HEAD、与 `origin/main` 的 ahead/behind 数量、工作树是否干净。
- Docker/Go/Node/pnpm/kubectl/kind 版本和端口冲突。
- Compose 三服务健康状态、最新 migration 名称。
- 快速/完整门禁结果和证据路径。
- 当前要做的里程碑、明确非目标和预计触及的合同边界。

## Non-Negotiable Invariants

- 确定性规则是诊断主链路；AI 只能解释带引用的证据，不能直接执行集群变更。
- 不提供任意 GVK、任意 YAML/patch、无限日志、任意 PromQL 或敏感值查看代理。
- 所有跨集群查询必须有集群数、并发、超时、每类对象和总结果上限，并显式报告
  部分失败、遗漏和截断。
- Secret 值、kubeconfig、token、私钥、Webhook 密钥、数据库密码和未脱敏上游错误
  不进入 API 响应、日志、审计详情、Git 或 CI 产物。
- 所有写操作由服务端导出目标与 diff，先 server-side dry-run，再一次性确认、幂等键、
  UID/resourceVersion 前置条件、最小权限 RBAC、审计和失败安全重试。
- migration 只向前追加；不得修改已发布 migration 的语义来迁就本地数据库。
- 真实 E2E 必须使用唯一名称、短期凭据、隔离资源和 `finally` 清理，不得复用或删除
  用户保留的 `aiops-test`。
- 未实际运行的门禁不得标记通过；生成证据必须脱敏，`.artifacts` 不提交 Git。

## Safe Agent Working Rules

1. 先检查 `git status`，保留用户已有改动；禁止用 reset/checkout 丢弃未知内容。
2. 先冻结请求/响应、错误码、上限、RBAC 和非目标，再并行或分层开发。
3. 修复根因并增加能复现问题的测试；不要只调整验收脚本绕过产品缺陷。
4. 公共路由、DTO、前端类型、OpenAPI 和路由合同测试必须同一提交对齐。
5. 涉及 schema、写权限、幂等或恢复的变更必须有 ADR 和一次性真实环境证据。
6. 每个里程碑结束前更新 README、handoff、test matrix、roadmap 和 changes 归档。
7. 提交前运行差异/敏感信息/Git 完整性审计；推送后等待统一 CI 结果作业成功。

## Where To Continue

当前优先级从 `docs/project-vision-and-delivery-standards.md` 和 `docs/roadmap.md` 获取。
默认顺序为 M26 可复现交付与组织准入、M27 历史告警生命周期、M28 受控备份创建、
M29 发布绑定的论文/演示刷新。任何改序都要说明用户价值、风险、依赖和为什么不破坏
当前固定合同。
