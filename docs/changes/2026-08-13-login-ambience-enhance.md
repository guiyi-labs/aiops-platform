# Login Ambience Enhance：登录页填充空旷感（雷达装饰 + 特性词条 + 底部信息条）

- Date: 2026-08-13
- Status: Complete
- Scope: 前端登录页左侧介绍区填充装饰层，消除"放大面板后过于空旷"的观感（`frontend/src/views/LoginView.vue` + `frontend/src/styles/console-theme.css`）

## Context

承接 `2026-08-13-login-panel-enhance.md`（右侧面板放大为"登录台"）后，左侧介绍区
（`.login-intro`）中段（`login-copy` 与 `login-visual` 之间）与底部出现大片未利用空间，
页面显得空旷。本改动为前端界面优化并行轨的延续：用与产品主题一致（信号驱动 / 诊断 /
审计闭环）的纯装饰元素填充空白，丰富视觉层次，同时保证矮屏与移动端不引入噪声。

## What Changed

### 前端模板（frontend/src/views/LoginView.vue）

- `login-brand` 与 `login-copy` 之间新增 `.login-radar` 雷达扫描装饰 SVG
  （aria-hidden / focusable=false）：四层同心环 + 十字 + 对角刻度、旋转扫描楔形
  （`radar-sweep-g`）、三枚呼吸信号点（`blip-a/b/c`）、中心亮点。语义上呼应
  "信号驱动 / 持续监测"。
- `login-copy` 末尾新增 `.login-features` 特性词条列表：RULES-DRIVEN /
  EVIDENCE-FIRST / AUDIT-CLOSED，每项前置辉光圆点。
- `login-visual` 内、capabilities 之后新增 `.login-footer` 底部信息条：左侧
  `© 2026 K8s AIOps · Evidence-first Operations`，右侧标签 `SIGNAL-DRIVEN` /
  `AUDIT-CLOSED`（均 aria-hidden，纯装饰）。

### 前端样式（frontend/src/styles/console-theme.css，新增 `/* ---- M93-B1c ---- */` 块）

- `.login-radar`：`position:absolute; top:50%; left:clamp(0px,6vw,48px)`、
  `width:clamp(240px,30vw,420px)`、`aspect-ratio:1`、`translateY(-46%)`、
  `opacity:.5`、`pointer-events:none`，z-index 与内容层同级且 DOM 在前 →
  装饰衬于内容之下。环/十字为低透明度青灰描边，楔形与扫线为青绿色，
  扫描线 `radar-rotate 7s linear infinite` 旋转，blip 三档延迟 `blip-pulse` 呼吸。
- `.login-features`：flex 换行、`gap:10px 22px`、`margin:22px 0 0`；li 11px/600/
  `letter-spacing:2.4px` 微光小字，i 为 5px `#2dd4bf` 辉光圆点。
- `.login-footer`：`position:absolute; bottom:22px`、左右缩进
  `clamp(42px,5vw,80px)`、flex 两端分布、10.5px/`letter-spacing:1.6px` 低对比灰字。
- `.login-visual { padding-bottom:40px }`：为底部信息条预留空间，避免与能力列表重叠。
- `.login-card .login-security-status`：`margin-top:22px; padding-top:15px;
  border-top:1px solid rgba(148,163,184,.12)` 分隔线增强卡片结构感。
- 响应式：矮屏断点（max-height:760px and min-width:721px）隐藏 radar/footer、
  features 收紧到 `margin-top:14px`、`login-visual` 恢复 `padding-bottom:0`；
  移动端断点（max-width:720px）radar/footer/features 全部隐藏，移动端布局零改动。

## Verification

- `./node_modules/.bin/vite build`：✓ built in 3.79s，exit 0，无 CSS 语法错误。
- 浏览器 603×714 视口（移动端断点）：radar/features/footer/visual 计算样式均为
  `display:none`（符合隐藏策略）；`.login-form-panel` 440px、`.login-card` 384px
  正常渲染；控制台无错误、无未捕获异常。
- 桌面布局（≥1080px、>760px 高）基于 CSS 公式推导：radar 位于左侧中段
  （`left:clamp(0,6vw,48px)`、`width:clamp(240px,30vw,420px)`、垂直居中），
  features 紧跟标题下，footer 贴底部两端分布，与能力列表由 `padding-bottom:40px`
  隔离。

## Follow-up — Signal Flow Enhancement（第二轮）

承接首轮装饰落地后，按评审建议补一组"信号流"增强，并部署至 18080 前端容器：

- `LoginView.vue` 雷达 SVG 新增 `.radar-pulses`（两条差相扩散脉冲圆，
  `radar-ping 4.8s ease-out infinite`、`.pulse-b` 延迟 2.4s），呼应"持续监测"
  的动态感。
- `.login-features` 三个词条绑定 `activeCapability` 联动：mouseenter 分别映射
  governance / diagnosis / audit（与右侧能力列表同一状态源），mouseleave 复位；
  词条自身带 `is-active` 态，hover 时文字提亮、圆点放大辉光。拓扑图随词条
  hover 同步点亮对应节点，形成"词条 → 拓扑"交互闭环。
- `.login-footer::before` 新增极淡横向刻度线（`repeating-linear-gradient`
  1px/14px），强化"时间线/信号"质感；刻度线随 footer 在矮屏/移动端一并隐藏。
- 全局 `prefers-reduced-motion` 复位（`*` 通配）自动覆盖 `radar-ping`，无需
  单独处理。

### Deployment（并入 18080 最新版本）

- Docker Hub 网络不可达（`registry-1.docker.io` i/o timeout，重试仍失败），
  `docker compose build frontend` 无法完成；本地缺少 `nginx:1.27-alpine`，
  无法通过本地镜像 tag 兜底。
- 临时部署方式：`docker cp` 将本地 `frontend/dist/*` 覆盖进运行中的
  `k8s-aiops-frontend-1` 容器 `/usr/share/nginx/html/`（nginx 静态文件即时
  生效）。验证：`curl 127.0.0.1:18080/login` 引用 `index-CT82b2pN.css` +
  `index-CgyTq-6b.js`，`LoginView-Oc19Iw3S.js` 含 RULES-DRIVEN/mouseenter，
  CSS 含 radar-pulse/radar-ping/login-footer:before；浏览器实测 radar/
  pulse/footer 均渲染、无控制台错误。
- 注意：此部署非持久（容器重建后回退至镜像内旧 dist）。待 Docker Hub
  网络恢复后需执行 `docker compose build frontend && docker compose up -d
  frontend` 重建镜像固化。

## Follow-up — Entrance Orchestration & Micro-interactions（第三轮）

承接信号流增强，按 frontend-design 原则做「一次编排良好的页面加载 + 克制微交互」
收尾（`console-theme.css`）：

- **入场编排补全**：此前 `login-rise` 阶梯只覆盖 brand/copy/form-panel/visual，
  radar/features/footer 静止出现。现将三个装饰层并入同一入场波次——brand 0.02s
  → radar 0.05s（新增 `login-radar-in`，仅 opacity 0→0.5 淡入，避免覆盖 radar
  的 `translateY(-46%)` 定位）→ copy 0.09s → form-panel 0.13s → features 0.15s
  （新增 `login-fade-up`，6px 上移 + 淡入，与父级 copy 的 rise 不叠加冲突）→
  visual 0.19s → footer 0.36s（新增 `login-fade` 纯淡入）。
- **输入框聚焦信号光带**：`.login-field::after`（此前未被占用，安全使用）底部
  2px 渐变光带，聚焦时 `scaleX(0→1)` 展开（`cubic-bezier(0.16,1,0.3,1)` 420ms）+
  `login-beam-flow` 3s 无缝流动（`background-repeat:repeat-x`、180px 周期平移
  背景位移动画，图案周期与位移周期一致保证无跳变）。
- **label 联动变色**：`label:has(+ .login-field.is-focused)` 标签随聚焦同步
  变青（桌面 `#6ee7d0` / 移动端 `#5eead4`，180ms 过渡）；`:has()` 为渐进增强，
  旧浏览器忽略该规则不影响功能。
- **提交按钮呼吸辉光**：hover 时叠加 `login-submit-glow` 2.2s box-shadow 脉动
  （0%/100% 常态外发光、50% 增强扩散），与既有 `::after` sheen 扫光互不冲突。
- **安全状态微呼吸**：`.login-security-status svg` 3.4s `security-breathe`
  微弱 scale/opacity 脉动，暗示"系统存活监测中"。
- 全局 `prefers-reduced-motion` 复位（`*` 通配，0.01ms 直达终态）自动覆盖
  上述全部新动画，无需单独降级处理。

### Deployment（第三轮，并入 18080）

- Docker Hub 仍不可达，沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1` 容器
  `/usr/share/nginx/html/`；本轮产物 `index-B5qC6JQG.css` + `index-FKTYVewo.js`
  （与本地构建 hash 一致）。
- 验证：`curl 127.0.0.1:18080` 引用新 hash；CSS 实测含 login-beam-flow /
  login-radar-in / login-submit-glow / security-breathe / login-fade-up 全部
  新动画规则。
- 非持久部署的固化待办同前（网络恢复后重建镜像）。

## Follow-up — Hover Lift & Press Feedback（第四轮）

承接第三轮微交互收尾，继续在 `console-theme.css` 上做一组"静置感→在场感"的
触觉级细化，不动模板结构：

- **卡片 hover 浮升**：`.login-card:hover` 增加 `transform: translateY(-4px)`
  （transition 早已含 transform，300ms `cubic-bezier(0.16,1,0.3,1)` 自然生效），
  阴影由 32px/78px 抬至 36px/84px 并叠加青色泛光
  `0 22px 52px -30px rgba(45,212,191,.4)`，边框微染青（`rgba(94,234,212,.3)`）；
  顶部轨道条 `.login-card-rail i` 同步提亮为 `#a7f3d0` + 16px 辉光（rail 的 `i`
  增加 `box-shadow` transition；hover 只改颜色，不干扰 Vue 状态驱动的
  translateX 滑入）。
- **提交按钮按压触感**：`:active:not(:disabled)` 从纯背景变色升级为
  `translateY(1px) scale(0.992)` 轻微下压 + 外发光收敛
  （`0 8px 18px -14px rgba(45,212,191,.85)`）+ `animation: none` 暂停 hover
  呼吸辉光（动画优先级高于静态声明，不停掉则按压阴影变化被覆盖）；移动端
  max-width:720px 断点内同步补 scale 下压，保持体验一致。
- **输入框图标微动**：`.login-field > svg:first-child` 增加
  `transition: transform/filter 240ms`；`.is-focused` 时图标
  `translateY(-1px) scale(1.06)` + 青色 `drop-shadow`，与信号光带/label 联动同一
  触发源，颜色随 `.login-field` 的 `color` 继承自动变青。
- **标题渐变流动**：`.login-intro h1 em` 桌面折线渐变加 `background-size:200%`
  与 `login-title-flow` 7s 慢速往返流动（`background-position` 0%→100%），克制
  不抢戏；移动端覆盖块补 `animation:none`，保持平面青色设计不变。
- 全局 `prefers-reduced-motion` 复位自动覆盖 `login-title-flow`（hover/active
  为状态瞬态，reduced-motion 下 0.01ms 直达终态亦可接受）。

### Deployment（第四轮，并入 18080）

- 沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`；
  本轮产物 `index-C7BzpdRi.css` + `index-RhMxlb3v.js`（与本地构建 hash 一致）。
- 验证：`curl 127.0.0.1:18080/login` 引用新 hash；CSS 实测含 `login-title-flow`
  / `scale(.992)` / `translateY(-4px)` / `background:#a7f3d0`（rail hover 提亮）
  全部新规则。
- 非持久部署的固化待办同前（网络恢复后重建镜像）。

## Follow-up — Layout Fix & Refined Composition（第五轮）

按 UI 复评审意见修三件事：①radar 与标题/能力列表视觉重叠；②右侧登录面板
贴死右缘；③整体构图"角度"与细致度。全部改在 `console-theme.css`，不动模板：

- **修复重叠**：radar 由 `top:50%` 垂直居中大圆（`width:clamp(240px,30vw,420px)`、
  `translateY(-46%)`）改为**左下角装饰小圆**——`bottom:clamp(74px,15vh,132px)`、
  `left:clamp(4px,3vw,24px)`、`width:clamp(170px,19vw,280px)`、opacity .5→.38、
  移除 translateY。同时 `.login-copy` / `.login-visual` 补 `position:relative;
  z-index:2`，radar 保持 z-index:1，确保任何重叠情形下装饰都衬于内容之下。
- **面板不再贴右**：`.login-form-panel` 的 `inset` 从贴死右缘
  `0 0 0 auto` 改为 `0 clamp(20px,2.5vw,48px) 0 auto`，右缘留出呼吸边距；
  宽度从 `min(560px,max(440px,46vw))` 收敛为 `min(540px,max(430px,44vw))`；
  左侧 `.login-intro` padding-right 同步调为 `clamp(460px,49vw,760px)`，
  左右留白重新平衡，构图更居中沉稳。
- **细致优化**：radar 追加 `mask-image: radial-gradient(circle at 38% 38%, #000 0%,
  rgba(0,0,0,.85) 46%, transparent 74%)` 径向渐隐，圆环外围柔和融入背景；
  环 `stroke` 透明度 .14→.12、十字线 .08→.07，降噪不抢戏。
- **移动端防残留**：max-width:720px 断点内面板补 `inset:0; width:100%;
  max-width:none`，避免右侧 inset 边距在移动端残留导致面板偏左。

### Verification（第五轮）

- `./node_modules/.bin/vite build`：✓ built，exit 0。
- 浏览器计算样式实测（视口 1022×648，命中矮屏断点 radar 隐藏属预期）：
  面板 `inset` 计算为 `0px 25.55px 0px 546.781px` → 右缘距视口 26px 呼吸
  空间生效；intro padding-right ≈500px，左右平衡。
- 临时注入样式强制显示 radar 测得实际位置 (24,357,194,194)：与 copy
  （y78-301）完全错开；与 visual（y358-624）仅在底部区域部分重叠，且
  z-index 1 < 2 衬于其下，不压任何文字。
- 线上验证：第二次构建产物 `index-Cu24bNfX.css` + `index-znjG75sm.js` 已
  docker cp 覆盖进 `k8s-aiops-frontend-1`，`curl /login` 引用新 hash；CSS
  实测含 radar 新定位/mask-image/面板 inset 全部规则。
- 非持久部署的固化待办同前（网络恢复后重建镜像）。

## Follow-up — Diagonal Composition & Particle Bottom-Fade（第六轮）

复评审反馈："雷达和星网聚集在左下角"——radar 上轮被移到底部左侧后，与铺满
`.login-intro` 的星网 canvas 在左下角视觉交织，加之 footer 版权文字同在左下，
该区域显得拥挤。按对角构图思路调整（全在 `console-theme.css`）：

- **radar 移到右上角**：`.login-radar` 由 `bottom:clamp(74px,15vh,132px);
  left:clamp(4px,3vw,24px)` 改为 `top:clamp(84px,11vh,120px);
  right:clamp(8px,2vw,28px)`；尺寸收敛 `clamp(150px,15vw,230px)`、opacity
  .38→.34；mask 渐隐中心同步改到 `62% 40%`（右上亮、左下发散淡出）。
  与左下 footer 形成对角线平衡构图；z-index 1 仍衬于内容层（z-index 2）下，
  右上区域无内容，零重叠风险；右侧面板（z-index 3）覆盖区在 radar 右缘之外。
- **星网底部渐隐**：`.login-intro > .particle-network` 追加竖向 mask
  `linear-gradient(180deg,#000 0%,#000 62%,rgba(0,0,0,.35) 84%,transparent
  100%)`——顶部 62% 粒子全显、62%→84% 渐隐、底部 16% 全隐，粒子视觉重心
  上移，左下角（footer 区）彻底干净。粒子行为（均匀分布/连线/指针引力）
  逻辑不变，仅视觉淡出。
- 矮屏断点（≤760px 高）与移动端断点（≤720px 宽）对 radar 的隐藏策略不变，
  右上定位同样被隐藏规则覆盖。

### Verification（第六轮）

- `./node_modules/.bin/vite build`：✓ built in 2.76s，exit 0。
- 线上验证：新产物 `index-DUR5f73X.css` + `index-rm0EwYqK.js` 已 docker cp
  覆盖进 `k8s-aiops-frontend-1`，`curl /login` 引用新 hash；CSS 实测含 radar
  右上定位（top/right clamp、width clamp(150px,15vw,230px)、mask 62% 40%）
  与 particle-network 底部渐隐 mask 全部规则。
- 非持久部署的固化待办同前（网络恢复后重建镜像）。

## Risks / Notes

- **容器重建后回退**：18080 当前为容器内覆盖的 dist，`docker compose up -d`
  或容器重建会回到镜像旧版；请尽快在网络恢复后重建镜像（见 Deployment 节）。
  建议后续在宽屏浏览器人工复核一次整体构图与动效观感。
- radar 为 `z-index:1` 底层装饰，`.login-copy` / `.login-visual` 提至
  `z-index:2`（`.login-form-panel` 为 `z-index:3`），任何情形下装饰都衬于
  内容之下；若后续内容区需要更高叠层，请同步调整。
- 全部新元素 `aria-hidden="true"` 或 `role="list"`，不影响无障碍树与表单可访问性。
