# M108：关联归一收口 — 集群级信号关联段 + drill 修复（demo-drill 41/41）

- Date: 2026-08-13
- Status: Complete
- Scope: 修复关联引擎对集群级（Node）信号的漏关联（命名空间段只覆盖 ns 内资源），并修复 demo-drill 两处既有断言缺陷（batch 载荷类型、2/2 信号归并依赖），M108 验收端到端全绿。

## Context

demo-drill 全量复验暴露两个问题：

1. **关联引擎漏 Node 级信号**：`correlation.Worker.runClusterPass` 按命名空间逐段关联，而 Node 级信号（`diag.node.not_ready.v1`）namespace 为空，永远进不了任何命名空间段 → `maintenance_causes_node_failure` 规则在能列出命名空间的集群上永不触发，demo 中 Node 关联案例不产生。
2. **drill 既有断言缺陷**：M107 batch-assign 步骤把 `incident_ids` 发成字符串数组 `["1"]`，Go `encoding/json` 拒绝字符串→int64（`cannot unmarshal string into ... int64`），断言必失败；M108 Block 1 的「2/2 信号归并」依赖 Node 案例，被问题 1 卡住。

## What Changed

### Backend：worker 集群级关联段

- `backend/internal/correlation/worker.go`：`runClusterPass` 在命名空间段之后追加一次 all-namespace 段（`runScope(ctx, c.ID, "")`，provider 空 namespace = 不限），集群级信号（Node 等）也能进入关联；upsert 按 case_key 归并，重复段幂等。空命名空间集群保持原有跳过行为不变。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：batch-assign 载荷 `incident_ids` 由字符串数组改为数字数组（`[$INCIDENT_ID]`，与 OpenAPI schema/Go 解码一致）。
- 第 13 节「2/2 信号归并」断言随 worker 修复转绿：Node 信号归一出第二个案例并可独立提升为事故。

### 测试

- `backend/internal/correlation/worker_test.go`：`TestWorkerRunPassScopesAndSkipsDisabled` 期望序列补集群级段（cluster1: app/data/""、cluster3: tools/""）。

## Verification

- `cd backend && go test ./internal/correlation/ ./internal/signal/ ./internal/incident/ ./internal/httpserver/ ./cmd/server/`：全绿。
- `./scripts/demo-drill.sh`（本地重建 `k8s-aiops-backend:latest` 后全量复跑）：**41/41 PASS**，含新增 `correlation-case-merge`/`correlation-incident-merge`（2/2）与 `incident-batch-assign`；报告 `.artifacts/demo-drill/report-20260813-162314-8518b3.json`（summary 41/0，correlation_incident case_id=1 incident_id=8，correlation_incident_2 case_id=2 incident_id=10；artifacts 不入库）。

## Risks / Notes

- 每次关联周期每集群多一次 all-namespace 段：重复段幂等（case_key 归并），成本为每周期一次额外引擎运行；集群资源多时可经 `CORRELATION_INTERVAL` 调节频率。
- batch 载荷修复只影响 drill 脚本，不改后端契约（OpenAPI 本就要求 integer 数组）。
- 本地镜像 `k8s-aiops-backend:latest` 已重建承载 worker 修复（离线：以既有镜像为基础仅替换二进制，未依赖 Docker Hub）。
