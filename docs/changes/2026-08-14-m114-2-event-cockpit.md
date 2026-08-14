# M114-2 事件驾驶舱：重复事件聚合（严重级/原因/资源分组 + 趋势 + 深链）

- Date: 2026-08-14
- Status: Complete
- Scope: M114 可观测性深化第二个切片——把 Kubernetes 原生事件按严重级、原因、命名空间、资源和时间窗聚合为**事件驾驶舱**：去重折叠（event_count / raw_count）、首次/最近发生时间、按天趋势、原始证据深链；全程只读、有界查询、fail-closed 空窗不视为健康。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track E 第 170–171 行：事件驾驶舱——按严重级、原因、集群、命名空间和资源聚合重复事件，提供时间窗口、去重计数、首次/最近发生时间、异常趋势和深链；不只展示固定数量的原始事件。
- 验收：所有查询有界（时间窗/条数上限）；事件聚合不丢失原始证据深链；无新增全量写入平台数据库的路径。
- 数据源：`kubernetes.Service.Events()`（M51 已建立的只读 gateway，`/api/v1/events` 或 `/api/v1/namespaces/{ns}/events`），Event 模型带 Type（Normal/Warning → 严重级）、Reason、Message、Count、FirstTimestamp/LastTimestamp、InvolvedObject（kind/namespace/name/uid）。事件在 K8s 端本身带 `count` 折叠语义，驾驶舱在此基础上按维度聚合。
- 复用既有机制：`authorizedNamespaceLists` / `ResolvedNamespaceScope`（M35 命名空间授权）、`apiquery.ListQuery` 有界分页（limit ≤ 100）、路由审计 `kubernetes.events.cockpit.read`。无任何写路径。

## What Changed

### 新增纯包 `backend/internal/eventcockpit/`（ADR 0004，无 cluster 访问、无副作用）
- `model.go`：
  - `EventInput`（聚合所需的 k8s 事件投影：ID/Severity/Reason/Message/Count/FirstSeen/LastSeen/Namespace/Kind/Name/UID）。
  - `AggregatedGroup`（severity / reason / namespace / kind / resource_name / resource_uid / raw_count（折叠后的 k8s count 之和）/ event_count（折叠到本组的原始事件条数）/ first_seen / last_seen / sample_message）。
  - `TrendPoint`（按天：day / events / groups）。
  - `CockpitResponse`（scope / observed_at / window_minutes / groups_total / groups / trend / total_events / total_raw_count / fail_closed / empty_note），沿用 M99-D 资源上下文契约。
  - 常量：`MinWindowMinutes=1` / `MaxWindowMinutes=7*24*60` / `MinGroups=1` / `MaxGroups=200`。
  - `Aggregate(inputs, window, now, maxGroups)`：窗口外事件静默丢弃；按（severity, reason, namespace, kind, name）分组；severity 归一化（"Warning"→"warning"，其余→"info"）；按 raw_count 降序 + 稳定性排序后截断到 maxGroups；按天聚合趋势；`total_events==0` 时 `FailClosed=true` + `EmptyNote`。
- `model_test.go`：分组折叠（同组两条 CrashLoopBackOff → event_count=2/raw_count=15/首末时间不同）、窗口过滤（窗口外事件不计入）、严重级归一化、空窗 fail-closed、maxGroups 截断 + 排序、按天趋势有序。**6 个用例全绿**。

### 新增 HTTP 层 `backend/internal/httpserver/event_cockpit.go`
- `cockpitRequest`（window_minutes 1–10080 默认 1440 / max_groups 1–200 默认 50 / page_limit 1–1000 默认 500）+ `parseCockpitRequest`（400 INVALID_WINDOW / INVALID_GROUPS / INVALID_LIMIT）。
- `kubernetesHandler.eventCockpit`：解析 scope（`ResolvedNamespaceScope`）→ AllNamespaces 时查一次集群全量页；仅命名空间授权时逐命名空间取页（失败跳过，聚合已有的）；全部转 `EventInput` 后调用纯包 `Aggregate` 返回 JSON。
- `fetchEventsPage`：`service.Events(ctx, clusterID, ns, apiquery.ListQuery{Page:1, Limit:pageLimit})` → 转换字段；双时间戳全部缺失的事件跳过（无窗口可放）。
- `parseEventTime`：RFC3339 及宽松格式解析，失败返回零时间。
- `event_cockpit_test.go`：三个参数校验用例（window=0 / max_groups=500 / page_limit=9999 → 400 与对应错误码），用零值 `kubernetesHandler` 直接挂路由测试。

### 路由注册 `router.go`
- 在 `resourceRoutes`（`requireNamespaceAccess` + `requireNamespaceQueryAccess` + workspace 过滤）块内新增 `GET /clusters/:cluster_id/events/cockpit`（AuthRequired；AuditAction `kubernetes.events.cockpit.read`；AuditResource `EventCockpit`），仅在 `options.Kubernetes != nil` 时注册。

### OpenAPI / 权限矩阵 / typegen
- `docs/api/openapi.yaml`：新增 `/api/v1/clusters/{cluster_id}/events/cockpit`（operationId `eventCockpit`，参数 window_minutes/max_groups/page_limit，description 注明 fail-closed 与有界语义）与 `EventCockpitResponse` / `EventCockpitGroup` / `EventCockpitTrendPoint` 三个 schema。
- `docs/security/permission-matrix.md`：重生成（`kubernetes.events.cockpit.read`，any-auth + cluster 范围）。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。

### 前端
- `frontend/src/types/kubernetes.ts`：新增 `EventCockpitGroup` / `EventCockpitTrendPoint` / `EventCockpitResponse`。
- `frontend/src/api/kubernetes.ts`：新增 `getEventCockpit(token, clusterID, params)`（GET `/api/v1/clusters/:id/events/cockpit`，`queryString` 省略空参数）。
- `frontend/src/api/kubernetes.test.ts`：新增用例——请求拼参正确（`/clusters/9/events/cockpit` + `window_minutes=60` + `max_groups=30` + Authorization header）。
- `frontend/src/views/EventsView.vue`：
  - import `getEventCockpit` + `EventCockpitResponse`；新增状态 `cockpit` / `cockpitLoading` / `cockpitWindowMinutes`（默认 1440）。
  - `loadCockpit()`（失败置 null 不阻塞主事件流）、`cockpitSeverityClass()`、`trendBarHeight()`；`watch(selectedClusterID)` 与 `watch(cockpitWindowMinutes)` 触发加载；`onMounted` 初始化后加载。
  - Template：事件摘要四卡片改为「当前范围 / Warning 组 / Normal 组 / 原始事件（窗口内折叠前）」；新增「事件驾驶舱」面板——窗口选择（1h/6h/24h/7d）+ 刷新按钮、fail-closed 黄色告警横幅、三统计卡（聚合组/原始事件/累计次数）、聚合组表格（级别徽标、原因+命名空间、资源、首末时间、×raw_count + event_count 折叠提示）、按天趋势柱形图（hover 显示日期/事件数/组数）。
  - 移除原未使用的 `warningCount` / `normalCount` / `affectedResourceCount` / `selectedCluster` computeds（摘要卡已由驾驶舱指标取代）。
- `frontend/src/styles/base.css`：新增 `.event-cockpit-panel` / `.cockpit-*` / `.trend-bars` 等样式（沿用平台 token），响应式（≤1100px 单列、≤720px 卡片单列）。

## Verification

- `go test ./...`：70 包全绿（新增 eventcockpit 纯包 + httpserver 参数校验测试）。
- `pnpm typegen` / `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（**156** 通过）/ `pnpm build`：全绿。
- OpenAPI 路由匹配 + 权限矩阵一致性：通过（新增路径/audit 已入文档）。
- 只读性：纯包无 cluster 访问；handler 只走 `Event` 只读 gateway（每命名空间一页，page_limit ≤ 1000）；前端无写按钮；不写任何平台数据库（事件仍实时查 API Server）。
- 有界查询：window_minutes（1–10080）、max_groups（1–200）、page_limit（1–1000）三重上限全部校验。
- fail-closed：窗口内无事件 → `fail_closed=true` + `empty_note`；前端黄框告警，不显示虚假"健康"。
- 深链保留：每个聚合组携带 `resource_uid` / `kind` / `namespace` / `resource_name` / `first_seen` / `last_seen` / `sample_message`，可追溯到原始证据。

## Risks / Notes

- 事件在授权粒度上按命名空间聚合：AllNamespaces 授权取一次集群全量页（K8s 事件 API 一页通常 ≤ 5000 条，且本端 page_limit ≤ 1000）；仅命名空间授权时逐命名空间取页，跨命名空间失败跳过（部分结果可见，不整体失败）。
- 趋势按天（UTC）桶，窗口 ≤ 1 天时可能只出现 1–2 根柱；后续可扩展按小时桶（当前契约已为 `TrendPoint{day,...}`，扩展需 schema 变更评审）。
- 严重级仅区分 warning/info（K8s Event 本身只有 Normal/Warning 两态），与告警系统高/严重级语义不混用。
- 事件源为 API Server 实时查询，不做本地持久化，符合"无新增全量写入平台数据库的路径"验收。
- M114 剩余切片：M114-1 SLO burn 扩展与告警降噪、M114-3 指标历史下采样、M114-4 事件流/日志探索增强（另立 change-record）。