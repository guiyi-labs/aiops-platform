# ADR 0049: Route Descriptor Contract and RBAC Inventory

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M34, ADR 0004 (bounded read-only Kubernetes gateway), ADR 0008 (sanitized append-only audit trail), ADR 0039 (governance workbench)
- Supersedes: the hardcoded `auditedOperation` map in `backend/internal/httpserver/audit.go` (audit classification now derives from `RouteDescriptor`)

## Context

`docs/kubesphere-optimization-plan.md` §4.2 ("RBAC resource contract debt") records that
ADR 0039 promised a fixed read-only contract for Role, ClusterRole, RoleBinding and
ClusterRoleBinding, but the Gin routes, OpenAPI, frontend and managed-cluster observer
RBAC were never implemented. §4.3 ("Documentation truth debt") records that Gin route,
OpenAPI, role check and audit action metadata had drifted apart: the audit middleware
classified routes through a hardcoded `map[string][2]string` that had to be hand-synced
with `router.go`, while role checks were applied inline at each `group.Handle` call. The
same route could appear in three places (Gin registration, audit map, OpenAPI document)
with no enforced parity.

The KubeSphere comparison (`docs/kubesphere-optimization-plan.md` §2) noted that
KubeSphere drives request classification from a single `RequestInfo`, while the current
platform scattered route metadata across three sources. M34 is the milestone that closes
both debts before M35/M36/M37 may proceed.

## Decision

### 1. RouteDescriptor is the single source of truth for route metadata

A new `RouteDescriptor` struct in `backend/internal/httpserver/routes.go` carries, for
every API route, its method, path, handler, auth requirement, required roles, audit
action and audit resource. A `routeRegistrar` binds the auth middleware once and exposes
`register(group, descriptor)` so descriptors can declare `AuthRequired` without the caller
repeating the auth service.

`New()` registers every `/api/v1` route through `reg.register`. The same call appends a
`registeredRoute` to a package-level `routeTable`. The audit middleware
(`auditedOperation`) now delegates to `findAuditedRoute` over `routeTable` instead of a
hardcoded map. The previous `operations` map is deleted.

This means the Gin route, the role check, the audit action and the audit resource are
defined in exactly one place per route. Adding a route without going through the registrar
is detected by `TestRouteTableCoversAllGinRoutes`; declaring audit metadata without a
resource (or vice versa) is detected by `TestDescriptorMetadataWellFormed`; duplicate
registration is detected by `TestDescriptorNoDuplicateRoutes`.

### 2. RBAC inventory is a fixed read-only contract

Four new bounded projections (`Role`, `ClusterRole`, `RoleBinding`,
`ClusterRoleBinding`) are added to `backend/internal/kubernetes/service.go`. They expose
only metadata, `rules` (PolicyRule) and `subjects`/`roleRef`; aggregation rules and any
write verb are explicitly omitted. The service methods (`Roles`, `Role`, `ClusterRoles`,
`ClusterRole`, `RoleBindings`, `RoleBinding`, `ClusterRoleBindings`,
`ClusterRoleBinding`) read through the existing `Gateway.Get` and the typed
`client-go`-backed `ClusterClientProvider` introduced in M33. No RBAC mutation is
exposed at any layer.

The `manifestAllowlist` and `manifestPath` are extended so the existing redacted-manifest
endpoint also covers `RoleBinding` and `ClusterRoleBinding`. The managed-cluster observer
`ClusterRole` (`deploy/managed-cluster/observer.yaml`) gains `get,list` on
`roles/clusterroles/rolebindings/clusterrolebindings`.

Eight new GET routes are registered through descriptors and documented in
`docs/api/openapi.yaml` under the `rbac` tag. `TestRegisteredRoutesMatchOpenAPI` enforces
bidirectional parity.

### 3. Descriptor metadata is verified by table-driven tests

`backend/internal/httpserver/route_descriptor_test.go` adds five invariants over the
populated `routeTable`:

- `TestRouteTableCoversAllGinRoutes` — every Gin-registered `/api/v1` route has a
  descriptor (the `/metrics` system route is excluded);
- `TestDescriptorMetadataWellFormed` — audited routes have both action and resource,
  role-restricted routes use only the closed set of platform roles, audit actions match
  the dotted lowercase convention;
- `TestDescriptorHTTPMethodsValid` — no `Get`/`get` typos that would silently create dead
  routes;
- `TestDescriptorFullPathStartsWithAPIV1` — no descriptor escapes the versioned prefix;
- `TestDescriptorNoDuplicateRoutes` — no method+path shadowing.

### 4. Null slices normalize to empty arrays

RBAC `rules` and `subjects` slices are normalized to `[]` (not `null`) on both list and
detail responses, matching the platform-wide "empty collections serialize as arrays"
invariant from `docs/kubesphere-optimization-plan.md` §6.2.

## Alternatives considered

- **Keep the audit map and add a separate OpenAPI parity check.** Rejected: the map is
  the source of the drift. A parity check would catch drift after the fact but still
  require three hand-synced sources.
- **Generate the audit map from OpenAPI.** Rejected: OpenAPI is the public contract, but
  role checks and auth requirements are runtime concerns that should not be derived from
  a YAML document. The descriptor is the runtime source; OpenAPI parity is enforced by
  test.
- **Implement RBAC inventory through a generic GVR read.** Rejected: ADR 0004 and the
  project non-goals explicitly forbid arbitrary GVR/GVK CRUD. A bounded, code-owned
  projection per RBAC kind is the only path that keeps the writable surface explicit.

## Consequences

- Adding a new `/api/v1` route requires exactly one `reg.register` call with a
  `RouteDescriptor`. The route, role check, audit action and audit resource are filled in
  the same call. The OpenAPI document must be updated in the same change or
  `TestRegisteredRoutesMatchOpenAPI` fails.
- The audit middleware no longer has its own classification table. A route that is not
  registered through the registrar is silently unaudited; `TestRouteTableCoversAllGinRoutes`
  guards against this.
- RBAC reads are now part of the platform's bounded read-only surface. The observer
  ClusterRole must grant `get,list` on the four RBAC resources, or the new endpoints
  return 403 on managed clusters.
- No RBAC mutation is possible through the platform. Cluster administrators who need to
  change RBAC must use `kubectl` or their cluster's native UI. This is intentional and
  aligns with the project non-goals.
- Real-kind E2E for the RBAC endpoints is deferred until the multi-worker kind cluster
  used for M30/M31 acceptance is available; the contract is verified by unit tests and
  the descriptor parity suite.
