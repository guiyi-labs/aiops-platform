# UI 截图基线机制（Track A · 关键页面截图基线 · 登录页先行）

- Date: 2026-08-13
- Status: Complete
- Scope: `scripts/capture-ui-baselines.mjs` + `docs/ui-baselines/`（登录页 Desktop/Mobile 基线）

## Context

Track A（前端优化轨收口）要求建立"关键页面截图基线 + 像素容差"的视觉回归机制
（复用 M96 基线思路；登录页 13–15 轮打磨成果一并纳入）。此前仓库仅有
`scripts/capture-thesis-screenshots.mjs`——其为一次性 thesis 截图，不提供"重截图 →
像素对比 → 断言"的可重复验证。本改动补上可入 CI 的确定化截图基线。

## What Changed

### 新脚本 `scripts/capture-ui-baselines.mjs`
- CDP 驱动 headless Chrome（macOS/Edge 自动探测，`AIOPS_BROWSER_PATH` 可覆盖），
  双关闭 `--verify`：
  - 默认：按 `views` + `viewportSet` 捕获 PNG 到 `docs/ui-baselines/images/`，写
    `manifest.json`（schema/commit/viewport/sha256/掩码/阈值）。
  - `--verify`：重截图到 `.tools/` 临时目录（不覆盖基线），sha256 相同即 IDENTICAL，
    否则 `sips` 转 BMP 后逐像素对比（跳过掩码区），diff 比例 ≤ `AIOPS_UI_DIFF_THRESHOLD`
    （默认 0.002）即 PASS，否则 FAIL 且退出码 1。
- **确定性保证**（可重复对比的三层手段）：
  1. `Emulation.setEmulatedMedia` 仿真 `prefers-reduced-motion: reduce`，CSS 动画与
     粒子画布循环复位（`ParticleNetwork` 尊重 reduced motion）。
  2. `Page.addScriptToEvaluateOnNewDocument` 注入固定种子 `Math.random`，使
     `spawnParticle` 等随机初始位置每轮渲染一致（修复画布抖动导致的 ~0.7–2.9% 差异）。
  3. `masks` 动态区掩码：本地时钟文本 DOM 矩形记录进 manifest，对比时跳过
     （登录页掩码 `.login-signal:nth-child(3) b`）。
- 视口：Desktop 1440×900 / Mobile 375×812（`viewportSet` 可扩展）。

### 基线产物 `docs/ui-baselines/`
- `images/login-desktop-1440x900.png`、`images/login-mobile-375x812.png`
  （登录页第 15 轮状态，commit 0da3462）。
- `manifest.json`（两档条目，含掩码、sha256）。
- `README.md`（用法、确定性与扩展说明）。

## Verification

- `node scripts/capture-ui-baselines.mjs`：捕获两档成功。
- `node scripts/capture-ui-baselines.mjs --verify`（同环境重截对比）：

| 视口 | 结果 | 详值 |
|---|---|---|
| login@1440x900 | PASS | diff 0.000%，0/1,294,812 px，maxΔ=0 |
| login@375x812 | IDENTICAL | 两次 sha256 一致 |

- 中途曾发现粒子画布随机初始位置导致 0.7–2.9% 像素差异，注入确定性 `Math.random`
  后归零；验证窗口未覆盖任何基线文件（verify 写 `.tools/` 临时目录）。
- 未触碰 `LoginView.vue` 逻辑、无障碍语义与任何线上样式。

## Follow-up

- 结合 `docs/polish-plan.md` P0–P3 与 Track A：将 incident 列表/详情、告警、巡检、
  信号、工作负载、拓扑各视图陆续纳入 `views`（每页需配 `readyText` 与鉴权登录流程）。
- 后续可把 `--verify` 接入 CI job（`pnpm`/node 门禁），对线上镜像产物做像素回归。
- 跨平台对比把 `sips→BMP` 换成纯 Node PNG 解码后，Windows CI 亦可用同一断言。
