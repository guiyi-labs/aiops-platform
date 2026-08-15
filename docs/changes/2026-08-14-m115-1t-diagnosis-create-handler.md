# M115-1t：diagnosis create 处理器 0% → 全覆盖

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第二十片：diagnosisHandler.create（此前 0%）。

## Context

create 处理器（diagnosis.go:53）此前完全未测：路由未注册、service 用 nil source。

## What Changed

`internal/httpserver/diagnosis_handler_test.go` 新增：

- `diagSourceStub`（11 方法 canned err）+ `diagSourceWithPod` + `diagSourceDeploymentStub`。
- `performDiagnosisCreate` helper（gin 直挂 create 路由 + actor metadata）。
- `TestDiagnosisHandler_CreateValidationBranches`（空 body / Pod 缺 ns / 不支持 kind / Node 免 ns）。
- `TestDiagnosisHandler_CreateErrorMapping`（404/422/409/404/502 错误映射）。
- `TestDiagnosisHandler_CreatePodSuccess`（OOMKilled pod → 201）。
- `TestDiagnosisHandler_CreateDeploymentSuccess`（Deployment source → 201/422）。

## Verification

- `go test ./internal/httpserver/`：全绿。
- create 0% → 100%；错误映射 switch 全覆盖。

## Risks / Notes

- Deployment.Spec.Replicas 是 *int32，须显式指针。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
