# M47: Three-Tier Console Navigation + Workspace Resource Filter

- Date: 2026-07-31
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0062](../adr/0062-three-tier-console-and-workspace-filter.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 96.99s; backend=True frontend=True manifests=True)

## Summary

Delivered the M47 backend increment that unblocks the KubeSphere-style
three-tier console (platform / workspace / cluster) and workspace-filtered
resource management. The frontend navigation refactor is out of scope for
this deliverable; only the two backend capabilities required by the roadmap
are implemented:

1. **CRD discovery preview** — `GET /api/v1/clusters/:cluster_id/api-resources`
   returns the union of a fixed operator-curated GVR whitelist and the
   cluster's dynamically discovered API resources, with graceful degradation
   when discovery is unavailable. This is the M47 preview of M49's full CRD
   browsing.
2. **`workspace_id` query parameter** — an optional visibility-narrowing
   filter on the existing namespace-scoped resource list endpoints that
   intersects the authorized namespace scope with a workspace's member
   namespaces on the current cluster.

The defining invariant: **the `workspace_id` filter is a pure visibility
filter, not an authorization decision.** The caller has already passed
`requireClusterAccess` + `requireNamespaceQueryAccess` before the filter
runs; the filter only narrows the already-authorized scope and never expands
it. This preserves the 2D authorization matrix from M35 (ADR 0050) and the
WorkspaceGrant orthogonality from M46 (ADR 0061).

Anti-leakage (404 > 403) is preserved end-to-end: an unauthorized cluster
returns 404 before the filter runs; a non-existent or unauthorized workspace
yields an empty scope so list handlers return 200 with `items: []` rather
than leaking the workspace's existence via 404.

M47 production gates (hosted CI, production OIDC/MFA, HA PostgreSQL, signed
releases, real-kind E2E) remain external and are not closed by this
development deliverable.

## Changes

### New files

- `backend/internal/httpserver/workspace_filter.go` —
  `withWorkspaceNamespaceFilter` middleware and `narrowScopeByWorkspace`
  pure function. Runs after `requireNamespaceQueryAccess`; narrows the
  resolved `authz.ClusterScope` by the workspace's member namespaces on the
  current cluster.
- `backend/internal/httpserver/workspace_filter_test.go` — 11 unit tests (4
  pure-function + 7 middleware) covering AllNamespaces narrowing,
  namespace-grant intersection, anti-leakage empty-scope collapse, invalid
  `workspace_id` 400, repository-error 500, nil-service pass-through, and
  no-param pass-through.
- `docs/adr/0062-three-tier-console-and-workspace-filter.md` — ADR
  documenting the discovery preview contract, the pure-visibility-filter
  invariant, the anti-leakage empty-list contract, and route/OpenAPI
  consistency.

### Modified files

- `backend/internal/kubernetes/service.go` — added `DiscoveryProvider`
  dependency to `Service` and `NewService`; implemented `APIResources`
  (whitelist + dynamic discovery merge with subresource/non-listable
  filtering and dedup); extracted `sortAPIResources` helper so all
  return paths (including early-return fallbacks) produce a sorted catalog;
  added `fixedAPIResources` whitelist (27 core/apps/batch/networking/
  discovery/policy/autoscaling/rbac/storage GVRs).
- `backend/internal/kubernetes/api_resources_test.go` — 7 discovery tests
  (nil-discovery whitelist-only, CRD merge with dedup/subresource skip,
  discovery-error fallback, credential-error fallback, sorted output,
  whitelist core-kind coverage).
- `backend/internal/kubernetes/service_test.go` — updated 5 `NewService`
  call sites from 2-arg to 3-arg signature (`..., nil` discovery provider).
- `backend/internal/workspace/repository.go` — added
  `ListMembershipsByCluster` to the `Repository` interface and
  `GormRepository` (returns all memberships on a cluster, ordered by
  workspace_id/namespace).
- `backend/internal/workspace/service.go` — added
  `NamespacesForWorkspaceFilter` (returns the namespaces on a cluster bound
  to a workspace; pure read, no workspace_viewer authorization — the filter
  is a visibility narrowing, not an authorization decision).
- `backend/internal/workspace/service_test.go` — 5 filter tests (zero-ID
  short-circuit, cross-workspace/cluster isolation, unknown-workspace empty
  result, no-memberships-on-cluster empty result, repository-error
  propagation); added `listMembershipsByClusterErr` field to
  `fakeRepository` for error-injection.
- `backend/internal/httpserver/kubernetes.go` — added `apiResources`
  handler for `GET /api-resources`.
- `backend/internal/httpserver/router.go` — registered
  `/clusters/:cluster_id/api-resources` route (cluster-scoped authz only);
  wired `withWorkspaceNamespaceFilter` into the namespace-scoped resource
  list route group (after `requireNamespaceQueryAccess`).
- `docs/api/openapi.yaml` — added `APIResource` and `APIResourceList`
  schemas, `WorkspaceIDQuery` reusable parameter, and the
  `/clusters/{cluster_id}/api-resources` path.

## Routes

| Method | Path | Audit action | Authorization |
|--------|------|--------------|---------------|
| GET | /api/v1/clusters/{cluster_id}/api-resources | kubernetes.api_resources.read | cluster-scoped (requireClusterAccess) |

The `workspace_id` query parameter is added to the existing namespace-scoped
resource list routes (Pods, Deployments, Services, StatefulSets, DaemonSets,
ReplicaSets, Jobs, CronJobs, HPAs, ResourceQuotas, LimitRanges, Secrets,
Ingresses, EndpointSlices, PVCs, etc.) as an optional visibility filter. It
introduces no new routes.

## Invariants enforced

1. **Pure visibility filter** — `workspace_id` only narrows the
   already-authorized scope; it never expands it and never authorizes
   namespace reads on its own.
2. **Filter ordering** — `withWorkspaceNamespaceFilter` runs strictly after
   `requireClusterAccess` and `requireNamespaceQueryAccess`; the authz scope
   is resolved before narrowing.
3. **404 > 403 anti-leakage** — unauthorized cluster returns 404 (before
   filter); non-existent/empty workspace returns 200 with `items: []` (not
   404) so workspace existence is not leaked.
4. **Graceful discovery degradation** — discovery failures (nil provider,
   credential error, API error, partial result) return the whitelist only;
   the endpoint never 500s due to discovery unavailability.
5. **Sorted output** — `APIResources` always returns resources sorted by
   group, version, resource across all return paths (including fallbacks).
6. **Whitelist dedup** — dynamic discovery never duplicates a whitelist
   entry; subresources (`pods/log`) and non-listable/non-gettable resources
   are skipped.

## Verification

- `go build ./...` — passes.
- `go vet ./internal/workspace/... ./internal/httpserver/...
  ./internal/kubernetes/...` — passes.
- `go test ./internal/kubernetes/ -count=1` — passes (7 discovery tests +
  existing rbac/service tests).
- `go test ./internal/workspace/ -run TestNamespacesForWorkspaceFilter
  -count=1` — 5 filter tests pass.
- `go test ./internal/httpserver/ -run "TestNarrowScopeByWorkspace|
  TestWithWorkspaceNamespaceFilter" -count=1` — 11 filter tests pass.
- `verify-fast.ps1 -Scope All` — fast gate passed (backend=True
  frontend=True manifests=True, 96.99s).
