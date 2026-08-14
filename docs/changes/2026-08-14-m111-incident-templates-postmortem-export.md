# M111：事故响应模板、严重级矩阵与复盘 Markdown 导出

- Date: 2026-08-14
- Status: Complete
- Scope: 为事故创建和复盘交付补齐模板目录、可配置 SLA 目标和可携带 Markdown 导出。

## Context

M111 已完成事故 KPI、Runbook 关联和 SLA 升级链，但人工创建事故仍需要重复填写默认信息，SLA 目标也固定在代码中；已有复盘视图只能在控制台内查看。本次完成 M111 剩余的模板/严重级矩阵与复盘导出模块，保持来源解析、通知升级和受控动作边界不变。

## What Changed

### 事故模板与严重级矩阵

- `backend/internal/incident/response.go`：新增版本化内置模板目录和 `severity -> target_minutes` 矩阵，支持模板按来源类型校验、默认值填充和 SLA 截止时间计算。
- `backend/internal/incident/model.go`、`service.go`、`repository.go`：事故持久化 `template_id`，创建时应用模板默认值；CSV 快照同步导出模板标识。
- `backend/internal/config/config.go`、`.env.example`、`compose.yaml`、`deploy/kubernetes/configmap.yaml`、`deploy/helm/aiops-platform/values.yaml`：新增 `INCIDENT_SLA_TARGETS` 配置，默认值为 critical/high/warning/info=`1h/4h/24h/72h`。
- `backend/migrations/000048_incident_response_templates.(up|down).sql`：增加模板标识字段和条件索引。
- `backend/internal/httpserver/incidents.go`、`router.go`、`docs/api/openapi.yaml`：新增模板目录 API 和创建请求 `template_id`。
- `frontend/src/api/incidents.ts`、`frontend/src/types/incident.ts`、`frontend/src/views/IncidentsView.vue`：事故创建表单支持模板选择并展示当前严重级目标；切换模板时同步刷新模板标题、摘要与严重级默认值。

### 复盘 Markdown 导出

- `backend/internal/incident/markdown.go`、`service.go`：输出事故叙事、证据时间线、决策/动作时间线和结果指标，来源证据继续复用现有 fail-closed 解析器。
- `backend/internal/httpserver/incidents.go`、`router.go`、`docs/api/openapi.yaml`：新增 `GET /api/v1/incidents/{incident_id}/postmortem/export`，返回 `text/markdown` 并记录审计动作。
- `frontend/src/api/incidents.ts`、`frontend/src/views/IncidentsView.vue`、`frontend/e2e/incidents.spec.ts`：详情页提供 Markdown 下载入口，保留 CSV 导出。

### 契约与门禁

- `frontend/src/api/openapi.d.ts`：由 OpenAPI typegen 刷新。
- `docs/security/permission-matrix.md`：由路由注册表刷新，登记模板目录和复盘导出审计动作。
- `backend/internal/incident/service_test.go`、`backend/internal/httpserver/incidents_test.go`、`backend/internal/config/config_test.go`、`frontend/src/api/incidents.test.ts`、`frontend/e2e/incidents.spec.ts`：覆盖模板默认值与切换、矩阵配置校验、Markdown 内容和客户端下载契约。

## Verification

- `cd backend && go test ./... && go vet ./... && golangci-lint run ./...`：全部通过，lint 0 issues。
- `cd frontend && pnpm typegen && pnpm typecheck && pnpm lint && pnpm test -- --run && pnpm build && pnpm bundle:gate`：全部通过，27 files / 148 tests，bundle gate 通过。
- `cd frontend && pnpm test:e2e`：Desktop/Mobile 全量 76/76 通过，包含事故创建、完整协作旅程、复盘导出和 viewer 权限路径。
- `cd frontend && pnpm ui:gate`：CSS token、62 条截图基线、axe 32 视图双端和 bundle 四项全绿；事故页桌面/移动截图与基线一致。
- `docker compose config --quiet`：通过。
- `bash scripts/scan-sensitive-fields.sh`：通过，1381 个 tracked files 无敏感字段命中。
- PostgreSQL 迁移 `000048`：在本地 `k8s-aiops-postgres-1` 完成 up/down/up 往返，最终 `template_id` 与 `incidents_template_idx` 均存在。
- `docker compose build backend frontend && docker compose up -d --no-deps backend frontend`：新镜像构建并切换成功；backend/frontend/postgres 均 healthy，`000048_incident_response_templates.up.sql` 已写入 `schema_migrations`。
- 运行态：`GET /api/v1/health/ready` 返回 ready，`admin/admin123` 登录成功；`GET /api/v1/incidents/templates` 返回 4 个模板及 60/240/1440/4320 分钟矩阵，前端 `/incidents` 返回 200。

## Risks / Notes

- 模板目录当前为代码版本化只读目录，不提供运行时模板管理 API；后续如需运营自定义模板，应新增持久化配置契约和版本迁移。
- `INCIDENT_SLA_TARGETS` 接受 JSON duration 字符串，单项范围为 `1m` 到 `720h`；未配置时使用内置默认值。
- Markdown 导出允许未解决事故导出当前快照，未写入任何事故状态；复盘正文为空时使用事故摘要作为叙事回退。
