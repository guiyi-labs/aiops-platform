# ADR 0085 - M96 authenticated console shell convergence

- Date: 2026-08-10
- Status: Accepted
- Milestone: M96
- Related: ADR 0084, M93-C console theme, M85 browser regression

## Context

Each authenticated view previously mounted its own `ConsoleLayout`, which
made the shell lifecycle a property of every page component. That allowed
future navigation changes to create duplicate sidebar/topbar instances or an
empty shell during route replacement. The existing page title and named
actions also needed to keep working without changing all 34 views at once.

## Decision

1. `App.vue` owns one `ConsoleLayout` shell for non-public routes and keeps a
   direct `RouterView` for public routes. Existing route definitions and
   authorization metadata remain unchanged.
2. `ConsoleLayout` has a shell mode and a page-bridge mode. Page bridges
   forward eyebrow/title through a stable injected context and Teleport named
   actions to the shell topbar; their default slot is the only changing page
   content.
3. The shell exposes stable `data-testid="console-shell"` and the topbar
   action target. Browser regression asserts one shell/sidebar/topbar, no
   shell on login, route title replacement and persisted sidebar collapse.
4. The imported CSS order is fixed as `base.css`, `console-theme.css`,
   `motion.css`, `premium-ui.css`. The unreferenced `kubesphere-theme.css`
   layer is removed. A report-mode audit records bytes, lines, selector
   occurrences, unique selectors and hashes for the active four layers.

## Consequences

- Authenticated navigation keeps the same shell DOM while replacing only the
  routed page content and topbar metadata.
- Named actions remain local to each view's source while rendering in the
  stable topbar, avoiding a broad view-by-view template rewrite.
- CSS changes remain measurable, but the first M96 style report does not yet
  fail CI on size or selector drift; stable hosted cycles are required before
  a fail-closed budget is considered.
