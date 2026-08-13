# M107：事故 SLA 提醒接入通知（Incident SLA Notifications）

- Date: 2026-08-13
- Status: Complete
- Scope: incident 的 `Overdue` 状态此前仅在 UI 展示，未接入通知。本改动复用现有 diagnosis
  notification outbox，新增 incident SLA monitor（临近/逾期）在后台将提醒事件原子写入投递队列，
  并打通 Webhook 视图的事件类型/incident_id 过滤与前端 SLA 状态展示。

## Context

M98 的 incident 已具备 `sla_due_at` / `overdue`（SQL 计算），M21+ 已具备 diagnosis 通知
outbox（`notification_deliveries` + webhook 投递 + 重试）。但 outbox 只支持 `diagnosis_id`
且事件类型固定为 diagnosis 三态；incident 的 SLA 到期/临近没有任何通知触发，值班团队只能
在 incident 列表内人工盯守。本块打通「SLA → 通知」，让临近 15min 与逾期成为 webhook 事件。

## What Changed

### 数据库
- `backend/migrations/000045_incident_sla_notifications.(up|down).sql`（新增）：`notification_deliveries`
  增加可空 `incident_id` 外键；`diagnosis_id` 改为可空；部分唯一索引
  `(incident_id, event_type) WHERE incident_id IS NOT NULL` 保证每个 incident 每种 SLA
  事件至多一条（SLA monitor 幂等重跑）；`incident_id` 查询索引。down 迁移可回滚并把空行恢复 NOT NULL。
- 已在本地 Postgres 实测 up/down 往返通过。

### 后端
- `backend/internal/notification/model.go`：`Delivery` 增加 `incident_id`；
  `EnqueueInput`（诊断或事故二选一）；事件类型常量
  `incident.sla_approaching` / `incident.sla_breached`；`ListFilter` 增加 `IncidentID`。
- `backend/internal/notification/repository.go`：新增 `Enqueue`（insert + 部分唯一索引
  `ON CONFLICT DO NOTHING`，天然幂等）；`List` 支持 `incident_id` 过滤；
  `storedDelivery` 支持可空 incident_id。
- `backend/internal/notification/service.go`：暴露 `Enqueue`。
- `backend/internal/incident/sla_monitor.go`（新增）：`SLAMonitor` 后台任务，按轮询周期
  先扫逾期（`sla_due_at < now`）再扫临近（`[now, now+window]`），
  每个事件类型经幂等入队；payload 含事故号/标题/严重度/截止/深链。
- `backend/internal/incident/repository.go`：新增 `ListSLAEligible`（仅 open/confirmed +
  未发过对应事件类型，防风暴）。
- `backend/cmd/server/sla_enqueuer.go`（新增）：adapter 把 notification service 适配为
  `incident.SLAEnqueuer`。
- `backend/cmd/server/main.go`：接线 `incidentSLAMonitor`（在 `NotificationEnabled` 时运行）。
- `backend/internal/config/config.go`：新增
  `INCIDENT_SLA_MONITOR_ENABLED` / `INCIDENT_SLA_POLL_INTERVAL` /
  `INCIDENT_SLA_APPROACHING_WINDOW` / `INCIDENT_SLA_BATCH_SIZE` 配置。

### 契约
- `backend/internal/httpserver/notification.go`：`list` handler 支持 `incident_id` 过滤与
  两类 SLA 事件类型。
- `docs/api/openapi.yaml`：`notification-deliveries` 增加 `incident_id` 参数与两类
  event_type；`frontend/src/api/openapi.d.ts` typegen 重新生成。

### 前端
- `frontend/src/types/notification.ts`：`NotificationDelivery` 增加 `incident_id`；
  事件枚举纳入两类 SLA 事件。
- `frontend/src/api/notifications.ts`：`incidentID` 过滤参数。
- `frontend/src/views/NotificationDeliveriesView.vue`：事故 ID 过滤、SLA 事件类型选项、
  投递记录区分事故/诊断引用。
- `frontend/src/views/IncidentsView.vue`：SLA 徽标新增 `approaching` 色调（临近 15min 高亮），
  逾期/临近一目了然。

## Verification

- 后端：`go build ./...`、`go vet ./...`、`go test ./... -short` 全绿；受影响包
  （notification / incident / httpserver / config）全量单测通过（含新增
  `SLAMonitor`、`Service.Enqueue`、handler `incident_id`/非法事件类型用例）。
- 契约：`TestRegisteredRoutesMatchOpenAPI`、`TestPermissionMatrixMatchesCommittedDocument` 通过。
- 前端：`pnpm typecheck`、`pnpm lint`、`pnpm test`（26 files / 141 tests）、`pnpm build` 全绿。
- 敏感扫描：`scripts/scan-sensitive-fields.sh` clean（1240 tracked files）。
- 迁移：本地 Postgres 实测 `000045` up / down / 再 up 往返无错，索引与外键生效。

## Notes

- SLA monitor 与 diagnosis 触发器共用 outbox；incident 事件不依赖 diagnosis 存在
  （finding/alert/inspection/signal 源事故也可提醒）。
- 幂等由部分唯一索引保证；`remaining`/总数仍按各自过滤维度统计。
