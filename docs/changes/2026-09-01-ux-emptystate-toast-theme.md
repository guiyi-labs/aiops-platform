# UX Enhancement — EmptyState, Toast, Cluster Onboarding, Theme Switching

**Date:** 2026-09-01
**Scope:** frontend

## Summary

Four UX improvements: unified empty-state component, toast notification system,
cluster onboarding experience, and dark/light theme switching.

## Changes

### 1. Unified EmptyState component

Enhanced `EmptyState.vue` with optional `icon`, `iconSize`, and `hero` props.
The hero variant renders a larger icon (80px), bigger title (18px), and more
padding — suitable for first-time-empty pages that need a prominent call-to-action.

**Files:** `EmptyState.vue`

### 2. Toast notification system

New `Toast.vue` component providing global success/error/warning/info toasts.
Teleported to `<body`, supports `top-right`, `top-center`, `bottom-right` positions,
auto-dismiss with configurable duration, and TransitionGroup animations.
Exposed via `defineExpose` for parent ref access.

**Files:** `Toast.vue` (new)

### 3. Cluster onboarding UX

- Clusters page empty state now uses the hero EmptyState variant with a prominent
  "接入第一个集群" CTA button, replacing the previous text-only hint.
- Probe success now shows a notice message.
- Remove success shows a confirmation notice.

**Files:** `ClustersView.vue`

### 4. Dark/light theme switching

- New `useTheme` composable with localStorage persistence (`aiops.theme`).
- Toggle button in the topbar (Sun/Moon icon) switches between dark and light.
- Light theme CSS overrides under `.theme-light` class: white topbar, light card
  backgrounds, adapted text colors, form inputs. Sidebar stays dark as brand identity.
  Login page stays dark.

**Files:** `useTheme.ts` (new), `ConsoleLayout.vue`, `console-theme.css`

## Testing

- `pnpm typecheck` — clean
- `pnpm test -- --run` — 160/160 pass
- `docker compose build frontend` — image built
- `docker compose up -d frontend` — container recreated and healthy
