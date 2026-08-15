# M115-1e：namespaceposture 包覆盖率 54% → 90.2%（workload 聚合全分支测试）

- Date: 2026-08-14
- Status: Complete
- Scope: M115 工程卓越冲刺第五片：`internal/namespaceposture` 覆盖率从约 54%
  提到 90.2%，WorkloadSummary 五种 workload fetcher 全路径 + 各 section 错误
  分支全部覆盖。

## Context

M115（docs/development-roadmap-post-m110.md Track F）全局覆盖率 65%→70%。
namespaceposture 是 Get/List 聚合服务，依赖 KubernetesSource 接口（mockK8sSource
每方法可独立注入成功/失败），成本极低。本片将其顶上 90% 一线，为全局 70% 贡献
约 200+ stmts。

## What Changed

全部为 `backend/internal/namespaceposture/service_test.go` 扩展（无生产代码改动）：

- 新增 makeStatefulSet/makeDaemonSet/makeJob/makeCronJob 构造器（与
  k8sgateway 匿名结构体含 json tag 精确对齐）。
- `TestGet_AllWorkloadFetchersPopulated`：五种 workload（deployment/sts/ds/
  job/cronjob）同时返回数据时的聚合（DesiredTotal/ReadyTotal、无 partial）。
- `TestGet_WorkloadFetchersPartialFailure`：sts/ds/job 失败 → workloads 落入
  PartialSections，Get 不失败。
- `TestList_NamespaceLookupError`、`TestCollectPodsError`、
  `TestCollectNodeCapacityError`：单一 source 错误分支。
- `TestCollectWorkloadsPartialFailure`：全部 fetcher 失败 → ByKind 空 +
  Evidence.Status=SourcePartial + errPartial。
- `TestGet_PartialSectionForResourceQuotaError`：单一 section 失败 → 该 section
  入 PartialSections。
- `TestGet_ErrorAggregatedInSections`：所有 section 失败 → 多 section partial，
  聚合不整体失败（best-effort 语义）。

## Verification

- `go test ./internal/namespaceposture/`：全绿（新增 9 测试）。
- `go test -cover ./internal/namespaceposture/`：90.2%（基线约 54%，+36pp）。
- 剩余 47 stmts 主要为 deriveFindings 边界与 error 哨兵类型，收益低，暂不追。

## Risks / Notes

- 测试构造器必须镜像 k8sgateway 匿名结构体（含 json tag），k8sgateway 若改
  Config 结构需同步。
- 覆盖率门禁 ci.yml 65.0 仍未改（M115 门禁统一上调片尚未执行）。
