# M112-1 事故上下文驾驶舱：资源上下文契约首次落地

- Date: 2026-08-14
- Status: Complete
- Scope: M112 第一个切片——事故上下文驾驶舱（Context Cockpit）后端聚合接口、前端首屏区块与跨 M112–M114 的资源上下文契约（scope/observed_at/来源/freshness/空样本语义）首次落地。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track C（M112 AI 协调查询与解释深化，P0）。
- M112 分解为四个切片：M112-1 上下文驾驶舱、M112-2 会话式调查、M112-3 AI 事故摘要、M112-4 解释覆盖率大盘。
- 变更记录 `docs/changes/2026-08-14-roadmap-resource-context-integration.md`（提交 `6a29ee5`）已确立 RPC 契约要求；
  本切片是第一个真正返回该契约的接口，为后续 M112-2/3/4 与 M113/M114 提供可复制的响应形状。
- 不新增任何写路径、不调用 Kubernetes API、不伪造集群健康：驾驶舱完全由 incident 快照、证据解析器与
  insight runbook 映射的确定性数据组装。

## What Changed

### 后端
- `backend/internal/incident/context.go`（新增）：定义 `ResourceContext`（scope/observed_at/source/freshness/empty_sample）
  与 `ContextCockpit`（incident 快照、SLA、Health、证据来源汇总、最近 10 条时间线、runbook brief、只读 dry-run 建议动作）；
  `BuildContextCockpit` 为纯确定性函数，Freshess 由最旧证据的 observed_at 派生，空样本语义固定为 `fail_closed`。
- `backend/internal/incident/context_test.go`（新增）：契约块字段断言、证据聚合计数、时间线最新优先与 10 条上限、
  空域 runbook 返回 nil（不误报 `RunbookAvailable`）。
- `backend/internal/httpserver/incidents.go`：新增 `context` handler——evidence 解析失败回退 incident 快照、runbook
  来源域缺失时 `RunbookAvailable=false` 而非报错；`GET /api/v1/incidents/:incident_id/context`。
- `backend/internal/httpserver/incidents_test.go`：新增 handler 测试——200 契约字段、404 缺失事故、无 resolver 时
  fail-closed（runbook_brief=nil 且 health.runbook_available=false）。
- `backend/internal/httpserver/router.go`：注册路由 `GET /incidents/:incident_id/context`（AuditAction
  `incident.context.get`，AuditResource `IncidentContext`，读写角色均只读访问）。

### OpenAPI / 权限矩阵 / 前端
- `docs/api/openapi.yaml`：新增 `/api/v1/incidents/{incident_id}/context` 与 `IncidentContextCockpit` schema
  （含 resource_context 契约块、evidence_sources、recent_events、recommended_actions）。
- `docs/security/permission-matrix.md`：由门禁自动重生成，登记 `incident.context.get`。
- `frontend/src/api/openapi.d.ts`：`pnpm typegen` 重生成。
- `frontend/src/types/incident.ts`：新增 `IncidentContextCockpit` 及配套类型。
- `frontend/src/api/incidents.ts`、`incidents.test.ts`：新增 `getIncidentContext` 客户端与两条用例
  （契约块可用、无 runbook fail-closed）。
- `frontend/src/views/IncidentsView.vue`：事故详情抽屉新增「上下文驾驶舱」区块（契约标签、健康/SLA 指标、
  证据来源深链、建议动作只读列表），并在打开详情时并行加载。

## Verification

- `go build ./...`：通过。
- `go test ./...`：全绿（含 OpenAPI 路由匹配、权限矩阵一致性、全新 cockpit 单测与 handler 测试）。
- `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（150 通过）/ `pnpm build`：全绿。
- `go test ./internal/httpserver -run TestPermissionMatrixMatchesCommittedDocument -update`：矩阵已同步。
- 未改动数据库迁移（本切片为只读聚合，无持久化变更）。

## Risks / Notes

- 驾驶舱的 Health 是**事故生命周期健康**（状态/SLA/证据可用性），不是集群实时健康；实时集群健康仍通过
  证据 deep link 与后续 M114 事件驾驶舱承载，避免本切片伪造数据。
- M112-2/3/4 将复用同一 `ResourceContext` 契约形状；后续若新增聚合（事件驾驶舱、覆盖率大盘）应直接复用
  `incident.ResourceContext`，不做第二种响应包装。
- M110 RC-6 发布仍待用户授权；本切片不依赖发布动作。