# M97 Release Candidate Supply-Chain Closure

- Date: 2026-08-10
- Status: Draft pending local lifecycle rehearsal and hosted release permissions
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

## Verification

The final record will include the exact `.artifacts/m97-release/` evidence
paths after the local package and lifecycle rehearsal. Hosted GitHub Release
creation and keyless Cosign verification remain dependent on remote access and
organization workflow permissions; a failed remote submission is not treated
as local evidence.

## Risks / Notes

- The package must remain `vX.Y.Z-rc.N`; no GA or production-ready wording is allowed.
- A Helm rollback or Kustomize reapply does not reverse database migrations. A validated backup and explicit cutover plan remain required.
- Missing Helm, Syft or Cosign causes strict rehearsal failure rather than a false pass.
