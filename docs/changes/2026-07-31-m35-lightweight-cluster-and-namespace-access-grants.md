# M35: Lightweight Cluster and Namespace Access Grants

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0050](../adr/0050-lightweight-cluster-and-namespace-access-grants.md)
- Baseline: `baseline-m32-20260731` (M33/M34 layered on top; no new baseline tag cut)
- Fast gate: 39.44s (backend vet+test, 73 frontend tests, Compose/Kustomize contracts)

## Summary

Introduced the platform's first *resource-scope* authorization dimension on top
of the four global platform roles (the *action* dimension) that ADR 0004/ADR
0039 already govern. `docs/kubesphere-optimization-plan.md` §3.4 ("Multi-team
authorization") had recorded the gap: any authenticated non-admin user could
read every registered cluster's resources. M35 closes that gap with two narrow
grant tables, a single policy evaluator, deny-by-default middleware that
returns 404 (not 403) to avoid leaking hidden clusters, and fleet/global-search
fan-out that silently omits unauthorized clusters.

No public API contract was broken. No new writable Kubernetes surface was
introduced. SystemAdmin behavior is unchanged.

## Changes

### M35.1 — Database migration

#### New files

- `backend/migrations/000025_access_grants.up.sql`: creates
  `user_cluster_grants(id, user_id, cluster_id, created_at)` with a unique
  constraint on `(user_id, cluster_id)` and indexes on `user_id` and
  `cluster_id`; creates `user_namespace_grants(id, user_id, cluster_id,
  namespace, created_at)` with a unique constraint on `(user_id, cluster_id,
  namespace)` and an index on `(user_id, cluster_id)`. Both tables cascade on
  user and cluster deletion.
- `backend/migrations/000025_access_grants.down.sql`: drops both tables.

### M35.2 — Authorization domain model and policy evaluator

#### New files

- `backend/internal/authz/model.go`: defines `ClusterGrant`,
  `NamespaceGrant`, `ClusterScope` and `AccessDecision`. `ClusterGrant` and
  `NamespaceGrant` carry GORM tags that mirror the migration; `ClusterScope`
  summarizes a user's access to one cluster (`AllNamespaces` or an explicit
  `NamespaceGrants` slice); `AccessDecision` carries `Allowed` and a stable
  machine-readable `Reason` for denied decisions.
- `backend/internal/authz/repository.go`: defines the `Repository` interface
  and the GORM implementation. `ClusterScope` returns `AllNamespaces=true` when
  a cluster grant exists; otherwise it returns the exact authorized namespaces.
  `VisibleClusters` returns the distinct union of cluster IDs from both grant
  tables, sorted ascending. `isUniqueViolation` maps GORM's
  `ErrDuplicatedKey` to `ErrGrantAlreadyExists`.
- `backend/internal/authz/service.go`: defines `Service` (the policy evaluator)
  and `GrantManager` (SystemAdmin-only CRUD). `IsSystemAdmin` short-circuits
  every evaluation. `CanAccessCluster`, `CanAccessNamespace`, `VisibleClusters`
  and `ClusterScope` are the four decision methods. `GrantManager` is a thin
  wrapper over the repository; the HTTP layer enforces the `system_admin` role.

### M35.3 — Authorization middleware

#### New files

- `backend/internal/httpserver/authz_middleware.go`: defines
  `requireClusterAccess`, `requireNamespaceAccess` and
  `authorizedClusterFilter`. Both middlewares return 404 on denial so that the
  existence of an unauthorized cluster or namespace cannot be distinguished
  from a genuinely missing one. `requireNamespaceAccess` is a no-op when the
  `:namespace` path parameter is absent (list routes that use `?namespace=`
  are not enforced by the middleware). `authorizedClusterFilter` returns
  `nil` (meaning "all enabled clusters") for SystemAdmin and the explicit
  allowlist for other roles.

#### Modified files

- `backend/internal/httpserver/router.go`: the `resourceRoutes` group now
  chains `requireClusterAccess(options.Authz)` and
  `requireNamespaceAccess(options.Authz, "namespace")` after
  `withClusterContext()`. `fleetHandler` and `globalSearchHandler` gain an
  `authz *authz.Service` field and are constructed with `options.Authz`.

### M35.4 — Fleet/Search authorized cluster filtering

#### Modified files

- `backend/internal/fleet/service.go`: `Service.Compare` now accepts a
  `visibleClusters []int64` parameter. A `nil` slice means "all enabled
  clusters" (SystemAdmin path); a non-nil slice is the explicit allowlist.
  The filter is applied after the `Enabled` check and before the limit.
- `backend/internal/globalsearch/service.go`: `Service.Search` now accepts the
  same `visibleClusters []int64` parameter with the same semantics. The filter
  is applied before the `ClusterLimit` truncation so that
  `ClustersRemaining` reflects only authorized clusters.
- `backend/internal/httpserver/fleet.go`: `fleetHandler.health` calls
  `authorizedClusterFilter(h.authz, c)` and passes the result to
  `service.Compare`. A scope-evaluation failure returns 500 with the existing
  `FLEET_QUERY_FAILED` code.
- `backend/internal/httpserver/global_search.go`: `globalSearchHandler.search`
  calls `authorizedClusterFilter(h.authz, c)` and passes the result to
  `service.Search`. A scope-evaluation failure returns 500 with the existing
  `GLOBAL_SEARCH_FAILED` code.
- `backend/internal/fleet/service_test.go`,
  `backend/internal/globalsearch/service_test.go`: existing tests pass `nil`
  for the new parameter to preserve the SystemAdmin semantics.

### M35.5 — Grant management API and audit

#### New files

- `backend/internal/httpserver/grants.go`: defines `grantHandler` with
  `listClusterGrants`, `createClusterGrant`, `deleteClusterGrant`,
  `listNamespaceGrants`, `createNamespaceGrant`, `deleteNamespaceGrant` and
  `myGrants`. Mutating routes call `setAuditTarget` and `setAuditClusterID`
  so the audit trail records the resource and scope. `writeGrantError` maps
  `ErrGrantAlreadyExists` to 409, `ErrGrantNotFound` to 404, and any other
  error to 500.

#### Modified files

- `backend/internal/httpserver/router.go`: seven new routes registered through
  `RouteDescriptor` under `rolesSystemAdmin` (except `me/grants`):
  - `GET    /users/:user_id/cluster-grants`
  - `POST   /users/:user_id/cluster-grants`
  - `DELETE /users/:user_id/cluster-grants/:cluster_id`
  - `GET    /users/:user_id/namespace-grants`
  - `POST   /users/:user_id/namespace-grants`
  - `DELETE /users/:user_id/namespace-grants/:cluster_id/:namespace`
  - `GET    /auth/me/grants`
  Every mutating route carries `AuditAction` and `AuditResource` so the audit
  middleware produces one record per call.
- `backend/cmd/server/main.go`: wires `authz.NewService` and
  `authz.NewGrantManager` into `httpserver.Options`.
- `docs/api/openapi.yaml`: adds the `access-grants` tag, seven new paths, and
  two new schemas (`ClusterGrantCreate`, `NamespaceGrantCreate`).
  `TestRegisteredRoutesMatchOpenAPI` enforces bidirectional parity.

### M35.6 — Unit tests

#### New files

- `backend/internal/authz/service_test.go`: 21 tests over `Service` and
  `GrantManager` using an in-memory `fakeRepository`. Covers SystemAdmin
  bypass, cluster-grant allow, namespace-grant allow, denial reasons
  (`cluster_not_authorized`, `namespace_not_authorized`), repo-error
  propagation, `VisibleClusters` delegation, and `GrantManager` CRUD with
  duplicate/not-found error mapping.
- `backend/internal/httpserver/authz_middleware_test.go`: 11 tests over
  `requireClusterAccess`, `requireNamespaceAccess` and
  `authorizedClusterFilter`. Covers SystemAdmin bypass, cluster-grant allow,
  namespace-grant allow, denial returning 404, nil-service passthrough,
  namespace-param-absent skip, and the three `authorizedClusterFilter` return
  shapes (nil for SystemAdmin, explicit set for viewer, nil for nil service).
- `backend/internal/httpserver/grants_test.go`: 13 tests over `grantHandler`
  using an in-memory `grantsTestRepo`. Covers list/create/delete for both
  grant kinds, duplicate 409, missing 404, invalid user_id 400, invalid body
  400, `myGrants` for the current user, and `writeGrantError` error mapping.

#### Modified files

- `backend/internal/httpserver/openapi_route_test.go`: the parity test now
  wires `Authz` and `GrantManager` so the seven access-grants routes are
  exercised. Adds a no-op `openAPITestAuthzRepo` so the test does not require
  a database.

## Preserved invariants

- ADR 0004 bounded read-only gateway: no new writable resource kind; the grant
  tables are platform-owned metadata, not Kubernetes resources.
- ADR 0008 sanitized audit trail: every mutating grant route produces one
  audit record with action+resource; denial reasons are stable codes that do
  not leak hidden cluster or resource names.
- ADR 0049 route descriptor contract: every new route is registered through
  `reg.register` with a `RouteDescriptor`; the OpenAPI parity test enforces
  bidirectional coverage.
- Project non-goals: no arbitrary GVR/GVK CRUD; no Kubernetes RBAC mirroring;
  no informer/watch cache; no change to any existing response schema.
- M33 transport layer: `ClusterClientProvider` is the only Kubernetes
  transport; M35 does not touch `client-go` assembly.
- M34 audit classification: the audit middleware still derives metadata from
  `routeTable`; M35 only adds new descriptors, it does not change the
  classification mechanism.

## Verification

- `go build ./...`: pass
- `go vet ./internal/authz ./internal/httpserver ./internal/fleet ./internal/globalsearch`: pass
- `go test ./...`: all packages pass
  - `internal/authz`: 21 new tests
  - `internal/httpserver`: 11 new middleware tests, 13 new grants tests,
    OpenAPI parity test updated to exercise the seven access-grants routes
  - `internal/fleet`, `internal/globalsearch`: existing tests updated for the
    new `visibleClusters` parameter
- `TestRegisteredRoutesMatchOpenAPI`: the seven new access-grants routes are
  present in both `routeTable` and `openapi.yaml`; no orphan route in either
  direction.
- Fast gate: `scripts/verify-fast.ps1 -Scope All` (see run output for timing)

## Non-goals

- No Kubernetes RBAC mirroring. The grant tables are platform-owned metadata
  that compose with the four global roles; they do not sync from
  RoleBindings/ClusterRoleBindings.
- No frontend view for grant management in M35. The contract is exposed via
  OpenAPI; a dedicated admin UI is deferred to a later milestone if a real
  operator need appears.
- No enforcement of `?namespace=` query-parameter routes in M35. Only
  path-parameter `:namespace` routes are enforced by the middleware; list
  routes that accept `?namespace=` are filtered at the service level when
  needed (fleet/global-search are wired; other list routes will be wired in a
  follow-up if a real operator need appears).
- No real-kind E2E for the grants endpoints in M35. The contract is verified
  by the unit tests and the descriptor/OpenAPI parity suite. Real-kind E2E is
  deferred until the multi-worker kind cluster used for M30/M31 acceptance is
  available.
- No change to any existing audit action, role check, or response schema.
  M35 is an authorization milestone, not a contract-change milestone.

## Real-kind E2E

Deferred. The seven new grants endpoints exercise the same `Repository`
interface and `RouteDescriptor` registration used by every other audited
route; a real-kind regression run on the multi-worker kind cluster will
validate them end-to-end. The local fast gate, descriptor parity suite,
OpenAPI parity test, and the unit tests over the in-memory repository verify
the contract-level correctness.

## Post-M35 state

- A non-SystemAdmin user now needs an explicit grant to read any cluster's
  resources. Existing non-admin users will see zero clusters until a
  SystemAdmin grants them access. This is the intended behavior; the bootstrap
  administrator is always SystemAdmin and is unaffected.
- `authz.Service` is the single policy evaluator. `requireClusterAccess` and
  `requireNamespaceAccess` are the single enforcement points for path-scoped
  routes. `authorizedClusterFilter` is the single fan-out scope helper.
- Fleet and global search silently omit unauthorized clusters from their
  results; a denied direct access returns 404, not 403.
- M36/M37 (as listed in `docs/kubesphere-optimization-plan.md`) may now
  proceed on top of the grant-backed authorization layer.
