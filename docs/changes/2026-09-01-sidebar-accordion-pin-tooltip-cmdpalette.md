# Sidebar navigation enhancement — accordion, pin, tooltip, command palette

**Date:** 2026-09-01
**Scope:** frontend

## Summary

Enhanced the left sidebar navigation with four usability improvements to address
the growing number of navigation items (34 across 5 groups).

## Changes

### 1. Collapsed sidebar hover tooltip

When the sidebar is collapsed to icon-only mode (72px), hovering a nav item now
shows a positioned tooltip displaying the group label and item name. The tooltip
is teleported to `<body>` to avoid overflow clipping and styled with the existing
teal accent theme. Group name font-size matches nav item labels (13px).

**Files:** `ConsoleLayout.vue`, `console-theme.css`

### 2. Accordion nav groups

Each navigation group header is now a clickable button that toggles the group's
items open/closed. The expanded/collapsed state is persisted to
`localStorage` under `aiops.sidebar.accordion`. When the sidebar itself is
collapsed, accordion state is ignored and all items remain visible (icon-only).
A chevron indicator rotates to show state.

**Files:** `ConsoleLayout.vue`, `console-theme.css`

### 3. Pinned / favorite navigation

A pin button appears on hover next to each nav item (in expanded mode). Pinned
items are displayed in a dedicated "收藏" section at the top of the sidebar,
separated by a divider. Pin state is persisted to `localStorage` under
`aiops.sidebar.pins`. Pinned items are also visible in collapsed mode.

**Files:** `ConsoleLayout.vue`, `console-theme.css`

### 4. Ctrl+K command palette

A global `Ctrl+K` / `Cmd+K` keyboard shortcut opens a command palette overlay
with a search input. Typing filters all navigation items by label or group name.
Selecting an item navigates directly. The palette is also accessible via a search
button in the topbar. `Escape` closes it.

**Files:** `ConsoleLayout.vue`, `console-theme.css`

### 5. Topbar layout fix

Sidebar toggle and command palette trigger wrapped in `.topbar-left` container
so the action buttons (user info, avatar, account security, logout) remain
right-aligned in the topbar and do not shift to the left.

## localStorage keys

| Key | Purpose | Default |
|---|---|---|
| `aiops.sidebar.collapsed` | Sidebar expand/collapse | `0` (expanded) |
| `aiops.sidebar.accordion` | Per-group expanded state JSON | `{}` (all expanded) |
| `aiops.sidebar.pins` | Pinned route paths JSON | `[]` (none) |

## Testing

- `pnpm typecheck` — clean
- `pnpm test -- --run` — 160/160 pass
- Manual: hover tooltip in collapsed mode, accordion toggle, pin/unpin, Ctrl+K search, topbar button positions
