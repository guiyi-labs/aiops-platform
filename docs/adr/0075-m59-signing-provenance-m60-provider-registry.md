# ADR 0075: M59 Signed Releases + SLSA Provenance (Structural) and M60 Compile-Time Provider Registry

- Date: 2026-08-02
- Status: Accepted
- Milestones: M59, M60
- Supersedes: none
- Related: ADR 0038 (engineering-delivery & supply chain), ADR 0051 (delivery
  hardening), ADR 0069 (Helm app catalog & controlled deploy), ADR 0070
  (DevOps read-only & copyops & backup GUI)

## Context

Phase 5 (delivery & operations closure) of the post-M45 roadmap adds two
complementary platform-wide enhancements:

1. **M59 — Managed CI + Real-Kind E2E matrix + Signed Releases with HA,
   PITR, and provenance (structural)**. The existing release workflow
   (`.github/workflows/release.yml`) builds multi-architecture OCI images,
   produces SPDX SBOMs via Syft, and uploads an `aiops-platform-${VERSION}`
   artifact bundle. Before M59, however, the bundle was *unsigned* — a
   consumer of the release artifacts had no way to verify the SHA256SUMS
   manifest or the release metadata JSON came from this repository's
   workflow. M59 adds:
   - Cosign keyless signing (Fulcio x509 cert + Rekor transparency log)
     for the SHA256SUMS manifest and the release metadata JSON.
   - Structural SLSA v1 in-toto provenance — a placeholder the
     maintainers can later swap for `slsa-framework/slsa-github-generator`
     once the repository's builder-level provenance project is registered.
   - The cosign `attest-blob` step binds the provenance statement to the
     signed bundle digest so downstream consumers can enforce both
     signature *and* provenance checks in CI.
   HA (high-availability) and PITR (point-in-time restore) are *not*
   implemented in this milestone — ADR 0075 marks their place in the
   signing chain so they can reuse the same keypair/keyless-identity when
   they arrive.

2. **M60 — Static extension framework (provider registry) + Provider
   lifecycle/health/cluster-role selectors**. By M58 the backend already
   contains ten distinct "capability" surfaces: metrics (prometheus), logs
   (loki), federation, inspection, service-mesh readonly, GitOps,
   copyops, Helm app catalog, Velero backup/restore, and the AI
   investigator. Before M60 each capability was wired ad-hoc into
   `cmd/server/main.go` and the HTTP router via nil-gated Options fields.
   Adding a new capability required edits in at least four places, there
   was no uniform startup/shutdown order (leading to startup races between
   inspection_scheduler and the metrics provider), and the GUI had no
   unified "show me the health of every provider" endpoint. M60 resolves
   this with a single Registry (ADR 0075 §2) that unifies registration,
   lifecycle (Start/Stop), health probes, cluster-role gating, and
   dependency order.

Key non-functional requirements:

- **Compile-time catalog.** The registry's full provider catalog must be
  visible from `main.go`; no dynamic loading, no plugin system. This
  keeps the platform reproducible and avoids CGO/plugin complexity on
  Windows/macOS developer workstations.
- **Startup order must be deterministic.** `inspection_scheduler` depends
  on `metrics_prometheus`; `copyops_cross_cluster` depends on
  `federation`. Providers must start in topological order and stop in
  *reverse* topological order.
- **Cluster-role gating is per-process.** A member cluster (per M48
  federation) must not start the `federation`, `inspection_scheduler`, or
  `copyops_cross_cluster` providers. They must be visible in the registry
  (so the GUI can show them as "role-disabled") but never started.
- **Signing must be keyless.** Maintainers must not manage long-lived
  signing keys. Cosign keyless mode binds the workflow's OIDC identity to
  a Fulcio certificate; the identity is verifiable offline using the
  standard workflow issuer (`token.actions.githubusercontent.com`).
- **Provenance must be structural (fail-closed).** The provenance step is
  a placeholder that produces a valid in-toto statement but does not yet
  enforce SLSA-3. Once the slsa-github-generator integration is wired in
  the placeholder step must be replaced, not extended.

## Decisions

### 1. M60 Registry API is kept deliberately small — Register / StartAll / StopAll / List / Get / CheckHealth

The full registry surface is:

```go
type Registry struct { /* unexported */ }

func NewRegistry(clusterRoles []string, healthTimeout time.Duration) *Registry
func (r *Registry) Register(desc ProviderDescriptor) error
func (r *Registry) StartAll(ctx context.Context) error
func (r *Registry) StopAll(ctx context.Context) error
func (r *Registry) List() []ProviderInfo
func (r *Registry) Get(name string) (ProviderInfo, error)
func (r *Registry) CheckHealth(ctx context.Context, name string) (ProviderInfo, error)
// helpers for code that wants to reason about providers without listing
func (r *Registry) IsEnabled(name string) bool
func (r *Registry) Dependencies(name string) ([]string, error)
func (r *Registry) ClusterSelector(name string, clusterRole string) bool
```

Explicitly rejected alternatives:

- *Go plugins (plugin.Open).* Would require CGO, break cross-compile to
  Windows/macOS dev hosts, and the provider catalog would no longer be
  visible at compile time. Fail.
- *Service-locator / global `var Registry`.* Makes testing harder (global
  mutable state) and creates hidden coupling between packages. The
  registry is explicitly passed via `httpserver.Options.CapabilityRegistry`
  like every other service.
- *Lazy start on first use.* Creates unbounded startup latency on the
  first `GET /api/v1/capability/providers?refresh=true` call. StartAll is
  invoked explicitly from main.go right after the notification, metrics
  collector, and alert scheduler background goroutines are started, so the
  GUI sees a stable "already started" state.

ProviderInfo maps 1:1 onto the new OpenAPI `ProviderInfo` schema defined
in `docs/api/openapi.yaml` paths `/api/v1/capability/providers` and
`/api/v1/capability/providers/{name}`. No conversions at the HTTP handler
boundary — the handler directly serializes `capability.ProviderInfo`.

### 2. Dependency graph uses DFS topological sort with explicit cycle detection

`topologicalOrderLocked` performs a standard three-color DFS (`white` →
`gray` → `black`) over `r.order` (registration order). Back edges from
`gray` → `gray` return `ErrCyclicDependency` *before* any Start is called,
so half-started graphs never escape to callers. The same order is used by
both `StartAll` (dependencies first) and `StopAll` (reversed, so
dependents shut down before dependencies — critical for `inspection`
stopping before the Prometheus client is torn down).

Missing dependencies are *not* fatal at registration time: descriptors
may arrive in any order from the `Register(…)` calls in main.go. A
missing dep is caught later: `StartAll` skips the dependent provider and
marks its state as `ProviderStateDegraded` with reason `"one or more
dependencies failed to start"`, which the GUI displays inline.

### 3. Health checks have a 1s cache; non-configured/role-disabled providers never probe

`CheckHealth` always returns the cached `<lastCheck + 1s` result for any
provider. Cache key is per-provider (separate probes). HealthChecker
implementations are invoked via `safeCheckHealth` which recovers panics
and translates them to `ProviderStateUnhealthy + "health check panicked"`
so a misbehaving provider never takes the registry down.

Providers whose `Configured == false` OR role does not match the process
role set are short-circuited directly — `HealthChecker` is *never*
invoked, and the GUI sees `state=disabled` instead of a false unhealthy.

### 4. M59 signing is SHA256SUMS-first. Provenance attestation attests the *signed* bundle digest, not the raw images

Signing order in the `package` job:

1. Build images, generate SBOMs, assemble release assets, hash them all
   into `SHA256SUMS`.
2. `cosign sign-blob` SHA256SUMS → `SHA256SUMS.sig` + `SHA256SUMS.cert.pem`.
3. `cosign sign-blob` release-metadata.json → `.sig` + `.cert.pem`.
4. Compute an aggregate `bundle_digest = sha256(sha256(all files in .artifacts/release/*))`
   and emit it as a step output.
5. Build in-toto provenance statement whose subject is
   `{name: "release-artifacts-bundle", digest: {sha256: bundle_digest}}`.
6. `cosign attest-blob` the provenance statement → `.sig` + `.cert.pem`.
7. Re-hash SHA256SUMS so the signatures and provenance attestation files
   are themselves included in the final checksum manifest.

Why sign SHA256SUMS first rather than sign every file individually?
Offline verifiers compute a single `sha256sum -c SHA256SUMS` after
validating the signature — O(1) signature verification even for large
release bundles. Individual signatures are additive (they add security
but require N verifications); SHA256SUMS gives one consistent root of
trust.

Cosign attest-blob takes `/dev/null` as its blob because the *real*
subject is embedded inside the provenance predicate (the
release-artifacts-bundle digest). The step is wrapped in `|| true` so
the structural placeholder never breaks a real release run. Once the
maintainers wire in slsa-github-generator the placeholder is replaced
wholesale.

## Consequences

### Positive

- M60: Adding a new provider is now a *single* change to
  `cmd/server/main.go` + a descriptor registration. HTTP routes, route
  contract tests, health rendering, and lifecycle ordering come "for
  free" from the registry.
- M60: Startup/shutdown is deterministic. The inspection scheduler can no
  longer race against the Prometheus client's initialization.
- M59: Release artifacts are verifiable offline with standard cosign
  CLI. No maintainer-managed signing keys.
- M59: Provenance subject is the *signed* bundle, so the signature and
  provenance chains compose cleanly.

### Risks / follow-ups

- M59: Cosign keyless mode requires public internet access to Fulcio and
  Rekor. Air-gapped installations must either (a) run their own Sigstore
  stack and add `COSIGN_REKOR_URL` / `COSIGN_FULCIO_URL` overrides, or
  (b) fall back to a repository-level key pair. The structural steps
  above are compatible with both modes — `cosign sign-blob` accepts a
  `--key` flag that replaces the OIDC flow.
- M60: Provider descriptors are registered after background goroutines
  start but before `http.Server.ListenAndServe`. A future enhancement
  (post-Phase 5) may add `runtime ReRegister` for hot-swap, but the ADR
  explicitly keeps registration compile-time for now.
- M60: No background periodic re-health — GUI drives refresh via
  `refresh=true` query param. If SREs want a periodic re-prober they can
  add one scheduled goroutine that calls CheckHealth on an interval; the
  1s cache prevents GUI and re-prober from both paying the probe cost.

## Verification gates (§5, for CI/verify scripts)

- `go test ./internal/capability/… -cover` must report ≥ 80 % statement
  coverage.
- `TestRegisteredRoutesMatchOpenAPI` in `internal/httpserver` must pass
  (covers the new `/api/v1/capability/providers` routes).
- M59 signing / provenance steps are structural — they are only
  exercised on real release tags (they rely on GitHub OIDC + Fulcio +
  Rekor). A manual release rehearsal with `workflow_dispatch` and a
  rehearsal version is sufficient to verify them before the next GA tag.
