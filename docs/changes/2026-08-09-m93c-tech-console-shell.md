# M93-C：科技主题控制台与无白屏导航

- Date: 2026-08-09
- Status: Complete
- Scope: 登录页与控制台统一工业科技主题、侧栏持久化折叠、路由白屏消除及双视口回归。

## Context

M93-B1.1 已把登录页扩展为全屏场景，但视觉仍偏柔和青绿色，登录后的控制台继续使用旧企业蓝主题，
两处产品语言不一致。用户同时反馈侧栏无法收回，以及点击功能入口时页面会短暂全白。

本次不改认证 API、路由权限、业务数据契约或登录前数据真实性边界；M93-B2 的低端设备帧耗与
登录页专属体积预算仍保持待开始。

## What Changed

### Technology Theme

- `frontend/src/styles/console-theme.css`：建立碳黑侧栏、深色技术顶栏、冷灰网格工作区，以及青色、
  翠绿和琥珀色状态体系；统一登录页、控制台、导航、表单、卡片和交互状态。
- 登录页改为更硬朗的工业控制台表达：分级网格、扫描线、清晰边界、终端式表单和高对比拓扑，
  保留 Canvas2D 粒子与认证状态联动。
- `frontend/src/components/ParticleNetwork.vue`：移除偏紫粒子与密码态连线，收敛为青、翠绿、青蓝和琥珀色。
- 提高深色侧栏分组标题对比度；控制台卡片和列表入场不再改变整体透明度，避免动画期间产生
  WCAG 对比度下降和内容闪烁。

### Navigation Shell

- `frontend/src/components/ConsoleLayout.vue`：增加带 Lucide 图标、tooltip 和 ARIA 状态的侧栏收起按钮；
  桌面端收敛为 72px 图标栏，移动端可完全隐藏，并使用 `aiops.sidebar.collapsed` 持久化选择。
- 当前路由重复点击直接返回，不再创建无意义导航。
- `frontend/src/App.vue`：移除包裹整个路由视图的离场/入场 Transition；增加永久存在的
  `.route-viewport` 技术底板。
- `frontend/src/styles/motion.css`：删除整页淡出和面板透明度入场，只保留轻量位移微动效。

### Regression Coverage

- `frontend/e2e/smoke.spec.ts`：新增侧栏收起、刷新持久化、重新展开和本地存储状态回归。
- 新增 500ms 连续采样，验证路由切换期间永久视口底板不会暴露白色画面。
- Desktop Chrome 与 Mobile Chrome 同时覆盖新增行为；现有登录粒子、全屏布局、认证转场和
  控制台页面回归保持通过。

### Baseline Documentation

- 同步 `README.md`、`CHANGELOG.md`、`docs/PROJECT_STATUS.md`、`docs/long-term-roadmap.md` 与
  `docs/next-long-term-plan.md`。
- 交付标签：`baseline-m93c-tech-console-20260809`。

## Verification

- `pnpm lint`：通过。
- `pnpm typecheck`：通过。
- `pnpm test -- --run`：23 files / 130 tests 全绿。
- `pnpm build`：通过（Vite 7.3.6，1791 modules，7.71s）；LoginView JS gzip 5.75 kB。
- `pnpm bundle:gate`：通过；entry JS gzip 42.1 kB，largest chunk 21.6 kB，
  total JS gzip 241.9 kB，total CSS gzip 53.4 kB。
- 侧栏与无白屏专项：Desktop Chrome + Mobile Chrome 共 4/4 通过。
- 全量 Playwright smoke + axe：Desktop Chrome + Mobile Chrome 共 42/42 通过；
  axe critical/serious = 0，console error = 0。

## Risks / Notes

- 应用各业务视图仍各自持有 `ConsoleLayout`；本次通过永久根底板和取消整页 Transition 消除可见白屏，
  未进行跨 34 个视图的布局架构迁移。
- in-app Browser 的本地 URL 策略阻止接管 `127.0.0.1:5173`，因此未补手工浏览器截图；
  双视口 Playwright smoke、像素、布局边界和 axe 作为本次可重复验证证据。
- M93-B2 仍需补低端设备帧耗报告和登录页专属 JS/CSS 预算，本次不声明 60fps 目标完成。
