# M96 frontend scale budget

- Date: 2026-08-10
- Status: Complete
- Scope: Add deterministic 50k Pod browser evidence and repair virtual-list scroll stability.

## Context

M96-A/B fixed the backend fixture and report-mode baseline, but the frontend
still lacked evidence for DOM bounds, filtering and scrolling at 50,000 Pods.
The existing virtual list rendered an initial window without a bottom spacer,
and the shared row-entry animation caused newly mounted rows to move while a
user scrolled.

## What Changed

### Virtualized Pod workbench
- `frontend/src/views/WorkloadsView.vue`: render top and bottom spacer rows,
  expose non-sensitive window metrics, and reuse the unfiltered response array.
- `frontend/src/composables/useVirtualList.ts`: synchronize the mounted
  viewport and observe size changes with `ResizeObserver`.
- `frontend/src/styles/motion.css`: disable geometry-changing row entry
  animation only inside virtual lists.
- `frontend/src/utils/resource-filter.ts`: centralize reference-preserving
  name filtering; its test covers a 50,000-item array.

### Browser scale evidence
- `frontend/testdata/scale/m96-pods-v1.json`: versioned 50k Pod fixture
  configuration.
- `frontend/scripts/pod-scale-fixture.mjs`: deterministic payload and hash
  generator used by the sampler.
- `frontend/scripts/pod-scale-perf-sample.mjs`: production-preview Playwright
  sampler for desktop/mobile first render, filtering, middle scroll, long
  tasks, DOM, heap, virtual window and console errors.
- `frontend/scripts/pod-scale-perf-report.mjs`: report-mode baseline JSON and
  Markdown report with measured thresholds and hard invariants.
- `.github/workflows/ci.yml`: run and retain the M96 Pod scale report beside
  the existing login performance artifact.

## Verification

- `pnpm test -- --run src/composables/useVirtualList.test.ts src/utils/resource-filter.test.ts`: 8 tests passed.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm build`: passed.
- `pnpm perf:pods`: 6 visits, 0 failures, 0 invariant failures.
- Current report: `frontend/.artifacts/pod-scale-perf/m96-pod-scale-report.md`.
- Current fixture payload: 50,000 Pods, 18,890,589 bytes, SHA-256 `01274b8d8223887fdc80d211f61a9579605842ca8d096b7fcd8e282f4dbf02cd`.
- Current observations: first render desktop/mobile P50 `962/882.5 ms`, filter P50 `235.4/181.6 ms`, scroll P50 `36.1/30.3 ms`, initial DOM `894`, rendered window `16` rows.

## Risks / Notes

- This fixture measures browser parsing and bounded UI behavior, not
  kube-apiserver, network or production-device capacity.
- Thresholds remain report mode. Exact counts, row bounds, scroll behavior,
  filter convergence and console errors fail the sampler immediately.
- M96 shell routing and CSS responsibility convergence remain open and are
  intentionally separate from this frontend scale evidence increment.
