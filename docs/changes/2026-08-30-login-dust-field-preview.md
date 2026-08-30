# 2026-08-30 Login Dust Field Preview

## Scope

- 调研 internal.limxdynamics.com 登录页背景特效（灰黑色散点缓慢漂浮 + 缩放闪烁），评估是否引入到本项目。
- 新增 `frontend/src/components/DustField.vue`：Canvas 2D 实现的静谧尘埃粒子场组件，无第三方依赖，支持浅色/深色背景与 prefers-reduced-motion。
- 新增 `outputs/dust-field-preview.html`：独立可双击预览的演示页，含浅色/深色对照、登录卡合成效果以及密度/速度/闪烁/半径参数滑杆。
- 在 `LoginView.vue` 中将 `<ParticleNetwork />` 提升到 `.login-page` 直接子节点以覆盖全页（左右两个面板都接收鼠标轨迹/涟漪/粒子交互），并新增 `<DustField :color-lightness="[70, 92]" />` 作为衬底层；`FluidBackground` 保留在左侧介绍区。
- 调整 `console-theme.css`：删除原 `.login-intro > .particle-network` 规则，新增 `.login-page > .dust-field` (z-1) 与 `.login-page > .particle-network` (z-2) 规则；mask 渐变下沿淡出效果同步迁移至页面级选择器。

## Context

- 用户在 2026-08-30 提出参考 internal.limxdynamics.com/user/login 登录页空白处特效，希望评估是否移植。
- 用户在评审后明确需求："重点不是背景，而是鼠标在空白区域的彩色特效"；最终落定：保留并扩展现有 `ParticleNetwork`（彩色粒子 + 鼠标轨迹/涟漪）覆盖整个登录页，并用 `DustField` 作为衬底（适配深色登录底）。
- 现有 `frontend/src/components/ParticleNetwork.vue` 此前只挂在 `.login-intro` 内、只覆盖左侧。提升到 `.login-page` 直接子节点后，左右两侧空白处都会响应鼠标交互，且表单面板（z-3）仍然在最上层不会被打断输入。
- `DustField` 的 `colorLightness` 默认偏暗（22–42% 灰阶）适配浅色背景；在我们的深色登录页里改为 `[70, 92]`（浅色灰阶）才能看清。
- 组件设计目标：体量轻、不阻塞输入、与登录卡视觉不冲突、键盘/无障碍无副作用、SSR 安全（仅在 `onMounted` 中初始化）。

## What Changed

### frontend/src/components/DustField.vue
- 新增 Vue 3 SFC，Canvas 2D 渲染。
- 粒子参数：密度 1/18000 px²、最少 36 / 最多 96、半径 0.7–2.2 px、速度 0.05–0.22 px/帧、alpha 0.18–0.62、twinkle 振幅 0.28。
- 灰阶 HSL 范围可配 `colorLightness: number[]`（默认 `[22, 42]`），通过 `length >= 2` 守护，越界回退到默认。
- 监听 `ResizeObserver`、`prefers-reduced-motion`、`document.visibilitychange`，页面隐藏或减少动效时停止 RAF。
- 暴露可选 `props` 覆盖默认参数，`<style scoped>` 限定 `position: absolute; inset: 0; pointer-events: none`。

### outputs/dust-field-preview.html
- 独立 HTML 演示，包含三个面板：浅色背景（参考源风格）、深色背景、登录卡合成。
- 提供密度/速度/闪烁/半径滑杆，仅联动第一个浅色面板，便于用户调参。
- 脚本是与组件同源的纯 Canvas 实现，复制即用，不依赖打包工具。

### frontend/src/views/LoginView.vue
- 引入 `DustField` 组件。
- 在 `<main class="login-page">` 直接子节点层增加 `<DustField :color-lightness="[70, 92]" />` 与 `<ParticleNetwork :phase="authPhase" />`，把后者从 `.login-intro` 中移除。
- 脚本逻辑未改动。

### frontend/src/styles/console-theme.css
- 删除原 `.login-intro > .particle-network` 规则（已不匹配 DOM）。
- 新增 `.login-page > .dust-field { z-index: 1; }`。
- 新增 `.login-page > .particle-network { z-index: 2; mask-image: linear-gradient(...); }`，保留原下沿淡出遮罩。

## Verification

- 在受管 Chromium 中打开 `outputs/dust-field-preview.html`，截图确认：
  - 浅色面板：灰黑色散点均匀分布，无重叠遮挡文字。
  - 深色面板：浅色散点在深色渐变背景上仍然清晰。
  - 登录卡合成：登录卡居于粒子场之上，可读性未受影响。
- 间隔 2 秒截取参考站登录页两张图（`/tmp/limx-t1.png`、`/tmp/limx-t2.png`），确认原站粒子随时间漂移、缩放有变化；自建组件在 RAF 循环下复现相同运动语义。
- `pnpm typecheck` 通过（`vue-tsc -b` 0 错误）。
- `pnpm exec eslint src/components/DustField.vue src/views/LoginView.vue` 通过（0 警告 0 错误）。
- `pnpm build` 通过；产物 `dist/assets/LoginView-BnenJ3EM.js` 44.51 kB（gzip 14.01 kB），体积增加来自 `DustField` 组件（约 1.6 kB gzip 估算），仍远低于 `perf:login` 阈值。

## Risks / Notes

- 跨页面交互：`.login-page > .particle-network` 现在把事件委托到 `.login-page`；表单卡内的输入仍由 `.login-form-panel` 顶层覆盖（z-3 > z-2），输入事件不会丢。
- `FluidBackground` 仍位于 `.login-intro` 内（z-0），继续承担左侧暗色渐变 + 鼠标流体；未在 `FluidBackground` 上做改动。
- 暂未补 `DustField.spec.ts`：项目 `vitest.config.ts` 当前 `environment: 'node'` 且 `include: src/**/*.test.ts`，Vue 组件 DOM 测试需要 jsdom 切换；与既有约定一致，先不在本次引入。如后续要补，建议新增一个 `src/components/__tests__/DustField.test.ts` 并配套把 `environment: 'jsdom'` 作用于该文件。
- `pnpm perf:login` 跑分可在集成审阅阶段再执行一次，确认 Canvas 双层（dust + particles）叠加后的登录首帧/CPU 占用未退化。
- 集成与归档已同步：`CHANGELOG.md` Unreleased 区块中本条目已从"预览版未集成"升级为"已接入 LoginView"。

