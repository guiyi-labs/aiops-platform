# ADR 0087: M97 Release Manifest And Offline RC Package

- Date: 2026-08-10
- Status: Accepted
- Milestone: M97
- Supersedes: the structural release/provenance packaging portions of ADR 0075

## Context

The previous release workflow produced independently named archives, a small
metadata file and a checksum list. It did not bind the image archive digests,
Helm chart, Kustomize root, offline package, SBOM, provenance and verification
commands into one consumer-facing contract. Its provenance statement was also
explicitly a placeholder, and the checksum file was rewritten after signing.

M97 needs a release candidate that can be inspected and verified before a
hosted GitHub Release is created. M89 production OIDC/MFA and M90 WAL/PITR/HA
remain external authorization tracks, so this decision cannot promote the
project to GA or production-ready status.

## Decisions

### 1. `aiops.release-manifest/v1` is the release index

`release-manifest.json` records the RC version/tag, full Git revision,
repository, channel, RC boundary, every payload asset's size and SHA-256,
OCI archive index digest and platform list, Helm/Kustomize/offline entry
points, SBOM/provenance assets and exact verification commands.

The manifest is itself covered by `SHA256SUMS`. Signature files and the
checksum file are trust metadata and are excluded from that checksum list to
avoid a circular hash. The signed object is the final, immutable
`SHA256SUMS`; no workflow step may rewrite it after signing.

### 2. Provenance is a real in-toto statement, without an overstated SLSA claim

The package contains an in-toto Statement v1 with SLSA provenance predicate
type. Its subjects are the exact pre-manifest payload files and their hashes;
the build definition records the repository, revision, RC version and builder
identity. Hosted CI supplies the workflow identity and invocation URL. Local
rehearsals identify the local packaging script and are not hosted provenance.

The workflow no longer labels a hand-built JSON object as a generator
placeholder and no longer uses a fail-open provenance attestation step. A
future SLSA builder integration may replace the builder metadata while
preserving the manifest subject contract.

### 3. RC assets include both deployment paths and an offline bundle

The release package contains multi-architecture OCI archives for the backend
and frontend, SPDX SBOMs generated from those archives, a packaged Helm chart,
a Kustomize archive, an offline archive with internal `OFFLINE-SHA256SUMS`, a
Secret template, the operations runbook, source and contract/license assets.

The offline archive is intentionally self-contained but does not contain
operator Secret values. Image archives must be mirrored into an approved
registry or imported by an environment-specific OCI tool before Kubernetes
installation.

### 4. Verification is fail-closed at publish time

The manifest verifier always checks schema, RC status, asset paths, sizes,
SHA-256 values, exact checksum coverage, provenance subjects and OCI metadata.
Strict mode additionally requires all M97 asset classes and both
`linux/amd64` and `linux/arm64`. Publish mode requires Cosign signature,
certificate and bundle verification. Missing Helm, Syft, Cosign, image assets,
or hosted GitHub Release permissions are reported as blocked/deferred and
cannot be described as a completed Gate C.

## Consequences

- Consumers have one stable entry point for release discovery and offline verification.
- Local key signatures prove the verification mechanics but do not replace hosted keyless identity evidence.
- Helm and Kustomize installation, upgrade, rollback, health, login and cleanup are exercised by the M97 rehearsal script.
- Database rollback remains an explicit backup/cutover decision because application startup only applies forward migrations.
- M97 may produce an RC, but GA remains prohibited until Gate D and the M89/M90 evidence are complete.
