# M111：事故 SLA 有界升级链

- Date: 2026-08-14
- Status: Complete
- Scope: 为未确认/未解决事故补齐两级 SLA 升级、幂等投递和可查询审计记录。

## Context

M107 已将事故 SLA 临近与逾期状态写入通知 outbox，但事故在逾期后持续未确认或未解决时没有后续升级，值班团队也无法按升级级别筛选投递历史。本改动继续复用现有 signed webhook outbox，固定为两个升级级别，避免无限制通知风暴。

## What Changed

### 后端
- `backend/internal/incident/sla_monitor.go`：新增 `incident.sla_escalated`，按逾期 30 分钟、2 小时两个有界窗口扫描 level 1/2，限定 `open` / `confirmed` 状态，payload 记录升级阶段和原因。
- `backend/internal/incident/repository.go`：SLA 候选按事件类型和升级级别检查幂等记录。
- `backend/internal/notification/model.go`、`repository.go`：投递模型增加 `escalation_level`，支持升级级别过滤；Claim/List 返回事故归属和升级级别。
- `backend/cmd/server/main.go`、`sla_enqueuer.go`、`backend/internal/config/config.go`：接线并增加首次/最终升级延迟配置。

### 数据库与契约
- `backend/migrations/000047_incident_sla_escalation.(up|down).sql`：增加 level 约束，替换事故通知唯一键为 `(incident_id, event_type, escalation_level)`。
- `backend/internal/httpserver/notification.go`、`docs/api/openapi.yaml`：新增升级事件和 `escalation_level` 审计筛选。
- `.env.example`、`.env`、`compose.yaml`、Helm/Kubernetes 配置及 `docs/development.md`：同步升级默认值和部署参数。

### 前端
- `frontend/src/types/notification.ts`、`frontend/src/api/notifications.ts`、`frontend/src/views/NotificationDeliveriesView.vue`：支持升级事件、级别筛选和 level 标识展示。
- `frontend/src/api/openapi.d.ts`：由 OpenAPI typegen 刷新。

## Verification

- 后端：`gofmt`、`go test ./internal/incident ./internal/notification ./internal/httpserver ./internal/config`。
- 前端：`pnpm typegen`、`pnpm typecheck`、`pnpm lint`、`pnpm test -- --run`、`pnpm build`。
- 契约：OpenAPI 与注册路由、权限矩阵门禁通过；迁移 `000047` up/down 往返在本地 PostgreSQL 验证。

## Risks / Notes

- 升级级别固定为 0/1/2；level 0 为既有临近/逾期提醒，level 1/2 为首次/最终升级，后续如需更多级别必须新增契约和迁移。
- 升级仍依赖通知开关；通知关闭时不扫描、不写队列。通知记录是投递审计，事故状态恢复为 resolved/dismissed 后不会继续产生升级。
