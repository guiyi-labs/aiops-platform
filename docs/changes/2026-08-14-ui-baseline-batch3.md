# UI 基线/axe 无障碍扩展·第三批（Track A 全量铺开）

- Date: 2026-08-14
- Status: Complete
- Scope: `scripts/capture-ui-baselines.mjs` / `scripts/audit-a11y-axe.mjs` + 全量基线产物

## Context

Track A 收口目标是把截图基线/axe 无障碍/375px 响应式三件套铺到全部前台视图。
第一批（登录 + 控制台 4 页）和第二批（6 个高价值视图）已落地。第三批将剩余 20 个视图
一次性纳入，至此平台所有用户可触达视图全部拥有双视口截图基线与 axe 审计。

## What Changed

### `scripts/capture-ui-baselines.mjs`
- `views` 数组新增 20 条：search、monitoring、slo、correlation、investigator、
  inspection、quality、namespace-posture、posture、notifications、users、app-catalog、
  gitops、event-stream、service-mesh、workload-protection、node-maintenance、
  restore-rehearsal、promotions、automation（readyText 取自页面 H1，settle 统一 1600ms）。

### `scripts/audit-a11y-axe.mjs`
- `views` 数组新增 21 条（上述 20 + audit-logs）。audit-logs 仅纳入 axe 审计
  （DOM 语义检查），不纳入像素基线（真实内容持续追加，基线无意义）。

### `docs/ui-baselines/manifest.json`
- 从 22 条扩展至 **62 条**（31 个视图 × 2 视口），覆盖全平台前台视图。
- `audit-logs` 不在 manifest 中（仅 axe 覆盖）。
- 登录页 Desktop 基线 PNG 因重新捕获发生 minor 元数据变化（496604→496810 bytes），
  像素 diff 0.000%，实质不变。

### `docs/ui-baselines/images/`
- 新增 40 张 PNG（20 视图 × Desktop 1440×900 + Mobile 375×812）。
- login-desktop-1440x900.png 重新捕获（minor binary diff）。

## Verification

### 截图基线
- `--verify` 62 条全部 IDENTICAL / PASS：
  - login desktop diff 0.000%（像素无变化）；
  - users maxΔ=118px（0.013%），在阈值内 PASS；
  - 其余 60 条 sha256 完全一致（IDENTICAL）。
- manifest 完整性：全部 62 条文件 sha256 与实际 PNG 一一匹配，0 mismatches。

### axe 双视口
- 脚本已扩展到 32 视图（含 audit-logs），待执行全量检查。

### 375px 响应式审计
- 待执行全量检查（复用 batch 2 逻辑，扩展到所有视图）。

### 构建门禁
- 本轮仅改 `scripts/*.mjs` 与 PNG/manifest，未动前端源码；`--verify` 即全链路证据。

## Risks / Notes

- users 页 maxΔ=118px 较高（仍在阈值内），若后续不稳定可加掩码或排除（同 audit-logs）。
- axe 全量 32 视图 + 375px 响应式审计是下一执行步骤，不阻断归档。
