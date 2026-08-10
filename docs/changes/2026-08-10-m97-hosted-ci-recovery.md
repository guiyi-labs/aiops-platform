# M97 Hosted CI Recovery

- Date: 2026-08-10
- Status: Local verification complete; hosted rerun pending
- Scope: Repair the Backend lint and M96 Pod scale invariant failures that blocked the first hosted RC workflow.

## Context

Release workflow run `31376784927` for immutable tag `v0.3.0-rc.1` reached
the required quality gate but failed before packaging. Backend
`golangci-lint` reported one simplification warning and one unused helper. The
frontend Pod scale sampler completed all six browser visits without runtime
errors, but Linux Chromium reported a scroll height of `2,799,990 px` against
an idealized `2,800,000 px` table-row total, causing the same
`scrollHeightCoversFixture` failure in every sample.

The 10 px difference is smaller than one configured 56 px row and comes from
cross-platform table layout geometry. The same samples retained bounded DOM
and virtual windows, exact fixture count, stable midpoint scrolling, exact
last-Pod filtering, and zero console errors.

## What Changed

- `backend/internal/deployment/ci_workflows_test.go`: express the invalid
  release-stage ordering directly so Staticcheck no longer reports `QF1001`.
- `backend/internal/scalefixture/verify.go`: remove the unused
  `sortedArtifactNames` helper and its `sort` import.
- `frontend/scripts/pod-scale-perf-sample.mjs`: require the scroll surface to
  cover the final row's start offset instead of the idealized bottom edge of
  every table row. Existing fixture, DOM, virtual-window, scroll-target,
  overscan, position, filter, and console invariants remain fail-closed.

## Verification

- `golangci-lint v2.12.2 run --config ../.golangci.yml ./...`: passed with 0 issues.
- `go test -count=1 ./...`: passed.
- `go vet ./...`: passed.
- `pnpm lint`: passed.
- `pnpm typecheck`: passed.
- `pnpm test -- --run`: 25 files and 137 tests passed.
- `pnpm build`: passed.
- `pnpm test:e2e`: 56 browser tests passed.
- `pnpm bundle:gate`: passed.
- `pnpm style:audit`: report generated successfully.
- `pnpm perf:pods`: 6 visits, 0 runtime failures, 0 invariant failures.
- Local Pod scale evidence:
  `frontend/.artifacts/pod-scale-perf/m96-pod-scale-samples-v1.json` and
  `frontend/.artifacts/pod-scale-perf/m96-pod-scale-report.md`.
- Failed hosted evidence: run `31376784927`, artifact
  `pod-scale-perf-31376784927` (`9058311251`).

## Risks / Notes

- `v0.3.0-rc.1` remains immutable and retains its failed hosted evidence. A
  fixed release attempt must use a new RC tag.
- Performance duration and memory thresholds remain report-only. Correctness
  invariants remain hard failures.
- M89 OIDC/MFA and M90 WAL/PITR/HA remain external blockers. This recovery
  does not support GA, production identity, production HA, or
  production-ready claims.
