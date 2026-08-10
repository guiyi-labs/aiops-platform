# M95：前端统一 Finding 证据组件

- Date: 2026-08-10
- Status: Complete
- Scope: 将 Posture、Optimization、Diagnosis 和 Inspection 接入同一 `FindingDetailV2` 适配器与可折叠证据组件，并补齐桌面/移动端真实数据回归。

## Context

M95 后端已经提供统一 FindingDetail v2、严重度映射、证据引用、建议类型和按资源合并语义。本次增量完成消费侧闭环，避免四个页面继续各自解释 legacy finding、diagnosis timeline 和 inspection result。

## What Changed

### 前端统一模型与组件

- `frontend/src/types/finding.ts`：新增 FindingDetail v2 前端类型、证据引用和建议类型。
- `frontend/src/utils/finding-detail.ts`：新增 analyzer、FinOps、diagnosis、inspection 适配器及按资源/严重度合并逻辑。
- `frontend/src/components/FindingEvidencePanel.vue`：新增可折叠证据链组件，展示规则来源、证据类型/时间/缺失语义和建议能力；默认不展开、不执行动作。
- `frontend/src/views/PostureView.vue`：合并 finding 后保留完整 v2 detail，避免重新生成时丢失来源和证据。
- `frontend/src/views/OptimizationView.vue`：11 个 analyzer tab 和 FinOps recommendation 使用共享面板；移动端 tab/表格横向滚动约束在内容区域内。
- `frontend/src/views/DiagnosesView.vue`、`frontend/src/views/InspectionView.vue`：接入 diagnosis/inspection v2 适配器和共享面板。

### 测试

- `frontend/src/utils/finding-detail.test.ts`：锁定严重度映射、适配器、证据缺失、建议能力和重复证据去重语义。
- `frontend/e2e/diagnosis-timeline.spec.ts`：增加诊断共享证据面板展开断言。
- `frontend/e2e/finding-evidence.spec.ts`：使用真实形状的 Posture、Optimization、Inspection 响应，覆盖 Desktop/Mobile 面板展开和移动端视口边界。

## Verification

- `pnpm typecheck`：通过。
- `pnpm lint`：通过。
- `pnpm test -- --run`：24 个测试文件、135/135 通过。
- `pnpm build`：通过，Vite production bundle 构建完成。
- `pnpm bundle:gate`：通过（entry JS gzip 42.6 kB、total JS gzip 247.6 kB、total CSS gzip 54.8 kB）。
- `pnpm exec playwright test`：56/56 通过（Desktop/Mobile，含 axe 路径）。
- `pnpm exec playwright test e2e/finding-evidence.spec.ts`：6/6 通过（Desktop/Mobile）。
- `pnpm exec playwright test e2e/diagnosis-timeline.spec.ts`：8/8 通过（Desktop/Mobile）。
- `git diff --check`：通过。
- 功能提交：`b0db833341efab48d6f595083db212b57cfb0a0a`；基线 tag：`baseline-m95b-finding-evidence-ui-20260810`。
- 远端 CI：run `31348763940` success（head `b0db833341efab48d6f595083db212b57cfb0a0a`）；main + baseline tag 已同步。

## Risks / Notes

- legacy analyzer finding 的同资源、同严重度重复证据使用稳定证据 ID 去重；原始规则仍保留在 `origin_ids`。
- 前端只消费证据和建议，不新增任意命令、Pod exec、WebShell 或自动执行路径。
- M89 OIDC/MFA 与 M90 WAL/PITR/HA 尚未完成，项目继续保持 RC，不宣称生产身份、生产 HA 或 GA。
