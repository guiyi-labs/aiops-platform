# M111：事故详情只读 Runbook 关联

- Date: 2026-08-14
- Status: Complete
- Scope: 事故详情复用 M81 Insight 展示可信的诊断、巡检、AI 解释与 dry-run 候选，并对不完整来源 fail-closed。

## Context

M111 已完成事故响应 KPI 基础层。下一块需要把事故从来源证据继续连接到现有 Runbook，但事故模型本身不持久化
Insight 的故障域，不能从人工来源或不完整来源猜测 domain。本次改动建立来源解析元数据契约，并将只读关联挂到
事故详情；所有实际执行继续复用既有诊断、AI 和受控 remediation 路径。

## What Changed

### 后端

- `backend/internal/incident/service.go`：`SourceInfo` 增加可信 `Domain` / `FindingCode` 元数据。
- `backend/cmd/server/incident_resolver.go`：为诊断、告警、巡检、信号和关联案例提供来源域/代码；关联案例要求所有已知信号域一致，诊断和告警增加集群归属校验。
- `backend/internal/httpserver/incidents.go`：新增 `GET /api/v1/incidents/{incident_id}/runbook`，解析失败或域不明确时返回 `available=false`，不返回猜测的 Runbook。
- `backend/internal/httpserver/router.go`、`backend/cmd/server/main.go`：注入来源解析器并登记只读审计动作 `incident.runbook.get`。
- `backend/internal/httpserver/incidents_test.go`、`backend/cmd/server/incident_resolver_test.go`：覆盖只读响应、fail-closed、来源域元数据与跨集群拒绝。

### 前端与契约

- `frontend/src/api/incidents.ts`、`frontend/src/types/incident.ts`、`frontend/src/api/incidents.test.ts`：增加 Runbook API 类型、封装和 2 个客户端测试。
- `frontend/src/views/IncidentsView.vue`：事故详情抽屉新增诊断入口、巡检校验、AI 解释入口与 dry-run 候选的只读展示，并保留不可用原因。
- `docs/api/openapi.yaml`、`frontend/src/api/openapi.d.ts`：登记 `incidentRunbook` 与 `IncidentRunbookResponse`。
- `docs/security/permission-matrix.md`：登记只读路由和审计动作。
- `docs/development-roadmap-post-m110.md`、`CHANGELOG.md`、`docs/README.md`：同步 M111 进度与归档索引。

## Verification

- `cd backend && go test ./internal/httpserver/... ./cmd/server/... ./internal/incident/...`：通过。
- `cd frontend && pnpm typegen`：通过。
- `cd frontend && pnpm typecheck && pnpm lint && pnpm test -- --run`：通过，27 files / 145 tests。
- `cd frontend && pnpm build`：通过；`pnpm bundle:gate`：通过；事故页独立 axe（桌面/移动）：0 critical/serious、0 app errors。
- `TestPermissionMatrixMatchesCommittedDocument`：通过并刷新权限矩阵。
- `pnpm ui:gate`：CSS、事故页截图与其他视图检查通过；综合门禁在既有登录桌面基线处以 `0.225%` 差异中止，登录页不属于本次改动，未改动其基线。

## Risks / Notes

- 人工上报来源没有持久化故障域，当前会明确展示不可用；后续若引入模板域字段，必须同步迁移、OpenAPI、校验和归档。
- Runbook 只展示现有 M81 映射；没有新增集群写操作，也不绕过 remediation 的预览、确认、幂等和审计流程。
- 本次不打 `baseline-m111-*`，M111 的升级链、复盘导出和完整旅程验收尚未完成。
