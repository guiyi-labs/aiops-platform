# 2026-07-17 Diagnosis Rules and History

## Scope

- 增加 `pod.crash_loop_backoff.v1`，采集当前 Waiting 状态、重启次数、上一次终止原因/退出码/时间和 BackOff Event。
- 增加 `service.no_ready_endpoints.v1`，采集 Service selector/端口与同名 Endpoints 地址计数。
- 排除 ExternalName、无 selector、已有 Ready address 的 Service，降低确定性规则误报。
- 增加 `GET /api/v1/diagnoses/{diagnosis_id}`，按需恢复持久化证据。
- 增加独立“智能诊断”页面，支持集群筛选、历史列表和证据详情抽屉。
- 工作负载页增加 CrashLoopBackOff Pod 与 selector Service 诊断入口。

## Verification

- `go test ./...` 全部通过，覆盖三条规则及 Service/Endpoints 固定读取路径。
- `go build ./cmd/server` 通过。
- `pnpm typecheck`、5 个 Vitest 文件共 10 项测试、`pnpm build` 全部通过。
- 真实 PostgreSQL 中写入 2 条诊断及 3 条证据；历史 API 返回 2 条，Service 详情恢复 2 条证据。
- 浏览器验证智能诊断导航、历史列表、资源引用、严重级别、详情抽屉、根因/建议及两类 Service 证据展示。
- `docker compose config --quiet` 与 `git diff --check` 通过后方可归档。

## Boundaries

- 规则只读取资源，不执行 Pod 重启、Deployment 修改或 Service 变更。
- Service 规则当前读取 core/v1 Endpoints；EndpointSlice 迁移单独设计。
- 列表响应不展开 evidence；详情 API 才读取证据行。
- AI 仍未接入，可能根因和建议来自版本化规则模板。

## Deferred

- AI Provider、引用式解释、上下文裁剪和失败回退已在后续变更 `2026-07-17-cited-ai-explanations.md` 完成。
- kind 故障场景、EndpointSlice 和完整端到端自动化。

诊断状态流转、负责人、人工反馈和 Dashboard 真实诊断统计已在后续变更 `2026-07-17-diagnosis-workflow-dashboard.md` 完成。
