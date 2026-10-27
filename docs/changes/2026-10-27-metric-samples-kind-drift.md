# metric-samples-kind-drift：修正指标历史采集的 resource_kind 口径漂移

- Date: 2026-10-27
- Status: Complete
- Scope: `metric_samples` / `metric_samples_downsampled` 的 `resource_kind` 约束与列宽；补迁移 `000051`

## Context

指标历史采集器自 M99-B 起会写入 **Deployment** 的 readiness 样本
（`internal/metricshistory/model.go: ResourceDeployment = "Deployment"`，
指标名 `readiness_ready` / `readiness_total`，单位 `count`），
但 `000017_metrics_history.up.sql` 建表时把 `resource_kind` 定为 `VARCHAR(8)`，
且 CHECK 只允许 `('Node', 'Pod')`。

后果：**每一批插入都因 `Deployment`（10 字符）超长而整笔事务回滚**，`metric_samples` 长期为空。
上游日志每 1 分钟一条：

```
ERROR metricshistory/collector.go:161 record metrics history collection
  {"cluster_id": 3, "error": "ERROR: value too long for type character varying(8) (SQLSTATE 22001)"}
```

连带失效的能力：

- 指标历史查询（`GET /clusters/{id}/metrics/history`）永远返回 `points: []`、`coverage.collections: 0`；
- 告警规则永远停在 `insufficient_data`（`minimum_points` 拿不到样本）；
- SLO 评估永远 `unavailable`（workload_readiness 无数据源）；
- 大盘「节点历史趋势」图恒为空。

`000049_metric_samples_downsampled.up.sql` 的注释已记录这处不一致（"pre-existing inconsistency"），
但当时只让新表更宽松，原表未修，问题一直未闭环。

## What Changed

### migrations
- `backend/migrations/000051_widen_metric_resource_kind.up.sql`：新增。
  - `metric_samples.resource_kind` 与 `metric_samples_downsampled.resource_kind` 放宽到 `VARCHAR(32)`；
  - `metric_samples_resource_kind_check` 纳入 `Deployment`；
  - `metric_samples_resource_shape_check` 区分三种形态：Node（无 namespace / 无 container）、
    Pod（有 namespace / 有 container）、Deployment（有 namespace / **无** container）；
  - `metric_samples_metric_check` 纳入 `readiness_ready|readiness_total` + `count`。
- `backend/migrations/000051_widen_metric_resource_kind.down.sql`：新增，回滚时先删除 Deployment 形态样本再收紧约束。

> 未改动任何 Go 代码：本次是 schema 与既有代码口径的对齐，不是逻辑修复。

## Verification

- `docker compose up -d --force-recreate backend` 后 `GET /health/ready` 返回 `ready`，
  `schema_migrations` 出现 `000051_widen_metric_resource_kind.up.sql`；
- `information_schema` 确认 `metric_samples.resource_kind` 已为 `32`；
- 采集日志由 `failed: 2` 变为 `{"attempted": 2, "recorded": 2, "failed": 0}`；
- `metric_samples` 行数（两集群）：cluster 2 = Node 6 / Pod 72 / Deployment 42，
  cluster 3 = Node 18 / Pod 234 / Deployment 102；
- `GET /clusters/3/metrics/history?...`：`points: 3`、`coverage.collections: 3`、`missing: 0`；
- 告警规则 `last_evaluation_state` 由 `insufficient_data` → `normal`；
- `POST /aiops/slos/{id}/evaluate`：`state: healthy`，readiness 100%。

## Risks / Notes

- 迁移对 `resource_kind` 放宽列宽属于在线 DDL，`ALTER COLUMN TYPE` 会重写表；当前表量为 0 行级别，代价可忽略，
  生产环境需按实际行数评估锁窗口。
- 回滚会**删除** `resource_kind = 'Deployment'` 的样本（旧约束无法容纳），属有意为之，已在 down 迁移中显式写出。
- 后续建议：采集器写入前对 (kind, shape) 组合做一次本地校验，让口径漂移在写入侧就报错，而不是等数据库 22001。
