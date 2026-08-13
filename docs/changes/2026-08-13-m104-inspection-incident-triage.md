# M104：巡检结果 → 事故工作空间联动（Inspection-to-Incident Triage）

- Date: 2026-08-13
- Status: Complete
- Scope: 把巡检（inspection）结果一键提升为事故工作空间，补齐「巡检 → 事故 → 复盘」闭环；后端巡检来源富集与集群防泄漏、REST/OpenAPI、前端巡检视图入口、事故视图来源徽标，并扩展确定性演示演练证据。

## Context

M103 已把触发中的告警实例连入事故工作空间；巡检（M52，KubeEye 风格编译期规则目录）是另一条持续检测信号线，此前 findings 只能在巡检视图被查看，无法直接进入事故复盘流程。M104 沿用 M103 的 `incidentResolver` 模式新增第 4 个来源 `inspection`：`source_ref` 格式 `inspection:<resultID>`，复用严重级富集，强制来源去重，并对来源做集群作用域防泄漏校验（要求结果 `ClusterID` 与调用方一致）。

## What Changed

### Backend：巡检来源富集 + 集群防泄漏

- `backend/internal/incident/model.go`：新增 `SourceTypeInspection = "inspection"` 与 `SourceRefForInspection(resultID)` 稳定去重身份。
- `backend/internal/incident/service.go`：`Create` 接受 `inspection` 来源。
- `backend/cmd/server/incident_resolver.go`：
  - 新增 `inspectionResultReader` 接口与 `inspectionServiceAdapter`（包装 `*inspection.Service.GetResult`），使 resolver 可单测。
  - `incidentResolver` 增加 `inspections` 字段，`NewIncidentResolver(records, alerts, inspections)` 签名扩展。
  - 新增 `resolveInspection`：前缀校验、`ErrResultNotFound` → `ErrInvalidSource`、cluster 防泄漏（结果 `ClusterID` 必须等于调用方）、`normalizeIncidentSeverity` 严重级映射（critical→critical、warning→warning、其余→info，巡检无 high）。
  - 更新文档注释（诊断/告警/巡检/finding 四类来源）。
- `backend/cmd/server/main.go`：把 M52 inspection 装配块移至 alertService 之后、incidentService 之前（依赖 clusterService/kubernetesService/logger），更新 `NewIncidentResolver(diagnosisRepo, alertService, inspectionService)`。
- `backend/migrations/000043_incident_inspection_source.up.sql`（+ down）：放开 `incidents.source_type` CHECK 增加 `'inspection'`（000042 已加 alert）。
- `docs/api/openapi.yaml`：`IncidentCreateRequest.source_type` enum 增加 `inspection`，source_ref 注释补充 `inspection:<id>`。

### 测试

- `backend/internal/incident/model_test.go`：`SourceRefForInspection`。
- `backend/internal/incident/service_test.go`：`TestCreateFromInspection`（富集 + 重复去重）。
- `backend/cmd/server/incident_resolver_test.go`：`TestResolveInspection`、`TestResolveInspectionRejectsForeignCluster`（集群防泄漏）、`TestResolveInspectionInvalidOrMissing`（非法/缺失结果）。

### Frontend

- `src/types/incident.ts`：`IncidentSourceType` 增加 `'inspection'`。
- `src/views/InspectionView.vue`：巡检结果行新增「创建事故工作区」按钮（`FilePlus2`，`operations_admin`/`system_admin` 且结果未 resolved 时可用），调用 `createIncident(source_type:'inspection', source_ref:'inspection:<id>')`，处理 `SOURCE_ALREADY_USED`，顶部业务提示（新增全局 `.ok-message` 成功样式）。
- `src/views/IncidentsView.vue`：创建表单来源类型增加「巡检结果」，标题/严重级/摘要自动填充禁用，`sourceRefPlaceholder` 与详情友好来源标签同步（诊断记录/人工上报/告警实例/巡检结果）。
- `src/api/openapi.d.ts`：`pnpm typegen` 重新生成，`source_type` union 含 `inspection`。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：新增第 11 节「Inspection → incident」6 条断言：`POST /aiops/inspection/run`（`node_not_ready`，demo-node 恒 NotReady 必命中）→ 轮询任务至 completed → 按 `task_id`+`cluster_id` 取结果 → 提升事故 → 严重级从结果富集为 critical → 重复提升被 `SOURCE_ALREADY_USED` 拒绝。

## Verification

- `cd backend && go build ./...`：通过。
- `cd backend && go vet ./...`：通过；`golangci-lint run ./internal/incident/... ./cmd/server/... ./internal/httpserver/...`：0 issues。
- `cd backend && go test ./...`：全绿。
- `cd frontend && pnpm typecheck && pnpm lint && pnpm test`：typecheck/lint 干净，26 files / 141 tests 通过。
- `cd frontend && pnpm build`：成功。
- `./scripts/demo-drill.sh`：**28/28 PASS**（原 22/22 + 6 条 inspection→incident），报告 `.artifacts/demo-drill/report-20260813-083810-7d4d9f.json`（artifacts 已被 gitignore，本地复验用）。

## Risks / Notes

- 巡检→事故富集要求结果存在且 `ClusterID` 与调用方一致；不满足时 resolver 返回 `ErrInvalidSource` 并回退到调用方字段（需 title/severity/resource）。
- 巡检严重级无 high：映射 critical→critical、warning→warning、其余→info，与巡检目录语义一致。
- 新增枚举/来源不影响既有 diagnosis/finding/alert 事故；迁移只放开 CHECK，不破坏存量数据。
- `k8s-aiops-backend:latest` 本地镜像已按 arm64 交叉编译重建以承载新代码（`.artifacts` 不入库）。
