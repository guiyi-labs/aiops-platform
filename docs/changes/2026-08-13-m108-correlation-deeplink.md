# M108：关联归一 Block 2 — 案例 ↔ 事故双向深链与关联事故回显

- Date: 2026-08-13
- Status: Complete
- Scope: 在 Block 1（correlation → incident 第 6 来源）之上补齐 M108 路线图的双向深链与「相关 case 在 incident 详情提示关联案例入口」：case view 回显关联事故、correlation 证据深链精确聚焦案例、前端深链定位 + 关联事故入口。

## Context

Block 1 已交付 correlation 案例一键提升为事故（含 `SOURCE_ALREADY_USED` 去重）。路线图 M108 剩余「双向深链」与「incident 详情关联案例入口」：事故详情证据卡的深链只落到案例列表，无法定位具体 case；关联案例视图也无法回看已提升的事故。Block 2 把两端打通。

## What Changed

### Backend：incident 反查 + case view 富集

- `backend/internal/incident/repository.go`：`Repository` 接口新增 `FindBySource(ctx, sourceType, sourceRef)`；`GormRepository` 实现（复用 `incidentSelect`，`(source_type, source_ref)` 唯一约束保证至多一行，无记录返回 `ErrNotFound`）。
- `backend/internal/incident/service.go`：新增 `Service.FindBySource`（只读富集用）。
- `backend/internal/httpserver/correlation.go`：`correlationHandler` 新增可选 `incidentBySource` 钩子；`getCorrelationCase` 把案例视图富集为 `correlationCaseViewResponse`（嵌入 `CaseView` + 可选 `incident{id,number,title,status}`，M108 双向深链）。缺失/未配置时视图原样返回，永不因富集失败 5xx。
- `backend/internal/httpserver/router.go`：`CorrelationService` 与 `Incidents` 同时可用时注入 `incidentBySource`（`SourceRefForCorrelation` + `FindBySource`）。

### Backend：correlation 证据深链精确聚焦

- `backend/internal/incident/evidence.go`：`IncidentDeepLink(sourceType, sourceRef)` 签名扩展；correlation 源返回 `/aiops/correlation?case_id=<id>`（从 `correlation:<id>` 解析），其余来源行为不变。
- `backend/cmd/server/incident_resolver.go`：`ResolveEvidence` 深链调用同步传 `sourceRef`。

### OpenAPI / 前端

- `docs/api/openapi.yaml`：`CorrelationCaseView` 增加可选 `incident` 对象（id/number/title/status）；typegen 重新生成。
- `frontend/src/types/aiops.ts`：`CaseView` 增加 `incident?`。
- `frontend/src/views/CorrelationCasesView.vue`：
  - 深链聚焦：`initialize()` 读取 `route.query.case_id`，直接拉取该案例 + 动作候选，自动选中所属集群并展开详情（M108 深链目标端）。
  - 关联事故入口：案例详情徽章行显示「已关联事故 INC-xxxx ↗」链接到 `/incidents`；提升成功后立即把返回的 incident 挂到 case view，无需刷新。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：第 13 节新增 `correlation-incident-deeplink` 断言——提升事故后 `GET /aiops/correlation/cases/<id>` 返回的 `.incident.id` 必须等于事故 ID（验证双向深链富集端到端）。

## Verification

- `cd backend && gofmt -l`（改动包）：干净；`go vet ./internal/incident/ ./internal/httpserver/ ./cmd/server/` 通过。
- `cd backend && go test ./... -short`：全部包通过；新增单测 `TestIncidentDeepLink_CorrelationFocused`、`TestService_FindBySource`、`TestCorrelationHandler_CaseViewIncludesLinkedIncident` / `...OmitsIncident` 全过。
- `cd frontend && pnpm typegen && pnpm typecheck && pnpm lint && pnpm test -- --run`：typecheck/lint 干净，26 files / 141 tests 通过。
- `cd frontend && pnpm build`：成功。
- `./scripts/scan-sensitive-fields.sh`：clean（1250 tracked files）。
- demo-drill 本地复验需重建 `k8s-aiops-backend:latest`（含富集逻辑），报告 `.artifacts/demo-drill/report-<run>.json`（不入库）。

## Risks / Notes

- 富集是只读 best-effort：`incidentBySource` 错误（含未找到）不阻断案例视图，保持现状。
- 深链沿用 M94 模块列表落地模式：案例 → `/incidents` 列表（IncidentsView 暂不支持 `?incident_id=` 聚焦，与告警/巡检一致）；事故 → 案例精确聚焦（`?case_id=` 为新增能力）。
- 无新路由/权限变更，不触及 permission matrix；OpenAPI 仅 schema 增量。
