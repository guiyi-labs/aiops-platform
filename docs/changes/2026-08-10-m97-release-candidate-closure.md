# M97 Release Candidate Supply-Chain Closure

- Date: 2026-08-10
- Status: Local lifecycle and multi-architecture build verified; strict package and hosted release evidence pending final clean revision
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

## Verification

The local Kustomize and Helm install/upgrade/rollback/health/login/cleanup
rehearsal passed in:

- `.artifacts/m97-release/lifecycle-aiops-m97-helmfix.json`
- `.artifacts/m97-release/frontend-multiarch-smoke-oci.tar` (SHA-256
  `11cd90a326cd45a2871c9192d78e745dcd30547681a3361ce47e993855dc4094`)
- `.artifacts/m97-release/m97-release-20260810-061402.json` (non-strict
  local-key rehearsal package)

The initial Helm rehearsal timed out during upgrade because the command did not
retain the initial repository and namespace values; the fixed rehearsal passed
after adding `--reuse-values`. The first strict package attempt also exposed
Syft's multi-architecture OCI index limitation; the per-platform SBOM input fix
above addresses that local tooling boundary. The strict full package must be
generated from the final clean revision. Hosted GitHub Release creation and keyless Cosign
verification remain dependent on remote access and organization workflow
permissions; a failed remote submission is not treated as local evidence.

## Risks / Notes

- The package must remain `vX.Y.Z-rc.N`; no GA or production-ready wording is allowed.
- A Helm rollback or Kustomize reapply does not reverse database migrations. A validated backup and explicit cutover plan remain required.
- Missing Helm, Syft or Cosign causes strict rehearsal failure rather than a false pass.
