# M94（第二步）：诊断行动区（只读建议 / 受控动作）

- Date: 2026-08-10
- Status: Complete（M94 第二步；回放模式与深链仍为后续增量）
- Scope: 诊断详情新增类型化“行动区”，区分只读建议与需要 dry-run+确认的受控动作；无权限/依赖不可用显式降级。

## Context

M94 第一步（根因卡 + 证据时间线）已交付。本步完成 M94 范围的“行动区”：
把诊断的建议区从无差别的文本列表升级为类型化动作区——

- 只读建议（advisory）不需要确认；
- 受控动作（controlled_action）必须经过 Kubernetes dry-run 与显式人工确认才能执行；
- 无权限、非 confirmed 状态等不可用原因在 UI 中显式说明，不以禁用按钮掩盖。

约束不变：不新增任意命令、Pod exec、WebShell 或绕过确认的写操作；受控动作仍复用既有
受控修复管线（M19 dry-run / M81 闭环）。

## What Changed

### 后端（backend/internal/diagnosis）

- `actions.go`（新增）：`DiagnosisAction`（kind / title / detail / action /
  requires_dry_run / requires_confirmation）与 `buildActionArea(record)` 纯投影——
  每条 recommendation → `advisory` 动作；Pod 资源追加 `deployment.rollout_restart`
  `controlled_action`（dry-run + confirmation 均为 true）。
- `model.go`：`Record` 新增 `Actions []DiagnosisAction`（json `actions,omitempty`）。
- `timeline.go`：`WithNarrative` 追加 `record.Actions = buildActionArea(record)`。
- `actions_test.go`（新增）：Pod 含受控能力、Service 仅 advisory、Node 仅 advisory、
  空建议 Pod 仍暴露受控能力等 4 个测试。

### API 契约与前端类型

- `docs/api/openapi.yaml`：新增 `DiagnosisAction` schema（纯增量）。
- `frontend/src/api/openapi.d.ts`：`pnpm typegen` 重新生成。
- `frontend/src/types/diagnosis.ts`：新增 `DiagnosisActionKind`、`DiagnosisActionItem`，
  `DiagnosisRecord` 增加 `actions?`。

### 前端渲染（frontend/src/views/DiagnosesView.vue）

- 详情抽屉新增“行动区”区块：每个动作按 kind 标记（只读建议 / 受控动作），
  受控动作显示 dry-run + 确认说明。
- 当存在受控动作但当前不可用（无权限 / 非 confirmed 状态）时显示降级提示条
  （permission / dependency），如实说明原因；remediation-form 仍只在
  canManage && confirmed && Pod 时出现。
- `frontend/src/styles/base.css` 与 `console-theme.css`：行动区条目、kind 徽章、
  降级提示样式，含移动端 1fr 布局。

### 浏览器旅程

- `frontend/e2e/diagnosis-timeline.spec.ts`：Node 详情断言行动区含只读建议；
  新增 Pod confirmed 场景断言受控动作显示 + open 状态依赖降级提示 + 无 remediation-form。

## Verification

- `go test ./internal/diagnosis/ ./internal/httpserver/`：通过（含 4 个新 action 测试）。
- `pnpm typegen`：通过（DiagnosisAction schema 增量）。
- `pnpm typecheck`、`pnpm lint`：通过。
- `npx playwright test`：46/46 通过（原 44/44 + 新增 2/2 行动区旅程，Desktop+Mobile）。
- git diff --check：通过。
- 远端 CI：run 31332489433 success（head afecc146c84cb00db1850c81a22a52614f79e17）；main + baseline tag 已同步。

## Risks / Notes

- 行动区为只读投影：`buildActionArea` 只读持久化 record，不触达集群、不依赖调用者会话。
- 受控能力映射仅面向 Pod（rollout restart）；其他资源类型当前只提供 advisory，后续按需扩展。
- M94 剩余增量：回放模式与深链，将在新的 change-record 中归档。