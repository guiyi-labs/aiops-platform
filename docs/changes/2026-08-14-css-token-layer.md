# CSS Token 层收口（Track A · 主题收敛 · 第一批安全迁移）

- Date: 2026-08-14
- Status: Complete
- Scope: `frontend/src/styles/{base,console-theme,motion,premium-ui}.css` + 新审计脚本

## Context

Track A「主题收敛」要求四层样式建立**可审计的 CSS token 层**并清理失效覆盖。
现状：`base.css`/`console-theme.css` 共 3 个 `:root` 块（base 浅色 / console 蓝 /
M93-C 青绿），token 体系已存在但大量规则仍硬编码颜色字面量（`#ffffff`×85、
`#2dd4bf`×14 等），且部分字面量与主题 token 值不一致（如 `#5a6672`×45 是旧调色板
遗留，与 `--text-muted:#66777d` 不符），无法被审计。

## What Changed

### 新脚本 `scripts/audit-css-tokens.mjs`
- 计算四层级联下每个自定义属性的**有效值**（`var(--x)` 引用解析，后 `:root` 覆盖先）。
- 扫描全部规则中的颜色字面量（hex/rgb/rgba），跳过 `:root` 定义、注释与
  `var(--x, fallback)` 回退区。
- 字面量分类：**MATCHED**（存在有效值完全相等的 token → 可零风险替换为 `var()`）
  与 **ORPHAN**（无精确 token，含不一致遗留值，量化留待后续视觉变更轮）。
- 三模式：`--apply`（按位置从后往前安全替换）、`--check`（残留 MATCHED 即 exit 1，
  可作为 CI 门禁）、默认（审计报告）。

### 第一批迁移（仅精确值匹配，像素零变化）
- `base.css`：85 处替换（`#ffffff/#fff`→`var(--gray-0)`×80、`#edf2f3`→`var(--gray-100)`×2、
  `#dcfce7`→`var(--success-bg)`、`#fee2e2`→`var(--danger-bg)`、`#26796d`→`var(--brand-600)`）。
- `console-theme.css`：27 处替换（`#2dd4bf`→`var(--blue-400)`×13、`#ffffff`→`var(--gray-0)`×10、
  `#15803d`→`var(--status-success)`×2、`#edf2f3`→`var(--gray-100)`、`#14b8a6`→`var(--blue-500)`）。
- `motion.css` / `premium-ui.css`：本就干净，0 替换。
- 合计 112 处；替换后 `--check` PASS（MATCHED=0）。

### 遗留（有意保留，供后续轮次）
- `#5a6672`×45、`#dfe5e8`×44、`#5eead4`×20 等无精确 token 的字面量：其中部分与
  主题 token 值不一致（旧调色板），若直接换 token 会改变像素，需按视图逐个走
  基线重建流程，不纳入本轮零风险迁移。

## Verification

- `node scripts/audit-css-tokens.mjs --apply`：112 替换成功；`--check`：PASS。
- `./node_modules/.bin/vite build`：✓ 2.25s；新产物 `index-Ctmh9Q5N.css` 已
  `docker cp` 覆盖 `k8s-aiops-frontend-1:/usr/share/nginx/html/`（非持久）。
- **像素基线回归**：`node scripts/capture-ui-baselines.mjs --verify` 10/10 全绿——
  login@1440x900 diff `0.000%`（0/1,294,812 px），其余 9 条 sha256 完全一致。
  证明 112 处替换全部为解析值等价的纯 token 化，无任何视觉变化。

## Follow-up

- 遗留不一致字面量（`#5a6672` 等）逐视图评估 → 更新 token 值或替换后重建
  `docs/ui-baselines/` 基线，作为后续"主题收敛第二轮"。
- 可把 `--check` 接入前端 CI（样式层 token 门禁）。
