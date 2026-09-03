# Light theme sidebar adaptation + CI lint fixes

**Date:** 2026-09-01
**Scope:** frontend

## Summary

Follow-up to the theme-switching feature: the sidebar now follows the active
theme (dark/light) instead of staying dark, and CI lint failures from the
UX-enhancement commit are fixed.

## Changes

### 1. Sidebar follows theme switching

Previously the sidebar intentionally stayed dark in light theme ("brand
identity"). User feedback requested it follow the theme. Added `.theme-light`
overrides for the sidebar: light gradient background, dark text, adapted
nav-item hover/active states, workspace-selector, and group labels. The login
page intentionally stays dark.

**Files:** `console-theme.css`

### 2. CI lint fixes

The UX-enhancement commit introduced five ESLint errors that failed the CI
frontend job:

- `EmptyState.vue`: `icon?: any` → `icon?: typeof Inbox`
- `Toast.vue` → renamed to `AppToast.vue` (single-word component names are
  forbidden by `vue/multi-word-component-names`); fixed unused `props` binding
  by using bare `defineProps<>()` (Vue `<script setup>` exposes props to the
  template automatically)
- `useTheme.ts`: empty `catch {}` blocks → commented fallbacks (no-empty)

**Files:** `EmptyState.vue`, `AppToast.vue` (renamed from `Toast.vue`),
`useTheme.ts`

### 3. e2e a11y & smoke fixes

The CI frontend e2e job failed with axe violations and smoke timeouts caused by
the sidebar nav changes:

- `nested-interactive` (34 nodes): the `pin-toggle` `<button>` was nested inside
  the `nav-item` `<button>` — invalid HTML. Wrapped each nav item in a new
  `.nav-row` flex container and moved the pin button out as a sibling.
- `button-name` (5 nodes): the accordion group-header buttons lost their
  accessible name when the label was `display:none`. Added
  `aria-label="收起{group}"/"展开{group}"` and `aria-expanded`.
- `color-contrast` on `/clusters`: the "体验演示集群" link used
  `--accent-primary` (#0f9588, 3.7:1 on white) — switched to the darker
  `--accent-primary-active` (≥5.2:1) to meet WCAG AA 4.5:1.
- Smoke timeouts: `getByRole('button', { name: '集群', exact: true })` failed to
  match because the nested pin button polluted the accessible name; fixed by the
  same restructure.

**Files:** `ConsoleLayout.vue`, `console-theme.css`, `ClustersView.vue`

## Testing

- `pnpm lint` — clean (0 errors)
- `pnpm typecheck` — clean
- `pnpm test -- --run` — 160/160 pass
- `pnpm build` — success
- `pnpm style:audit` / `pnpm bundle:gate` — pass
- `pnpm typegen` — sync check OK
- `scripts/check-change-record.sh --base HEAD^` — gate passes with this record
- `pnpm test:e2e` (Desktop + Mobile Chrome) — 78/78 pass, including all a11y
  scans and the previously failing smoke tests
