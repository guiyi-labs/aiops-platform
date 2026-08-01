# Kubernetes Multi-Cluster AIOps Platform

[![CI](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/Vue.js-3-4FC08D?logo=vuedotjs&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.34-326CE5?logo=kubernetes&logoColor=white)

## 当前基线（2026-07-31）

M1-M32 的本地开发路线已归档到 `baseline-m32-20260731`。M27 历史告警、
M28 固定范围 Velero Backup 创建、M29 Namespace 治理态势、M30 Node 维护和
M31 隔离恢复演练均已通过一次性真实 kind 验收；API/OpenAPI、24 组迁移、
最小权限 RBAC、前端类型和响应式工作流已重新对齐。

M33 将原始 `net/http` Kubernetes 网关迁移到受控 `client-go` 传输层
（ADR 0048）；M34 建立 `RouteDescriptor` 统一路由契约并补齐 ADR 0039
承诺的 RBAC 只读盘点（ADR 0049）；M35 引入轻量集群/Namespace 授权
（ADR 0050），Authorization 失败返回 404 以避免泄漏隐藏集群。M38 完成
工程化、交付与供应链加固（ADR 0051）：CI 门禁新增 race 检测、
golangci-lint、ESLint、50% 覆盖率基线和 OpenAPI 破坏性变更检查；real-kind
E2E 覆盖 M23-M31；新增官方 Helm 图表；发布流水线产出多架构 OCI 镜像
（linux/amd64、linux/arm64）和 SPDX SBOM；许可 allowlist 在门禁时强制。
`SECURITY.md` 和 `CHANGELOG.md` 成为受跟踪的交付物。两个里程碑均通过
本地 fast gate，未改变任何公开 API 契约。

最终快速门禁用时 26.17 秒；完整门禁用时 97.68 秒并通过全量 Go、73 个
Vitest 用例、前端生产构建、Compose 三服务健康、Kustomize 16/5/22/3 和
直连/代理 readiness。证据为
`.artifacts/verification/verify-20260731-015255.json`；归档见
[`docs/changes/2026-07-31-final-baseline-archive.md`](docs/changes/2026-07-31-final-baseline-archive.md)。

M32 后参照本地 KubeSphere 源码制定的优化路线、开发要求和验收标准见
[`docs/kubesphere-optimization-plan.md`](docs/kubesphere-optimization-plan.md)。
M38 完成记录见
[`docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md`](docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md)。
完整变更历史见 [`CHANGELOG.md`](CHANGELOG.md)，安全策略与漏洞披露流程见
[`SECURITY.md`](SECURITY.md)。

> 面向中小规模 Kubernetes 环境的多集群可观测、故障诊断与受控运维平台。

![AIOps Dashboard](docs/thesis/screenshots/01-dashboard.png)

## 核心能力

- **多集群健康视图**：以固定并发、超时和采样上限汇总集群状态，显式呈现覆盖率、截断和局部失败。
- **资源工作台**：提供工作负载（Pod/Deployment/StatefulSet/DaemonSet/ReplicaSet/Job/CronJob/HPA）、网络（Service/Ingress）、存储（PVC/PV）、策略（PDB/NetworkPolicy/ResourceQuota/LimitRange）、配置（ConfigMap/Secret 元数据/ServiceAccount）和 RBAC（Role/ClusterRole/RoleBinding/ClusterRoleBinding）等常见 Kubernetes 资源的只读列表、详情、关联事件、拓扑、脱敏 Manifest 和深链接。
- **全局资源搜索**：在固定 Pod、Deployment、Service、Ingress 范围内执行有界跨集群名称搜索。
- **证据型诊断**：确定性规则保留资源快照、事件和可追溯证据，AI 仅作为可选解释增强。
- **真实 Metrics**：读取可选 Metrics API，展示 Node/Pod CPU、内存绝对用量、利用率和消费者排行，并保留七天的精确序列历史与持续窗口评估。
- **历史告警生命周期**：基于 M21 精确序列的受限后台评估器、去重和确认生命周期（M27）。
- **受控运维操作**：Deployment rollout restart/scale、镜像更新与 ReplicaSet 修订回滚（M23）、CronJob suspend/resume、Velero Backup 创建（M28）、Node cordon/uncordon/PDB 感知驱逐（M30）、隔离命名空间恢复演练（M31），统一经过 dry-run、确认、幂等和审计。
- **安全与治理**：包含四类角色、加密集群凭据、会话管理、平台审计、安全 CSV、签名 Webhook outbox、Namespace 治理态势（M29）、RBAC 只读盘点（M34）和轻量集群/Namespace 授权（M35，未授权返回 404）。
- **交付门禁**：覆盖 Go/Vitest、Docker Compose、Kustomize、真实 kind E2E、版本化打包与校验和；CI 强制 race 检测、golangci-lint、ESLint、50% 覆盖率基线和 OpenAPI 破坏性变更检查；发布产出多架构 OCI 镜像、SPDX SBOM 和 SHA256 清单；许可 allowlist 在门禁时强制；提供官方 Helm 图表（`deploy/helm/aiops-platform/`）与 Kustomize 双路径部署。

## 技术栈

- Backend：Go、PostgreSQL/pgvector、Kubernetes client-go v0.34.x
- Frontend：Vue 3、TypeScript、Vite、Lucide
- Runtime：Docker Compose、Kustomize、Helm 3、kind、NGINX
- Delivery：GitHub Actions、Dependabot、actionlint、golangci-lint、ESLint、oasdiff、syft SBOM、多架构 OCI 归档

当前功能型 MVP 主链路已经闭环，并进入生产安全加固阶段。完整架构、设计决策、
测试矩阵和阶段归档见 [`docs/`](docs/README.md)。

生产 OIDC/MFA、物理/WAL PITR、HA 切换和远端 release 仍需组织授权与基础设施，
不属于本地基线的完成声明。Windows 主机缺少 `gcc`，因此 race 检测记录为环境阻塞，
没有被标记为通过。

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

### Helm 部署（可选）

M38 提供官方 Helm 3 图表，与 Kustomize 基线并行支持。Secrets 必须由外部提供（图表不生成 Secret）：

```powershell
# 准备 aiops-secrets（参考 deploy/kubernetes/secret.example.yaml 的 schema）
kubectl apply -f /secure/path/aiops-secret.yaml

# 渲染并安装
helm install aiops deploy/helm/aiops-platform -n aiops-system --create-namespace
```

图表契约由 `backend/internal/deployment/helm_chart_test.go` 强制保护，包含元数据、values、schema、必需模板、安全基线和不生成 Secret 规则等 10 项测试。

## Verification

```powershell
# Backend, frontend, Compose, manifests and HTTP health
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1

# Fast local feedback: backend, frontend and delivery contracts
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-fast.ps1

# M21/M23/M24/M25 disposable real-kind acceptance
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m21-history-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m23-release-lifecycle-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m24-cross-cluster-promotion-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m25-workload-protection-kind.ps1

# M27-M31 final disposable real-environment acceptance
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m27-alert-lifecycle-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m28-backup-creation-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m29-governance-posture-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m30-node-maintenance-kind.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m31-isolated-restore-kind.ps1

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

# Isolated PostgreSQL logical backup, source destruction and fresh restore gate
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-postgres-backup-restore.ps1

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
backend/             Go API service
frontend/            Vue 3 web console
deploy/              Compose, Kubernetes, Helm chart and demo manifests
deploy/helm/aiops-platform/   Official Helm 3 chart (Chart.yaml, values.schema.json, templates/)
docs/                Architecture, conventions, decisions and change records
SECURITY.md          Supported versions, vulnerability reporting, threat-model boundaries
CHANGELOG.md         Keep a Changelog 1.1.0 / SemVer 2.0.0 history
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

## Delivery Status（历史记录）

以下内容保留早期路线决策与证据链；当前状态以文首 2026-07-31 基线和最终归档为准。

当前已完成 M1-M19 和 M20 第一至第十二阶段。固定操作目录包含 Deployment rollout restart/scale 与 CronJob
suspend/resume，全部经过 dry-run、typed diff、精确资源快照、一次性确认、幂等执行、
审计和 namespaced RBAC；真实 kind 已验证执行、重放和 fixture 恢复。Deployment 镜像更新与
精确 ReplicaSet 修订回滚已在 M23 实现，绑定 preview 时刻捕获的不可变 ReplicaSet revision；
不接受客户端模板或任意 patch。
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
真实集群门禁仅运行一次性诊断/fleet/search 套件。第七阶段完成依赖审查、major 更新隔离和
Node 24 Actions 治理；第八阶段增加隔离 PostgreSQL 17 逻辑备份恢复门禁，在销毁源实例后
恢复到全新实例并校验迁移、代表性关系、加密字节和外键一致性，且不声明生产 RPO/RTO、PITR 或 HA。
第九至第十二阶段完成应用凭据再加密、签名审计归档、离线 OIDC/MFA 就绪准入和
生产恢复策略准入，均已通过托管 CI，但仍不声明生产 SSO、PITR 或 HA 已启用。
KRM/Ratel 复评后，后续路线已调整为 M21 历史可观测、M22 日常排障与治理、
M23 安全部署更新/回滚、M24 固定跨集群发布、M25 集群工作负载备份集成和
M26 组织集成/正式发布。
运维合同见 `docs/ci-release.md`，归档见
`docs/changes/2026-07-27-bounded-multi-cluster-health.md` 和
`docs/changes/2026-07-27-two-cluster-fleet-e2e.md`、
`docs/changes/2026-07-27-bounded-global-resource-search.md`、
`docs/changes/2026-07-27-user-owned-global-search-filters.md`、
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`、
`docs/changes/2026-07-28-versioned-ci-release-pipeline.md`、
`docs/changes/2026-07-28-dependency-governance.md`、
`docs/changes/2026-07-28-postgres-backup-restore.md`、
`docs/changes/2026-07-28-credential-key-reencryption.md`、
`docs/changes/2026-07-28-signed-audit-archives.md`、
`docs/changes/2026-07-28-identity-readiness-gate.md`、
`docs/changes/2026-07-28-recovery-readiness-gate.md` 和
`docs/changes/2026-07-28-product-roadmap-reprioritization.md`，交接入口见
`docs/development-handoff.md`。本地初始 Git 基线已经冻结并通过全量门禁，远端流水线状态以页首 CI 徽章为准。
