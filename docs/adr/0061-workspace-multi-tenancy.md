# ADR 0061: Workspace Multi-Tenancy (M46)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M46
- Supersedes: none
- Related: ADR 0050 (lightweight cluster and namespace access grants),
  ADR 0004 (bounded read-only Kubernetes gateway), ADR 0008 (sanitized
  append-only audit trail), ADR 0049 (route descriptor contract and RBAC
  inventory)

## Context

Through M45 the platform had two orthogonal authorization dimensions:

1. **Action dimension** — four fixed platform roles (`system_admin`,
   `operations_admin`, `security_auditor`, `viewer`) that gate which API
   actions a user may perform.
2. **Resource-scope dimension** — `user_cluster_grants` and
   `user_namespace_grants` (ADR 0050) that gate which clusters and
   namespaces a user may read.

The optimization plan (`docs/post-m45-development-roadmap.md` §M46) records
the gap: operators need an **aggregation dimension** that groups cluster
namespaces across the fleet for UI grouping, quota display and cross-cluster
namespace attribution — the KubeSphere Workspace concept — without
introducing dynamic RBAC or CRDs.

KubeSphere solves this with a Workspace CRD that carries its own three-role
model (`workspace_admin` / `workspace_editor` / `workspace_viewer`). The
platform's hard constraints (ADR 0004, project README, project_memory) forbid
arbitrary GVR/GVK CRUD and forbid mirroring Kubernetes RBAC into the
platform's database. M46 introduces a lightweight, platform-owned workspace
layer implemented as SQL tables that composes with the existing 2D
authorization matrix instead of replacing it.

## Decision

### 1. Five SQL tables, no CRD

Create five tables in migration `000034_workspaces_and_grants`:

- `workspaces` — the workspace entity (name, display_name, owner_user_id,
  metadata_json). Name follows DNS-subdomain policy (CHECK constraint).
- `workspace_memberships` — binds a workspace to one `(cluster_id,
  namespace)` tuple. A `(cluster_id, namespace)` pair may belong to at most
  one workspace (unique constraint).
- `workspace_quotas` — one row per workspace. Soft quota display only
  (CPU, memory, pod count, namespace count). The platform does NOT enforce
  it against cluster ResourceQuota.
- `user_workspace_grants` — binds a user to a workspace with one of the
  three fixed workspace roles.
- `workspace_role_bindings_audit` — append-only audit trail of role-binding
  changes (granted / revoked / changed).

### 2. Three fixed workspace roles, independent of platform roles

The workspace layer carries its own three-role model:

| Role               | Workspace metadata | Membership | Quota | Role bindings |
|--------------------|:------------------:|:----------:|:-----:|:-------------:|
| `workspace_admin`  | read + write       | read + write | read + write | read + write |
| `workspace_editor` | read               | read       | read  | read          |
| `workspace_viewer` | read               | read       | read  | read          |

Custom workspace roles are explicitly deferred. The `role` column has a
CHECK constraint that accepts only the three fixed values.

### 3. WorkspaceGrant is orthogonal, not a namespace read grant

The defining invariant: **WorkspaceGrant does not grant namespace read
access.** A `workspace_admin` who has no `user_cluster_grant` or
`user_namespace_grant` for a workspace's member namespaces still cannot read
those namespaces through the bounded Kubernetes gateway. The 2D
authorization matrix from ADR 0050 is unchanged; WorkspaceGrant is a third,
orthogonal grant type that only authorizes workspace metadata / membership /
quota / role-binding edits.

This is the deliberate difference from KubeSphere's CRD-based model, where
`workspace_admin` implies namespace access. The platform's anti-leakage
invariant (404 > 403, ADR 0050 §6) is preserved: an unauthorized workspace is
indistinguishable from a missing one.

### 4. SystemAdmin bypass

`system_admin` bypasses all workspace grant checks, mirroring ADR 0050 §3.
This is implemented in `workspace.Service.authorizeRole` via
`authz.IsSystemAdmin(roles)`.

### 5. Anti-leakage (404 > 403)

Every workspace-scoped operation that fails authorization returns
`ErrWorkspaceNotFound` (mapped to HTTP 404), never `ErrAccessDenied` (which
would map to 403). This prevents an attacker from distinguishing "workspace
exists but I am not a member" from "workspace does not exist". The handler
`writeWorkspaceError` maps both `ErrWorkspaceNotFound` and `ErrAccessDenied`
to 404.

### 6. Owner is always workspace_admin

The workspace `owner_user_id` is the user who created the workspace. On
creation the service atomically seeds an `workspace_admin` grant for the
owner and records an audit entry. The owner grant:

- Cannot be revoked while the workspace exists (`RevokeRole` rejects
  `targetUserID == ws.OwnerUserID`).
- Cannot be downgraded (`GrantRole` rejects `in.Role != RoleAdmin` when
  `in.UserID == ws.OwnerUserID`).

This guarantees there is always at least one workspace_admin per workspace.

### 7. Workspace creation and deletion are SystemAdmin-only

Workspace self-service creation is intentionally deferred. `CreateWorkspace`
and `DeleteWorkspace` require `system_admin`; non-admins receive
`ErrWorkspaceNotFound` (404 anti-leakage, not 403). This prevents accidental
loss of cross-cluster attribution and keeps the workspace namespace clean.

### 8. Append-only audit trail

Every role-binding change (grant / revoke / change) appends a row to
`workspace_role_bindings_audit`. The table is append-only; there is no
UPDATE or DELETE path. The `action` column has a CHECK constraint that
accepts only `granted`, `revoked`, `changed`.

### 9. Soft quota is display-only

`workspace_quotas` mirrors KubeSphere Workspace ResourceQuota but is
display-only. The platform does NOT enforce it against cluster
ResourceQuota. This avoids introducing a new enforcement path that would
compose poorly with the existing Kubernetes ResourceQuota model. All Hard*
fields are nullable so a workspace can omit fields it does not track.

## Consequences

### Positive

- Operators get a cross-cluster aggregation dimension without a new CRD or
  dynamic RBAC.
- The 2D authorization matrix is unchanged; existing ClusterGrant /
  NamespaceGrant semantics are preserved.
- Anti-leakage is consistent with ADR 0050.
- The audit trail is append-only, consistent with ADR 0008.
- The SQL implementation avoids the operational overhead of a CRD
  controller.

### Negative

- WorkspaceGrant does not grant namespace read access, which is a departure
  from KubeSphere's mental model. Operators who expect `workspace_admin` to
  imply namespace access must also be granted `user_cluster_grant` or
  `user_namespace_grant`. This is documented in the API and the handler
  comments.
- Workspace quota is display-only; teams that need enforcement must rely on
  Kubernetes ResourceQuota directly.
- Workspace creation is SystemAdmin-only, which adds a manual step for new
  workspaces. This is a deliberate trade-off for namespace hygiene.

### Neutral

- The workspace layer adds 5 tables and 14 routes. The OpenAPI route
  contract test (`TestRegisteredRoutesMatchOpenAPI`) covers all 14 routes.

## Implementation

- Migration: `backend/migrations/000034_workspaces_and_grants.up.sql` /
  `.down.sql`.
- Package: `backend/internal/workspace/` — `model.go`, `repository.go`,
  `service.go`, `service_test.go`.
- HTTP handler: `backend/internal/httpserver/workspace.go` +
  `workspace_test.go`.
- Routes: `backend/internal/httpserver/router.go` (14 routes under
  `/api/v1/workspaces`).
- OpenAPI: `docs/api/openapi.yaml` (14 paths + 11 schemas).
- Wiring: `backend/cmd/server/main.go` creates the workspace service and
  injects it into `httpserver.Options`.
