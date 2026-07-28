# 2026-07-17 AI Runtime Guardrails

## Scope

- 新增 `AI_DAILY_TOKEN_BUDGET`、`AI_MAX_CONCURRENT_REQUESTS` 与 `AI_MAX_OUTPUT_TOKENS` 配置，并同步 Compose 与环境变量模板。
- Responses-compatible 请求显式携带 `max_output_tokens`。
- 新增 `000009_ai_usage_reservations`，在 Provider 调用前短期预留估算 token。
- 通过 PostgreSQL advisory lock 对多实例共享的 UTC 每日预算执行原子检查；成功后按实际 usage 记账，完成、失败或超时均释放预留。
- 使用进程内非阻塞并发门控，满载和预算不足分别返回 `AI_BUSY` 与 `AI_BUDGET_EXCEEDED`。
- 新增 `/api/v1/ai/status`，公开非敏感 Provider、并发、预算、当日用量与最近成功状态。
- 诊断页面展示 AI 运行状态和 token 预算，在禁用或余额为零时关闭生成入口，并保留规则结果与证据。

## Verification

- Go 全量测试覆盖配置范围、`max_output_tokens` 请求、预算预留/释放、并发拒绝、状态余额和错误映射。
- 前端类型检查与 7 个 Vitest 文件共 16 项测试通过，包含状态 API 契约。
- PostgreSQL 真实应用 `000009`；请求执行期间状态显示 1 个活动请求与 684 个预留 token。
- 本地延迟 Provider 下第二个并发请求返回 HTTP 429 `AI_BUSY`，首个请求返回 201；完成后预留表为 0。
- 首个成功响应实际记账 180 输入与 72 输出 token，当日余额由 900 变为 648；下一次估算预留超过余额，返回 HTTP 429 `AI_BUDGET_EXCEEDED`，解释历史仍为 1 条。
- 页面验收确认运行状态、用量、引用式解释和预算不足回退信息可见，确定性证据不受影响。

## Boundaries

- 每日预算按 UTC 重置；状态数值是读取时快照。
- token 预留使用字符长度近似估算，不替代 Provider 的实际 usage。
- 并发上限按后端进程生效；多实例全局并发限流与 Provider 主动健康探测仍未实现。
- AI 护栏只限制解释增强，不限制规则诊断、证据读取或人工处置。
