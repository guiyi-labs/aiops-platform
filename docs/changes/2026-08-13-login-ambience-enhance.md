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

## Follow-up — Round 8: Narrative Coherence, Depth & Card Focus（第八轮）

承接前 7 轮（面板放大 → 空旷填充 → 信号流 → 入场编排 → 触觉细节 → 重叠修复/贴右 →
对角构图）后，用户表示"仍不满意"，经关键路径代码审查锁定四类结构性短板并一次性收口：
①左区品牌/标题/拓扑呈"上下两块孤岛"、中间空洞；②左右缺视觉引线；③中央空洞未定调；
④radar/粒子偏满。本轮回应在 `console-theme.css` 新增 `/* ---- M93-B1d ---- */` 块，
并在 `LoginView.vue` / `ParticleNetwork.vue` 做最小改动，全部纯视觉层、不动表单逻辑与无障碍。

### 改动清单

- **中央景深（解决空洞定调）**：`LoginView.vue` 新增 `.login-depth`（aria-hidden，
  z-index:1，置于 ParticleNetwork 之后、内容层之下），以左中偏置的双层径向渐变
  （青 + 紫）模拟"信号核心"柔光，叠加 `login-depth-breathe` 14s 慢呼吸；mask 限定在
  左中区域并向右侧（面板区）渐隐，避免与右侧登录卡争视线。
- **信号脊柱（解决孤岛 + 左右引线）**：新增 `.login-spine`（aria-hidden，z-index:1，
  贴左留白沟槽 `left:clamp(16px,2vw,36px)`），一条 2px 青色渐隐竖线将品牌→标题→拓扑
  串成叙事主线；线身带一枚 `login-spine-travel` 5.5s 往复游走的光点，暗示"信号在流转"。
- **卡片焦点升级（解决"主角光环"缺失）**：
  - `LoginView.vue` 卡片内新增 `.login-card-frame`（aria-hidden）四角括号
    （`::before` 左上 / `::after` 右下 L 形，常态 opacity .5，hover 或 `:has(.is-focused)`
    时提亮 + 青色辉光），把操作区框成"仪表盘"。
  - 新增 `login-card-attract` 1.5s、延迟 0.92s 的一次性青色光晕脉冲（仅动画
    box-shadow、无 fill，`both` 不保留，故载入后 hover 浮升/按压仍正常），把注意力
    在入场后收束到卡片。
  - 新增 `.login-card:has(.login-field.is-focused)` 整卡聚焦联动：聚焦任意输入框时
    整卡描边与内发光同步提青（`:has()` 为渐进增强，旧浏览器忽略不影响功能）。
- **减噪与克制**：
  - `ParticleNetwork.vue` 桌面粒子密度 `11000→14000`、上限 `90→70`、下限 `28→24`，
    连线基础透明度 `0.22→0.16`，让星网更安静；行为（均匀分布/连线/指针引力）不变。
  - radar 透明度 `0.34→0.30`、mask 渐隐点 `48%/76% → 44%/72%`，进一步弱化为背景仪器。
  - 全部新增动画均被全局 `prefers-reduced-motion` 通配复位覆盖，无需单独降级。

### 分层与响应式

- 叠层：`.login-depth` / `.login-spine` 用 `.login-page .login-intro > ...`
  （specificity 0,3,0）覆盖既有 `> *:not(.particle-network){z-index:2}` 规则，
  强制落在 z-index:1 装饰层（衬于内容 z-index:2 / 面板 z-index:3 之下）。
- 矮屏断点（max-height:760px & min-width:721px）与移动端断点（max-width:720px）
  均将 `.login-depth` / `.login-spine` 列入 `display:none`，与 radar/footer 同策略隐藏；
  卡片角标在窄屏保留（低存在感、无害）。

### Verification

- `./node_modules/.bin/vite build`：✓ built in 3.06s，exit 0，无 CSS 语法错误。
- 线上验证：新产物 `index-BQPzgRJy.css` + `index-BRIbTalY.js` 已 docker cp 覆盖进
  `k8s-aiops-frontend-1`，`curl /login` 引用新 hash；CSS 实测含 `login-depth` /
  `login-spine` / `login-card-frame` / `login-card-attract` / `login-spine-travel` /
  `login-depth-breathe` 全部新规则；radar `opacity:.3` 生效。
- 浏览器视觉复核受限（截图/resize 不可靠），构图与动效观感建议在网络恢复后于宽屏
  人工复核一次；逻辑层与无障碍结构未经改动，无回归风险。

### Deployment（第八轮，并入 18080）

- 沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久部署，
  容器重建回退）；Docker Hub 仍不可达，固化待办同前（网络恢复后重建镜像）。

---

## 第九轮（2026-08-13）— 中区紧凑化与平衡填充

### Why

用户反馈"网页元素中间显得空旷"：根因并非某个细节，而是 `.login-intro` 原为
`display:grid; grid-template-rows: auto auto minmax(0,1fr)`，且 `.login-visual`
（拓扑块）以 `align-self:end` 锚定在 `minmax(0,1fr)` 行的底部——标题块（品牌+标题+
描述+特性）停在上方，拓扑块沉在下方，二者之间的 `1fr` 行被撑开成一段大尺度"死区"。
第八轮的柔光景深 / 信号脊柱只是背景氛围，不足以填实这段空白。

### What changed

- **结构性修复**：`.login-intro` 由 `grid`+底部锚定改为
  `display:flex; flex-direction:column; justify-content:flex-start; gap:clamp(16px,2.2vh,30px)`。
  品牌 → 标题 → 状态面板 → 拓扑成为连续纵向栈，中区空洞收敛为富有节奏的统一间距。
- **关键踩坑（cascade 透传）**：`base.css` 的 `.login-intro` 设了
  `justify-content:space-between`，而 console-theme 的覆盖规则此前未声明该属性，导致
  `space-between` 透传、把列重新撑开（中区空洞复活）。已在 console-theme 显式
  `justify-content:flex-start` 抵消，并在产物中验证生效。
- **中区填充块 `.login-signal-strip`（aria-hidden）**：新增一处"实时信号"状态面板——
  顶部分隔线 + 呼吸点 + 标签 `实时信号 · LIVE OPERATIONS`，下方 3 枚图标统计胶囊
  （在线集群 12 / SLO 达标 99.9% / 待处理告警 3），把空旷中区赋予明确意图（状态概览），
  而非松散装饰。胶囊 hover 与能力卡同款微抬升。
- **间距收紧（视觉平衡）**：`.login-copy` 去除过大的 `padding-top: clamp(28px,7vh,76px)`
  （改由 flex `gap` 统一节奏）；`.login-description` 收紧 `margin-top:14px` + `max-width:540px`
  + `line-height:1.62`；`.login-features` `margin-top:18px`。
- **响应式**：≤1080px 收紧状态面板内边距与数字字号；矮屏（max-height:760 & min-width:721）
  保留状态面板并紧凑化（`margin-top:10px` + 减 padding）；移动端（max-width:720）与
  `.login-depth` / `.login-spine` / `.login-radar` / `.login-footer` 同策略 `display:none`。
- 全部纯视觉层改动，未动表单逻辑与无障碍结构；状态面板 `aria-hidden`，不污染无障碍树。

### Verification

- `./node_modules/.bin/vite build`：✓ built，exit 0，无 CSS 语法错误。
- 线上验证：新产物 `index-B2k1Maxs.css` + `index-DSssqMZx.js` 已 docker cp 覆盖进
  `k8s-aiops-frontend-1`，`curl /login` 引用新 hash；CSS 实测 console-theme 的
  `.login-intro` 同时含 `justify-content:flex-start` 与 `padding-right:clamp(460px…)`
  （证伪 base.css 覆盖），且 `login-signal-strip` / `login-signal-grid` /
  `login-signal-ico` / `login-signal-dot` 全部新规则 present。
- 浏览器视觉复核仍受限（截图/resize 不可靠），构图与节奏观感建议在网络恢复后于宽屏
  人工复核一次；逻辑层与无障碍结构未经改动，无回归风险。

### Deployment（第九轮，并入 18080）

- 沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久部署，
  容器重建回退）；Docker Hub 仍不可达，固化待办同前（网络恢复后重建镜像）。

---

## 第十轮 · 去伪数据，接入真实控制台状态（#login-ambience）

### Why

第九轮为填充中区新增的 `.login-signal-strip` 使用了**硬编码虚构指标**
（在线集群 12 / SLO 达标 99.9% / 待处理告警 3）。这违反"页面内容均为有效数据"
的要求。本轮移除全部虚构展示，改用真实、可获取的信息。

### 真实数据来源

- 公开健康接口 `GET /api/v1/health/live`（backend/internal/httpserver/health.go，
  路由注册于 router.go:195，**无 `AuthRequired`**，对未登录用户开放）返回
  `{ status, service, version, checked_at }`：
  - 服务状态：映射 `status==="ok" → 正常`；非 ok 显示原始状态；请求失败/非 200 →
    `未响应`（如实反映，绝不编造数字）。
  - 平台版本：取 `version` 字段；`dev`/`unknown`/空 → `开发版`，正式 semver 自动补 `v` 前缀。
- 本地时间：由浏览器 `new Date()` 实时驱动（每秒刷新），属真实本地时间，不依赖后端。

### 改动

- `LoginView.vue`：移除 `Bell` 导入、新增 `Clock` 与 `onMounted`/`onBeforeUnmount`；
  新增 `consoleHealth`/`healthError`/`localTime` ref 与 `statusLabel`/`versionLabel`/
  `statusDotClass` computed；挂载时拉取健康并每 30s 轮询，时钟每秒刷新；卸载清理定时器。
  状态条模板绑定真实字段，标题由"实时信号 · LIVE OPERATIONS"改为
  "控制台状态 · CONSOLE STATUS"（语义与真实数据一致）；装饰层仍 `aria-hidden`。
- `console-theme.css`：状态指示点新增 `.is-waiting`（检测中·琥珀）/ `.is-error`
  （异常/未响应·红），颜色随真实状态联动；其余 `.login-signal-*` 样式沿用。

### Verification

- 公开健康接口实测可达：`curl /api/v1/health/live` →
  `{"status":"ok","service":"k8s-aiops-api","version":"dev","checked_at":"…"}`，确为真实后端数据。
- 新建 `LoginView-DelsYB2k.js` 含 `health/live` 拉取逻辑；旧虚构串
  `99.9%` / `在线集群` / `SLO 达标` / `待处理告警` / `LIVE OPERATIONS` 全部 absent。
- CSS 含 `is-waiting` / `is-error`；`/login` 引用新 entry `index-CZlwC1f_.js`。
- 说明：本预览构建 `version` 为 `dev`（未注入语义化版本），故展示"开发版"；正式构建含
  semver 时自动显示 `vX.Y.Z`。"未响应"仅在后端不可达时出现，属真实状态而非占位。

### Deployment（第十轮，并入 18080）

- 沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久部署，
  容器重建回退）；Docker Hub 仍不可达，固化待办同前（网络恢复后重建镜像）。

## Round 11 — 底部文字统一与图形居中对齐

### 问题

用户反馈登录页底部：
1. 文字样式不统一：特性卡片小标签（`多集群治理` 等）与 footer 小字在字号、字重、
   颜色、行高、字间距上不一致，视觉零散。
2. 图形偏左：`.login-topology`（节点拓扑 SVG）max-width 620px 且左对齐，而下方
   `.login-capabilities` 只有 540px 且居中，导致拓扑图明显偏左、与能力卡片错位，
   整体底部显得突兀。

### 改动

- `frontend/src/views/LoginView.vue`
  - 简化 `login-footer`：把 copy + tags（带 dot 分隔）的两段式结构合并为单行居中文字，
    去掉 `<i>` 装饰点与 `login-footer-copy/login-footer-tags` 类名。
- `frontend/src/styles/console-theme.css`
  - `.login-visual`：改为 flex column + `align-items: center`，并添加
    `margin-top: auto; align-self: center;`，使整个底部装饰柱（拓扑 → 能力卡 → footer）
    在左区水平居中并锚定到底部。
  - `.login-topology`：`max-width` 由 620px 收紧为 540px，与能力条同宽，
    `margin-inline: auto` 保证居中。
  - `.login-capability span`：统一为 caption 规范
    `11px/500/0.4px letter-spacing/line-height 1.35`，颜色改为
    `rgba(148,163,184,0.78)`，与 footer 小字一致。
  - `.login-capability strong`：由 `#ffffff 19px/700` 调整为
    `rgba(248,250,252,0.98) 18px/650/line-height 1.25`，降低侵略性。
  - `.login-visual > .login-footer`：从 `.login-intro` 的绝对定位改为 visual 内部普通流，
    宽度 540px 居中，字号/字重/颜色/行高/字间距与 capability span 对齐，顶部保留
    点状分隔线。
  - 新增 `M93-B1f` 注释块归档本轮改动。
  - 响应式同步：1080px 下 capability span / footer 同步缩小为 10px；
    移除矮屏断点中已废弃的 `.login-visual { padding-bottom: 0 }`。

### Verification

- 产物 `index-Dfcxq6tu.css` + `index-BP5Gmvn2.js`；`/login` 引用新 hash。
- CSS 实测最终 `.login-visual` 含 `margin-top:auto; align-self:center; align-items:center`。
- CSS 实测 `.login-topology` 含 `max-width:540px; margin-inline:auto`。
- CSS 中旧结构 `bottom:22px`、`login-footer-copy`、`login-footer-tags` 已全部消失。
- JS 产物中已无 `login-footer-tags` 字符串。

### Deployment（第十一轮，并入 18080）

- 沿用 `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久部署，
  容器重建回退）；Docker Hub 仍不可达，固化待办同前（网络恢复后重建镜像）。
