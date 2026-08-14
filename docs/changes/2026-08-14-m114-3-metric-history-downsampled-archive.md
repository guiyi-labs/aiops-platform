# M114-3 指标历史下采样归档

- Date: 2026-08-14
- Status: Complete
- Scope: M114 可观测性深化——30 天下采样归档层（1 小时档），有界查询，无新全量写入路径。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track E 第 166–177 行：指标历史下采样——
  现有 7 天精确序列，扩展 30 天下采样归档（有界查询 + 前端渲染预算内，沿用 M96 预算机制）。
- 验收：所有查询有界（时间窗/条数上限）；事件聚合不丢失原始证据深链；前端 50k Pod DOM
  预算不回退；无新增全量写入平台数据库的路径（资源仍实时查 API Server）。
- M114-4（事件流/日志探索增强）经用户调整归入后续增强，不在本次 M114 基线内，以避免
  无边界延长里程碑。

## What Changed

### 迁移 `backend/migrations/000049_metric_samples_downsampled`
- 新增 `metric_samples_downsampled` 表：按 `(cluster_id, resource_kind, resource_namespace, resource_name, container_name, metric_name, bucket_hour)` 去重；
  每行存 `value_avg`/`value_max`/`sample_count`，`window_milliseconds = 3600000`（1 小时）。
- 资源种类（resource_kind）/指标名（metric_name）CHECK 有意不做约束——沿用精确表的设计但不
  携带 M99-B 历史约束不一致（精确表 CHECK 仅允许 Node/Pod + cpu/memory，但代码写 Deployment
  readiness 样本）。在迁移注释中显式标注。
- 下迁移为 `DROP TABLE IF EXISTS metric_samples_downsampled`。

### 纯包 `internal/metricshistory` 扩展（ADR 0004，无 cluster 访问，无新写入路径）
- **model.go**：新增 `DownsampledSample`（ClusterID~Unit, BucketHour, ValueAvg/Max, SampleCount,
  WindowMilliseconds）、`ArchiveSeriesQuery`、`Config.DownsampleRetention`（默认 30d）/
  `Config.MaxArchiveQueryWindow`（默认 30d）。
- **service.go**：新增 `defaultDownsampleRetention = 30d`、`defaultMaxArchiveQueryWindow = 30d`、
  `maxArchiveQueryWindow = 30d`（精确查询 maxQueryWindow 仍为 24h，不影响既有门禁）。
  新增 `Repository.QueryArchiveSeries` / `SaveDownsampledBatch` / `ListExpiringSamples` 方法。
  新增 `Service.QueryArchive(ctx, ArchiveSeriesQuery) → SeriesResponse`（只读，有界：
  MaxArchiveQueryWindow 上限 30d，Limit 上限 MaxQueryPoints=1440）。
  新增 `Service.DownsampleAndArchive(ctx, []Sample) → (int, error)`：将一批精确样本按
  1 小时档聚合（avg/max/count），幂等 upsert 到归档表。
  `Service.Cleanup` 扩展：先 `ListExpiringSamples`（有界批次），再 `DownsampleAndArchive`
  幂等写入归档，最后 `DeleteExpired` 删除精确行。归档失败不影响精确数据清理。
- **repository.go**：新增 `GormRepository.QueryArchiveSeries`（读归档表，有界 LIMIT，
  返回 Point{ValueAvg, SourceTimestamp=BucketHour}）；`SaveDownsampledBatch`（按
  bucket key 幂等 upsert，有冲突时取 SampleCount 更大者更新）；`ListExpiringSamples`
 （JOIN metric_samples + runs WHERE expires_at < cutoff ORDER BY expires_at LIMIT）。
- **validConfig**：新增 `DownsampleRetention`（1h–90d）与 `MaxArchiveQueryWindow`
  （≥ MaxQueryWindow，≤ 30d）验证。`MaxQueryWindow` 保持 24h 不变，不影响既有测试。

### HTTP Handler `internal/httpserver/metrics_history.go`
- 新增 `metricsHistoryHandler.archiveSeries` 方法：解析 from/to/limit + 集群 ID + 系列键，
  调用 `Service.QueryArchive`，返回 JSON。400 INVALID_QUERY / 404 CLUSTER_NOT_FOUND /
  500 METRICS_HISTORY_QUERY_FAILED。

### 路由注册 `router.go`
- `metricsHistoryRoutes` 新增 `GET /clusters/:cluster_id/metrics/history/archive`
  （AuthRequired；AuditAction `metrics.history.archive.read`；AuditResource
  `MetricHistoryArchive`）。使用 `withClusterContext + requireClusterAccess`（集群级
  RBAC，与精确端点一致）。

### OpenAPI / 权限矩阵 / typegen
- `docs/api/openapi.yaml`：新增 `GET /api/v1/clusters/{cluster_id}/metrics/history/archive`
  （operationId `getMetricHistoryArchive`，description 明确只读/有界/小时档），
  复用 `MetricHistoryResponse` schema，from/to 描述改为 30 天上限。
- `docs/security/permission-matrix.md`：重生成（`metrics.history.archive.read` | any | cluster）。
- `frontend/src/api/openapi.d.ts`：typegen 重生成（archive 端点已入生成产物）。

### 前端
- `frontend/src/types/metrics-history.ts`：
  - `MetricHistoryRangeHours` 扩展为 `1 | 6 | 24 | 168 | 720`（新增 7d、30d）。
  - `MetricHistoryLimits.max_window_seconds` 类型扩展为 `86400 | 2592000`（30d = 2592000 秒）。
- `frontend/src/api/metrics-history.ts`：新增 `getMetricHistoryArchive(token, clusterID, input)`
  → `/api/v1/clusters/${id}/metrics/history/archive`（复用 `historyQueryString`）。
- `frontend/src/api/metrics-history.test.ts`：新增测试"路由 7d/30d 窗口到归档端点"，断言
  pathname = `.../archive`，查询参数正确，Authorization 头正确。
- `frontend/src/components/MetricsHistoryPanel.vue`：
  - 引入 `getMetricHistoryArchive`。
  - 时间范围控件扩展为 `[1h, 6h, 24h, 7d, 30d]`（`h: 1/6/24/168/720`，`label: 1h/6h/24h/7d/30d`）。
  - `loadHistory()` 根据 `rangeHours.value >= 168` 选择调用归档端点或精确端点。
  - `periodLabel`：rangeHours ≥ 168 时附加 ` · 下采样(小时档)` 标识。

### 测试
- `service_test.go`：新增 `repositoryStub.ListExpiringSamples` / `QueryArchiveSeries` /
  `SaveDownsampledBatch` 方法，新增 `downsampled` 字段。新增测试：
  - `TestDownsampleAndArchiveAggregatesHourlyBuckets`（同小时同系列聚合，3 点→avg=300, max=500, count=3）
  - `TestDownsampleAndArchiveEmptyInput`（空输入返回 0）
  - `TestQueryArchiveValidatesBoundsAndReturnsSeries`（有界窗口/limit/返回 series）
  - `TestQueryArchiveRejectsInvalidShapeWindowAndMetric`（6 个拒绝场景：零集群、非法
    shape、非法 metric、窗口 >30d、from=to、limit >1440）
  - `TestCleanupArchivesExpiringSamplesBeforeDelete`（验证 Cleanup 调用 ListExpiringSamples）
  全部**绿**。
- `metrics_history_test.go`：`metricsHistoryRepositoryStub` 新增 3 个方法。新增测试：
  - `TestMetricsHistoryHandlerParsesArchiveSeriesQuery`（48h 窗口命中归档端点、status 200、
    MaxWindowSeconds=2592000）
  - `TestMetricsHistoryHandlerRejectsInvalidArchiveQueryShapes`（>30d / 非法时间 / limit>1440）
  全部**绿**。
- `slo/metricshistory_source_test.go`：`fakeMetricsRepo` 新增 3 个方法满足接口。
- 全量后端：**72 包全绿**；OpenAPI 路由双向匹配 + 权限矩阵一致性通过。
- 前端：pnpm typegen / typecheck / lint / test（**160** 通过）/ build 全绿。

## Verification

- `go test ./...`：72 包全绿。
- `pnpm typegen` / `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（160 passed）/
  `pnpm build`：全绿。
- `backend/internal/metricshistory/`：5 个新增测试全部绿（DownsampleAndArchive 聚合、
  空输入、QueryArchive 返回值/有界拒绝、Cleanup 归档流程）。
- `backend/internal/httpserver/metrics_history_test.go`：2 个新增测试绿（归档端点参数解析、
  非法查询形状拒绝）。归档端点在 `TestRegisteredRoutesMatchOpenAPI` 和
  `TestPermissionMatrixMatchesCommittedDocument` 中均通过。
- 归档表为纯追加（幂等 upsert），不修改精确数据；Cleanup 中的归档步骤为只读 + best-effort
  写入（归档失败不阻止精确数据清理）。
- 精确查询 MaxQueryWindow 仍为 24h，现有测试不受影响。

## Risks / Notes

- **Deployment/readiness CHECK 不一致**：精确表 `metric_samples` 的 000017 迁移约束
  resource_kind IN ('Node','Pod') / metric_name IN ('cpu','memory')，但 M99-B 代码写入
  Deployment/readiness_ready/total 样本。归档表有意移除这两个 CHECK（见迁移注释），
  避免将历史不一致带入新表。后续应统一约束（ALTER TABLE 约束 + 迁移）。
- **归档为只读延迟写**：DownsampleAndArchive 在 Cleanup 期间调用，是有界的批量读 + 幂等
  upsert（非实时流写入），不新增"全量写入路径"，符合 M114 验收要求。
- **前端 50k Pod DOM 预算不回退**：归档端点复用现有 MaxQueryPoints=1440 界限，
  渲染侧使用同一 `buildMetricChart`，无新增 DOM 树。
- **下采样精度**：每小时一个 value_avg + value_max，原始分钟级波动在长窗口下不可见，
  但覆盖趋势面。前端标注"下采样(小时档)"避免误读。
