# M111 事故响应 KPI 基础层：MTTA / MTTR / SLA

- Date: 2026-08-14
- Status: Complete
- Scope: 为事故响应深化提供真实时间戳派生的只读 KPI API 与前端客户端契约

## Context

M111 的目标是把事故工作空间从“能协作”推进到“可运营、可度量、可交接”。现有事故已经
保存创建时间、SLA 截止时间、解决时间和 append-only 生命周期时间线，适合先建立不新增
数据库字段的 KPI 基础层。

## What Changed

### Backend

- `backend/internal/incident/metrics.go`：新增事故 KPI 派生逻辑和服务方法。
- `backend/internal/httpserver/incidents.go`：新增只读接口
  `GET /api/v1/incidents/metrics`，支持 `days=1..90` 和可选 `cluster_id`。
- KPI 口径：
  - `first_assigned_seconds`：创建到首次 handoff 时间。
  - `mtta_seconds`：创建到首次 `open -> confirmed` 时间。
  - `mttr_seconds`：创建到 `resolved_at` 时间。
  - SLA 达标率仅对已解决事故评估，按 `resolved_at <= sla_due_at` 计算。
  - 缺少有效生命周期样本时返回 `null`，不把无数据误报成 0。
- 每次最多统计最近 200 条事故，响应通过 `sample_limit`、`sampled`、`truncated` 明确披露样本边界。

### Contract / Frontend

- `docs/api/openapi.yaml`：登记 `incidentMetrics` operation 和 `IncidentMetrics` schema。
- `frontend/src/api/openapi.d.ts`：重新生成 OpenAPI 类型。
- `frontend/src/api/incidents.ts`、`frontend/src/types/incident.ts`：新增
  `getIncidentMetrics` 和客户端类型，供后续 KPI 视图直接消费。
- `docs/security/permission-matrix.md`：同步新增只读路由。

## Verification

- `go test ./... -count=1`：通过。
- `golangci-lint run --config ../.golangci.yml ./...`：`0 issues.`
- `pnpm typecheck`：通过。
- `pnpm lint`：通过。
- `pnpm test -- --run`：143 tests passed。
- `pnpm build`：通过。
- `pnpm ui:gate`：`PASS: 4/4`，62 条截图基线、32 视图 axe、bundle 全绿。

## Risks / Notes

- 这是 M111 的 KPI 基础层，不代表 M111 完成；事故 KPI 视图、runbook 关联、升级链和复盘导出仍待后续切片。
- 指标基于当前最多 200 条列表样本；`truncated=true` 时消费方应展示样本边界，不应宣称全量统计。
- 未新增迁移或改变既有写路径，旧事故记录可安全返回空样本指标。
