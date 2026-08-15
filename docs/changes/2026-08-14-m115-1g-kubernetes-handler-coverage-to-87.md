# M115-1g：httpserver/kubernetes.go 资源处理器覆盖率 9.6% → 87.6%

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第七片：kubernetes 资源 HTTP 处理器（约 70 个 handler 从 0%
  覆盖）带起 httpserver/kubernetes.go 从 9.6% 到 87.6%。这是全局覆盖率迈向 70%
  的单片最重杠杆（+389 stmts）。

## Context

`internal/httpserver/kubernetes.go` 包装 kubernetes 网关，约 70 个资源只读
handler。此前无集成测试基建（`Option.Kubernetes` 在优化测试里为 nil），全部
0%。本片用 `k8sgateway.NewService(credStub, getStub, nil)` 构造真实 service +
内存 gateway，借既有 `withClusterActor` 注入 actor 元数据，逐个 handler 走
HTTP。

## What Changed

新增 `internal/httpserver/kubernetes_handler_test.go`（无生产代码改动）：

- `k8sCredStub`（enabled 开关）+ `k8sGetStub`（按 path 返回 `{"items":[]}`）。
- `TestKubernetesListHandlersReturnOK`：30 个 list 资源（namespaces/nodes/
  metrics/pods/deployments/statefulsets/daemonsets/replicasets/jobs/cronjobs/
  hpa/resourcequotas/limitranges/secrets/services/ingresses/endpointslices/
  storageclasses/pvc/configmaps/pods/events/networkpolicies/pdb/sa/roles/
  clusterroles/rolebindings/clusterrolebindings/persistentvolumes）。
- `TestKubernetesDetailHandlersReturnOK`：27 个 detail/manifest handler。
- `TestKubernetesDisabledClusterReturnsConflict`、`TestKubernetesLogsValidationErrors`、
  `TestKubernetesLogsSinceValidationErrors`、`TestKubernetesAllContainerLogsValidation`、
  `TestKubernetesContainersAndEventsOK`、`TestKubernetesCustomResourcesBranches`
  （缺 path 参数 400 / 未 whitelist 404 / 缺 name 400）、
  `TestKubernetesVeleroCapabilityAndBackups`。

## Verification

- `go test ./internal/httpserver/`：全绿（既有测试 + 新增 9 测试）。
- `go test -cover ./internal/httpserver/`：kubernetes.go 9.6% → 87.6%（+389 stmts）。
- 全局覆盖率由 ~67.7% → ~68.9%（差 ~285 stmts 到 70%）。

## Risks / Notes

- Detail handler 测试用 gin.CreateTestContext + 显式 Params 直调 handler，
  不在路由树上；`authorizedNamespaceLists` 走默认 AllNamespaces scope，
  与 aucer=20 授权中间件等价的 nil-service 直通一致。
- Manifest 测试用 kind "Pod"（manifestApplylist 白名单内）。
- 剩余 62 stmts 主要是 apiResources（DiscoveryProvider nil → 500）与
  authorizedNamespaceLists 多 namespace 聚合分支，M115 收口片补齐。
