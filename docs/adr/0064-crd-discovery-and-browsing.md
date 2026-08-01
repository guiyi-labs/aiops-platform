# ADR 0064: CRD Discovery + Read-Only Custom Resource Browsing (M49)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M49
- Supersedes: none
- Related: ADR 0004 (bounded read-only Kubernetes gateway), ADR 0022 (sanitized
  manifest redaction), ADR 0050 (lightweight cluster and namespace access
  grants), ADR 0061 (workspace multi-tenancy), ADR 0062 (three-tier console
  and workspace filter), ADR 0049 (route descriptor contract and RBAC
  inventory)

## Context

M47 shipped a read-only **CRD discovery preview**
(`GET /api/v1/clusters/:cluster_id/api-resources`) that returns GVR metadata
(whitelist + dynamic discovery) but explicitly defers instance browsing to M49
(ADR 0062 §1, Open Questions). The M49 roadmap entry
(`docs/post-m45-development-roadmap.md` §M49) calls for the next step: a
read-only custom resource browser that lets the console render instances of
operator CRDs (Velero backups, Prometheus stacks, Flux releases, cert-manager
certificates) without exposing a write surface or a transparent Kubernetes API
proxy.

The design space includes a dynamic GVR proxy (arbitrary CRD GET/PUT/DELETE
through one generic endpoint), a CRD-schema-driven form generator, and a
runtime-extensible whitelist. All three are rejected by the project's
static-extension hard constraints and ADR 0004's bounded-gateway model:

- A dynamic GVR proxy would let any installed CRD become reachable without a
  contract change, breaking the deterministic-browseability invariant and
  reopening the write-surface risk (the platform is read-only by design for
  operator CRDs).
- A CRD schema form generator implies a write surface (forms submit writes),
  which is explicitly deferred and out of scope for M49.
- A runtime-extensible whitelist (admin API to add GVRs) would let a
  misconfiguration or compromise broaden the reachable surface without a code
  review; the whitelist must be a compile-time contract change.

M49 therefore delivers the minimum viable CRD browsing surface: a
**compile-time-fixed GVR whitelist** of common operator CRDs, a **read-only**
list + detail endpoint pair, **manifest redaction** reused from M22, and
**authorization** reused from M35 (namespace scope) + M47 (workspace filter).
No write operations are exposed; non-whitelisted GVRs are indistinguishable
from missing resources (404, anti-leakage).

## Decision

### 1. Compile-time-fixed GVR whitelist; no runtime expansion

`customResourceWhitelist` in `internal/kubernetes/service.go` is a
package-level `map[string]customResourceDescriptor` keyed by
`"group/version/resource"`. Adding an entry is a code change reviewed through
the normal pull-request gate — there is no admin API and no discovery-based
runtime expansion. This preserves the static-extension hard constraint and
makes browseability deterministic: a GVR is browsable today if and only if it
is in the map.

The M49 whitelist covers four operator CRD families (22 entries):

- **Velero** (3): `velero.io/v1/backups`, `restores`, `schedules` —
  namespaced. (These overlap with the existing typed `backups` endpoint, which
  remains the preferred projection; the generic browser is the escape hatch
  for `restores`/`schedules` and for cross-family uniformity.)
- **Prometheus operator** (8): `monitoring.coreos.com/v1/prometheuses`,
  `alertmanagers`, `servicemonitors`, `podmonitors`, `prometheusrules`,
  `thanosrulers`, `probes`, `alertmanagerconfigs` — all namespaced.
- **Flux Helm release + source controllers** (5):
  `helm.toolkit.fluxcd.io/v2beta1/helmreleases`,
  `source.toolkit.fluxcd.io/v1/helmrepositories`, `gitrepositories`,
  `buckets`, `ocirepositories` — all namespaced.
- **cert-manager** (6): `cert-manager.io/v1/certificates`,
  `certificaterequests`, `issuers`, `clusterissuers` (cluster-scoped),
  `orders`, `challenges` — `clusterissuers` is the only cluster-scoped entry.

Core resources (empty group) are intentionally absent — they are already
covered by the typed list endpoints and the M47 `fixedAPIResources` whitelist.
The `customResourceDescriptor` carries a single `Namespaced bool` field that
drives both API path construction (`/namespaces/{ns}/...` vs cluster-scoped)
and the handler's fan-out decision.

### 2. Read-only list + detail endpoints; writes are not registered

Two routes are registered in the `resourceRoutes` group:

- `GET /api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource`
  — list (audit action `kubernetes.custom_resources.list`).
- `GET /api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource/:name`
  — detail (audit action `kubernetes.custom_resources.read`).

Only the `GET` verb is registered. There is no `POST`/`PUT`/`PATCH`/`DELETE`
route for custom resources, and the `kubernetes.Service` exposes no
`CreateCustomResource`/`UpdateCustomResource`/`DeleteCustomResource` method.
The read-only contract is enforced structurally (absent routes + absent
service methods), not by an authorization check — a request to write cannot
match a route and therefore cannot reach the service. This mirrors the
platform's broader read-only posture for operator CRDs and the bounded-gateway
model (ADR 0004).

### 3. Manifest redaction reused from M22 (ADR 0022)

Both the list and detail endpoints return the raw CRD manifest as a
`map[string]interface{}` and run it through `redactManifest` (the M22
Secret/ConfigMap redaction helper) before returning. Sensitive field names
(`password`, `secret`, `token`, `key`, `credential`, ...) are recursively
redacted to the literal string `"<redacted>"`, including nested objects and
array elements. This reuses the existing, audited redaction contract rather
than introducing a CRD-specific redactor.

The redaction is applied **after** the Kubernetes API response is decoded and
**before** pagination/filtering, so a redacted field can never leak through
the list envelope. The detail endpoint redacts the single object in place.

### 4. Authorization: M35 namespace scope + M47 workspace filter; 404 > 403 anti-leakage

The custom-resources routes are registered in the `resourceRoutes` group,
which applies the full middleware chain:

1. `withAuthentication` — bearer/session auth.
2. `withClusterContext` — resolves `cluster_id`.
3. `requireClusterAccess` — gates cluster access (404 on denial, never 403 —
   ADR 0050 anti-leakage).
4. `requireNamespaceAccess` — gates the `:namespace` **path** parameter
   (unused by these routes, which take GVR in the path, but harmless).
5. `requireNamespaceQueryAccess` — resolves the caller's
   `authz.ClusterScope` for the cluster and stores it in the Gin context.
6. `withWorkspaceNamespaceFilter` — narrows the resolved scope by the
   optional `?workspace_id` query parameter (M47 pure visibility filter).

The handler then branches on `Namespaced`:

- **Cluster-scoped CRD** (e.g. `clusterissuers`): the namespace dimension is
  irrelevant. The handler calls `CustomResources` once with an empty
  namespace. Authorization is cluster-scoped only (the caller already passed
  `requireClusterAccess`).
- **Namespaced CRD**: the handler calls `authorizedNamespaceLists`, which
  fans out across the caller's resolved `ClusterScope` — either the explicit
  `?namespace=` query (authz-checked by `requireNamespaceQueryAccess`) or the
  full authorized namespace set. `?workspace_id` (already applied by the
  middleware) narrows the set further. An empty authorized scope yields an
  empty list (200 with `items: []`), not 404 — the anti-leakage contract.

Non-whitelisted GVRs are rejected **before** the gateway is called.
`IsCustomResourceBrowsable` returns `(false, false)` and the handler writes
404 `RESOURCE_NOT_FOUND` with the same body as a genuinely missing resource.
The gateway is never contacted, so a non-whitelisted CRD is indistinguishable
from a missing one — the caller cannot probe which CRDs are installed. This
is the same anti-leakage discipline as ADR 0050/0061/0063.

### 5. Bounded list with pagination, name filter, and selectors

The list endpoint reuses the platform's standard `apiquery.ListQuery` parsing
(`page`, `limit`, `sort_by`, `ascending`, `name`, `label_selector`,
`field_selector`). `limit` is capped at 100 by `apiquery.Parse`. The service
calls `getJSON` with the selectors, decodes the Kubernetes list envelope, and
applies `filterAndPage` (name substring filter + offset pagination) on the
redacted items. `total` counts named items matching the `name` filter;
`remaining` is `total - offset - len(items)`. This mirrors the typed list
endpoints (Pods, Deployments, ...) so the frontend list components are
reusable.

### 6. Detail endpoint: namespace required for namespaced CRDs

The detail handler validates that for namespaced CRDs the `?namespace=` query
parameter is present (400 `INVALID_QUERY` if absent). For cluster-scoped CRDs
the namespace query is ignored. This asymmetry is intentional: a namespaced
CRD instance has no unique identity without its namespace, and the Kubernetes
API would 404 on a cluster-scoped path anyway — surfacing 400 early gives the
frontend a clearer contract than letting the gateway 404.

### 7. Route registration and OpenAPI contract

Both routes are registered in `internal/httpserver/router.go` with
`AuditAction` and `AuditResource` metadata (ADR 0049) and documented in
`docs/api/openapi.yaml`:

- `GET /clusters/{cluster_id}/custom-resources/{group}/{version}/{resource}`
  — list, returns `CustomResourceList`.
- `GET /clusters/{cluster_id}/custom-resources/{group}/{version}/{resource}/{name}`
  — detail, returns `CustomResource`.

Two schemas are added: `CustomResource` (a free-form object — CRD manifests
vary per GVR, so a fixed schema would be misleading) and `CustomResourceList`
(`items` + `total` + `remaining`). Route-contract consistency (ADR 0049) is
maintained: the OpenAPI document and the route registrar agree on path,
method, parameters, and response schema. `TestRegisteredRoutesMatchOpenAPI`
covers both routes.

## Consequences

- **Positive**: Operators get a uniform, read-only browser for the most
  common operator CRDs without a per-CRD typed endpoint. The compile-time
  whitelist keeps the reachable surface deterministic and reviewable.
- **Positive**: The read-only contract is structural (no write routes, no
  write service methods), not advisory. A request to write cannot match a
  route.
- **Positive**: Redaction is reused from M22, so the audited Secret/ConfigMap
  redaction contract covers CRD manifests without a new redactor.
- **Positive**: Authorization is reused from M35 (namespace scope) + M47
  (workspace filter). No new authorization path is introduced; the 2D
  authorization matrix (ADR 0050/0061) is intact.
- **Positive**: Anti-leakage is preserved end-to-end. Non-whitelisted GVRs
  return 404 indistinguishable from missing resources; the gateway is never
  contacted for them.
- **Negative**: The whitelist is fixed at compile time. Adding a new operator
  CRD family (e.g. an IngressNGINX or a future CRD) requires a code change
  and re-deploy. This is intentional (static-extension hard constraint) but
  means the browser cannot cover a newly installed CRD without a release.
- **Negative**: `CustomResource` is a free-form object (`additionalProperties:
  true`) in the OpenAPI schema. Frontend type-safety is weaker than for the
  typed endpoints; the frontend must render the manifest generically. A
  per-GVR typed schema was considered and rejected as over-engineering for a
  read-only browser.
- **Neutral**: The Velero `backups` GVR overlaps with the existing typed
  `/backups` endpoint. Both remain: the typed endpoint returns the bounded
  Velero projection (phase, errors, warnings, ...); the generic browser
  returns the raw redacted manifest. They serve different console views.
- **Neutral**: Discovery (M47 `api-resources`) and browsing (M49
  `custom-resources`) are deliberately separate endpoints. Discovery returns
  GVR metadata for the resource catalog; browsing returns instances. A
  non-whitelisted GVR may appear in discovery (if installed) but is not
  browsable — this is correct, since discovery is metadata-only and browsing
  is instance-level.

## Open Questions

None for M49. Future milestones may address:
- A CRD-schema-driven read-only detail renderer (form-style display of the
  OpenAPI v3 schema in the CRD's `spec.versions[].schema`). Out of scope for
  M49; the manifest is rendered generically.
- Cross-cluster CRD browsing (browsing instances across federated clusters,
  M48). Out of scope for M49; browsing is single-cluster via `cluster_id`.
- A runtime whitelist extension mechanism guarded by a four-eyes admin API.
  Explicitly rejected for M49 (static-extension hard constraint); revisit
  only if the static whitelist proves operationally unsustainable.
