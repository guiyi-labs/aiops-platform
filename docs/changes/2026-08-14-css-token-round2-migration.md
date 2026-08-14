# CSS Token 第二轮：239+ 旧调色板字面量迁移 + 登录基线确定性修复 + text-muted 收敛

- Date: 2026-08-14
- Status: Complete
- Scope: `frontend/src/styles/base.css` / `console-theme.css` + `scripts/capture-ui-baselines.mjs` + 42 基线产物

## Context

Track A 主题收敛第一轮完成 112 处精确匹配迁移后，`audit-css-tokens.mjs` 报告仍有大量
"orphan 字面量"（无精确 token 匹配的旧调色板值：`#5a6672`×45、`#dfe5e8`×44 等）。
分析发现这些值大多是 base 主题 `:root` 的 token 定义值被 console-theme 覆盖后、
使用点仍硬编码字面量，导致主题级联失效（console 主题的颜色覆盖不到这些元素）。
本轮按语义映射收敛，并顺手修复登录基线的实时状态竞态。

## What Changed

### `frontend/src/styles/base.css`（230 处）
- 按语义 1:1 映射（值 = base token 定义值）：`#5a6672→var(--text-muted)`×45、
  `#dfe5e8→var(--border-soft)`×44、`#e5e9eb→var(--border-subtle)`×10、
  `#f8fafb→var(--bg-secondary)`×10。
- 近邻语义收敛：`#e6eaec→var(--border-subtle)`、`#dce3e6→var(--border-soft)`、
  `#f7faf9→var(--bg-secondary)`、`#cfd8dc→var(--border-default)`、
  `#43515a/#59676e/#526068→var(--text-secondary)`、`#39474e/#344149→var(--gray-700)`、
  `#7c8990/#87938e/#62706b→var(--text-muted)`、`#2e7867→var(--brand-600)`、
  `#9c3c2f→var(--status-danger)`、`#e2e7e9/#e7ebed/#e5eae8→var(--border-subtle)`、
  `#ffffff→var(--gray-0)` 等。

### `frontend/src/styles/console-theme.css`（5 处 + 1 收敛）
- `#f8fafc→var(--gray-0)`×5（侧栏文字等）。
- **`--text-muted` 重复定义收敛**：M93-C `:root` 块内 `--text-muted: #66777d`
  → `#5a6672`（与 base 一致）。修复 wizard-steps 等元素对比度
  （`#66777d` on `#f1f4f5` = 4.22:1 < AA → `#5a6672` = 5.31:1 ✓）。

### `scripts/capture-ui-baselines.mjs`（确定性修复）
- 登录基线竞态根因：`/api/v1/health/live` 在 1400ms settle 窗口内完成与否，导致
  状态条渲染 "检测中/正常" 不定，且掩码只覆盖时钟单元素。修复：
  1) 新文档脚本确定性 stub `health/live` 固定载荷（status=ok/version=dev）；
  2) 登录掩码从时钟 `<b>` 扩为整个 `.login-signal-strip`（实时状态区）。

### 基线重建
- 42 产物（41 PNG + manifest）重建：迁移改变了主题级联计算值（border/text/bg 全部
  走 console 主题 token），整体重捕获后 `--verify` 稳定全绿。

## Verification

- `audit-css-tokens.mjs`：四层 `replaceable=0`；orphan 从 ~265 降至 ~60，其余为
  刻意设计值（登录氛围 `#5eead4`/rgba 光晕、阴影 `#000000`/rgba、通知绿 `#eaf5f1`）。
- **基线 `--verify`**：62 条全绿（3 次连续验证 login 0.000%，掩码区扩至状态条后
  比较基数 1294812→1226490）。
- **axe 32 视图 × 2 视口**：`PASS: 0 critical/serious / 0 app errors`
  （text-muted 收敛修复 promotions wizard-steps 4.22→5.31）。
- **`pnpm ui:gate`**：`=== UI GATE PASS: 4/4 ===`（CSS tokens / Baselines / axe / Bundle）。
- 契约门禁：`pnpm typecheck`、`pnpm lint`、`pnpm test`（141）全绿。

## Risks / Notes

- 主题级联恢复后部分元素计算值从 base 色变为 console 色（如边框 `#dfe5e8→#cfdadd`
  等），视觉有细微变化（均 AA 达标），基线已按新视觉重建；如后续回退主题方向需
  同步重建基线。
- 登录基线 stub 仅作用于基线截图脚本（`Page.addScriptToEvaluateOnNewDocument`），
  不影响真实页面。
