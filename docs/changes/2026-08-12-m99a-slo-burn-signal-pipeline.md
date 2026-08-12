# M99-A：SLO Burn 信号管道与 AIOps 生产接线

- Date: 2026-08-12
- Status: Complete
- Scope: M99 信号关联第一步——SLO 错误预算燃烧信号化（slo→signal 管道）、correlation SLO-burn 规则与黄金场景、signal/slo/correlation 服务接入生产后端。

## Context

M99 目标是"把事件、指标、日志、变更、拓扑和 SLO 关联到同一资源与时间窗口"。此前 M39-M42 的 signal/slo/correlation 模块仅有库与测试，生产 main.go 从未接线（`/api/v1/aiops/signals|slos|correlation/*` 全部 404），且 SLO burn 状态变化没有进入信号管道。本步骤把错误预算消耗变成可关联信号，并让 AIOps 路由在生产可用。

## What Changed

### Backend：SLO burn 信号化

- `backend/internal/signal/model.go`：新增 `ProducerSLO = "slo"` 生产者。
- `backend/internal/signal/catalog.go`：注册 `slo.burn.fast.v1`（fallback critical）、`slo.burn.slow.v1`（warning）、`slo.burn.recovery.v1`（info），`CorrelationDims=["resource_uid","cluster_id","namespace"]`、`RequiredEvidence=["slo_burn_window"]`。
- `backend/internal/signal/slo_burn_normalizer.go`：`SLOBurnSignalSink` 实现 `slo.BurnAlertSink`——breach 转换映射为 fast/slow 信号（按定义 `FastBurnRate` 分类，读取失败回退默认 14.4），breach→healthy 映射为 recovery 信号；指纹按（code, slo_id, cluster, target, window_end）稳定；coverage 透传；证据 `slo_burn_window` 携带 content hash（ratio/burn_rate/window/版本）。
- `backend/internal/slo/service.go`：`BurnTransition` 增加 `Coverage` 字段（由 `eval.Coverage` 填充），使缺失数据窗口在信号层可见。
- `backend/internal/signal/slo_burn_normalizer_test.go`：fast/slow/fallback/recovery 分类、稳态跳过、coverage 透传、指纹稳定性、sink 幂等（重复投递经 upsert 去重）。

### Backend：correlation SLO-burn 规则与黄金场景

- `backend/internal/correlation/catalog.go`：新增 `correlation.rollout_causes_slo_burn.v1`（触发 `slo.burn.fast/slow.v1`，change kinds promotion/rollout，PrimaryKind Deployment，要求 same_uid + time_distance + change_symptom_rule）。
- `backend/internal/correlation/fixtures.go`：新增 2 个确定性黄金 fixture——`slo_burn_fast`（同 UID Deployment 变更→fast burn，confirmed）与 `slo_burn_unrelated`（不同 UID 无拓扑路径，contradicted），黄金回放从 9 场景增至 11 场景。

### Backend：生产接线

- `backend/cmd/server/main.go`：构造 `signalService`（gorm 仓储）、`sloService`（gorm 仓储 + `WithBurnAlertSink(SLOBurnSignalSink)`）、`correlationService`（gorm 仓储，查询模式）；写入 `httpserver.Options`，`/aiops/signals|signals/catalog|slos|slos/templates|correlation/cases|correlation/cases/timeline|correlation/rules` 生产路由变为可用。
- SLO 求值器暂以 nil 源构造（无 metrics provider 时返回 StateUnavailable，不产生 burn 信号）；真实 MetricsSource 适配器列为 M99-B。

## Verification

- `cd backend && go test ./...`：全绿（signal/slo/correlation 含新测试）。
- `go vet ./...`：通过；`golangci-lint run ./...`：0 issues。
- 黄金回放：`TestGoldenFixtures` 11/11（含 `slo_burn_fast` confirmed、`slo_burn_unrelated` contradicted）。
- 前端门禁（无前端改动，回归）：`vue-tsc -b --force`、`eslint`、137 单测、build 全部通过。
- 运行时冒烟（`docker compose build backend` + `up -d backend`，容器 healthy）：
  - `GET /api/v1/aiops/signals`、`/signals/catalog`（含 3 个 slo.burn 码）、`/slos`、`/slos/templates`、`/correlation/rules`（含 `correlation.rollout_causes_slo_burn.v1`）、`/correlation/cases?cluster_id=1`、`/correlation/cases/timeline?cluster_id=1` 均 200。
  - 回归：`GET /api/v1/incidents` 200。

## Risks / Notes

- 生产 SLO 求值器无 metrics 源：在 MetricsSource 适配器（M99-B，例如 metricshistory/Prometheus 桥接）落地前，生产不产生 burn 信号；信号管道、规则与黄金场景已由确定性测试覆盖。
- correlation 服务当前为查询模式（NopInputProvider），自动关联 worker 与生产 InputProvider（读取 signal/topology/change/diagnosis 仓储）列为 M99-C。
- 前端展示时间窗口/缺样本/数据延迟（SLODashboardView 与 CorrelationCasesView 深化）列为 M99-D。
