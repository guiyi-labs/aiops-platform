# M99-D：SLO / 信号 / 关联案例的缺样本与数据延迟显式展示

- Date: 2026-08-12
- Status: Complete
- Scope: M99 收尾步——满足 M99 验收“时间窗口、缺样本和数据延迟在 UI 中显式可见，不把缺失数据当作健康”：把 coverage/freshness/window 元数据落到关联案例，并在 SLO 仪表盘、信号列表、关联案例三处 UI 显式展示。

## Context

M99-A/B/C 已打通 burn 信号管道、workload_readiness 指标源与生产关联 worker，后端数据里已有 coverage（complete/partial/unavailable/truncated）、freshness、window_start/window_end、evaluated_at 等字段，但前端：
- 信号列表不展示覆盖度/窗口/数据延迟；
- SLO 仪表盘只显示状态与预算，缺样本（unavailable）与评估延迟不可见；
- 关联案例的信号链路只有观察时间，无覆盖度/窗口；
- `SignalCoverage` 类型缺 `unavailable`/`truncated`，与后端取值不一致。

## What Changed

### Backend：关联案例信号链路携带数据元数据

- `backend/internal/correlation/model.go`：`SignalLink` 新增 `Coverage`、`Freshness`、`WindowStart`、`WindowEnd`（freshness/window 为可空时间）。
- `backend/migrations/000041_correlation_signal_link_metadata.up.sql`（+ down）：`correlation_signal_links` 新增 `coverage`（默认 `complete`）、`freshness`、`window_start`、`window_end` 列。
- `backend/internal/correlation/engine.go`：`buildTriggerLink` 从触发信号输入拷贝覆盖度/新鲜度/采样窗口（freshness 为零时存 NULL）。
- `backend/internal/correlation/repository.go`：signalLink 行映射持久化/读取新字段。
- 单测：`TestBuildTriggerLinkCopiesSignalMetadata`（coverage/freshness/window 逐字段）、`TestBuildTriggerLinkZeroFreshnessIsNil`（零值不落库）。

### Frontend：缺样本与数据延迟显式可见

- `src/types/aiops.ts`：`SignalCoverage` 增加 `unavailable`/`truncated`（与后端一致）；`SignalLink` 增加 `coverage`/`freshness`/`window_start`/`window_end`。
- `src/views/SLODashboardView.vue`：评估卡片新增“数据覆盖”徽标（完整/部分样本/无数据/截断）+“评估延迟”（`evaluated_at - window_end`）+ 无样本提示“无样本窗口不视为健康”；评估历史表新增覆盖度与延迟列。
- `src/views/AIOpsOverviewView.vue`：信号列表新增覆盖度（含 title 解释，unavailable=无样本 fail-closed）、时间窗口、数据延迟（`ingested_at - observed_at`）列；补 `badge-unavailable`/`badge-truncated` 样式。
- `src/views/CorrelationCasesView.vue`：信号链路表新增覆盖度徽标与时间窗口列；存在非完整覆盖时显示提示“部分信号缺样本或覆盖不完整，案例置信度已相应调整”。

## Verification

- Backend：`go test ./...`、`go vet ./...`、`golangci-lint run ./...` 全绿（新增 2 个 correlation 用例）。
- Frontend：`pnpm typecheck`、`eslint`、137 单测、`pnpm build` 全过。
- 运行时：重建 backend 镜像并启动，容器 healthy；迁移 `000041` 已应用（`correlation_signal_links` 新列存在，`schema_migrations` 含 000041）；`/api/v1/aiops/correlation/cases|timeline|rules`、`signals`、`slos`、`slos/templates` 均 200。
- 前端镜像重建被环境网络阻塞：Docker Hub（registry-1.docker.io）不可达，`node:22.13.1-alpine3.21`/`nginx:1.27-alpine` 拉取超时；本地前端门禁（与 CI 相同的 typecheck/eslint/test/build）全绿，镜像由 CI 重建。

## Risks / Notes

- 迁移为存量 signal link 回填默认 `coverage='complete'`；新链接由引擎写入真实覆盖度/新鲜度/窗口，历史链接不重复写入（upsert 幂等）。
- “无样本”在 UI 中显式标记为 `unavailable`（fail-closed），与 healthy 状态视觉分离，不把缺失数据当作健康。
- 下一步：M100 安全与租户治理加固（见 `docs/next-long-term-plan.md`）。
