# M108：关联归一 Block 1 — Correlation 案例 → 事故工作空间（第 6 来源）

- Date: 2026-08-13
- Status: Complete
- Scope: 把关联引擎（M43+）产出的 correlation case 接入事故工作空间，作为第 6 类事故来源：resolver 解析/防泄漏/严重级富集、证据时间线深链、前端一键提升、demo-drill 断言。

## Context

M103/M104/M105 已把告警、巡检、信号连入事故工作空间；事故来源覆盖 diagnosis/finding/alert/inspection/signal。M108 目标是把关联案例（多源归一）也纳入事故闭环：关联案例本身是「跨信号归因」的成果，应能一键提升为事故工作空间，并保持同源去重语义（一个案例最多一个事故）。本 Block 1 交付核心链路（correlation 源 + 提升 + 去重）。

## What Changed

### Backend：事故第 6 来源 `correlation`

- `backend/migrations/000046_incident_correlation_source.up.sql`（+ down）：`incidents.source_type` CHECK 约束加入 `'correlation'`。
- `backend/internal/incident/model.go`：新增 `SourceTypeCorrelation = "correlation"` 与 `SourceRefForCorrelation(caseID)`（`correlation:<id>` 稳定去重身份）。
- `backend/internal/incident/service.go`：`Create` 白名单接受 `correlation` 来源。
- `backend/internal/incident/evidence.go`：`IncidentDeepLink` correlation 源 → `/aiops/correlation`。
- `backend/cmd/server/incident_resolver.go`：新增 `correlationCaseReader` 接口 + `correlationServiceAdapter`（包装 `*correlation.Service.GetCase`）；`resolveCorrelation`：前缀/ID 校验、`ErrCaseNotFound` → `ErrInvalidSource`、跨集群防泄漏（`Case.ClusterID != clusterID → ErrInvalidSource`）、severity 按置信度富集（confirmed→high / candidate→warning / 其余 info）、title/summary 带 case_key/rule_id/信号数、资源/首次观测时间取自案例；`NewIncidentResolver(..., cases *correlation.Service)` 第 5 参。
- `backend/cmd/server/main.go`：correlation provider/service/worker 装配移动到 incident resolver 之前，并把 service 传入 resolver。
- `backend/cmd/server/incident_resolver_test.go`：`fakeCorrelationCaseReader` + 4 个测试（解析/跨集群防泄漏/invalid refs/case 不存在）。

### OpenAPI / 前端

- `docs/api/openapi.yaml`：`IncidentCreateRequest.source_type` 与 `IncidentEvidenceItem.source_type` enum 增加 `correlation`，source_ref 注释补 `correlation:<id>`。
- `frontend/src/api/openapi.d.ts`：`pnpm typegen` 重新生成。
- `frontend/src/types/incident.ts`：`IncidentSourceType` 增加 `'correlation'`。
- `frontend/src/views/IncidentsView.vue`：`sourceTypeLabel` / `evidenceSourceLabel` 增加「关联案例」。
- `frontend/src/views/CorrelationCasesView.vue`：案例详情头部新增「提升事故」按钮（`createIncident(source_type:'correlation', source_ref:'correlation:<id>')`），处理 `SOURCE_ALREADY_USED` 去重提示，成功/失败分别提示。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：isolated compose 后端 env 加 `CORRELATION_INTERVAL=30s`（确定性出案例）；新增第 13 节「Correlation case → incident」4 条断言：轮询 `GET /api/v1/aiops/correlation/cases` 取 active 案例 → 从案例提升事故 → source_type=correlation 且 severity 富集 → 重复提升被 `SOURCE_ALREADY_USED` 拒绝；报告 `evidence` 增加 `correlation_incident`，并顺手修复 evidence 块缺失的逗号（原 JSON 非法）。

## Verification

- `cd backend && gofmt -l`（改动文件）：干净；`go vet ./cmd/server/` 通过。
- `cd backend && go test ./... -short`：全部包通过（含 `internal/incident`、`internal/httpserver` 契约测试）。
- `cd frontend && pnpm typegen && pnpm typecheck && pnpm lint && pnpm test -- --run`：typecheck/lint 干净，26 files / 141 tests 通过。
- `cd frontend && pnpm build`：成功。
- `./scripts/scan-sensitive-fields.sh`：clean（1247 tracked files）。
- demo-drill 本地复验需要重建 `k8s-aiops-backend:latest` 镜像（迁移 000046 + resolver），报告 `.artifacts/demo-drill/report-<run>.json`（artifacts 不入库）。

## Risks / Notes

- correlation 案例在 demo 中走冷启动路径（无 change event 时 ConfidenceUnknown → severity=info）；断言只验证 severity 非空与 source_type=correlation，不锁死具体严重级。
- `CORRELATION_INTERVAL=30s` 仅写入 demo-drill 的 isolated compose，不影响生产/开发 compose 默认 5m。
- 新增枚举/来源不影响既有五类来源事故；迁移只放开 CHECK，不破坏存量数据。
- M108 后续 Block（可选）：incident 详情「关联案例」入口、风暴去重演练强化、双向深链齐全。
