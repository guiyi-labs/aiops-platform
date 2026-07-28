# 2026-07-17 Cited AI Explanations

## Scope

- 新增 `000008_ai_explanations`，追加保存结构化解释、证据引用、Provider/模型、响应 ID和 token 用量。
- 新增默认关闭的 Responses-compatible Provider 配置，远程地址要求 Key，生产环境要求 HTTPS。
- 构建有界诊断上下文，对敏感键脱敏并将持久化证据编号为 `E1...En`。
- 使用 `store=false` 和严格 JSON Schema；服务端二次验证引用 ID和建议优先级。
- 新增解释生成与历史 API；生成仅允许系统/运维管理员，历史对所有登录角色可读。
- AI 禁用、Provider 失败和非法输出使用独立错误码，不影响确定性诊断主链路。
- 生成操作进入平台审计，但审计不保存 API Key、提示上下文或模型正文。
- 诊断详情增加解释历史、建议优先级、证据映射、token 用量和重新生成入口。

## Verification

- `go test ./...` 与服务端构建通过；Provider、结构校验、引用拒绝、脱敏、禁用回退和配置测试通过。
- `pnpm typecheck`、7 个 Vitest 文件共 15 项测试、`pnpm build` 通过。
- PostgreSQL 真实应用 `000008`，解释成功入库并通过历史 API 恢复。
- 本地模拟 Provider 验证 `/responses`、`store=false`、严格 Schema、模型响应 ID和 180/72 token 用量。
- 成功解释包含 `E1` 引用并产生 success 审计；停止 Provider 后返回 502 `AI_PROVIDER_ERROR`，历史数量不变并产生 failure 审计。
- 浏览器验证解释卡片、`E1 · container_state · pod.status.containerStatuses` 映射和重新生成；两次生成保留两条历史。
- 测试结束后删除测试集群、诊断、解释、审计和会话，停止模拟 Provider 与临时前后端。

## Boundaries

- AI 不参与规则命中，不修改诊断证据，不自动执行 Kubernetes 变更。
- 当前同步返回，单次请求超时由 `AI_REQUEST_TIMEOUT` 控制。
- 上下文只来自诊断记录，不读取 Secret、kubeconfig 或任意集群代理接口。
- 模型输出是辅助解释，仍需人工根据引用证据确认。

## Deferred

- Provider 主动健康探测、流式生成和缓存；并发限制与每日 token 预算已在同日运行护栏变更中完成。
- 多 Provider 路由、模型版本评估与提示词离线评测。
- 解释质量人工反馈与按模型汇总已在同日质量评估变更中完成；反馈仍不自动修改规则或提示词。
