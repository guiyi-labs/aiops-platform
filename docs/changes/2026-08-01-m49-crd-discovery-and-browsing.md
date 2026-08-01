# M49: CRD Discovery + Read-Only Custom Resource Browsing

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0064](../adr/0064-crd-discovery-and-browsing.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 58.96s; backend=True frontend=True manifests=True)

## Summary

Delivered the M49 backend increment implementing a read-only custom resource
browser for operator CRDs. Building on the M47 CRD discovery preview (which
returns GVR metadata only), M49 adds instance-level browsing: list + detail
endpoints that return redacted manifests for whitelisted CRD GVRs.

The design is bounded by three structural invariants:

1. **Compile-time-fixed whitelist** — `customResourceWhitelist` is a
   package-level map in `internal/kubernetes/service.go` keyed by
   `"group/version/resource"`. Adding an entry is a code change reviewed
   through the normal gate; there is no admin API and no discovery-based
   runtime expansion (static-extension hard constraint). The M49 whitelist
   covers 22 GVRs across four operator CRD families: Velero (3), Prometheus
   operator (8), Flux Helm/source (5), cert-manager (6). One entry
   (`cert-manager.io/v1/clusterissuers`) is cluster-scoped; the rest are
   namespaced.
2. **Read-only** — only `GET` routes are registered. There is no
   `POST`/`PUT`/`PATCH`/`DELETE` route and no
   `CreateCustomResource`/`UpdateCustomResource`/`DeleteCustomResource`
   service method. The read-only contract is enforced structurally (absent
   routes + absent methods), not by an authorization check.
3. **Manifest redaction reused from M22** — both endpoints run the raw CRD
   manifest through `redactManifest` (the M22 Secret/ConfigMap redaction
   helper) before returning. Sensitive fields (`password`, `secret`, `token`,
   `key`, ...) are recursively redacted to `"<redacted>"`.

Authorization reuses the M35 namespace scope + M47 workspace filter without
introducing a new authorization path. The routes are registered in the
`resourceRoutes` group, which applies the full middleware chain
(`requireClusterAccess` → `requireNamespaceQueryAccess` →
`withWorkspaceNamespaceFilter`). Namespaced CRDs fan out across the caller's
resolved `ClusterScope` via `authorizedNamespaceLists`; cluster-scoped CRDs
are listed cluster-wide. The `?workspace_id` filter narrows the namespace set
further (M47 pure visibility filter).

Anti-leakage (404 > 403) is preserved: a non-whitelisted GVR returns 404
`RESOURCE_NOT_FOUND` with the same body as a genuinely missing resource, and
the gateway is never contacted — the caller cannot probe which CRDs are
installed. An empty authorized scope yields an empty list (200 with
`items: []`), not 404.

M49 production gates (hosted CI, production OIDC/MFA, HA PostgreSQL, signed
releases, real-kind E2E) remain external and are not closed by this
development deliverable.

## Changes

### Modified files

- `backend/internal/kubernetes/service.go` — added the M49 CRD browsing
  surface: `ErrCustomResourceNotWhitelisted` sentinel, `customResourceDescriptor`
  type, `customResourceWhitelist` map (22 GVRs across Velero/Prometheus
  operator/Flux/cert-manager), `IsCustomResourceBrowsable` method,
  `CustomResources` list method (reuses `getJSON` + `redactManifest` +
  `filterAndPage`), `CustomResource` detail method, `customResourceListPath` /
  `customResourcePath` path builders (namespaced vs cluster-scoped), and
  `customResourceName` metadata extractor.
- `backend/internal/httpserver/kubernetes.go` — added `customResources` and
  `customResource` handlers. The list handler branches on `Namespaced`:
  cluster-scoped CRDs call the service once; namespaced CRDs fan out via
  `authorizedNamespaceLists` (M35 scope) honoring the `?workspace_id` filter
  (M47). The detail handler requires `?namespace=` for namespaced CRDs (400
  otherwise) and ignores it for cluster-scoped CRDs. Added the
  `ErrCustomResourceNotWhitelisted` → 404 `RESOURCE_NOT_FOUND` mapping to
  `writeServiceError` (anti-leakage).
- `backend/internal/httpserver/router.go` — registered two read-only routes
  in the `resourceRoutes` group: `GET /custom-resources/:group/:version/:resource`
  (audit `kubernetes.custom_resources.list`) and
  `GET /custom-resources/:group/:version/:resource/:name` (audit
  `kubernetes.custom_resources.read`). Only `GET` is registered — no write
  surface.
- `docs/api/openapi.yaml` — added 2 custom-resources paths and 2 schemas
  (`CustomResource` free-form object, `CustomResourceList` with `items` +
  `total` + `remaining`).

### New files

- `backend/internal/kubernetes/custom_resources_test.go` — 17 unit tests
  covering whitelist allow/deny, namespaced vs cluster-scoped path building,
  list + detail with redaction, name filter + sort, selector forwarding,
  cluster-disabled propagation, not-found propagation, and path/name helpers.
- `backend/internal/httpserver/custom_resources_test.go` — 17 handler tests
  covering list (namespaced/cluster-scoped/namespace-query/namespace-grant
  fan-out/empty-scope), detail (namespaced/cluster-scoped), 400 (invalid
  query, missing namespace for namespaced detail), 404 (non-whitelisted,
  not-found), 409 (cluster disabled), label-selector forwarding, only-GET
  registered, and `writeServiceError` whitelisted mapping.
- `docs/adr/0064-crd-discovery-and-browsing.md` — ADR documenting the 7 key
  decisions: compile-time whitelist, read-only routes, M22 redaction reuse,
  M35+M47 authorization with 404 > 403 anti-leakage, bounded list with
  pagination, detail namespace requirement, and route/OpenAPI contract.

## Verification

- `go test ./internal/kubernetes/ -run TestCustomResource`: 17 tests, all
  pass.
- `go test ./internal/httpserver/ -run TestCustomResource`: 17 tests, all
  pass.
- `TestRegisteredRoutesMatchOpenAPI`: pass — both custom-resources routes
  are registered and documented (route-contract consistency, ADR 0049).
- `go vet ./...`: clean.
- `verify-fast.ps1 -Scope All`: pending (will be run as the final gate).

## Key invariants maintained

- **Compile-time whitelist**: `customResourceWhitelist` is a package-level
  map; no runtime expansion. Browseability is deterministic.
- **Read-only (structural)**: only `GET` routes registered; no write service
  methods. A write request cannot match a route.
- **M22 redaction reuse**: `redactManifest` applied to every returned
  manifest; sensitive fields → `"<redacted>"`.
- **M35 + M47 authorization reuse**: namespaced CRDs fan out across the
  caller's `ClusterScope`; `?workspace_id` narrows the set. No new
  authorization path; 2D matrix intact.
- **404 > 403 anti-leakage**: non-whitelisted GVR → 404
  `RESOURCE_NOT_FOUND` before gateway contact; empty scope → 200 `items: []`.
- **Static extension model**: no dynamic GVR proxy, no runtime whitelist, no
  CRD schema form generator. Cross-cut by ADR 0004 (bounded gateway).
