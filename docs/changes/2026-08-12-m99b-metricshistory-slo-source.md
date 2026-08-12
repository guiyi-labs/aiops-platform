# M99-B：metricshistory 驱动的 SLO workload_readiness 指标源

- Date: 2026-08-12
- Status: Complete
- Scope: M99 第二步——让生产 SLO 求值真正基于历史数据：metricshistory 采集 workload readiness、`slo.MetricsSource` 适配器、SLO 服务接线与 CREATE 响应 ID 修复。

## Context

M99-A 已把 SLO burn 转换接入信号管道，但生产 SLO 求值器无 metrics 源（nil evaluator），任何模板都返回 StateUnavailable，永不产生 burn 信号。本步骤实现真实的指标源：采集器记录每个 Deployment 的 readiness 采样，SLO 求值器把采样转换为滚动窗口内的"就绪副本占比"，从而在 Deployment 变更导致就绪率下降时触发 burn 信号。无数据时如实返回 unavailable（fail-closed），绝不伪造健康。

## What Changed

### Backend：metricshistory 采集 workload readiness

- `backend/internal/metricshistory/model.go`：新增 `ResourceDeployment = "Deployment"` 资源类型与 `MetricReadinessReady` / `MetricReadinessTotal` / `UnitCount` 常量。
- `backend/internal/metricshistory/service.go`：`validSeriesShape` 接受 Deployment（namespace 必填、container 为空）；`metricUnit` 注册 readiness 指标（UnitCount）。
- `backend/internal/metricshistory/collector.go`：可选 `WithWorkloadReadinessSource`（实现 `Deployments(...)` 接口，生产接 kubernetesService）；每个采集轮次记录每个 Deployment 的 `readiness_ready`（ReadyReplicas）与 `readiness_total`（Replicas）样本，受 MaxSamples 预算约束；源失败时不产生样本（缺失窗口如实上报）。
- `backend/internal/metricshistory/collector_test.go`：readiness 样本记录、源失败不记录两个用例。

### Backend：SLO workload_readiness 指标源

- `backend/internal/slo/metricshistory_source.go`：`MetricshistorySource` 实现 `slo.MetricsSource.QuerySLI`——仅服务 `workload_readiness` 模板，按采集时间戳配对 ready/total 并转换为单调累计计数器（同一时间戳缺任一计数则丢弃，缺失数据≠未就绪）；输出按时间升序。request_* 模板返回空序列（如实无数据）。
- `backend/internal/slo/metricshistory_source_test.go`：累计转换、配对丢弃、乱序排序、request 模板空序列、evaluator 稳态健康/滚动就绪率下降触发 breach（burn rate ≥ fast）/无数据 fail-closed unavailable。
- `backend/cmd/server/main.go`：collector 加 `WithWorkloadReadinessSource(kubernetesService)`；SLO 求值器换为 `NewMetricshistorySource(metricsHistoryService)`，生产 burn 信号链路完整。

### Fixed

- `backend/internal/slo/repository.go`：gorm `CreateDefinition` 不把数据库生成的 ID 回写到 `def`，导致 `POST /api/v1/aiops/slos` 响应 `id: 0`；改为创建后 `def.ID = row.ID`（运行时验证返回 id=2）。

## Verification

- `cd backend && go test ./...`：全绿；`go vet ./...`：通过；`golangci-lint run ./...`：0 issues。
- 关键新用例：`TestMetricshistorySource_QuerySLIWorkloadReadiness`、`TestEvaluator_MetricshistoryRolloutBreachesBudget`（readiness 3→1，StateBreached 且 burn rate ≥ fast）、`TestEvaluator_MetricshistoryNoDataHonorsMissingPolicy`。
- 前端门禁回归：`vue-tsc -b --force`、`eslint`、137 单测、build 全过。
- 运行时冒烟（重建 backend 镜像，容器 healthy）：
  - `POST /aiops/slos` 创建 workload_readiness 定义返回真实 id（修复前为 0）。
  - `POST /aiops/slos/1/evaluate` 在无集群数据时返回 `state: unavailable, coverage: unavailable`，`/aiops/signals` 为空——无数据不伪造 burn。
  - `/aiops/slos`、`/aiops/signals` 等路由保持 200。

## Risks / Notes

- 当前环境无启用集群（smoke 集群 disabled、6443 未监听），readiness 历史为空；接入真实集群后采集器每轮记录 readiness，SLO 求值即产生真实 burn 信号（M99-D 前端展示待续）。
- request_success_ratio / request_latency_target_ratio 模板需要流量计数源（Prometheus 等），本步骤如实返回 no-data，不推断。
- 下一步：M99-C（correlation 生产 InputProvider + 周期关联 worker）、M99-D（前端时间窗口/缺样本/延迟展示）。
