# Kubernetes Multi-Cluster AIOps Platform

面向中小规模 Kubernetes 环境的多集群统一管理与可解释故障诊断平台。

当前状态：功能型 MVP 主链路已经闭环，并进入生产安全加固阶段。系统支持认证、四类角色、系统管理员用户管理及全会话失效密码重置，加密集群接入、含 EndpointSlice 与可选 Metrics API 的固定 Kubernetes 资源查询、17 类资源的可深链只读详情与关联事件、固定 Pod/Deployment/Service/Ingress 的有界跨集群名称搜索、Ingress 到 Deployment 的完整只读资源拓扑、真实 CPU/内存绝对用量、七类可追溯规则诊断、分级 SLA、负责人转派、人工处置、可筛选及安全 CSV 导出的平台审计，以及默认关闭、严格引用规则证据、受并发与每日 token 预算保护、支持逐条人工评价且失败不影响主链路的 Responses-compatible AI 解释。Secret 只在公开模型中展示最小 metadata、类型、immutable 和 data key 名，不返回值、labels 或 annotations。

诊断创建、状态变化和负责人变化现可通过默认关闭的事务 outbox 投递到 HMAC-SHA256 签名 Webhook。投递支持持久重试、dead 状态和受 RBAC/审计保护的人工重新排队；事件 payload 不包含诊断证据、处置备注或凭据。

受控操作保留与 confirmed Pod 诊断匹配的 Deployment rollout restart，并新增资源详情发起的 Deployment 扩缩容、CronJob 暂停和恢复。服务端先执行 Kubernetes server-side dry-run，再通过一次性确认 token、UID/resourceVersion 快照、字段级前后值、幂等键和审计记录执行固定 patch；不会接受任意 Kubernetes 写请求。

## Quick Start

准备环境变量：

```powershell
Copy-Item .env.example .env
```

使用 Docker Compose 启动：

```powershell
docker compose up --build
```

访问地址：

- Web UI: `http://localhost:18080`（可通过 `FRONTEND_PORT` 修改宿主机端口）
- Backend liveness: `http://localhost:8080/api/v1/health/live`
- Backend readiness: `http://localhost:8080/api/v1/health/ready`
- Backend metrics (local/internal monitoring only): `http://localhost:8080/metrics`

开发环境首次启动会创建系统管理员，默认账号为 `admin` / `change_me_now`。该口令仅用于本地开发，部署前必须通过环境变量修改。

停止服务：

```powershell
docker compose down
```

## Verification

```powershell
# Backend, frontend, Compose, manifests and HTTP health
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1

# Real kind diagnosis and controlled-remediation workflow
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-kind.ps1

# Real Metrics available path; retain the populated demo cluster after success
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-metrics-kind.ps1 -KeepPlatformCluster

# Disposable real kind validation for Node/Deployment diagnosis rules
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-diagnosis-kind.ps1

# Disposable two-cluster fleet fan-out, fault-isolation and cleanup gate
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-fleet-kind.ps1

# Disposable two-cluster fixed-kind global-search and cleanup gate
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-global-search-kind.ps1

# CI, tagged release and self-hosted real-kind runner contract
# See docs/ci-release.md before enabling remote workflows

# Retain populated defense data and capture authenticated thesis screenshots
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-up.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\capture-thesis-screenshots.ps1

# Remove retained platform data; add -CleanupDemoResources to remove kind fixtures
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-down.ps1
```

验收证据写入已忽略的 `.artifacts`。论文与答辩材料索引见 `docs/thesis/README.md`。

## Repository Layout

```text
backend/    Go API service
frontend/   Vue 3 web console
deploy/     Compose, Kubernetes and demo manifests
docs/       Architecture, conventions, decisions and change records
```

开发前请先阅读 [开发指南](docs/development.md) 和 [目录与文件规范](docs/conventions/project-structure.md)。

## Project Boundaries

- Kubernetes 实时资源通过 API Server 查询，不全量写入平台数据库。
- 规则诊断是确定性主链路，AI 只做解释增强。
- AI 不直接执行集群变更操作。
- 所有高风险操作必须经过权限校验、人工确认并写入审计日志。
- kubeconfig 和其他凭据不得提交到 Git。

## Reference Projects

KRM 和 Ratel 仅用于需求、交互及部署思路参考。本仓库独立实现，不包含参考项目的应用源码或容器镜像内容。

## Delivery Status

当前已完成 M1-M19 和 M20 第一至第六阶段。固定操作目录包含 Deployment rollout restart/scale 与 CronJob
suspend/resume，全部经过 dry-run、typed diff、精确资源快照、一次性确认、幂等执行、
审计和 namespaced RBAC；真实 kind 已验证执行、重放和 fixture 恢复。Deployment rollback
因缺少精确 ReplicaSet revision/template 历史而明确延后，不接受客户端模板或任意 patch。
M20 第一阶段增加有界多集群健康比较，固定 20 集群、4 并发、单集群 4 秒与每类 100
对象采样上限，并显式返回部分失败和采样覆盖率；第二阶段已用两个独立 kind 集群和隔离平台
验证真实 fan-out、超时、恢复、不可用隔离、只读 RBAC 与完整清理。第三阶段新增固定
Pod/Deployment/Service/Ingress 的跨集群名称搜索，沿用 20 集群、4 并发和单集群 4 秒
边界，显式返回结果截断、集群覆盖与局部失败，并深链既有资源工作台。第四阶段新增
当前用户私有的搜索筛选器，限制 20 条，仅保存名称、查询词、Namespace 和四类固定资源子集，
支持应用、重命名、完整覆盖和删除，并以所有权谓词、并发锁、兼容状态和统一审计保持边界。
第五阶段通过两个一次性 Kubernetes v1.34.0 kind 集群和隔离平台，验证 9 条固定资源搜索结果的
跨集群排序、覆盖率、全局截断、暂停超时、恢复、停机失败隔离、只读 RBAC 与八项完整清理。
第六阶段新增分层 CI、语义版本发布打包和自托管 Windows 真实 kind 周期门禁：常规 PR 无密钥运行，
tag 发布前复用完整 CI 并生成 OCI 归档、源码、OpenAPI、依赖许可、元数据和 SHA-256 清单；
真实集群门禁仅运行一次性诊断/fleet/search 套件。运维合同见 `docs/ci-release.md`，归档见
`docs/changes/2026-07-27-bounded-multi-cluster-health.md` 和
`docs/changes/2026-07-27-two-cluster-fleet-e2e.md`、
`docs/changes/2026-07-27-bounded-global-resource-search.md`、
`docs/changes/2026-07-27-user-owned-global-search-filters.md`、
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`、
`docs/changes/2026-07-28-versioned-ci-release-pipeline.md`，交接入口见
`docs/development-handoff.md`。仓库尚无初始 Git commit，需人工确认作者身份和提交范围后再冻结 revision。
