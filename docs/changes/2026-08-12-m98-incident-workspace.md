# M98 Incident Workspace：事故工作空间与协作闭环

- Date: 2026-08-12
- Status: Complete
- Scope: 事故工作空间（incidents）后端领域模块、迁移、REST/OpenAPI、前端视图与桌面/移动端 e2e；前端 Dockerfile 网络可靠性修复。

## Context

M98 把单条诊断提升为可交接、可追踪、可复盘的事故工作空间：事故编号、负责人、关注者、显式状态机、SLA 与 append-only 时间线。目标是在不扩大审计敏感信息暴露的前提下，让确认、交接、解决、驳回、重开与复盘形成闭环，并用 CAS 保证并发更新不互相覆盖。

## What Changed

### Backend：incident 领域模块

- `backend/internal/incident/model.go`：事故/时间线领域模型、状态机（open→confirmed→resolved/dismissed，resolved↔open 重开）、SLA 截止与逾期、`safeCSVCell` 脱敏导出。
- `backend/internal/incident/repository.go`：PostgreSQL 仓储，`WHERE id=? AND version=?` 实现 compare-and-swap，版本冲突返回 409。
- `backend/internal/incident/service.go`：创建（finding/diagnosis 来源去重）、状态变更、负责人交接、关注者增删、备注、复盘，时间线系统事件与人工备注分离。
- `backend/internal/incident/model_test.go` / `service_test.go`：状态机边界、SLA、CAS 冲突、工作流与导出（fake repo）单测。
- `backend/migrations/000040_incidents.up.sql` / `.down.sql`：incidents / incident_timeline / incident_followers 表与索引；容器启动自动应用（psql 已验证）。
- `backend/internal/httpserver/incidents.go` + `incidents_test.go`：REST 处理器（含 CSV 导出 `ExportOne`）。
- `backend/internal/httpserver/router.go`：v1 组注册 incidents 路由，写操作要求 `rolesSystemOpsAdmin`。
- `backend/cmd/server/main.go` + `backend/cmd/server/incident_resolver.go`：服务接线，diagnosis→SourceInfo 解析。
- `docs/api/openapi.yaml` + `backend/internal/httpserver/openapi_route_test.go`：incidents 路径/schema、`IncidentID` 参数；`frontend/src/api/openapi.d.ts` 已重新生成。

### Frontend：事故工作空间视图

- `frontend/src/types/incident.ts` / `frontend/src/api/incidents.ts`：类型与 API 客户端（CSV 下载用原生 fetch）。
- `frontend/src/views/IncidentsView.vue`：汇总面板、列表、详情抽屉（状态变更/交接/关注/备注/复盘/时间线）与新建表单；`@media (max-width: 720px)` 下表格横向滚动、drawer/form 全宽、action-row 纵向堆叠。
- `frontend/src/router/index.ts`：`/incidents` 路由；`frontend/src/components/ConsoleLayout.vue`：侧栏“事故工作空间”入口。
- `frontend/e2e/incidents.spec.ts`：4 个用例（列表+详情、完整工作流、创建校验、viewer 无权限）× Desktop/Mobile = 8 条。

### Fixed（本轮修复）

- 时间线系统事件 `actor_user_id=0` 违反外键 → `ID<=0` 时插入 NULL。
- Transition/Assign/SetPostmortem 的 `WHERE id=? AND version=?` 参数顺序写反（曾导致 409）→ 修正为 `id, stored.Version`。
- Transition 原不递增 version → 增加 `version = version + 1`。
- 错误码 `USER_NOT_FOUND` 与 users 冲突 → 改为 `INCIDENT_USER_NOT_FOUND`（error_code_audit_test 要求唯一 UPPER_SNAKE）。
- 导出端点曾按 `ListFilter{ClusterID, Limit:1}` 导出最后一行 → 改为 handler 先 `Get(id)` 再 `service.ExportOne(id)`（运行时用 INC-000004 验证单行正确）。
- 移动端 e2e 失败根因：`base.css` 在 `@media (max-width: 720px)` 给 `.compact-table` 设 `min-width: 620px`，表格无横向滚动容器把页面撑宽到 711px，fixed overlay 命中测试错乱 → 新增 `.incident-table-scroll` 滚动容器 + drawer/form 全宽适配。
- `frontend/Dockerfile`：`pnpm install --frozen-lockfile --network-concurrency 4 --fetch-retries 5`，降低镜像构建网络抖动。

## Verification

- `cd backend && go test ./...`：全绿；`go vet ./...`：通过；`golangci-lint run ./...`：0 issues（v2.12.2）。
- `cd frontend && pnpm exec vue-tsc -b --force`：通过；`pnpm lint`：0 issues；`pnpm test`：25 files / 137 tests 通过；`pnpm build`：成功。
- `cd frontend && pnpm exec playwright test e2e/incidents.spec.ts`：8/8 通过（Desktop Chrome 4 + Mobile Chrome 4）。
- 运行时冒烟：compose 栈 healthy；迁移 000040 自动应用；`INC-000004` 导出为单行正确；`/api/v1/incidents` 列表/详情/CSV 正常。
- 本地环境：安装 golangci-lint 2.12.2（brew）与 Playwright chromium；`.env`（含 `BOOTSTRAP_ADMIN_PASSWORD`、DB DSN）不入库。

## Risks / Notes

- 开发库 postgres 存在 `smoke` 集群与若干测试 incidents（INC-000002/4/5/6…），属冒烟残留，无实际影响。
- 事故导出仅覆盖内置列，自由备注/复盘正文不进入导出，保持默认脱敏。
- 下一步：M99 信号关联与 SLO（`SignalRef`、Deployment 变更→错误预算黄金场景、关联结果 golden replay）。
