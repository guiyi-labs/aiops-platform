# sidebar-three-tier：侧栏按研究范围分三层收敛

- Date: 2026-11-01
- Status: Complete
- Scope: 仅改导航呈现（分组、顺序、默认折叠），不删路由、不删页面、不改后端

## Context

平台累计 73 个后端包、38 个前端视图，侧栏 5 组共 35 个入口**全部平铺在首屏**。
而项目声明的功能需求清单只有 6 项（F1–F6），两侧口径差了一个数量级：外部看到的
"功能点数量"等于侧栏条目数，而不是那 6 项。一场 10–15 分钟的演示也不可能翻 35 个页面。

因此本次不改功能，只把"读者看到的范围"和"需求清单声明的范围"对齐。

## What Changed

### 导航分层（`frontend/src/components/ConsoleLayout.vue`）

- 5 组 35 项重组为 3 层：
  - **核心能力（6 项）**：总览、AIOps 概览、智能诊断、资源拓扑、关联案例、解释覆盖率
  - **平台能力（10 项）**：集群、资源工作台、事件中心、事件流与告警、告警规则、智能巡检、
    质量仪表盘、审计日志、用户管理、模型接入
  - **扩展能力（19 项）**：其余页面，`defaultCollapsed: true` 首次进入即收起
- `isGroupExpanded` / `toggleGroup` 增加 `defaultCollapsed` 参数；**用户手点的状态存在
  localStorage 里，优先级仍高于默认值**，不覆盖既有用户习惯。
- 分组按钮 `title` 显示分组用途（如「平台既有能力，非本文研究范围」）；侧栏折叠成图标栏时
  仍显示分组名。
- 所有路由、页面、`adminOnly` / `auditOnly` 权限过滤、pin 收藏机制均保持不变。

### 折叠容器高度（`frontend/src/styles/console-theme.css`）

- `.nav-group-items` 的 `max-height` 由 `600px` 提到 `900px`。
- 原因：收起动画靠 `max-height` 插值，必须是够大的定值；扩展组 19 项实测内容高 **722px**，
  600px 会把末尾 3 项裁掉（`overflow: hidden` 下既看不到也点不到）。这是本次分组引入的
  真缺陷，已在浏览器内实测确认修复（`clientHeight=722`、`contentClipped=false`）。

## Verification

- `vue-tsc -b`：exit 0。
- Playwright（1440×900，mock API）：3 组分别 6 / 10 / 19 项，扩展组 `aria-expanded=false`、
  容器 `clientHeight=0`、`opacity=0`；点击后 `aria-expanded=true`、19 项全部可见。
- 移动端 390×844：`scrollWidth === clientWidth`，无横向溢出。
- `node scripts/audit-a11y-axe.mjs`（对 dev server，64 次扫描）：critical/serious/moderate/minor
  全为 0。该脚本最终仍以非零退出，原因是本地后端对这些路由返回 502 触发 `appErrors` 断言，
  与本次改动无关。
- `node scripts/bundle-gate.mjs`：通过，entry JS gzip 55.6 kB、总 JS 289.0 kB，与改动前一致。
- **像素基线未重录，属有意为之**：`scripts/capture-ui-baselines.mjs` 抓的是
  `AIOPS_UI_BASE`（默认 `127.0.0.1:18080`）上的**已构建产物**，而该实例跑的是改动前的 bundle，
  截图里的侧栏仍是旧分组（新旧截图逐像素相同即为证）。同时该实例有 6 个视图当前返回接口错误
  （如 `/namespace-posture` 渲染 `unable to aggregate namespace posture`），此时重录会把错误态
  固化进基线。**正确做法是重新构建并重启前端后再录基线。**

## 后续

- 重新构建前端产物后，执行 `node scripts/capture-ui-baselines.mjs` 重录基线，
  再跑 `node scripts/ui-gate.mjs` 确认 4/4。
