# M112-3 引用式 AI 事故摘要：确定性阶段门 + 引用校验的自动摘要

- Date: 2026-08-14
- Status: Complete
- Scope: M112 第三个切片——事故自动摘要（根因候选/影响/证据摘要/下一步），输出全部引用事故证据时间线，阶段门与引用校验双重 fail-closed，AI 不可用时确定性降级。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track C 第 141–142 行：确定性阶段门 + 引用校验的自动摘要；验收含「引用校验 0 泄漏；Provider 故障/关闭时确定性降级路径可用；门禁全绿」。
- 复用 M112-1 确立的资源上下文契约（resource_context 块）与 M112-2 的 incidentchat 基础设施（授权证据集、引用校验、NopProvider 降级、IncidentReader adapter），未新增第二个包装层。

## What Changed

### 后端：`backend/internal/incidentchat/`
- `summary.go`（新增）：
  - `SummaryResponse`：incident_id、resource_context 契约块、mode(`ai`/`deterministic`)、root_cause_candidate、impact、evidence_summary、next_steps、citations、provider/model/tokens、fail_closed、stage_gate_passed、stage_gate_reason。
  - `SummaryStageGate(hasEvidence, aiEnabled)`：纯函数阶段门——无证据或 AI 禁用则拒绝调用 provider（确定性）；返回理由 `no_evidence`/`ai_disabled`/`ok`。
  - `BuildSummaryPrompt`：把事故快照 + 证据时间线组装为一个一次性摘要 prompt（同 M112-2 授权证据 ID 纪律：`incident:<id>` + `evidence:<source_ref>`）。
  - `summaryProviderResult` + `DecodeSummaryProviderJSON`：解析 provider JSON。
  - `ValidateSummaryResult`：逐条校验 citations.evidence_id 在授权集内（未授权引用 → ErrCitationRejected）、summary 三字段非空、citations ≤ 64、next_steps ≤ 6、prompt-injection 拒收。
  - `SummaryProvider` 接口 + `NopSummaryProvider`（AI 禁用/降级时确定性输出：明确「未做真实 AI 分析」，只引用真实 incident 记录）+ `ResponsesSummaryProvider`（OpenAI-style responses API，独立 `incident_summary` strict JSON schema）。
- `service.go`：Service 增加 `summaryProvider`/`model` 字段；`Summarize(ctx, incidentID, observedAt)`——阶段门未通过 → 确定性返回（StageGatePassed=false）；provider 失败 → 确定性降级（fail_closed=true）；引用校验失败 → 确定性降级（fail_closed=true）；成功 → mode=ai。并发限制与 Chat 共享 semaphore。
- `service_test.go`：新增阶段门规则、无证据/禁用 AI 确定性、provider 故障降级、AI 成功路径、越权引用 fail-closed、授权集构建、DecodeSummaryProviderJSON 有效样例、空 root_cause 拒绝。

### 后端：HTTP 层
- `internal/httpserver/incidentchat.go`：新增 `incidentChatHandler.summary`——GET 只读路由 `AuditAction incident.summary.read` / `AuditResource IncidentSummary`；404/503/500 错误映射与 chat 一致。
- `internal/httpserver/router.go`：`Options.IncidentChat` 非 nil 时注册 `GET /api/v1/incidents/:incident_id/summary`。
- `internal/httpserver/incidents_test.go`：`TestIncidentSummaryHandler_DeterministicStageGate`（无证据 → 阶段门阻断 → deterministic）、`TestIncidentSummaryHandler_NonExistentIncident404`。
- `internal/httpserver/router_harness_test.go`：路由契约测试自动覆盖 summary 路由。

### OpenAPI / 权限矩阵 / 前端
- `docs/api/openapi.yaml`：新增 `GET /api/v1/incidents/{incident_id}/summary`（operationId `incidentSummary`）与 `IncidentSummaryResponse` schema（含 resource_context 契约块、stage_gate_passed/reason、citations）。
- `docs/security/permission-matrix.md`：重生成，登记 `incident.summary.read`（GET /api/v1/incidents/:incident_id/summary）。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。
- `frontend/src/types/incident.ts`：新增 `IncidentSummaryResponse`。
- `frontend/src/api/incidents.ts`：新增 `getIncidentAISummary(token, incidentID)`（为避免与既有 metrics `getIncidentSummary(token)` 冲突，命名区分）。
- `frontend/src/views/IncidentsView.vue`：事故详情抽屉新增「AI 事故摘要」区块——阶段门状态、mode 徽标、根因候选/影响/证据摘要/下一步列表、引用证据（evidence_id + claim）、fail-closed 提示；打开详情时并行加载。

## Verification

- `go test ./...`：68 个包全绿，无 FAIL。
- `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（151 通过）/ `pnpm build`：全绿。
- OpenAPI 路由匹配 + 权限矩阵一致性：通过。
- 引用校验 0 泄漏：`TestSummarize_UnauthorizedCitationFailsClosed` 确认越权引用整体降级为确定性输出；prompt-injection 标记拒收。
- 无数据库迁移（纯只读聚合 + 无状态 AI 调用）。

## Risks / Notes

- 摘要每次请求现算（无缓存）；若后续需要为相同 incident + 相同证据生成结果去重，可复用 M112-2 的 `PromptHash` 或引入 `incident_summary` 记录表（migration 000049）。
- 模型输出 token 数当前无 provider 层回传（responses API 未解析 usage）；`input_tokens`/`output_tokens` 前端展示默认 0，后续 M112-4 大盘如需精确 token 度量可补解析。
- M110 RC-6 发布仍待用户授权；M89/M90 授权 Track 保持 Deferred，不阻塞本切片。