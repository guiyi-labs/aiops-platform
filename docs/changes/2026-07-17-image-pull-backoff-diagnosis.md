# 2026-07-17 ImagePullBackOff Diagnosis

## Scope

- 扩展 Node、Deployment、Service 只读 API 和前端资源概览。
- 增加诊断记录、证据表及事务持久化。
- 实现 `pod.image_pull_backoff.v1` 确定性规则。
- 证据采集关联 Pod container state 和按 Pod UID 筛选的 Warning Event。
- 前端增加诊断按钮、严重级别、结论、可能根因、建议和证据抽屉。
- 增加诊断历史 API，为后续智能诊断中心和人工反馈预留数据。

## Verification

- `go test ./...` 与 `go build ./cmd/server` 通过。
- 规则测试覆盖 ImagePullBackOff 命中和健康 Pod 不命中。
- 资源测试覆盖 Node、apps/v1 Deployment 和 Service 固定路径。
- `pnpm typecheck`、Vitest 和 `pnpm build` 通过。
- PostgreSQL 真实迁移成功；端到端验证 Node、Deployment、Service 各返回 1 条。
- `broken-api` 命中 `pod.image_pull_backoff.v1`，严重级别为 high；API 返回 2 条证据，数据库事务落库 2 条证据，历史接口返回记录。
- 浏览器验证 ImagePullBackOff 状态、诊断入口、根因、建议和两类证据；390px 下无横向溢出，控制台无错误或警告。

## Boundaries

- 根因列表是排查假设，不是已验证事实；只有 Pod 状态和 Event 进入 evidence。
- AI 未接入，规则调用失败不会回退到生成式结论。
- 未实现自动修复、Deployment 写操作或 Pod 删除。

## Deferred

- 处理状态和人工反馈。
- AI Provider、上下文裁剪、结构化输出和失败回退已在后续变更 `2026-07-17-cited-ai-explanations.md` 完成。
- client-go、真实 kind 故障清单与集成测试。

CrashLoopBackOff、Service 无 Ready Endpoint 与独立诊断历史页面已在后续变更 `2026-07-17-diagnosis-rules-and-history.md` 完成。
