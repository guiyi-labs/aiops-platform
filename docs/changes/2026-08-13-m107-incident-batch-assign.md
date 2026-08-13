# M107：事故批量指派（Incident Batch Assignment）

- Date: 2026-08-13
- Status: Complete
- Scope: incident 工作空间新增批量指派能力——一次 API 调用将多个事故移交给同一负责人，
  前端表格支持多选 + 批量操作工具栏；成功/失败聚合结果可直接反馈操作人，每个事故的
  交接事件和审计条目自动生成。

## Context

M98 的 incident 已具备单条移交（`PATCH /assignment`）与关注者增删，但无跨事故批量操作，
交接时需逐条点击，不符合「10 秒可交接」目标。本块补充批量指派端到端路径：后端 partial-success
语义（部分失败不中断其他事故）、前端多选 + 工具栏、OpenAPI/typegen/权限矩阵同步，
本地 Postgres + 路由 E2E 均已验证。

## What Changed

### 后端
- `backend/internal/incident/service.go`：新增 `BatchAssignInput` / `BatchAssignResult` /
  `AssignFailure`；`Service.BatchAssign` 按 id 列表逐条 Get→Assign（CAS 版本校验由 repository
  事务保证），跳过重复 id，最大 50 条/请求，失败项按 `ErrNotFound` / `ErrVersionConflict` /
  `ErrAssigneeNotFound` 映射为结构化错误码，不影响其余 id 移交。
- `backend/internal/incident/model.go`：新增 `ErrBatchEmpty` / `ErrBatchTooLarge`、
  `MaxBatchAssignSize = 50`。
- `backend/internal/httpserver/incidents.go`：`batchAssign` handler，200 返回聚合结果
  （`assigned/total/failed`），400 校验参数上限，审计 target `batch:<count>`。
- `backend/internal/httpserver/router.go`：新增路由 `POST /api/v1/incidents/batch-assign`
  （`incident.assignment.batch`）。

### 契约
- `docs/api/openapi.yaml`：新增 `/api/v1/incidents/batch-assign` + schema
  `IncidentBatchAssignRequest` / `IncidentBatchAssignResult` / `IncidentAssignFailure`。
- `frontend/src/api/openapi.d.ts`：typegen 重新生成。
- `docs/security/permission-matrix.md`：权限矩阵重算（282 条路由，batch-assign 需
  `operations_admin` / `system_admin`，审计 `incident.assignment.batch`）。

### 前端
- `frontend/src/types/incident.ts`：新增 `IncidentBatchAssignResult` / `IncidentAssignFailure`。
- `frontend/src/api/incidents.ts`：新增 `batchAssignIncidents`。
- `frontend/src/views/IncidentsView.vue`：
  - 表格新增复选框列（含表头全选）；选中行背景高亮。
  - 统计卡片下方新增批量工具栏：显示已选数量、负责人下拉、说明输入、提交、取消选择。
  - 操作结果支持成功/部分失败消息展示。

### 演示
- `scripts/demo-drill.sh`：incident-journey 新增 `incident-batch-assign` 断言——
  对已创建事故调用 batch-assign，校验 assigned ≥ 1。

## Verification

- 后端：`go vet ./...`、`go test ./... -short`、受影响包（incident/httpserver）
  全量测试新增 `TestBatchAssign*` / `TestIncidentHandler_BatchAssign*` 通过。
- 契约：`TestRegisteredRoutesMatchOpenAPI`、`TestPermissionMatrixMatchesCommittedDocument`
  通过；本地离线镜像 E2E 验证路由返回符合 200+partial failure / 400+INVALID_REQUEST。
- 前端：`pnpm typecheck`、`pnpm lint`、`pnpm test`（26 files / 141 tests）、
  `pnpm build` 全绿。
- 敏感扫描：`scripts/scan-sensitive-fields.sh` clean（1245 tracked files）。
