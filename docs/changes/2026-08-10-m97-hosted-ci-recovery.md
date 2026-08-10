# M97 Hosted CI Recovery

- Date: 2026-08-10
- Status: Complete; full hosted quality gate and signed RC prerelease verified
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
- `.github/workflows/ci.yml`: add the `force_runtime` reusable-workflow input
  so Release tags cannot downgrade to documentation-only quality gates.
- `.github/workflows/release.yml`: pass `force_runtime: true` and pin the
  Docker actions to their verified upstream commits. Generate four SPDX SBOMs
  from cached single-platform OCI inputs because the pinned Syft version does
  not consume a multi-architecture OCI index.

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
- Fixed-code hosted verification: main CI run `31378870337` completed
  successfully at commit `ec3da0c6d878a0a4b8fcef49826d02ff67b1a3d9`; Backend,
  Backend race, Frontend, M96 Gate B, Compose runtime, and final CI result all
  passed.
- RC rerun `31380408636` passed its quality gate but stopped before package
  execution because the pinned QEMU and Buildx action SHAs did not exist.
- Main CI `31381359114` passed all 12 jobs after the workflow correction.
- RC rerun `31382016265` proved the forced full quality gate and
  multi-architecture OCI build, then failed at image SBOM generation with
  Syft's `application/vnd.oci.image.index.v1+json` limitation.
- Main CI `31384162209` passed all 12 jobs at revision
  `0f69256c1ddb0f874a12c79e7fcbda4f20a8fa9a` after the per-platform SBOM
  workflow fix.
- Release run `31384939856` for immutable tag `v0.3.0-rc.4` passed the full
  required quality gate and the `Build and verify RC package` job. GitHub
  published the non-draft prerelease at
  `https://github.com/guiyi-labs/aiops-platform/releases/tag/v0.3.0-rc.4`.
- The prerelease contains 20 assets: 16 payloads covered by `SHA256SUMS`, the
  checksum root, and its Cosign bundle, certificate, and detached signature.
  All 16 release-asset digests match the checksum entries.
- `release-manifest.json` binds the release to the exact RC tag and revision,
  two multi-architecture OCI archives, and four per-platform SPDX SBOMs. The
  published certificate SAN is the exact workflow identity
  `https://github.com/guiyi-labs/aiops-platform/.github/workflows/release.yml@refs/tags/v0.3.0-rc.4`.

## Risks / Notes

- `v0.3.0-rc.1`, `v0.3.0-rc.2`, and `v0.3.0-rc.3` remain immutable failure
  evidence. `v0.3.0-rc.4` is the completed hosted RC evidence.
- Performance duration and memory thresholds remain report-only. Correctness
  invariants remain hard failures.
- GitHub reports Node.js 20 deprecation annotations for several pinned
  third-party actions while forcing them onto Node.js 24. These warnings did
  not weaken or skip any required result but remain a maintenance follow-up.
- M89 OIDC/MFA and M90 WAL/PITR/HA remain external blockers. This recovery
  does not support GA, production identity, production HA, or
  production-ready claims.
