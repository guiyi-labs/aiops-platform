# M99-C：correlation 生产 InputProvider 与周期关联 worker

- Date: 2026-08-12
- Status: Complete
- Scope: M99 第三步——让关联案例在生产环境自动产生：`InputProvider` 从 signal/topology/diagnosis 仓储读取真实输入，周期 worker 按集群/命名空间自动跑关联，无需人工触发。

## Context

M99-A/B 已打通 SLO burn 信号管道与 workload_readiness 指标源，但生产 `correlation.NewService(..., nil, nil)` 处于查询模式（NopInputProvider）：没有输入源，也没有任何进程触发 `CorrelateNamespace`，`correlation_cases` 永远不会有新案例。本步骤实现生产 provider 与周期 worker，把关联引擎接入真实数据。

## What Changed

### Backend：correlation 生产输入源

- `backend/internal/correlation/provider.go`：`RepositoryInputProvider` 实现 `InputProvider`，通过三个窄接口读取仓储——signal（仅 active、按 lookback 起点的 `observed_at`）、topology（edges 取当前有效、changes 取 lookback 内 `started_at`）、diagnosis（lookback 内 `observed_at`）。
- 类型映射：signal `Coverage`/`Severity`/`State` 转字符串透传、资源 UID 缺失标 `Incomplete`、signal/topology 证据引用（kind/id/content_hash）映射为 correlation `EvidenceRef`、change 的 `ChangeKinds`/结果/置信度/来源逐字段透传、edge kind 字符串化。
- 每个读取源有独立上限（signals 200 / changes 100 / edges 200 / diagnoses 100），一次关联 pass 永不扫描无界历史。
- `backend/internal/diagnosis/model.go` + `repository.go`：`ListFilter` 新增可选 `Since *time.Time`（`observed_at >= ?`），供 provider 只取 lookback 内诊断；向后兼容（未设置时行为不变）。

### Backend：周期关联 worker

- `backend/internal/correlation/worker.go`：`Worker` 按 `Interval` 周期跑 `runPass`——列出集群（稳定 ID 序）、跳过 `Enabled=false` 的集群；每个集群按命名空间列表逐 scope 调 `CorrelateNamespace`；命名空间列表失败（集群不可达）时回退为一次全命名空间（`""`）pass，已落库的行仍然关联。
- 每集群 pass 有 `PerClusterTimeout`（默认 10s）；错误只记日志不崩溃；ctx 取消即停。
- `backend/internal/config/config.go`：新增 `CORRELATION_INTERVAL`（默认 5m，校验 30s–24h）。
- `backend/cmd/server/main.go`：`correlation.NewService` 换用生产 provider；新增 worker goroutine（与其他 collector 同生命周期，`backgroundWait` 计数 3→4）。

## Verification

- `cd backend && go test ./...`：全绿；`go vet ./...`：通过；`golangci-lint run ./...`：0 issues。
- 新用例：`TestProviderActiveSignalsMapsAndFilters`（active+lookback 过滤、coverage/severity/state 字符串映射、UID 缺失标 Incomplete、evidence 透传）、`TestProviderRecentChangesMapsAndFilters`、`TestProviderTopologyEdgesMapsAndFilters`（ValidAt=now）、`TestProviderRecentDiagnosesMapsAndFilters`（Since 过滤）、错误透传；`TestWorkerRunPassScopesAndSkipsDisabled`（禁用集群跳过、按命名空间 scope）、`TestWorkerRunPassNamespaceErrorFallsBackToAllNamespaces`、`TestWorkerRunPassEmptyNamespacesSkipsCluster`、`TestWorkerRunStopsOnContextCancel`。
- 运行时冒烟（重建 backend 镜像，容器 healthy）：`/api/v1/aiops/correlation/cases|timeline|rules`、`signals`、`slos` 均 200；无启用集群时 worker 冷启动不产生案例、不报错（符合预期）。

## Risks / Notes

- 当前环境无启用集群（smoke 集群 disabled、6443 未监听），worker 每轮 list 到的集群均为 disabled，不产生案例；接入真实集群后每轮自动按命名空间关联，SLO burn/诊断/变更输入即可落库为关联案例。
- 命名空间列表失败时回退全命名空间 pass 是刻意设计：宁可关联全量也不漏关联，case_key 幂等保证重复 pass 不产生重复案例。
- 下一步：M99-D（前端时间窗口/缺样本/延迟展示）。
