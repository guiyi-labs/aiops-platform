# M59 (Structural) + M60: Signed Releases + SLSA Provenance (Placeholders) and Static Provider Registry

- Date: 2026-08-02
- Status: Development Complete (backend increment only; local development deliverables; M59 signing/provenance steps are structural — exercised on release tags only)
- ADR: [0075](../adr/0075-m59-signing-provenance-m60-provider-registry.md)
- Fast gate: PASSED — `verify-fast.ps1 -Scope All` in 76.71s (backend=True frontend=True manifests=True; helm lint CI-enforced)

## Summary

Closes Phase 5 of the post-M45 roadmap with two combined deliveries:

### M60: Compile-time provider registry + lifecycle + health + cluster-role selectors (full implementation)

The backend now has a single centralized `capability.Registry` that
unifies the ten distinct capability surfaces previously wired ad-hoc
into `cmd/server/main.go` and the HTTP router. Every provider now:

1. Carries a stable `ProviderDescriptor` — name, kind, description,
   declared dependencies, cluster role eligibility, configured flag,
   optional Lifecycle (start/stop background goroutines), optional
   HealthChecker (probe the upstream).
2. Starts in deterministic topological order (dependencies before
   dependents) and stops in reverse topological order. This eliminates
   the prior startup race between the M52 inspection scheduler and the
   Prometheus capability provider.
3. Honors cluster-role gating. `federation`, `inspection_scheduler`, and
   `copyops_cross_cluster` are only started when the process role set
   includes `host` or `standalone`; member clusters (per M48 federation)
   keep them in `state = disabled` with `InactiveReason = "cluster role"`
   — visible in the GUI, never started.
4. Reports cached health (1s cache, refreshable via `refresh=true`
   query param). Non-configured or role-disabled providers skip probing
   entirely, so there is no "false unhealthy" in the GUI.
5. Exposes a new OpenAPI surface:
   - `GET /api/v1/capability/providers` — list all providers.
   - `GET /api/v1/capability/providers/{name}` — single provider detail +
     optional health refresh.
   Both routes are protected by the new `system_ops_admin` scope and use
   `ProviderInfo` as the response schema (1:1 with the registry struct).

Registered providers (M60 stable catalog):
- `metrics_prometheus` (capability, Configured iff cfg.Capability.PrometheusEndpoint is set)
- `logs_loki` (capability, Configured iff cfg.Capability.LokiEndpoint is set)
- `federation` (federation, roles = host | standalone, dependency root)
- `inspection_scheduler` (inspection, roles = standalone | host, depends on `metrics_prometheus`)
- `service_mesh_readonly` (mesh, depends on `metrics_prometheus`)
- `gitops_argocd` (gitops, no dependencies)
- `copyops_cross_cluster` (copyops, roles = host | standalone, depends on `federation`)
- `app_catalog_helm` (appcatalog, no dependencies)
- `backup_restore_velero` (backup, no dependencies)
- `ai_investigator` (ai, Configured = cfg.AIEnabled)

### M59 Structural: Cosign keyless signing + in-toto SLSA v1 provenance placeholders

`.github/workflows/release.yml` now extends the `package` job with two
signing gates plus a provenance placeholder. All steps use standard
Sigstore tooling pinned to specific release SHAs:

- `sigstore/cosign-installer@…` → Cosign v2.4.1.
- `cosign sign-blob --tlog-upload=true` signs both the SHA256SUMS
  manifest and the release-metadata.json keylessly; the Fulcio x509 cert
  and Rekor signature are published alongside each blob
  (`SHA256SUMS.sig` / `.cert.pem`, `release-metadata.json.sig` / `.cert.pem`).
- A placeholder `aiops-platform-${VERSION}-provenance.in-toto.json`
  statement is generated with one subject:
  `{name: "release-artifacts-bundle", digest: {sha256: bundle_digest}}`.
  `bundle_digest` is an aggregate sha256(sha256(all files in the
  release bundle)) so the provenance covers the *signed* release, not
  the pre-signing intermediates.
- `cosign attest-blob` (placeholder) binds the provenance statement to
  the bundle subject, producing `aiops-platform-${VERSION}-provenance.sig`
  and `.cert.pem`. The step uses `|| true` fail-open so a rehearsal run
  without Rekor access does not break release CI — the maintainers are
  expected to swap this placeholder + the in-toto generator for the
  upstream `slsa-framework/slsa-github-generator` when the repository's
  builder-level provenance project is registered.
- SHA256SUMS is re-hashed at the end so signatures and provenance files
  are themselves covered by the final manifest.

HA and PITR are *not* implemented in M59. ADR 0075 marks their place in
the signing chain so they can reuse the same workflow identity when
they land.

## Files Changed

### New Files

- `backend/internal/capability/registry.go` — `Registry` struct +
  compile-time catalog API. `ProviderDescriptor`, `ProviderInfo`,
  `ProviderState*` state enum, `ErrProviderAlreadyRegistered` /
  `ErrProviderNotFound` / `ErrProviderNotStarted` /
  `ErrInvalidProviderName` / `ErrCyclicDependency` sentinels,
  `Lifecycle` and `HealthChecker` optional interfaces, three-color DFS
  topological sort, cache-1s health probes with panic-safe wrappers
  (`safeStart` / `safeStop` / `safeCheckHealth`), lexical name
  validation (`[a-z][a-z0-9_-]{1,63}`), `ClusterRoles` gating.
- `backend/internal/capability/registry_test.go` — 12 table tests
  covering Register duplicate/invalid-name branches, missing dep vs
  out-of-order registration, cycle detection, Start/Stop dependency
  ordering (4-node diamond graph), ClusterRoles gating (member-only
  role set), Configured+health+1s cache behavior, Get/CheckHealth
  miss/ErrProviderNotFound, StopAll ctx timeout via ctx-aware stub
  Lifecycle, IsEnabled/ClusterSelector/Dependencies helpers. Statement
  coverage = 84.2 % (≥ 80 % gate).

### Modified Files

- `backend/internal/httpserver/router.go` — new `CapabilityRegistry *capability.Registry`
  option field. `capabilityRoutes` now conditionally registers
  `/api/v1/capability/providers` and `/providers/:name` when
  `CapabilityRegistry != nil` (mirrors the existing
  `CapabilityMetricsProvider != nil` guard for the M37 SLI/log routes).
  The two new routes are placed under `api/v1/capability` group and use
  the `system_ops_admin` auth scope.
- `backend/internal/httpserver/capability.go` — `capabilityHandler` now
  carries `registry *capability.Registry` (mirroring the existing
  `MetricsProvider` / `LogProvider` fields). Added `listProviders` and
  `getProvider` handler methods. `listProviders` returns the registry's
  `List()` output sorted alphabetically; `getProvider` supports
  `?refresh=true` which bypasses the registry's 1s health cache via a
  `CheckHealth(ctx, name)` call. Both handlers return 503 with a JSON
  body when the registry is nil.
- `backend/internal/httpserver/openapi_route_test.go` — new
  `mustProviderRegistryForContract(t)` helper builds a minimal registry
  populated with the ten-entry stable catalog; injects it into
  `Options.CapabilityRegistry` so the route-contract test covers the
  new providers routes.
- `docs/api/openapi.yaml` — new `ProviderInfo` schema with every
  registry field (`name`, `description`, `version`, `kind`,
  `dependencies`, `cluster_roles`, `configured`, `state`,
  `health_reason`, `started_at`, `last_check`) plus two new path
  entries under `/api/v1/capability/providers` and
  `/api/v1/capability/providers/{name}`. `TestRegisteredRoutesMatchOpenAPI`
  passes with the new entries.
- `backend/cmd/server/main.go` — after capability providers are
  configured, builds a `capability.NewRegistry` with the
  `{standalone, host, member}` role set, registers all ten providers
  with correct dependency edges (copyops → federation,
  inspection_scheduler/service_mesh → metrics_prometheus), calls
  `capabilityRegistry.StartAll(backgroundContext)` after the three
  background goroutines are launched, calls
  `capabilityRegistry.StopAll(shutdownCtx)` before `server.Shutdown`
  during graceful teardown. Start errors are logged as `Warn` (never
  fatal); stop errors are aggregated and logged similarly.
  `Options.CapabilityRegistry` is set so the HTTP routes are wired.
- `.github/workflows/release.yml` — id-token:write permission added to
  `package` job; new Cosign install step; SHA256SUMS + release-metadata
  keyless signing step with bundle_digest output; SLSA v1 in-toto
  provenance placeholder (structural); cosign attest-blob placeholder;
  final SHA256SUMS rehash. `signed_digest` job output exposes the
  bundle digest for downstream provenance consumers.

## Verification

Fast gate (per ADR 0075 §5):

```text
go test -count=1 ./internal/capability/... -cover
  → ok …  coverage: 84.2% of statements.

go test -count=1 ./internal/httpserver/... -run TestRegisteredRoutesMatchOpenAPI
  → PASS  (covers new OpenAPI routes + schema)

go build ./cmd/server/...  → success (no capability/registry link errors)
```

M59 signing and provenance steps are structural-only. They are not
exercised by the local development fast gate; a `workflow_dispatch`
release rehearsal with a throwaway `vX.Y.Z-rc.1` tag is the maintainer
handoff step that validates them.

## Maintainer Handoff

To activate the M59 placeholders into a full SLSA-3 pipeline:

1. Register the repository with
   `slsa-framework/slsa-github-generator` via the builder-level
   provenance onboarding flow (the generator project tracks a repo
   allowlist).
2. Replace the two "M59 structural placeholder" steps in
   `.github/workflows/release.yml` with the
   `slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml`
   reusable workflow, feeding it the same subject (bundle_digest) that
   the placeholder currently emits.
3. Remove the `|| true` fail-open from the `cosign attest-blob` step —
   once the real generator is wired the attestation must fail-closed.
4. Air-gapped installations: override Cosign flags with `--key` + a
   repository-owned keypair to bypass Fulcio/Rekor. The
   `signed_digest` output and `ProviderInfo` wire format are
   key-type agnostic.
