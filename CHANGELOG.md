# Changelog

All notable changes to the aiops-platform project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Milestones are released as git tags of the form `baseline-mNN-YYYYMMDD`.
Detailed change records for each milestone live under `docs/changes/`.

## [Unreleased]

### Added - M99D Data Visibility (coverage / latency)

- 关联案例信号链路携带数据元数据：`SignalLink` 新增 coverage/freshness/window_start/window_end，迁移 `000041` 落列，引擎在构建触发链接时拷贝（freshness 零值存 NULL）。
- 前端三处显式展示缺样本与数据延迟：SLO 评估卡片与历史表新增“数据覆盖”徽标 + “评估延迟”（`evaluated_at - window_end`）+ 无样本提示；信号列表新增覆盖度（含 title 解释）/时间窗口/数据延迟（`ingested_at - observed_at`）列；关联案例信号链路新增覆盖度徽标与时间窗口列及部分覆盖提示。
- `SignalCoverage` 类型补 `unavailable`/`truncated`，与后端取值对齐；无样本（fail-closed）与健康状态视觉分离。
- See [M99-D change record](docs/changes/2026-08-12-m99d-data-visibility.md).

### Added - M99C Correlation Input Provider & Worker

- 新增 correlation 生产输入源 `RepositoryInputProvider`：从 signal（active + lookback）、topology（有效边 + lookback 内变更）、diagnosis（lookback 内记录）仓储读取真实输入并映射为引擎类型（coverage/severity 透传、UID 缺失标 Incomplete、证据引用映射、每源读取上限）。
- 新增周期关联 worker：按 `CORRELATION_INTERVAL`（默认 5m）遍历启用集群、按命名空间自动跑 `CorrelateNamespace`，命名空间列表失败时回退全命名空间 pass；每集群超时与 ctx 取消安全。
- `diagnosis.ListFilter` 新增 `Since` 时间过滤（`observed_at >= ?`），向后兼容。
- 生产 `correlation.NewService` 换用真实 provider 并启动 worker goroutine，`/api/v1/aiops/correlation/cases` 可自动产生案例。
- See [M99-C change record](docs/changes/2026-08-12-m99c-correlation-provider-worker.md).

### Added - M99B Metricshistory SLO Source

- metricshistory 采集器新增 Deployment readiness 采样（`readiness_ready` / `readiness_total`），支持 Deployment 资源与 count 单位。
- 新增 `slo.MetricshistorySource`：`workload_readiness` 模板基于历史 readiness 计算滚动窗口就绪占比（累计计数器转换、缺配对样本如实丢弃）；request_* 模板如实返回 no-data。
- 生产接线：采集器接入 kubernetesService，SLO 求值器改用 metricshistory 源；Deployment 变更导致就绪率下降时可触发真实 burn 信号。
- 修复 `POST /aiops/slos` 返回 `id: 0`：gorm 创建后回写数据库生成的 ID。
- See [M99-B change record](docs/changes/2026-08-12-m99b-metricshistory-slo-source.md).

### Added - M99A SLO Burn Signal Pipeline

- 新增 SLO 错误预算燃烧信号化管道：`slo.burn.fast.v1` / `slo.burn.slow.v1` / `slo.burn.recovery.v1` 信号码，`SLOBurnSignalSink` 把 burn 状态转换规范化为 signal 事件（指纹去重、coverage 透传、`slo_burn_window` 证据哈希）。
- `BurnTransition` 携带数据覆盖率，缺失数据窗口在信号层显式可见。
- correlation 新增 `correlation.rollout_causes_slo_burn.v1` 规则（Deployment 变更→SLO burn），黄金回放场景 9→11（confirmed 与 contradicted 各一）。
- signal/slo/correlation 服务接入生产后端，`/api/v1/aiops/signals|slos|correlation/*` 路由由 404 变为可用。
- See [M99-A change record](docs/changes/2026-08-12-m99a-slo-burn-signal-pipeline.md).

### Added - M98 Incident Workspace

- 新增事故工作空间全链路：事故编号、负责人、关注者、显式状态机（确认/解决/驳回/重开）、SLA 截止与逾期、append-only 时间线（系统事件与人工备注分离）。
- 后端新增 `internal/incident` 领域模块（model/repository/service + 单测）、`000040_incidents` 迁移、REST 处理器与诊断来源解析；写操作限定 `system_ops_admin` 角色。
- 更新 OpenAPI 契约与 typegen；前端新增 `/incidents` 路由、侧栏入口、IncidentsView（汇总面板/列表/详情抽屉/新建表单）与 CSV 导出。
- Playwright e2e 覆盖列表、创建校验、完整工作流（确认/移交/备注/解决/复盘/重开）与 viewer 无权限路径，Desktop + Mobile 共 8/8 通过。
- See [M98 incident workspace record](docs/changes/2026-08-12-m98-incident-workspace.md).

### Fixed - M98 Incident Workspace

- 修复时间线系统事件 `actor_user_id=0` 违反外键；`ID<=0` 时写入 NULL。
- 修复 Transition/Assign/SetPostmortem 版本条件参数顺序与 Transition 未递增 version，消除偶发 409 与版本漂移。
- 错误码 `USER_NOT_FOUND` 改为 `INCIDENT_USER_NOT_FOUND`，恢复错误码审计唯一性。
- 修复 CSV 导出按 `Limit:1` 导出最后一行的问题；改为按 ID 定位后导出单条。
- 前端 Dockerfile 的 `pnpm install` 增加 `--network-concurrency 4 --fetch-retries 5`，降低镜像构建网络抖动。
- 移动端适配：incident 表格横向滚动容器 + drawer/form 全宽 + action-row 纵向堆叠，修复窄屏下 fixed overlay 拦截点击。

### Fixed - M97 Hosted CI Recovery

- 修复发布工作流顺序测试的静态检查表达式，并删除规模 fixture 校验器中遗留的未使用辅助函数，使 Backend `golangci-lint` 恢复通过。
- 修正 M96 Pod 虚拟列表滚动高度不变量的跨平台判定：以末行起点可覆盖为硬边界，避免 Chromium 表格布局的亚行级高度差导致 CI 假失败，同时保留窗口、滚动、筛选与 console 硬检查。
- 修复 RC 发布工作流中不存在的 Docker QEMU/Buildx action SHA，并让 Release 调用强制执行完整运行时质量门禁。
- Hosted Release 改为从四个单平台 OCI 输入生成 backend/frontend SPDX SBOM，规避 Syft 无法直接解析多架构 OCI index 的限制，同时保留双架构发布归档。
- `v0.3.0-rc.4` 的 Hosted Release 全部通过并发布非草稿 prerelease；20 个发布资产包含 16 个 checksum 覆盖的 payload、四份平台 SBOM 和完整 Cosign 签名证据。
- CI 文档快速路径现在包含 `CHANGELOG.md`，并在文档范围下跳过 Backend 与全部运行时任务；最终汇总对 `true`/`false` 范围均保持 fail-closed，Release 的 `force_runtime: true` 不受影响。
- 合并 Backend 的普通测试与全局覆盖率测试，消除对全部 Go package 的一次重复执行，同时保留 60% 全局覆盖率、核心包覆盖率、race、fuzz 和 benchmark 门禁。
- CI 新增一次性的共享 Backend 镜像构建与短期 artifact；四项演练和 Compose runtime 复用该镜像，Compose 仅构建 frontend，避免重复执行 Backend Docker build。
- Race detector gate 从串行 (-p=1) 切换到 2 路并行 (-p=2)；远端 A/B 实验 (run 31413981487) 显示墙钟由 6m14s 降至 4m6s (~33%)，未出现 OOM 或竞态漏检。
- GitHub Actions artifact 下载及 Release 的 QEMU/Buildx setup 升级到固定 SHA 的 Node.js 24 版本，清除 hosted runner 的 Node.js 20 弃用注解。
- 修复 `TestJWKECPublicKey` 偶发失败：测试辅助 `ecJWK` 现将 P-256 X/Y 坐标填充至固定 32 字节，避免前导零字节导致生产解析器拒绝签名密钥。
- 修复 Release 演练在 `workflow_dispatch` 下 Cosign keyless 身份不匹配：发布清单现在记录实际触发 ref（tag push 为 `refs/tags/<version>`，手动演练为分支 ref），签名与严格校验使用同一身份。
- See [M97 hosted CI recovery record](docs/changes/2026-08-10-m97-hosted-ci-recovery.md).

### Added - M97 Release Candidate Supply-Chain Closure

- 新增 `aiops.release-manifest/v1` 统一发布清单，绑定 RC tag、完整 revision、OCI archive index digest、Helm、Kustomize、离线包、SBOM、provenance、SHA256SUMS 与验证命令。
- 发布包新增双架构 OCI 镜像、镜像 SBOM、Kustomize/Helm/离线部署资产和内部 `OFFLINE-SHA256SUMS`；离线包不包含 Secret 值。
- 严格包为每个目标平台生成单平台 OCI SBOM 输入，兼容 Syft 对多架构 OCI index 的解析边界，同时保留最终双架构 OCI archive。
- 发布校验改为严格检查最终 checksum root、provenance subject、镜像平台和签名状态；修复签名后重写 `SHA256SUMS` 的一致性缺陷。
- 修复发布证据的工具版本采集提前关闭原生命令管道、误报 `error:-1` 的问题。
- OCI 清单检查器递归解析 Buildx 外层与内层 index，校验 blob digest、排除 attestation manifest，并从真实镜像 manifest 收集 amd64/arm64 平台。
- 新增 kind 生命周期演练入口，覆盖全新环境安装、升级、回滚、健康检查、认证和清理；数据库迁移回滚边界保持显式。
- 修复 Helm 升级演练丢失首次安装值的问题；演练端口改为动态分配，失败路径也会写入脱敏结构化证据；前端静态资源构建阶段固定使用宿主架构，解除本地 arm64 OCI 构建卡死。
- 远端 GitHub Release 与 keyless Cosign 已取得 `v0.3.0-rc.4` 证据；M89/M90 仍未完成，因此版本保持 RC，不宣称 GA 或 production-ready。
- See [M97 release candidate change record](docs/changes/2026-08-10-m97-release-candidate-closure.md).

### Added - M96 Deterministic Scale Fixture

- 新增版本化 `m96-v1` 规模 fixture 配置和 `scale-fixture` CLI，流式生成 500 Node、50,000 Pod、100,000 Event，并覆盖 workload、topology、global search 与 metrics history 映射。
- 生成器输出确定性 gzip NDJSON 和带计数、字节数、SHA-256、配置哈希及覆盖范围的 manifest；校验器逐条读取并拒绝缺失、篡改、额外文件或计数漂移。
- 新增 `scale-bench` fixture-backed 基准器，记录拓扑派生、全局搜索、Pod/Event 分页、历史查询/评估和背压流的 P50/P95/P99、heap、goroutine、取消与超时行为。
- CI 以 report 模式生成、校验和基准完整 fixture，仅上传 manifest 与结构化报告，不把大数据集提交到仓库；性能阈值尚未 fail-closed。
- See [M96 scale fixture change record](docs/changes/2026-08-10-m96-scale-fixture.md).
- See [M96 backend scale benchmark record](docs/changes/2026-08-10-m96-backend-scale-benchmark.md).

### Added — M96 Frontend Scale Budget

- WorkloadsView now preserves a bounded virtual Pod window with top/bottom
  spacers, mounted viewport measurement and stable scroll geometry at 50,000
  Pods; name filtering reuses the unfiltered response array and retains object
  identity for matched rows.
- Added deterministic `m96-pods-v1` fixture generation and production-preview
  Playwright sampling for desktop/mobile first render, filtering, scrolling,
  long tasks, DOM nodes, heap and virtual-window bounds.
- CI retains the structured M96 Pod scale report in report mode; exact fixture,
  DOM, scroll, filter and console invariants are hard checks.
- See [M96 frontend scale budget record](docs/changes/2026-08-10-m96-frontend-scale-budget.md).

### Changed — M96 Authenticated Console Shell

- Authenticated routes now share one stable sidebar/topbar shell; view title
  metadata and named actions update through a page bridge while login remains
  shell-free.
- Added desktop/mobile assertions for one shell, route title replacement and
  persisted sidebar collapse; removed the unreferenced `kubesphere-theme.css`
  layer and added a report-mode active CSS layer audit.
- See [M96 console shell convergence record](docs/changes/2026-08-10-m96-console-shell-convergence.md).

### Added — M96 Gate B Evidence Aggregation

- Added a versioned report-mode Gate B aggregator that binds the `m96-v1`
  fixture, backend scale report, frontend 50k Pod samples and active CSS audit
  to their hashes, commit metadata and hard invariants.
- CI now downloads the independent M96 artifacts into a clean Gate B job and
  retains the combined JSON/Markdown report; hosted thresholds remain report-only.
- See [M96 Gate B evidence aggregation record](docs/changes/2026-08-10-m96-gate-b-evidence-aggregation.md).

### Added — M95 Frontend Finding Evidence Surface

- Posture、Optimization（11 个 analyzer tab）、Diagnoses 和 Inspection 统一使用 `FindingDetailV2` 前端适配器与可折叠证据链组件，展示规则来源、证据引用、缺失语义和建议能力。
- 同资源 finding 合并后保留完整 `origin_ids` 与去重后的证据引用；Posture 不再将合并结果重新降级为 legacy finding。
- 新增 Desktop/Mobile 真实响应 Playwright fixture，覆盖面板展开、建议展示和移动端视口边界；修正优化页移动端 tab/表格横向滚动约束。
- See [M95 frontend change record](docs/changes/2026-08-10-m95-finding-evidence-ui.md).

### Added — M95 Unified Finding & Evidence Model (FindingDetail v2)

- 后端定义 `FindingDetail v2` 统一证据模型：规则身份（rule_id/framework/source/version）、
  资源引用、严重度、稳定证据引用、类型化建议与版本信息；内嵌并保留全部 v1 Finding 字段。
- 提供 v1→v2 兼容层（`FromV1` / `ToV1`）：v2 → v1 扁平化与原始 v1 JSON 字节一致，
  旧 API 消费者继续工作；同一资源的重复 finding 经 `MergeDistinct` 合并展示并保留每条
  规则来源与原始 ID。
- 建议类型 `advisory` / `controlled_action_available` / `manual_only`，默认不自动执行；
  共享严重度映射 `SeverityRank` / `NormalizeSeverity` / `MaxSeverity` 统一 posture /
  optimization / diagnosis / inspection。
- golden `DatasetVersion` 1.1 → 1.2；`DatasetMigrationHint` 保证旧快照（v1.0/v1.1）
  仍可读取并给出迁移提示。
- 新增 schema-parity 与序列化快照测试：11 个 posture 分析器 + finops 均使用规范
  `finding.Finding`；v1 JSON wire contract 字节级锁定。
- See [M95 change record](docs/changes/2026-08-10-m95-finding-detail-v2.md).

### Added — M94 Diagnosis Deep Links

- 诊断详情抽屉新增“关联入口”：从诊断直达资源详情（Pod/Service/Node/Deployment/Ingress/PVC）、
  工作负载与相关事件（/workloads 查询路由）、审计记录（security_auditor/system_admin）。
- 纯只读导航，不新增写路径；HorizontalPodAutoscaler 等不支持类型仅提供工作负载入口。
- Playwright 新增深链旅程（Desktop+Mobile 4/4），浏览器回归 50/50。
- See [M94 deep links change record](docs/changes/2026-08-10-m94-diagnosis-deep-links.md).

### Added — M94 Diagnosis Action Area

- 诊断详情新增类型化“行动区”：每条 recommendation 标记为只读建议（advisory）；
  Pod 资源追加 rollout restart 受控动作（controlled_action，必须 dry-run + 显式确认）。
- 无权限 / 非 confirmed 等不可用原因在行动区显式降级提示；受控修复表单仍只在
  canManage && confirmed && Pod 时出现，不绕过确认。
- OpenAPI 新增 `DiagnosisAction` schema；`pnpm typegen` 重新生成；`DiagnosisRecord` 增加 `actions?`。
- Playwright 新增行动区旅程（受控动作 + 依赖降级，Desktop+Mobile 2/2），浏览器回归 46/46。
- See [M94 action area change record](docs/changes/2026-08-10-m94-diagnosis-action-area.md).

### Added — M94 Diagnosis Narrative (Root Cause Card & Evidence Timeline)

- 新增诊断根因卡（结论、严重度、状态、首次观察、置信来源、≤5 条关键证据引用）与只读证据时间线：
  六类证据分类（resource_state/event/log/alert/change/automation）、按类型提取 occurred_at、
  SHA-256 完整性、缺失语义（status=Missing 传播、不可解析时间回退 observedAt）。
- 后端 `WithNarrative` 纯投影经 `save()` 与 `Get/Transition/AddFeedback/Assign` 统一返回；
  四条黄金场景（Node NotReady / Deployment unavailable / OOMKilled / Service 无后端）单测覆盖。
- OpenAPI 新增 `DiagnosisTimelineEntry`、`RootCauseCard` schema；`pnpm typegen` 重新生成；
  `DiagnosisRecord` 增加 `timeline?` / `root_cause_card?`。
- 诊断详情抽屉顶部渲染根因卡，证据时间线替换默认原始 JSON；原始证据收进可折叠区块，
  未携带 timeline 的旧响应保留原证据卡回退。
- Playwright 新增诊断旅程（Desktop+Mobile 2/2），浏览器回归 44/44。
- See [M94 change record](docs/changes/2026-08-10-m94-diagnosis-narrative.md).

### Added — M93-B2 Login Performance Budget & Evidence

- 新增登录页专属体积分析（LoginView chunk 14.75 kB raw / 5.62 kB gzip）+ 三模式
  （desktop / mobile / reduced-motion）Playwright 采样脚本、报告模式基线 JSON 与 Markdown 报告。
- 采样捕获 FCP/LCP、long-task、rAF 帧率、交互延迟、Canvas DPR/粒子数、隐藏暂停/恢复
  与 console error；产出版本化基线 login-baseline-v1.json。
- CI frontend job 新增 Playwright chromium 安装 + pnpm perf:login + 保留 7 天 artifact；
  预算为报告模式，不阻塞 CI，连续稳定后升级 fail-closed。
- See [M93-B2 change record](docs/changes/2026-08-10-m93b2-login-perf-budget.md).

### Documentation — Post-M93-C Program Plan

- 将下一阶段执行入口扩展为 M93-B2–M102 的 12–16 周计划，加入依赖图、Gate A–D、并行轨道、
  前十个工作日任务板、风险登记与 GA 准入。
- M89 OIDC/MFA 与 M90 WAL/PITR/HA 保持组织授权轨；未形成真实演练证据时版本保持 RC。
- See [Post-M93-C program plan record](docs/changes/2026-08-10-post-m93c-program-plan.md).

### Changed — M93-C Technology Console Shell

- 登录页与登录后控制台统一为碳黑、冷灰、青色与状态色组成的工业科技主题，移除偏紫粒子色调。
- 左侧功能栏支持桌面图标栏收起、移动端完全隐藏、刷新持久化和无障碍状态提示。
- 移除整页路由淡出，增加永久技术底板并取消卡片透明度入场，消除功能切换时的白屏闪烁。
- Playwright 新增侧栏持久化与路由连续着色回归；双视口 smoke + axe 提升到 42/42 全绿。
- See [M93-C change record](docs/changes/2026-08-09-m93c-tech-console-shell.md).

### Changed — M93-B1.1 Fullscreen Login Refinement

- 登录页由左右明暗分栏改为全视口控制平面场景，粒子、网格与拓扑覆盖整个背景，表单作为右侧悬浮操作区。
- 表单统一为暗色玻璃层，补齐输入、自动填充、密码显隐、错误/成功、按钮与安全状态的暗色对比度细节。
- 移动端保持全屏粒子背景，隐藏拓扑/能力卡并将完整表单控制在首屏；新增全屏尺寸和首屏边界回归。
- See [M93-B1.1 change record](docs/changes/2026-08-09-m93b1-fullscreen-login-refinement.md).

### Changed — M93-B1 Control-Plane Login Motion

- 登录页新增用户名焦点、密码焦点、认证中、成功与失败五种状态驱动的拓扑/表单联动动画。
- ParticleNetwork 接入 ResizeObserver、Page Visibility、动态 reduced-motion 与移动端粒子/DPR 降级。
- 修复 SVG 节点呼吸动画覆盖 translate 坐标导致外围节点叠在左上角的问题；补充密码显隐和成功转场。
- Playwright 新增 Canvas 非空像素、动态 reduced-motion、隐藏暂停、容器 resize 与认证成功转场回归。
- See [M93-B1 change record](docs/changes/2026-08-09-m93b1-control-plane-login-motion.md).

### Fixed — M93-A Login Data Truth & Deterministic E2E

- 移除登录页未经证实的 12 / 186 / 99 “实时”指标，改为不泄露资源数量的核心能力卡。
- Playwright 新增确定性认证/API fixture，登录页匿名路径与受保护页面不再依赖真实后端状态。
- 修复 Dashboard 三处 WCAG AA 对比度回归；双视口 smoke + axe 28/28 全绿。
- See [M93-A change record](docs/changes/2026-08-09-m93-login-data-truth.md).

### Added — M92 Interactive Login Visual System

- 新增 Canvas2D 粒子网络、SVG 多集群拓扑、能力卡与分层动效；支持触屏、2x DPR、
  `prefers-reduced-motion` 静态降级和移动端语义化隐藏。
- 认证流程与安全契约保持不变；M92 的静态展示数字已在 M93-A 移除。
- See [M92 change record](docs/changes/2026-08-09-m92-interactive-login-visual.md).

### Added — 归档体系（强制所有修改留档）

- 新增根目录 AGENTS.md：所有代码/文档改动必须先写 change-record 才能提交，附提交前归档完整性检查表。
- 新增 docs/ARCHIVING.md：四层归档体系（change-record / CHANGELOG / ADR / baseline tag）+ 提交与标签流程。
- 新增 docs/changes/TEMPLATE.md：change-record 标准模板。
- 还原 3 处未归档、来源不明的 UI 动效回退改动至已交付基线。
- See [archive change record](docs/changes/2026-08-09-archive-system.md).

### Fixed — 远端 CI：同步 main 修复达 7 条 dependabot PR 的 Typecheck 失败

- 根因是 PR 基于旧 base，缺 W12 类型契约修复；合并 main 后全部转绿、mergeStateStatus=CLEAN，未改依赖版本。
- See [CI recovery change record](docs/changes/2026-08-09-ci-dependabot-recovery.md).

### Added — M86 W10 Contract & API Governance

- OpenAPI 契约修复（重复 schema、缺失 schemas 与参数）+ pnpm typegen + CI sync gate；
  insight.ts 消费生成类型；全路由错误码审计归一化 VELERO_UNAVAILABLE → 503。
- See [W10 change record](docs/changes/2026-08-09-w10-openapi-typesync.md).

### Added — M87 W11 UX & A11Y & Bundle Gate

- premium-ui.css 微交互层；Playwright + axe（WCAG 2A/2AA/2.1AA）双视口 0 critical/serious；
  修复 27 类低对比度文字色；新增 pnpm bundle:gate + CI 门禁。
- See [W11 change record](docs/changes/2026-08-09-w11-ux-a11y-bundle.md).

### Added — M88 Release Loop Localized + fail-closed signing

- 新增 scripts/release-verify.ps1：语义化版本校验 + 发布包组装 + SHA256SUMS 自校验；
  cosign 存在时真实 sign-blob/verify-blob，否则 SIGNING_SKIPPED 哨兵；
  release.yml cosign attest-blob 由 fail-open 改为 fail-closed，发布前签名门禁。
- See [M88 change record](docs/changes/2026-08-09-m88-release-loop.md).

### Added — W12 Real-Cluster kind E2E Evidence

- 修复前端构建契约缺陷（openapi.ts OperationResponse 泛型、insight.ts 契约形态）；
  e2e-diagnosis-kind/fleet-kind/global-search-kind 三套真实集群 E2E 全部通过；
  证据在 .artifacts/<suite>-e2e/。
- See [W12 change record](docs/changes/2026-08-09-w12-real-cluster-e2e.md).

### Added — M91 Windowed Virtual Scroll

- 新增 useVirtualList（computeWindow 纯函数 + rAF 节流 + overscan）+ 6 条 Vitest；
  WorkloadsView Pod 表窗口化渲染 + sticky 表头；数值为 23 files / 130 tests 全绿。
- See [M91 change record](docs/changes/2026-08-09-m91-virtual-scroll.md).

### Added — M81 AIOps Closed-Loop Runbook (W5)

- **End-to-end insight runbook**: 优化中心 findings → 巡检佐证（M52）→ 确定性
  诊断（M43）→ AI 引用解释（M55）→ dry-run 预览（M19），一条链路可点击、可回放、
  只读不扩安全边界；只读 insight runbook 端点。
- See [M81 change record](docs/changes/2026-08-09-m81-insight-loop.md).

### Added — M82 Golden Analyzer-Discovery Contract (W6)

- 黄金回放新增 `analyzer_discovery` 场景：posture/insight/diagnosis/inspection
  快照纳入 M56 黄金回放 + 质量报告；`DatasetVersion` → 1.1。
- See [M82 change record](docs/changes/2026-08-09-m82-analyzer-discovery.md).

### Added — M83 Topology Deepening (W7)

- Gateway API（Gateway/HTTPRoute）只读接入拓扑 + collapse 折叠/聚合参数；
  新增 ADR 0080 与拓扑深化测试。500 节点 fixture 渲染验证依赖真实集群环境。
- See [M83 change record](docs/changes/2026-08-09-m83-topology-deepening.md).

### Added — M80 Aggregated Governance Posture + UI Motion Baseline (W4)

- 聚合治理态势视图（posture）；count-up 指标滚动、aurora 登录背景、premium
  motion 层（useCountUp + Vitest 覆盖）。

### Added — M85 W8/W9 Closure: Coverage 60% Gate + Playwright E2E + Unified Motion

- 全局覆盖率 59.1% → 60.03%，CI 全局覆盖率门禁 50% → 60%（.github/workflows/ci.yml）；
  新增 8 个低覆盖包纯逻辑测试（automation/auth/alert/alertroute/authz/workspace/cluster/insight）；
  修复 correlation `EvidenceRef` 测试字段（RefID → ID）。
- Playwright 浏览器 E2E：7 条关键链路 × 双视口（Desktop 1280×720 / Mobile 390×844）
  全绿、console error=0（frontend/e2e/smoke.spec.ts）；新增 `frontend/src/styles/motion.css`
  统一微交互动效层、`SkeletonCard.vue` 骨架屏、`EmptyState.vue` 空态。
- 文档收口：README/PROJECT_STATUS/long-term-roadmap/polish-plan 对齐真实 HEAD；
  补齐 M61–M66、M74、M75 缺失 change-record；新增 W8/W9 change-record。
- See [W8 record](docs/changes/2026-08-09-w8-coverage-closure.md) /
  [W9 record](docs/changes/2026-08-09-w9-playwright-e2e.md).
### Added — M84 Test Intensity Upgrade (fuzz targets + benchmarks + core coverage gate)

- **New seed fuzz targets** across the pure parsers/validators behind the
  analyzers (CI runs them as deterministic seed smoke via `go test -run '^Fuzz'`):
  `metricshistory` quantity parsing, `deprecatedapi` apiVersion+catalog lookup,
  `apiquery` list-query contract, `netpolicy` port parser, `namespaceposture`
  quota ratio, `optimization` collector CPU/mem parse, `kubernetes` rollback
  revision, `posture` severity ordering, `topology` evidence/plan-ID hash.
- **New benchmarks** (first-time baseline, trend-tracked in docs):
  `metricshistory.EvaluateWindow` (~15µs), `topology.SortEdges` 500 edges
  (~578µs), `posture` aggregate severity sort (~307µs), `capability.Registry.List`
  (~6µs).
- **CI core-package coverage gate (≥70%)** for `metricshistory`, `apiquery`,
  `deprecatedapi`, `optimization` (currently 79/100/93/76%) + a fuzz/benchmark
  smoke step. The existing global ≥50% baseline is retained; the M84 "global
  ≥60%" delta is tracked as a follow-up incremental lift in `docs/polish-plan.md`.
- See [M84 change record](docs/changes/2026-08-09-m84-test-intensity.md).


### Added — M59 Signed Releases + SLSA Provenance (Structural Placeholders)

- **Cosign keyless signing** in `.github/workflows/release.yml` `package`
  job: installs `sigstore/cosign-installer@v3.7.0` (Cosign v2.4.1) and
  keylessly signs both `SHA256SUMS` and `release-metadata.json` via
  `cosign sign-blob --tlog-upload=true`. The Fulcio x509 certificate
  (`*.cert.pem`) and Rekor signature (`*.sig`) are bundled with the
  release artifacts.
- **Aggregate bundle digest** step output (`signed_digest`) =
  sha256(sha256(all files in the release folder)) → downstream gates can
  verify one digest that covers the *signed* artifacts, signatures,
  and provenance as a unit.
- **Structural SLSA v1 in-toto provenance placeholder**: generates an
  in-toto statement whose subject is the signed bundle digest. The
  predicate layout (`buildType`, `externalParameters`,
  `runDetails.builder.id`) mirrors the upstream
  `slsa-framework/slsa-github-generator` generic v3 builder so
  maintainers can swap it in without changing the subject format.
- **Cosign attest-blob structural placeholder**: binds the in-toto
  provenance statement to the bundle subject with its own sig + cert
  pair. Currently `|| true` fail-open (so rehearsal runs without Rekor
  access still succeed); documented in ADR 0075 as fail-closed once the
  real SLSA generator is wired in.
- HA / PITR are *not* implemented in M59; ADR 0075 reserves the
  workflow identity slot so they can reuse the same keyless signing
  identity when delivered.
- Reference: [ADR 0075 §4](docs/adr/0075-m59-signing-provenance-m60-provider-registry.md)
  and the [M59+M60 change record](docs/changes/2026-08-02-m59-m60-signing-and-provider-registry.md).

### Added — M60 Compile-Time Provider Registry + Lifecycle/Health/Role Selectors

- **New package `backend/internal/capability/registry.go`**:
  - `Registry` struct (RWMutex-protected) + `ProviderDescriptor`
    (name, kind, description, dependencies, cluster_roles, configured,
    optional Lifecycle, optional HealthChecker).
  - `Register(desc ProviderDescriptor)` — lexical name validation
    (`[a-z][a-z0-9_-]{1,63}`), `ErrProviderAlreadyRegistered` on
    duplicates, `ErrInvalidProviderName` otherwise. Slices are copied so
    callers cannot mutate descriptors after registration.
  - `StartAll(ctx)` / `StopAll(ctx)` — three-color DFS topological sort
    with explicit `ErrCyclicDependency` detection before any start.
    Start order = dependencies before dependents; stop order is the
    *reverse*, so `inspection_scheduler` always shuts down before
    `metrics_prometheus`. Missing dependencies degrade dependents to
    `state = degraded` with a reason instead of crashing startup.
  - `List()`, `Get(name)`, `CheckHealth(ctx, name)` —
    `ProviderInfo` projects directly to the new OpenAPI `ProviderInfo`
    schema; 1s health cache (GUI drives refresh via `?refresh=true`);
    non-configured / role-disabled providers never probe.
  - Panic-safe wrappers: `safeStart`, `safeStop`, `safeCheckHealth`
    (panic → state = unhealthy, not a platform crash).
  - Sentinal errors exported for callers to errors.Is against.
- **Unit tests** (`backend/internal/capability/registry_test.go`):
  12 table tests covering duplicate/invalid-name registration, cycle
  detection, missing dep late-registration, start/stop order on a
  4-node diamond graph (a←b,c←d), cluster role gating (member-only
  role set), configured+health+1s cache behavior, not-found /
  miss paths, ctx-aware stop timeout, IsEnabled / ClusterSelector /
  Dependencies helpers. Statement coverage **84.2 %**.
- **HTTP surface** — 2 new routes protected by `system_ops_admin`:
  - `GET /api/v1/capability/providers` → `listProviders`
    (registry.List(), alphabetic order).
  - `GET /api/v1/capability/providers/{name}?refresh=true|false` →
    `getProvider` (registry.Get or CheckHealth on refresh=true).
  Non-nil guard in `capabilityRoutes` mirrors the existing capability
  providers guard; nil registry returns 503.
- **OpenAPI contract**: `docs/api/openapi.yaml` new `ProviderInfo`
  schema + two new path entries. `TestRegisteredRoutesMatchOpenAPI`
  continues to pass.
- **cmd/server wiring** (`cmd/server/main.go`):
  - `capability.NewRegistry({standalone, host, member}, 5s)` after
    capability provider configuration.
  - 10-entry stable catalog registered: `metrics_prometheus`,
    `logs_loki`, `federation`, `inspection_scheduler`,
    `service_mesh_readonly`, `gitops_argocd`, `copyops_cross_cluster`,
    `app_catalog_helm`, `backup_restore_velero`, `ai_investigator`
    (with correct dependency edges: inspection/mesh → metrics,
    copyops → federation).
  - `StartAll(backgroundContext)` runs after the 3 existing background
    goroutines (notification, metrics collector, alert scheduler).
    Start errors → Warn-level log only (never fatal; partial startup is
    acceptable).
  - `StopAll(shutdownCtx)` runs *before* `server.Shutdown` so
    dependents (inspection, mesh) stop before their dependency
    providers tear down. Stop errors aggregated into single Warn log.
  - `Options.CapabilityRegistry` injected so the route guard enables
    the new handler paths.
- **Route contract test wiring**: new `mustProviderRegistryForContract(t)`
  helper in `internal/httpserver/openapi_route_test.go` builds the
  10-entry minimal catalog and injects it into the Options so
  `/capability/providers` routes are covered by the route↔OpenAPI
  match test.
- Reference: [ADR 0075 §1–3](docs/adr/0075-m59-signing-provenance-m60-provider-registry.md).

### Added — M61/M62/M63 Optimization Analyzers (FinOps Right-sizing, CIS Posture, Deprecated API)

- **M61 FinOps Right-sizing Advisor** (`backend/internal/finops`): read-only
  right-sizing + cost-waste advisor that turns already-collected M21 metrics
  (CPU/memory in nanocores/bytes) into suggested requests/limits (p95 × headroom,
  rounded) and a monthly dollar waste estimate. Pure `Recommend` function,
  configurable `CostRate`, in-memory `Repository`, `QuantityFromResourceMap`
  bridge for Kubernetes resource strings. See ADR 0077.
- **M62 CIS Kubernetes Compliance Posture** (`backend/internal/cis`): read-only
  CIS Kubernetes Benchmark posture check (kube-bench / Kubescape style) over a
  compiled-in control catalog across four domains — component flag controls (26:
  kube-apiserver / scheduler / controller-manager / etcd / kubelet, CIS
  1.2/1.3/1.4/1.5/4.2), workload security (6: privileged, privilege escalation,
  run-as-non-root, host namespace, hostPath, CAP_NET_RAW), RBAC (2: cluster-admin
  to non-system subject, wildcard role), namespace Pod Security Admission (1:
  enforce not privileged/unset). Pure `Evaluate(clusterID, Inputs, observedAt)`
  function emits `internal/finding`-shaped findings with rationale + remediation
  references. See ADR 0078.
- **M63 Deprecated / Removed API Check** (`backend/internal/deprecatedapi`):
  read-only analyzer flagging objects using deprecated or removed `apiVersion`s
  relative to a target Kubernetes minor version. Compiled-in catalog
  (pluto/kubent-style), severity `removed` (critical) / `deprecated` (warning),
  emits `internal/finding`-shaped findings for uniform rendering. See ADR 0076.
- **`internal/finding`**: dependency-free canonical read-only finding contract
  (mirrors `namespaceposture.Finding` JSON shape) so all analyzers render
  uniformly without pulling in the heavier `cluster`/`kubernetes` graph.
- All three analyzers are pure functions over already-fetched data and mutate
  nothing, per the read-only posture of ADR 0004. The service layer that builds
  the input structs from the live Kubernetes API / `metricshistory` store is
  intentionally deferred to the API route to avoid colliding with the M46–M60
  frontend work; unit tests cover every control shape today.
- ADRs: 0076 (deprecated API check), 0077 (FinOps right-sizing), 0078 (CIS
  compliance posture).

### Added — M64 Optimization Analyzers HTTP API (wire M61–M63 into the server)

- **`POST /api/v1/optimization/cis/analyze`**: runs the M62 CIS posture check
  over a supplied observation bundle (`components`, `workloads`, `bindings`,
  `namespaces`) and returns the `cis.Status` rollup (totals, by-severity,
  by-family, findings with rationale + remediation).
- **`POST /api/v1/optimization/finops/analyze`**: runs the M61 FinOps
  right-sizing advisor over a per-container request/limit/usage bundle and
  returns the `finops.WasteSummary` (waste USD, idle cores/GB, per-container
  recommendations). An optional `rate` override replaces the configured default
  cost rate.
- **`POST /api/v1/optimization/deprecated-api/analyze`**: runs the M63
  deprecated/removed API check over a supplied object list against a target
  Kubernetes minor version and returns the `deprecatedapi.Status` rollup.
- All three endpoints are **read-only** and accept the already-collected
  observation bundle in the request body; the server never reaches into a
  cluster (ADR 0004). They are gated behind `httpserver.Options.Optimization`
  and registered only when the optimization service is configured, matching the
  per-milestone route pattern. The OpenAPI contract (`docs/api/openapi.yaml`)
  and the route-parity test were updated to cover the new routes.
- A new thin `internal/optimization` package carries the shared service config
  (default `finops.CostRate`); the analyzers themselves remain pure functions.
- The earlier "deferred service layer" is now the API surface above. Server-side
  auto-collection of the observation bundles from the live Kubernetes API /
  `metricshistory` store is delivered in **M65** (see below) — the analyze
  endpoints now auto-collect when the request body carries no bundle and a
  collector is configured.

### Added — M65 Server-Side Auto-Collection Layer (P1-①, completes M61–M64)

- **`internal/optimization/collector.go`** — `Collector` turns live cluster
  data into the exact observation bundles the M61–M63 analyzers consume. It is
  read-only (ADR 0004): it only reads and maps, never mutates cluster state.
  - `ClusterLister` interface — the only cluster access the collector needs:
    `List(ctx, clusterID, path) ([]json.RawMessage, error)`. A fake can be
    supplied in tests (no real cluster, no network), matching the existing
    `kubernetesEventLister` adapter pattern.
  - `NewKubernetesLister(gateway kubernetes.Gateway, creds kubernetes.CredentialSource) ClusterLister` — production adapter that talks to the **same read-only
    `kubernetes.Gateway`** the rest of the platform uses (no new client); it
    resolves the kubeconfig via `CredentialSource.Access`, calls
    `Gateway.Get(path, ...)`, and returns the List `items`.
  - `CollectCIS` — maps pods (workload security context + hostPath count),
    RBAC (clusterrolebindings→clusterroles + per-namespace rolebindings→roles
    with resolved policy rules), and namespace Pod Security Admission labels
    into `cis.Inputs`. Control-plane component flags (kube-apiserver etc.) are
    intentionally **not** reachable through the K8s API, so `Components` is
    left empty by design; callers with node/manifest access may populate it
    directly.
  - `CollectFinOps` — maps Deployment/StatefulSet/DaemonSet requests/limits/
    replicas and joins per-pod p95 usage from a `MetricsSource` (see below).
  - `CollectDeprecatedAPI` — scans 23 resource list paths (core + apps + batch
    + networking + autoscaling + policy + rbac + storage); a 404 / not-installed
    type is silently skipped so adding paths is safe.
- **`internal/optimization/metrics.go`** — `metricsHistorySource` implements
  the new `MetricsSource` interface (`PodContainerP95(ctx, clusterID, namespace,
  pod, container) (cpuNanocores, memBytes, ok)`) over `metricshistory.Service`,
  querying a configurable window (default 24h) of per-pod container cpu/memory
  samples and returning the p95. When no metrics source is configured the
  FinOps collector degrades gracefully to request/limit-only collection (the
  analyzer simply reports no over-provisioning for containers without a usage
  signal, per `finops.Recommend`).
- **`internal/optimization/service.go`** — `NewService(rate finops.CostRate,
  collector *Collector)`; the collector may be `nil` (body-only mode). New
  `HasCollector()` plus `CollectCIS` / `CollectFinOps` / `CollectDeprecatedAPI`
  delegate methods.
- **`internal/httpserver/optimization.go`** — auto-collect wiring: each
  `/analyze` handler, when the request body carries no observation bundle **and**
  the service has a collector, auto-collects from the live cluster before
  running the pure analyzer; a collection failure returns `502 COLLECT_FAILED`
  (distinct from a `400` client-input error). The three analyze routes are
  unchanged (OpenAPI contract untouched).
- **`cmd/server/main.go`** — wires the collector into the optimization service:
  `optimization.NewCollector(optimization.NewKubernetesLister(clusterRegistry,
  clusterService), optimization.NewMetricsHistorySource(metricsHistoryService,
  24*time.Hour))`, reusing the existing `clusterRegistry` (Gateway) and
  `clusterService` (CredentialSource) variables.
- **Tests**:
  - `internal/optimization/collector_test.go` (new) — 5 tests with an in-memory
    `fakeClusterLister` + `fakeMetrics`: CIS workload/RBAC/namespace mapping,
    FinOps requests/limits/p95, no-metrics degradation, deprecated-API scan
    (preserves its own apiVersion), and the `kubernetesLister` Gateway adapter.
  - `internal/httpserver/optimization_test.go` — 2 new handler-level
    auto-collect tests (`TestOptimizationCISAnalyzeAutoCollect`,
    `TestOptimizationDeprecatedAPIAnalyzeAutoCollect`) proving the
    body-empty → auto-collect → findings path end-to-end with a fake lister;
    both `NewService` call sites updated for the new signature.
- `internal/httpserver/openapi_route_test.go` — `NewService` call site updated
  for the new signature.
- `go build ./...`, `go vet ./...` and `go test ./...` (all packages) are green.

### Added — M66 Optimization Console (frontend for the M61–M65 analyzers)

Until now the FinOps / CIS / deprecated-API analyzers were reachable only over
HTTP — the console had no entry point, so none of M61–M65 was visible to an
operator. M66 closes that gap with a single read-only "优化中心" view.

- **`frontend/src/types/optimization.ts`** — TypeScript contracts mirroring the
  backend exactly: `CISStatus` (`cis.Status`), `FinOpsWasteSummary` /
  `FinOpsRecommendation` / `FinOpsQuantity` (`finops.*`), `DeprecatedAPIStatus`
  (`deprecatedapi.Status`), and a shared `OptimizationFinding` — CIS and
  deprecated-API both alias `finding.Finding` server-side, so the console
  renders both through one table shape.
- **`frontend/src/api/optimization.ts`** — client for the three `/analyze`
  endpoints. Each request sends only `cluster_id` (plus `target_version` for
  the deprecated-API check), which triggers the M65 server-side auto-collection
  path. Go marshals a nil slice/map as `null`, so `findings`, `recommendations`,
  `by_severity` and `by_family` are normalised to `[]`/`{}` at the boundary
  instead of forcing every view to guard against null.
- **`frontend/src/views/OptimizationView.vue`** — three tabs over one shared
  cluster selector:
  - *成本优化* — headline cards (monthly savings, over-provisioned containers,
    idle CPU cores, idle memory) plus a right-sizing table sorted by monthly
    waste. CPU is rendered from nanocores (cores / millicores) and memory from
    bytes (GiB / MiB); `-1` renders as 未设置, matching `finops.Quantity`'s
    unset sentinel. Costs stay in USD because `finops.CostRate` is defined as
    USD per core-/GB-month.
  - *CIS 合规* — pass-rate score, failed controls, severity split, per-family
    chips, and a findings table ordered critical → warning → info, showing the
    catalog `remediation` under each summary.
  - *废弃 API* — target-version input (default `1.29`), removed/deprecated/clean
    counters, and a migration table pairing each object's current
    `api_version` with its `replacement`.
  - Results are cached per tab and invalidated when the cluster changes; a
    request sequence guard prevents a slow response for a previous cluster from
    overwriting a newer one. `COLLECT_FAILED` (502) and `NO_INPUTS` (400) are
    translated into actionable Chinese messages rather than raw error codes.
- **`frontend/src/router/index.ts`** / **`ConsoleLayout.vue`** — `/optimization`
  route and a "优化中心" entry in the 分析与治理 navigation group.
- **`frontend/src/api/optimization.test.ts`** (new, 4 tests) — asserts the
  auto-collection request shape (`cluster_id` only), null-to-empty
  normalisation, the optional cost-rate override, and that a 502
  `COLLECT_FAILED` surfaces as a stable typed API error.
- The whole view is read-only: it issues no mutating request and offers no
  apply/patch action, consistent with ADR 0004.
- Frontend `eslint`, `vue-tsc -b`, `vitest run` (19 files / 85 tests) and
  `vite build` are green.

### Added — M67 Read-Only Network Connectivity / NetworkPolicy Posture Analyzer (P1-②)

P1-② adds a fourth read-only analyzer to the 优化中心: a *static* network
reachability / NetworkPolicy posture check. It never sends a probe packet —
consistent with ADR 0004 it reasons purely from the cluster's declared state
(Service selectors, targetPorts, and the NetworkPolicy whitelist overlay), so
it cannot perturb traffic.

- **`backend/internal/netpolicy/model.go`** (new) — analyzer contract reusing
  `type Finding = k8sfinding.Finding`; family constants `FamilyCoverage` /
  `FamilyPolicyHygiene` / `FamilyReachability` / `FamilyExposure`; finding
  codes `CodeNamespaceNoDefaultDeny`, `CodePodIngressUnrestricted`,
  `CodeServiceNoBackends`, `CodeServicePortBlocked`, `CodeExposedUnrestricted`,
  `CodePolicyDeadSelector`, `CodePolicyAllowAllIngress`, `CodePolicyFromAllNs`,
  `CodePolicyWideIngress`, `CodePolicyWideEgress`, etc. `systemNamespaces`
  (kube-system / kube-public / kube-node-lease) downgrade to `info`. `Selector`
  keeps the empty-vs-missing distinction (`SelectsAll` / `Matches` /
  `coversConservatively`) so a selector with `matchExpressions` is treated
  conservatively (never flagged as uncovered or dead). `Status` carries
  inventory counters (`namespaces_total`, `pods_total`, `policies_total`,
  `services_total`, `ingress_covered_pods`, `egress_covered_pods`,
  `isolated_namespaces`, `exposed_services`) in addition to `by_severity` /
  `by_family` / `findings`.
- **`backend/internal/netpolicy/service.go`** (new) — pure `Evaluate(clusterID,
  Inputs, observedAt) Status`: builds a per-namespace index then runs
  `evalNamespaceBaseline` (default-deny only matters where Pods exist),
  `evalPodCoverage` (in a partially-covered namespace, only Pods matched by no
  ingress policy are warned), `evalPolicyHygiene` (dead selector → warning,
  allow-all / from-all-ns / wide ipBlock / world-egress rules), and
  `evalServices` (no backends → critical; named targetPort unresolved →
  critical, numeric targetPort undeclared → info; default-deny blocking a
  required port → warning; exposed NodePort/LoadBalancer service with no
  backing ingress policy → critical). Findings are sorted stably by
  severity → code → namespace → name.
- **`backend/internal/optimization/collector.go`** — new `CollectNetPolicy(ctx,
  clusterID)` reads namespaces / pods / services and *optionally* NetworkPolicies
  (a missing networking API is tolerated, not fatal). `IntOrString` is preserved
  as a string; empty vs missing `namespaceSelector` is preserved through
  `toSelector`; ingress `From` / egress `To` are mapped by `toRules`.
- **`backend/internal/optimization/service.go`** — `Service` delegates
  `CollectNetPolicy`, reusing the existing `HasCollector()` gate.
- **`backend/internal/httpserver/optimization.go`** — new `POST /network/analyze`
  handler: when the body is empty it auto-collects via the collector (502
  `COLLECT_FAILED` on failure); otherwise it evaluates a supplied bundle.
- **`backend/internal/httpserver/router.go`** — registers
  `POST /api/v1/optimization/network/analyze`
  (`AuditAction: optimization.network.analyze`).
- **`docs/api/openapi.yaml`** — documents `POST /api/v1/optimization/network/analyze`
  (`analyzeNetworkPosture`) with the `NetworkStatus` schema and 200/400/502
  responses.
- **Coverage**: `internal/netpolicy` at 95.2% (22+ tests); collector + http tests
  cover IntOrString spellings, empty-vs-missing namespaceSelector, missing
  networking API tolerance, and pod-list failure propagation.
- **Frontend** — `types/optimization.ts` gains `NetworkStatus`; `api/optimization.ts`
  gains `analyzeNetwork` (null → `[]`/`{}` normalised); `OptimizationView.vue`
  gains a 4th *网络连通* tab reusing the shared cluster selector, request-sequence
  guard, and findings table, with headline cards for default-deny namespaces,
  ingress-covered Pods, exposed services, and the policy baseline. `api/
  optimization.test.ts` adds the `analyzeNetwork` suite (auto-collect shape,
  null normalisation, 502 `COLLECT_FAILED`).
- Backend `gofmt` / `go vet` / `go build ./...` / `golangci-lint` / `go test
  -cover` and frontend `eslint` / `vue-tsc -b` / `vitest` / `vite build` are
  green.

### Added — M68 Read-Only Image Supply-Chain / Reproducibility Analyzer (P1-③)

P1-③ adds a fifth read-only analyzer to the 优化中心: a *static* container
image supply-chain and reproducibility check. It never contacts a registry and
never pulls a manifest — consistent with ADR 0004 it reasons purely from the
image references the workloads declare.

Scope note: real CVE scoring needs a vulnerability source (Trivy / Grype / an
advisory API) and is a deliberate follow-up. This milestone delivers the
inventory and reproducibility findings such a source would consume, because an
image pinned only by a mutable tag cannot be reasoned about after the fact —
a "fixed" version may never actually reach production.

- **`backend/internal/imagepolicy/model.go`** (new) — analyzer contract reusing
  `type Finding = k8sfinding.Finding`; family constant `FamilySupplyChain`;
  finding codes `CodeMutableTag`, `CodeNoDigestPin`, `CodePullAlwaysLatest`,
  `CodeSharedAcrossNamespaces`, `CodeMultipleTags`. `ImageInfo` decomposes a
  reference into repository / tag / digest / pullPolicy; `ImageUsage` links one
  container (`ContainerRef`: namespace, workload kind/name, container) to one
  image. `Status` carries inventory counters (`images_total`,
  `containers_total`, `mutable_tag_images`, `unpinned_images`) alongside
  `by_severity` / `by_family` / `findings`.
- **`backend/internal/imagepolicy/service.go`** (new) — pure `Evaluate(clusterID,
  Inputs, observedAt) Status`: groups usages by `repository|tag|digest`, then
  per image checks a mutable tag (`:latest` or omitted → warning), a missing
  digest pin (fixed tag but no digest → info), `imagePullPolicy: Always`
  combined with a mutable tag (info), and cross-namespace sharing (info);
  finally a per-repository tag-skew check (one repo under several tags → info).
  A digest pin short-circuits the mutable/unpinned checks: it is the fully
  reproducible case regardless of the tag. Exported `ParseImage` splits a
  reference without mistaking a registry port (`registry.io:5000/team/api`)
  for a tag. Findings are sorted stably by severity → code → name → namespace.
- **`backend/internal/optimization/collector.go`** — new `CollectImagePolicy(ctx,
  clusterID)` reads Deployments / StatefulSets / DaemonSets / Jobs plus
  *ownerless* Pods. Controller-owned Pods are skipped so an image is counted
  once per workload instead of once per replica; CronJobs are omitted because
  the Jobs they create are already listed. Init containers are included — they
  carry the same supply-chain risk. New raw types `containerImageRaw` /
  `podSpecImageRaw` / `workloadImageRaw` decode both `.spec.template.spec`
  (controllers) and `.spec` (bare Pods) via an embedded struct, and pick up
  `imagePullPolicy`, which `kubernetes.WorkloadContainer` does not carry.
- **`backend/internal/optimization/service.go`** — `Service` delegates
  `CollectImagePolicy`, reusing the existing `HasCollector()` gate.
- **`backend/internal/httpserver/optimization.go`** — new `POST /image/analyze`
  handler: when the body is empty it auto-collects via the collector (502
  `COLLECT_FAILED` on failure); otherwise it evaluates a supplied bundle.
- **`backend/internal/httpserver/router.go`** — registers
  `POST /api/v1/optimization/image/analyze`
  (`AuditAction: optimization.image.analyze`).
- **`docs/api/openapi.yaml`** — documents `POST /api/v1/optimization/image/analyze`
  (`analyzeImagePosture`) with 200/400/502 responses.
- **Coverage**: `internal/imagepolicy` at 96.9%; collector tests cover init
  containers, the registry-port parse trap, owned-pod de-duplication,
  image-less containers, absent collections, and list-failure propagation;
  http tests cover the supplied bundle, auto-collection, and the
  missing-cluster / empty-bundle rejections.
- **Frontend** — `types/optimization.ts` gains `ImageStatus`;
  `api/optimization.ts` gains `analyzeImage` (null → `[]`/`{}` normalised);
  `OptimizationView.vue` gains a 5th *镜像供应链* tab reusing the shared cluster
  selector, request-sequence guard, and findings table, with headline cards for
  images in use, mutable-tag images, unpinned images, and a derived
  reproducibility rate. `api/optimization.test.ts` adds the `analyzeImage`
  suite (auto-collect shape, null normalisation, 502 `COLLECT_FAILED`).
- Backend `gofmt` / `go vet` / `go build ./...` / `golangci-lint` / `go test
  -cover` and frontend `eslint` / `vue-tsc -b` / `vitest` / `vite build` are
  green.

### Added — M58 DevOps Read-Only + Cross-Cluster Copy + Backup/Restore GUI Backend

- **GitOps (ArgoCD Application) read-only** (`backend/internal/gitops/*`):
  - `Capability` probe for `argoproj.io/v1alpha1 Application` CRDs →
    `GET /api/v1/clusters/:cluster_id/gitops/capability` reports
    `{available, mode: none|direct_application_cr}` with counts and any
    probe-level last-error instead of failing.
  - `List`/`Get` project a GUI-friendly row (name, namespace, project,
    sync_status, health_status, repo_url/path/revision, destination)
    plus the full `raw_manifest` for detail views.
  - All reads flow through the existing ADR 0004 bounded Kubernetes
    gateway; no ArgoCD SDK or proxy credentials required.
  - Unit tests cover capability-none / capability-present, list filters
    and get hit/miss using a stubbed Raw client.
- **Interactive cross-cluster copy (copyops)**:
  - New table `copy_plans` (migration `000039_copyops_and_gitops.up.sql`)
    with `CHAR(36)` CHECKed IDs, status enum, JSONB-packed resource_items
    / copy_summary / diff, confirmation-token-hash, idempotency-key,
    TTL (`expires_at`), `locked_at` claim lease.
  - `copyops.Service.Preview`: source-namespace identity capture,
    MaxBundle=20 (enforced BEFORE normalize/dedup), fixed label/annotation
    prefix scrubbing, Secret scrub toggle, destination namespace
    must-exist preflight, per-item "already exists on destination" skip
    and server-side dry-run create so failed-admission items bubble
    `DryRunError`. Each item is stored with a `Diff = {ModeCreate,
    after=scrubbed}` projection for the GUI viewer.
  - `copyops.Service.Execute`: reuses the M19 controlled-operation
    Claim state machine via `Repository.ClaimAndLoad` (rewritten in M58
    to short-circuit idempotency-replay *before* "already executed"),
    re-checks SourceNamespaceIdentity (M28 backup-style CAS), re-checks
    destination namespace, applies pending items as create-only,
    aggregates success/partial/fail into `succeeded`/`failed` statuses
    with per-item applied UIDs.
  - Read routes: `GET /api/v1/copy-plans` (actor's own plans),
    `GET /api/v1/clusters/:cluster_id/copy-plans` (cluster-scoped list),
    `GET /api/v1/copy-plans/:plan_id` (plan + resource_items JSONB
    projection).
  - Write routes: `POST /api/v1/clusters/:cluster_id/copy-plans/preview`
    (source in path, target + bundle in body; returns 201 + ephemeral
    `confirmation_token` with `Cache-Control: no-store`),
    `POST /api/v1/copy-plans/:plan_id/execute` (confirmation_token +
    optional client idempotency_key).
  - Kind whitelist enforced by `k8sgateway.KindToGVR` (admission-style
    GVR registry). Disallowed kinds surface `ErrKindDisallowed`.
  - Service tests use an `inmemRepo` stub (no CGO) to walk all
    branches: invalid inputs, bundle-too-large pre-dedup, disallowed
    kind, success preview + write, missing namespace, Execute
    idempotency replay (SAME key → returns finished plan; DIFFERENT
    key → ErrInvalidIdempotency), Execute CAS drift (source-ns UID
    changed → failPlan + visible "drift" LastError).
- **Velero backup/restore GUI browse extensions**:
  - `backup.go` adds `listBackups` / `getBackup` reading `velero.io/v1
    Backups` via the existing cluster dynamic client; rows project
    phase, errors, warnings, started/completed, storage_location,
    schedule_name, included/excluded namespaces.
  - `restore.go` adds `listRestores` / `getRestore` with the same
    projection plus `backup_name`. Existing M22/M23 Plan CRUD is
    untouched.
- **OpenAPI contract updated** in `docs/api/openapi.yaml` with 11 new
  paths and 13 new schemas (GitOpsCapability, GitOpsApplication[List],
  VeleroBackupList, VeleroRestoreList, CopyBundleItem,
  CopyPlanPreviewRequest, CopyPlanExecuteRequest, CopyItemDiff,
  CopyPlanResourceItem, CopyPlanDiffSummary, CopyPlan[List]).
  `TestRegisteredRoutesMatchOpenAPI` continues to pass.
- **Service DI**: `cmd/server/main.go` wires `gitops.NewService` and
  `copyops.NewService(copyops.NewGormRepository(db))` into the
  HTTP server options. All handlers reuse existing auth/session/aikit
  middleware.
- Authorization: all 11 routes protected by `bearerAuth` and the
  existing per-cluster + workspace tenancy gates. copyops preview/
  execute writes audit target labels ("CopyPlan", sourceNamespace,
  planID) into the unified audit trail.
- Documentation: ADR 0070 captures the architectural decisions and
  the ClaimAndLoad idempotency-replay rewrite; change record
  `docs/changes/2026-08-01-m58-devops-readonly-copyops-backup-gui.md`
  describes file-level changes, tests, gate, and open items.
- Gate: `verify-fast.ps1 -Scope Backend` passes (gofmt, go vet,
  `go test ./...` 47 packages green in ~58s; CGO_ENABLED=0 safe).

### Added — M51 Bounded Event Stream + Alert Inhibits

- Bounded Server-Sent Events (SSE) event stream over Kubernetes Events:
  `GET /api/v1/clusters/:cluster_id/events/stream` (audit
  `kubernetes.events.stream`). The stream is a bounded poller (default 5s,
  min 50ms) over the read-only gateway (ADR 0004), not a Kubernetes Watch.
  Events are deduped by UID against a bounded ring (256 entries) and pushed
  with drop-oldest backpressure. The M35 namespace scope is honoured:
  all-namespaces polls cluster-wide; an authorized namespace set polls each
  namespace; an empty scope yields an immediately-closed empty stream
  (anti-leakage, not 404). The stream emits hello / event / stream-closed
  SSE messages. See ADR 0066.
- Alert inhibit rules (source_match → target_match suppression):
  `GET/POST/DELETE /api/v1/alert-routes/inhibits` (create/delete audit
  `alert_route.inhibit.create` / `alert_route.inhibit.delete`, require
  `operations_admin`). An inhibit is not time-bounded; suppression depends
  on the source alert's live firing state (a non-resolved delivery within
  the 5m active window), re-checked on every `MatchAndDeliver` call. This
  complements the M37B time-bounded silences. `reason` mandatory (1..500
  chars); 30 inhibits per user; at least one source and one target matcher
  required.
- New `internal/eventstream` package with `Service.Subscribe` (per-client
  poller goroutine), `Stream`, `EventSummary` projection, `EventLister`
  interface, `seenRing` UID dedup, `pushEvent` drop-oldest backpressure.
  Bounded constants (PollInterval 5s default / 50ms min / 60s max;
  BufferCap 256 default / 16 min / 1024 max; ListLimit 500 default /
  1000 max).
- `alert_inhibits` migration (`000036`) with CHECK constraints requiring
  at least one source and one target matcher; indexes on creator_id and
  enabled=TRUE.
- `MatchAndDeliver` now checks `IsInhibited` (after `IsSilenced`, before
  route matching). An inhibited alert produces no delivery record — fully
  suppressed.
- Authorization reused from M35 (cluster + namespace scope) + M37B
  (alertroute roles). No new authorization path; the 2D matrix is intact.
- 404 > 403 anti-leakage preserved: unauthorized inhibit delete → 404
  `INHIBIT_NOT_FOUND`; unauthorized namespace on event stream → empty
  stream (not 404).
- 38 unit tests (23 service + 15 handler) covering eventstream
  config/subscribe/dedup/backpressure/poll-error/cancel, inhibit
  create/list/delete/validation/limit/non-creator, IsInhibited firing
  semantics, MatchAndDeliver suppression, SSE handler (503/empty
  scope/delivery/headers/cancel).
- OpenAPI document updated with 4 paths and 4 schemas (`AlertInhibitView`,
  `AlertInhibitCreate`, `EventStreamMessage`, `EventStreamEvent`).
- `TestRegisteredRoutesMatchOpenAPI` covers all 4 M51 routes (route-contract
  consistency, ADR 0049).

### Added — M57 Helm Application Catalog + Controlled Deploy Plans

- Simplified Helm application catalog with M19 controlled-operation
  deploy plans. Operators can register Helm chart repositories
  (with optional basic-auth credentials), browse charts via read-only
  `index.yaml` HTTP fetch, and deploy charts through a preview →
  execute flow that reuses the M19 confirmation-token + idempotency-key
  + Claim state machine (mirrors promotion/backup/restore/maintenance).
  See ADR 0069.
- No Helm SDK dependency — chart metadata comes from `index.yaml`
  HTTP fetch (10 MiB body limit, 15s timeout); deployment targets a
  Flux `HelmRelease` CR (`helm.toolkit.fluxcd.io/v2beta1`, already in
  the M49 CRD whitelist). The Flux helm-controller handles
  reconciliation; the platform just creates the CR via the M49 generic
  `CreateResource` path. The HelmRelease manifest is built once at
  preview and applied verbatim at execute — deterministic, no
  re-rendering.
- Credentials never returned in API responses: the `Repository` model
  has `CredentialsJSON` tagged `json:"-"`; the `RepositoryView`
  projection has no credentials field at all (only `has_auth`
  boolean). Structurally impossible to leak credentials.
- 10 new routes under `/api/v1/app-catalog`: 4 repository CRUD
  (`GET/POST /repositories`, `GET/DELETE /repositories/:repo_id`),
  2 chart read (`GET /repositories/:repo_id/charts`,
  `GET /repositories/:repo_id/charts/:chart_name`), 2 plan read
  (`GET /plans`, `GET /plans/:plan_id`), 2 plan write
  (`POST /plans/preview`, `POST /plans/:plan_id/execute`). Write
  operations (create repo, delete repo, preview, execute) require
  `system_ops_admin`; reads are any-auth. All tagged with audit verbs
  per ADR 0008.
- `internal/appcatalog` package: `Service` with `KubernetesSource`
  (namespace check + HelmRelease dry-run + create) and
  `ChartIndexSource` (index.yaml fetch) interfaces. `Preview`
  validates request, checks namespace, fetches chart metadata, checks
  no existing HelmRelease, builds CR manifest, runs server-side
  dry-run, persists plan with one-time confirmation token (SHA-256
  hashed). `Execute` claims plan (row-level lock + constant-time token
  compare + idempotency key check), creates HelmRelease CR, completes/
  fails plan. A 409 conflict during execute is treated as success.
  Plan TTL 30 minutes; claim TTL 5 minutes.
- Migration `000038_app_catalog`: 2 new tables — `helm_repositories`
  (name UNIQUE, credentials_json JSONB) and `app_catalog_plans` (id
  VARCHAR(36) PK, status, repo_id FK, chart snapshot, target cluster/
  namespace, release_name, values_yaml, chart_metadata JSONB,
  release_manifest JSONB, deploy_diff JSONB, confirmation_token_hash
  BYTEA, idempotency_key, locked_at, executed_at, last_error,
  expires_at). Indexes on name, status, (target_cluster_id,
  target_namespace), idempotency_key.
- Authorization reuses M35 namespace scope + `RouteDescriptor`
  pattern. No new roles, no new middleware. The 2D authorization
  matrix is intact — the app-catalog is a platform-level resource.
  Routes are on `v1` (not `resourceRoutes`) — same pattern as
  promotion routes, since app-catalog endpoints don't take a
  `:cluster_id` path parameter.
- 32 appcatalog service tests (repository CRUD, chart listing/detail,
  preview success + 4 error paths, execute success + invalid-token +
  invalid-idempotency + plan-not-found + idempotent-replay, list
  plans, manifest building, HelmRelease path resolution, validation)
  + 24 handler tests (each route's success + error path, sentinel
  error mapping). `TestRegisteredRoutesMatchOpenAPI` covers all 10
  M57 routes.
- OpenAPI document updated with 10 paths and 11 schemas
  (`HelmRepositoryView`, `HelmRepositoryList`,
  `CreateHelmRepositoryRequest`, `ChartSummary`, `ChartDetail`,
  `ChartList`, `AppCatalogPlan`, `AppCatalogPlanList`,
  `DeployPreviewRequest`, `ExecuteDeployRequest`,
  `DeployPreviewResponse`, `ExecuteDeployResponse`).

### Added — M56 Golden Dataset Replay + Quality Dashboard

- Golden dataset replay runner (`internal/golden.ReplayRunner`) that
  verifies the M45 golden dataset (3 scenarios: mandatory 10-step
  end-to-end + 2 negative companions) against the current M39-M44
  engine contracts (package version constants + valid plan/verification
  status enumerations). The runner is deterministic: identical contracts
  + dataset produce identical results. Pure Go — no Kubernetes client,
  no Prometheus, no AI provider. See ADR 0068.
- Quality report storage (`internal/golden.FileReportStorage`) writes
  timestamped JSON files under `.artifacts/quality-report/` and loads
  the latest by reverse-chronological sort. Mirrors the audit-archive
  pattern (ADR 0031): artifacts survive database restores, no migration
  needed. `NopReportStorage` provided for tests.
- Async replay service (`internal/golden.Service`) with in-memory task
  tracking. `RunReplay` returns a task ID immediately (202) and launches
  a background goroutine (5-minute timeout) that runs the replay, loads
  the previous baseline report for before/after comparison, builds
  per-scenario `ScenarioQuality` (delta classified as
  preserved/improved/regressed/unchanged), and persists the report.
  `GetLatestReport` reads the latest persisted report; `GetTask` polls
  task status.
- 2 quality-report HTTP routes under `/api/v1/aiops`:
  `GET /quality-report` (read latest report, any authenticated user,
  audit `aiops.quality_report.read`) and `POST /quality-report/run`
  (trigger async replay, `system_ops_admin` only, audit
  `aiops.quality_report.run`). Returns 404 `QUALITY_REPORT_NOT_FOUND`
  when no report exists; 503 `GOLDEN_UNAVAILABLE` when the service is
  nil.
- Engine contracts adapter (`cmd/server/golden_contracts.go`) reads
  live version constants and status enumerations from the M39-M44
  engine packages, keeping the golden package free of engine imports
  (avoids cycles). Same adapter pattern as M52's
  `inspection_cluster_lister.go`.
- Before/after baseline comparison: when a replay completes, the
  service loads the previous report and records `PassedBefore` /
  `PassedAfter` / `Delta` per scenario, plus `DatasetVersionBefore` /
  `EngineVersionsBefore` from the baseline. `QualitySummary` aggregates
  totals (passed/improved/regressed/preserved/unchanged, step counts).
- 26 golden package unit tests (runner, storage, service, quality,
  dataset integrity) + 6 handler tests (GET 200/404/503, POST 202/503,
  end-to-end report production). `TestRegisteredRoutesMatchOpenAPI`
  covers both M56 routes.
- OpenAPI document updated with 2 paths and 5 schemas (`QualityReport`,
  `EngineVersions`, `ScenarioQuality`, `QualitySummary`,
  `QualityReplayResponse`).
- Fixed pre-existing data race in `inspection.fakeExecutor` (concurrent
  append to `calls` slice); added `sync.Mutex` guard.

### Added — M52 Intelligent Inspection + Service Mesh Read-Only

- KubeEye-style intelligent inspection with 8 compile-time rules:
  `node_not_ready`, `pod_restart_loop`, `pod_oom_killed`, `pvc_pending`,
  `image_pull_backoff`, `pod_crash_loop`, `endpoints_orphan`,
  `namespace_quota_high`. Each rule has a stable M39 signal code of the
  form `inspect.<domain>.<code>.v1`, a default severity, a per-rule
  timeout, and remediation hints. Rules are compiled into
  `internal/inspection.DefaultCatalog()` — no runtime rule injection, no
  external KubeEye operator. Per-cluster `enabled`/`severity` overrides
  are persisted via the `inspection_rules` SQL table. See ADR 0067.
- Inspection routes under `/api/v1/inspection`:
  `GET /rules/catalog` (read rule catalog);
  `GET/PUT /clusters/:cluster_id/inspection/rules` (read/save per-cluster
  effective overrides);
  `POST /run-once` (trigger a one-shot run, audit `inspection.run.create`,
  require `operations_admin`);
  `GET/POST /plans` + `DELETE /plans/:plan_id` (periodic plan lifecycle
  as standard 5-field cron, audit `inspection.plan.create` /
  `inspection.plan.delete`, require `operations_admin`);
  `GET /tasks` + `GET /tasks/:task_id` + `GET /tasks/:task_id/results`
  (task lifecycle and findings list, audit `inspection.task.read`,
  require `operations_viewer`).
- Bounded execution engine: `MaxConcurrentClusters` (default 4, 1..32),
  `PerClusterTimeout` (default 15s, 5..120s), per-rule descriptor
  timeout, `MaxTaskResults` (default 1000) short-circuits to
  `RESULTS_TRUNCATED` when the row cap is reached. Worker fan-out uses
  `MaxConcurrentClusters` goroutines; each worker routes to a dedicated
  `ruleXxx` method that reads exclusively through the ADR 0004 read-only
  gateway.
- Findings normalized into the M39 `signal_occurrences` table:
  `source = 'inspection'`, `evidence_id` references
  `inspection_results.id`, fingerprint = `sha256(cluster_id +
  signal_code + resource_uid + summary_prefix)` with a 300s dedup
  window. The M42 correlation and M44 automation engines therefore see
  inspection findings as first-class signals with no additional wiring.
- In-process cron scheduler (`internal/inspection/scheduler.go`) ticks
  every 30s, re-reads enabled plans from SQL each tick (no in-memory
  cache → no multi-replica split-brain), and uses
  `UPDATE … SET last_run_at = NOW() WHERE id = ? AND last_run_at < ?`
  via `clause.OnConflict` to elect exactly one runner per plan tick.
  Not a K8s CronJob (escapes single-binary boundary, ADR 0002).
- Service mesh read-only list/detail views for Istio
  `VirtualService`/`DestinationRule`:
  `GET /api/v1/servicemesh/virtualservices`,
  `GET /api/v1/servicemesh/virtualservices/:namespace/:name`,
  `GET /api/v1/servicemesh/destinationrules`,
  `GET /api/v1/servicemesh/destinationrules/:namespace/:name`. Shallow
  projection (hosts/gateways/http_routes_count for VirtualService;
  host/subsets_count/traffic_policy_summary for DestinationRule) via
  the M49 CustomResource gateway; raw manifest not returned from mesh
  routes (go via M49 generic CRD browser if required). Cluster not
  running Istio → empty list, never 5xx.
- Service mesh traffic metrics:
  `GET /api/v1/servicemesh/traffic-metrics?cluster_id=&namespace=&service=&window=&step=`
  aggregates request_count, http_2xx/4xx/5xx counts, error_rate, and
  latency p50/p95/p99 points arrays from the M36 Prometheus history.
  Six fixed-template series calls; no client PromQL injection. Window
  capped at 24h; step normalized to `max(window/500, 15s)`. Feeds the
  M41 SLO evaluator and M40 topology edge attributes — never an
  action input.
- 4 new tables in migration `000037_inspection_and_servicemesh.up.sql`:
  `inspection_rules` (per-cluster override, UNIQUE(cluster_id,rule_code)),
  `inspection_plans` (cron + cluster_ids[] + rule_codes[] + severity_floor),
  `inspection_tasks` (QUEUED→RUNNING→SUCCEEDED/PARTIAL/FAILED lifecycle
  with RESULTS_TRUNCATED summary state, trigger_source/triggered_by,
  cluster/rule snapshot, counts, timestamps),
  `inspection_results` (task_id FK, resource_ref JSON, labels JSON).
  CHECK constraints on not-empty arrays; indexes on plan.enabled,
  task.status, task.triggered_by, result(task_id, rule_code).
- Authorization reused from M35 (namespace scope) + M46 (workspace
  scope) + M47 (three-tier nav RBAC roles matrix): `operations_admin`
  for plan create/delete + override save + run-once,
  `operations_viewer` for reads; `mesh_admin`/`mesh_viewer` for mesh
  routes. No new roles.
- 404 > 403 anti-leakage preserved: unauthorized plan/rule/override
  delete → 404 `PLAN_NOT_FOUND` / equivalents; unauthorized namespace
  on VirtualService/DestinationRule → 404
  `VIRTUAL_SERVICE_NOT_FOUND` / `DESTINATION_RULE_NOT_FOUND`; empty
  scope → empty task-results list + empty metrics (not 403/404).
- New `internal/inspection` package (model, catalog of 8, repository,
  DefaultExecutor, Service, Scheduler; 27 service tests ~97% coverage)
  and `internal/servicemesh` package (model, Service; 16 service tests)
  and `cmd/server/inspection_cluster_lister.go` adapter that bridges
  `cluster.Service.ListWorkspacesClusters` into
  `inspection.ClusterLister`.
- 38 handler tests (21 inspection + 17 servicemesh) covering route 200s,
  validation 400s, auth 401s, role 403s, 404 anti-leakage, workspace
  cluster scoping, M35 namespace scope filtering on per-cluster
  task-results.
- OpenAPI document updated with 13 paths and 14 schemas
  (`InspectionRuleDescriptor`, `InspectionEffectiveRule`,
  `InspectionEffectiveRuleSave`, `InspectionTaskRunRequest`,
  `InspectionTaskView`, `InspectionPlanCreate`, `InspectionPlanView`,
  `InspectionResultView`, `InspectionTaskResultList`,
  `VirtualServiceView`, `DestinationRuleView`,
  `ServiceMeshTrafficMetrics`, `ServiceMeshTrafficPoint`,
  `InspectionTaskFilter`).
- `TestRegisteredRoutesMatchOpenAPI` covers all 13 new paths.

### Added — M50 Monitoring Dashboard + Log Explorer

- Fixed-template monitoring dashboard with 4 compile-time templates
  (node_overview, workload_overview, pod_overview, workspace_overview) in the
  new `internal/monitoring` package. Clients cannot inject PromQL; the
  dashboard returns panel descriptors (metric, unit, resource_kind) that the
  frontend uses to drive existing `/metrics/history` calls. See ADR 0065.
- 2 dashboard HTTP routes:
  `GET /api/v1/clusters/:cluster_id/monitoring/dashboard/:template`
  (single-cluster, audit `monitoring.dashboard.read`) and
  `GET /api/v1/workspaces/:workspace_id/monitoring/dashboard` (workspace
  cross-cluster, audit `monitoring.dashboard.read`).
- Workspace dashboard returns a topology-only response: the fixed
  `workspace_overview` template plus the workspace's cross-cluster
  `(cluster_id, namespaces)` topology. The backend does NOT pre-fetch
  per-cluster time series; the frontend fans out using the topology. Fan-out
  bounded by `MaxClusters` (20, matching federation).
- Log explorer endpoint `POST /api/v1/clusters/:cluster_id/logs/query`
  (audit `monitoring.logs.query`) reusing the M37A `capability.LogProvider`
  (Loki). Clients cannot inject LogQL. The namespace arrives in the body and
  is re-checked against the M35 resolved namespace scope (anti-leakage 404).
- Dashboard time window bounded to 24h (`MaxDashboardWindow`, matching
  metricshistory `MaxQueryWindow`).
- Authorization reused from M35 (cluster + namespace scope) + M46 (workspace
  roles) + M47 (workspace filter). No new authorization path; the 2D matrix
  is intact.
- 404 > 403 anti-leakage preserved: unauthorized workspace → 404
  `WORKSPACE_NOT_FOUND`; unauthorized namespace in logs/query → 404
  `RESOURCE_NOT_FOUND`.
- 28 unit tests (11 service + 17 handler) covering template validation,
  window validation, panel cloning, workspace topology (sorted + bounded),
  nil service/provider (503), logs query (200/400/404/500 + scope
  allowed/denied + default direction + provider error).
- OpenAPI document updated with 3 paths and 4 schemas (`MonitoringPanel`,
  `ClusterDashboardResponse`, `WorkspaceDashboardResponse`,
  `MonitoringLogQuery`).
- `TestRegisteredRoutesMatchOpenAPI` covers all 3 M50 routes (route-contract
  consistency, ADR 0049).

### Added — M49 CRD Discovery + Read-Only Custom Resource Browsing

- Read-only custom resource browser for operator CRDs, building on the M47
  CRD discovery preview (GVR metadata) with instance-level list + detail
  endpoints. See ADR 0064.
- `customResourceWhitelist` compile-time-fixed GVR map in
  `internal/kubernetes/service.go` (22 entries across Velero (3), Prometheus
  operator (8), Flux Helm/source (5), cert-manager (6)). One entry
  (`cert-manager.io/v1/clusterissuers`) is cluster-scoped; the rest are
  namespaced. Adding an entry is a code change — no admin API, no runtime
  expansion (static-extension hard constraint).
- 2 read-only HTTP routes under
  `/api/v1/clusters/:cluster_id/custom-resources`:
  `GET /:group/:version/:resource` (list, audit
  `kubernetes.custom_resources.list`) and
  `GET /:group/:version/:resource/:name` (detail, audit
  `kubernetes.custom_resources.read`). Only `GET` is registered — no write
  routes and no write service methods; the read-only contract is structural.
- Manifest redaction reused from M22 (`redactManifest`): sensitive fields
  (`password`, `secret`, `token`, `key`, ...) are recursively redacted to
  `"<redacted>"` on every returned manifest.
- Authorization reused from M35 (namespace scope) + M47 (`?workspace_id`
  visibility filter). Namespaced CRDs fan out across the caller's
  `ClusterScope` via `authorizedNamespaceLists`; cluster-scoped CRDs are
  listed cluster-wide. No new authorization path; the 2D authorization matrix
  (ADR 0050/0061) is intact.
- 404 > 403 anti-leakage preserved: a non-whitelisted GVR returns 404
  `RESOURCE_NOT_FOUND` before the gateway is contacted (indistinguishable
  from a missing resource). An empty authorized scope yields 200 `items: []`,
  not 404.
- Detail endpoint requires `?namespace=` for namespaced CRDs (400 otherwise)
  and ignores it for cluster-scoped CRDs.
- 34 unit tests (17 service + 17 handler) covering whitelist allow/deny,
  namespaced vs cluster-scoped path building, list + detail with redaction,
  name filter + sort, selector forwarding, cluster-disabled (409) and
  not-found (404) propagation, namespace-grant fan-out, empty-scope empty
  list, only-GET registered, and the `writeServiceError` whitelisted mapping.
- OpenAPI document updated with 2 custom-resources paths and 2 schemas
  (`CustomResource` free-form object, `CustomResourceList` with `items` +
  `total` + `remaining`).
- `TestRegisteredRoutesMatchOpenAPI` covers both custom-resources routes
  (route-contract consistency, ADR 0049).

### Added — M48 Multi-Cluster Federation (Host/Member Model)

- `federation` package implementing a KubeSphere-style host/member cluster
  model as a SQL aggregation view over the existing `clusters` table. No new
  CRD, no Cluster Agent, no inter-cluster sync controller. See ADR 0063.
- Migration `000035_cluster_federation` extending `clusters` with
  `cluster_role` (host/member/standalone), `federation_status`
  (registered/healthy/degraded/disconnected), `registered_at`, and
  `last_heartbeat_at` columns. CHECK constraints bound the enums; a partial
  unique index `clusters_single_host_uq` enforces the single-host invariant.
- `cluster_federation_events` append-only table recording every federation
  state transition (registered/deregistered/heartbeat/status_change/
  role_change). No UPDATE/DELETE path is exposed by the repository.
- 9 HTTP routes under `/api/v1/federation`: overview, events, resource
  summary, cluster register/deregister/promote/demote/heartbeat/status, and
  per-cluster events. Write operations require `operations_admin`; reads
  require authentication only (visible clusters narrowed by authz scope).
- Cross-cluster resource summary with bounded fan-out (20 clusters, 4s
  per-cluster timeout) over a fixed 9-entry GVR whitelist. Missing/unreachable
  clusters contribute zero counts with `TIMEOUT` / `QUERY_FAILED` error codes;
  partial results are always returned.
- `federation_status` is orthogonal to the existing `clusters.status` — the
  cluster probe updates `clusters.status`; the federation heartbeat updates
  `federation_status`. A cluster can be `ready` but `degraded`.
- Anti-leakage (404 > 403) preserved: `ErrClusterNotFound` surfaces as 404 so
  a missing cluster is indistinguishable from an unauthorized one.
- `ClusterLister` interface decoupling the federation service from the
  kubernetes package; `kubernetesClusterLister` adapter in `cmd/server/`
  translates typed list methods into `CountResult`.
- 58 unit tests (38 service + 20 handler) covering register/deregister/
  promote/demote/heartbeat/status/overview/events/resource-summary, single-host
  invariant, idempotency, anti-leakage, timeout error mapping, and all HTTP
  status paths (200/400/404/409/503).
- OpenAPI document updated with 9 federation paths and 9 schemas
  (`FederationOverview`, `FederationEvent`, `FederationEventList`,
  `FederationResourceSummary`, `FederationCluster`,
  `RegisterFederationClusterRequest`, `DemoteFederationClusterRequest`,
  `FederationHeartbeatRequest`, `UpdateFederationStatusRequest`).
- `TestRegisteredRoutesMatchOpenAPI` updated to wire `FederationService` so
  the route-contract test covers all federation routes.

### Added — M47 Three-Tier Console Navigation + Workspace Resource Filter

- `GET /api/v1/clusters/:cluster_id/api-resources` CRD discovery preview
  endpoint returning the union of a fixed operator-curated GVR whitelist
  (27 core/apps/batch/networking/discovery/policy/autoscaling/rbac/storage
  resources) and the cluster's dynamically discovered API resources. See ADR
  0062.
- Graceful discovery degradation: discovery failures (nil provider,
  credential error, API error, partial result) return the whitelist only;
  the endpoint never 500s due to discovery unavailability.
- Dynamic discovery merge with dedup (whitelist entries are never
  duplicated), subresource skip (`pods/log`), and non-listable/non-gettable
  resource skip.
- `workspace_id` optional query parameter on namespace-scoped resource list
  endpoints (Pods, Deployments, Services, ...) that narrows the authorized
  namespace scope to a workspace's member namespaces on the current cluster.
- `withWorkspaceNamespaceFilter` middleware and `narrowScopeByWorkspace`
  pure function implementing the workspace visibility filter. The filter is
  a pure visibility narrowing, NOT an authorization decision — it runs after
  `requireClusterAccess` + `requireNamespaceQueryAccess` and only narrows
  the already-authorized scope.
- Anti-leakage (404 > 403) preserved end-to-end: unauthorized cluster
  returns 404 before the filter; non-existent/empty workspace returns 200
  with `items: []` (not 404) so workspace existence is not leaked.
- `ListMembershipsByCluster` repository method and
  `NamespacesForWorkspaceFilter` service method supporting the workspace
  filter read path.
- `DiscoveryProvider` dependency on the Kubernetes `Service` for pluggable
  discovery (nil in route-contract tests; wired to client-go discovery in
  production).
- 23 unit tests (7 discovery + 5 workspace-filter service + 11
  httpserver-filter) covering whitelist-only fallback, CRD merge/dedup,
  discovery/credential error fallback, sorted output, zero-ID short-circuit,
  cross-workspace/cluster isolation, unknown-workspace empty result,
  AllNamespaces narrowing, namespace-grant intersection, anti-leakage
  empty-scope collapse, invalid `workspace_id` 400, repository-error 500,
  and nil-service pass-through.
- OpenAPI document updated with `APIResource`, `APIResourceList` schemas,
  `WorkspaceIDQuery` reusable parameter, and the `/api-resources` path.

### Added — M46 Workspace Multi-Tenancy

- `workspace` package with 5 SQL tables (`workspaces`,
  `workspace_memberships`, `workspace_quotas`, `user_workspace_grants`,
  `workspace_role_bindings_audit`) in migration `000034_workspaces_and_grants`.
  See ADR 0061.
- Three fixed workspace roles (`workspace_admin` / `workspace_editor` /
  `workspace_viewer`) independent of the four platform roles. The
  `role` column has a CHECK constraint that accepts only the three fixed
  values.
- `WorkspaceGrant` as a third, orthogonal grant type that does NOT grant
  namespace read access — the 2D authorization matrix from M35 (ADR 0050)
  is unchanged. WorkspaceGrant only authorizes workspace metadata /
  membership / quota / role-binding edits.
- 14 HTTP routes under `/api/v1/workspaces` covering workspace CRUD,
  memberships, quota, role bindings and audit trail. Authorization is
  enforced inside the service layer; the handler only parses inputs and
  maps errors.
- Anti-leakage (404 > 403): unauthorized workspace access returns
  `ErrWorkspaceNotFound` (HTTP 404), never 403, so hidden workspaces
  cannot be distinguished from missing ones.
- SystemAdmin bypass for all workspace grant checks.
- Workspace creation and deletion are SystemAdmin-only (workspace
  self-service creation is deferred).
- Owner is always `workspace_admin`; the grant is seeded atomically on
  creation and cannot be revoked or downgraded while the workspace exists.
- Append-only audit trail (`workspace_role_bindings_audit`) for every
  role-binding change (granted / revoked / changed).
- Soft quota display (`workspace_quotas`) — the platform does NOT enforce
  workspace quota against cluster ResourceQuota.
- 39 unit tests (29 service-level + 10 handler-level) covering
  create/get/list/update/delete, membership, quota, role bindings,
  anti-leakage, role hierarchy, owner protection and metadata
  normalization.
- OpenAPI document updated with 14 workspace paths and 11 workspace
  schemas.

### Added — M45 Versioned AIOps Golden Dataset And Quality Report

- `golden` package with `DatasetVersion = "1.0"`, `ScenarioVersion =
  "1.0"`, 10 `StepID` constants (establish_healthy_service/
  publish_bad_image/capture_signals/build_impact_graph/rank_cause_candidate/
  generate_investigation/preview_approve_rollback/execute_verify/
  recover_alert/cleanup), `AllSteps` ordered list, 3 `ScenarioID`
  constants (mandatory_end_to_end/negative_misattribution/
  negative_partial_evidence), `StepOutcome` with expected signal/topology/
  SLO/correlation/investigation/action plan/verification/alert recovery
  flags, `Scenario`, `Dataset`, `DefaultDataset()` returning 3 scenarios.
  See ADR 0060.
- Mandatory 10-step end-to-end golden scenario mapping each step of the
  AIOps loop to an expected outcome: establish_healthy_service (M41),
  publish_bad_image (M23), capture_signals (M39+M41), build_impact_graph
  (M40), rank_cause_candidate (M42), generate_investigation (M43),
  preview_approve_rollback (M44 approved), execute_verify (M44
  verified+effective), recover_alert (M27), cleanup.
- Negative companion `negative_misattribution`: unrelated simultaneous
  change in another Namespace must NOT be attributed to the primary case
  (expects correlation case but does NOT expect action plan).
- Negative companion `negative_partial_evidence`: when one metrics/log
  provider is stopped, the case must be partial/unknown rather than
  falsely healthy or resolved (expects valid advisory investigation but
  does NOT expect alert recovery). Preserves the M41 fail-closed
  invariant.
- `QualityReport` with before/after dataset versions, `EngineVersions`
  tracking M39-M44 (Signal/Topology/SLO/Correlation/Investigator/
  Automation/Verifier), per-scenario `ScenarioQuality` (passed_before/
  after, delta, steps_passed_before/after, notes), `QualitySummary`
  aggregation (TotalScenarios/PassedBefore/After/Improved/Regressed/
  Preserved/Unchanged/TotalSteps), `ClassifyDelta` (preserved/improved/
  regressed/unchanged), `Summarize`. JSON-serializable. Generated offline;
  never self-modifies rules, prompts or policy online.

### Added — M44 Policy-Constrained Automation And Post-Action Verification

- `automation` package with `AutomationVersion = "1.0"`, `VerifierVersion =
  "1.0"`, 4 `AutomationLevel` values (L0/L1/L2/L3; L2 is the default, L3
  not enabled in M44), 9 `PlanStatus` values (draft/previewed/approved/
  executing/succeeded/failed/expired/cancelled/verified), 2 `ApprovalType`
  values (single/four_eyes), 3 `GateStatus` values (passed/failed/skipped),
  8 `GateCode` values (uid_rv_recheck/scope/pdb_blast_radius/slo_burn/
  freeze_window/concurrent_plans/attempt_cap/rollback_point), `ActionPlan`
  with deterministic `plan_key` (SHA-256 over case_id + runbook_id +
  target_uid + automation_version), `ActionVerification` with deterministic
  `verification_key` (SHA-256 over plan_id + verifier_version +
  evidence_hash), `EvidenceSnapshot`, `SLOSnapshot`. See ADR 0059.
- `RunbookDescriptor` + compiled `catalog` map with 2 V1 executable
  runbooks: `rollback_last_rollout` (`deployment.rollback`, four-eyes),
  `rollout_restart_pods` (`deployment.rollout_restart`, single). Mirrors
  the M43 catalog but only includes runbooks with non-empty `ActionCode`
  (advisory-only runbooks cannot be materialized into action plans).
  `LookupRunbook` fails closed; adding a runbook is a contract change.
- `GateEvaluator` is stateless and pure. `RequiredGates(actionCode)`
  returns the action-specific gate set (core: uid_rv_recheck/scope/
  freeze_window/concurrent_plans/attempt_cap; Pod-affecting add
  pdb_blast_radius; SLO-bound add slo_burn; rollback adds rollback_point).
  `Evaluate` runs at preview; `Recheck` runs at execute with `Rechecked =
  true` and fresh `GateContext`. `AllPassed` treats `skipped` as
  non-failure. Stale UID/RV, opened freeze window, exhausted PDB budget,
  or exceeded attempt cap all fail closed.
- Confirmation token (32-byte base64; SHA-256 hashed at rest) issued at
  preview; required at execute. Idempotency key (operator-supplied UUID)
  stamps the claim; replay returns the recorded outcome. Stale executing
  rows past `claimTTL` are reclaimable.
- `approvalTypeFor(actionCode)` returns `four_eyes` for rollback and
  image_update, `single` otherwise. Four-eyes requires
  `approver_user_id != requested_by_user_id`; enforced at the DB layer
  (CHECK constraint) and re-checked by the service. Self-approval of a
  four-eyes plan yields `ErrSelfApprovalForbidden` (403).
- `Verifier` is pure given (plan, pre, post). `CapturePreSnapshot` at
  execute time; `CapturePostSnapshot` after cooldown (default 300s, min
  60s). `compareEvidence` is deterministic: SLO state transitions take
  precedence (healthy > burning_slow > burning_fast > breached); resource
  state (replicas/available_replicas/image/suspended) is compared for
  actions without SLO evidence or when SLO state is unchanged. Missing
  evidence yields `ComparisonInsufficient` and `VerificationStatusUnknown`
  — the verifier never auto-resolves a diagnosis from missing data.
- Server-owned rollback contract: when verification yields ineffective/
  failed, `evaluateRollbackContract` checks target UID unchanged, no
  freeze, no concurrent plan, attempt cap not exceeded. If safe, a
  rollback plan is drafted automatically (status `draft`,
  `rollback_of_plan_id` set). If unsafe, verification records
  `reason = "unsafe_rollback_escalated_to_human"`. M44 never auto-executes
  rollback plans.
- `GormRepository` with `SavePlan`/`GetPlan`/`GetPlanForExecute` (row
  lock)/`ListPlans`/`CountAttemptsSince`/`CountConcurrentPlans`/
  `MarkPreviewed`/`Approve`/`Claim` (idempotent, row-lock, stale
  reclaimable)/`Complete`/`Fail`/`MarkVerified`/`Cancel`/`ExpireStale`/
  `SaveVerification`/`GetVerification`/`GetVerificationByPlan`/
  `UpdateVerification`. Partial unique indexes `uq_action_plans_active`
  (one non-terminal plan per `plan_key`) and
  `uq_action_verifications_active` (one pending verification per plan).
  `NopRepository` for testing/disabled mode.
- `Service` with `CreatePlan` (validate runbook + eligibility, materialize
  parameters, capture target snapshot, compute `plan_key`, issue
  confirmation token, persist draft), `Preview` (refresh snapshot,
  evaluate gates, store results, transition to previewed), `Approve`
  (enforce four-eyes distinctness), `Execute` (recheck gates, idempotent
  claim, build + apply patch, transition succeeded/failed, schedule
  verification), `Verify` (run verifier, evaluate rollback contract on
  ineffective/failed, mark verified), `Cancel`, `ListPlans`, `GetPlan`,
  `GetVerification`.
- Migration `000033_policy_constrained_automation` introducing
  `action_plans` and `action_verifications` tables with CHECK constraints
  (status/approval_type/evidence_comparison/verification_status, four-eyes
  distinctness, missing-evidence → insufficient+unknown), partial unique
  indexes, and FKs to `correlation_cases(id)` and `ai_investigations(id)`
  ON DELETE SET NULL.
- HTTP routes under `/api/v1/aiops/automation`: `GET /runbooks`,
  `GET /plans`, `POST /plans`, `GET /plans/:plan_id`,
  `POST /plans/:plan_id/preview`, `POST /plans/:plan_id/approve`,
  `POST /plans/:plan_id/execute`, `POST /plans/:plan_id/cancel`,
  `POST /plans/:plan_id/verify`, `GET /plans/:plan_id/verification`.
  Write routes require `rolesSystemOpsAdmin`; read routes require only
  authentication. Actor derived from the authenticated session.
  Idempotency-Key header read by execute.
- OpenAPI documentation for all 9 M44 paths and 9 schemas
  (AutomationRunbookList, AutomationRunbook, CreateActionPlanRequest,
  ApproveActionPlanRequest, ExecuteActionPlanRequest,
  ActionPlanListResponse, ActionPlanResponse, ActionVerification,
  PolicyGate).

### Added — M42 Multi-Signal Correlation And Deterministic RCA

- `correlation` package with 4 `ConfidenceClass` values (confirmed/
  candidate/contradicted/unknown), 3 `CaseStatus` values (active/resolved/
  stale), `Case` with deterministic `case_key` (SHA-256 over
  cluster_id+resource_uid+rule_id+correlation_version), `SignalLink`,
  `ResourceLink`, `ChangeCandidate`, `ActionCandidate` (fixed codes from
  M19 catalog). `CorrelationVersion = "1.0"`. See ADR 0057.
- `catalog` with 6 V1 rules covering golden replay scenarios
  (rollout→pod_failure, rollout→unavailable_deployment,
  rollout→no_endpoints, maintenance→node_failure, pvc_pending→pod_failure,
  rollout→metric_breach). Fail-closed lookup; adding a rule is a contract
  change.
- `Engine.Correlate` is pure and stateless: identical inputs + identical
  rule/correlation versions yield identical results. Explicit factors:
  `same_uid`, `topology_distance` (bidirectional BFS over M40 edges),
  `time_distance`, `change_symptom_rule`, `signal_freshness`,
  `signal_completeness`, `diagnosis_match`, `contradicting_signal`.
  `classifyConfidence` is a pure function; temporal proximity alone is never
  causality.
- 9 golden replay fixtures (ImagePull, CrashLoop, OOM, PVC-Pending,
  NoEndpoints, ReplicasUnavailable, NodeNotReady, MetricBreach,
  BadRollout-contradicted) plus a cold-start scenario. Each fixture is a
  deterministic (inputs, expected) pair; replaying produces byte-identical
  case_keys and confidence.
- `GormRepository` with idempotent `UpsertResult` (ON CONFLICT DO NOTHING),
  unique indexes on `case_key` (active), `(case_id, signal_occurrence_id,
  relation)`, `(case_id, uid, relation)`, `(case_id, change_event_id)`.
  `NopRepository` for testing/disabled mode.
- `Service` with `CorrelateNamespace` (bounded lookback, idempotent
  persist), `GetCase`, `ListCases`, `ListTimeline`, `GetCaseGraph`,
  `ListActionCandidates` (derives `deployment.rollback` /
  `deployment.rollout_restart` — no execute endpoint).
- Migration `000031_diagnosis_correlation` introducing
  `correlation_cases`, `correlation_signal_links`,
  `correlation_resource_links`, `correlation_change_candidates` tables with
  CHECK constraints and unique indexes.
- HTTP routes under `/api/v1/aiops/correlation`: `GET /rules`, `GET /cases`,
  `GET /cases/timeline`, `GET /cases/:id`, `GET /cases/:id/graph`,
  `GET /cases/:id/actions`. Read-only; case correlation is an internal
  operation, not HTTP-triggered.
- OpenAPI documentation for all 6 M42 routes and schemas.

### Added — M43 Cited And Evaluated AI Investigator

- `aiinvestigator` package with `InvestigatorVersion = "1.0"`, 3
  `InvestigationStatus` values (completed/failed/stale), 3
  `HypothesisConfidence` values (high/medium/low), 7 `EvidenceKind` values,
  `Investigation` with deterministic `investigation_key` (SHA-256 over
  case_id + investigator_version + prompt_hash), `Hypothesis`, `Citation`,
  `EvidenceRef`. See ADR 0058.
- `RunbookDescriptor` + compiled `catalog` map with 4 V1 runbooks:
  `rollback_last_rollout` (`deployment.rollback`),
  `rollout_restart_pods` (`deployment.rollout_restart`),
  `inspect_pvc_capacity` (advisory), `inspect_node_maintenance` (advisory).
  `LookupRunbook` fails closed; advisory runbooks always eligible; adding a
  runbook is a contract change.
- `BuildPrompt` assembles the system prompt (role, output schema, citation
  rules, runbook rules, prohibitions, prompt-injection defense) and the
  user prompt (redacted authorized evidence only — no raw logs/events/
  manifests). `ValidateProviderResult` enforces 8 rules; rejection is total
  — fabricated, out-of-scope or unauthorized citations discard the entire
  output.
- 10 golden validation fixtures (correct, insufficient, conflicting,
  prompt-injection, hidden-scope, fabricated-citation, ineligible-runbook,
  confirm-root-claim, empty-summary, no-citations). Each fixture is a
  deterministic (provider result, authorized evidence, eligible codes,
  expected valid/invalid + failure substring) pair.
- `GormRepository` with `Save` (insert + `MarkStale`), `Get`, `ListByCase`,
  `ListByFilter`. Partial unique index `uq_ai_investigations_active` on
  `(case_id, investigation_key) WHERE status != 'stale'`. `NopRepository`
  for testing/disabled mode.
- `Service` with `Investigate` (read case + eligible codes, build prompt,
  call provider, validate, persist completed/failed), `GetInvestigation`,
  `ListByCase`, `ListRunbooks`. On provider failure →
  `failed`/`provider_error`; on validation failure →
  `failed`/`citation_rejected` (provider summary retained for audit).
- Migration `000032_aiinvestigator` introducing `ai_investigations` table
  with CHECK constraints (status/tokens, completed-summary/
  completed-citations/failed-reason invariants) and a FK to
  `correlation_cases(id)` ON DELETE CASCADE.
- HTTP routes under `/api/v1/aiops/investigator`: `GET /runbooks`,
  `GET /cases/:case_id/investigations`, `GET /investigations/:id`,
  `POST /cases/:case_id/investigations`. The POST is the only write; it
  persists an investigation but never modifies the case/diagnosis/alert.
  Actor derived from the authenticated session.
- OpenAPI documentation for all 4 M43 routes and 8 schemas
  (InvestigatorRunbookList, InvestigatorRunbook, InvestigationListResponse,
  Investigation, InvestigationActor, InvestigationHypothesis,
  InvestigationCitation, EvidenceRef).

### Added — M41 SLO, Error Budget And Impact

- `slo` package with server-owned SLI templates (3 values:
  `request_success_ratio`, `request_latency_target_ratio`,
  `workload_readiness`), `MissingDataPolicy` (2 values), `EvaluationState`
  (5 values), `EvaluationCoverage` (3 values), `Definition` (versioned,
  enabled, bounded burn windows), and `Evaluation` (append-only,
  deterministic). See ADR 0056.
- `catalog` is the single source of truth for which templates exist, what
  they require and which missing-data policies they admit. The API never
  accepts raw PromQL, LogQL or arbitrary query languages.
- `Evaluator.Evaluate` is pure: same Definition + same MetricsSource output
  → same Evaluation. Counter resets detected as monotonicity violations and
  handled as "counter went to 0". Sparse data → `CoveragePartial`; no
  samples → `CoverageUnavailable`. Clock boundaries inclusive
  `window_start`, exclusive `window_end`. Missing data fail-closed by
  default; only `workload_readiness` may fail-open with explicit operator
  opt-in, and even then `Coverage` remains `Unavailable` (auditable).
  `classifyState` precedence: breached > burning_fast > burning_slow >
  healthy. Zero error budget (objective == 1.0) handled explicitly.
- `GormRepository` with `ON CONFLICT DO NOTHING` for idempotent evaluation
  inserts, partial unique index `uq_slo_definitions_active` for
  at-most-one-active-definition. `NopRepository` for testing/disabled mode.
- `Service` with `CreateDefinition` (version=1), `PatchDefinition` (actor
  required, version increment), `DeleteDefinition` (enabled=false, row
  retained), `EvaluateSLO` (404 > 503 precedence, persists unavailable,
  emits `BurnTransition` to `BurnAlertSink` only on state change, sink
  best-effort), and bounded list/read helpers.
- `BurnAlertSink` integration point for the M27 alert lifecycle. The SLO
  service never creates alert Rules — it only emits lifecycle transitions
  for the sink to translate into alert instances. Default `NopBurnAlertSink`.
- Migration `000030_slo_definitions_and_evaluations` introducing
  `slo_definitions` (CHECK constraints on template/policy/objective/window/
  burn bounds, partial unique active index, query indexes) and
  `slo_evaluations` (CHECK constraints on state/coverage/window/event-count/
  ratio bounds, query indexes) tables.
- HTTP routes under `/api/v1/aiops/slos`: `GET /templates`, `GET /`,
  `POST /` (SystemOpsAdmin), `GET /:id`, `PATCH /:id` (SystemOpsAdmin),
  `DELETE /:id` (SystemOpsAdmin), `POST /:id/evaluate` (SystemOpsAdmin),
  `GET /:id/evaluations`. Read-only routes open to any authenticated user;
  M35 scope enforced via cluster_id binding at create time and middleware on
  underlying Kubernetes resources.
- OpenAPI documentation for all 8 M41 routes and 10 schemas.

### Added — M40 Temporal Topology And Change Intelligence

- `topology` package with 8 reviewed `EdgeKind` values (Owns/Selects/
  RoutesTo/BackedBy/RunsOn/Mounts/Scales/ProtectedBy), 8 `DerivationMethod`
  values, `ResourceCitation` (cluster_id + kind + UID primary key; name-only
  marked incomplete), `Edge` with validity interval, and `ChangeEvent` with
  confidence/source. See ADR 0055.
- `Collector` snapshots 8 Kubernetes resource types with bounded paging
  (1000-page safety cap) and deterministically derives all 8 edge kinds from
  exact observed evidence (OwnerReference, label selector, EndpointSlice,
  Ingress backend, nodeName, PVC mount, HPA scaleTargetRef, PDB selector).
  Same-name/temporal proximity never creates an edge.
- `GormRepository` with `ON CONFLICT DO UPDATE` for edge refresh and
  change-event idempotency. Partial unique index
  `uq_topology_edges_active` enforces at-most-one-active-edge per identity.
  `NopRepository` for disabled/testing mode.
- `Service` with `CollectNamespace` (snapshot → derive → upsert → close
  stale), `CollectCluster`, `GetTopologyGraph` (nodes from edge endpoints +
  completeness indicator), `GetChangeTimeline`, and `IngestChangeEvent`
  (validated persistence).
- `ChangeNormalizer` pure mapping function from `ChangePlanInput`/
  `AuditChangeInput` to `ChangeEvent`. Domain statuses normalized
  (succeeded/failed/expired/partial/awaiting_confirmation/executing →
  succeeded/failed/failed/partial/pending/pending). Confidence high for
  platform+audit_id, low otherwise.
- Migration `000029_topology_edges_and_change_events` introducing
  `topology_edges` (partial unique active index, query indexes, CHECK
  constraints) and `change_events` (idempotent plan_id index, CHECK
  constraints) tables.
- HTTP routes `GET /api/v1/aiops/topology/graph` and
  `GET /api/v1/aiops/topology/changes`. Read-only; require authentication;
  M35 scope filtering applied by middleware. Bounded limits (graph 500,
  timeline 200) with truncation disclosed.
- OpenAPI documentation for both M40 routes and 8 schemas.

### Added — M39 Unified Service Identity and Signal Model

- `signal` package with `Occurrence` envelope, `SignalDescriptor` catalog (28
  signal codes across 7 domains), `ResourceCitation` (cluster_id + kind + UID
  primary key; name-only marked incomplete), and `EvidenceRef` (stable,
  redacted evidence pointers). See ADR 0054.
- Fingerprint-based deduplication: SHA256 over identity fields (excluding
  ObservedAt) with unique DB index + ON CONFLICT DO UPDATE ensures duplicate
  producer delivery yields one row.
- Normalizers for diagnosis (11 rules), alert (firing/resolved), metric
  (sustained breach), posture (4 finding codes), and change outcomes
  (promotion/backup/maintenance/restore × succeeded/failed).
- `signal.Service` with `Ingest` (fail-closed for unregistered signals),
  `IngestBatch`, `List`, `Overview`, and `CleanupRetention`. `SourceReader`
  interface for overview aggregation; `NopSourceReader` default.
- Migration `000028_signal_occurrences` introducing the
  `signal_occurrences` table with unique fingerprint index, query indexes,
  and CHECK constraints.
- HTTP routes `GET /api/v1/aiops/overview`, `GET /api/v1/aiops/signals`,
  `GET /api/v1/aiops/signals/catalog`. All require authentication; M35 scope
  filtering applied by middleware.
- `SignalConfig` in `backend/internal/config` with fail-closed validation;
  disabled by default.
- OpenAPI documentation for all 3 M39 routes and 8 schemas.

### Added — M37 Capability Plane Adapters

- `capability` package with `MetricsProvider` and `LogProvider` interfaces,
  Prometheus and Loki adapters, and `Nop*` defaults. Public APIs accept
  fixed template/query AST fields only — never PromQL, LogQL or arbitrary
  labels. See ADR 0053.
- `alertroute` package with route priority (1..100), exact cluster/rule/
  severity match, dedupe key, group/repeat interval, HTTPS webhook
  receiver, time-bounded silences (5m..7d, reason required, permanent
  forbidden) and idempotent delivery with retry and dead-letter.
- Migration `000027_alert_routes_and_silences` introducing
  `alert_route_receivers`, `alert_routes`, `alert_silences` and
  `alert_route_deliveries` tables.
- HTTP routes `GET /api/v1/capability/metrics` and
  `POST /api/v1/capability/logs` for M37A; 10 alert-route endpoints under
  `/api/v1/alert-routes/` for M37B. SystemOpsAdmin role required for
  mutations; deliveries restricted to SystemSecurityAudit.
- `CapabilityConfig` and `AlertRouteConfig` in `backend/internal/config`
  with fail-closed validation; both disabled by default.
- OpenAPI documentation for all 12 M37 routes and their schemas.

### Added — M38 Engineering, Delivery and Supply-Chain Hardening

- CI workflow now runs `go test -race -p=1 -count=1 ./...` on every pull
  request.
- CI workflow enforces a 50.0% backend coverage baseline via
  `go tool cover -func`.
- CI workflow runs `golangci-lint@v2.12.2` with a project-specific
  `.golangci.yml` configuration.
- CI workflow runs `pnpm lint` against the new flat ESLint config
  (`eslint.config.js`) covering TypeScript and Vue.
- CI workflow runs `oasdiff breaking --fail-on ERR` on pull requests to
  reject OpenAPI breaking changes against the base branch.
- Real-kind E2E workflow now covers the M23 release lifecycle, M24 cross-
  cluster promotion, M25 workload protection, M27 alert lifecycle, M28
  backup creation, M29 namespace posture, M30 node maintenance and M31
  isolated restore rehearsal suites.
- Official Helm chart under `deploy/helm/aiops-platform/` with `Chart.yaml`,
  `values.yaml`, `values.schema.json` and templates for namespace, service
  accounts, ConfigMap, PostgreSQL StatefulSet, backend/frontend Deployments,
  Ingress and NetworkPolicies.
- Helm chart contract tests covering metadata, values structure, schema,
  required templates, security baseline and the "no generated Secret" rule.
- `SECURITY.md` describing supported versions, the vulnerability reporting
  process, the threat-model boundaries and the supply-chain controls.
- License allowlist enforcement for production dependencies.
- SBOM generation step in the release workflow.
- Multi-architecture container build (linux/amd64, linux/arm64) for the
  backend and frontend images.

### Added — M67 NetworkPolicy / Connectivity Read-Only Analyzer (P1-①)

- **`backend/internal/netpolicy`**: read-only NetworkPolicy posture analyzer
  (committed `a8f4039`). Evaluates pod connectivity against NetworkPolicies
  and emits `internal/finding`-shaped findings. `CollectNetwork` feeds the
  collector; `POST /api/v1/optimization/network/analyze` (audit
  `optimization.network.analyze`) exposes it; frontend adds a "网络" tab.

### Added — M68 Image Supply-Chain / Reproducibility Read-Only View (P1-③)

- **`backend/internal/imagepolicy`**: read-only image supply-chain and
  reproducibility analyzer (committed `a50cd52`). Surfaces unsigned / missing
  digest / non-reproducible image references. `POST /api/v1/optimization/image/analyze`
  (audit `optimization.image.analyze`) exposes it; frontend adds a "镜像供应链"
  tab.

### Added — M69 GitOps Configuration-Drift Read-Only Detector (P1-④)

- **`backend/internal/gitopsdrift`** (committed `f014afa`): compares applied
  manifests against the GitOps source of truth and reports drift. `CollectGitOps`
  + `POST /api/v1/optimization/gitops/analyze` (audit `optimization.gitops.analyze`);
  frontend adds a "GitOps 漂移" tab (managed / drifted / unmanaged + drift rate
  + findings).

### Added — M70 Capacity Trend Prediction Read-Only Analyzer (P1-⑤)

- **`backend/internal/capacity`** (committed `9b2e919`): least-squares
  projection of CPU/memory usage over a horizon (default 30d); flags imminent
  saturation. `CollectCapacity` + `POST /api/v1/optimization/capacity/analyze`
  (audit `optimization.capacity.analyze`); frontend adds a "容量预测" tab.

### Added — M71 Policy-as-Code Read-Only Analyzer (P2-①)

- **`backend/internal/policy`** (committed `15b8f12`): declarative baseline
  (KubeSphere-style) checking cpu/mem requests+limits, privileged,
  allowPrivilegeEscalation, runAsNonRoot, probes, host namespaces — no Rego/OPA
  engine. `CollectPolicy` + `POST /api/v1/optimization/policy/analyze` (audit
  `optimization.policy.analyze`); frontend adds a "策略合规" tab. Probe presence
  detected via `*json.RawMessage`.

### Added — M72 Topology Collection Parallelization (P2-②)

- **`Collector.Snapshot`** (committed `da58511`) concurrently fetches 8 resource
  kinds via `WaitGroup` + mutex-guarded first-error; `Service.CollectCluster` uses
  a bounded per-namespace worker pool (default concurrency 4, `WithNamespaceConcurrency`
  option). A data race in the test stub counter (`countingRepository.upsertCount`)
  under the parallel worker was fixed with `atomic.Int64` in `be8ecbd` after the
  CI `go test -race` gate caught it (unreproducible locally under `CGO_ENABLED=0`).

### Added — M73 M46–M58 kind E2E Suite (P2-③)

- **`scripts/e2e-m46-m58-kind.ps1`** (committed `dd82e54`, race fix `be8ecbd`):
  first post-M45 kind E2E suite, asserting M46 workspace CRUD, M48 federation
  overview, M52 inspection catalog + plan lifecycle, M56 quality report, M57
  app-catalog plans, M58 copy-plans. Registered in `real-kind-e2e.yml`.

### Added — M76 HPA Scaling-Posture Read-Only Analyzer

- **`backend/internal/hpa`** (committed `b405730`): evaluates `autoscaling/v2`
  HPAs — missing target metric, replicas at `maxReplicas`, thin `maxReplicas`,
  over/under-target utilization. `CollectHPA` + `POST /api/v1/optimization/hpa/analyze`
  (audit `optimization.hpa.analyze`); frontend adds an "HPA 扩缩容" tab.
  See [M76 change record](docs/changes/2026-08-02-m76-hpa-scaling-posture.md).

### Added — M77 PodDisruptionBudget Protection Read-Only Analyzer

- **`backend/internal/pdb`** (committed `40877fc`): evaluates PDBs — missing PDB,
  missing `minAvailable`/`maxUnavailable`, `maxUnavailable=100%`, allow-voluntary
  annotation. `IntOrString` decoded via `json.RawMessage` + `rawToText`. `CollectPDB`
  + `POST /api/v1/optimization/pdb/analyze` (audit `optimization.pdb.analyze`);
  frontend adds a "PDB 保护" tab.
  See [M77 change record](docs/changes/2026-08-02-m77-pdb-protection.md).

### Added — M78 Ingress Exposure-Surface Audit Read-Only Analyzer

- **`backend/internal/ingressposture`** (committed `675ca54`): evaluates Ingresses
  — no TLS, dead backend Service, wildcard host, missing `ingressClassName`.
  `CollectIngress` + `POST /api/v1/optimization/ingress/analyze` (audit
  `optimization.ingress.analyze`); frontend adds an "Ingress 暴露面" tab. This
  completes the M76–M78 analyzer trio; the optimization center now exposes **11
  analyzer tabs**.
  See [M78 change record](docs/changes/2026-08-02-m78-ingress-exposure-audit.md).

### Changed

- M33: migrated Kubernetes API interactions from raw HTTP to client-go
  v0.34.x. See ADR 0048.
- M34: introduced the `RouteDescriptor` contract as the single source of
  truth for routing, authentication, authorization and audit metadata.
  Added the bounded RBAC read-only inventory. See ADR 0049.
- M35: added lightweight cluster and namespace access grants with a policy
  evaluator and authorization middleware. Authorization failures return
  404. See ADR 0050.
- M36: added production OIDC provider (Authorization Code + PKCE S256, JWKS
  cache, MFA evidence, GORM identity resolver, session management, break-glass
  drill). OIDC disabled by default. See ADR 0052.
- M37: added capability plane adapters (Prometheus/Loki) and alert routing
  with bounded silences. All adapters disabled by default. See ADR 0053.
- M39: added unified signal model normalizing M21-M31 outputs into
  `signal_occurrences` with fingerprint dedup. Native signal path unchanged.
  See ADR 0054.
- M40: added temporal topology graph (8 reviewed edge kinds with validity
  intervals) and unified change timeline normalizing M23-M31 platform
  operations. Native signal path unchanged. See ADR 0055.

## [baseline-m35-20260731] — 2026-07-31

### Added — M35 Lightweight Cluster And Namespace Access Grants

- Migration `000025_access_grants` introducing `user_cluster_grants` and
  `user_namespace_grants` tables.
- `authz` package with a policy evaluator (`Service`) and a
  `GrantManager` for grant CRUD.
- Authorization middleware (`requireClusterAccess`,
  `requireNamespaceAccess`) wired into fleet, search and resource routes
  carrying the `:cluster_id` or `:namespace` path parameter.
- Fleet and global search services now filter results by the caller's
  visible clusters.
- Grant management REST API under `/api/v1/authz/grants`.
- OpenAPI tag `access-grants` with the corresponding schemas.

See `docs/changes/2026-07-31-m35-lightweight-cluster-and-namespace-access-grants.md`
and ADR 0050.

## [baseline-m34-20260731] — 2026-07-31

### Added — M34 Route Descriptor Contract and RBAC Inventory

- `RouteDescriptor` registry as the single source of truth for routing,
  authentication, role requirements and audit classification.
- Bounded RBAC read-only inventory exposing Role, ClusterRole, RoleBinding
  and ClusterRoleBinding projections.
- Documentation refresh: README, `docs/architecture/overview.md`,
  `docs/development-handoff.md`, `docs/thesis/test-matrix.md`.

See `docs/changes/2026-07-31-m34-route-descriptor-and-rbac-inventory.md`
and ADR 0049.

## [baseline-m33-20260731] — 2026-07-31

### Changed — M33 Restricted client-go Migration

- Replaced raw HTTP Kubernetes interactions with client-go v0.34.x while
  preserving all API contracts, desensitization, preconditions and timeouts.

See `docs/changes/2026-07-31-m33-restricted-client-go-migration.md` and
ADR 0048.

## [baseline-m32-20260731] — 2026-07-31

### Final Baseline Archive

- Final defect fixes, ADR/API/RBAC alignment, responsive UI repair and the
  M27-M31 disposable real-environment suites. Fresh repository evidence at
  `.artifacts/verification/verify-20260731-015255.json`.

See `docs/changes/2026-07-31-final-baseline-archive.md`.

## Earlier milestones

Earlier milestones (M1 through M31) are documented in
`docs/changes/` and in the project roadmap (`docs/roadmap.md`). Each
milestone record lists the ADRs, migrations and acceptance evidence
captured at that baseline.
