# Changelog

All notable changes to the aiops-platform project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Milestones are released as git tags of the form `baseline-mNN-YYYYMMDD`.
Detailed change records for each milestone live under `docs/changes/`.

## [Unreleased]

### Added - M115-1u optimization finops/no-inputs/rate/auto-collect 分支

- 扩展 `internal/httpserver/optimization_test.go`：FinOpsNoInputsNoCollector400、FinOpsRateOverride、CISNoInputsNoCollector400、DeprecatedAPIRequiresCluster。
- See [change record](docs/changes/2026-08-14-m115-1u-optimization-finops-branches.md)。

### Added - M115-1t diagnosis create 处理器 0% → 全覆盖（validation/error-map/成功路径）

- 扩展 `internal/httpserver/diagnosis_handler_test.go`：diagSourceStub 家族 + performDiagnosisCreate；CreateValidationBranches（4 校验）、CreateErrorMapping（5 错误映射）、CreatePodSuccess（OOMKilled 201）、CreateDeploymentSuccess。
- See [change record](docs/changes/2026-08-14-m115-1t-diagnosis-create-handler.md)。

### Added - M115-1s kubernetes PersistentVolume/NetworkPolicy/ServiceAccount 0% 清零（6 函数）

- 扩展 `internal/kubernetes/rollout_test.go`：PV/NetworkPolicy/ServiceAccount 的 list+detail（含 cluster/namespace path 分支）。
- See [change record](docs/changes/2026-08-14-m115-1s-kubernetes-pv-netpol-sa.md)。

### Added - M115-1r automation approve 处理器成功路径 + 未预览 409 分支

- 扩展 `internal/httpserver/automation_test.go`：approveRepoStub（内嵌 NopRepository）+ ApproveSuccess（single 审批 200 含 audit 分支）+ ApproveNotPreviewed（409）。
- See [change record](docs/changes/2026-08-14-m115-1r-automation-approve-handler.md)。

### Added - M115-1q appcatalog Get/List repo、GetPlan、validRepoURL、extractCredentials、构造器（4 个 0% 函数清零）

- 扩展 `internal/appcatalog/service_test.go`：GetRepository/ListRepositories/GetPlan（含 not-found）、validRepoURL 边界、extractCredentials 三分支、NewService/NewTestService/NewHTTPIndexSource 构造器。
- See [change record](docs/changes/2026-08-14-m115-1q-appcatalog-crud-queries.md)。

### Added - M115-1p copyops Execute 校验 + failPlan 错误分支（validation/NSIdentity/destNsExists/createRes）

- 扩展 `internal/copyops/service_test.go`：ExecuteValidationBranches（短 planID/空 token/空 idempotency）、ClaimNotFound、NSIdentityError（first-call 模式）、DestNamespaceDeleted、CreateResourceError（dryRun 成功/real 失败）。
- See [change record](docs/changes/2026-08-14-m115-1p-copyops-execute-branches.md)。

### Added - M115-1o remediation List/ListOperations 0% 清零 + 校验边界分支

- 扩展 `internal/remediation/service_test.go`：List 诊断校验、ListOperations 三非法输入 + 合法、RolloutHistory/RolloutStatus 非法输入与 kube 错误传播、validResourceName/validContainerImage 边界。
- See [change record](docs/changes/2026-08-14-m115-1o-remediation-list-validations.md)。

### Added - M115-1n incident 处理器分支全覆盖（list/metrics/batch/follower/note/postmortem/export）

- 扩展 `internal/httpserver/incidents_test.go`：引擎注册 context/export/followers/notes/postmortem；List/Metrics 校验 10+6 非法 query、BatchAssign 3 校验分支、AddFollower/RemoveFollower/AddNote/SetPostmortem 全错误分支、Export/ExportPostmortem 404+200。
- See [change record](docs/changes/2026-08-14-m115-1n-incident-handler-branches.md)。

### Added - M115-1m workspace 处理器错误分支（nil-service 503 全路由 + membership/grant 错误注入）

- 扩展 `internal/httpserver/workspace_test.go`：handlerFakeRepo 加 addMembershipErr/removeMembershipErr/grantErr；13 路由 nil-service 503 巡检、list 过滤、membership 409/404/400、duplicate 409、get 404、grant 缺失 404。
- See [change record](docs/changes/2026-08-14-m115-1m-workspace-error-branches.md)。

### Added - M115-1l optimization RBAC/kubernetesLister/NodeUsageSource 分支（两个 0% 函数清零）

- 扩展 `internal/optimization/collector_test.go`：collectRBAC 全字段（cluster/namespaced bindings）+ list 失败、NewKubernetesLister.List 三错误分支 + happy path、NewNodeUsageSource/NodeUsageSeries（errSeriesRepo）。
- See [change record](docs/changes/2026-08-14-m115-1l-optimization-rbac-lister-source.md)。

### Added - M115-1k kubernetes 分页/patch 解码/指标路径分支（包覆盖率 72.5% → 73.1%）

- 扩展 `internal/kubernetes/rollout_test.go`：Namespaces 过滤+分页+Remaining、Nodes/NodeMetrics 响应、PodMetrics 容器归一化与 namespace 路径、三组 Patch 解码错误、PatchDeployment 双层错误（disabled/网关）。
- See [change record](docs/changes/2026-08-14-m115-1k-kubernetes-pagination-patch-branches.md)。

### Added - M115-1j incident assign/batch-assign 处理器分支（assign 0% → 100%）

- 扩展 `internal/httpserver/incidents_test.go`：测试引擎注册 assignment 路由；AssignSuccessAndErrors（400/200/409/404）、AssignInvalidID、BatchAssignTooMany、EvidenceForMissingIncident。
- See [change record](docs/changes/2026-08-14-m115-1j-incident-assign-handler.md)。

### Added - M115-1i promotion Get/List 查询面测试（0% 函数清零）

- 扩展 `internal/promotion/service_test.go`：previewForGetTest 建 plan + Get（命中/ErrNotFound）、List（命中/非法 clusterID）。
- See [change record](docs/changes/2026-08-14-m115-1i-promotion-get-list-branches.md)。

### Added - M115-1h copyops Get/ListByUser/ListByCluster 查询面测试（0% 函数清零）

- 扩展 `internal/copyops/service_test.go`：previewK8sFake 共享 fake + Get（命中/空 ID 拒绝）、ListByUser（多用户筛选/非法 userID）、ListByCluster（命中/非法 clusterID）、NewService 默认构造。
- See [change record](docs/changes/2026-08-14-m115-1h-copyops-get-list-branches.md)。

### Added - M115-1g httpserver/kubernetes.go 资源处理器覆盖率 9.6% → 87.6%（~70 个 handler）

- 新增 `internal/httpserver/kubernetes_handler_test.go`：k8sCredStub+k8sGetStub 内存 gateway 构造真实 service；30 个 list + 27 个 detail/manifest handler 直测；disabled cluster 409、logs/logsSince/all_logs 参数校验 400、customResources 缺参 400/未白名单 404 分支。全局覆盖率 ~67.7% → ~68.9%（差 ~285 stmts 到 70%）。
- See [change record](docs/changes/2026-08-14-m115-1g-kubernetes-handler-coverage-to-87.md)。

### Added - M115-1f alertroute 配置/错误分支测试（WithCipher/ConfigureDelivery/UpdateRoute 校验/repo 错误传播）

- 扩展 `internal/alertroute/service_test.go`：WithCipher 返回 receiver、ConfigureDelivery 正值应用与 0 值 no-op、CreateReceiver 不泄露明文 secret、DeleteReceiver/DeleteRoute repo 错误传播、UpdateRoute 三组越界校验（priority 上下界/groupInterval/repeatInterval）+ 缺失元素错误。
- See [change record](docs/changes/2026-08-14-m115-1f-alertroute-config-error-branches.md)。

### Added - M115-1e namespaceposture 覆盖率 ~54% → 90.2%（workload 五种 fetcher 全分支）

- 扩展 `internal/namespaceposture/service_test.go`：新增 sts/ds/job/cronjob 构造器；Get 全 workload 聚合（deployment+sts+ds+job+cronjob 同时复数据）、workload fetcher 部分失败 → PartialSections best-effort、List/collectPods/collectNodeCapacity 错误分支、全部 fetcher 失败 → errPartial + SourcePartial、单 section 失败聚合。
- See [change record](docs/changes/2026-08-14-m115-1e-namespaceposture-coverage-to-90.md)。

### Added - M115-1d kubernetes 包覆盖率 64.5% → 72.5%（rollout/replica-set/node/workload 读写路径）

- 新增 `internal/kubernetes/rollout_test.go`：ReplicaSetsByOwner 过滤 + RolloutHistory 全流程（双请求、revision 排序/跳过、错误分支）、RolloutStatus（ProgressDeadlineExceeded phase 推导、默认 replicas=1、空条件归一）、PatchNode（成功/dryRun/非 PatchGateway fail-fast/disabled）、WorkloadTemplate.UnmarshalJSON（Raw 保留）；10 个列表函数（Namespaces/Nodes/Deployments/StatefulSets/DaemonSets/RS/Jobs/CronJobs/HPA/NodeMetrics）+ PatchDeployment/PatchCronJob 的 gateway-error 与 disabled-cluster 分支。
- See [change record](docs/changes/2026-08-14-m115-1d-kubernetes-coverage-to-72.md)。

### Added - M115-1c automation 覆盖率 46.5% → 66.9%（Execute/Verify 全生命周期测试）

- 扩展 `internal/automation/service_test.go`：Execute 全生命周期（execRepo 内存 Claim/Complete/Fail/SaveVerification）——happy path Approved→Succeeded + 验证调度、patch 失败→Failed 仍调度验证、gate recheck fail-closed、错误 confirmation token；Verify happy path（SLO 改善 + rollout_restart 后快照 → Effective + MarkVerified）；Preview 全分支（disabled/not-draft/k8s 缺失/成功过渡 previewed）；materializeParameters 全分支（rollback 历史/无回滚点/no-change/缺 override/cronjob.suspend/unsupported action）；refreshSnapshot（Deployment/CronJob 成功 + 错误分支）；GetPlan。
- See [change record](docs/changes/2026-08-14-m115-1c-automation-coverage-to-67.md)。

### Added - M115-1b httpserver 覆盖率 66.1% → 70.0%（全局最大单包顶到 70% 门禁线）

- 新增 `internal/httpserver/clusters_test.go`：clusterHandler 全六 handler + clusterID util（list/get/create/setEnabled/updateCredential/probe/delete 全部错误哨兵：400/404/502/500 与 happy path 200/201/204）。
- 扩展 `event_cockpit_test.go`（parseCockpitRequest 默认/显式/NaN/越界全分支 + parseEventTime 四分支）、`workspace_test.go`（listWorkspaces/updateWorkspace/deleteWorkspace/listMemberships/getQuota/listRoleBindings/revokeRole 七个 0% handler）、`inspection_test.go`（getPlan/deletePlan/listTasks/getTask/listResults/getResult/effectiveRules + 全 handler nil-service 503 巡检）、`aiexplain_handler_test.go`（quality/coverage 错误→500）、`alert_handler_test.go`（deleteRule 404/404/500/400 分支）。
- See [change record](docs/changes/2026-08-14-m115-1b-httpserver-coverage-to-70.md)。覆盖率门禁 65% 暂保持，70% 上调随后续切片累计完成后一并修改 ci.yml。

### Added - M115-1 fuzz 状态机扩展与覆盖率首片（automation plan/rollback + SLA monitor）

- 新增 fuzz：`internal/automation/FuzzPlanLifecycle`（action plan 状态机：approve/cancel/expire 任意状态下仅文档化哨兵错误、不 panic）、`FuzzRollbackContract`（仅 Deployment image_update/rollout_restart 可创建 rollback 计划）；`internal/incident/FuzzSLAMonitorStateMachine`（任意 escalation 窗口下 first<final 单调契约 + payload 合法性 + deep_link）。
- CI fuzz seed smoke 包列表加入 `./internal/automation/`（12 包全绿）。
- 覆盖率提升：`internal/signal` 63.9%→70.1%、`internal/correlation` 67.2%→70.7%（service/metrics/factors/merge/provider 分支）；`internal/incident` 57.0%→61.7%（Service.Metrics/ResponseCatalog/导出/跟随者校验等）。
- See [change record](docs/changes/2026-08-14-m115-1-fuzz-expansion-and-coverage-slice.md)。覆盖率门禁 65% 暂保持，70% 上调随后续切片（M115-1b/c）累计完成后一并修改 ci.yml。

### Added - M114-3 指标历史下采样归档（7天精确 → 30天下采样小时档，有界查询）

- 新增迁移 `000049_metric_samples_downsampled`：30 天下采样归档表（每系列每小时一条 avg/max/count，幂等 upsert；表约束有意不重演精确表 Node/Pod 的历史不一致）。
- `internal/metricshistory` 扩展：`Service.QueryArchive`（只读，窗口 ≤30d、点 ≤1440）、`DownsampleAndArchive`（清理前把过期精确样本聚合进归档）、`Cleanup` 先归档再删除精确行；`Repository` 新增 `QueryArchiveSeries`/`SaveDownsampledBatch`/`ListExpiringSamples`。全部有界，无全量写入路径。
- 新增只读 `GET /api/v1/clusters/:cluster_id/metrics/history/archive?resource_kind=&name=&metric=&from=&to=&limit=`（AuditAction `metrics.history.archive.read`）；OpenAPI + 权限矩阵同步。
- 前端"指标历史"面板时间范围扩展为 1h/6h/24h/7d/30d：≥7d 自动走归档端点，并标注 `下采样(小时档)`；新增 `getMetricHistoryArchive` API + 客户端测试。
- See [change record](docs/changes/2026-08-14-m114-3-metric-history-downsampled-archive.md)。M114-4（事件流/日志探索增强）按路线调整归入后续增强，不在本次 M114 基线内。

### Added - M114-1 SLO Burn 总览与告警降噪聚合（两个只读聚合端点，复用 correlation 引擎）

- 新增只读 `GET /api/v1/aiops/slos/burn-summary?cluster_id=&namespace=&template=&state=&limit=`：纯读 SLO 定义 + 最新评估，计算每定义 burn posture（burning / healthy / unavailable / no_data）、burn rate、coverage、错误预算剩余；排序燃烧优先，limit ≤ 200，无写路径。
- 新增只读 `GET /api/v1/clusters/:cluster_id/alerts/overview?window_minutes=&max_groups=&limit=`：告警降噪——按规则聚合实例（firing/resolved 计数、首末触发），并关联活跃 correlation case（资源 kind+name 匹配 → related_case_ids 深链）；fail-closed 空窗不视为健康。
- 新增纯包 `internal/alertoverview`、`internal/sloburnsummary`（ADR 0004，无 cluster 访问、无副作用）；全部查询有界（window 1–10080 / max_groups 1–200 / limit 1–200）。
- 前端"告警规则"页新增告警降噪面板（窗口切换、统计卡、聚合表、`/correlation?case=N` 深链）；"SLO 仪表盘"新增 Burn 总览卡片区（状态色、Burn ×率、Ratio、Budget）。
- See [change record](docs/changes/2026-08-14-m114-1-slo-burn-and-alert-noise-reduction.md)。

### Added - M114-2 事件驾驶舱（重复事件按严重级/原因/资源聚合 + 趋势 + 深链）

- 新增只读 `GET /api/v1/clusters/:cluster_id/events/cockpit?window_minutes=&max_groups=&page_limit=`：把 Kubernetes 原生事件按（严重级 warning/info、原因、命名空间、资源类型与名称）分组折叠，输出去重计数（event_count + raw_count 累计次数）、首次/最近发生时间、按天趋势与原始证据深链（resource_uid/kind/namespace/name + sample_message）。
- 新增纯包 `internal/eventcockpit`（ADR 0004，无 cluster 访问、无写路径）：`Aggregate()` 窗口外事件过滤、严重级归一化、稳定排序后截断、按天趋势桶；fail-closed——窗口内无事件一律 `fail_closed=true` 且不视为健康（M99-D 契约）。
- 全部查询有界：window_minutes（1–10080，默认 1440）、max_groups（1–200，默认 50）、page_limit（1–1000，默认 500）逐一校验（400 INVALID_*）。
- 事件"事件中心"页新增"事件驾驶舱"面板：窗口切换（1h/6h/24h/7d）、fail-closed 黄色告警、三统计卡、聚合组表格（级别徽标/原因/资源/首末时间/折叠提示）、按天趋势柱形图（hover 显示日期/事件数/组数）；新增 `getEventCockpit` API + 客户端测试。
- 授权复用 M35 命名空间粒度：AllNamespaces 查一次集群页；仅命名空间授权逐命名空间取页（失败跳过）；无写库路径（事件仍实时查 API Server）。
- See [change record](docs/changes/2026-08-14-m114-2-event-cockpit.md)。

### Added - M113-3 巡检趋势与覆盖率度量（plan→findings 时间序列 + 规则命中覆盖率）

- 新增只读 `GET /api/v1/aiops/inspection/coverage?window_days=7|30|90`：跨 `inspection_plans / inspection_tasks / inspection_results` 聚合，输出计划数/启用数、任务总数/完成/失败、定时 vs 手动触发分布、发现总数、去重命中规则码、严重级别分布、规则覆盖率（命中规则码 / 编译期目录大小）与每日趋势（任务数+发现数）。
- 新增纯包 `internal/inspection` 的 `CoverageSummary`/`CoverageTrendPoint` 与 `Repository.Coverage + Service.Coverage`（ADR 0004，纯读 SQL 聚合、无写路径）；fail-closed：空窗口/无任务/无发现一律 `fail_closed=true`，空样本不视为健康（M99-D 显式覆盖度约定）。
- 前端"智能巡检"页新增"覆盖率与趋势"面板：窗口切换（7/30/90 天）、四指标卡片、严重级别分布表、纯 CSS 每日趋势柱形图（hover 显示日期/任务/发现数）；fail-closed 时显示黄色告警横幅。新增 `getInspectionCoverage` API + 客户端测试。
- 修复既有 M52 技术债：所有巡检路由（rules/catalog、plans CRUD、run、tasks、results、per-cluster rules）首次纳入 OpenAPI 文档与路由合约双向校验（`router_harness_test.go` 现注册 `InspectionService`），补齐 11 条路由 + 3 个 schema + 权限矩阵。
- See [change record](docs/changes/2026-08-14-m113-3-coverage-trend.md)。

### Changed - 明确基础设施项目的 Day 0/1/2 生命周期边界

- README 新增 `kubernetes-cluster-bootstrap`、`devops-automation` 与 `aiops-platform` 的职责矩阵，
  明确 AIOps 从已有 Kubernetes 集群开始，不负责操作系统初始化、containerd、kubeadm、CNI 或控制平面 HA。
- 新增 ADR 0088 和对应 change record，固定三个项目的入口、交接信息和不重复建设原则。
- See [change record](docs/changes/2026-08-14-lifecycle-boundaries.md)。

### Added - M113-2 容量感知预览（节点按剩余资源排序的只读适配评估）

- 新增只读 `POST /api/v1/optimization/capacity/preview`：输入候选工作负载资源请求（CPU/内存/GPU/存储），实时读取集群节点的 `status.allocatable` 与用量指标，评估每节点的剩余头寸并按适配度排序；逐约束输出"为什么适配/为什么不适配"（satisfied/violated/unknown）与数据更新时间。
- 新增纯包 `internal/capacitypreview`（ADR 0004，无 cluster 访问、无写路径）；缺样本约束为 `unknown` 且 fail-closed（带未知约束的节点不计为适配，空样本不视为健康）；返回 `scope`/`observed_at`/`freshness` 沿 M112 资源上下文契约。
- 前端优化中心容量 tab 新增"容量感知预览"面板（表单 + 适配结果表 + 数据观测时间），只读不创建任何资源。
- 同步 OpenAPI/typegen（`CapacityPreview` schema）、权限矩阵（`optimization.capacity.preview`）。
- See [change record](docs/changes/2026-08-14-m113-2-capacity-preview.md)。

### Added - M113-1 Finding → Runbook 预览导航（优化中心闭环第一步）

- 新增可复用只读组件 `FindingRunbookPanel.vue`：从任意 posture/optimization finding 一键跳转到对应 insight runbook（确定性诊断路由 → 巡检佐证 → AI 引用解释 deep-link → 受控操作 dry-run 预览），全程只读，零写操作。
- 优化中心 11 个分析器 tab（FinOps/CIS/废弃API/网络/镜像/GitOps漂移/容量/策略/HPA/PDB/Ingress）的 finding 行全部接入"查看闭环"导航，复用既有 M81 `GET /api/v1/aiops/insight` 端点（无新增后端路由）。
- 与 PostureView 既有闭环导航共享同一 M81 端点，行为一致；未选集群时按钮禁用不发请求，加载失败显示可重试错误。
- See [change record](docs/changes/2026-08-14-m113-1-finding-runbook-nav.md)。

### Changed - 公开 README 基线与项目入口说明

- 将旗舰仓库 README 首屏从 M93 更新至 M112，增加多集群运维、可观测诊断、事故响应、AI 辅助、
  受控运维和工程交付能力摘要。
- 修正 CI 覆盖率门禁的过时表述（60% → 65%），并明确当前 RC 边界、生产条件限制及论文答辩材料入口。
- 将历史安全脱敏说明和 M1-M32 早期路线下移，减少首屏维护信息对项目能力的干扰。
- See [change record](docs/changes/2026-08-14-public-readme-baseline.md)。

### Added - M112-4 解释覆盖率大盘（AI 解释可用率 / 引用率 / 降级率只读展示）

- 新增只读 `GET /api/v1/ai/coverage`，聚合 ai_explanations + ai_explanation_feedback 基线：解释可用率（有解释的 distinct 诊断）、引用率（带引用解释占比）、降级率（确定性 nop provider 占比）、质量反馈基线（好评率/贡献者/按模型）。
- 纯只读编排：零写操作、零 AI 调用、零数据库迁移；`window_note` 说明聚合口径（全量窗口）。
- 同步 OpenAPI/typegen（`AICoverage` schema）、权限矩阵（`ai_explanation.coverage.read`）、前端类型与客户端；新增只读大盘页 `/aiops/ai-coverage` 与侧栏「解释覆盖率」入口。
- 至此 M112 全部完成（1 上下文驾驶舱 / 2 会话式调查 / 3 引用式事故摘要 / 4 解释覆盖率大盘）。See [change record](docs/changes/2026-08-14-m112-4-ai-coverage.md)。

### Added - M112-3 引用式 AI 事故摘要（确定性阶段门 + 引用校验的自动摘要）

- 新增 `GET /api/v1/incidents/{incident_id}/summary`，一次性生成引用式事故摘要（root_cause_candidate / impact / evidence_summary / next_steps），所有事实断言引用事故证据时间线（同 M112-2 引用校验纪律）。
- 确定性阶段门：无证据或 AI 禁用时直接返回确定性摘要（StageGatePassed=false），不调用 provider；provider 故障或引用校验失败 → fail-closed 降级（fail_closed=true）。
- AI 可用且引用校验通过时 mode=ai；根因只能写候选，不允许声称已确认根因。
- 响应携带资源上下文契约块（scope/observed_at/source/freshness/empty_sample=fail_closed）。
- 同步 OpenAPI/typegen（`IncidentSummaryResponse`）、权限矩阵（`incident.summary.read`）、前端类型与客户端；事故详情抽屉新增「AI 事故摘要」区块（阶段门状态、引用列表、fail-closed 提示）。
- 无数据库迁移。See [change record](docs/changes/2026-08-14-m112-3-incident-summary.md)。

### Added - M112-2 会话式 AI 调查（事故上下文中引用校验的连续问答）

- 新增 `POST /api/v1/incidents/{incident_id}/chat`，在事故上下文中连续提问；每个回答将事实断言与事故证据时间线一一对应（M44 同款引用校验：未授权 evidence_id → fail-closed；禁止 prompt injection；citations ≤ 64、next_checks ≤ 8）。
- 无持久化设计（客户端持有 bounded 历史记录，每个请求独立），满足连续提问验收；若需服务端历史可后续迁移 `000049`。
- 响应携带跨 M112–M114 资源上下文契约块（scope/observed_at/source/freshness/empty_sample=fail_closed），mode 字段区分 `ai` / `deterministic`。
- AI 禁用或引用校验失败时确定性降级：引用真实 incident 记录，不伪造根因或集群状态；fail_closed=true 告知前端降级。
- 同步 OpenAPI/typegen（`IncidentChatRequest` / `IncidentChatResponse`）、权限矩阵（`incident.chat.create`）、前端类型与客户端；事故详情抽屉新增「会话式 AI 调查」区块。
- 无数据库迁移。See [change record](docs/changes/2026-08-14-m112-2-incident-chat.md)。

### Added - M112-1 事故上下文驾驶舱（资源上下文契约首次落地）

- 新增只读 `GET /api/v1/incidents/{incident_id}/context`，聚合事故快照、SLA 状态、证据来源汇总、最近 10 条时间线、runbook 摘要与只读 dry-run 建议动作，供值班人员在事故详情首屏完成判断。
- 首次落地跨 M112–M114 资源上下文契约：响应携带 `resource_context`（scope、observed_at、来源、freshness、空样本语义，空样本固定 `fail_closed`）。
- 驾驶舱为纯确定性只读聚合：不调用 Kubernetes API、不伪造集群健康；来源域缺失时 runbook 明确不可用（fail-closed）；实时集群健康经证据 deep link 与后续 M114 事件驾驶舱承载。
- 同步 OpenAPI/typegen、权限矩阵（`incident.context.get`）、前端类型与客户端；事故详情抽屉新增「上下文驾驶舱」区块。
- 无数据库迁移（只读聚合）。See [change record](docs/changes/2026-08-14-m112-1-context-cockpit.md)。

### Changed - 后续路线吸收资源上下文与容量感知能力

- 基于外部 K8s 运维平台调研，更新 M112–M114 执行路线：M112 增加事故上下文驾驶舱，
  M113 增加容量感知的 dry-run 预览，M114 增加事件聚合驾驶舱。
- 新增跨里程碑资源上下文契约，要求显式返回 scope、observed_at、来源、freshness 和空样本语义；
  外部工具仅使用权限校验后的深链/状态卡，不复制 iframe、默认凭据或越权操作模式。
- See [change record](docs/changes/2026-08-14-roadmap-resource-context-integration.md)。

### Fixed - 开源许可识别（NOASSERTION → Apache-2.0）

- 将 `LICENSE` 替换为规范 Apache License 2.0 全文并保留 `Copyright 2025-2026 Guiyi Labs` 声明，修复 GitHub 将许可证识别为 NOASSERTION 的问题（原文件被截断改写，缺 4.4/4.6/4.7 条款与 APPENDIX 段）。
- See [change record](docs/changes/2026-08-14-license-apache2.md)。

### Added - M111 事故响应模板与复盘 Markdown 导出

- 新增只读 `GET /api/v1/incidents/templates`，返回 Node NotReady、Deployment Unavailable、OOMKilled、通用事故模板和当前严重级 SLA 矩阵；创建事故可携带 `template_id`，模板默认值与来源解析保持兼容。
- 事故快照持久化 `template_id`；严重级目标支持 `INCIDENT_SLA_TARGETS` 环境配置，默认 critical/high/warning/info 为 1h/4h/24h/72h，SLA 截止时间按矩阵计算。
- 新增 `GET /api/v1/incidents/{incident_id}/postmortem/export`，输出包含事故叙事、证据时间线、决策/动作时间线和结果指标的 Markdown；事故详情支持下载，CSV 导出同步带出模板标识。
- 同步迁移 `000048`、OpenAPI/typegen、权限矩阵、前端模板选择和部署配置；切换响应模板会同步刷新标题、摘要和严重级默认值。
- See [change record](docs/changes/2026-08-14-m111-incident-templates-postmortem-export.md)。

### Added - M111 事故响应 KPI 基础层

- 新增只读 `GET /api/v1/incidents/metrics`，按时间窗口和可选集群派生事故的首次指派耗时、
  MTTA、MTTR、解决事故 SLA 达标率和逾期数量。
- 指标严格使用已有事故时间戳与 append-only 时间线；默认 30 天、最多 90 天、最多 200 条样本，
  响应披露 `sample_limit` / `sampled` / `truncated`，无有效样本返回 `null`。
- 同步 OpenAPI、前端 typegen 与 `getIncidentMetrics` 客户端封装；后端全量测试/lint、前端 143 测试
  与 `pnpm ui:gate` 4/4 全绿。
- See [change record](docs/changes/2026-08-14-m111-incident-kpi.md)。

### Added - M111 事故详情只读 Runbook 关联

- 新增 `GET /api/v1/incidents/{incident_id}/runbook`，复用现有 M81 Insight 映射，返回诊断、巡检、AI
  解释与 dry-run 候选；该接口不执行集群操作，写路径仍由既有受控动作目录承载。
- 来源解析补充可信 `domain` / `finding_code`，人工来源、源记录缺失或跨集群来源无法确认时返回
  `available=false`，不猜测故障域；诊断来源同步增加集群归属校验。
- 事故详情抽屉新增响应步骤区块，并同步 OpenAPI、typegen、权限矩阵与 API 客户端。
- See [change record](docs/changes/2026-08-14-m111-incident-runbook.md)。

### Added - M111 事故 SLA 有界升级链

- 事故 SLA 监控在临近/逾期提醒之后增加首次、最终两级升级；默认在逾期 30 分钟和 2 小时触发，
  仅针对仍处于 `open` / `confirmed` 的未解决事故，级别有界且可配置。
- 通知 outbox 增加 `escalation_level` 与 `(incident_id, event_type, escalation_level)` 幂等键，
  升级 payload 记录级别、阶段、原因；重复扫描不会重复投递，通知记录可作为升级审计。
- 通知查询支持 `incident.sla_escalated` 与 `escalation_level` 过滤，前端通知中心展示升级级别；同步迁移
  `000047`、OpenAPI/typegen、配置模板和部署清单。
- See [change record](docs/changes/2026-08-14-m111-sla-escalation.md)。

### Fixed - CI Backend lint：aiexplain 测试桩无效赋值（SA4005）

- `golangci-lint` staticcheck 曾以 `SA4005` 报
  `backend/internal/httpserver/aiexplain_handler_test.go` 的
  `aiexplainRepoStub.Save`：值接收者下 `saved` 字段赋值对外不可见且从未被读取。
- 删除该未读字段与无效赋值行，接口实现与错误返回不变；backend
  `gofmt` / `go vet` / `golangci-lint ./...` / `go test ./...` 全绿。
- See [change record](docs/changes/2026-08-14-ci-backend-lint-sa4005.md)。

### Added - UI 截图基线机制（Track A · 登录页先行）

- 新增 `scripts/capture-ui-baselines.mjs`：CDP 驱动 headless Chrome 捕获登录页
  Desktop 1440×900 / Mobile 375×812 的确定性像素基线，写 `docs/ui-baselines/`
  （manifest 含 commit/viewport/sha256/动态区掩码/差异阈值）。
- `--verify` 模式重截图后按像素对比（sha256 相同即 IDENTICAL，否则 sips 转 BMP
  逐像素 diff，跳过掩码区），≤0.2% 阈值即 PASS；为可重复对比注入固定种子
  `Math.random` + `prefers-reduced-motion` 仿真，解决粒子画布随机抖动。
- 登录页（第 15 轮成果）基线两档通过：desktop diff 0.000%、mobile sha256 一致。
- See [change record](docs/changes/2026-08-13-ui-baseline-screenshots.md)。



### Added - Track A 无障碍修复·第三批（axe 32 视图收口 + 375px 全量响应式）

- 修复 6 类 axe 违规：`--status-danger` 全局变暗 `#dc2626 → #b91c1c`（5.01:1），
  `.user-avatar` 底加深（4.09→5.46:1），用户页 4 处灰字迁 token（`#89959a` 等
  → `var(--gray-500/600)`），3 个 select 增 `aria-label="选择集群"`。
- 修复 2 处前端 400 缺陷：`AppCatalogView` / `AIInvestigatorView` 的
  `listAppCatalogPlans` / `listCorrelationCases` 必须携带 `cluster_id`，
  无集群时置空（此前无参触发 400 Bad Request）。
- `scripts/audit-a11y-axe.mjs`：`classify()` 将 `status of 404` 网络错误归为
  probes（后端 feature-gate 路由在本地环境未注册属预期）。
- axe 32 视图 × 双视口全量 **PASS**（0 critical/serious / 0 app errors）；
  62 条截图基线 `--verify` 59 IDENTICAL + 3 PASS；
  375px 响应式 31/31 视图无溢出/无可达元素裁切。
- See [change record](docs/changes/2026-08-14-a11y-fixes-batch3.md)。

### Added - UI 基线/axe/响应式扩展·第三批（Track A 全量铺开）

- 截图基线 + axe 审计覆盖扩展至全部前台视图（20 个新视图：全局搜索 `/search`、
  监控大盘 `/monitoring`、SLO 仪表盘 `/aiops/slo`、关联案例 `/aiops/correlation`、
  AI 调查 `/aiops/investigator`、智能巡检 `/inspection`、质量仪表盘 `/aiops/quality`、
  命名空间治理态势 `/namespace-posture`、集群治理态势 `/posture`、Webhook 投递
  `/notifications`、用户管理 `/users`、Helm 应用目录 `/app-catalog`、GitOps 应用
  `/gitops`、事件流与告警 `/event-stream`、服务网格 `/service-mesh`、工作负载保护
  `/workload-protection`、节点维护 `/node-maintenance`、恢复演练 `/restore-rehearsal`、
  Promotion 向导 `/promotions`、自动化控制台 `/aiops/automation`）。
- 基线扩至 **62 条**（31 视图 × 2 视口），`--verify` 全绿（login desktop diff
  0.000%，users maxΔ=118px 在阈值内，其余 IDENTICAL）；axe 视图扩至 32 个
  （含 audit-logs，audit-logs 仅 axe 覆盖、排除像素基线——实时追加内容）。
- See [change record](docs/changes/2026-08-14-ui-baseline-batch3.md)。

### Added - UI 基线/axe/响应式扩展·第二批（Track A）

- 截图基线 + axe 审计覆盖扩展 6 个高价值视图：事件中心 `/events`、资源拓扑
  `/topology`、事故工作空间 `/incidents`、优化中心 `/optimization`、告警规则
  `/alerts`、AIOps 概览 `/aiops/overview`（Desktop 1440×900 / Mobile 375×812）。
- 基线扩至 22 条，`--verify` 全绿（login desktop diff 0.000%，其余 IDENTICAL）；
  axe 双视口新增 12 组合格（0 critical/serious / 0 app 错误）；375px 响应式
  6 页无横向溢出、无不可达交互元素（优化中心 Tab 为可滚动容器内元素，可达）。
- See [change record](docs/changes/2026-08-14-ui-baseline-batch2.md)。

### Added - UI 无障碍审计门禁（Track A · axe 双视口）+ 对比度修复

- 新增 `scripts/audit-a11y-axe.mjs`：CDP 注入 `axe-core` 跑 WCAG 2.x A/AA，覆盖
  5 视图 × Desktop/Mobile 双视口；console 错误分类为 app / axe-induced / auth-probe，
  门禁为 critical/serious + app 错误为 0（axe 诱导的 Vue patch 竞态单独归因不阻断）。
- 修复 Workloads 资源类型 Tab 对比度：`#64727a`（4.15 低于 AA）→ `var(--gray-600)`
  `#4c5c61`（5.91 AA ✓），并消除一处硬编码色。
- 全量 10 组合格：0 critical/serious、0 app 错误；workloads 对比度基线按新产物重建后
  `--verify` 全绿（login desktop diff 0.000%，其余 IDENTICAL）。
- See [change record](docs/changes/2026-08-14-a11y-axe-audit.md)。

### Changed - CSS Token 第二轮：旧调色板字面量迁移（Track A · 主题收敛）

- `base.css` 迁移 ~230 处 orphan 字面量 → `var()` 引用（45 处 `#5a6672`→`var(--text-muted)`、
  44 处 `#dfe5e8`→`var(--border-soft)` 等，语义 1:1 或近邻收敛），恢复 console 主题
  级联；`console-theme.css` 迁移 5 处 + 收敛 M93-C 重复 `--text-muted`（`#66777d`→`#5a6672`，
  wizard-steps 对比度 4.22→5.31:1）。
- 登录基线确定性修复：`capture-ui-baselines.mjs` 新文档脚本确定性 stub
  `/api/v1/health/live`，登录掩码扩至整个 `.login-signal-strip`，消除状态条
  "检测中/正常" 竞态导致的间歇失败。
- 42 基线产物重建；`--verify` 62 条全绿，axe 32 视图 0/0，`pnpm ui:gate` 4/4 PASS。
- See [change record](docs/changes/2026-08-14-css-token-round2-migration.md)。

### Added - CI 门禁集成：`pnpm ui:gate` 一键全量校验（Track A · CI 衔接）

- 新增 `scripts/ui-gate.mjs`：按序执行 CSS token 审计 (`--check`) → 截图基线
  `--verify` → axe 审计（32 视图 × 2 视口）→ bundle gate，任一步非零即中止。
- `frontend/package.json`：新增 `"ui:gate": "node ../scripts/ui-gate.mjs"` 脚本入口。
- 一键通过：`pnpm ui:gate` → `PASS: 4/4`（CSS tokens + baselines + axe + bundle）。
- See [change record](docs/changes/2026-08-14-ui-gate-ci-integration.md)。

### Changed - CSS Token 层第一轮收尾：console-theme 遗留字面量清零

- `console-theme.css` 剩余 2 处 `#b91c1c` 字面量替换为 `var(--status-danger)`，
  四层 CSS audit 全部 `replaceable=0`（pure refactor，零视觉变化）。
- See [change record](docs/changes/2026-08-14-css-token-cleanups.md)。

### Changed - CSS Token 层收口（Track A · 主题收敛 · 第一批 112 处安全迁移）

- 新增 `scripts/audit-css-tokens.mjs`：解析四层级联（base→console→motion→premium）
  的 token 有效值，扫描硬编码颜色字面量并分类 MATCHED/ORPHAN，提供
  `--apply`（精确值安全替换）/`--check`（CI 门禁）/默认审计报告三模式。
- 首批迁移 112 处精确值匹配字面量为 `var()`：base.css 85 处（`#ffffff`→`var(--gray-0)`×80 等）、
  console-theme.css 27 处（`#2dd4bf`→`var(--blue-400)`×13 等）。
- 像素基线回归 10/10 全绿（login desktop diff 0.000%、其余 sha256 一致），证明零视觉变化；
  `--check` 后 MATCHED=0。遗留不一致旧调色板值（`#5a6672`×45 等）量化留待第二轮。
- See [change record](docs/changes/2026-08-14-css-token-layer.md)。

### Added - UI 响应式审计（Track A · ≤720px 首批）

- 对登录页 + `/`、`/clusters`、`/workloads`、`/diagnoses` 在 375×812 移动视口做
  CDP 实测：页级无横向溢出、无可点击元素出屏（审计证据见
  `docs/changes/2026-08-13-ui-responsive-audit-mobile.md`），无代码改动。

### Added - UI 截图基线扩展：控制台关键页面（鉴权捕获）

- `scripts/capture-ui-baselines.mjs` 新增幂等 `login()`（原生 value setter + requestSubmit，
  默认 admin/admin123，`AIOPS_UI_USERNAME/PASSWORD` 可覆盖），把截图基线机制扩展到
  控制台页面。
- 新收录 4 个鉴权页面 × Desktop 1440×900 / Mobile 375×812：`/`（集群态势）、
  `/clusters`（多集群管理）、`/workloads`（资源工作台）、`/diagnoses`（故障分析）。
- 基线共 10 条，`--verify` 全绿：login desktop diff 0.000%，其余 9 条 sha256 一致
  （控制台页面当前为无集群空态，数据稳定故像素确定）。
- See [change record](docs/changes/2026-08-13-ui-baseline-console-pages.md)。

### Changed - 登录页视觉第 15 轮：短视口纵向适配

- 短屏手机（`max-width:720px and max-height:780px`）：隐藏顶部 brand/copy 文案层，
  面板上移并紧凑化卡片内部间距（净高 542→472px），修复 320×568/360×640/375×667
  下表单输入框与按钮被裁切（最高出屏 185px）的问题。
- 矮屏横向（`min-width:721px and max-height:660px`）：`.login-page` 改 `overflow-y:auto`、
  面板改 `place-items:safe center`，解除 `.login-page{overflow:hidden}` 对卡片的纵向裁切，
  溢出时顶部对齐可滚动；`max-height:560px` 时隐藏已出屏的 `.login-visual`/`.login-signal-strip`。
- 纯样式层，桌面/常规移动端（375×812/414×896/1440×900/1920×1080）几何与
  第 13–14 轮完全一致，无回归。
- See [change record](docs/changes/2026-08-13-login-short-height.md)。

### Changed - 登录页视觉第 14 轮：移动端登录卡片出屏修复

- 修复移动端断点级联陷阱：`@media (max-width:720px)` 内 `.login-form-panel` 选择器提升
  为 `.login-page .login-form-panel`（特异性对齐桌面规则），`max-width:none` 收紧为
  `max-width:100%`；375/414px 窄屏下面板贴齐视口、卡片两侧保留 14px 安全边距，
  解决第 13 轮桌面居中公式在移动端把卡片挤出屏幕（左缘出屏 16–42px、右侧空档）的问题。
- 实测 375/414/768/1024/1440/1920 全断点通过：无横向滚动、无元素重叠、桌面端
  13 轮基线几何不变（card 1440: `861,161 452x578`、1920: `1323,251 434x578`）。
- See [change record](docs/changes/2026-08-13-login-mobile-flush.md)。

### Added - 后续开发路线规划：M110 收口 + M111–M115 执行序

- 新增 `docs/development-roadmap-post-m110.md` 作为 M110 之后的执行入口：Track 0 M110 收口
  （推送本地提交、授权 push `v0.3.0-rc.6`、发布后全新环境/升级回滚/备份恢复演练、封口）；
  Track A 前端优化轨收口（CSS token 层、关键页面截图基线、≤720px 响应式审计、交互统一）；
  Track B M111 事故响应深化（runbook 关联、MTTA/MTTR KPI、事故模板与严重级矩阵、SLA 升级链、
  复盘导出）；Track C M112 AI 协调查询与解释深化（会话式调查、AI 事故摘要、解释覆盖率大盘，
  严守引用纪律）；Track D M113 优化中心闭环与巡检深化（finding→runbook 预览导航）；
  Track E M114 可观测性深化（SLO burn 扩展、指标历史下采样、事件流增强）；Track F M115
  工程卓越冲刺（覆盖率 65%→70%、性能基准 fail-closed、fuzz 扩展）；Track G M89/M90 授权轨
  （Deferred，随时可启动）。
- `docs/development-roadmap-post-m106.md` 头部增加被取代指针（M111+ 以本路线为准）。
- See [change record](docs/changes/2026-08-13-roadmap-post-m110.md)。

### Changed - 登录页视觉第 13 轮：登录框向页面中心靠拢（右区内部水平居中）

- 登录框 `.login-form-panel` 右插边由固定 `clamp(20px,2.5vw,48px)`（贴右缘）改为
  `calc((clamp(460px,49vw,760px) - min(540px,max(430px,44vw))) / 2)`，在其预留右区内
  水平居中，向页面中心方向内收，各宽度下均不与左侧内容重叠、不动预留区（不影响其他排版）。

### Changed - 登录页视觉第 12 轮：分层居中对齐（容器居中 · 文字左齐）

- 采纳"分层对齐"而非"全部居中"：左区整列在可用区域内水平+垂直居中（`align-items:center` + `justify-content:safe center`），块内文字保持左对齐。
- 左区各直接子块（`.login-brand` / `.login-copy` / `.login-signal-strip` / `.login-visual`）统一 `width:100%; max-width:680px`，对齐到同一条 680px 左基线；eyebrow/h1/描述/特性均为 `.login-copy` 子元素自动跟随。
- `.login-description` 维持 `max-width:540px` 限宽，落实主内容/辅助信息分层、提升长文本可读性。
- 修复 M93-B1f 重复 `.login-visual`（`min(100%,620px)` + `margin-top:auto`）覆盖主区块居中的级联陷阱，改为 `max-width:680px; margin-top:0`。
- 表单主操作按钮组：`.login-submit` 改为 `width:100%; justify-content:flex-start`（占满表单宽、内部文字左齐），`.login-submit-label` 改 `text-align:left`。

### Changed - 登录页视觉第 11 轮：底部文字统一与图形居中对齐

- 统一底部 caption 样式：特性卡片标签与 footer 小字统一为 11px / font-weight 500 / 0.4px 字间距 / 1.35–1.45 行高。
- 简化 footer 结构：去掉 copy+tags 两段式与 dot 分隔，改为单行居中文字。
- 将 footer 从 `.login-intro` 绝对定位改回 `.login-visual` 内部普通流，宽度与能力条对齐（540px）并居中。
- 拓扑图形 `.login-topology` max-width 从 620px 收紧到 540px，与能力条同宽；`.login-visual` 改为 flex column + `align-self:center` + `margin-top:auto`，使底部装饰柱水平居中并锚定到底部。

### Changed - 登录页视觉第 10 轮：去伪数据，接入真实控制台状态

- 移除第九轮 `.login-signal-strip` 的硬编码虚构指标（在线集群 12 / SLO 99.9% / 待处理告警 3）。
- 中区状态条改为真实数据：服务状态与平台版本来自公开接口 `GET /api/v1/health/live`（无鉴权），本地时间由浏览器实时时钟驱动；后端不可达时如实显示"未响应"，不编造。
- 状态指示点新增"检测中/异常"两态，颜色随真实状态联动；标题改为"控制台状态 · CONSOLE STATUS"。

### Changed - 登录页视觉第 9 轮：中区紧凑化与平衡填充

- 根因修复：`.login-intro` 原 `grid` + `.login-visual` 底部锚定造成标题与拓扑间长期"死区"；改为 `flex` 连续纵向栈（`justify-content:flex-start` + 统一 `gap`），中区空洞收敛为节奏化间距。
- 新增中区填充块 `.login-signal-strip`（aria-hidden）：分隔线 + 标签 + 3 枚图标统计胶囊，把空旷中区赋予明确意图（状态概览）。
- 间距收紧：去除 `.login-copy` 过大 `padding-top`，描述/特性行高与边距统一。矮屏保留并紧凑化该面板，移动端同策略隐藏。
- 关键踩坑：抵消 base.css `.login-intro` 的 `justify-content:space-between` 透传。纯视觉层，无表单/无障碍回归。产物 `index-B2k1Maxs.css` + `index-DSssqMZx.js`。
- See [change record](docs/changes/2026-08-13-login-ambience-enhance.md)。

### Changed - 登录页视觉第 8 轮：叙事连贯 / 中央景深 / 卡片焦点 / 减噪

- 左区新增信号脊柱（`.login-spine`）串联品牌→标题→拓扑，中央新增柔光景深（`.login-depth`）把空洞变为有意为之的纵深；登录卡新增四角括号（`.login-card-frame`）、载入一次性青色光晕收束（`.login-card-attract`）、整卡聚焦联动 `:has(.is-focused)`。
- 减噪：桌面粒子密度 11000→14000、上限 90→70、连线透明度 0.22→0.16；radar 透明度 0.34→0.30、mask 更早渐隐。矮屏/移动端同步隐藏景深与脊柱。
- 全部纯视觉层改动，未动表单逻辑与无障碍结构；沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1`，产物 `index-BQPzgRJy.css` + `index-BRIbTalY.js`。
- See [change record](docs/changes/2026-08-13-login-ambience-enhance.md)。

### Added - M110 RC-6 发布预检（本地就绪确认）

- 新增 `docs/m110-release-preflight.md`：15 项本地预检全过（后端编译/测试、前端 typecheck/lint/build、release 工具测试、Dockerfile/Helm/kustomize、迁移自包含、release workflow 质量门继承 M109 全部 CI 门禁）。
- 触发方式：用户授权后 push `v0.3.0-rc.6` tag 触发 Release workflow；发布后按清单执行全新环境安装/升级/回滚/备份恢复演练与 digest 签名核对。
- See [change record](docs/changes/2026-08-13-m110-release-preflight.md)。

### Changed - M109 Gate B 性能门禁翻转为 fail-closed

- 四处证据生产者（`pod-scale-perf-report.mjs`、`login-perf-report.mjs`、`style-audit.mjs`、`scalebench/report.go`）的 `mode` 从 `report` 翻转为 `fail-closed`；CI `GATE_B_MODE` 同步切换。超阈值现在显式视为回归，阻断 CI。
- `m96-gate-b.mjs`：`EXPECTED.css.mode` 应模式、`performanceThresholds` 说明按模式动态输出。
- `scalebench_test.go`：断言 `report.Mode` 更新为 `"fail-closed"`。
- See [change record](docs/changes/2026-08-13-m109-gate-b-fail-closed.md)。

### Added - M109 工程卓越收口：覆盖率门禁 65% + fuzz smoke 扩展 + Gate B 性能基线记录/模式开关

- 覆盖率门禁：`ci.yml` 全局门禁基线 60%→**65%**（实测 65.16%），随此前覆盖率冲刺提交一并达成。
- Fuzz 门禁：CI fuzz seed 列表追加 `./internal/incident/ ./internal/correlation/`（纳入 `FuzzEngineCorrelate` / `FuzzCanTransition` / `FuzzTransitionSequence`）。
- Gate B 性能门禁预备：新增 `docs/m96-gate-b-baselines.md` 记录两个稳定成功周期（Run 31682162681、31683950601）；`scripts/m96-gate-b.mjs` 支持 `GATE_B_MODE` 开关（report / fail-closed），CI 门禁显示为 `report`，翻转步骤见入库文档。
- 归档机械门禁：`scripts/check-change-record.sh` + `scripts/git-hooks/pre-commit` 本地强制归档铁律；`ci.yml` 新增 `change-record` job 并纳入 `result` 必填集（并行 Agent 交付，本块合入）。
- See [change record](docs/changes/2026-08-13-m109-gate-closeout.md)。

### Added - M109 httpserver 覆盖率冲刺：60.9% → 65.16%，达成 65% 门禁

- 覆盖 `auth.go` 全分支（login/refresh/logout/me/changePassword/sessions/revoke/withAuthentication/requireRoles 的错误哨兵表与成功路径，含 auth_version 失配、用户禁用、cookie 解析失败等）。
- 覆盖 `servicemesh.go`（VS/DR 列与详情成功/404/500、traffic-metrics、`writeServiceMeshError` 全哨兵表）、`topology.go`（graph/changes 全校验与成功/错误）、`grants.go`（namespace 授权列表/`myGrants` 错误分支）、`incidents.go`（transition/assign/follower/note/postmortem/summary/export 的错误映射）。
- 全局语句覆盖率 60.9%→**65.16%**（`go test -cover -p=1 -count=1 ./...`），达成 M109 路线图 65% 门禁；`go test ./... -short` 全绿、`gofmt -l` 干净。
- See [change record](docs/changes/2026-08-13-m109-coverage-handler-tests.md)。

### Added - M109 incident 关键旅程 E2E：a11y 扩展 + correlation 深链/提升事故

- a11y：axe 扫描路由新增 `/incidents`、`/aiops/correlation`（空态 + mock API），wcag2a/aa 0 critical/serious。
- E2E：新增 `correlation.spec.ts`——`?case_id=` 深链聚焦案例详情、提升事故（通知 + 已关联徽章 + 跳转 `/incidents` 双向深链回程）、409 `SOURCE_ALREADY_USED` 去重稳定提示；Desktop/Mobile 双视口，console error 严格断言（仅过滤预期 409 资源日志）。
- Playwright 全量 76/76 PASS（Desktop + Mobile）。
- See [change record](docs/changes/2026-08-13-m109-incident-e2e.md)。

### Added - M109 工程卓越起步：fuzz 扩展 + 覆盖率提升

- Fuzz：`FuzzEngineCorrelate`（correlation 引擎结构化随机输入，永不 panic + 结果一致性）、`FuzzCanTransition` / `FuzzTransitionSequence`（incident 状态机真值表 + Service 层 CAS 序列）。实测 FuzzEngineCorrelate 15s ≈189K execs、FuzzCanTransition 10s ≈1.2M execs、FuzzTransitionSequence 15s ≈1.5M execs 全绿。
- 覆盖率：全局 60.6%→**60.9%**；重点包 incident 40.0%→43.1%、correlation 64.7%→67.2%、signal 56.4%→63.9%（补 List/Summary、assignFailureCode、SLA monitor Run、signal Get/IngestBatch/NopSourceReader、drain Run 生命周期、computeReasonCode、SourceRefForCorrelation）。
- See [change record](docs/changes/2026-08-13-m109-fuzz-coverage.md)。



### Changed - M108 关联收口：集群级信号关联段 + demo-drill 41/41

- 后端：`correlation.Worker` 命名空间段后追加 all-namespace 段，Node 级信号（namespace 为空）也能被关联（`maintenance_causes_node_failure` 等集群级规则生效）；upsert 按 case_key 归并，幂等。
- 演示：demo-drill 修复 batch-assign 载荷（`incident_ids` 字符串→数字）与 2/2 信号归并断言（Node 案例第二案例提升）；全量复跑 **41/41 PASS**。
- See [change record](docs/changes/2026-08-13-m108-correlation-cluster-scope.md)。



### Added - M108 Correlation Case ↔ Incident 双向深链 (v0.3.0-m108)

- 后端：`incident` 新增 `FindBySource(source_type, source_ref)` 反查；correlation case view 富集可选 `incident{id,number,title,status}`（只读 best-effort，缺失不阻断视图）；`IncidentDeepLink` 签名扩展，correlation 证据深链精确到 `/aiops/correlation?case_id=<id>`。
- 前端：`CaseView` 类型加 `incident?`；`CorrelationCasesView` 支持 `?case_id=` 深链聚焦（自动选集群并展开详情），详情徽章行显示「已关联事故 INC-xxxx ↗」入口（提升成功后即时回显）。
- OpenAPI：`CorrelationCaseView` schema 增加 `incident`；typegen 重新生成。
- 演示：demo-drill 第 13 节新增 `correlation-incident-deeplink` 断言（案例视图回显事故 ID）+「2/2 信号归并」断言（pod.oom_killed 信号归一为第二案例并可提升）。
- See [change record](docs/changes/2026-08-13-m108-correlation-deeplink.md)。



### Added - M108 Correlation Case → Incident (v0.3.0-m108)

- 后端：关联案例成为第 6 类事故来源 `correlation`——`incidents.source_type` CHECK 迁移 `000046`；resolver 解析 `correlation:<id>`（前缀/ID 校验、案例不存在→`ErrInvalidSource`、跨集群防泄漏），severity 按案例置信度富集（confirmed→high / candidate→warning / 其余 info），title/summary 带 case_key/rule_id/信号数，资源与首次观测时间取自案例。
- 证据时间线：correlation 源证据卡「来源=关联案例」+ 深链 `/aiops/correlation`（`IncidentDeepLink`）。
- 前端：`CorrelationCasesView` 案例详情新增「提升事故」按钮（`source_type:'correlation'`，处理 `SOURCE_ALREADY_USED` 去重提示）；`IncidentsView` 来源标签加「关联案例」；typegen 重新生成。
- 演示：`demo-drill.sh` 新增第 13 节「Correlation case → incident」断言（isolated compose 加压 `CORRELATION_INTERVAL=30s` 保证出案例；案例提升 + severity 富集 + 重复提升 `SOURCE_ALREADY_USED`），并修复报告 evidence 块缺失逗号。
- See [change record](docs/changes/2026-08-13-m108-correlation-source.md)。


### Added - Change-record archive gate

- 为 AGENTS.md §1 归档铁律添加最小机械门禁：新增
  `scripts/check-change-record.sh`，当改动包含非文档代码文件时要求同一改动
  存在 `docs/changes/YYYY-MM-DD-<slug>.md` change-record，缺失时输出指向
  AGENTS.md 归档规则的可读错误并拦截。
- CI：`ci.yml` 新增 `change-record` job（push/PR/dispatch 均运行）并纳入
  `result` 汇总的必填结果；另提供可选本地钩子 `scripts/git-hooks/pre-commit`。
- See [change record](docs/changes/2026-08-13-archive-gate-change-record.md)。

## [Unreleased]

- 登录页填充空旷感：`LoginView.vue` 左侧介绍区新增纯装饰三层——中段雷达扫描
  SVG（同心环 + 旋转扫线 + 呼吸信号点，呼应"信号驱动/持续监测"）、标题下特性词条
  strip（RULES-DRIVEN / EVIDENCE-FIRST / AUDIT-CLOSED）、底部信息条
  （版权 + SIGNAL-DRIVEN / AUDIT-CLOSED 标签）。
- `console-theme.css` 新增 `/* ---- M93-B1c ---- */` 块：radar 绝对定位垂直居中
  （`width:clamp(240px,30vw,420px)`、opacity .5、衬于内容层之下）、features
  11px 字距微光词条、footer 贴底两端分布；`login-visual` 加 `padding-bottom:40px`
  隔离能力列表，`login-security-status` 加分隔线增强卡片结构感。
- 响应式：矮屏断点（≤760px 高）隐藏 radar/footer、features 收紧；移动端断点
  （≤720px 宽）radar/footer/features 全部隐藏，移动端布局零改动；新元素均
  aria-hidden 或 role=list，不影响无障碍。
- 信号流增强（第二轮）：雷达加双扩散脉冲圆（radar-ping 差相动画）；三个特性
  词条 hover 联动 `activeCapability`，与右侧拓扑图节点点亮互操作，词条自身
  `is-active` 高亮；footer 加极淡横向刻度线。
- 部署：Docker Hub 不可达导致镜像重建失败，临时以 `docker cp` 将新 dist 覆盖
  进 18080 前端容器（非持久，容器重建后回退，待网络恢复后重建镜像固化）。
- 入场编排与微交互（第三轮）：`login-rise` 阶梯补全，radar/features/footer
  并入入场波次（radar 0.05s 淡入、features 0.15s 上移淡入、footer 0.36s 纯淡入）；
  输入框聚焦新增底部信号光带（`.login-field::after` scaleX 展开 + 3s 无缝流动）；
  `label:has()` 标签随聚焦联动变色（渐进增强）；提交按钮 hover 叠加 2.2s
  呼吸辉光（保留 sheen 扫光）；安全状态图标 3.4s 微呼吸。全部新动画自动受
  全局 `prefers-reduced-motion` 复位覆盖。
- 触觉细节（第四轮）：登录卡片 hover 上浮 4px 并叠加青色泛光与边框微染，顶部
  轨道条同步提亮辉光；提交按钮按下时 `translateY(1px) + scale(0.992)` 触感
  下压并收敛外发光（暂停 hover 呼吸辉光），移动端同步；输入框聚焦时前置图标
  轻微抬起 + 青色 drop-shadow；标题 `em` 强调字 7s 慢速渐变流动（桌面生效，
  移动端保持平面青色）。
- 布局修整（第五轮）：修复 radar 装饰与标题/能力列表重叠——radar 由垂直居中
  大圆改为左下角小圆（`bottom:clamp(74px,15vh,132px)`、`width:clamp(170px,19vw,280px)`、
  opacity .5→.38，z-index 1 衬于内容层下，copy/visual 提至 z-index 2）；
  右侧登录面板不再贴死右缘，`inset` 右留 `clamp(20px,2.5vw,48px)` 呼吸边距，
  宽度收敛至 `min(540px,max(430px,44vw))`，左侧介绍区 padding-right 同步
  调至 `clamp(460px,49vw,760px)` 保持左右平衡；radar 追加 radial mask 径向
  渐隐（圆环外围柔和融入背景），环/十字透明度微降；移动端断点面板重置
  `inset:0; width:100%` 防边距残留。
- 对角构图（第六轮）：radar 由左下角移至右上角
  （`top:clamp(84px,11vh,120px); right:clamp(8px,2vw,28px)`，尺寸收敛
  `clamp(150px,15vw,230px)`、opacity .38→.34、mask 渐隐中心改 62% 40%），
  与左下 footer 形成对角线平衡，彻底消除左下角"radar+星网+版权"视觉拥挤；
  星网 canvas 追加竖向渐隐 mask（顶部 62% 全显→底部 16% 全隐），粒子视觉
  重心上移、左下角干净，粒子行为逻辑不变。
- See [change record](docs/changes/2026-08-13-login-ambience-enhance.md)。

## [Unreleased]

### Added - M107 Incident Evidence Timeline (v0.3.0-m107)

- 后端：incident 详情新增「证据时间线」只读区块——`GET /api/v1/incidents/{incident_id}/evidence`
  把 incident 背后的五源（diagnosis/finding/alert/inspection/signal）解析为结构化证据块
  （来源/严重度/标题/摘要/资源/字段/深链），resolver 失败时安全回退到 incident 快照，
  永不断详情；加入审计 `incident.evidence.get` 与权限矩阵。
- OpenAPI：`/incidents/{incident_id}/evidence` + `IncidentEvidenceItem`/`IncidentEvidenceField`
  schema；typegen 与 permission matrix 重新生成（CI sync gate 生效）。
- 前端：`IncidentsView` 详情抽屉新增「证据时间线」证据卡（含前端深链「查看原始证据」）；
  `getIncidentEvidence` API 封装。
- 演示：`demo-drill.sh` incident-journey 新增 `incident-evidence` 断言（diagnosis 源 +
  deep_link=/diagnoses）。

## [Unreleased]

### Added - M107 Incident SLA Notifications (v0.3.0-m107)

- 后端：incident 的 SLA `overdue` 此前仅在 UI 展示；新增 `SLAMonitor` 后台任务，在通知启用时
  周期扫描 open/confirmed 事故，把「临近 15 分钟」与「已逾期」事件原子写入现有
  `notification_deliveries` outbox（幂等部分唯一索引 `(incident_id, event_type)`），随原有
  webhook 投递 + 重试闭环送出去；payload 含事故号/标题/严重度/SLA 截止/深链。
- 数据库：迁移 `000045` 让 outbox 支持可空 `incident_id`（不依赖 diagnosis 存在，
  finding/alert/inspection/signal 源事故同样可提醒），并加对应索引；up/down 实测往返可用。
- 契约：`notification-deliveries` API 新增 `incident_id` 过滤与
  `incident.sla_approaching` / `incident.sla_breached` 事件类型，OpenAPI/typegen/权限矩阵同步。
- 前端：Webhook 投递视图支持事故 ID 过滤 + 两类 SLA 事件；incident 列表/详情 SLA 徽标新增
  `approaching` 色调（临近高亮），逾期/临近更醒目。
- 配置：`INCIDENT_SLA_MONITOR_ENABLED` / `INCIDENT_SLA_POLL_INTERVAL` /
  `INCIDENT_SLA_APPROACHING_WINDOW` / `INCIDENT_SLA_BATCH_SIZE`。
- See [change record](docs/changes/2026-08-13-m107-incident-sla-notifications.md)。
- See [change record](docs/changes/2026-08-13-m107-incident-evidence-timeline.md)。

## [Unreleased]

### Added - M107 Incident Batch Assignment (v0.3.0-m107)

- 后端：incident 工作空间新增批量指派——`POST /api/v1/incidents/batch-assign` 一次把多个
  事故移交给同一负责人；逐条 CAS 校验，部分失败不中断其余移交，聚合 `assigned/total/failed`
  结果返回；上限 50 条/请求。
- 契约：`IncidentBatchAssignRequest` / `IncidentBatchAssignResult` / `IncidentAssignFailure`
  schema 与 OpenAPI/typegen/权限矩阵同步；审计 `incident.assignment.batch`。
- 前端：`IncidentsView` 表格新增多选列与批量工具栏（负责人/说明/提交/取消），成功与部分
  失败结果均展示。
- 演示：`demo-drill.sh` incident-journey 新增 `incident-batch-assign` 断言（assigned ≥ 1）。
- See [change record](docs/changes/2026-08-13-m107-incident-batch-assign.md)。

## [Unreleased]

### Added - M107 Postmortem Narrative View (v0.3.0-m107)

- 前端：incident 详情抽屉时间线新增「全部/备注/系统」过滤 tabs，证据时间线在来源数 > 1 时
  新增按来源过滤 tabs（诊断记录/人工上报/告警实例/巡检结果/信号实例）。
- 复盘视图（已解决事故）：只读叙事区块——复盘结论 + 结果指标卡（SLA 达标/逾期、解决耗时、
  系统事件数、人工备注数、证据来源数）；编辑复盘仍限 ops admin / system admin。
- E2E：`incidents.spec.ts` 新增双来源证据 mock 与过滤/复盘视图断言，Desktop/Mobile
  双项目 4/4 通过。
- See [change record](docs/changes/2026-08-13-m107-postmortem-narrative.md)。

### Changed - Login Panel Enhance (Frontend UX Track)

- 登录页大气化：`console-theme.css` 右侧 `.login-form-panel` 加宽至
  `min(560px, max(440px, 46vw))`，`.login-page` 左侧介绍区 `padding-right` 同步
  `clamp(440px, 46vw, 720px)`；`.login-card` 显式放大至 `width:min(100%,470px)`、
  内边距 `46px 48px 42px`、圆角 16px、背景加深 `rgba(18,31,29,.86)`。
- 面板「舞台感」：`::before` 新增双径向 + 线性渐变辉光背板与青色左边框
  `rgba(94,234,212,.14)`，消除"角落小卡片"观感。
- 内部元素精修（`/* ---- M93-B1b ---- */` 块）：rail 4px、icon 48px、标题 28px、
  输入框 52px 高/50px input/15px 字号、按钮 52px/650 字重、间距与 label 全面放大；
  矮屏（≤760px 高）与移动端（≤720px 宽）断点内边距同步适配。
- See [change record](docs/changes/2026-08-13-login-panel-enhance.md)。

## [Unreleased]

### Added - Post-M106 Development Roadmap

- 新增 `docs/development-roadmap-post-m106.md`：规划前端界面优化并行轨（主题收敛/截图基线/
  响应式审计/交互统一/性能预算 + 衔接契约）、主线 M107 事故协作闭环（复盘/SLA/交接/五源证据
  时间线）、M108 关联归一（correlation → incident 第 6 来源 + 风暴去重）、M109 工程卓越
  （性能 fail-closed/旅程 E2E/覆盖率 65%）、M110 RC-6 刷新，以及 M89/M90 授权轨 → GA Gate D。
- See [change record](docs/changes/2026-08-13-roadmap-post-m106.md)。

## [Unreleased]

### Changed - M106 Local UX Polish (v0.3.0-m106)

- 口令：本地开发默认凭据统一 `admin123`——`BOOTSTRAP_ADMIN_PASSWORD`、Postgres
  `POSTGRES_PASSWORD`/`DATABASE_URL` 默认值全面切换（`backend/internal/config/config.go`、
  `compose.yaml`、`.env.example`）；production guard 同步拒绝 `admin123`；文档
  （README / development / security-statement / PROJECT_STATUS）与所有演示/演练脚本、
  前端登录测试断言同步；敏感扫描 allowlist 增加 `admin123`。
- 登录页：`console-theme.css` 登录页改为全屏单一场景——`.login-page` 由两列 grid 改
  `display:block`，`.login-form-panel` 选择器提升为 `.login-page .login-form-panel`
  （特异性压制 `premium-ui.css` 的 `section[class*="panel"]{position:relative}` 误匹配），
  布局改 `position:absolute; inset:0 0 0 auto; width:min(440px,100vw)`，遮罩层改极淡
  渐变，消除右侧大片黑带。
- 侧栏：折叠状态基线复核（72px 折叠宽 / 44px nav-item），label 隐藏无溢出，折叠开关
  展开/收起均可用。
- 栈：本地栈已启用最新镜像——后端 `k8s-aiops-backend:latest`（`v0.3.0-m106`，
  alpine:3.22 离线重建）与前端 `k8s-aiops-frontend:latest`（新 dist），全新 volume
  bootstrap，`admin/admin123` 登录验证通过。
- See [M106 change record](docs/changes/2026-08-13-m106-local-shell-polish.md)。

### Added - M105 Signal-to-Incident Triage (v0.3.0-m105)

- 后端：接通 M39 诊断信号摄取管线——新增 `DiagnosisDrain` 后台 worker（`updated_at`
  严格游标 + fingerprint upsert 幂等），`signal_occurrences` 现在能收到诊断产生的
  `diag.*.v1` 信号；`signal` 仓库/服务新增 `Get` 与导出的 `ErrSignalNotFound`；
  `diagnosis.ListFilter` 新增 `UpdatedAfter` 游标；config 新增
  `SIGNAL_DIAGNOSIS_INGESTION`（默认开）与 `SIGNAL_DIAGNOSIS_DRAIN_INTERVAL`（默认 5s）。
- 后端：`incident` 支持第 5 个来源 `signal`——`SourceTypeSignal` + `SourceRefForSignal`；
  `incidentResolver.resolveSignal` 集群防泄漏 + 严重级 1:1 富集；迁移 `000044` 放开
  `incidents.source_type` CHECK 增加 `'signal'`；OpenAPI enum/注释同步。
- 前端：`AIOpsOverviewView` 信号列表新增「创建事故工作区」按钮（ops/system_admin 且非
  resolved，处理 `SOURCE_ALREADY_USED`）；`IncidentsView` 来源类型增加「信号实例」；
  `IncidentSourceType` / typegen 同步含 `signal`。
- 演示演练：`demo-drill.sh` 新增「Signal → incident」4 条断言（诊断信号归一化→提升事故→
  严重级富集 critical→重复提升去重，drain 2s 轮询），端到端 32/32 PASS。
- See [M105 change record](docs/changes/2026-08-13-m105-signal-incident-triage.md)。

### Added - M104 Inspection-to-Incident Triage (v0.3.0-m104)

- 后端：`incident` 支持 `inspection` 来源——`SourceTypeInspection` + `SourceRefForInspection`；
  `incidentResolver` 新增巡检来源：查结果 → 集群防泄漏校验（结果 `ClusterID` 必须等于调用方）
  → `normalizeIncidentSeverity` 严重级富集（critical/warning/info）；迁移 `000043` 放开
  `incidents.source_type` CHECK 增加 `'inspection'`；OpenAPI `source_type` enum 增加 `inspection`。
- 前端：`InspectionView` 巡检结果新增「创建事故工作区」按钮（ops/system_admin 且未 resolved，
  处理 `SOURCE_ALREADY_USED`，新增 `.ok-message` 成功样式）；`IncidentsView` 来源类型增加
  「巡检结果」、自动填充禁用与详情友好来源标签；`IncidentSourceType` / typegen 同步含 `inspection`。
- 演示演练：`demo-drill.sh` 新增「Inspection → incident」6 条断言（运行 n‍ode_not_ready →
  轮询任务 → 取结果 → 提升事故 → 严重级富集 critical → 重复提升去重），端到端 28/28 PASS。
- See [M104 change record](docs/changes/2026-08-13-m104-inspection-incident-triage.md)。

### Added - M103 Alert-to-Incident Triage (v0.3.0-m103)

- 后端：`incident` 支持 `alert` 来源——`SourceTypeAlert` + `SourceRefForAlert`，
  `incidentResolver`（接口化）按来源分发：诊断沿用 diagnosis 富集，告警查实例并
  复用关联诊断富集严重级/资源/摘要/首触时间；`SourceResolver.Resolve` 增加
  `clusterID`；迁移 `000042` 放开 `incidents.source_type` CHECK 增加 `'alert'`；
  OpenAPI `IncidentCreateRequest.source_type` enum 增加 `alert`。
- 前端：`AlertsView` 触发中实例新增「创建事故工作区」按钮（处理 `SOURCE_ALREADY_USED`
  与仅 firing 可提升）；`IncidentsView` 创建表单增加「告警实例」来源、自动填充禁用与
  详情友好来源标签；`IncidentSourceType` / typegen 同步含 `alert`。
- 演示演练：`demo-drill.sh` 新增「Alert → incident」5 条断言（规则创建→触发→提升事故→
  严重级富集 high→重复提升去重），端到端 22/22 PASS。
- See [M103 change record](docs/changes/2026-08-13-m103-alert-incident-triage.md)。

### Fixed - Main CI Restoration (pnpm / license-scan / ineffassign)

- `dependency-scan` (Dependency & supply chain) 作业补全 `pnpm/action-setup` 与
  `actions/setup-node`，修复前端 `pnpm audit --prod` 因 `pnpm not found` 必然失败
  的回归（该作业自 M100-D 引入以来从未在完整 runtime 下通过，此前被 gofmt/govulncheck
  先行失败与 docs-only 跳过路径掩盖）。govulncheck 与 pnpm audit 现均按真实环境执行。
- `scripts/license-scan.sh` 将模块许可证发现收敛到模块根（`-maxdepth 1 -type f`），
  不再把 `licenses/` 子目录或第三方 LICENSE 误当模块许可证，修复 `bytedance/sonic`
  （Apache-2.0）在 CI 上被误判 UNKNOWN 的环境依赖失败。
- `backend/cmd/oidc-provider/main_test.go` 移除 PKCE 缺失断言中 ineffectual 的
  `badURL` 首次赋值，修复 golangci-lint ineffassign。
- `dependency-scan` 的 SBOM diff self-test 改为同时捕获 stdout 与 stderr（`2>&1`），
  使 `sbom-diff.mjs` 发往 stderr 的 `fail-closed` 门线可被正确断言（该断言自 M100-D
  起因仅捕获 stdout 而永远无法命中）。
- See [dependency-scan pnpm change record](docs/changes/2026-08-13-ci-dependency-scan-pnpm.md)。

### Added - M94 Replay Demo Drill & Offline Bundle Refresh (v0.3.0-rc.5-replay)

- 重建含回放代码的镜像 `k8s-aiops-backend:v0.3.0-rc.5-replay`（宿主机交叉编译 + alpine 封包）与 `k8s-aiops-frontend:v0.3.0-rc.5-replay`（nginx 复用 + 覆盖新 dist），解决既有 latest 镜像不含 replay 代码无法端到端验证的问题。
- `scripts/demo-drill.sh` 新增两处回放断言：受控动作前验证 Node 诊断回放（schema/steps/stages/time-sorted）、受控动作后验证 Pod 诊断回放包含 activity+remediation（created+executed），演示闭环从 15/15 提升至 17/17。
- `scripts/offline-install-drill.sh` 用新镜像刷新离线包（`aiops-platform-offline-v0.3.0-rc.5-replay`），离线安装全链路 10/10 PASS，确保最新功能代码包含在可复用离线安装包内（允许安装）。
- 双环境演练以 k8s-aiops-backend:v0.3.0-rc.4 为基线、k8s-aiops-backend:v0.3.0-rc.5-replay 为升级目标复跑：双全新环境安装/关键旅程/持久化，跨 digest 升级（version dev -> v0.3.0-rc.5-replay，marker 保持），回滚（version 复原），第三环境逻辑备份恢复，全部 PASS（报告 .artifacts/dual-env-compose-drill/report-20260812-225042-e68b90.json），当前产物补齐「安装、升级、回滚、备份恢复、关键旅程」全链路。
- 回放模式浏览器通道 e2e 闭环：`diagnosis-timeline.spec.ts` 新增 2 项双视口测试（replay-panel 正常链路 seek/筛选/播放 + replay API 不可用降级），全量 e2e 68/68。
- 契约同步：M94 OpenAPI 增量重新生成 `frontend/src/api/openapi.d.ts`（typegen sync 门禁无 diff）。
- 离线包自包含化：迁移文件入 bundle（`migrations/000001_init_schema.sql`），compose initdb 由宿主绝对路径改为 bundle 相对挂载，离线安装不再依赖仓库源码；SHA256SUMS 6→7 文件，offline-install-drill 10/10。
- 离线演练闭环：`docker load` 与安装/清理改用已发布离线包（`$BUNDLE_STABLE`），发布产物被真实安装验证（10/10，`report-20260812-230945-4bb47d.json`）。
- See [offline bundle self-contained change record](docs/changes/2026-08-12-m102-offline-bundle-self-contained.md)、[openapi typegen sync change record](docs/changes/2026-08-12-m94-openapi-typegen-sync.md)、[replay e2e browser change record](docs/changes/2026-08-12-m94-replay-e2e-browser.md) 与 [rc.5-replay dual-env evidence](docs/changes/2026-08-12-m102-rc5-replay-dual-env-evidence.md).

## [Unreleased]

### Added - M94 Diagnosis Replay Mode

- 新增诊断详情只读回放模式：按事件时间重放 M81 insight 链路（诊断创建 → 证据采集 → 状态与协作 → AI 引用解释 → 受控动作），严格使用已存储产物，绝不重新生成或伪造历史 AI 结论。
- 后端：`diagnosis` 领域新增纯函数 `BuildReplay`（schema `aiops.diagnosis-replay/v1`，五阶段、稳定时间排序、无步骤不伪造）+ `GET /api/v1/diagnoses/:diagnosis_id/replay`（AuditAction `diagnosis.replay.read`）；AI 解释/remediation 可选服务失败时自动降级为纯存储步骤，接口保持 200；OpenAPI 新增 3 个 schema + replay path，权限矩阵同步重新生成（280 路由 / 158 已审计）。
- 前端：`useDiagnosisReplay` 状态机（播放/暂停/上一步/下一步/seek/按阶段筛选/reset）+ 诊断详情 `replay-panel`（控制条、进度条、阶段 chips、步骤卡片，置于证据时间线之前）。
- 测试：后端 diagnosis 3 单测 + httpserver 4 单测、前端 composable 4 单测；前后端全量门禁（build/vet/test/typecheck/lint）全绿。
- See [M94 replay mode change record](docs/changes/2026-08-12-m94-diagnosis-replay.md).

## [Unreleased]
## [Unreleased]

### Added - M102 Offline Install Bundle Drill (Local Track)

- 新增离线安装包全链路演练 `scripts/offline-install-drill.sh`：`docker save` 三个镜像（backend/frontend/pgvector）到 bundle `images/*.tar`（布局对齐 M97 release 离线包），写入 `pull_policy: never` 的 `compose.offline.yaml` + `config/env.example` + runbook 副本 + `OFFLINE-SHA256SUMS`；`shasum -a 256 -c` 校验完整性（6 文件 OK，模拟空气隔离传输）；逐 tar `docker load` 且 digest 不变；以 `pull_policy: never` 在全新隔离环境（22432/22080/22081）安装 → backend ready → frontend 200 + admin 登录 `/me` → `system_admin` → 持久化标记跨 recreate 保持（count=1）→ `down -v` 清理；校验后同步发布可复用离线包 `.artifacts/offline-install-drill/bundle/aiops-platform-offline-<version>/`（images/deploy/config/docs/OFFLINE-SHA256SUMS）。10/10 PASS（报告 `report-20260812-222514-a0fbf8.json`、`report-20260812-222537-4e6c8d.json`、`report-20260812-222730-5165f1.json`，连续多轮一致）。
- 意义：补齐「只有离线包时能完成分发→加载→安装」的本地证据（此前只证明镜像已在本地时可安装）；`pull_policy: never` 使镜像缺失即失败，反证离线充分性。
- See [M102 offline install drill change record](docs/changes/2026-08-12-m102-offline-install-drill.md).

## [Unreleased]

### Added - M102 Final Delivery Documents (Test Matrix / Limitations / Compatibility / Runbook / Security Statement)

- 新增 M102 最终交付文档五件套：
  - `docs/testing/test-matrix.md`：分层测试矩阵（Go 单元/集成 74 包 / 205 测试文件、覆盖率 60.03%、OpenAPI↔路由双向契约、vitest 25、Playwright 双视口 26 用例 + 42/42 回归基线、axe、静态安全扫描、kind e2e、release 供应链）与本地确定性演练汇总（WAL/PITR 8、双环境 10/14/16、闭环演示 15、OIDC 14），含关键用户旅程 API+浏览器双通道证据与「未覆盖/待组织环境项」。
  - `docs/testing/known-limitations.md`：身份/数据韧性授权轨（M89/M90）、真实 kind/Helm 生命周期、演练工具边界（demo-kube-mock / oidc-provider）、本地环境约束、功能/契约限制与文档证据缺口。
  - `docs/testing/compatibility.md`：工具链（Go 1.26 / Node 22 / client-go 0.36 / PG17+pgvector / Vue 3）、部署形态（compose / kustomize / Helm / kind / 离线包 / 双架构镜像）、环境变量配置面、端口约定、数据恢复兼容与明确不兼容项。
  - `docs/operations/runbook.md`：快速开始、健康就绪端点、升级回滚、备份恢复（pg_dump + WAL/PITR）、关键旅程验收、日志/审计/供应链门禁、故障排查与演练清理不变量。
  - `docs/security/security-statement.md`：身份与访问（2D 授权、404 防泄漏、OIDC/MFA fail-closed）、数据保护（凭据加密、脱敏、签名审计）、受控运维边界、供应链门禁、前端安全与 GA 前已知例外。
- 同步更新 `docs/README.md` 文档索引（testing/security/operations 行）。
- 结论：M89/M90 与真实组织演练未关闭前版本保持 RC，不宣称 GA。
- See [M102 final documents change record](docs/changes/2026-08-12-m102-final-docs.md).

## [Unreleased]

### Added - M102 Reproducible Demo Drill (Local Track)

- 新增离线 mock Kubernetes API 服务 `backend/cmd/demo-kube-mock`（HTTPS + 启动时自签证书）：覆盖演示旅程所需最小 API 面（`/version` 探活、nodes、namespaces、pods、events、deployments、replicasets、metrics），确定性 fixture（Node `demo-node` Ready=False + 压力条件、Pod `demo-pod` 容器 OOMKilled + 警告事件、Deployment `demo-app`）；PATCH 按 strategic-merge 语义合并并支持 `dryRun=All`，`/mock/mutations` 记录已落地的变更供验证。演练/开发工具，绝不作生产 Kubernetes API。
- 新增 bash 版可复现闭环演示 `scripts/demo-drill.sh`：完全隔离的离线 compose 环境（pg 21432 / backend 21080 / frontend 21081，与开发栈及双环境演练零共享）驱动平台真实 API 完成「登录 → 态势 → 根因 → 证据 → 受控动作 → 验证 → 事故复盘」15 项断言，15/15 PASS（报告 `.artifacts/demo-drill/report-20260812-221752-b358d9.json`）。
- 全链路证据：Node 诊断 `node.not_ready.v1`（critical）与 Pod 诊断 `pod.oom_killed.v1`（critical，含 `container_termination` 证据）；诊断确认 → `deployment.rollout_restart` preview（confirmation token）→ 带 `Idempotency-Key` 执行 `succeeded` → mock 记录真实 PATCH（`k8s-aiops.local/restarted-at` + `remediation-id`）；事故工作区 create → confirmed → note → resolved → postmortem → CSV export。
- mock 镜像用宿主机交叉编译 + `FROM scratch` 封包，全部本地镜像离线复现；`go test ./...` 通过、`bash -n` 语法通过、结束后无残留容器/卷/网络。
- See [M102 demo drill change record](docs/changes/2026-08-12-m102-demo-drill.md).


### Added - M101 WAL/PITR Local Data-Track Drill

- 新增本地 WAL 归档 + Point-In-Time-Recovery + 流式备库 + 故障注入确定性演练 `scripts/wal-pitr-drill.sh`（8 场景）：无损 PITR（150/150 行）、时间点恢复（175 行 + late=0）、缺 WAL 快速失败（2.3s 内报错退出）、迁移前逻辑备份（恢复回迁移前 100 行）、SIGKILL 硬崩溃故障注入（已提交 20 行全部存活 + 归档链路恢复）、流式备库（追平 130/150 行、优雅停机重启追赶、`pg_promote()` 故障切换后 155 行可写）、网络分区（备库隔离保持 150 行快照、重连追平 170 行）、归档目标故障（`archive_command=false` 下主库 130 行可写、`failed_count=5`、恢复后积压排空、无损 PITR 150/150）。
- 实测（本地环境观测值，非生产声明）：RPO≤2s（archive_timeout=1s）、RTO≈1.2–2.7s；连续两轮运行结果一致，报告落 `.artifacts/wal-pitr-drill/report-*.json`。
- 修复演练脚本原始确定性缺陷：容器 heredoc 终止符缩进吞命令、`printf %p` 格式符、`rm` 匿名卷挂载点、恢复等待判定反转、PG17 archive recovery 无 target 时自建 timeline 完成恢复等。
- M101 数据轨本地第一步；网络中断/磁盘压力故障注入与多副本 HA 演练仍依赖组织授权（M90 授权轨），保持 Deferred。
- See [M101 WAL/PITR drill change record](docs/changes/2026-08-12-m101-wal-pitr-drill.md).

### Added - M102 Dual Fresh-Environment Install Drill (Local Track)

- 新增双全新环境离线安装/升级/回滚一致性演练 `scripts/dual-env-compose-drill.sh`：每次运行在完全隔离的两套环境（独立 project name、postgres 25432/26432、backend 28080/29080、frontend 28081/29081、独立 volume 与网络）安装同一不可变镜像 digest，断言 backend `health/ready`、frontend 200、admin 登录 + `/api/v1/auth/me` → `system_admin`、确定性 audit 标记持久、`down -v` 完整清理（10/10 PASS，报告 `report-20260812-213243-bf91f0.json`）。
- 设置 `APP_UPGRADE_BACKEND_IMAGE`（与基线不同 digest 的后端镜像，本地离线构建 `k8s-aiops-backend:v0.3.0-rc.5-local`）后在两套环境再执行跨 digest 升级（`/api/v1/health/ready` version `dev → v0.3.0-rc.5`，audit 标记保持）与回滚（version 还原 `dev`，标记保持），14/14 PASS（报告 `report-20260812-214043-17a17c.json`）。
- 设置 `APP_BACKUP_RESTORE=1`（配合升级镜像）后：环境 A 做 `pg_dump` 逻辑备份（244169 字节），再在**第三套全新环境**还原并断言 audit 标记 count=1 + 登录/`system_admin`，16/16 PASS（报告 `report-20260812-215101-1fcddd.json`）。
- 全部用本地已有镜像离线复现，与运行中开发 compose 栈零共享；作为 M102「两套全新环境安装/升级/回滚」的本地完整证据。真实组织 kind/Helm 生命周期仍需授权后由 CI/组织环境补齐；M89/M90 未完成前版本保持 RC，不宣称 GA。
- See [M102 dual-env compose drill change record](docs/changes/2026-08-12-m102-dual-env-compose-drill.md).

### Added - M89 OIDC Local Login Drill (Identity Track)

- 新增本地 OIDC Provider 驱动工具 `backend/cmd/oidc-provider`（运行于 HTTPS，自签证书；discovery/JWKS/authorize/token/logout 端点，RS256 + PKCE S256 + 一次性 authorization code + nonce/state 回显 + acr MFA 证据；支持 9 种 `?fail=` 注入模式）与进程内测试。
- 新增全链路演练 `scripts/oidc-login-drill.sh`：起仓库内 IdP + 第二个带 OIDC 的平台后端（读同一本地 PostgreSQL），curl 驱动真实 Authorization Code + PKCE 登录，14/14 场景通过并落报告 `.artifacts/oidc-drill/report-*.json`：happy path（login 302 → callback 200 → `/me` 200 且 `operations_admin`）、缺预关联 403 `OIDC_SUBJECT_NOT_PRELINKED`、nonce/state 篡改 502、缺/不接受 MFA 证据 502、组无角色映射 502、轮换密钥 502、过期/unsigned token 502、审计落库、Provider 运行中 token 端点不可达 502、Provider 下线后 discovery 缓存过期 502 `OIDC_UNAVAILABLE`、Provider 启动期不可达导致 server 启动失败退出。
- macOS 依赖 Keychain 信任自签证书（`security add-trusted-cert`/`delete-certificate`，退出自动清理）；演练在宿主机另起后端进程，不干扰运行中 compose 栈。真实 OIDC/MFA 验收仍需组织授权 Provider，状态保持 Deferred。
- See [M89 OIDC local drill change record](docs/changes/2026-08-12-m89-oidc-local-drill.md).

### Added - M100D Dependency & Supply-Chain Gates

- 修复首次扫描发现的真实漏洞：`pgx/v5` v5.6.0→v5.9.2（GO-2026-5004 SQL 注入）、`quic-go` v0.59.0→v0.59.1（GO-2026-5676 HTTP/3 内存耗尽）；前端 `pnpm-workspace.yaml` overrides `nanoid` 3.3.17（GHSA-2v37-7h3g-55p8）。govulncheck 可达漏洞 2→0，`pnpm audit --prod` 无已知漏洞。
- 新增供应链门禁：`scripts/dependency-vuln-scan.sh`（govulncheck + pnpm audit 生产依赖 fail-closed）、`scripts/license-scan.sh`（Go/前端许可证 allowlist，UNKNOWN fail）、`scripts/image-base-drift.sh` + `docs/security/image-base-manifest.md`（4 基础镜像 digest 漂移 fail-closed）、`scripts/sbom-diff.mjs`（SPDX 差异，新增包默认 fail-closed）；CI 新增 `dependency-scan` job。
- 追踪例外：`golang.org/x/crypto` openpgp（GO-2026-5932，未调用、无修复版）与 4 个前端 dev 工具链告警（不进入产物），随 Dependabot 主版本窗口复评。
- See [M100-D change record](docs/changes/2026-08-12-m100d-dependency-and-supply-chain-gates.md).

### Added - M100C Sensitive-Field Scan & Log Redaction Gates

- 新增敏感字段静态扫描门禁 `scripts/scan-sensitive-fields.sh` + allowlist：扫描全部 git-tracked 文件，fail-closed 检出私钥块、内联 kubeconfig 凭据数据、云/API token（AKIA/ghp_/xoxb-/sk-/AIza）、JWT、被跟踪的密钥文件与非占位符 `PASSWORD=` 赋值；CI 新增 `sensitive-scan` job（含文档-only 变更也执行）。
- 新增 3 个日志/审计脱敏契约测试：请求日志永不输出 query/header/Cookie/body 中的凭据；审计条目 `Details` 为 method/path_template/cluster_id 固定闭集；`/audit-logs/export` CSV 全链路不含凭据值。
- 运行时复验：重建 backend 镜像后角色变更使旧 access token 立即 401；容器日志与 190 行审计导出对冒烟密码/管理密码 0 命中。
- See [M100-C change record](docs/changes/2026-08-12-m100c-sensitive-field-scan-and-log-redaction.md).

### Added - M100B Session Invalidation Journeys

- 补齐安全变更失效契约：`UpdateUser` 的 `securityChanged` 分支（禁用用户、角色变更）新增 `auth_version + 1`，与 `ResetPassword`/`ChangePassword` 对齐——存量 access token 立即以 `ErrInvalidAccessToken` 拒绝，refresh session 全部撤销，用户必须重新认证（此前角色变更后旧 token 仍携带旧角色被中间件信任）。
- 新增 4 个会话失效旅程测试（禁用 / 角色变更 / 本人改密 / 管理员重置），`repositoryStub` 建模仓库失效契约；运行时冒烟 14 项旅程全部符合预期（改密/重置后旧 token 401、旧 refresh 401、旧密码失效；禁用后存量 token 403、登录 403）。
- See [M100-B change record](docs/changes/2026-08-12-m100b-session-invalidation-journeys.md).

### Added - M100A Permission Matrix & AIOps Query Scope Enforcement

- 新增路由权限矩阵生成器（`BuildPermissionMatrix` + 确定性 Markdown 渲染）与差异门禁：`docs/security/permission-matrix.md` 必须与实时 RouteDescriptor 注册表一致（`-update` 重新生成），279 条路由按角色/scope（workspace/cluster/namespace/none）/审计动作归档。
- 修复真实授权缺口：`/aiops` 读路由新增 `requireClusterQueryAccess` 中间件，`?cluster_id=`/`?namespace=` 按集群/命名空间授权校验，拒绝返回 404（M35 反泄漏）；无授权 viewer 探测任意集群的 signals/slos/correlation 由“返回数据”变为 404。
- 提取完整生产路由 harness（`buildFullEngine`）供 OpenAPI 契约测试与权限矩阵测试共享。
- See [M100-A change record](docs/changes/2026-08-12-m100a-permission-matrix-and-aiops-scope.md).

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
