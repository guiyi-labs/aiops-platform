# M112-2 会话式 AI 调查：事故上下文中引用校验的连续问答

- Date: 2026-08-14
- Status: Complete
- Scope: M112 第二个切片——事故上下文中会话式 AI 调查，每个回答引用校验 fail-closed，AI 不可用时确定性降级。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track C（M112 AI 协调查询与解释深化，P0）。
- M112-1 已完成事故上下文驾驶舱并落地跨里程碑资源上下文契约；本切片是在同一事故工作面上提供连续问答能力的 P0 项。
- 采纳 M44 aiinvestigator 的引用校验纪律（每条断言引用授权证据 ID；未授权引用 → fail-closed），但范围改为 incident 证据时间线，不是 correlation case。

## What Changed

### 后端
- `backend/internal/incidentchat/`（新增包）：M112-2 会话式 AI 调查服务，无持久化（客户端持有历史对话，请求携带 bounded 历史记录；每个请求状态独立，不写数据库）。
  - `model.go`：ChatRequest/ChatResponse 类型、ResourceContext 契约块、Provider 接口、ErrBusy/ErrNoMessages/ErrLastMessageNotUser/ErrHistoryTooLong。
  - `prompt.go`：BuildPrompt——将事故快照 + 证据时间线组装为授权证据集（incident:<id> + evidence:<source_ref>），system prompt 规定 JSON 输出（answer + next_checks + citations），只允许引用授权证据 ID；ValidateResult 逐条校验 citations.evidence_id 在授权集内、禁止 prompt injection、边界约束（citations ≤ 64, next_checks ≤ 8）；PromptHash 提供稳定哈希（黄金 fixture 回放一致性）。
  - `provider.go`：NopProvider（AI 禁用时确定性降级：引用 incident 记录，输出无推断）；ResponsesProvider（OpenAI-style responses API，json_schema strict 模式输出）。
  - `service.go`：Chat——provider 不可用时自动切 NopProvider 降级（返回 mode=deterministic）；provider 成功但引用校验失败时同样 fail-closed 降级（fail_closed=true）；返回 ResourceContext 契约块（scope/observed_at/source/freshness/empty_sample=fail_closed）。
  - `service_test.go`：覆盖授权证据 ID 构建、NopProvider 优先引用 incident ID、确定性降级路径、空消息/历史过长/最后消息必须是 user、PromptHash 稳定性、DecodeProviderJSON 有效/缺字段/畸形。
- `backend/internal/httpserver/incidentchat.go`（新增）：`POST /api/v1/incidents/:incident_id/chat` handler，AuditAction `incident.chat.create`，input normalization、404 检查、429（ErrBusy）→ 503，历史记录消息 trim（忽略空内容）。
- `backend/internal/httpserver/router.go`：新增 `Options.IncidentChat *incidentchat.Service` 字段；当非 nil 时注册 chat 路由。
- `backend/internal/httpserver/router_harness_test.go`：路由契约测试覆盖 chat 路由（NopProvider + incident.NewService(nil) 组合）。

### OpenAPI / 权限矩阵 / 前端
- `docs/api/openapi.yaml`：新增 `POST /api/v1/incidents/{incident_id}/chat`（operationId `incidentAIChat`）、`IncidentChatRequest`、`IncidentChatMessage`、`IncidentChatResponse`（含 resource_context 契约块、mode、citations、fail_closed）。
- `docs/security/permission-matrix.md`：自动重生成，登记 `incident.chat.create`。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。
- `frontend/src/types/incident.ts`：新增 `IncidentChatMessage`、`IncidentChatCitation`、`IncidentChatResponse`。
- `frontend/src/api/incidents.ts` + `incidents.test.ts`：新增 `sendIncidentChat` 客户端与测试（POST 调用验证）。
- `frontend/src/views/IncidentsView.vue`：事故详情抽屉新增「会话式 AI 调查」区块（会话气泡、输入框、状态指示），打开详情时清空对话；支持按 Enter 发送。

## Verification

- `go test ./...`：全绿。
- `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（151 通过）/ `pnpm build`：全绿。
- OpenAPI 路由匹配 + 权限矩阵一致性：通过。
- 未增加新 DB migration（状态性由客户端持有）。
- 确认：AI 禁用路径始终引用真实 incident 记录 ID，不伪造根因；引用校验失败时 fail_closed=true 且降级为确定性模式。

## Risks / Notes

- 对话状态由客户端持有；后续若需服务器端历史持久化（分析、审计追溯），需新增 migration 000049。当前切片满足路线验收（连续提问、引用校验 0 泄漏、确定性降级）。
- `IncidentChat.IncidentService` 在 `router_harness_test.go` 通过 `incident.NewService(nil)` 创建（测试仅覆盖路由注册，不需要数据库）。
- M110 RC-6 发布仍待用户授权；本切片不依赖发布动作。