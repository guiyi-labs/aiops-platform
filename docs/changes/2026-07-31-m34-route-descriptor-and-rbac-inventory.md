# M34: Route Descriptor Contract and RBAC Inventory

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0049](../adr/0049-route-descriptor-contract-and-rbac-inventory.md)
- Baseline: `baseline-m32-20260731` (M33 layered on top; no new baseline tag cut)
- Fast gate: 26.64s (3 backend packages vet+test, 73 frontend tests, Compose/Kustomize contracts)

## Summary

Closed two documentation-truth and read-only-contract debts that
`docs/kubesphere-optimization-plan.md` §4.2/§4.3 had recorded against
`baseline-m32-20260731`:

1. **M34A — Route Descriptor Contract.** Replaced the scattered Gin route
   registration, hardcoded audit map, and inline role checks with a single
   `RouteDescriptor` source of truth. The same descriptor now drives route
   registration, authentication/role enforcement, and audit classification.
2. **M34B — RBAC Inventory.** Delivered the ADR 0039-promised fixed read-only
   contract for `Role`, `ClusterRole`, `RoleBinding`, `ClusterRoleBinding`,
   including bounded projections, manifest redaction, observer RBAC, OpenAPI
   documentation and descriptor-driven routes.

No public API contract was broken. No new writable Kubernetes surface was
introduced. M34 is a contract-debt closure milestone, not a feature milestone.

## Changes

### M34A — Route Descriptor Contract

#### New files

- `backend/internal/httpserver/routes.go`: defines `RouteDescriptor`,
  `registeredRoute`, `routeTable`, `routeRegistrar`, `newRouteRegistrar`,
  `register`, and the helpers `normalizeFullPath`, `resetRouteTable`,
  `findAuditedRoute`. The descriptor is the single source of truth for a
  route's method, path, handler, `AuthRequired`, `RequiredRoles`,
  `AuditAction` and `AuditResource`.
- `backend/internal/httpserver/route_descriptor_test.go`: five table-driven
  invariants over the populated `routeTable`:
  - `TestRouteTableCoversAllGinRoutes` — every Gin-registered `/api/v1`
    route has a descriptor (the `/metrics` system route is excluded);
  - `TestDescriptorMetadataWellFormed` — audited routes have both action and
    resource, role-restricted routes use only the closed set of platform
    roles, audit actions match the dotted lowercase convention;
  - `TestDescriptorHTTPMethodsValid` — no `Get`/`get` typos that would
    silently create dead routes;
  - `TestDescriptorFullPathStartsWithAPIV1` — no descriptor escapes the
    versioned prefix;
  - `TestDescriptorNoDuplicateRoutes` — no method+path shadowing.

#### Modified files

- `backend/internal/httpserver/router.go`: removed the `withAuth` helper;
  introduced `routeRegistrar` via `newRouteRegistrar(options.Auth)`; every
  `/api/v1` route is now registered through `reg.register(group, descriptor)`.
  Group-level authentication middleware is preserved on the resource group;
  descriptor-level `AuthRequired` is only set for routes on groups without
  group-level auth.
- `backend/internal/httpserver/audit.go`: deleted the hardcoded `operations`
  map. `auditedOperation` is retained as a thin shim over
  `findAuditedRoute(method, path)` so existing call sites and tests continue
  to work. `auditTrail` now looks up audit metadata from `routeTable`.

### M34B — RBAC Inventory

#### Modified files

- `backend/internal/kubernetes/service.go`: added bounded projections
  `PolicyRule`, `Role`, `ClusterRole`, `RoleRef`, `Subject`, `RoleBinding`,
  `ClusterRoleBinding`. Added service methods `Roles`, `Role`,
  `ClusterRoles`, `ClusterRole`, `RoleBindings`, `RoleBinding`,
  `ClusterRoleBindings`, `ClusterRoleBinding`. All methods read through the
  existing `Gateway.Get` (M33 `ClusterClientProvider`). `aggregationRule` is
  intentionally omitted; no RBAC mutation is exposed. `normalizeRBACRules`
  and `normalizeSubjects` normalize nil slices to `[]` so empty collections
  serialize as arrays (project-wide invariant from
  `docs/kubesphere-optimization-plan.md` §6.2). The `manifestAllowlist` map
  and the `manifestPath` switch are extended so the existing redacted-manifest
  endpoint also covers `RoleBinding` and `ClusterRoleBinding`. `Role` and
  `ClusterRole` were already allowlisted under M22.
- `backend/internal/httpserver/kubernetes.go`: added handler methods
  `roles`, `role`, `clusterRoles`, `clusterRole`, `roleBindings`,
  `roleBinding`, `clusterRoleBindings`, `clusterRoleBinding` mirroring the
  existing list/detail handler shape (cluster-scoped kinds do not accept
  `namespace`).
- `backend/internal/httpserver/router.go`: registered eight new GET routes
  through descriptors under the existing `resourceRoutes` group:
  `/roles`, `/roles/:namespace/:name`, `/clusterroles`, `/clusterroles/:name`,
  `/rolebindings`, `/rolebindings/:namespace/:name`,
  `/clusterrolebindings`, `/clusterrolebindings/:name`.
- `deploy/managed-cluster/observer.yaml`: added `get,list` on
  `roles/clusterroles/rolebindings/clusterrolebindings` to the observer
  `ClusterRole`. Without this grant the new endpoints return 403 on managed
  clusters.
- `docs/api/openapi.yaml`: added eight GET paths under the `rbac` tag
  matching the descriptor-registered routes. `rbac` was added to the tag
  list. `TestRegisteredRoutesMatchOpenAPI` enforces bidirectional parity.

#### New files

- `backend/internal/kubernetes/rbac_test.go`: table-driven unit tests for
  `Roles`, `Role`, `ClusterRoles`, `ClusterRole`, `RoleBindings`,
  `RoleBinding`, `ClusterRoleBindings`, `ClusterRoleBinding` using the
  existing `roundTrip` test harness. Covers list pagination, detail 404,
  nil-slice normalization, and path construction for namespaced vs
  cluster-scoped kinds.

## Preserved invariants

- ADR 0004 bounded read-only gateway: no new writable resource kind; RBAC
  reads go through the same `Gateway.Get` used by M22/M33.
- ADR 0008 sanitized audit trail: every audited route still produces one
  audit record with action+resource; the source of the metadata changed
  from a hardcoded map to the descriptor, the audit-record shape did not.
- Project non-goals: no arbitrary GVR/GVK CRUD; no RBAC mutation; no
  informer/watch cache; no change to any public response schema beyond the
  new RBAC kinds.
- M33 transport layer: `ClusterClientProvider` is the only Kubernetes
  transport; M34 does not touch `client-go` assembly.

## Verification

- `go build ./...`: pass
- `go vet ./internal/httpserver ./internal/kubernetes`: pass
- `go test ./internal/httpserver ./internal/kubernetes ./internal/deployment`:
  pass (httpserver includes the new descriptor suite and the OpenAPI parity
  test; kubernetes includes the new RBAC suite)
- `vue-tsc -b`: zero frontend type errors
- `vitest run`: 17 files, 73 tests, all pass
- `scripts/verify-fast.ps1 -Scope All`: 26.64s, all green
  (backend=True frontend=True manifests=True)
- `TestRegisteredRoutesMatchOpenAPI`: the eight new RBAC routes are present
  in both `routeTable` and `openapi.yaml`; no orphan route in either
  direction.

## Non-goals

- No RBAC mutation endpoint. Cluster administrators must use `kubectl` or
  their cluster's native UI to change RBAC. This is intentional and aligns
  with the project non-goals.
- No frontend view for RBAC inventory in M34. The contract is exposed via
  OpenAPI and the existing `ResourceDetailView` (M22) already renders
  role/clusterrole manifests; a dedicated RBAC browser is deferred to a
  later milestone if a real operator need appears.
- No informer/watch cache for RBAC objects. Every read goes through
  `Gateway.Get` like every other read-only resource.
- No real-kind E2E for the RBAC endpoints in M34. The contract is verified
  by the descriptor parity suite, the OpenAPI parity test, and the unit
  tests over the `roundTrip` harness. Real-kind RBAC E2E is deferred until
  the multi-worker kind cluster used for M30/M31 acceptance is available.
- No change to any existing audit action, role check, or response schema.
  M34 is a contract-debt closure milestone, not a feature milestone.

## Real-kind E2E

Deferred. The eight new RBAC endpoints exercise the same `Gateway.Get`
interface used by every other read-only resource; a real-kind regression
run on the multi-worker kind cluster will validate them end-to-end. The
local fast gate, descriptor parity suite, OpenAPI parity test, and the
RBAC unit tests verify the contract-level correctness.

## Post-M34 state

- `routeTable` is the single source of truth for `/api/v1` route metadata.
  Adding a route without going through the registrar is detected by
  `TestRouteTableCoversAllGinRoutes`.
- The audit middleware no longer has its own classification table.
- RBAC inventory is part of the platform's bounded read-only surface. The
  observer ClusterRole must grant `get,list` on the four RBAC resources, or
  the new endpoints return 403 on managed clusters.
- M35/M36/M37 (as listed in `docs/kubesphere-optimization-plan.md`) may now
  proceed on top of the descriptor-driven contract.
