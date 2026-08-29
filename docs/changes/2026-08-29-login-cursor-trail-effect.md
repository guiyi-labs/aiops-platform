# login-cursor-trail-effect：登录页粒子网络新增光标轨迹与点击涟漪特效

- Date: 2026-08-29
- Status: Complete
- Scope: 登录页 `ParticleNetwork` 组件增强——鼠标在空白处移动时留下连珠光点残影与连线，点击产生扩散涟漪。

## Context

用户希望把其参考站（内网 `internal.limxdynamics.com` 登录页）那种「鼠标在空白处产生图案特效」的观感移植到本项目登录页。
项目登录页 (`frontend/src/views/LoginView.vue`) 已挂载 `ParticleNetwork`（canvas 粒子网络 + 已有指针吸附/连线），
但光标仅在 `pointerRadius` 内被动吸附已有粒子，缺少**主动在空白处绘制跟随光点轨迹**的视觉反馈。

参考页为内网域名，当前网络无法解析（SSL 握手失败），故按用户描述实现一版等价、且符合本项目既有规范的特效：
- 游标移动时以 ~16ms 节流生成光点残影（`trail`，最多 26 个，逐帧衰减），并用低透明度描边连接形成「拖尾」；
- 点击空白处生成扩散涟漪（`ripple`，半径递增、透明度衰减）；
- 触屏 `touchmove` 同样生成轨迹；
- 全部遵守 `prefers-reduced-motion` 降级：reduced-motion 下不绘制轨迹/涟漪（与组件既有 motion 纪律一致）。

## What Changed

### 前端组件
- `frontend/src/components/ParticleNetwork.vue`：
  - 新增 `TrailPoint` / `Ripple` 接口与 `trail` / `ripples` 数组、`TRAIL_MAX` 上限、`trailSpawnTimer` 节流计时器；
  - 新增 `spawnTrailPoint(x,y)`、`advanceTrail()`（逐帧推进残影/涟漪生命周期并清理过期项）、`handlePointerClick(event)`；
  - `handlePointerMove` / `handleTouchMove` 在节流窗口内调用 `spawnTrailPoint` 记录光标位置；
  - `animationLoop` 调用 `advanceTrail()`；`draw()` 在粒子绘制后追加：游标残影径向光晕 + 拖尾连线、点击涟漪描边；
  - 在 `onMounted` 注册 `parentElement` 的 `click` 监听（canvas 本身 `pointer-events:none`，监听挂在父元素），
    `onBeforeUnmount` 同步移除，避免泄露。

## Verification

- `cd frontend && pnpm typecheck`（`vue-tsc -b`）：**0 issue**。
- `cd frontend && pnpm build`（`vue-tsc -b && vite build`）：**built 成功**，产物 `dist/assets/index-DYUBbBDe.js` 等。
- 运行容器热更新：已将新 `dist` 拷入 `k8s-aiops-frontend-1`（`docker cp ./dist/. ...:/usr/share/nginx/html/`），
  首页 `http://127.0.0.1:18080/` 已引用新 chunk，`LoginView` chunk 内含轨迹色值 `129,230,217` / `94,234,212`、`createRadialGradient`、`ripple` 逻辑（已 curl 校验 200）。
- 说明：本会话因 `auth.docker.io` 临时不可达未能 `docker compose build frontend` 重建镜像，
  改用 `docker cp` 热更新运行容器，效果等价；待网络恢复后建议补一次 `docker compose build frontend && up -d --no-build` 固化镜像。

## Risks / Notes

- 性能：轨迹点上限 26、涟漪按需生成并在 `life<=0` 时 spliced，无无限增长；reduced-motion 下完全跳过绘制。
- 降级边界：reduced-motion 与成功态（`phase==='success'`）下不绘制光标残影，保持既有交互语义。
- 后续：若需把特效扩展到其他页面（如 Dashboard 背景），可将该组件复用或抽取为通用 composable。
