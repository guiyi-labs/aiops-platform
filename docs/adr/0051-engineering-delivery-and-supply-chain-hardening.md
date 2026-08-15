# ADR 0051: Engineering, Delivery and Supply-Chain Hardening

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M38 (A: CI completeness, B: Helm chart, C: supply chain), ADR 0028 (versioned CI/release pipeline), ADR 0033 (offline production recovery readiness), ADR 0048 (restricted client-go migration), ADR 0049 (route descriptor contract), ADR 0050 (lightweight access grants)

## Context

Through M37 the platform shipped a vetted CI/release pipeline (ADR 0028) and a
strong static-deployment baseline (`deploy/kubernetes`). However, three gaps
remained relative to a defensible, reproducible delivery:

1. **CI completeness.** The pull-request gate ran the backend test suite but
   did not run the race detector, static analysis (golangci-lint, ESLint), a
   coverage baseline, or an OpenAPI breaking-change check. The real-kind E2E
   workflow covered M17-M21 and the diagnosis/fleet/search suites but not the
   M23-M31 disposable acceptance suites that had been added since.

2. **Helm chart.** Operators deployed the platform by applying
   `deploy/kubernetes` with `kubectl kustomize`. There was no parameterized,
   versioned chart, so per-environment customization required forked manifests
   or ad-hoc `kustomize edit` overlays. This is acceptable for a demo
   but not for a multi-environment product.

3. **Supply chain.** Releases produced `docker save` tarballs for
   `linux/amd64` only, with no SBOM, no license allowlist enforcement at gate
   time, and no signed attestation. The license inventory in
   `docs/supply-chain/dependency-licenses.md` was descriptive, not enforced.

`docs/kubesphere-optimization-plan.md` records these gaps under "Engineering
and delivery hardening" and the M38 milestone.

## Decision

M38 is split into three sequential phases — A (CI completeness), B (Helm
chart), C (supply chain) — each with its own contract tests. The decisions
below are enforced by the deployment test suite and the workflow contract
tests; they cannot regress without a new ADR.

### 1. CI gate is a single pull_request workflow with mandatory quality controls

`.github/workflows/ci.yml` runs on `pull_request` and `workflow_call` only.
`pull_request_target` and any reference to `secrets.*` are forbidden in CI
(contract test in `ci_workflows_test.go`). The workflow must include, in
order:

- `go test -p=1 -count=1 ./...` (unit tests)
- `go test -race -p=1 -count=1 ./...` (race detector)
- `golangci-lint@v2.12.2` with `.golangci.yml`
- `pnpm install --frozen-lockfile` and `pnpm lint` (ESLint flat config)
- coverage baseline: `go tool cover -func` must report ≥ 50.0%
- `git diff --exit-code` (format check)
- `oasdiff breaking --fail-on ERR` against the base branch OpenAPI
- `Change scope` detection to skip CI for documentation-only changes
- the four runtime drills (`credential-drill`, `audit-drill`,
  `identity-drill`, `recovery-drill`) gated on `runtime_required`

### 2. Real-kind E2E workflow must cover M23-M31

`.github/workflows/real-kind-e2e.yml` runs on `schedule` and
`workflow_dispatch` on the `self-hosted` `aiops-kind` runner. It must execute
the diagnosis, fleet, search, M21-history and M23-M31 disposable kind suites.
`cancel-in-progress: false` and `retention-days: 14` are mandatory. The
contract test enumerates every required script path.

### 3. Official Helm chart under `deploy/helm/aiops-platform/`

The chart is the supported path for parameterized deployment. It is a Helm 3
chart (`apiVersion: v2`) with:

- `Chart.yaml`, `values.yaml`, `values.schema.json` and `.helmignore`
- Templates: `_helpers.tpl`, `namespace.yaml`, `service-accounts.yaml`,
  `configmap.yaml`, `postgres.yaml`, `backend.yaml`, `frontend.yaml`,
  `ingress.yaml`, `network-policies.yaml`
- `values.schema.json` enforces required fields, replica bounds (1-20) and
  the `pullPolicy` enum (`Always`, `IfNotPresent`, `Never`)

The chart **never** renders a Secret. Operators must provide an existing
Secret named `aiops-secrets` (schema in `secret.example.yaml`). The contract
test `TestHelmChartDoesNotGenerateSecrets` scans every template for
`kind: Secret`, `CHANGE_ME` placeholders and inline secret values.

The chart must reproduce the security baseline from `deploy/kubernetes`:
non-root containers, read-only root filesystem, `drop: [ALL]` capabilities,
`automountServiceAccountToken: false`, `seccompProfile: RuntimeDefault`, and
the `restricted` pod security namespace labels. The contract test
`TestHelmChartBackendTemplateEnforcesSecurityBaseline` enforces the markers.

### 4. Multi-architecture release builds

`.github/workflows/release.yml` builds backend and frontend images for
`linux/amd64,linux/arm64` using `docker/setup-qemu-action` and
`docker/setup-buildx-action`. Images are exported as OCI tarballs
(`-o type=oci,dest=...`) and shipped in the release artifact bundle.
`docker push` is forbidden in the release workflow (contract test) — the
release is package-only, per ADR 0028.

### 5. SPDX SBOM for every release

The release workflow installs `syft v1.27.0` and generates
`sbom-backend-spdx.json` and `sbom-frontend-spdx.json` from the backend and
frontend source trees. Both SBOMs are included in the release artifact bundle
and covered by `SHA256SUMS`. The contract test requires the `syft`,
`spdx-json` and SBOM filename markers.

### 6. License allowlist is enforced at gate time

`docs/security/license-allowlist.json` declares the allowed license set
(`MIT`, `ISC`, `BSD-2-Clause`, `BSD-3-Clause`, `Apache-2.0`) and the
review-required set (`MPL-2.0`, `LGPL`, `GPL`, `UNKNOWN`, `SEE-LICENSE`).
Two contract tests enforce it:

- `TestLicenseAllowlistRejectsReciprocalLicenses` — the allowlist itself
  must not include reciprocal or unknown licenses
- `TestDependencyLicenseInventoryStaysWithinAllowlist` — every entry in
  `docs/supply-chain/dependency-licenses.md` must use a license from the allowlist

Adding a new license requires an ADR update; the contract test fails
otherwise.

### 7. SECURITY.md and CHANGELOG.md are tracked delivery assets

`SECURITY.md` documents the supported version policy, the private
vulnerability reporting channels (GitHub Security Advisory, PGP email), the
disclosure timeline, the threat-model boundaries and the supply-chain
controls. `CHANGELOG.md` follows Keep a Changelog 1.1.0 and SemVer 2.0.0 and
records every milestone with a link to its change record. Both files are
enforced as delivery assets in `delivery_assets_test.go`.

## Consequences

### Positive

- A pull request can no longer merge with a regression in race safety,
  static analysis, coverage, OpenAPI compatibility or license compliance.
- Operators can deploy the platform with `helm install` and override values
  per environment without forked manifests.
- Releases are reproducible, multi-architecture, and ship SPDX SBOMs, meeting
  the SLSA-build-level expectations for a defensible supply chain.
- The license allowlist turns the descriptive inventory into an enforceable
  gate; reciprocal licenses cannot enter production silently.

### Negative

- The CI gate is heavier: race detection, golangci-lint, ESLint, coverage,
  oasdiff and four runtime drills must all pass. The fast gate
  (`scripts/verify-fast.ps1`) remains the developer loop; CI is the
  authoritative gate.
- The Helm chart duplicates the manifest content in `deploy/kubernetes`.
  The two are intentionally redundant so that operators who cannot use Helm
  still have a kustomize path. The contract tests for both must remain
  green; divergence is a defect.
- Multi-architecture builds require QEMU and buildx in the release
  environment. Self-hosted runners without QEMU will fail the release;
  this is documented in `docs/ci-release.md`.

### Neutral

- The license allowlist is conservative. New dependencies that use
  MPL-2.0 or LGPL will block the gate until an ADR records the review
  outcome. This is intentional and aligned with the project's
  redistribution posture.

## Alternatives considered

- **Reusing kustomize overlays instead of a Helm chart.** Rejected: kustomize
  overlays do not provide `values.schema.json` validation, the
  `helm install --set` UX, or a versioned `Chart.yaml` that consumers can pin.
  The chart and the kustomize baseline are kept in lockstep by contract
  tests.
- **Pushing multi-arch images to a registry in the release workflow.**
  Rejected for now: the project does not own a registry, and ADR 0028
  explicitly states that "Creating that commit and release tag remains a
  human action." The release is package-only; operators load the OCI tarball
  into their registry of choice.
- **Cosign-based image signing.** Deferred: there is no signing identity in
  the project yet. SBOM + SHA256SUMS + `--verify-tag` is the current
  integrity boundary; cosign can be added in a future ADR without reshaping
  the release workflow.
- **Running the license check with a third-party tool (e.g.,
  `go-licenses-check`).** Rejected: the project already has
  `scripts/generate-license-report.ps1` producing a structured inventory.
  Enforcing the allowlist through a Go contract test keeps the gate
  dependency-free and aligned with the existing test strategy.
