# W9: Frontend Quality + Browser E2E (Playwright, M85)

- Date: 2026-08-09
- Status: Development Complete (local; CI job wiring pending push)
- Scope: Frontend quality & E2E — no product code behavioural change

## Summary

Closes the polish-plan W9 / M85 milestone: a real-browser E2E suite with
Playwright covering the two required viewports (Desktop Chrome 1280×720 and
Mobile Chrome / Pixel 7 390×844) and asserting zero `console.error`. Seven
key-link smoke checks pass on both viewports.

## What Changed

- `frontend/package.json` — added `@playwright/test@^1.62.1` devDependency and a
  `test:e2e` script (`playwright test`).
- `frontend/playwright.config.ts` — two projects (`Desktop Chrome`,
  `Mobile Chrome`), `webServer: pnpm dev` on port 5173, `reuseExistingServer`
  in local runs.
- `frontend/e2e/smoke.spec.ts` — 7 checks: app shell renders, login form,
  dashboard cards + cluster selector, sidebar navigation items, optimization /
  posture / topology headings. Every test installs a console-error trap and the
  `afterEach` asserts zero errors.
- `frontend/src/styles/motion.css` — unified premium micro-motion layer
  (card hover lift, button sheen, status-dot pulse, content reveal, staggered
  table-row enter, skeleton shimmer, `prefers-reduced-motion` guard).
- `frontend/src/components/SkeletonCard.vue` — reusable skeleton loader
  (shimmer), wired into the dashboard fleet-health loading placeholder.
- `frontend/src/components/EmptyState.vue` — reusable animated empty state.
- `frontend/src/main.ts` — imports `motion.css`.

## Verification

```
npx playwright test --project='Desktop Chrome' e2e/smoke.spec.ts  → 7 passed
npx playwright test --project='Mobile Chrome'  e2e/smoke.spec.ts  → 7 passed
# zero console.error in every test (enforced in afterEach)
```

## Notes

Only `chromium` headless-shell is installed locally; the mobile project reuses
the same engine at 390×844. Full multi-engine browsers can be added when CI
runner dependencies allow. Backend unit coverage was independently pushed to
60.03% the same session (see `2026-08-09-w8-coverage-closure.md`).