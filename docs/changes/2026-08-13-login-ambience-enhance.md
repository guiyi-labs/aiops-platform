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

## Risks / Notes

- **容器重建后回退**：18080 当前为容器内覆盖的 dist，`docker compose up -d`
  或容器重建会回到镜像旧版；请尽快在网络恢复后重建镜像（见 Deployment 节）。
  建议后续在宽屏浏览器人工复核一次 radar 与能力列表/标题的实际叠加效果。
- radar 与内容层 z-index 同为 1（DOM 顺序 radar 在前），属于"装饰衬于内容下"的
  预期分层；若后续内容区需要更高叠层，请同步调整。
- 全部新元素 `aria-hidden="true"` 或 `role="list"`，不影响无障碍树与表单可访问性。
