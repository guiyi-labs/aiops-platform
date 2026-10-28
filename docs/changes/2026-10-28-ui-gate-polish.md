# ui-gate-polish：CSS token 清零、移动端触摸目标与 UI 门禁修复

- Date: 2026-10-28
- Status: Complete（含两处已知缺口，见文末）
- Scope: 前端样式（tokens/移动端/警示色）与 `scripts/` 下两个 UI 门禁脚本

## Context

`ui-gate.mjs` 是前端视觉与可访问性的总门禁，四步：CSS tokens → 像素基线 → axe a11y → bundle。
本次之前它**跑不过第 1 步**（26 处 CSS 字面量），且第 3 步根本无法启动。逐项处理后：

- 第 1 步：26 处字面量全部替换为等价 token；
- 第 2 步：基线已过期（连登录页都差 3.16%，而登录页无数据变化），重新生成；
- 第 3 步：脚本读不到 `axe-core`（版本号写死），修好后暴露出一批对比度问题；
- 第 4 步：本就通过（entry JS gzip 55.6 kB / 总 JS 289 kB / CSS 67.5 kB）。

## What Changed

### 样式（frontend/src/styles）

- `console-theme.css`：26 处硬编码色值改为等价 `var(--token)`。由 `scripts/audit-css-tokens.mjs --apply`
  生成——该脚本只在「字面量与某 token 的**有效值完全相同**」时才替换，属值等价变换，不改变渲染。
- `console-theme.css`（`@media (max-width: 720px)`）：
  - `.nav-item` 增加 `height: 44px`（原 38px，低于 WCAG 2.5.5 / iOS HIG 的 44px 触摸目标；桌面保持 38px 密度）；
  - `.nav-group-chevron { display: none }`：移动端轨道仅 64px、分组标签被隐藏，箭头仍渲染就成了孤立图标。
- `base.css` / `console-theme.css`：`--status-warning` 由 `#c27d1d` / `#d97706` 统一为 **`#b45309`**。
  原值在浅底上对比度仅 2.86–3.18（AA 要求 4.5），改后约 5.0。这一项让 dashboard 的对比度告警节点从 6 降到 5，
  但**并未清完**（见文末）。

### 门禁脚本（scripts）

- `capture-ui-baselines.mjs` · `login()`：已登录时路由守卫会在文档 load 之后把 `/login` 重定向走，
  原实现只等 `#username`，会在这个窗口里 20 s 超时（实测：第 2 个视图起必失败）。改为等
  「表单出现 **或** 已离开 `/login`」二者之一，再决定是否填表。
- `audit-a11y-axe.mjs`：
  - `axe-core` 路径原写死 `axe-core@4.12.1`，而 `@axe-core/playwright@4.13.0` 安装的是 `axe-core@4.13.0`
    → 读文件直接 `MODULE_NOT_FOUND`，第 3 步从未真正执行。改为动态解析（先找顶层 `node_modules/axe-core`，
    再回退到 `.pnpm` 下任一 `axe-core@*`），找不到时给出可执行的提示。
  - `login()` 同上修正。
- `docs/ui-baselines/`：按 README 的「提交入库前重生成」重新生成（62 条目）。视觉变化来自上面的警示色与移动端改动。

## Verification

- `node scripts/audit-css-tokens.mjs --check` → `TOTAL replaceable: 0` / PASS（原 26）。
- 375×812 实测：`.nav-group-chevron` 可见数 **0**（原 5）；`.nav-item` 最小高度 **44.0px**、最小宽度 47.0px。
- `node scripts/bundle-gate.mjs` → PASS。
- `node scripts/audit-a11y-axe.mjs` → 现在能完整跑完 31 个视图（此前 `MODULE_NOT_FOUND` 直接崩）。
- `node scripts/ui-gate.mjs` → 第 1、4 步 PASS；第 2、3 步仍 FAIL（原因见下）。

## Risks / Notes

**已知缺口 1：像素基线在「活数据」视图上不稳定。**
`capture-ui-baselines.mjs` 的每个视图都有 `masks` 字段用于遮掉动态区域，但**31 个视图的 `masks` 全是空数组**。
dashboard 有指标轮询、gitops 有能力探测横幅，重生成基线后立刻校验仍差 0.21%–0.46%（超过 0.2% 阈值）。
解决方向是逐视图补掩码（KPI 数值、图表、探测横幅），这是一次独立普查，不应与本次改动混做。

**已知缺口 2：对比度尚未清完。**
修掉 `--status-warning` 后，仍剩 **7 个**低对比度色值分布在 6 类组件上：
`#929c98`、`#8a6d10`、`#15803d`、`#0f9588`、`#9aa39f`、`#8a969c`、`#607278`
（`.metrics-consumer-row`、`.burn-card`、`.active`、`.topology-stack-group`、`.resource-category-tabs`、`.is-muted`）。
这些是分散的组件内色值而非单一 token，需要逐个核对背景色后调整，属设计 token 普查。

**风险**：基线 PNG 已重新生成，因此本次提交包含较多二进制变更；审查时请以 `manifest.json` 的条目数
（62）与上面两条缺口说明为准，不要仅凭图像差异判断回归。
