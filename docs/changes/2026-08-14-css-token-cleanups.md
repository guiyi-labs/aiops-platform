# CSS token 层第一轮收尾：console-theme 遗留字面量清零

- Date: 2026-08-14
- Status: Complete
- Scope: `frontend/src/styles/console-theme.css` 纯 refactor（token 化，零视觉变化）

## Context

`audit-css-tokens.mjs` 第一轮迁移 112 处后，报表仍显示 console-theme 2 处可替换字面量
（`#b91c1c`）。本轮消除，使四层 CSS 全部 `replaceable=0`。

## What Changed

- `console-theme.css:504` `.status-pill.unavailable`：`color: #b91c1c` → `var(--status-danger)`
- `console-theme.css:2363` `.login-page--error .login-card-rail i`：`background: #b91c1c`
  → `var(--status-danger)`

## Verification

- `scripts/audit-css-tokens.mjs`：四层全部 `0 replaceable literals`（clean）。
- `pnpm typecheck` / `vite build` ✓；部署后 `capture-ui-baselines.mjs --verify`
  62 条全绿（59 IDENTICAL + PASS），Git 无基线产物 diff → 纯 refactor 零视觉变化。

## Risks / Notes

- 仅 token 引用替换，值不变；`#b91c1c` 即 `--status-danger` 当前定义值。
