# 控制台 7 个页面不可用：未装配的服务、契约不一致与空集合崩溃

- **日期**：2026-10-29
- **类型**：修复（后端装配 / API 契约 / 前端健壮性）
- **影响面**：控制台 35 个路由中的 7 个
- **发现方式**：对全部 35 个路由做无头浏览器巡检，同时采集 `pageerror`、`console.error` 与 4xx/5xx 请求

## 现象

用户报告「有一些页面打不开」。巡检结果分三类：

| 页面 | 现象 | 直接原因 |
|---|---|---|
| AIOps 概览 | 主内容区完全空白 | 前端渲染时抛 `TypeError: Cannot read properties of null (reading 'length')` |
| AI 调查 | 两处面板显示 `the requested route does not exist` | 服务未装配，路由未注册（404） |
| 自动化控制台 | 同上 | 同上 |
| 质量仪表盘 | 同上 | 服务已装配，但报告只存内存，重启即 404 |
| 拓扑图（概览内） | `INVALID_QUERY: namespace is required` | 前端传空 namespace，后端按设计必填 |
| 服务网格 | `service mesh query failed`（500） | Istio GVR 不在 CRD 白名单，错误被兜成 500 |
| 监控大盘 | `INVALID_QUERY: from must be an RFC3339 timestamp` | 前端未传 `from`/`to` |
| 事件中心 | `no events found in the time window (fail-closed)` | **非缺陷**：K8s Event 默认只保留 1 小时，窗口内确实为空 |

## 根因

### 1. 四个服务从未被构造，路由因此从未注册

`internal/httpserver/router.go` 的路由注册全部包在 `if options.XxxService != nil` 里。`cmd/server/main.go` 的
`httpserver.Options` 字面量中缺少 `TopologyService`、`AIInvestigatorService`、`AutomationService`、
`AlertRouteService` 四项，对应服务包（`internal/topology`、`internal/aiinvestigator`、`internal/automation`、
`internal/alertroute`）的仓储与迁移却都已就绪，表也是空的——即「写完了但没接上」。

修复：在 `main.go` 构造这四个服务并注入 Options。四个服务的 `NewService` 都对空依赖做了兜底
（`NopCaseReader`、`NopProvider`），因此本轮按查询能力装配：

- `topology`：collector 与 namespaceLister 传 nil（查询模式），不启后台采集
- `alertroute`：repo + logger
- `automation`：repo + `kubernetesService`（满足 `KubernetesSource`），case reader 用 Nop 兜底
- `aiinvestigator`：repo，provider 留空（AI 关闭时由 `NopProvider` 兜底）

**遗留**：`automation`/`aiinvestigator` 的 `CaseReader` 适配器（`GetCase` + `EligibleActionCodes`）尚未实现，
因此这两个页面只支持目录浏览与列表，无法基于具体关联用例生成计划/调查。`topology` 因为没有采集器，
边表始终为空，图为空图。

### 2. nil slice 序列化为 JSON `null`，前端当数组用

Go 的 `nil` slice 编码为 `null`。三处因此崩溃或降级：

- `signal.Overview.TopSignals` / `RecentChanges` → `overview.recent_changes.length` 抛错，**整页渲染中断**
- `automation.ListPlans` 的 `Items` → 返回 `"items":null`
- `golden` 质量报告的 `changed_components` 为可选字段（`omitempty`），前端却直接 `.length`

修复：后端在构造响应处初始化空切片（`signal/service.go`、`automation/service.go`）；前端对可选字段用
`?.length ?? 0` 与 `?? []` 兜底（`QualityDashboardView.vue`）。

> 说明：这类缺陷不会在单测里暴露——单测直接比对结构体，不经过 JSON 序列化，也不渲染前端。

### 3. runbook 契约两端不一致

后端 `RunbookDescriptor`（`aiinvestigator/catalog.go`、`automation/catalog.go`）**没有 json tag**，
序列化成 PascalCase（`RunbookID`、`ActionCodes`…）；前端类型按另一套更丰富的模型写
（`id`、`display_name`、`action_codes[]`、`prerequisites[]`、`level`、`rollback_strategy`）。

`docs/api/openapi.yaml` 的 `InvestigatorRunbook` 定义的是
`runbook_id / action_code / title / steps[]`——即后端字段正确、缺 tag，前端类型凭空多出三个字段。

修复：后端补 json tag；前端 `Runbook` 类型与两个视图的卡片模板按契约重写（`action_code` 为空即标「建议性」）。

### 4. 服务网格与 CRD 白名单不一致

`internal/servicemesh` 查询 `networking.istio.io/v1beta1/{virtualservices,destinationrules}`，
但 `internal/kubernetes` 的 `customResourceWhitelist` 里没有任何 istio 条目。查询在进入 K8s API 之前
就被拒为 `ErrCustomResourceNotWhitelisted`，handler 落进 `default` 分支 → 500 `SERVICEMESH_FAILED`。
即使集群真装了 Istio 也一样失败。

修复：白名单补两条 istio GVR。此后未装 Istio 的集群返回 404 `SERVICEMESH_NOT_INSTALLED`，
前端显示「该集群未安装 Istio」的中性提示（而非红色错误条）。

### 5. 监控大盘与拓扑图缺参数

- `clusterDashboard` 的 `from`/`to` 为必填（`parseMonitoringTime`），前端 `getClusterDashboard` 一个都不传。
  修复：前端默认取最近 1 小时；`getWorkspaceDashboard` 同样处理。
- `getTopologyGraph` 按命名空间返回子图，`namespace` 必填；概览页命名空间输入框留空时传 `undefined`。
  修复：留空兜底为 `default`。

### 6. 质量报告不持久

`golden.Service` 把报告存在内存（`tasks`/`latest`），没有表。后端一重启就回到
`404 QUALITY_REPORT_NOT_FOUND`。修复：启动时调用一次 `RunReplay`（异步、有界），失败仅告警不致命。

## 验证

- 全站巡检：35 个路由，`pageerror` + `console.error` + 4xx/5xx 均为 0（服务网格的 404 属「未安装」的预期语义，
  页面显示中性提示；事件空白属 K8s Event 1h TTL）
- `go test ./internal/automation ./internal/kubernetes ./internal/servicemesh ./internal/signal` 通过
- `vue-tsc -b` 无错
- 各页主内容区文本量：概览 199 / AI 调查 1123 / 自动化 556 / 质量 329（修复前 41–42，即只有标题）

## 未解决

1. `automation` / `aiinvestigator` 的 `CaseReader` 适配器未实现——目录可浏览，按用例生成不可用
2. `topology` 无采集器，边表为空，图为空图
3. `slo` 列表不带 `cluster_id` 时返回空（需带过滤参数），属接口易用性问题，未改
