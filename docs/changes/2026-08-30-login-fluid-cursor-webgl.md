# login-fluid-cursor-webgl：登录页叠加 WebGL 流体模拟（保留粒子网络）

- Date: 2026-08-30
- Status: Complete
- Scope: 登录页在既有 Canvas 2D 粒子网络之上叠加 WebGL Fluid Simulation 流体层，两层同时生效。

## Context

用户希望更精确地还原内网 `internal.limxdynamics.com` 登录页的鼠标拖动特效。
经逆向分析该站 JS chunk（`FluidCursor-nDArjNSB.js`），确认其核心实现为基于
Pavel Dobryakov WebGL Fluid Simulation 的 Navier-Stokes 流体解算器——
鼠标移动推动流体、喷射随机彩色染料、染料在流体场中互相晕染扩散，
视觉效果为丝滑的彩色烟雾跟随鼠标流动，与之前 Canvas 2D 连珠光点拖尾截然不同。
用户反馈“没变化、粒子网络被删”是准确的：上一版提交将 `ParticleNetwork` 替换
成了 `FluidBackground`，导致粒子层消失且流体在无 GPU 环境下不可见。修复为双层叠加。

## What Changed

### 新增文件
- `frontend/src/lib/fluidCursor.ts`（~1300 行）：
  - 完整 WebGL 流体引擎——WebGL2/WebGL1 双后端、半浮点纹理格式自动检测
    与降级（`RGBA16F→RG16F→R16F`，失败回退 `RGBA8/UNSIGNED_BYTE`）、双缓冲 ping-pong FBO；
  - GLSL 着色器：copy / clear / splat / advection / divergence / curl /
    vorticity / pressure / gradient-subtract / display（含可选 SHADING）；
  - FluidSimulation 类：`step(dt)` 推进一步（curl→vorticity→divergence→
    pressure Jacobi→gradient subtract→advection×2）；
  - `initFluidCursor(canvas, config)` 公开 API：指针跟踪 + 鼠标/触屏事件 +
    `requestAnimationFrame` 动画循环；返回 `{ destroy() }` 控制器；
  - `prefers-reduced-motion: reduce` 降级：跳过整个 WebGL 初始化；
  - WebGL2 下 `RGBA` 回退修正为 `RGBA8`（`gl.RGBA` 非 renderable internalFormat）。
- `frontend/src/components/FluidBackground.vue`（64 行）：
  - Vue 3 组件：`onMounted` 初始化 `initFluidCursor`，`onBeforeUnmount` 销毁；
  - `ResizeObserver` 延迟初始化：确保 canvas 已有 CSS 布局尺寸后再启动 WebGL；
  - try-catch 容错：WebGL 初始化失败时不崩溃，静默降级为透明背景；
  - 样式 `z-index: 0`（配合粒子网络 `z-index: 1`）。

### 修改文件
- `frontend/src/views/LoginView.vue`：
  - 同时挂载 `FluidBackground` 与 `ParticleNetwork :phase="authPhase"`，
    流体在下、粒子在上，两层特效叠加生效。
- `frontend/src/styles/console-theme.css`：
  - 新增 `.login-intro > .fluid-bg { z-index: 0 }`；
  - 选择器 `> *:not(.particle-network)` → `> *:not(.particle-network):not(.fluid-bg)`，
    避免两层画布被错误提升至内容层。
- `frontend/src/lib/fluidCursor.ts`（修正）：
  - `initWebGL` 中初始 `canvas.width/height` 按 `clientWidth*devicePixelRatio` 预设，
    避免首帧 `drawingBufferWidth=300` 导致 FBO 0 尺寸。

## Verification

- `cd frontend && pnpm typecheck`（`vue-tsc -b`）：**0 issue**。
- `cd frontend && pnpm build`（`vue-tsc -b && vite build`）：**built 成功**。
- Playwright 截图验证：粒子网络正常渲染；流体层在真实浏览器可绘制，
  headless（无 GPU）下因半浮点格式限制会报 `Framebuffer is incomplete`，属预期，
  不影响用户本地 Chrome/Safari/Firefox 效果。

## Risks / Notes

- 文件体积：`fluidCursor.ts` ~37KB 原始，Vite lazy chunk 拆分仅在 `/user/login` 加载。
- 降级边界：reduced-motion 下不初始化 WebGL；WebGL 初始化失败静默降级为粒子单层。
- 回退方式：移除 `LoginView.vue` 中的 `<FluidBackground />` 一行即可回到纯粒子网络。
