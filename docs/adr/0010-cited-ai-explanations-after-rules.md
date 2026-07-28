# ADR 0010: Cited AI Explanations After Deterministic Rules

- Status: Accepted
- Date: 2026-07-17

## Context

规则诊断已经提供可复现结论和证据，但自然语言解释与处置建议仍较固定。直接让模型读取任意集群数据、生成无引用结论或参与规则事务，会扩大凭据泄露、幻觉和外部服务故障的影响范围。

## Decision

AI 只作为规则诊断后的显式解释步骤。Provider 实现 Responses-compatible `POST /responses`，请求设置 `store=false`，输出由严格 JSON Schema 约束。默认关闭；启用远程 Provider 时 Key 只来自环境变量，生产地址必须为 HTTPS。本地 loopback Provider 可无 Key，支持离线模型和可复现测试。

上下文只包含已持久化的规则摘要、根因假设、建议和证据。证据获得稳定的本次请求 ID；敏感键递归脱敏，单项和总输入有界。模型必须返回证据引用，服务端再次校验 ID，未知或空引用使整次结果失败且不入库。

解释历史只追加，保存操作者、Provider、模型、响应 ID、token 用量和结构化结果。API Key、完整提示词、上游错误正文和模型隐藏推理不保存。AI 不改变规则命中、证据、状态、负责人或 Kubernetes 资源。

接口形态参考 OpenAI 官方 [Responses API 指南](https://developers.openai.com/api/docs/guides/migrate-to-responses) 与 [Structured Outputs 指南](https://developers.openai.com/api/docs/guides/structured-outputs)。

## Consequences

- Provider 故障不会阻断诊断详情和人工处置。
- 每条关键解释可映射回本次持久化证据，前端能显示引用来源。
- 重新生成会产生新历史，便于比较模型或提示变化。
- 当前只支持同步文本解释；并发与每日 token 预算由 ADR 0011 补充，尚无流式输出、缓存和多 Provider 路由。
- 解释质量的追加式人工反馈和模型汇总由 ADR 0012 补充。
