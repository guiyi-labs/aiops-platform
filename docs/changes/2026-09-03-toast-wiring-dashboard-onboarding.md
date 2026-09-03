# Toast notifications wiring + Dashboard cluster onboarding

**Date:** 2026-09-03
**Scope:** frontend

## Summary

Two follow-ups to the earlier UX-enhancement milestone: the AppToast component
(created but never wired) is now backed by a shared composable and mounted in
the console shell, and the Dashboard gains an empty-cluster onboarding call to
action.

## Changes

### 1. useToast composable + AppToast wiring

`AppToast.vue` previously owned its own toast state and was never mounted.
Now:

- New `src/composables/useToast.ts` holds module-level toast state so any
  component can call `toast.success(...)` without a ref to the component
  instance.
- `AppToast.vue` consumes the composable and renders the shared toast list;
  still teleported to `<body>`.
- `ConsoleLayout.vue` mounts `<AppToast />` once in the shell, so toasts are
  globally available across all console pages.

**Files:** `useToast.ts` (new), `AppToast.vue`, `ConsoleLayout.vue`

### 2. Cluster operations now emit toasts

`ClustersView.vue` replaces inline notice text with toast feedback for create /
probe / rotate / toggle / remove operations, and errors surface as error toasts.

**Files:** `ClustersView.vue`

### 3. Dashboard empty-cluster onboarding

When the console loads with zero enabled clusters, the Dashboard now renders a
hero EmptyState with a "去接入集群" CTA instead of an empty cockpit. The full
cockpit (context bar, fleet comparison, metrics, workspaces) renders inside a
`v-else` branch. The e2e API fixture now returns one enabled cluster by default
so existing smoke tests keep rendering the full cockpit; a new smoke test
overrides `/api/v1/clusters` with an empty list to cover the onboarding state.

**Files:** `DashboardView.vue`, `e2e/api-fixtures.ts`, `e2e/smoke.spec.ts`

### 4. Contrast fixes surfaced by the default-cluster fixture

Making the e2e fixture return one enabled cluster by default caused the a11y
scans to render summary cards that were previously hidden by empty state —
exposing pre-existing sub-AA text colors on light card surfaces. Darkened the
hard-coded grays to ≥ 4.5:1 (verified against their real backgrounds):

- `.topology-summary-grid span/small` → `#5f6d69` / `#5d6a66`
- `.topology-empty` → `#596561`; `.topology-stack-group > header` → `#55625d`
- `.event-summary-grid small` → `#566267`
- `.detail-empty` → `#526066`
- `.metric-mode button.active` background → `#0e7a70` (white text 5.2:1)

On narrow viewports (Mobile Chrome), the scrollable `topology-canvas` and
`event-center-panel` regions triggered `scrollable-region-focusable` when empty;
both now carry `tabindex="0"` so the scroll region itself is keyboard-focusable.

**Files:** `base.css`, `console-theme.css`, `TopologyView.vue`, `EventsView.vue`

## Testing

- `pnpm lint` — clean
- `pnpm typecheck` — clean
- `pnpm test -- --run` — 160/160 pass
- `pnpm build` — success
- `pnpm test:e2e` — smoke suite (incl. new onboarding test) + a11y scans green
