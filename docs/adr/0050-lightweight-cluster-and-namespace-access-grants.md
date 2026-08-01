# ADR 0050: Lightweight Cluster and Namespace Access Grants

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M35, ADR 0004 (bounded read-only Kubernetes gateway), ADR 0008 (sanitized append-only audit trail), ADR 0039 (governance workbench), ADR 0049 (route descriptor contract and RBAC inventory)

## Context

Through M34 the platform had four global platform roles (`system_admin`,
`operations_admin`, `security_auditor`, `viewer`) that governed the *action*
dimension of every request, but no *resource-scope* dimension. Any authenticated
non-admin user could read every registered cluster's resources through the
bounded gateway. `docs/kubesphere-optimization-plan.md` §3.4 ("Multi-team
authorization") records the gap: real operations teams need isolation between
the clusters and namespaces they own, without forcing every operator onto
`system_admin`.

KubeSphere solves this with per-workspace bindings over Kubernetes RBAC. The
platform's non-goals (ADR 0004, project README) forbid arbitrary GVR/GVK CRUD
and forbid mirroring Kubernetes RBAC into the platform's database. M35 is the
milestone that introduces a lightweight, platform-owned grant table that
composes with the existing global roles instead of replacing them.

## Decision

### 1. Two grant tables compose with the four global roles

Two new tables, `user_cluster_grants` and `user_namespace_grants` (migration
`000025_access_grants`), record which clusters and namespaces a user may
access. They are the *resource-scope* dimension; the four global roles remain
the *action* dimension. A user needs both a role that permits the action *and*
a grant that permits the scope.

- `user_cluster_grants(user_id, cluster_id)` authorizes access to every
  namespace in a cluster.
- `user_namespace_grants(user_id, cluster_id, namespace)` authorizes access to
  one exact namespace in a cluster. If the user also holds a cluster grant for
  the same cluster, the namespace grant is redundant but not harmful.

Foreign keys cascade on user and cluster deletion. Unique constraints make
duplicate grants a 409 instead of a silent overwrite.

### 2. SystemAdmin bypasses grants entirely

`authz.IsSystemAdmin(roles)` short-circuits every policy evaluation. A
SystemAdmin does not need any grant; every other role requires an explicit
grant. This preserves the existing operational guarantee that the bootstrap
administrator can always reach every cluster.

### 3. A single policy evaluator is the only authorization point

`authz.Service` is the single policy evaluator used by HTTP middleware, fleet
fan-out and global search. Three methods cover the three decision shapes:

- `CanAccessCluster(userID, roles, clusterID)` — cluster-scoped routes and the
  fleet fan-out filter.
- `CanAccessNamespace(userID, roles, clusterID, namespace)` — namespace-scoped
  detail routes.
- `VisibleClusters(userID, roles)` — fleet/global-search enumeration. Returns
  `nil` for SystemAdmin (meaning "all enabled clusters"); for other roles it
  returns the distinct set of cluster IDs from both grant tables.

`AccessDecision` carries `Allowed` and a stable machine-readable `Reason` for
denied decisions. The reason is suitable for audit logging and must not leak
hidden cluster or resource names.

### 4. Denial returns 404, not 403

The middleware (`requireClusterAccess`, `requireNamespaceAccess`) and the
`authorizedClusterFilter` helper all map a denial to HTTP 404 (or silent
omission from a list). This satisfies the M35 acceptance standard: "An
unauthorized target is absent from lists/fan-out and cannot be distinguished
through direct IDs or error details." Returning 403 would leak the existence of
a cluster or namespace the user is not entitled to see.

### 5. Grant management is SystemAdmin-only

Seven new routes under `/api/v1/users/:user_id/cluster-grants`,
`/api/v1/users/:user_id/namespace-grants` and `/api/v1/auth/me/grants` are
registered through `RouteDescriptor` (ADR 0049). The create/delete/list routes
for another user's grants require `system_admin`; the `me/grants` route is
available to any authenticated user and returns only the caller's own grants.
Every mutating route is audited through the existing audit middleware.

### 6. Fleet and global search scope their fan-out

`fleet.Service.Compare` and `globalsearch.Service.Search` now accept a
`visibleClusters []int64` parameter. A `nil` slice means "all enabled
clusters" (SystemAdmin); a non-nil slice is the explicit allowlist from
`authz.VisibleClusters`. The HTTP handlers call `authorizedClusterFilter` once
per request and pass the result down. This keeps the policy evaluator the
single decision point and the services free of any direct authz dependency.

## Alternatives considered

- **Mirror Kubernetes RBAC into the platform database.** Rejected: ADR 0004 and
  the project non-goals forbid arbitrary GVR/GVK CRUD. Mirroring RoleBindings
  would duplicate the source of truth, drift from the cluster, and require a
  reconciliation controller that the platform does not need.
- **Per-route role checks instead of a policy evaluator.** Rejected: ADR 0049
  already centralizes route metadata in `RouteDescriptor`; scattering scope
  checks across handlers would recreate the drift problem M34 just closed. The
  policy evaluator is the single decision point; the middleware is the single
  enforcement point.
- **Return 403 on denial with a generic error.** Rejected: 403 leaks the
  existence of a hidden cluster or namespace. The M35 acceptance standard
  requires that an unauthorized target be indistinguishable from a genuinely
  missing one, so denial returns 404 and fan-out silently omits.
- **Store grants as a single polymorphic table.** Rejected: a single
  `(user_id, cluster_id, namespace nullable)` table would make the unique
  constraint ambiguous (is `NULL` distinct?) and complicate the
  `AllNamespaces` shortcut. Two narrow tables keep the two grant shapes
  explicit and the SQL trivial.

## Consequences

- A non-SystemAdmin user now needs an explicit grant to read any cluster's
  resources. Existing non-admin users will see zero clusters until a
  SystemAdmin grants them access. This is the intended behavior; the bootstrap
  administrator is always SystemAdmin and is unaffected.
- Adding a new cluster-scoped or namespace-scoped route requires no authz
  change: registering it under `resourceRoutes` (which already chains
  `requireClusterAccess` and `requireNamespaceAccess`) is sufficient. Routes
  that use `?namespace=` query parameters instead of `:namespace` path
  parameters are not enforced by the middleware; they are filtered at the
  service level when needed (M35 only wires fleet/global-search for now).
- The fleet and global search services now have a `visibleClusters` parameter.
  Callers that pass `nil` get the old behavior (all enabled clusters); this is
  the SystemAdmin path. Existing tests pass `nil` to preserve their semantics.
- Real-kind E2E for the grants endpoints is deferred until the multi-worker
  kind cluster used for M30/M31 acceptance is available; the contract is
  verified by unit tests and the descriptor/OpenAPI parity suite.
