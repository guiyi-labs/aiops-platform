# Login Short-Height Adaptation：短屏纵向适配（第 15 轮视觉打磨）

- Date: 2026-08-13
- Status: Complete
- Scope: 前端登录页短视口纵向适配（`frontend/src/styles/console-theme.css`，新增 M93-B2 块）

## Context

第 14 轮修复移动端横向出屏后，用 headless Chromium CDP 对短视口边界做实测，
发现两处纵向裁切：

- **短屏手机**（320×568 / 360×640 / 375×667 等，高 ≤ 780px）：卡片固定高 542px，
  移动端面板 `padding-top:210px` 把卡片顶出视口底部，输入框/按钮被裁切
  （320×568 时底部出屏 185px）。
- **矮屏横向**（896×414 等，宽 ≥ 721px 且高 ≤ 660px）：`.login-page` 的
  `overflow:hidden` + 面板绝对定位居中，卡片高 556px 相对 414px 视口上下各溢出
  71px，卡片头部（标题区）被裁且无法滚动。

## What Changed

新增两段媒体查询（M93-B2 块，位于 M93-C 之前）：

1. `@media (max-width: 720px) and (max-height: 780px)`（短屏手机）：
   - 隐藏 `.login-intro > .login-brand` 与 `.login-intro > .login-copy`（文案层让位）。
   - `.login-page .login-form-panel` 内边距改为 `18px 14px 14px`、`place-items: start center`。
   - `.login-card` 及内部间距紧凑化（padding `20px 22px 18px`、`.context-label` 14px、
     `.form-hint` 上下 8/16px、`.login-field` 12px、`.login-security-status` 10px），
     卡片净高 542→472px。
2. `@media (max-height: 660px) and (min-width: 721px)`（矮屏横向）：
   - `.login-page { overflow-y: auto }` 解除纵向裁切。
   - `.login-page .login-form-panel { place-items: safe center; padding: 16px 0 }`
     ——溢出时顶部对齐、必要时可滚动。
3. `@media (max-height: 560px) and (min-width: 721px)`（极端矮宽）：
   - 隐藏已完全出屏的 `.login-visual` / `.login-signal-strip`，减少屏外杂项。

纯样式层，未触碰表单逻辑、无障碍语义与桌面断点。

## Verification（headless Chromium CDP 实测线上容器）

- 修复前：320×568 卡片底部出屏 185px、360×640 出屏 113px、896×414 上下各 71px。
- 修复后：

| 视口 | 类型 | card frame | 输入框 | 结论 |
|---|---|---|---|---|
| 320×568 | 短屏手机 | `15,19 290x472`（bottom 491） | y 315..365 | ✓ 完整入屏 |
| 360×640 | 短屏手机 | `15,19 330x472` | y 315..365 | ✓ |
| 375×667 | 短屏手机 | `15,19 345x472` | y 315..365 | ✓ |
| 720×500 | 短宽 | `15,19 690x472` | y 315..365 | ✓ |
| 896×414 | 矮屏横向 | 顶部可见（y 17）+ 可滚动 | y 363..413 | ✓ 可滚动不裁切 |
| 1024×600 | 矮屏横向 | `580,22 387x556` | y 368..418 | ✓ 完整入屏 |
| 375×812 / 414×896 | 常规手机 | `15,211 345/384x542`（与第 14 轮一致） | ✓ | ✓ 无回归 |
| 1440×900 / 1280×720 / 768×1024 / 1920×1080 | 桌面/平板 | 几何与第 13–14 轮完全一致 | ✓ | ✓ 无回归 |

- 全断点 `overflowX=false`；产出 `index-D7mHHrJV.css + index-hNzGr2t3.js` 已
  `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久）。
- 构建：`./node_modules/.bin/vite build` ✓ 2.37s。

## Follow-up

- 320px 以下（含 280px 折返屏）未实测；若需支持可继续压缩卡片水平 padding。
- 896×414 采用"顶部对齐 + 滚动"而非压缩卡片，属有意的边界取舍。
