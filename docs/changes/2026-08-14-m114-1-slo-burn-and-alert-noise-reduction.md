# M114-1 SLO Burn 总览与告警降噪聚合

- Date: 2026-08-14
- Status: Complete
- Scope: M114 可观测性深化——两个只读聚合端点，复用 correlation 引擎驱动告警降噪，复用既有 SLO 评估产出 burn 总览视图；全部有界查询、fail-closed 空窗、无写路径、无新 DB schema。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track E 第 166–177 行：SLO burn 扩展与告警降噪——更多 SLO 来源信号化（M99-A 管道扩展）、关联驱动的告警去重/聚合展示（复用 correlation 引擎）。
- 验收：所有查询有界；关联聚合保留原始证据深链；无新增全量写入平台数据库的路径。
- 现有基础设施：
  - `internal/slo`：DefinitionFilter（ClusterID/Namespace/Template/Enabled/Limit）+ Service.ListDefinitions/LatestEvaluation；EvaluationFilter + ListEvaluations；Evaluation{State（healthy/burning_slow/burning_fast/breached/unavailable）/Coverage/BurnRate/RemainingBudget}；SLITemplate catalog。
  - `internal/alert`：RuleListFilter + InstanceListFilter（State/Limit）+ Service.ListRules/ListInstances；Instance{RuleID/State/FirstFiredAt/LastFiredAt/ResolvedAt}。
  - `internal/correlation`：CaseFilter{ClusterID/Status/Limit} + Service.ListCases（返回 Case{PrimaryResource{Kind/Name/UID}/Status/RuleID}）。
  - M99-A 信号化管道已存在（`internal/signal/slo_burn_normalizer.go`）；M114-1 的 "更多 SLO 来源" 受限于评估入口仅 HTTP 触发（无周期 worker），因此本次取最简有界只读实现——burn posture 总览；Prometheus 源适配需 capability 配置，另立跟踪。

## What Changed

### 新增纯包 `internal/alertoverview`（ADR 0004，无 cluster 访问、无副作用）
- `model.go`：
  - `RuleRef{ID/DisplayName/ResourceKind/ResourceName/MetricName}`、`InstanceRef{RuleID/State/FirstFiredAt/LastFiredAt/ResolvedAt}`、`CaseRef{ID/Status/ReasonCode/Resources[]ResourceRef{Kind/Name/UID}}`。
  - `Group{RuleID/DisplayName/ResourceKind/ResourceName/MetricName/FiringCount/ResolvedCount/FirstFiredAt/LastFiredAt/RelatedCaseIDs[]}`。
  - `Response{Scope/ObservedAt/WindowMinutes/GroupsTotal/Groups/TotalFiring/TotalResolved/FailClosed/EmptyNote}`；常量 MinWindowMinutes=1/MaxWindowMinutes=10080/MinGroups=1/MaxGroups=200。
  - `Aggregate(rules, instances, cases, window, now, maxGroups)`：按 RuleID 分组；窗口过滤（resolved 仅保留 LastFiredAt ≥ now−window）；关联 Case 匹配（资源 kind+name 一致 → RelatedCaseIDs）；排序优先有关联案例 → LastFiredAt desc → DisplayName asc；截断到 maxGroups；`len(groups)==0` 时 `FailClosed=true`。
- `model_test.go`：5 个用例——按规则分组+计数（同规则 2 条 firing + 1 条 resolved）、resolved 窗口过滤、空窗 fail-closed、关联案例链接+排序优先、maxGroups 截断。**全绿**。

### 新增纯包 `internal/sloburnsummary`（ADR 0004，无 cluster 访问、无副作用）
- `model.go`：
  - `DefRef{ID/ClusterID/Service{Kind/Namespace/Name}/Template/Objective}`、`EvalRef{State/BurnRate/Ratio/Coverage/ErrorBudget/RemainingBudget/EvaluatedAt}`。
  - `BurnStatus` 常量：StatusBurning/StatusHealthy/StatusUnavailable/StatusNoData。
  - `Item{SLOID/ClusterID/Service/Template/Objective/Status/BurnRate/Ratio/Coverage/ErrorBudgetRemaining/EvaluatedAt}`。
  - `Response{Items/Total/Truncated/ObservedAt}`。
  - `Summarize(defs, latest, limit)`：无 eval → no_data；Coverage unavailable → unavailable；state==healthy → healthy；其余（burning_fast/burning_slow/breached）→ burning；按 status 优先级排序（burning→unavailable→no_data→healthy）+ BurnRate 降序 + SLOID 升序；截断到 limit。
- `model_test.go`：3 个用例——4 条不同状态分类+排序、截断、空输入。**全绿**。

### HTTP Handler `internal/httpserver/alert_overview.go`
- `alertOverviewRequest{WindowMinutes(1–10080，默认1440)/MaxGroups(1–200，默认50)/Limit(1–200，默认100)}` + `parseAlertOverviewRequest`（400 INVALID_WINDOW/INVALID_GROUPS/INVALID_LIMIT）。
- `alertHandler.overview`：解析 clusterID + 请求参数 → `h.service.ListRules` + `h.service.ListInstances`（均有界 Limit）；若 `h.correlation != nil` 则 `h.correlation.ListCases(ClusterID, Status=active, Limit=100)` 获取活跃案例，遍历 `caseResp.Items` 展平 PrimaryResource → `alertoverview.CaseRef`；调用 `alertoverview.Aggregate` 返回 JSON。
- `alertHandler` struct 新增可选字段 `correlation *correlation.Service`（nil 时仅跳过案例关联，端点仍可聚合）。

### HTTP Handler `internal/httpserver/slo.go` 新增 `burnSummary`
- `sloHandler.burnSummary`：解析 cluster_id/namespace/template/state/limit（≤200，默认50）→ `h.service.ListDefinitions`（Enabled=true）→ 逐条 `h.service.LatestEvaluation`（N+1，N≤200）→ `sloburnsummary.Summarize`；若提供 state 查询参数，后置过滤仅保留匹配 status 的 Item。返回 JSON。
- 新增导入 `sloburnsummary`。

### 路由注册 `router.go`
- `alertRoutes` 块新增 `GET /clusters/:cluster_id/alerts/overview`（AuthRequired；`alertHandler.overview`；AuditAction `alert.overview.read`；AuditResource `AlertOverview`）——在 `/:alert_id` 之前注册，gin 静态路径优先。
- `aiopsRoutes` SLO 块新增 `GET /aiops/slos/burn-summary`（AuthRequired；`sloHandler.burnSummary`；AuditAction `aiops.slo.burn.list`；AuditResource `SLOBurnSummary`）——在 `:id` 之前注册。
- `alertHandler` 构造改为 `alertHandler{service: options.Alert, users: options.Auth, correlation: options.CorrelationService}`。

### OpenAPI / 权限矩阵 / typegen
- `docs/api/openapi.yaml`：
  - 新增 `/api/v1/clusters/{cluster_id}/alerts/overview`（operationId `alertOverview`，参数 window_minutes/max_groups/limit，description 注明 fail-closed 与有界语义）+ `AlertOverviewGroup` / `AlertOverviewResponse` schema。
  - 新增 `/api/v1/aiops/slos/burn-summary`（operationId `sloBurnSummary`，参数 cluster_id/namespace/template/state/limit，description 注明纯读/有界）+ `SLOBurnSummaryItem` / `SLOBurnSummaryResponse` schema。
- `docs/security/permission-matrix.md`：重生成（`alert.overview.read` any-auth + cluster scope；`aiops.slo.burn.list` any-auth + none scope）。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。

### Handler 测试
- `alert_overview_test.go`：4 个用例——参数校验（bad window/max_groups/limit）、聚合实例（2 条同规则 + 1 条 resolved → 2 组 + counts + display_name from rule join + fail_closed=false）、空窗 fail-closed、invalid cluster_id → 400。使用 `alertRepoStub`（同 existing tests）+ 新增 `newAlertOverviewRouter` helper。
- `slo_burn_summary_test.go`：2 个用例——nil service → 503 + INVALID_QUERY（bad cluster_id/limit）+ empty definitions → 200 with `items:[]`。

### 前端
- `frontend/src/types/alert.ts`：新增 `AlertOverviewGroup` / `AlertOverviewResponse`。
- `frontend/src/api/alert.ts`：新增 `getAlertOverview(token, clusterID, params)`（GET `/clusters/:id/alerts/overview`）+ 客户端测试 `alert.test.ts`（2 个用例：查询参数拼接 + Bearer 认证）。
- `frontend/src/types/aiops.ts`：新增 `BurnStatus` / `SLOBurnSummaryItem` / `SLOBurnSummaryResponse`。
- `frontend/src/api/aiops.ts`：新增 `getSLOBurnSummary(token, params)`（GET `/aiops/slos/burn-summary`，支持 cluster_id/namespace/template/state/limit）+ aiops.test.ts 新增用例（查询参数含 state/cluster_id/limit）。
- `frontend/src/views/AlertsView.vue`：
  - 新增 `getAlertOverview` import + `AlertOverviewResponse` type + `overview` / `overviewLoading` / `overviewWindowMinutes(默认1440)` 状态；`loadOverview()` / `overviewSeverityType()` helper；`watch(overviewWindowMinutes)` 触发重载；`refresh()` 加入 `loadOverview`。
  - Template 新增「告警降噪 · 规则维度聚合」面板：窗口切换（1h/6h/24h/7d）、fail_closed 黄色告警、统计卡（聚合组/触发中/已恢复/关联案例数）、`el-table` 显示规则/资源/触发恢复计数/首次最近触发/关联案例深链（`/correlation?case=N`）。
- `frontend/src/views/SLODashboardView.vue`：
  - 新增 `getSLOBurnSummary` import + `SLOBurnSummaryItem` type + `burnSummary` / `burnSummaryLoading` 状态；`loadBurnSummary()` / `burnStatusLabel()` helper；`changeCluster()` 和 `initialize()` 均调用 `loadBurnSummary()`。
  - Template 新增「Burn 总览 · N 个 SLO」面板：`burn-summary-strip` > `burn-summary-grid` > `burn-card`（status 颜色：burning 红底/healthy 绿底/warning 黄底/灰底），展示服务名/命名空间/Kind、Burn ×率、Ratio、Error Budget；scoped CSS。

### 全局 CSS `src/styles/base.css`
- 新增 `.noise-panel` / `.noise-panel-heading` / `.noise-stats` / `.noise-stat` / `.noise-muted` 等样式（沿用平台 token），stats 4 列自适应 grid。

## Verification

- `go test ./...`：72 包全绿（+2 新增纯包）；OpenAPI 路由双向匹配 + 权限矩阵一致性通过。
- `pnpm typegen` / `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（**159** 通过）/ `pnpm build`：全绿。
- 只读性：两个端点均只读取现有 DB 表（alert_rules/alert_instances/slo_definitions/slo_evaluations）和 correlation cases；纯包零副作用；不写任何数据。
- 有界查询：
  - Alert overview：window_minutes 1–10080、max_groups 1–200、limit 1–200 三重上限全部校验（400 INVALID_*）。
  - SLO burn-summary：cluster_id/namespace/template/state 过滤 + limit 1–200 上限 + 仅 enabled 定义。
- fail-closed：告警 overview 聚合组为空 → FailClosed + EmptyNote "no alerts in window"；前端黄框告警，不显示"健康"。
- 案例关联：告警降噪聚合每个 Group 附带 RelatedCaseIDs，匹配规则资源 kind+name 与 correlation case PrimaryResource 一致；前端显示 `#/correlation?case=N` 深链。CorrelationService 可选（nil 时跳过，端点仍可聚合，仅 RelatedCaseIDs 为空）。
- SLO burn posture：无 eval → no_data；Coverage unavailable → unavailable；state=healthy → healthy；其余→burning；排序燃烧优先；状态过滤为后置（bounded by limit then filter），保证响应有界。

## Risks / Notes

- Alert overview 关联匹配仅基于 PrimaryResource（Case 列表项不含 ResourceLinks）；若告警规则针对的是 case 的次级资源（通过 ResourceLink 关联），当前不命中。后续可通过逐条 GetCase 获取 ResourceLinks 精确匹配，但会引入 N+1 查询（bounded by active case 数 ≤100），在性能要求更严格时再迭代。
- SLO burn-summary 使用 N+1 LatestEvaluation 调用（N ≤ limit ≤ 200）；当前评估仅 HTTP 触发，定义数有限。若后续有周期 worker + 大量定义，可改为 bulk LatestEvaluations SQL（当前 repository 无此方法）。
- Alert overview 的 resolved 过滤采用"窗口内 lastFiredAt"原则，不做 resolvedAt 判定——与告警调度器行为一致（resolved 实例仍保留 LastFiredAt 作为"最近状态变化时间"）。
- burn-summary 后置 state 过滤可能返回比 limit 少的 Items（filtered ≤ limit），但仍标 truncated=false——语义正确（不截断过滤后结果）。
- Prometheus SLO 源扩展（"更多 SLO 来源信号化"）需要 capability 模块 + cfg.Capability.Enabled 门禁，本次不纳入；可作为后续切片（docs/development-roadmap 中 Track E 已标注为可独立扩展的 follow-up）。
