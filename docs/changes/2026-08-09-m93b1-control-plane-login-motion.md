# M93-B1：控制平面登录动效首版

- Date: 2026-08-09
- Status: Complete（M93-B 首版；性能预算仍待 B2）
- Scope: 登录状态驱动动画、ParticleNetwork 生命周期、SVG 节点修复、移动端首屏与专项浏览器回归。

## Context

M92/M93-A 已建立粒子网络、集群拓扑和真实能力卡，但动画仍以持续装饰为主，表单状态与左侧视觉互不关联。
专项截图还暴露一个 SVG 根因缺陷：`.topo-node` 上的 CSS `transform` 动画覆盖了节点自身的
`translate(...)`，六个外围节点实际叠在左上角。

本次以“控制平面唤醒”为首版方向，不增加虚假实时数据，不修改认证 API、安全边界或路由契约。

## What Changed

### 认证状态叙事

- `frontend/src/views/LoginView.vue`：建立 `idle / username / password / submitting / success / error`
  状态，并将状态传给粒子 Canvas、SVG 拓扑、表单 rail 和提交按钮。
- 用户名/密码焦点分别强调不同数据链路；认证中加速流线；成功后短暂确认再进入控制台；错误只做局部颜色反馈。
- 增加密码显隐按钮、字段图标、稳定宽度的按钮状态与安全会话状态行。
- 能力卡悬停会强调对应治理、诊断或审计节点，不把能力卡伪装成可点击功能入口。

### ParticleNetwork 生命周期

- `frontend/src/components/ParticleNetwork.vue`：使用 `ResizeObserver` 跟随容器尺寸，不再依赖全局 resize。
- 页面隐藏时停止 RAF、恢复时继续；运行中切换 `prefers-reduced-motion` 会立即切换静态/动态模式。
- 移动端降低粒子上下限与 DPR 上限；认证阶段调整速度、连线色彩和成功汇聚趋势。
- Canvas 暴露粒子数、运行状态、动效偏好与认证 phase 数据属性，供确定性浏览器回归读取。

### 视觉与响应式

- `frontend/src/styles/console-theme.css`：用结构化线性网格替换光斑式背景，增加表单/拓扑状态动画。
- 将节点呼吸动画下沉到 circle 子元素，保留 SVG group 的 translate 坐标，修复外围节点重叠。
- 移动端固定 230px 品牌区，表单从 208px 进入首屏；390×844 下按钮和安全状态完整可见。

### Browser Regression

- `frontend/e2e/api-fixtures.ts`：增加确定性登录成功响应。
- `frontend/e2e/smoke.spec.ts`：增加焦点 phase、密码显隐、Canvas 非空像素、动态 reduced-motion、
  页面隐藏暂停、容器 resize 和成功转场断言。

### Baseline Documentation

- 同步 `README.md`、`docs/PROJECT_STATUS.md`、`docs/long-term-roadmap.md` 与
  `docs/next-long-term-plan.md`，将本首版记为 M93-B1，并把性能证明收敛为 M93-B2。
- 功能提交为 `e962332`，归档标签为 `baseline-m93b1-20260809`。

## Evidence

- Desktop 1440×900：[`assets/2026-08-09-m93b1-login-desktop.png`](assets/2026-08-09-m93b1-login-desktop.png)
- Mobile 390×844：[`assets/2026-08-09-m93b1-login-mobile.png`](assets/2026-08-09-m93b1-login-mobile.png)
- 浏览器人工复核：无 console error；桌面外围节点坐标分散且与六条链路一致；移动端表单完整进入首屏。

## Verification

- `pnpm lint`：通过。
- `pnpm typecheck`：通过。
- `pnpm test`：23 files / 130 tests 全绿。
- `pnpm build`：通过（Vite 7.3.6，1791 modules，8.72s）；LoginView JS gzip 5.75 kB。
- `pnpm bundle:gate`：通过；entry JS gzip 44.6 kB，largest chunk 21.6 kB，
  total JS gzip 243.9 kB，total CSS gzip 51.0 kB。
- 登录页专项 Playwright：Desktop Chrome + Mobile Chrome 共 10/10 通过。
- 全量 Playwright + axe：Desktop Chrome + Mobile Chrome 共 38/38 通过；匿名 `/login`
  critical/serious = 0；全套 console error = 0。

## Risks / Remaining

- M93-B2 仍需建立可重复的桌面帧率、低端移动设备帧耗与登录页专属 JS/CSS 预算，不在本首版中声称 60fps 已证明。
- 当前能力卡联动为鼠标悬停增强，不改变语义或键盘焦点顺序。
- 成功转场仅在认证成功后增加 520ms 视觉确认；reduced-motion 用户不等待该转场。
