# Operator/CRD 增强（Commit 2）：reconcile controller + 单元测试

- Date: 2026-08-15
- Status: Complete
- Scope: 战略任务「Operator/CRD 增强」第二个提交：controller 核心逻辑。

## What Changed

`backend/internal/operator/`：

- `controller.go`：`Reconciler`（核心 reconcile 逻辑，仅依赖 Client +
  TargetExecutor 接口，可单测）+ `Controller`（client-go workqueue 经典
  单 worker 循环）+ finalizer 清理 + 幂等判断（同 idempotencyKey 不重放）。
- `executor.go`：`dynamicExecutor` 生产实现 —— 经 dynamic client 对目标
  Deployment/CronJob 发 MergePatch；dry-run 时透传 `dryRun=All`（不落盘），
  且 dryRun 默认 true；固定 action 目录 → 固定 patch（restart 注解 /
  scale replicas / suspend）。
- `controller_test.go`：fake dynamic client + recording executor 单测，
  覆盖：成功路径、unsupported action 拒绝、unsupported targetKind 拒绝、
  executor 错误 → Failed、幂等（两次 reconcile 只执行一次）、NotFound
  no-op、删除 finalizer 清理、queue/ProcessOne 驱动、dynamicExecutor
  patch 落盘验证与缺参报错。

## Verification

- `go build` + `go vet` + `go test -count=1 ./internal/operator/`：15 用例全绿。

## Risks / Notes

- 永久性失败写入 status.Failed 不重试；仅瞬时 API 错误 requeue（QueueLen
  可探测）；informer 装配留给 cmd 入口（Commit 3 一并落地 deploy）。
