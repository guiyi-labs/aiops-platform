# M91: 前端窗口化虚拟滚动 + 规模性能基线

- Date: 2026-08-09
- Status: Development Complete（typecheck / vitest 23+6 / build 全绿）
- Scope: 前端规模性能（本地可做项）

## Context

为 500+ 节点 / 50k pod 规模预留前端渲染能力。大表一次性渲染全部行会
卡顿，M91 引入零依赖的窗口化虚拟滚动，让 Pod 资源表只渲染视口内的行，
并附单元测试覆盖窗口计算与边界。

## What Changed

- 新增 frontend/src/composables/useVirtualList.ts：
  - computeWindow：纯函数窗口计算（total/scrollTop/viewportHeight/rowHeight），
    带 overscan；修复滚过末行返回空窗口、viewport=0 返回空两个边界缺陷；
  - useVirtualList：reactive 绑定滚动容器，rAF 节流更新视口高度与 scrollTop；
  - windowRows：输出 visible/topPad/bottomPad，供模板 spacer 渲染。
- 新增 6 条 Vitest（useVirtualList.test.ts）：顶部窗口、中部带 overscan + offsetY、
  越过末尾返回空、空列表、非正行高/viewport 钳制、spacer 高度守恒。
- WorkloadsView.vue 接入 Pod 表：
  - 滚动容器 max-height 560px + overflow-y；thead 用 position: sticky 固定表头；
  - 模板用 podVisibleRows 窗口渲染 + topPad spacer，空表仍显示占位。
- base.css 新增 .virtual-list-scroll + sticky thead 规则。

## Acceptance

- pnpm typecheck / pnpm vitest run（23 files · 130 tests）/ pnpm build 全绿；
- 后续扩展：Deployment/Node/Service 等大表可复用 useVirtualList；
  聚合缓存与 500 节点 fixture 渲染留待真实集群基准（依赖 P0 kind E2E 产物）。
