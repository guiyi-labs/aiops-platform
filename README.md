# Kubernetes Multi-Cluster AIOps Platform

[![CI](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/Vue.js-3-4FC08D?logo=vuedotjs&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.34-326CE5?logo=kubernetes&logoColor=white)

> 面向中小规模 Kubernetes 环境的多集群可观测、故障诊断与受控运维平台。

![AIOps Dashboard](docs/thesis/screenshots/01-dashboard.png)

## 核心能力

- **多集群健康视图**：以固定并发、超时和采样上限汇总集群状态，显式呈现覆盖率、截断和局部失败。
- **资源工作台**：提供 17 类常见 Kubernetes 资源的只读列表、详情、关联事件、拓扑和深链接。
- **全局资源搜索**：在固定 Pod、Deployment、Service、Ingress 范围内执行有界跨集群名称搜索。
- **证据型诊断**：七类确定性规则保留资源快照、事件和可追溯证据，AI 仅作为可选解释增强。
- **真实 Metrics**：读取可选 Metrics API，展示 Node/Pod CPU、内存绝对用量、利用率和消费者排行。
- **受控运维操作**：支持 Deployment rollout restart/scale 与 CronJob suspend/resume，统一经过 dry-run、确认、幂等和审计。
- **安全与治理**：包含四类角色、加密集群凭据、会话管理、平台审计、安全 CSV 和签名 Webhook outbox。
- **交付门禁**：覆盖 Go/Vitest、Docker Compose、Kustomize、真实 kind E2E、版本化打包与校验和。

## 技术栈

- Backend：Go、PostgreSQL/pgvector、Kubernetes client-go
- Frontend：Vue 3、TypeScript、Vite、Lucide
- Runtime：Docker Compose、Kustomize、kind、NGINX
- Delivery：GitHub Actions、Dependabot、actionlint、版本化 OCI 归档

当前功能型 MVP 主链路已经闭环，并进入生产安全加固阶段。完整架构、设计决策、
测试矩阵和阶段归档见 [`docs/`](docs/README.md)。

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
`docs/development-handoff.md`。本地初始 Git 基线已经冻结并通过全量门禁，远端流水线状态以页首 CI 徽章为准。
