# Project Vision, Roadmap And Delivery Standards

- Updated: 2026-07-30
- Applies after: M21-M25 aligned baseline
- Detailed milestone state: `docs/roadmap.md`

## North Star

构建一个面向中小规模 Kubernetes 环境的“证据优先、边界明确、变更受控”的多集群
AIOps 平台。平台的差异化不在资源种类数量，而在于：操作员能从异常发现、证据诊断、
人工决策、受控执行、审计追踪直到恢复验证形成可信闭环。

北极星结果：

- 任何诊断结论都能追溯到结构化资源、事件、指标窗口或外部系统证据。
- 任何写操作都能说明“改什么、为什么、预检结果、谁确认、是否重复、最终状态”。
- 多集群和历史查询在失败、超时、截断、稀疏数据时诚实报告不确定性。
- AI 故障或关闭时核心平台仍可用；AI 不持有集群写权限。
- 每个发布候选可以在新设备从 Git 和受控依赖重建，而不依赖某台机器的隐藏状态。

## Product Boundaries

长期坚持：

- 固定领域合同优先于 Kubernetes 全量 CRUD。
- 确定性规则优先于不可解释模型推断。
- 服务端导出的 patch/manifest 优先于客户端提交任意内容。
- 最小权限、短期凭据和 Namespace 级写权限优先于共享 cluster-admin。
- PostgreSQL 保存平台状态和有界历史；实时 Kubernetes 对象默认从 API Server 查询。
- 局部失败优先于全局伪成功；缺样本保持缺失，不补零。

明确不追求：任意 GVK/YAML 编辑、无限终端/文件传输、Secret 值展示、通用 Prometheus
替代、自动 AI 修复、无预检的批量迁移、一键跨集群回滚、未设计 PV 冲突策略的恢复。

## Roadmap

### M26 — 可复现交付与组织准入（P0）

目标：让代码、运行环境、托管 CI、真实 kind runner、发布身份和恢复/身份决策形成正式
发布前闭环。

1. **M26A 新环境可复现接管**：使用本接管包在稳定设备从零克隆、生成新密钥、重建
   Compose 和依赖缓存，完成 L1-L3 验收并归档证据。
2. **M26B 专用 runner 与发布演练**：注册非生产 `aiops-kind` Windows runner，串行
   执行一次性套件；决定 registry、制品签名和 provenance 身份；演练语义版本发布。
3. **M26C 外部决策**：取得身份/安全/应用负责人对 OIDC/MFA 的命名批准，以及基础设施
   负责人对 PITR/HA、RPO/RTO、fencing、切换/回切的批准。准入报告通过不等于生产启用。

完成门槛：新设备验收、托管 CI、真实 kind、发布包校验和、runner 清理和决策记录均可
追溯；未取得批准的能力保持 fail-closed。

### M27 — 历史告警生命周期（P1）

目标：在 M21 精确序列和持续窗口评估之上，增加有界后台评估、去重、确认和关闭流程。

推荐分期：

1. 固定 Node CPU/内存规则配置和调度，不引入任意表达式语言。
2. 跨实例安全 claim、时间窗幂等、重复事件抑制和重启恢复。
3. 告警 open/acknowledged/resolved 生命周期、所有权、审计和通知 outbox。
4. Dashboard/事件中心消费、稀疏数据/超时/截断呈现。
5. 真实 Metrics API 故障、恢复、重复窗口和后端重启验收。

非目标：任意 PromQL、多指标图灵完备表达式、自动执行修复、Pod 高基数全量告警。

### M28 — 受控备份创建（P1）

目标：在 M25 只读 Velero 库存上增加固定范围的 Backup 创建，不开放 Restore。

必须先冻结：Namespace/include/exclude/TTL/storage location 的固定形状，能力和存储位置
预检，server-side dry-run（能力允许时）、一次性确认、幂等键、审计、状态轮询、超时和
失败码。不得从浏览器收集对象存储凭据。

Restore 必须是单独里程碑，先设计目标隔离、同名冲突、PV/CSI 行为、切换、回滚和数据
一致性证据；在此之前保持路由不存在。

### M29 — 正式发布、论文与演示绑定（P2）

目标：把一个语义版本 tag、源码、镜像、OpenAPI、依赖许可、测试矩阵、截图和演示脚本
绑定到同一 Git SHA。

包括：重新采集截图、更新架构图和数据口径、运行全量本地/托管/真实 kind 门禁、验证
SHA-256 和签名/provenance、清空演示残留并生成最终交接记录。

### 候选后续方向（M30+，未承诺）

- 固定 SLO/事件相关性视图，而非通用查询平台。
- 更细粒度的多租户/Namespace 授权与审计隔离。
- 经批准的 Restore 演练和跨集群灾难恢复。
- 指标/事件/变更证据的时间线关联和可解释 AI 摘要质量评估。

只有在 M26-M29 的安全和交付债务闭环后，才将候选方向提升为正式里程碑。

## Prioritization Standard

| 优先级 | 进入条件 | 示例 |
|---|---|---|
| P0 | 影响数据/凭据安全、现有正确性、可恢复性、CI 基线或新环境可复现性 | migration 错误、越权、证据泄漏、门禁红、环境无法重建 |
| P1 | 关闭高频操作员工作流，且依赖/风险已冻结 | 告警生命周期、受控备份创建 |
| P2 | 提升可用性、论文/演示、性能或维护体验，不改变安全边界 | 截图、文档、响应式细节、低风险缓存 |
| P3 | 探索性或竞争对齐，但用户价值/边界尚不明确 | 通用插件、任意表达式、批量迁移 |

同级排序按：用户频率 × 风险降低 × 证据可得性 ÷ 实施/运维复杂度。不能通过降低验证
标准来提高优先级。外部审批缺失的工作只推进到设计和 fail-closed 准入，不伪装完成。

## Milestone Proposal Standard

任何新 Mxx 进入开发前必须有一页提案，至少包含：

1. 问题、目标用户和当前可复现证据。
2. 可量化成功标准和明确非目标。
3. 固定请求/响应、状态机、稳定错误码和所有硬上限。
4. 数据模型、migration、保留/清理和失败恢复策略。
5. Kubernetes API、最小 RBAC、凭据寿命和敏感字段边界。
6. UI 操作路径、空/错/加载/截断/移动端状态。
7. 单元、HTTP、OpenAPI、前端、Compose、真实 kind 和故障注入计划。
8. 回滚/兼容策略、旧客户端和旧数据处理。
9. ADR、changes、handoff、test matrix 和 roadmap 更新项。
10. 外部依赖、负责人、批准者和不能由 Agent 自行决定的事项。

## Definition of Ready

里程碑只有同时满足以下条件才可进入实现：

- 用户价值、范围和非目标明确，和现有能力无重复。
- API/状态机/错误/上限/RBAC/数据敏感性边界已评审并冻结。
- 所需 upstream API 在目标版本可观察；不能依赖不可获得的证据做确定性结论。
- migration、兼容、失败恢复和清理路径可测试。
- 真实验收环境、固定镜像/manifest 和凭据来源已确定。
- 不需要的新权限、Secrets、云账号或生产资源已明确排除。
- 任务已拆为互不覆盖的代码边界；如并行开发，先冻结共享接口和文件所有权。

若任一关键外部决策缺失，状态为 `Gated`，不能标记 `In Progress` 或 `Accepted`。

## Delivery Sequence

每个里程碑遵循：

1. **Evidence/ADR**：复现问题，写 ADR 或提案，冻结合同。
2. **Domain first**：领域模型、纯逻辑、仓储事务和失败码，先写失败测试。
3. **Gateway/HTTP**：固定 Kubernetes/外部能力、鉴权、RBAC、OpenAPI 和路由合同。
4. **Frontend consumer**：只消费已接受 API；不把安全判断移到浏览器。
5. **Integration**：migration、Compose、Kustomize、CI 和现有工作流统一接线。
6. **Real acceptance**：一次性真实服务/集群、故障注入、重启/恢复、精确清理。
7. **Archive**：更新文档、证据矩阵、handoff、roadmap，审计差异和敏感材料。
8. **Baseline**：提交、annotated tag（仅正式基线）、推送并等待统一远端结果。

不得先做 UI 演示再反推不安全 API；不得把 fake client 成功当作真实集群接受。

## Verification By Change Risk

| 变更 | 最低门禁 |
|---|---|
| 纯文档 | `git diff --check`、链接/交付资产合同、托管 docs-only 分层结果 |
| 前端显示/类型 | typecheck、相关 Vitest、生产 build、API 类型/OpenAPI 核对 |
| 后端只读合同 | gofmt/vet、领域+HTTP 测试、OpenAPI parity、快速门禁 |
| migration/仓储 | 上述 + 真实 PostgreSQL apply/rollback/约束/重启证据 |
| Kubernetes 读取/RBAC | 上述 + fake/loopback + Kustomize + 一次性 kind |
| 受控写操作 | 上述 + dry-run/确认/幂等/前置条件/审计/最小 RBAC/恢复 fixture |
| CI/恢复/凭据 | 完整本地门禁 + 隔离故障演练 + 脱敏证据 + 托管 CI |

修复验收脚本缺陷时必须有合同测试；脚本失败若暴露产品问题，先修产品而不是绕过断言。

## Definition of Done

里程碑只有全部满足才可标记 Accepted：

- 实现与冻结合同一致，非目标仍不可达。
- 新旧测试通过且新增测试能复现关键失败；Go 格式和前端生产 build 通过。
- OpenAPI、前端类型、路由、错误码、migration 和 RBAC 同步。
- 真实环境验证成功，覆盖失败/超时/稀疏/重试/重启/清理等相关风险。
- 生成脱敏证据；无凭据、原始 manifest、数据库 payload 或用户数据进入产物。
- README、ADR、changes、handoff、test matrix、roadmap 更新。
- `git diff --check`、敏感信息扫描和 Git 完整性审计通过；工作树可解释。
- 已推送远端，必需 CI 和统一结果作业成功；外部审批状态如实记录。
- 给出下一里程碑、遗留风险和明确不承诺项。

“代码已写”“单测通过”“本机能打开页面”都不等于完成。

## Evidence Standard

本地机器证据写入 `.artifacts/<suite>/<suite>-YYYYMMDD-HHMMSS.json`，必须：

- 只含时间、版本、计数、状态、稳定错误码、哈希和清理断言。
- 不含 token、密码、kubeconfig、证书私钥、原始 Secret/ConfigMap 值、数据库行或日志正文。
- 记录 `cleanup_complete`，真实 kind 记录预存 `aiops-test` 是否保持。
- 由 `.gitignore` 排除；远端仅通过工作流的脱敏 artifact 上传。

变更归档引用证据路径和 CI URL，但不把本地证据文件提交。跨设备交接时，以 GitHub CI
为持久证据，新设备重新运行生成自己的本地证据。

## Git And Release Standard

- 开工前 fetch/prune，确认 ahead/behind 和 dirty worktree；保留未知用户改动。
- 日常提交使用聚焦的 Conventional Commit 风格；禁止把无关格式化或生成物混入。
- 禁止 force push `main`、`reset --hard`、重写已发布 migration/tag。
- annotated tag 只用于经过完整本地和远端门禁的命名基线/语义版本，不为每个文档提交打 tag。
- 推送前复查远端没有先行提交；推送后核对远端 SHA 和统一 CI 结果。
- 正式 release 必须绑定同一 Git SHA 的源码、镜像、OpenAPI、许可、元数据和校验和。

## Agent Exit Report

每次阶段收尾的最终报告按以下结构：

1. **Outcome**：完成了什么，哪些内容明确没做。
2. **Revision**：commit/tag/remote/CI URL。
3. **Verification**：实际运行命令、耗时、测试数、真实环境和证据路径。
4. **Safety**：RBAC、敏感信息、幂等、清理和数据迁移结论。
5. **Open risks**：外部审批、环境依赖、未覆盖故障和不承诺项。
6. **Next plan**：最多 3-5 个按 P0/P1/P2 排序、可独立验收的下一步。
