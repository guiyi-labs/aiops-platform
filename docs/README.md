# Documentation Index

文档按用途归档，禁止将长期有效的设计说明放在临时聊天记录或源码注释中。

| 目录或文件 | 内容 |
|---|---|
| `next-long-term-plan.md` | **当前唯一执行入口**：M93-B2–M102 的 12–16 周里程碑、依赖、阶段门、任务板、风险与 GA 准入 |
| `polish-plan.md` | M78 之后的打磨合同与历史差距基线；当前执行顺序以 `next-long-term-plan.md` 为准 |
| `ARCHIVING.md` | **归档手册**：所有修改必须遵守的四层归档体系、change-record 模板、提交/标签流程与完整性检查表（受根目录 AGENTS.md 强制约束） |
| `changes/TEMPLATE.md` | change-record 标准模板：日期、状态、范围、Context、What Changed、Verification、Risks |
| `kubesphere-optimization-plan.md` | M32 后以 KubeSphere 能力为底座、融合 AIOps 信号/诊断/AI/安全闭环的 M33-M45 路线和验收标准 |
| `next-development-plan.md` | 已归档的 M26-M32 开发执行合同，不再作为当前开发入口 |
| `development.md` | 本地开发、测试和运行方法 |
| `development-handoff.md` | 当前里程碑、稳定基线、进行中工作和下次启动入口 |
| `roadmap.md` | M19-M32 产品优化顺序、阶段状态、范围边界和发布前置条件 |
| `ci-release.md` | CI 分层、分支保护、RC tag 发布、产物校验和自托管 kind runner 运维合同 |
| `release-candidate-operations.md` | RC 在线/离线安装、升级、回滚、健康检查、认证和清理运行手册 |
| `operations/` | M102 运维手册（`runbook.md`）：起停、健康、升级回滚、备份恢复、验收、审计与故障排查 |
| `conventions/` | 目录、命名、接口和提交规范 |
| `architecture/` | 系统架构、模块和数据流设计 |
| `adr/` | 重要技术决策及其原因 |
| `changes/` | 按日期归档的阶段变更记录 |
| `references/` | 外部项目的可追溯分析与采用边界 |
| `api/` | API 契约和错误码 |
| `database/` | 数据模型与迁移说明 |
| `security/` | 身份源就绪合同、安全策略模板、接入运行手册与 M102 安全声明（`security-statement.md`） |
| `testing/` | 测试策略、M102 测试矩阵（`test-matrix.md`）、已知限制（`known-limitations.md`）、兼容范围（`compatibility.md`）与演示场景 |
| `thesis/` | 论文图表、实验数据和答辩材料 |

M32 后的权威优化入口见 `kubesphere-optimization-plan.md`。M25 之后曾使用的
`references/final-product-gap-analysis.md` 已完成其 M27-M32 路线决策职责，现作为历史分析保留。

当前发布冻结候选归档：

- `docs/changes/2026-08-10-m96-frontend-scale-budget.md`：M96 前端 50k Pod 确定性 fixture、虚拟列表边界与桌面/移动端 DOM/交互 report-mode 基线。
- `docs/changes/2026-08-10-m96-console-shell-convergence.md`：M96 认证单壳层、页面桥接、四层 CSS active baseline 与浏览器回归。
- `docs/changes/2026-08-10-m96-gate-b-evidence-aggregation.md`：M96 Gate B 规模证据聚合、哈希一致性与 CI 汇总报告。
- `docs/changes/2026-08-10-m97-release-candidate-closure.md`：M97 统一 release manifest、双部署路径、离线包、签名校验和生命周期演练。
- `docs/changes/2026-08-12-m98-incident-workspace.md`：M98 事故工作空间与协作闭环（后端领域模块、迁移、REST/OpenAPI、前端视图、桌面/移动端 e2e 8/8）。
- `docs/changes/2026-08-12-m99a-slo-burn-signal-pipeline.md`：M99-A SLO burn 信号化管道、correlation SLO-burn 规则与黄金回放、AIOps 路由生产接线。
- `docs/changes/2026-08-12-m99b-metricshistory-slo-source.md`：M99-B metricshistory 驱动的 SLO workload_readiness 指标源、Deployment readiness 采集与 `POST /slos` ID 修复。
- `docs/changes/2026-08-12-m99c-correlation-provider-worker.md`：M99-C correlation 生产输入源与周期关联 worker、`diagnosis.ListFilter.Since` 时间过滤。
- `docs/changes/2026-08-12-m99d-data-visibility.md`：M99-D SLO/信号/关联案例的缺样本与数据延迟显式展示、`SignalLink` 数据元数据与迁移 `000041`。
- `docs/changes/2026-08-12-m100a-permission-matrix-and-aiops-scope.md`：M100-A 路由权限矩阵生成与差异门禁、`/aiops` 查询维度集群/命名空间授权强制。
- `docs/changes/2026-08-12-m100b-session-invalidation-journeys.md`：M100-B 会话/密码/auth_version 失效旅程补齐（`UpdateUser` 禁用/角色变更 bump `auth_version`）与 4 旅程单测 + 14 项运行时冒烟复验。
- `docs/changes/2026-08-12-m100c-sensitive-field-scan-and-log-redaction.md`：M100-C 敏感字段静态扫描门禁（`scripts/scan-sensitive-fields.sh` + CI）与日志/审计脱敏契约测试、运行时 0 泄漏复验。
- `docs/changes/2026-08-12-m100d-dependency-and-supply-chain-gates.md`：M100-D 依赖漏洞修复（pgx/quic-go/nanoid）与 govulncheck、pnpm audit、许可证 allowlist、镜像基础层漂移、SBOM 差异门禁。
- `docs/changes/2026-08-12-m101-wal-pitr-drill.md`：M101 本地数据轨——WAL 归档 + PITR + 流式备库 + 故障注入 8 场景确定性演练（无损/时间点/缺 WAL/迁移前逻辑备份/SIGKILL 崩溃/备库故障切换/网络分区/归档目标故障），实测 RPO/RTO 观测值与归档链路恢复。
- `docs/changes/2026-08-12-m89-oidc-local-drill.md`：M89 身份轨本地预研——仓库内 OIDC Provider（`backend/cmd/oidc-provider`）+ 全链路登录演练（`scripts/oidc-login-drill.sh`）14/14 场景，真实 Authorization Code + PKCE + acr MFA 证据 + 各项 fail-closed（缺预关联 403、nonce/state 篡改、MFA 证据缺失/不接受、组无映射、轮换密钥、过期/unsigned token、Provider 下线）与审计落库、启动期 fail-fast。
- `docs/changes/2026-08-12-m102-dual-env-compose-drill.md`：M102 本地轨道——双全新隔离环境 install/关键旅程/数据持久化/清理（10/10）、跨 digest 升级回滚（14/14）与第三环境逻辑备份恢复（16/16，`scripts/dual-env-compose-drill.sh`，含离线构建镜像 `k8s-aiops-backend:v0.3.0-rc.5-local`）。
- `docs/changes/2026-08-12-m94-diagnosis-replay.md`：M94 回放模式——`BuildReplay` 纯函数按事件时间重放 M81 insight 链路（五阶段，只读不伪造）、`GET /diagnoses/:id/replay` 路由 + OpenAPI + replay-panel UI。
- `docs/changes/2026-08-12-m94-replay-demo-drill-and-offline-refresh.md`：重建含回放的 rc.5-replay 双镜像；demo-drill 回放断言（17/17）；offline-install 刷新离线包（10/10）。

- `docs/changes/2026-07-31-final-baseline-archive.md`：M32 最终本地基线、
  M27-M31 真实环境证据、响应式验收、完整门禁、清理不变量和外部门禁边界。
- `docs/changes/2026-07-30-m32-formal-closure.md`：M32 审计结论、缺陷修复和
  2026-07-31 最终复验结果。
- `docs/changes/2026-07-29-m21-authenticated-exact-series-history-api.md`：M21 第三阶段鉴权精确序列历史 API、稀疏覆盖、查询隔离和重启持久性验证。
- `docs/changes/2026-07-28-m21-bounded-background-metrics-collector.md`：M21 第二阶段已启用集群后台采集、Quantity 标准化、并发/超时隔离、稳定失败码与定时清理。
- `docs/changes/2026-07-28-m21-bounded-metrics-history-foundation.md`：M21 第一阶段 PostgreSQL 历史指标合同、采集覆盖、稀疏缺样本语义、容量上限和有界清理。
- `docs/changes/2026-07-28-product-roadmap-reprioritization.md`：基于 KRM/Ratel 对比关闭 M20，并重排 M21-M26 历史可观测、日常排障、发布回滚、跨集群发布、备份与组织集成路线。
- `docs/changes/2026-07-28-recovery-readiness-gate.md`：M20 第十二阶段生产恢复目标、策略一致性、真实逻辑恢复证据和 PITR/HA 实施准入边界。
- `docs/changes/2026-07-28-identity-readiness-gate.md`：M20 第十一阶段 OIDC/MFA 就绪合同、离线 discovery/JWKS 校验和降级拒绝边界。
- `docs/changes/2026-07-28-signed-audit-archives.md`：M20 第十阶段有界离线审计归档、Ed25519 签名、外部信任公钥验签和篡改拒绝。
- `docs/changes/2026-07-28-credential-key-reencryption.md`：M20 第九阶段版本化密钥环、离线批量再加密、整批回滚与脱敏实体验证。
- `docs/changes/2026-07-28-postgres-backup-restore.md`：M20 第八阶段隔离 PostgreSQL 备份、全新实例恢复、一致性校验与清理边界。
- `docs/changes/2026-07-28-dependency-governance.md`：M20 第七阶段依赖审查、major 更新隔离和 Node 24 Actions 治理。
- `docs/changes/2026-07-28-versioned-ci-release-pipeline.md`：M20 第六阶段分层 CI、版本发布打包、定时真实 kind 门禁与信任边界。
- `docs/changes/2026-07-27-two-cluster-global-search-e2e.md`：M20 第五阶段双真实 kind 集群固定资源搜索、故障隔离与完整清理证据。
- `docs/changes/2026-07-27-user-owned-global-search-filters.md`：M20 第四阶段私有保存筛选器、并发上限、兼容状态、审计与响应式交互。
- `docs/changes/2026-07-27-bounded-global-resource-search.md`：M20 第三阶段固定四类资源全局搜索、覆盖率、局部失败与深链接工作台。
- `docs/changes/2026-07-27-two-cluster-fleet-e2e.md`：M20 第二阶段双真实 kind 集群、超时/恢复/不可用隔离与完整清理证据。
- `docs/changes/2026-07-27-bounded-multi-cluster-health.md`：M20 第一阶段有界 fan-out、部分失败、采样覆盖与 Dashboard 集群比较。
- `docs/changes/2026-07-27-controlled-operations-catalog.md`：M19 固定操作目录、typed diff、最小 RBAC、真实 kind 恢复与 rollback 延后依据。
- `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`：M18 四条证据型规则、可重放 fixture、真实 kind 与工作台验收。
- `docs/changes/2026-07-27-common-workload-policy-coverage.md`：M17 九类常用工作负载/策略资源、Secret 安全边界、真实 kind 与响应式验收。
- `docs/changes/2026-07-27-real-metrics-utilization-consumers.md`：M16 可选 Metrics Server fixture、真实利用率、Pod 消费排行与 available-path 响应式验收。
- `docs/changes/2026-07-27-real-resource-metrics-foundation.md`：M15 固定 Node/Pod Metrics API、真实绝对用量、可选能力降级与响应式验收。
- `docs/changes/2026-07-27-complete-ingress-backend-topology.md`：M14 固定 EndpointSlice API、完整入口到工作负载拓扑与真实 kind 响应式验收。
- `docs/changes/2026-07-27-expanded-read-only-resource-workbench.md`：M13 四类扩展资源、安全字段裁剪、八类关联事件与诊断优先级回归修复。
- `docs/changes/2026-07-27-deep-link-resource-workbench.md`：M12 四类资源视图、固定详情 API、URL 深链接和统一详情抽屉。
- `docs/changes/2026-07-27-operations-cockpit-resource-topology.md`：M11 运维驾驶舱、selector 资源拓扑和控制台视觉升级。
- `docs/changes/2026-07-26-event-center-ui-unification.md`：M10 Kubernetes 事件中心、导航语义、Dashboard 共享壳层与响应式验收。
- `docs/changes/2026-07-26-release-freeze-candidate.md`：RC-Freeze 验证结果、演示状态、交付文档和提交前待确认事项。
- `.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json`：最新 M20 Phase 2 双真实集群脱敏证据，目录已加入 `.gitignore`。
- `.artifacts/search-e2e/search-e2e-20260727-225358.json`：M20 Phase 5 双真实集群全局搜索脱敏证据，八项清理断言通过。
- `.artifacts/postgres-recovery/postgres-recovery-20260728-131325.json`：M20 Phase 8 PostgreSQL 源实例销毁、全新实例恢复与清理断言的脱敏证据；托管 CI 为 run `30331048635`。
- `.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`：M20 Phase 9 dry-run、批次回滚、v1→v2 与 v2-only 解密的脱敏本地证据。
- `.artifacts/audit-archive/audit-archive-20260728-154047.json`：M20 Phase 10 两条合成审计记录签名/外部信任验签、超限无输出、字节篡改拒绝和五项清理断言的最终脱敏证据。
- `.artifacts/identity-readiness/identity-readiness-20260728-165405.json`：M20 Phase 11 无网络 OIDC/MFA 就绪检查、四类降级拒绝和完整清理的脱敏证据。
- `.artifacts/postgres-recovery/postgres-recovery-20260728-174419.json`：M20 Phase 12 使用的 16 迁移逻辑备份、新实例恢复、一致性与清理版本化证据。
- `.artifacts/recovery-readiness/recovery-readiness-20260728-174509.json`：M20 Phase 12 十五项恢复策略准入、降级拒绝和非生产声明证据。
- [hosted CI run 30348664880](https://github.com/guiyi-labs/aiops-platform/actions/runs/30348664880)：M20 Phase 12 四个 job、真实 PostgreSQL 恢复后的无网络恢复准入、随机 Compose、HTTP、脱敏上传和 teardown 通过记录。
- `.artifacts/verification/verify-20260728-153059.json`：M20 Phase 10 后端全包与三个二进制、前端、镜像、Compose、Kustomize 和 HTTP 完整本地门禁证据。
- [hosted CI run 30340088789](https://github.com/guiyi-labs/aiops-platform/actions/runs/30340088789)：M20 Phase 10 四个 job、三项隔离数据库演练、随机 Compose、HTTP、脱敏上传和 teardown 通过记录。
- [hosted CI run 30345051371](https://github.com/guiyi-labs/aiops-platform/actions/runs/30345051371)：M20 Phase 11 四个 job、无网络身份就绪门禁、三项隔离数据库演练、随机 Compose、HTTP、脱敏上传和 teardown 通过记录。
- `.artifacts/metrics-history-e2e/metrics-history-e2e-20260729-081759.json`：M21 Phase 3 真实 PostgreSQL 精确序列隔离、稀疏覆盖、重启持久性与清理证据。
- `.artifacts/verification/verify-20260729-082024.json`：M21 Phase 3 后端、前端、镜像、Compose、Kustomize 与 HTTP 完整本地门禁证据。
- [hosted CI run 30411146049](https://github.com/guiyi-labs/aiops-platform/actions/runs/30411146049)：M21 Phase 3 四个 job、精确序列历史隔离/重启演练、随机 Compose、HTTP、脱敏上传和 teardown 通过记录。
- `.artifacts/verification/verify-20260728-141111.json`：M20 Phase 9 后端、前端、镜像、Compose、Kustomize 与 HTTP 完整本地门禁证据。
- [hosted CI run 30334216631](https://github.com/guiyi-labs/aiops-platform/actions/runs/30334216631)：M20 Phase 9 四个托管 CI job、两项隔离 PostgreSQL 演练、脱敏上传和 teardown 通过记录。
- `.artifacts/verification/verify-20260728-100752.json`：M20 Phase 6 CI/release 流水线最终全量机器验证证据。
- `.artifacts/verification/verify-20260727-230204.json`：M20 Phase 5 最终全量机器验证证据。
- `.artifacts/verification/verify-20260727-194724.json`：最新 M20 Phase 2 全量机器验证证据。
- `.artifacts/verification/verify-20260727-210308.json`：M20 Phase 3 全局搜索最终机器验证证据。
- `.artifacts/verification/verify-20260727-222753.json`：M20 Phase 4 当前用户保存筛选器最终机器验证证据。
- `.artifacts/verification/verify-20260727-190133.json`：M20 Phase 1 全量机器验证证据。

文档状态建议使用：`Draft`、`Accepted`、`Superseded`。

当前交付入口：

- `docs/changes/2026-07-26-delivery-packaging.md`：M5 最终实现与验收记录。
- `docs/changes/2026-07-26-defense-demo-readiness.md`：M6 演示准备、清理、截图与最终回归记录。
- `docs/changes/2026-07-26-diagnosis-rule-expansion.md`：M7 Pending/OOMKilled 规则扩展、测试和范围边界。
- `docs/changes/2026-07-26-node-deployment-diagnosis.md`：M8 Node/Deployment 健康诊断、API/UI 扩展与验证证据。
- `docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`：M9 两条规则的独立真实 kind 验证、只读 RBAC 和完整清理证据。
- `docs/changes/2026-07-26-event-center-ui-unification.md`：M10 Kubernetes 实时事件中心和控制台壳层统一。
- `docs/changes/2026-07-27-operations-cockpit-resource-topology.md`：M11 实时集群态势、资源拓扑和视觉验收。
- `docs/changes/2026-07-27-deep-link-resource-workbench.md`：M12 分类资源工作台、深链接详情和响应式验收。
- `docs/changes/2026-07-27-expanded-read-only-resource-workbench.md`：M13 扩展资源、安全合同、关联事件和真实 kind 回归。
- `docs/changes/2026-07-27-complete-ingress-backend-topology.md`：M14 完整网络后端拓扑、空集合兼容和桌面/移动验收。
- `docs/changes/2026-07-27-real-resource-metrics-foundation.md`：M15 真实资源指标基础、显式不可用状态和最小 Metrics RBAC。
- `docs/changes/2026-07-27-real-metrics-utilization-consumers.md`：M16 Metrics available path、真实利用率和 Pod 消费排行。
- `docs/changes/2026-07-27-common-workload-policy-coverage.md`：M17 常用工作负载、弹性/配额资源与 Secret 键名安全合同。
- `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`：M18 Node/PVC/HPA/Ingress 证据型诊断与持续重启边界。
- `docs/changes/2026-07-27-controlled-operations-catalog.md`：M19 Deployment scale、CronJob suspend/resume 与固定操作安全合同。
- `docs/changes/2026-07-27-bounded-multi-cluster-health.md`：M20 有界集群健康比较、超时与部分失败合同。
- `docs/changes/2026-07-27-two-cluster-fleet-e2e.md`：M20 双独立 kind 集群 fan-out、故障隔离、恢复与清理验收。
- `docs/changes/2026-07-27-bounded-global-resource-search.md`：M20 固定 Pod/Deployment/Service/Ingress 全局名称搜索与工作台深链。
- `docs/changes/2026-07-27-user-owned-global-search-filters.md`：M20 当前用户私有搜索条件的保存、应用、维护与兼容迁移边界。
- `docs/changes/2026-07-27-two-cluster-global-search-e2e.md`：M20 双独立 kind 集群固定资源搜索、故障隔离、恢复与清理验收。
- `docs/thesis/README.md`：论文图表、测试矩阵、环境、许可证和答辩脚本索引。
- `scripts/verify.ps1`：一键质量门禁。
- `scripts/e2e-kind.ps1`：真实 kind 端到端验收。
- `scripts/e2e-diagnosis-kind.ps1`：一次性真实 kind Node/Deployment 只读诊断验收。
- `scripts/e2e-fleet-kind.ps1`：隔离平台与双真实 kind 集群 fleet 验收。
- `scripts/e2e-global-search-kind.ps1`：隔离平台与双真实 kind 集群全局搜索验收。
- `scripts/e2e-postgres-backup-restore.ps1`：隔离 PostgreSQL 17 逻辑备份、源实例销毁、全新实例恢复和一致性验收。
- `scripts/e2e-credential-reencryption.ps1`：隔离 PostgreSQL/backend 应用凭据密钥再加密、失败回滚和 v2-only 解密验收。
- `scripts/e2e-audit-archive.ps1`：隔离 PostgreSQL 审计归档签名、外部信任验签、超限无输出、篡改拒绝和完整清理验收。
- `.github/workflows/ci.yml`：无 PR 密钥的后端、前端、清单和 Compose 常规门禁。
- `.github/workflows/release.yml`：完整 CI 后的版本化打包与 tag 发布。
- `.github/workflows/real-kind-e2e.yml`：专用自托管 Windows runner 的周期/手动一次性 kind 门禁。
- `docs/ci-release.md`：流水线启用、分支保护、发布与 runner 运维手册。
