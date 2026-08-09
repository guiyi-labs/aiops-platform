# M92：交互式登录页粒子网络与集群拓扑视觉系统

- Date: 2026-08-09
- Status: Complete（代码与运行产物已验证；自动化回归增强列入 M93）
- Scope: 登录页 Canvas2D 粒子网络、SVG 集群拓扑、展示指标、动效与响应式适配。

## Context

M80/M87 已建立 Aurora 登录背景与统一动效层，但登录页仍以静态文案为主，无法在首屏
传达多集群 AIOps 的产品特征。M92 在不改变认证流程和安全契约的前提下，引入一套
可降级、可交互的运维拓扑视觉系统。

## What Changed

### 交互式粒子网络

- 新增 `frontend/src/components/ParticleNetwork.vue`：Canvas2D 粒子场，按画布面积在
  28–90 个粒子间自适应；支持近邻连线、鼠标/触屏吸引、边界环绕与 2x DPR 上限。
- `prefers-reduced-motion: reduce` 时仅渲染静态帧，不启动 RAF 循环。
- 画布使用 `aria-hidden="true"`，不进入辅助技术阅读顺序。

### 登录页视觉与语义结构

- `frontend/src/views/LoginView.vue`：加入 ParticleNetwork、6 个成员节点 + 1 个 Hub 的
  SVG 拓扑、数据流连线、三组计数展示，并将左侧结构拆为 `login-copy` / `login-visual`。
- 复用 `useCountUp` 渲染 12 / 186 / 99 三组展示值；认证、错误处理和跳转逻辑不变。

### 样式与响应式

- `frontend/src/styles/console-theme.css`：新增分层 z-index、错峰入场、拓扑流光、Hub 脉冲、
  节点呼吸、指标卡、表单聚焦与 hover 微交互，以及径向遮罩网格背景。
- `frontend/src/styles/base.css`：移动端通过语义类 `.login-visual` 隐藏视觉区，替代脆弱的
  `nth-child` 选择器。

### 基线与计划同步

- 更新 `README.md`、`docs/PROJECT_STATUS.md` 与 `docs/long-term-roadmap.md` 到 M92。
- 重写 `docs/next-long-term-plan.md`，将下一阶段拆为 M93–M97，并为每个里程碑定义验收证据。

## Change Size

- 4 个文件：`+631 / -7`，净增 624 行。
- 其中新组件 284 行；普通 `git diff --stat` 不包含未跟踪文件，因此仅显示已跟踪文件
  `+347 / -7`。

## Verification

- 用户侧验证：`vue-tsc` 通过；`pnpm build` 通过（35.78s）；Docker 静态文件更新并
  reload nginx；登录功能与三类动画正常；Canvas 实测 560×794px，层级 canvas=1/content=2。
- 本会话复核：`frontend/dist/index.html` 于 2026-08-09 18:24:51 重新生成；构建产物中
  可检索到 `particle-network` / `dash-flow` / 登录指标；`http://127.0.0.1:18080/login`
  返回 HTTP 200。
- 本会话未重新执行 Node 门禁：当前沙箱无法读取既有 pnpm hard-link `node_modules`，
  全局 pnpm 又会尝试重建依赖目录，因此保留用户侧通过结果，不虚构二次执行。

## Risks / Notes

- 三组数字当前是硬编码展示值，并非后端实时指标；对外发布前必须接入可信数据源，或
  明确标记为演示/能力基线，避免误导。
- ParticleNetwork 尚无独立组件测试；M93 应补 reduced-motion、ResizeObserver、
  Page Visibility 暂停和 Playwright Canvas 像素/布局断言。
- 粒子连线复杂度为 O(n²)，但粒子数上限 90；M93 仍需在低端移动设备上做帧耗验证。
