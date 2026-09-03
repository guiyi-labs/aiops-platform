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

## Testing

- `pnpm lint` — clean (0 errors)
- `pnpm typecheck` — clean
- `pnpm test -- --run` — 160/160 pass
- `pnpm build` — success
- `pnpm style:audit` / `pnpm bundle:gate` — pass
- `pnpm typegen` — sync check OK
- `scripts/check-change-record.sh --base HEAD^` — gate passes with this record
