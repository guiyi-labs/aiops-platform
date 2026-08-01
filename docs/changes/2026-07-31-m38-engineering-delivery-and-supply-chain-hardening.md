# M38: Engineering, Delivery and Supply-Chain Hardening

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0051](../adr/0051-engineering-delivery-and-supply-chain-hardening.md)
- Baseline: layered on top of `baseline-m35-20260731`; no new baseline tag cut
- Fast gate: see verification section

## Summary

M38 closes the engineering, delivery and supply-chain gaps recorded in
`docs/kubesphere-optimization-plan.md` under "Engineering and delivery
hardening". It is split into three sequential phases:

- **M38A — CI completeness.** The pull-request gate now runs the race
  detector, golangci-lint, ESLint, a 50% coverage baseline and an OpenAPI
  breaking-change check. The real-kind E2E workflow covers M23-M31.
- **M38B — Helm chart.** An official Helm 3 chart under
  `deploy/helm/aiops-platform/` provides a parameterized, schema-validated
  deployment path alongside the existing kustomize baseline. The chart never
  renders a Secret.
- **M38C — Supply chain.** Releases produce multi-architecture OCI images
  (`linux/amd64`, `linux/arm64`) and SPDX SBOMs. A license allowlist turns
  the descriptive dependency inventory into an enforced gate. `SECURITY.md`
  and `CHANGELOG.md` are now tracked delivery assets.

No public API contract was changed. No Kubernetes manifest contract was
broken. The kustomize baseline in `deploy/kubernetes` is unchanged and
remains the supported non-Helm path.

## Changes

### M38A — CI completeness

#### Modified files

- `.github/workflows/ci.yml`: added a `race` job running
  `go test -race -p=1 -count=1 ./...`; integrated `golangci-lint@v2.12.2`
  and `pnpm lint`; added the `Coverage baseline` step (≥ 50.0%) and the
  `OpenAPI breaking-change check` step (`oasdiff breaking --fail-on ERR`).
  The `Change scope` job still skips the runtime drills for documentation-only
  changes; `runtime_required` remains the gate.
- `.github/workflows/real-kind-e2e.yml`: extended the suite map to include
  `m23` through `m31` (release lifecycle, cross-cluster promotion, workload
  protection, alert lifecycle, backup creation, namespace posture, node
  maintenance, isolated restore rehearsal). Artifact paths and retention
  unchanged.
- `backend/internal/deployment/ci_workflows_test.go`: contract test now
  requires `golangci-lint@v2.12.2`, `pnpm lint`, `Coverage baseline`,
  `oasdiff`, `git diff --exit-code` and the M23-M31 script paths.

#### New files

- `.golangci.yml`: golangci-lint v2 configuration. Enables `govet`,
  `ineffassign`, `misspell`, `staticcheck`, `unused`. Disables noisy
  checks (`SA1019`, `SA5011`, `S1016`, `ST1000`, `ST1003`, `ST1005`,
  `QF1003`, `QF1006`, `QF1008`). `misspell.locale: US`.
- `frontend/eslint.config.js`: ESLint flat config covering TypeScript and
  Vue. Ignores `dist/**` and `node_modules/**`. Applies
  `js.configs.recommended`, `tseslint.configs.recommended` and
  `pluginVue.configs['flat/essential']`.

### M38B — Helm chart

#### New files

- `deploy/helm/aiops-platform/Chart.yaml`: Helm 3 chart metadata
  (`apiVersion: v2`, `type: application`, `version: 0.1.0`,
  `appVersion: "0.1.0"`).
- `deploy/helm/aiops-platform/values.yaml`: default values for namespace
  (with `restricted` pod security labels), backend (image, replicas,
  resources, full config map), frontend (image, replicas, resources),
  postgres (image, storage, resources), ingress (className, host, TLS),
  networkPolicies toggle and `existingSecret: aiops-secrets`.
- `deploy/helm/aiops-platform/values.schema.json`: JSON schema enforcing
  required fields, replica bounds (1-20) and the
  `pullPolicy ∈ {Always, IfNotPresent, Never}` enum.
- `deploy/helm/aiops-platform/.helmignore`: excludes `.git/`, `*.log`,
  `*.bak`, `*.tmp` and other transient files from packaged charts.
- `deploy/helm/aiops-platform/templates/_helpers.tpl`: `namespace`,
  `labels` and `selectorLabels` helpers.
- `deploy/helm/aiops-platform/templates/namespace.yaml`: conditional
  Namespace with the `restricted` pod security labels.
- `deploy/helm/aiops-platform/templates/service-accounts.yaml`: backend,
  frontend and postgres ServiceAccounts, all with
  `automountServiceAccountToken: false`.
- `deploy/helm/aiops-platform/templates/configmap.yaml`: backend ConfigMap
  rendered from `.Values.backend.config`.
- `deploy/helm/aiops-platform/templates/postgres.yaml`: headless Service and
  StatefulSet with `volumeClaimTemplates` sourcing credentials from
  `secretKeyRef` against `existingSecret`.
- `deploy/helm/aiops-platform/templates/backend.yaml`: ClusterIP Service
  (with Prometheus scrape annotations) and Deployment with startup, readiness
  and liveness probes, non-root security context, `drop: [ALL]`
  capabilities, read-only root filesystem and a `tmp` emptyDir volume.
- `deploy/helm/aiops-platform/templates/frontend.yaml`: ClusterIP Service
  and Deployment running as UID/GID 101 with the nginx cache/run/tmp
  emptyDirs required by the read-only root filesystem.
- `deploy/helm/aiops-platform/templates/ingress.yaml`: conditional Ingress
  with configurable className, host, annotations and TLS.
- `deploy/helm/aiops-platform/templates/network-policies.yaml`: default-deny
  plus postgres, backend and frontend policies mirroring
  `deploy/kubernetes/network-policies.yaml`.

#### New tests

- `backend/internal/deployment/helm_chart_test.go`: ten contract tests
  covering Chart.yaml metadata, values structure, schema alignment,
  required templates, rendered resource markers, the backend security
  baseline, the "no generated Secret" rule, the "no secrets in values"
  rule, helper consistency and `.helmignore` contents.

#### Modified files

- `backend/internal/deployment/delivery_assets_test.go`: added required
  markers for `Chart.yaml`, `values.yaml`, `values.schema.json`,
  `templates/backend.yaml`, `templates/network-policies.yaml` and
  `templates/_helpers.tpl`.

### M38C — Supply chain

#### Modified files

- `.github/workflows/release.yml`: added `docker/setup-qemu-action` and
  `docker/setup-buildx-action` steps; builds backend and frontend images
  for `linux/amd64,linux/arm64` with `docker buildx build ... -o type=oci`.
  Installs `syft v1.27.0` and generates `sbom-backend-spdx.json` and
  `sbom-frontend-spdx.json`. Bundles the license allowlist, the Helm chart
  tarball and the SBOMs in the release artifact directory. `release-metadata.json`
  now records `architectures`, `sbom` and `helm`. `docker push` remains
  forbidden.
- `backend/internal/deployment/ci_workflows_test.go`: contract test now
  requires `docker/setup-qemu-action`, `docker/setup-buildx-action`,
  `--platform linux/amd64,linux/arm64`, `syft`, `spdx-json`,
  `sbom-backend-spdx.json`, `sbom-frontend-spdx.json`,
  `license-allowlist.json` and `aiops-platform-helm`.

#### New files

- `SECURITY.md`: supported version policy, vulnerability reporting channels
  (GitHub Security Advisory, PGP email), disclosure timeline, threat-model
  boundaries (read-only gateway, append-only audit, 404-on-unauthorized,
  external secrets, restricted pod security, network policies, offline
  readiness gates) and supply-chain controls. Includes a
  security-conscious contribution checklist.
- `CHANGELOG.md`: Keep a Changelog 1.1.0 / SemVer 2.0.0 format. Records
  the M38 release and the M32-M35 baselines with links to the change
  records.
- `docs/security/license-allowlist.json`: allowlist
  (`MIT`, `ISC`, `BSD-2-Clause`, `BSD-3-Clause`, `Apache-2.0`) and
  review-required list (`MPL-2.0`, `LGPL`, `GPL`, `UNKNOWN`,
  `SEE-LICENSE`). Documents the review policy.
- `backend/internal/deployment/license_allowlist_test.go`: two contract
  tests. `TestLicenseAllowlistRejectsReciprocalLicenses` ensures the
  allowlist itself does not admit reciprocal licenses.
  `TestDependencyLicenseInventoryStaysWithinAllowlist` parses
  `docs/thesis/dependency-licenses.md` and fails if any row uses a license
  not on the allowlist.

#### Modified files (documentation)

- `backend/internal/deployment/delivery_assets_test.go`: added required
  markers for `SECURITY.md`, `CHANGELOG.md` and
  `docs/security/license-allowlist.json`.

### M38D — Decision record and change record

#### New files

- `docs/adr/0051-engineering-delivery-and-supply-chain-hardening.md`: this
  ADR. Records the seven decisions (CI gate composition, real-kind E2E
  coverage, official Helm chart, multi-architecture release builds, SPDX
  SBOM, license allowlist enforcement, SECURITY.md/CHANGELOG.md as delivery
  assets), consequences and alternatives considered.
- `docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md`:
  this change record.

## Verification

### Fast gate

The fast gate (`scripts/verify-fast.ps1`) was run after each phase. The
final fast gate run covers:

- `git diff --check`
- `go vet` and `go test` for the deployment package (including the new
  Helm chart and license allowlist contract tests)
- frontend typecheck and Vitest (when frontend changes are staged)
- Compose config and Kustomize contracts for `deploy/kubernetes`,
  `deploy/managed-cluster`, `deploy/demo-scenarios` and
  `deploy/diagnosis-e2e`

### Contract tests

- `TestCIWorkflowContractsAreParseableAndBounded` — CI, release, real-kind
  E2E, dependabot and actionlint YAML all match the required markers and
  contain no forbidden markers.
- `TestDeliveryAssetsCoverVerificationAndThesisMaterials` — every required
  delivery asset, including the new Helm chart files, `SECURITY.md`,
  `CHANGELOG.md` and `docs/security/license-allowlist.json`, is present
  and contains the expected markers.
- `TestHelmChart*` (ten tests) — Helm chart metadata, values, schema,
  templates and security baseline.
- `TestLicenseAllowlistRejectsReciprocalLicenses` and
  `TestDependencyLicenseInventoryStaysWithinAllowlist` — license allowlist
  enforcement.

### Deferred

- **Cosign image signing.** No signing identity exists in the project yet.
  SBOM + SHA256SUMS + `--verify-tag` is the current integrity boundary.
  Deferred to a future ADR.
- **Registry push.** The release remains package-only per ADR 0028.
  Operators load the OCI tarball into their registry of choice.
- **`helm lint` in CI.** Helm is not installed in the local fast-gate
  environment; the Go contract tests provide equivalent coverage of the
  chart structure, values and templates. Adding `helm lint` to the CI
  workflow is a future enhancement once a Helm action is pinned and
  audited.
- **Real-kind E2E for M38.** M38 does not introduce a new runtime surface;
  the existing real-kind suites continue to guard the platform.

## Accepted risks

- **CI gate is heavier.** Race detection, golangci-lint, ESLint, coverage,
  oasdiff and four runtime drills must all pass. The fast gate remains the
  developer loop; CI is the authoritative gate.
- **Helm chart duplicates the kustomize baseline.** Intentional, so that
  operators who cannot use Helm still have a kustomize path. The contract
  tests for both must remain green; divergence is a defect.
- **License allowlist is conservative.** New dependencies that use MPL-2.0
  or LGPL will block the gate until an ADR records the review outcome.
