# M93-B1.1：全屏登录页细节打磨

- Date: 2026-08-09
- Status: Complete
- Scope: 全视口场景、暗色悬浮表单、移动端首屏、自动化布局边界与视觉证据。

## Context

M93-B1 已完成认证状态联动与 ParticleNetwork 生命周期，但页面仍是左侧深色视觉区、右侧浅色表单区，
视觉被明确切成两半。用户要求改为全屏体验，并继续细化表单与响应式细节。

本次只调整登录页布局与视觉表达，不修改认证 API、数据真实性、安全边界或成功转场时序。

## What Changed

### Fullscreen Scene

- `frontend/src/styles/console-theme.css`：登录页改为单一全视口深色场景；`.login-intro` 绝对覆盖整个视口，
  ParticleNetwork、结构化网格和 SVG 拓扑不再局限于左半屏。
- 桌面端为表单保留右侧操作带，使用透明到深色的纵向层次和轻量模糊，不再出现白色半屏。
- 左侧文案、拓扑和能力卡使用固定内容宽度与高度约束；1440×900 下无重叠，1280×720 下使用紧凑高度规则。

### Dark Form Refinement

- 登录卡切换为暗色半透明表面，统一标题、标签、提示、输入框、密码显隐按钮和安全状态的颜色语义。
- 增加浏览器自动填充的暗色覆盖，避免保存账号后输入框突然变成浅色。
- 保留认证中 rail、成功确认和错误局部反馈，并将主操作色收敛为青绿色，和控制平面链路一致。

### Mobile First Viewport

- 390×844 下保留品牌、标题与全屏粒子背景；隐藏 SVG 拓扑和能力卡，避免它们出现在表单背后。
- 表单从 210px 开始，卡片、按钮和安全状态完整位于首屏；页面允许更矮设备自然纵向滚动。
- 修复 `.login-intro > *` 高优先级定位规则覆盖移动端标题绝对定位的问题。

### Browser Regression

- `frontend/e2e/smoke.spec.ts`：验证 `.login-intro` 与视口尺寸误差不超过浏览器滚动条容差；
  登录卡完整落在首屏；移动端 `.login-visual` 隐藏、桌面端可见。
- 匿名登录页 Desktop/Mobile axe 扫描继续保持 critical/serious = 0。

### Baseline Documentation

- 同步 `README.md`、`docs/PROJECT_STATUS.md`、`docs/long-term-roadmap.md` 与
  `docs/next-long-term-plan.md`，将本轮记为 M93-B1.1，M93-B2 继续只承担性能证据。
- 功能提交为 `51ab90d`，归档标签为 `baseline-m93b1-fullscreen-20260809`。

## Evidence

- Desktop 1440×900：[`assets/2026-08-09-m93b1-fullscreen-desktop.png`](assets/2026-08-09-m93b1-fullscreen-desktop.png)
- 浏览器复核：桌面场景覆盖完整视口，表单与左侧内容边界分离；移动端表单边界为 y=210–673px。

## Verification

- `pnpm lint`：通过。
- `pnpm typecheck`：通过。
- `pnpm test`：23 files / 130 tests 全绿。
- `pnpm build`：通过（Vite 7.3.6，1791 modules，8.24s）；LoginView JS gzip 5.75 kB。
- `pnpm bundle:gate`：通过；entry JS gzip 44.6 kB，largest chunk 21.6 kB，
  total JS gzip 244.0 kB，total CSS gzip 51.5 kB。
- 登录页专项 smoke + axe：Desktop Chrome + Mobile Chrome 共 12/12 通过。
- 全量 Playwright + axe：Desktop Chrome + Mobile Chrome 共 38/38 通过；console error = 0。

## Risks / Remaining

- M93-B2 的低端设备帧耗与登录页专属 JS/CSS 预算仍未完成，本次不新增 60fps 声明。
- 移动端主动隐藏拓扑和能力卡，但能力文案仍保留在 DOM 中并由 smoke 测试防止硬编码指标回归。
