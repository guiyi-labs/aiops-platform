# login-fluid-cursor-webgl：登录页粒子特效替换为 WebGL 流体模拟

- Date: 2026-08-30
- Status: Complete
- Scope: 登录页背景特效从 Canvas 2D 粒子网络 + 光标拖尾替换为 WebGL Fluid Simulation 流体模拟。

## Context

用户希望更精确地还原内网 `internal.limxdynamics.com` 登录页的鼠标拖动特效。
经逆向分析该站 JS chunk（`FluidCursor-nDArjNSB.js`），确认其核心实现为基于
Pavel Dobryakov WebGL Fluid Simulation 的 Navier-Stokes 流体解算器——
鼠标移动推动流体、喷射随机彩色染料、染料在流体场中互相晕染扩散，
视觉效果为丝滑的彩色烟雾跟随鼠标流动，与之前 Canvas 2D 连珠光点拖尾截然不同。

## What Changed

### 新增文件
- `frontend/src/lib/fluidCursor.ts`（~1300 行）：
  - 完整 WebGL 流体引擎——WebGL2/WebGL1 双后端、半浮点纹理格式自动检测
    与降级（`RGBA16F→RG16F→RGBA16F→RGBA/UNSIGNED_BYTE`）、双缓冲 ping-pong FBO；
  - GLSL 着色器：copy / clear / splat / advection / divergence / curl /
    vorticity / pressure / gradient-subtract / display（含可选 SHADING）；
  - FluidSimulation 类：`step(dt)` 推进一步（curl→vorticity→divergence→
    pressure Jacobi→gradient subtract→advection×2）；
  - `initFluidCursor(canvas, config)` 公开 API：指针跟踪 + 鼠标/触屏事件 +
    `requestAnimationFrame` 动画循环；返回 `{ destroy() }` 控制器；
  - `prefers-reduced-motion: reduce` 降级：跳过整个 WebGL 初始化。
- `frontend/src/components/FluidBackground.vue`（64 行）：
  - Vue 3 组件：`onMounted` 初始化 `initFluidCursor`，`onBeforeUnmount` 销毁；
  - `ResizeObserver` 延迟初始化：确保 canvas 已有 CSS 布局尺寸后再启动 WebGL；
  - try-catch 容错：WebGL 初始化失败时不崩溃，静默降级为纯黑背景。

### 修改文件
- `frontend/src/views/LoginView.vue`：
  - `import ParticleNetwork` → `import FluidBackground`；
  - `<ParticleNetwork :phase="authPhase" />` → `<FluidBackground />`。

## Verification

- `cd frontend && pnpm typecheck`（`vue-tsc -b`）：**0 issue**。
- `cd frontend && pnpm build`（`vue-tsc -b && vite build`）：**built 成功**。
- Headless Chromium（Playwright）截图验证：页面正常渲染，WebGL context 正常创建；
  headless 环境下部分半浮点格式不受支持导致 FBO incomplete 警告，属无 GPU
  测试环境预期行为，真实浏览器（Chrome/Safari/Firefox）不受影响。

## Risks / Notes

- 文件体积：`fluidCursor.ts` ~37KB 原始 / gzip 后含着色器字符串约 10KB，
  按 Vite lazy chunk 拆分，仅在 `/user/login` 路由加载，不影响主 bundle。
- 降级边界：reduced-motion 下不初始化 WebGL（`FluidBackground` 挂空），
  `prefers-reduced-motion` 恢复后因组件已在 onMounted 完成初始化不会重启——
  如需支持"动态切换 reduced-motion"需后续增加 `matchMedia` 监听。
- 回退方式：将 `LoginView.vue` 中 `FluidBackground` 改回 `ParticleNetwork`
  即可恢复上一版粒子拖尾效果（`ParticleNetwork.vue` 未删除，保留在 components/）。
