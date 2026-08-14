# Kubernetes Multi-Cluster AIOps Platform

[![CI](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/Vue.js-3-4FC08D?logo=vuedotjs&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.36-326CE5?logo=kubernetes&logoColor=white)

> 面向中小规模 Kubernetes 环境的多集群可观测、故障诊断与受控运维平台。

![AIOps Dashboard](docs/thesis/screenshots/01-dashboard.png)

## 项目简介

平台以确定性规则诊断为主链路、AI 解释为可选增强，提供从多集群资源浏览、
故障定位到受控修复的闭环能力。通过三层控制台（平台 → 集群 → 工作区）和
2D 授权矩阵（集群 + Namespace 粒度）实现安全的多租户隔离；所有高风险操作
经 dry-run、确认、幂等和审计，凭据加密存储且未授权资源返回 404 以避免
信息泄漏。

## 当前基线（M112 · 2026-08-14）

当前公开基线已推进至 **M112**。仓库把 Kubernetes 平台运维拆成可验证的工作链路，
重点展示“发现问题 → 保留证据 → 辅助判断 → 受控处置 → 事故复盘”的工程闭环。

| 方向 | 当前可验证能力 |
|---|---|
| 多集群运维 | 集群 / Namespace 授权、资源工作台、跨集群态势、CRD 只读浏览与全局搜索 |
| 可观测与诊断 | 指标、日志、事件、SLO、确定性规则诊断、多信号关联与证据时间线 |
| 事故响应 | 事故工作区、SLA / MTTA / MTTR、升级通知、Runbook、复盘 Markdown 导出 |
| AI 辅助 | 引用校验的调查、引用式事故摘要、解释覆盖率与质量反馈只读大盘 |
| 受控运维 | dry-run、人工确认、幂等执行、结果校验、审计；AI 不直接执行集群变更 |
| 工程交付 | Go / Vue 全栈测试、OpenAPI/typegen、axe、响应式截图基线、CI 与供应链门禁 |

M109 已将 CI 全局覆盖率门禁提升至 65%；M112 的后端测试、前端类型检查、Lint、测试与构建均已通过。
完整变更历史见 [`CHANGELOG.md`](CHANGELOG.md)，阶段记录见 [`docs/changes/`](docs/changes/)，
当前执行路线见 [`docs/development-roadmap-post-m110.md`](docs/development-roadmap-post-m110.md)，
论文与答辩材料见 [`docs/thesis/README.md`](docs/thesis/README.md)。

> **项目边界**：当前仓库保持 RC 口径。生产 OIDC/MFA、真实组织环境的 HA / PITR、发布授权与远端
> 基础设施演练仍需外部条件，不在本地基线中宣称已完成。

## 核心能力

- **多集群与多租户**：有界并发健康 fan-out 与覆盖率/截断显式呈现（M20）；固定资源范围全局搜索与用户私有筛选器（M20）；三层控制台与工作区多租户隔离（M46-M47）；Host/Member 多集群联邦（M48）；轻量集群/Namespace 授权，未授权返回 404（M35）。
- **资源工作台**：工作负载、网络、存储、策略、配置和 RBAC 等常见资源的只读列表/详情/事件/拓扑/脱敏 Manifest（M12-M17）；编译时白名单的 CRD 发现与只读实例浏览（M49）；深链接详情抽屉与响应式交互。
- **全栈可观测性**：监控大盘与日志探索器（M50）；有界 SSE 事件流与告警抑制规则（M51）；Node/Pod 真实 Metrics、七天精确序列历史与持续窗口评估（M15-M21）；历史告警去重与确认生命周期（M27）；时序拓扑与变更事件（M40）；SLO 错误预算与影响面（M41）。
- **证据型诊断**：确定性规则保留资源快照、事件和可追溯证据，AI 仅作可选解释增强（M18）；多信号关联与确定性 RCA（M43）；引用校验的 AI Investigator（M44）；智能巡检规则目录与计划任务（M52）。
- **受控运维**：Deployment rollout restart/scale/镜像更新/回滚、CronJob suspend/resume（M19/M23）；Velero Backup 创建与备份/恢复 GUI 浏览（M28/M58）；Node cordon/uncordon/PDB 感知驱逐（M30）；隔离命名空间恢复演练（M31）；Helm 应用目录与 Flux HelmRelease 受控部署（M57）；GitOps 只读浏览（M58）；跨集群资源复制（M58）；统一经 dry-run、确认、幂等和审计。
- **服务网格只读**：VirtualService/DestinationRule 只读浏览与固定 Prometheus 模板流量指标投影（M52）。
- **质量与交付**：黄金数据集回放与质量报告（M56）；编译时 Provider Registry 统一生命周期/健康/角色选择（M60）；Cosign keyless 签名与 SLSA provenance 占位（M59）。
- **安全与治理**：四类角色、加密集群凭据、会话管理、平台审计、安全 CSV 导出（M20）；OIDC/MFA 离线就绪准入（M32）；Namespace 治理态势（M29）；RBAC 只读盘点（M34）；签名审计归档（M31）；凭据密钥再加密（M30）。
- **交付门禁**：Go/Vitest、Docker Compose、Kustomize、真实 kind E2E、版本化打包与校验和；CI 强制 race 检测、golangci-lint、ESLint、65% 覆盖率门禁（核心包 ≥70%）和 OpenAPI 破坏性变更检查；发布产出多架构 OCI 镜像、SPDX SBOM 和 SHA256 清单；许可 allowlist 强制；提供官方 Helm 图表与 Kustomize 双路径部署。

## 技术栈

- Backend：Go 1.26、PostgreSQL/pgvector、Kubernetes client-go v0.36.x
- Frontend：Vue 3、TypeScript、Vite、Lucide
- Runtime：Docker Compose、Kustomize、Helm 3、kind、NGINX
- Delivery：GitHub Actions、Dependabot、actionlint、golangci-lint、ESLint、oasdiff、syft SBOM、Cosign、多架构 OCI 归档

完整架构、设计决策、测试矩阵和阶段归档见 [`docs/`](docs/README.md)。Windows 主机缺少
`gcc`，race 检测由 CI (Linux) 覆盖；生产环境能力边界以 [`SECURITY.md`](SECURITY.md)
和 [`docs/testing/known-limitations.md`](docs/testing/known-limitations.md) 为准。

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

开发环境首次启动会创建系统管理员，默认账号为 `admin` / `admin123`。该口令仅用于本地开发，部署前必须通过环境变量修改。

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

本项目从**已有且可访问的 Kubernetes 集群**开始，负责 Kubernetes 的 Day 2 运行期管理：

| 阶段 | 负责仓库 | 本项目边界 |
|---|---|---|
| Day 0/1 | [`kubernetes-cluster-bootstrap`](https://github.com/guiyi-labs/kubernetes-cluster-bootstrap) | Linux 预检、containerd、kubeadm、控制平面、Worker、CNI、HA 和交付验收 |
| Linux 运行期 | [`devops-automation`](https://github.com/guiyi-labs/devops-automation) | SSH 主机、systemd、进程、磁盘、批量任务、备份和主机监控 |
| Kubernetes 运行期 | `aiops-platform`（本项目） | 多集群、工作负载、指标/日志/事件、SLO、诊断、事故响应和受控修复 |

因此，集群创建、节点初始化、操作系统配置、containerd 安装、kubeadm init/join、CNI 安装和
控制平面 HA 交付不属于本项目范围。本项目只接收集群连接信息和验收结果，不复制 bootstrap
脚本，也不把 Linux 主机运维能力重新做成另一套控制台。

## Repository Notes

- Git 历史已完成安全脱敏重写，清除了历史提交中的个人邮箱；历史文档中的旧 commit hash
  仅用于归档参考，定位里程碑请优先使用 tag 名称和提交信息。
- 文档中的 `<repo-root>`、`<local-refs>`、`<docker-data>` 是路径占位符，部署时需替换为实际路径。

## Reference Projects

KRM 和 Ratel 仅用于需求、交互及部署思路参考。本仓库独立实现，不包含参考项目的应用源码或容器镜像内容。

## 早期里程碑索引（M1-M32）

以下仅保留早期路线的决策摘要；当前状态以文首 M112 基线和 [`CHANGELOG.md`](CHANGELOG.md) 为准。

- **M1-M19**：核心资源只读链路、证据型诊断、受控操作目录（Deployment scale/restart、CronJob suspend/resume），全部经 dry-run、幂等和审计。
- **M20-M26**：有界多集群健康 fan-out、全局搜索、用户私有筛选器、分层 CI/发布流水线、依赖治理、PostgreSQL 备份恢复、凭据再加密、签名审计归档、OIDC/MFA 就绪准入、恢复策略准入。
- **M27-M32**：历史告警生命周期、Velero Backup 创建、Namespace 治理态势、Node 维护、隔离恢复演练、最终基线归档。

完整里程碑归档见 [`docs/changes/`](docs/changes/)，交接入口见
[`docs/development-handoff.md`](docs/development-handoff.md)，运维合同见
[`docs/ci-release.md`](docs/ci-release.md)。远端流水线状态以页首 CI 徽章为准。
