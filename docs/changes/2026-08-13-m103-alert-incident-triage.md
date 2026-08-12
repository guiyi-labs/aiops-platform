# M103：告警 → 事故工作空间联动（Alert-to-Incident Triage）

- Date: 2026-08-13
- Status: Complete
- Scope: 把触发中的告警实例一键提升为事故工作空间，补齐「告警 → 事故 → 复盘」闭环；后端告警来源富集、REST/OpenAPI、前端告警视图入口、事故视图来源徽标，并扩展确定性演示演练证据。

## Context

本地路线 M1–M102 已收口（M98 事故工作空间覆盖 diagnosis/finding 两类来源；M99 信号关联；M101 本地数据轨；M102 双环境/离线安装演练）。按「功能开发优先级高」推进新修复 M103：告警（alert instance）是持续运行的检测信号，此前无法直接进入事故复盘流程，只能在告警关联的诊断间手动跳转。M103 把 firing 告警实例连入同一套事故工作空间（编号/负责人/关注者/状态机/复盘），复用诊断富集，并强制来源去重。

## What Changed

### Backend：告警来源富集 + 事故来源扩展

- `backend/internal/incident/model.go`：新增 `SourceTypeAlert = "alert"` 与 `SourceRefForAlert(alertID)` 稳定去重身份。
- `backend/internal/incident/service.go`：`SourceResolver.Resolve` 增加 `clusterID` 参数；`Create` 接受 `alert` 来源。
- `backend/cmd/server/incident_resolver.go`：重构为 `incidentResolver`（接口化，可单测），诊断来源沿用 diagnosis 富集，新增告警来源——查告警实例 → 取关联诊断富集严重级/资源/摘要/首触时间，标题加 `Alert ` 前缀。
- `backend/cmd/server/main.go`：组装 `NewIncidentResolver(diagnosisRepo, alertService)`，复用 alert service 的集群作用域校验。
- `backend/migrations/000042_incident_alert_source.up.sql`（+ down）：放开 `incidents.source_type` CHECK 增加 `'alert'`。
- `backend/internal/httpserver/incidents.go`：`source_type` 校验允许 `alert`；冲突提示改为通用「this source already has an incident workspace」。
- `docs/api/openapi.yaml`：`IncidentCreateRequest.source_type` enum 增加 `alert`，source_ref 注释补充 `alert:<id>`。

### 测试

- `backend/internal/incident/service_test.go`：`TestCreateFromAlert`（富集 + 重复去重）。
- `backend/cmd/server/incident_resolver_test.go`：告警解析、非法 source_ref、未知实例、RuleNotFound→ErrInvalidSource、未知来源类型。
- `backend/internal/incident/model_test.go`：`SourceRefForAlert`。
- `backend/internal/httpserver`：既有 incidents 测试通过。

### Frontend

- `src/types/incident.ts`：`IncidentSourceType` 增加 `'alert'`。
- `src/views/AlertsView.vue`：触发中告警实例行新增「创建事故工作区」按钮（`FilePlus2`），调用 `createIncident(source_type:'alert', source_ref:'alert:<id>')`，处理 `SOURCE_ALREADY_USED` + 仅 firing 可提升；顶部 success 提示。
- `src/views/IncidentsView.vue`：创建表单来源类型增加「告警实例」，标题/严重级/摘要自动填充禁用，`sourceRefPlaceholder` computed，详情来源友好标签（诊断记录/人工上报/告警实例）。
- `src/api/openapi.d.ts`：`pnpm typegen` 重新生成，`source_type` union 含 `alert`。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：后端启用 `ALERT_ENABLED=true` + 收紧采集/轮询间隔（确定性）；新增第 10 节「Alert → incident」5 条断言：创建 CPU 规则（demo-node 恒 3500m > 2 核必触发）→ 等待 firing 实例 → 从告警提升事故 → 严重级从关联诊断富集为 high → 重复提升被 `SOURCE_ALREADY_USED` 拒绝。

## Verification

- `cd backend && go build ./...`：通过。
- `cd backend && go vet ./...`：通过；`golangci-lint run ./internal/incident/... ./cmd/server/... ./internal/httpserver/...`：0 issues。
- `cd backend && go test ./...`：全绿。
- `cd frontend && pnpm typegen && pnpm typecheck && pnpm lint && pnpm test`：typecheck/lint 干净，26 files / 141 tests 通过。
- `cd frontend && pnpm build`：成功。
- `./scripts/demo-drill.sh`：**22/22 PASS**（原 17/17 + 5 条 alert→incident），报告 `.artifacts/demo-drill/report-20260813-021735-26479c.json`（artifacts 已被 gitignore，本地复验用）。

## Risks / Notes

- 告警→事故富集依赖告警实例关联的诊断（`CreateFiring` 在首次触发时创建）；若未来有告警实例无诊断，resolver 返回 `ErrInvalidSource` 并回退到调用方字段（需 title/severity/resource）。
- CPU 指标 `metricBreachSeverity` 为 `high`（非 critical），与告警语义一致，显示层已按此断言。
- 新增枚举/来源不影响既有 diagnosis/finding 事故；迁移只放开 CHECK，不破坏存量数据。
- `k8s-aiops-backend:latest` 本地镜像已按 arm64 交叉编译重建以承载新代码（`.artifacts` 不入库）。
