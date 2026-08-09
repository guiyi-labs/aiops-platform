# M94：诊断叙事与证据时间线（第一步：根因卡 + 时间线）

- Date: 2026-08-10
- Status: Complete（M94 第一步；行动区/回放/深链仍为后续增量）
- Scope: 诊断详情新增只读根因卡与证据时间线；证据时间/来源/引用/完整性/缺失语义固化为纯投影。

## Context

M94 的目标是让诊断详情从“字段集合”升级为“10 秒看清根因”的工作界面。
本步交付 ADR + 后端只读投影 + 契约 + 前端渲染，不新增任何写路径：

- 根因卡：主结论、严重度、状态、首次观察、置信来源与关键证据引用。
- 证据时间线：将持久化 evidence 归一化为统一时间轴条目（时间、来源、不可变引用、SHA-256 完整性、缺失语义、摘要）。
- 黄金场景全覆盖：Node NotReady、Deployment unavailable、OOMKilled、Service 无后端。

## What Changed

### ADR

- `docs/adr/0081-diagnosis-evidence-timeline.md`（Status: Accepted）：
  定义 `withNarrative` 纯投影、按证据类型的 `occurred_at` 提取表、六类证据分类
  （resource_state/event/log/alert/change/automation）、完整性与缺失语义、
  根因卡字段与 `key_evidence_refs ≤ 5` 约束。

### 后端（backend/internal/diagnosis）

- `timeline.go`（新增）：`WithNarrative(record) Record` 纯投影；`buildTimeline` 按时间升序
  稳定排序；`extractEvidenceTime` 支持 event（last/first_timestamp）、
  node_condition/pod_condition（last_transition_time + status=Missing 传播）、
  container_termination（finished_at）、metric（window_end），不可解析时回退 observedAt；
  `evidenceIntegrity` 输出 SHA-256；`buildRootCauseCard` 输出结论卡，missing 条目不进入关键引用。
- `model.go`：`Record` 新增 `Timeline []TimelineEntry` 与 `RootCauseCard *RootCauseCard`
  （json `timeline,omitempty` / `root_cause_card,omitempty`）。
- `service.go`：`save()` 与 `Get/Transition/AddFeedback/Assign` 统一返回 `WithNarrative(record)`，
  读接口自动携带叙事投影；持久化记录不变。
- `timeline_test.go`（新增）：四条黄金场景 + missing 条件传播 + 纯投影不变量，共 6 个测试。

### API 契约与前端类型

- `docs/api/openapi.yaml`：新增 `DiagnosisTimelineEntry` 与 `RootCauseCard` 两个 schema（纯增量）。
- `frontend/src/api/openapi.d.ts`：`pnpm typegen` 重新生成（43 行增量）。
- `frontend/src/types/diagnosis.ts`：新增 `DiagnosisTimelineEntry`、`RootCauseCard`，
  `DiagnosisRecord` 增加 `timeline?` / `root_cause_card?`。

### 前端渲染（frontend/src/views/DiagnosesView.vue）

- 详情抽屉顶部渲染根因卡（结论、置信来源、首次观察、关键证据引用）。
- 用“证据时间线”替换默认原始 JSON 证据区；原始证据 JSON 收进可折叠 `<details>`
  （无能力回退）；当 API 未返回 timeline 时保留旧证据卡渲染作为回退。
- `frontend/src/styles/base.css` 与 `console-theme.css`：根因卡与时间线条目样式，
  沿用现有卡片/时间线模式并补移动端 1fr+auto 降级。

### 浏览器旅程

- `frontend/e2e/diagnosis-timeline.spec.ts`（新增）：mock 一条 Node NotReady 详情，
  断言根因卡结论/置信来源/关键引用可见、时间线两条目升序、原始证据折叠可见；
  Desktop + Mobile 双视口通过。

## Verification

- `go test ./internal/diagnosis/ ./internal/httpserver/`：通过（含 6 个新 golden 测试）。
- `pnpm typegen`：通过（openapi.d.ts 增量生成）。
- `pnpm typecheck`：通过。
- `pnpm lint`：通过。
- `npx playwright test`：44/44 通过（原 42/42 回归 + 新增 2/2 诊断旅程）。
- `git diff --check`：通过。

## Risks / Notes

- 时间线为只读投影：`WithNarrative` 按值投影，不回写、不触达集群；列表接口不携带 timeline，
  由详情接口按需生成。
- 缺失语义：仅当证据对象显式报告缺失（如 Ready Condition status=Missing）才标记 missing；
  时间戳不可解析回退 observedAt，不误报缺失。
- 本步覆盖 M94 的“根因卡 + 证据时间线”两环；行动区、回放模式与深链作为 M94 后续增量，
  在新的 change-record 中归档。
- 诊断详情响应仍是通用 `Ok` schema；新增 schema 为契约文档增量，未来接死具体响应时再补引用。