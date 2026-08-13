# Login Panel Enhance：登录界面大气化精修

- Date: 2026-08-13
- Status: Complete
- Scope: 前端登录页右侧面板与卡片放大、辉光舞台化、内部元素精修（`frontend/src/styles/console-theme.css`）

## Context

M106 将登录页改为全屏单一场景后，右侧 `.login-form-panel` 仍为 `min(440px,100vw)`，
`.login-card` 由内容自适应、圆角仅 8px，视觉上"缩在角落"，不够大气。本改动属前端
界面优化并行轨（主题收敛/响应式审计）的一部分，目标：让登录表单成为右侧有视觉重量
的"登录台"，同时保持矮屏与移动端可用。

## What Changed

### 前端登录布局（frontend/src/styles/console-theme.css）

- `.login-page`：`padding-right` 改为 `clamp(440px, 46vw, 720px)`，与右侧面板留白同步。
- `.login-page .login-form-panel`：宽度 `min(440px,100vw)` → `min(560px, max(440px, 46vw))`，
  内边距 `clamp(28px, 3vw, 52px)`。
- `.login-form-panel::before`：背景升级为双径向 + 线性渐变辉光，新增
  `border-left: 1px solid rgba(94,234,212,.14)` 青色边缘强调。
- `.login-card`：显式 `width:min(100%,470px)`、`padding:46px 48px 42px`、圆角 16px、
  `background:rgba(18,31,29,.86)`、阴影 `0 28px 72px -34px rgba(0,0,0,.95)`。
- 新增 `/* ---- M93-B1b ---- */` 精修块：rail 4px、icon 48px、h2 28px、form-hint、
  label 13.5px、login-field 52px/20px 间距、input 50px/15px、password-visibility 36px、
  login-submit 52px/15px/650、security-status 12px。
- 响应式：矮屏断点（max-height:760px）卡片内边距 `34px 36px 32px`；移动端断点
  （max-width:720px）`26px 24px 26px`。

## Verification

- `./node_modules/.bin/vite build`：✓ built in 4.20s，exit 0，无 CSS 语法错误。
- 浏览器 896×714 视口（触发矮屏紧凑模式）：无控制台错误；panel 440×714@x456、
  card 384×540@(484,87) 垂直居中、scrollH=714 无页面溢出。
- 公式推导：≥1080px 宽屏下面板封顶 560px、卡片趋近 470px 设计目标。

## Risks / Notes

- 浏览器视口无法 resize 到 1440×900，宽屏表现基于公式推导，建议后续在宽屏浏览器
  人工复核一次。
- `.login-card` box-shadow 中 `0 0 0 1px rgba(255,255,255,.03) inset` 为散列写法，
  如需统一可折行。
