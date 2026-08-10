# M97 Release Candidate Supply-Chain Closure

- Date: 2026-08-10
- Status: Complete for RC scope; local lifecycle and hosted signed prerelease verified
- Scope: Unified RC manifest, offline package, deployment lifecycle rehearsal and fail-closed publishing

## Context

The M96 Gate B implementation left the repository with older M20/M38 release
packaging. It could make checksummed source and Helm assets, but it did not
provide a unified digest manifest, a Kustomize/offline package, or a repeatable
install/upgrade/rollback entry point. The existing release workflow also
rewrote `SHA256SUMS` after signing and described provenance as structural.

M97 is intentionally an RC stage. M89 production OIDC/MFA and M90
WAL/PITR/HA still require external authorization and real environment
evidence.

## What Changed

- Added `scripts/release-manifest.mjs` and focused Node tests. The tool creates
  and verifies `aiops.release-manifest/v1`, exact SHA-256 coverage, OCI archive
  metadata, provenance subjects, RC status and strict signature requirements.
- Reworked `scripts/release-verify.ps1` to require a clean revision, build or
  consume multi-architecture OCI archives, generate image SBOMs, package Helm
  and Kustomize assets, create an internally checksummed offline archive, and
  produce structured rehearsal evidence.
- Added `scripts/m97-release-rehearsal.ps1` for fresh kind lifecycle checks:
  Kustomize and Helm install, upgrade, rollback, readiness, authenticated
  login and cleanup. It records no credentials or response bodies.
- Reworked `.github/workflows/release.yml` to accept RC tags only, generate
  the manifest and provenance before signing, verify the final checksum root,
  upload only the signed bundle, and publish a GitHub prerelease.
- Added ADR 0087 and the operator runbook at
  `docs/release-candidate-operations.md`.
- Updated the rehearsal to reuse the complete Helm install values during
  upgrade, allocate health-check ports dynamically, and persist redacted
  failure evidence before cleanup.
- Updated `frontend/Dockerfile` so the architecture-neutral static build runs
  on `$BUILDPLATFORM`; the final Nginx image is still emitted for each target
  platform.
- Strict image SBOM generation now scans per-platform OCI views because the
  pinned Syft version does not consume a multi-architecture OCI index; those
  temporary views are excluded from the published asset set.
- OCI inspection now follows nested Buildx indexes, validates each referenced
  blob digest, ignores attestation manifests for platform coverage, and has a
  focused nested-index fixture test.
- Tool version evidence now captures complete native output before selecting a
  summary line, avoiding false `error:-1` values caused by a closed pipeline.

## Verification

The local Kustomize and Helm install/upgrade/rollback/health/login/cleanup
rehearsal passed in:

- `.artifacts/m97-release/lifecycle-aiops-m97-final.json`
- `.artifacts/m97-release/frontend-multiarch-smoke-oci.tar` (SHA-256
  `11cd90a326cd45a2871c9192d78e745dcd30547681a3361ce47e993855dc4094`)
- `.artifacts/m97-release/m97-release-20260810-061402.json` (non-strict
  local-key rehearsal package)
- `.artifacts/m97-release/m97-release-latest.json` (strict
  multi-architecture package, four platform SBOMs and verified local-key
  signature; timestamped copies remain beside it)

The initial Helm rehearsal timed out during upgrade because the command did not
retain the initial repository and namespace values; the fixed rehearsal passed
after adding `--reuse-values`. The first strict package attempt also exposed
Syft's multi-architecture OCI index limitation; the per-platform SBOM input fix
above addresses that local tooling boundary. The latest evidence binds the
manifest revision to the clean HEAD used for the final package; the baseline
and RC tags are created only after this verification. Hosted
GitHub Release creation and keyless Cosign verification remain dependent on
remote access and organization workflow permissions; a failed remote
submission is not treated as local evidence.

Remote synchronization succeeded for `main`,
`baseline-m97-release-candidate-tooling-20260810`, and `v0.3.0-rc.1`.
Hosted run `31376784927` completed with failure at revision
`94e403bb4e9a68b4c5fa5c387104a09eb4e45314`:

- Backend `golangci-lint` reported `QF1001` in
  `internal/deployment/ci_workflows_test.go` and an unused
  `sortedArtifactNames` function in `internal/scalefixture/verify.go`.
- The M96 frontend Pod scale report sampled six visits with zero runtime
  failures but six invariant failures.
- The required CI result failed, so the hosted RC package job was skipped and
  no GitHub Release was created.

The initial failures remain evidence for immutable `v0.3.0-rc.1`. Their
follow-up repair and hosted reruns are tracked in
[`2026-08-10-m97-hosted-ci-recovery.md`](2026-08-10-m97-hosted-ci-recovery.md).
Main CI `31384162209` passed all 12 jobs at revision
`0f69256c1ddb0f874a12c79e7fcbda4f20a8fa9a`. Release run `31384939856` for
immutable tag `v0.3.0-rc.4` then passed the full required quality gate,
multi-architecture packaging, per-platform SBOM generation, final manifest
verification, keyless Cosign signing, artifact upload, and GitHub prerelease
publication.

The published prerelease contains 20 assets. `SHA256SUMS` covers all 16
payload assets with digests matching GitHub's release metadata; the remaining
assets are the checksum root's Cosign bundle, certificate, and detached
signature. `release-manifest.json` binds the RC to the exact revision, both
OCI platforms, four SPDX SBOMs, M89/M90 release blockers, and the exact tag
workflow certificate identity. This completes Gate C for the RC scope without
changing the GA boundary.

## Risks / Notes

- The package must remain `vX.Y.Z-rc.N`; no GA or production-ready wording is allowed.
- A Helm rollback or Kustomize reapply does not reverse database migrations. A validated backup and explicit cutover plan remain required.
- Missing Helm, Syft or Cosign causes strict rehearsal failure rather than a false pass.
