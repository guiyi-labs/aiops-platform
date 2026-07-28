# ADR 0011: AI Runtime Budget and Concurrency Guardrails

- Status: Accepted
- Date: 2026-07-17

## Context

同步 AI 解释会占用外部 Provider 容量并产生 token 成本。只在单次调用后统计 usage，无法阻止多个并发请求同时越过每日预算；只使用进程内计数，又无法在多个后端实例之间协调。护栏失败也不能阻断确定性诊断、证据查看与人工处置。

## Decision

Provider 请求显式发送 `max_output_tokens`。服务在调用前按裁剪后提示文本的保守字符估算加最大输出量预留 token，成功后以 Provider 返回的实际 input/output usage 记账，失败不产生解释用量。

每日预算按 UTC 自然日统计。预算预留写入 PostgreSQL 短期表，事务通过固定 advisory lock 串行“清理过期记录、汇总已提交用量与有效预留、检查余额、插入预留”的过程。请求结束后删除预留；进程异常退出由过期时间回收。`AI_DAILY_TOKEN_BUDGET=0` 表示不限额。

单实例通过非阻塞 semaphore 限制 Provider 并发，满载立即返回 HTTP 429 `AI_BUSY`。预算不足返回 HTTP 429 `AI_BUDGET_EXCEEDED`。已登录用户可读取 `/api/v1/ai/status` 查看 Provider 配置、进程内活动请求、当日已用/预留/剩余 token 和最近成功时间。状态接口不替代生成时的原子检查。

## Consequences

- 多实例共享同一预算，不会因为并发读余额而系统性超额放行。
- 并发门控快速失败，不让请求在进程内无限排队。
- 预算估算偏保守，可能在仍有少量余额时拒绝较大上下文；这是限制最坏成本的预期取舍。
- Provider 未返回 usage 的成功响应按现有解析结果记账；生产接入必须选择提供 usage 的兼容实现并监控状态接口。
- 并发上限是单实例配置，不是跨实例全局并发限制；全局 token 预算仍由数据库协调。
