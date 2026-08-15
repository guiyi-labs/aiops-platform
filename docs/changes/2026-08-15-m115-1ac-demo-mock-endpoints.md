# M115-1ac：demo-kube-mock list/discovery/metrics 端点补齐

- Date: 2026-08-15
- Status: Complete
- Scope: M115 冲刺：demo-kube-mock 包 0% 函数清零（listPods/listDeployments/
  apiGroup/simpleMeta/fixture*) + notFoundWithMessage 分支。

## Context

`cmd/demo-kube-mock` 的 listPods（0%）、listDeployments（0%）、fixtureReplicaSet
（0%）、fixtureNodeMetric（0%）、fixturePodMetric（0%）、apiGroup（0%）、
simpleMeta（0%）、notFoundWithMessage（0%）以及 getObject/demo-kube-mock
ServeHTTP 的部分路由未测。

## What Changed

`cmd/demo-kube-mock/handler_test.go` 新增：

- `TestPodAndDeploymentListRoutes`（/api/v1/pods、/namespaces/demo/pods、
  /apis/apps/v1/.../deployments 全 200）。
- `TestDiscoveryAndMetricsEndpoints`（replicasets、/apis/apps/v1、
  metrics nodes/pods 全 200）。
- `TestGetObjectNotFoundWithMessage`（GET + PATCH 缺失对象 → 404）。

## Verification

- `go test ./cmd/demo-kube-mock/`：全绿。
- 上述 0% 函数清零；notFoundWithMessage/NotFound 分支补齐。

## Risks / Notes

- newHandler() 无参构造；直接 ServeHTTP 无需生成证书。
- 覆盖率门禁 ci.yml 65.0 暂保持，统一上调片在全局达 70% 后执行。
