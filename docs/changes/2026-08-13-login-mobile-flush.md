# Login Mobile Flush：移动端登录卡片出屏修复（第 14 轮视觉打磨）

- Date: 2026-08-13
- Status: Complete
- Scope: 前端登录页移动端断点修复（`frontend/src/styles/console-theme.css`，`@media (max-width: 720px)`）

## Context

第 13 轮将 `.login-form-panel` 改为桌面端"右区内水平居中"（`inset: 0 calc((clamp(460px,49vw,760px) - min(540px,max(430px,44vw)))/2) 0 auto`，
选择器 `.login-page .login-form-panel`，特异性 0,2,0）。移动端媒体查询中的同名规则

```css
.login-form-panel { inset: 0; width: 100%; ... }
```

特异性仅 0,1,0，**被桌面规则压掉**（两级级联坑的又一实例）：375px 视口下面板仍按桌面
`calc` 公式定位，实测 `x=-70, w=430`，卡片左缘出屏 16–42px，右侧留约 43px 空档，
输入框左缘 `x=-16` 被裁切。

## What Changed

- `@media (max-width: 720px)` 内 `.login-form-panel` 选择器提升为
  `.login-page .login-form-panel`（特异性 0,2,0，与桌面规则同级且位于其后 → 生效覆盖）。
- `max-width: none` 收紧为 `max-width: 100%`，配合 `width: 100%; inset: 0; padding: 210px 14px 24px;`
  保证任意窄屏（含 320px）下面板贴齐视口、卡片两侧保留 14px 安全边距。
- 未触碰表单逻辑、无障碍语义与桌面端任何规则。

## Verification（headless Chromium CDP 实测线上容器）

修复前（375×812）：`.login-form-panel` rect `-70,0 430x812`，输入框 `x=-16` 出屏。
修复后：

| 视口 | form panel | card frame | 输入框 | 桌面基线 |
|---|---|---|---|---|
| 375×812 | `0,0 375x812` | `15,211 345x542` | `40..335` | – |
| 414×896 | `0,0 414x896` | `15,211 384x542` | `40..374` | – |
| 768×1024 | `323,0 430x1024` | `352,223 372x578` | 入屏 | ✓ |
| 1440×900 | `817,0 540x900` | `861,161 452x578` | 入屏 | ✓ 与第 13 轮一致 |
| 1920×1080 | `1270,0 540x1080` | `1323,251 434x578` | 入屏 | ✓ 与第 13 轮一致 |

- 全断点 `document.documentElement.scrollWidth == clientWidth`，无横向滚动。
- 无元素互相重叠（visual × form 间隙 ≥ 26px）。
- 线上产物：`index-CFVWN5sg.css + index-DSCHtAIR.js`（`docker cp` 已覆盖
  `k8s-aiops-frontend-1:/usr/share/nginx/html/`，非持久，容器重建回退）。
- 构建：`./node_modules/.bin/vite build` ✓ 2.75s。

## Follow-up

- 移动端断点其余规则（copy 定位、卡片内边距）在 320px 等更窄宽度尚未实测；
  建议后续补齐 320/360 宽 + 短高（568px）边界并纳入响应式审计清单
  （Track A · ≤720px 响应式审计）。
