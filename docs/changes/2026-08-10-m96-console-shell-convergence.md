# M96 console shell convergence

- Date: 2026-08-10
- Status: Complete
- Scope: Keep one authenticated console shell across route changes and remove one unreferenced theme layer.

## Context

Every authenticated view previously rendered its own shell. M96 requires one
sidebar/topbar per session, route changes that replace only page content, and
an auditable style layer order without visual regression.

## What Changed

### Stable authenticated shell
- `frontend/src/App.vue`: mount one shell around all non-public routes and
  render public routes without the shell.
- `frontend/src/components/ConsoleLayout.vue`: add shell/page-bridge modes;
  page metadata is injected into the shell and named actions Teleport to the
  stable topbar target.
- `frontend/e2e/smoke.spec.ts`: assert exactly one shell/sidebar/topbar, no
  shell on login, title replacement, route navigation and collapse persistence
  in both desktop and mobile projects.

### CSS responsibility and evidence
- `frontend/src/styles/kubesphere-theme.css`: remove the unreferenced legacy
  theme layer; active import order remains explicit in `frontend/src/main.ts`.
- `frontend/scripts/style-audit.mjs`: emit the four-layer byte, line,
  selector, hash and import-order baseline in report mode.
- `frontend/package.json` and `.github/workflows/ci.yml`: run and upload the
  CSS audit artifact.

## Verification

- `pnpm test -- --run`: 137 tests passed.
- `pnpm lint`: passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed.
- `pnpm test:e2e`: 56/56 desktop/mobile tests passed; console errors remained zero.
- `pnpm style:audit`: wrote `frontend/.artifacts/style-audit/m96-style-baseline-v1.json` and Markdown.
- CSS baseline: 194,328 bytes, 6,377 lines, 1,744 selector occurrences,
  1,500 unique selectors across the four imported layers.
- `git diff --check`: passed.

## Risks / Notes

- CSS size and selector thresholds remain report mode until stable hosted CI
  cycles establish variance.
- The shell uses the existing route definitions and auth guard; no API or
  authorization contract changes are included.
- The project remains RC; M89/M90 external identity and data-resilience
  tracks are not represented as complete by this change.
