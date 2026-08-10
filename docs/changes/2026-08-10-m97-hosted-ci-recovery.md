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
- `.github/workflows/ci.yml`: include `CHANGELOG.md` in the documentation-only
  scope, skip Backend together with the runtime jobs for that scope, and make
  the final result validate both the all-success runtime path and the
  all-skipped documentation path fail-closed.
- `.github/workflows/ci.yml`: merge the duplicate Backend test and global
  coverage executions into one `go test -cover` pass. Core-package coverage,
  race, fuzz, benchmark, build, scale, frontend, drill, manifest, and Compose
  gates remain unchanged.
- `backend/internal/deployment/ci_workflows_test.go`: pin the documentation
  scope, Backend condition, and final result semantics as workflow contracts.
- `.github/workflows/release.yml`: pass `force_runtime: true` and pin the
  Docker actions to their verified upstream commits. Generate four SPDX SBOMs
  from cached single-platform OCI inputs because the pinned Syft version does
  not consume a multi-architecture OCI index.
- `.github/workflows/ci.yml`: build the Backend image once in a dedicated
  runtime job, retain it as a one-day artifact, and load it in the four
  readiness drills and Compose runtime. Compose now builds only the frontend
  and starts with `--no-build`.
- `scripts/e2e-credential-reencryption.ps1`,
  `scripts/e2e-audit-archive.ps1`, `scripts/e2e-identity-readiness.ps1`, and
  `scripts/e2e-recovery-readiness.ps1`: accept `-BackendImage`, inspect the
  prebuilt image, and preserve standalone-build behavior when omitted.
- `backend/internal/deployment/ci_workflows_test.go` and
  `backend/internal/deployment/delivery_assets_test.go`: lock the shared-image,
  frontend-only Compose build, and prebuilt-drill contracts.

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
- `go test -count=1 ./internal/deployment`: passed after the CI efficiency
  contract update.
- `go test -cover -p=1 -count=1 -coverprofile=<temp> ./internal/deployment`:
  passed, confirming the merged test/coverage command shape.
- `git diff --check`: passed.
- PowerShell AST parsing for all four drill scripts: passed.
- Shared Backend image build: passed once in 58.4 seconds; saved artifact size
  was 43,619,328 bytes.
- Prebuilt-image drills: Identity 2.5 seconds, Recovery readiness 2.2 seconds
  after backup/restore, Audit archive 10.3 seconds, and Credential
  re-encryption 13.5 seconds; all passed and cleaned up their containers,
  networks, and loaded images.
- Isolated Compose validation: shared Backend image loaded, frontend-only
  build 2.9 seconds, startup with `--no-build` 12.5 seconds, all three
  services healthy, Backend readiness returned `status=ready`, and the
  frontend request succeeded.
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
- Main CI `31392971966` passed all 12 jobs at revision
  `62cb133e4aae1261e8823f15a075f70e4bf786cc`, validating the merged Backend
  test/coverage command and the unchanged full runtime quality path.
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
