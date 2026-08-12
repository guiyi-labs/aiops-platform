# M94 回放模式：浏览器通道 e2e 闭环（replay-panel 双视口）

- Date: 2026-08-12
- Status: Complete
- Scope: 为诊断详情回放面板补充 Playwright 浏览器通道断言（正常链路 + 降级路径）

## Context

M94 回放模式已具备 API 层证据（demo-drill 17/17 含 replay-before/replay-after），
但浏览器通道（`replay-panel`）此前只有组件级手工验证，`test-matrix.md` 的浏览器证据列
标记为"待后续补充"。本回合补齐 e2e，使"回放"这一功能增量完成 API + 浏览器双通道闭环。

## What Changed

- `frontend/e2e/diagnosis-timeline.spec.ts`：
  - 新增 `replayView` fixture（`aiops.diagnosis-replay/v1`：6 步、4 阶段，含
    diagnosis_created/evidence/activity/remediation，remediation 含 created+executed）。
  - 路由 handler 增加 `GET /api/v1/diagnoses/42/replay` 分支返回 fixture。
  - 新增测试 `Diagnosis replay panel walks the stored insight chain`：断言面板标题
    （`回放模式 · 6 步` + schema）、初始空态、进度条 seek（确定性第 2 步）、上一步、
    阶段筛选（证据采集 1/2 + active chip）、取消筛选回空态（0/6 + 4 阶段 chip）、
    播放/暂停切换、受控动作阶段回溯（remediation detail 可展开）。
  - 新增测试 `Diagnosis replay degrades gracefully when the replay API is unavailable`：
    replay 路由 404 时详情其余部分（root-cause-card）正常渲染，`replay-error` 显示
    "回放模式不可用"；404 资源加载日志作为该异常场景预期副作用在测试内显式清理，
    保持 console-error 零容忍门禁语义。

## Verification

- `pnpm test:e2e diagnosis-timeline`：12/12 PASS（Desktop + Mobile 双视口，
  原 10 项 + 新增 2 项各 ×2）。
- `pnpm test:e2e` 全量：68/68 PASS（20.9s）。
- `pnpm lint`、`pnpm test -- --run`（141）、`pnpm typecheck` 全绿。
- `scripts/scan-sensitive-fields.sh`：clean。

## Risks / Notes

- e2e 走 Playwright route mock（`mockAuthenticatedAPI`），不依赖运行中的后端；
  真实后端行为由 demo-drill 17/17 API 层证据覆盖。
- 阶段筛选取消后 `cursor` 回到 `-1`（空态 0/6），为 composable 既有语义，未改产品逻辑。
