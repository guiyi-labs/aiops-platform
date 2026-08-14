# M113-3 巡检趋势与覆盖率度量

- Date: 2026-08-14
- Status: Complete
- Scope: M113 第三个切片——plan→findings 时间序列、规则命中覆盖率、执行方式分布；空窗口 fail-closed 不视为健康；附带修复所有 M52 巡检路由在 OpenAPI 中的缺文档空白。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track D 第 161–164 行：巡检趋势与覆盖率度量（plan→findings 时间序列、规则命中覆盖率、计划调度改进（M52 巡检深化）；数据可见性沿用 M99-D 的显式覆盖度展示约定）。
- 验收：巡检度量有真实数据且 fail-closed 无样本不视为健康。
- M52 巡检系统已有完整 Plan / Task / Result（finding）三表模型与 CRUD handler；本切片新增一个跨表聚合只读接口。

## What Changed

### 纯包 `internal/inspection`：Coverage 汇总

- `model.go`：新增 `CoverageSummary` / `CoverageTrendPoint` 两类型（scope / window_days / plan_total / plan_enabled / task_total / task_completed / task_failed / task_scheduled / task_manual / finding_total / distinct_rule_codes / by_severity / rule_coverage / trend / fail_closed / empty_note），沿用 M99-D 资源上下文契约（scope / observed_at / fail_closed）。
- `repository.go`：`Repository` 接口新增 `Coverage(ctx, windowDays, now) (CoverageSummary, error)`；`GormRepository` 实现为四段 SQL 聚合（plans 计数 + tasks 状态/触发源/按天 + results 发现总数/去重规则码/严重级别/按天），均仅扫描索引字段；`itoa` → `intToStr` 避免与测试文件同包重名；增加 `sort` / `strconv` / `time` 导入。
- `service.go`：新增 `Service.Coverage(ctx, windowDays)`；调 `s.repo.Coverage(...)` 后用编译期 `s.catalogList`（规则目录）计算 `rule_coverage = distinct_rules / catalog_size`；两层 fail-closed 防御：repo 层在 finding_total=0 时置 true，service 层在 task_total=0 且 finding_total=0 时兜底。
- `service_test.go`：
  - `inMemRepo.Coverage()`：完整内存聚合实现（plan 计数 + task 按天 + result 按天 + 严重级别 + 重规则码 + 趋势排序），使 `TestServiceCoverage_AggregatesWindow`（三种触发源 + 跨窗口结果过滤 + 严重级别）与 `TestServiceCoverage_FailClosedEmpty`（空仓库）在独立单测中执行。
  - `TestServiceCoverage_AggregatesWindow`：两个计划（enabled/disabled）+ 三任务（schedule×2/completed + manual×1/failed）+ 四结果（窗口内三条跨两规则、窗口外一条），断言 plans/tasks/triggers/findings/rules/severity/trend 全部正确，fail_closed=false。
  - `TestServiceCoverage_FailClosedEmpty`：空仓库必返回 fail_closed=true。

### HTTP 层 `inspection.go` + `inspection_test.go`

- 新增 `inspectionHandler.coverage`：验证 `window_days` query 参数（1–365 整数），调 `service.Coverage()` 返回 JSON。
- 测试文件：`inspectionRepoNoop.Coverage()` 返回带 scope + fail_closed=true 的空聚合（防御性 fake，对齐真实 repo 语义）；新增 `TestInspection_CoverageReturnsValidJSON`（200 + JSON 完整字段 + fail_closed=true for empty repo）与 `TestInspection_CoverageBadWindow`（window_days=0 → 400 INVALID_WINDOW）；`newInspectionTestEngine` 路由表新增 `GET /coverage → h.coverage`。

### 路由注册 `router.go`

- 在 `options.InspectionService != nil` 块内新增 `GET /inspection/coverage`（AuthRequired；AuditAction `aiops.inspection.coverage.read`；AuditResource `InspectionCoverage`）。

### OpenAPI（原有 M52 漏洞修复 + M113-3 新增）

- `docs/api/openapi.yaml`：
  - 新增 11 条原有 M52 巡检路由（`/aiops/inspection/rules/catalog`、`/aiops/inspection/plans`、`/aiops/inspection/plans/{id}`、`/aiops/inspection/run`、`/aiops/inspection/tasks`、`/aiops/inspection/tasks/{id}`、`/aiops/inspection/results`、`/aiops/inspection/results/{id}`、`/clusters/{cluster_id}/inspection/rules`）及对应 schemas（`InspectionPlanView` / `InspectionTaskView` / `InspectionResultView`），修复此前路由合约测试因 harness 不注册 `InspectionService` 而未暴露的全量 OpenAPI 空白。
  - 新增 `GET /aiops/inspection/coverage`（operationId `inspectionCoverage`；参数 `window_days`（1–365，默认 30））与 `InspectionCoverageResponse` schema。
- `docs/security/permission-matrix.md`：重生成，登记 `aiops.inspection.coverage.read`（any-auth；no required roles）。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。

### 路由合约测试 `router_harness_test.go`

- 新增 `mustInspectionService(t)` 辅助函数：用 `inspectionRepoNoop{}` + `inspectionExecutorNoop{}` + `inspectionClusterListerNoop{}` 构造真实 `inspection.Service`（仅注册路由，不执行任何检查逻辑），与现有 `mustMetricsHistoryService` / `mustAlertService` 模式一致。
- `buildFullEngine()` Options 新增 `InspectionService: mustInspectionService(t)`，使所有 M52+M113-3 巡检路由纳入 `TestRegisteredRoutesMatchOpenAPI` + `TestPermissionMatrixMatchesCommittedDocument` 双向校验。

### 前端

- `frontend/src/types/inspection.ts`：新增 `InspectionCoverageTrendPoint`（day/tasks/findings）与 `InspectionCoverageSummary`（scope / window_days / plan_total / … / trend / fail_closed / empty_note）。
- `frontend/src/api/inspection.ts`：新增 `getInspectionCoverage(token, params?)`（GET；空参数省略 query string）+ import `InspectionCoverageSummary`。
- `frontend/src/api/inspection.test.ts`（**新建**）：两测试——默认无参数请求（断言 URL 不含 query）+ 传 `window_days=7`（断言 URL 含 `?window_days=7`）；验证 Authorization header 与响应字段解析。
- `frontend/src/views/InspectionView.vue`：
  - import 新增 `TrendingUp`（lucide 图标）+ `getInspectionCoverage` + `InspectionCoverageSummary`。
  - 新增响应式状态：`coverage` / `coverageLoading` / `coverageError` / `coverageWindowDays`。
  - 新增 `loadCoverage()`（调 getInspectionCoverage；失败显示错误横幅）与 `trendBarHeight(point)`（按日最高发现数缩放柱形高度；零发现保留 6% 最小可视 stub）。
  - `refreshAll()` / `onMounted()` 均并入 `loadCoverage()`。
  - Template 在"任务与结果"区之后新增"覆盖率与趋势"section-block：窗口选择（7 / 30 / 90 天）、fail_closed 黄色告警横幅、四指标卡片（计划/任务/发现/规则覆盖率）、严重级别分布表、每日趋势柱形图（纯 CSS，hover 显示日期/任务/发现数）。
  - 新增 scoped CSS（`.coverage-grid` / `.coverage-card` / `.coverage-detail` / `.trend-bars` / `.window-select` / `.warn-message`；`@media (max-width: 720px)` 响应式降级），全部使用平台现有 token（`--surface` / `--border` / `--accent-primary` / `--status-warning` / `--warning-bg` / `--text-primary` / `--text-secondary`）。

## Verification

- `go test ./...`：69 包全绿（inspection + httpserver 校验测试，含 `TestServiceCoverage_AggregatesWindow` / `TestServiceCoverage_FailClosedEmpty` / `TestInspection_CoverageReturnsValidJSON` / `TestInspection_CoverageBadWindow`）。
- `pnpm typegen` / `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（**155** 通过）/ `pnpm build`：全绿。
- OpenAPI 路由匹配（`TestRegisteredRoutesMatchOpenAPI`）+ 权限矩阵一致性（`TestPermissionMatrixMatchesCommittedDocument`）：全绿（新增 harness wiring `InspectionService` 使 M52 全部路由首次纳入双向校验）。
- 只读性：纯读 SQL 聚合，无状态变更，无写路径；前端无写按钮。
- fail-closed 语义：空窗口/无任务/无发现均返回 `fail_closed=true`；service 防御性兜底不依赖特定 repo 实现；前端黄框告警引导"需先执行巡检产生数据"。

## Risks / Notes

- 规则覆盖率分母使用编译期 catalog（`DefaultCatalog().len`），不包含用户在运行时动态新增的 override 规则码；分母固定，分子来自结果表去重，语义清晰：平台内置规则集合中有多少比例曾产出至少一个 finding。
- 趋势柱形图是纯 CSS 实现（无 SVG/Canvas），浏览器端无需额外依赖；窗口较大（90 天）时柱形水平滚动，不影响竖向布局。
- `router_harness_test.go` 新增 `InspectionService` 后，所有 M52 巡检路由（此前从未纳入 OpenAPI 路由合约校验）现在均需满足 OpenAPI 文档存在，故本次一并补齐 11 条原有路由文档；这属于既有技术债修复。
- M113 全部三个切片（M113-1 finding→runbook 导航 / M113-2 容量感知预览 / M113-3 巡检覆盖率）均已提交，后续步骤：CHANGELOG + git commit + tag + Obsidian 同步。