# 移动端适配与暗色主题

日期：2026-10-30
范围：`frontend/src`（样式系统、ConsoleLayout、useTheme、新增 useResponsiveTables）、`scripts/audit-a11y-dark.mjs`

## 背景

从「还有哪些该做但没做」这个问题出发，跑了移动端与主题两条线的实测。两条线各有一个**看起来能用、实际不成立**的问题：

- 移动端：顶栏 112px（375px 屏宽的 14%），每页 6~15 个触摸目标小于 44px；8 列的表被压成 47px/列，「INC-000006」断成两行、状态徽章竖排。
- 主题：顶栏有「切换亮色主题」按钮，点击后按钮图标翻转，**画面没有任何变化**。

后者是产品诚实性问题：用户点了以为生效，其实没有。

## 一、移动端顶栏：112px → 61px

### 现状的成因

`@media (max-width: 720px)` 里写的是：

```css
.topbar { grid-template-columns: auto minmax(0, 1fr); }
.topbar-actions { grid-column: 1 / -1; }   /* 整排操作换到第二行 */
```

375px 宽放不下「侧栏开关 + 标题 + 用户 + 头像 + 主题 + 账号 + 退出」七个元素，于是把后四个挤到第二行。行高 2×56px 左右 = 112px，占 14% 屏幕。

### 改法

顶栏保持单行三列，操作收进「更多」面板：

```
[侧栏开关 44] [标题 flex] [更多 44]
                          └─ 点开后：绝对定位面板，不参与行高
```

- 触发按钮 `.topbar-more` 桌面端 `display: none`，桌面布局零变化
- 面板用 `position: absolute` 挂在顶栏下方，**不占行高**——这是顶栏能稳定在 61px 的关键
- 面板内每个入口 44px 高、整行宽，用 `::after { content: attr(aria-label) }` 从 `aria-label` 取文字，避免在模板里重复写一遍文案
- 关闭逻辑：`pointerdown` 监听外部点击（不是 `click`——`click` 要等 `pointerup`，在面板外「按下-拖走」不会触发，手感上像没反应）、Esc 关闭、路由切换关闭

### 触摸目标

`.page-content` 内的块级控件直接 `min-height: 44px`；行内小元素（`.link-button`、表格里的「查看」）不撑高——那会把表格行整体顶开——改用伪元素把命中区扩到 44×44。

复选框先放大到 20px（13px 几乎点不中），完整解法在卡片化里：卡片上整行可点，不必瞄准小方框。

实测：概览页触摸目标违规 6 → 0；其余页面剩余的少量小目标（勾选框、链接）由卡片化承接。

## 二、移动端表格卡片化

### 为什么不能靠缩字号

8 列的表在 375px 下每列 47px。列数不变，每列可分宽度是固定的——缩字号只会让「放不下」变成「看不清」。

### 为什么不做 27 个视图逐个改

常见做法是给每个 `<td>` 手写 `data-label`。这里有 27 个视图、上百个表格列，手写既容易漏，也容易和表头文案分叉。

改成**运行时注入**：`composables/useResponsiveTables.ts` 读 `thead` 的列名，写到对应 `td` 的 `data-label`。表头是唯一事实来源，改表头文案时卡片标签自动跟着变。

只在 `matchMedia('(max-width: 720px)')` 命中时启用，桌面端零开销。

### 三个踩过的坑

1. **观察器不能只看「新增节点是 table」**。表格行是异步插入的：首帧扫到的还是空 `tbody`，数据到达后插入的是 `<tr>`，会被漏掉。症状是卡片化生效了但一格标签都没有。改为任何子树变化都重扫，用 `requestAnimationFrame` 节流到每帧一次。

2. **`min-width: 620px` 会把卡片撑出屏幕**。桌面表格为避免列被压扁设了 `min-width`，配合外层 `overflow-x: auto` 提供横向滚动。卡片化后这个约束必须解除，否则每张卡仍是 620px 宽、值全被推到可视区外（实测 `td` 宽 596px 而视口只有 375px）。

3. **勾选列写在 scoped style 里**。`.incident-list .select-cell { width: 34px }` 被 scoped 追加 `[data-v-*]` 抬到 (0,3,0)，普通选择器压不住，得用 `!important`。作用域已由 `data-responsive` 属性限定在移动端卡片上，不会外溢。

### 空表头不编标签

操作列的 `<th>` 常常是空的。最初编了「第 8 列」，读起来既不是字段名，又让人以为那里本该有标题。改为：只有勾选列给「选择」，其余空表头不设 `data-label`，`::before` 自然不渲染。

## 三、暗色主题：从「没有」到「真的能用」

### 诊断

`useTheme` 会往 `<html>` 加 `theme-dark` / `theme-light`，但：

- `.theme-dark` 在 4 个 CSS 文件里规则数 **0**：默认 `theme='dark'` 落到的其实是 `:root` 上的浅色 token，所以「暗色主题」渲染出来是浅色
- `.theme-light` 有 46 条规则，但只调了外壳（顶栏、侧栏、卡片），画布本身不变

所以按钮点击后：图标翻转、`data-theme` 变化、`localStorage` 写入，**像素一个没动**。

诊断过程中我自己也踩了个坑：第一版 CSSOM 遍历用 `if (r.cssRules)` 判断容器，而现代浏览器里 `CSSStyleRule.cssRules` 也存在（空列表），于是所有普通样式规则都被当成容器跳过，结论一度是「生产 CSS 里一条 theme-light 都没有」——实际有 46 条。

### 改法

1. **默认主题改为 `light`**。补了暗色规则之后 `dark` 会真的把界面变暗，如果仍默认 dark，所有用户的首次观感会被改变；显式改成 light 保持既有外观，让用户主动选择。
2. **新增 `.theme-dark` 的完整语义 token**：surface / text / border / status / shadow。不动 `--blue-*` 与 `--gray-*` 调色板——灰阶被大量硬编码直接引用，整体反转会让徽章文字之类的意外变色。
3. **补 `color-scheme: dark`**，让原生控件（下拉、滚动条）跟随。

### 颜色 token 化（暗色的前提）

暗色能成立的前提是「颜色都来自 token」。实际情况是：

| 类别 | 数量 | 处置 |
|---|---|---|
| `color: var(--gray-800/700/600/500)` | 31 | → `--text-primary` / `--text-secondary` / `--text-tertiary` |
| 深灰字面量（`#3c4a51`、`#52615b`…） | 60 种 | 按亮度/饱和度自动映射到 text token |
| `background: var(--gray-0/50/100/200)` | 90+ | → `--bg-elevated` / `--bg-secondary` / `--bg-primary` / `--bg-tertiary` |
| 浅色状态底（`#eaf5f1`、`#fff3d8`…） | 84 种 | 按色相映射到 `--success-bg` / `--warning-bg` / `--danger-bg` |
| 深色状态文字（`#a43b2e`、`#8c6225`…） | 365 处 | → `--status-danger` / `--status-warning` / `--status-success-strong` |
| `background: #fff`（含 scoped style） | 20 | → `--bg-elevated` |
| `var(--panel-bg, #ffffff)` | 1 | 该 token 从未定义，一直在回落成白色 |

自动映射的边界：饱和度阈值以内的青绿色（`#3d7168`、`#486b3e`）被误判成灰，会把「选中态强调」和「configmap 图标绿」变灰。逐个改回 `--accent-strong` / `--status-success-strong`——**语义色不能交给亮度算法判断**。

### 一个必须拆开的 token

`--accent-strong` 原本同时承担「强调色当文字」与「承载白字的实心背景」。这两个需求在浅色主题下方向一致（都要深），暗色下**方向相反**：文字要亮、背景要深。同一个变量无法同时满足，于是拆出 `--accent-fill`（浅色 `#0a6f66` / 暗色 `#2455b8`）。

同理新增 `--status-denied`（浅色紫 `#7b5a78` / 暗色 `#c2a3bf`），与红（失败）区分开。

## 四、验证

| 项 | 结果 |
|---|---|
| `ui-gate` 四步 | **4/4 PASS** |
| 浅色 axe | 0 critical/serious |
| 暗色 axe（新增脚本） | 0 critical/serious（覆盖 7 个视图） |
| 全站巡检 | 36 路由 / 0 异常 |
| 移动端顶栏 | 112px → **61px** |
| 移动端触摸目标 | 概览页 6 → 0 |
| 暗色纯白背景元素 | 0 |

`scripts/audit-a11y-dark.mjs` 是新增脚本。ui-gate 里的 axe 只测默认（浅色）主题；只测浅色就发布暗色，等于有一半配色没验过。

## 五、已知限制

- **卡片化依赖 `thead` 的表头文案**。如果某张表的表头是图标或分组（`th` 带 `colspan`），列名会取不准；当前只取第一个 `thead tr` 的文本。现有 27 个视图均无此情况，新表格需注意。
- **运行时会改 DOM**。`useResponsiveTables` 直接给 `td` 写 `data-label`，属于 Vue 之外的手动 DOM 操作。它只写属性、不改变结构，Vue 的 patch 不会清除；但如果将来引入会整体重建 `<tr>` 的键策略，注入需要重新触发——现有实现靠 MutationObserver 兜住了这一点。
- **暗色未覆盖 canvas / SVG 的内部绘制颜色**（拓扑图的节点、图表线）。这些颜色写在 JS 里而不是 CSS，本轮未动；实测暗色下拓扑图与图表仍可辨，但未做逐像素核对。
