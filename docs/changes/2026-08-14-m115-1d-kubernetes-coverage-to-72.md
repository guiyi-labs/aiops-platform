# M115-1d：kubernetes 包覆盖率 64.5% → 72.5%（gateway 只读/写路径测试）

- Date: 2026-08-14
- Status: Complete
- Scope: M115 工程卓越冲刺第四片：`internal/kubernetes` 覆盖率从 64.5% 提到
  72.5%，新增 rollout/replica-set/node/workload 读取与 patch 全错误分支测试。

## Context

M115（docs/development-roadmap-post-m110.md Track F）全局覆盖率 65%→70%。
`internal/kubernetes` 是 k8s gateway 的 HTTP 客户端包装（getJSON 走 Gateway
接口），测试已有 gatewayStub 基建（不需要真实 HTTP 服务器），是低成本兑现
覆盖率的第三大杠杆（service.go 991 stmts）。本片服务.go 覆盖 0% 函数全部清零、
多数列表函数错误分支补齐。

## What Changed

全部为 `backend/internal/kubernetes/rollout_test.go`（新增，无生产代码改动）：

- `TestReplicaSetsByOwnerFiltersByOwnerUIDAndKind`：owner UID+Kind 双条件过滤、
  非 Deployment owner 与其它 UID 剔除。
- `TestRolloutHistoryBuildsSortedRevisions`：deployment+replicaSets 双请求
  组装 revision 列表（revision 排序、Current 标记、容器镜像收集）；
  `TestRolloutHistorySkipsZeroRevisionReplicaSets`（revision==0 跳过）；
  `TestRolloutHistoryRejectsEmptyUID` / `TestRolloutHistoryGatewayError` /
  `TestRolloutHistoryReplicaSetsGatewayError`（Deployment 成功但 RS 失败）。
- `TestRolloutStatusPopulatesPhaseAndDefaults`（Progressing=False +
  ProgressDeadlineExceeded → phase/reason/message）、
  `TestRolloutStatusDefaultsDesiredReplicasTo1`（Spec.Replicas nil → 1、
  Conditions nil → 空切片）、`TestRolloutStatusRejectsEmptyUID`、
  `TestRolloutStatusGatewayError`。
- `TestPatchNodeSuccess` / `TestPatchNodeDryRunSetsQuery` /
  `TestPatchNodeNonPatchGatewayRejected`（getter-only gateway → 错误）/
  `TestPatchNodeDisabledCluster`（ErrDisabled）。
- `TestWorkloadTemplateUnmarshalJSON`：自定义 UnmarshalJSON（Raw 保留）。
- 列表错误分支：Namespaces/Nodes/Deployments/StatefulSets/DaemonSets/
  ReplicaSets/Jobs/CronJobs/HorizontalPodAutoscalers/NodeMetrics
  gateway error 与 disabled-cluster 分支；PatchDeployment/PatchCronJob
  disabled + gateway error 分支。

## Verification

- `go test ./internal/kubernetes/`：全绿（既有 40+ 测试 + 新增 20+ 用例）。
- `go test -cover ./internal/kubernetes/`：72.5%（基线 64.5%，+8.0pp）。
- kubernetes 包内变量：被多个核心包（slo/posture/metricshistory 等）依赖，
  提升它是全局覆盖率高效路径；全局数字在 M115 后续切片统一汇总。

## Risks / Notes

- gatewayStub 是既有测试基建；新增测试沿用 responses map 按 path 分发响应。
- getterOnlyGateway 仅实现 Gateway.Get，用于证明 PatchNode 对非 PatchGateway
  的 fail-fast。
- 覆盖率门禁 ci.yml 65.0 仍未改（M115-1e 统一上调）。
