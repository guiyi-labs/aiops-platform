# M46: Workspace Multi-Tenancy

- Date: 2026-07-31
- Status: Development Complete (local development deliverables)
- ADR: [0061](../adr/0061-workspace-multi-tenancy.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 63.48s; backend=True frontend=True manifests=True)

## Summary

Introduced the lightweight KubeSphere-style Workspace multi-tenancy layer as
the M46 development-side deliverable. A Workspace is an aggregation dimension
that groups cluster namespaces across the fleet for UI grouping, quota display
and cross-cluster namespace attribution. It carries its own three-role model
(`workspace_admin` / `workspace_editor` / `workspace_viewer`) that is
independent of the four platform roles.

The defining invariant: **WorkspaceGrant does not grant namespace read access.**
A `workspace_admin` who has no `user_cluster_grant` or `user_namespace_grant`
for a workspace's member namespaces still cannot read those namespaces through
the bounded Kubernetes gateway. The 2D authorization matrix from M35 (ADR 0050)
is unchanged; WorkspaceGrant is a third, orthogonal grant type that only
authorizes workspace metadata / membership / quota / role-binding edits.

Anti-leakage (404 > 403) is preserved: an unauthorized workspace is
indistinguishable from a missing one. SystemAdmin bypasses all workspace grant
checks. Workspace creation and deletion are SystemAdmin-only. The owner is
always `workspace_admin` and cannot be downgraded or revoked while the
workspace exists. The audit trail is append-only.

M46 production gates (hosted CI, production OIDC/MFA, HA PostgreSQL, signed
releases, real-kind E2E) remain external and are not closed by this development
deliverable.

## Changes

### New files

- `backend/migrations/000034_workspaces_and_grants.up.sql` — 5 tables
  (`workspaces`, `workspace_memberships`, `workspace_quotas`,
  `user_workspace_grants`, `workspace_role_bindings_audit`) with CHECK
  constraints for role/action/name/namespace validation and unique constraints
  for workspace name and (cluster_id, namespace) membership.
- `backend/migrations/000034_workspaces_and_grants.down.sql` — reverse
  migration.
- `backend/internal/workspace/model.go` — data models, three fixed role
  constants, audit action constants, sentinel errors, `RoleRank` helper.
- `backend/internal/workspace/repository.go` — `Repository` interface +
  `GormRepository` implementation for all CRUD operations.
- `backend/internal/workspace/service.go` — application service enforcing all
  authorization invariants (SystemAdmin bypass, 404 > 403 anti-leakage, owner
  role fixed, append-only audit, role validation, quota validation).
- `backend/internal/workspace/service_test.go` — 29 service-level unit tests
  covering create/get/list/update/delete, membership, quota, role bindings,
  anti-leakage, role hierarchy, owner protection, and metadata normalization.
- `backend/internal/httpserver/workspace.go` — HTTP handler with 14 endpoints
  and stable error-to-HTTP mapping.
- `backend/internal/httpserver/workspace_test.go` — 10 handler-level unit
  tests covering create/get/membership/quota/role-bindings/audit and
  anti-leakage at the HTTP layer.
- `docs/adr/0061-workspace-multi-tenancy.md` — ADR documenting the design
  invariants, role model, anti-leakage, owner protection and consequences.

### Modified files

- `backend/internal/httpserver/router.go` — added `WorkspaceService` to
  `Options`, imported `workspace` package, registered 14 routes under
  `/api/v1/workspaces`.
- `backend/internal/httpserver/openapi_route_test.go` — wired
  `WorkspaceService` so the route contract test covers workspace routes.
- `backend/cmd/server/main.go` — created `workspaceService` and injected it
  into `httpserver.Options`.
- `docs/api/openapi.yaml` — added 14 workspace paths and 11 workspace schemas
  (Workspace, WorkspaceList, CreateWorkspaceRequest, UpdateWorkspaceRequest,
  WorkspaceMembership, WorkspaceMembershipList, AddMembershipRequest,
  WorkspaceQuota, SetQuotaRequest, UserWorkspaceGrant, UserWorkspaceGrantList,
  GrantRoleRequest, WorkspaceRoleBindingAudit, WorkspaceRoleBindingAuditList).

## Routes

| Method | Path | Audit action | Required role |
|--------|------|--------------|---------------|
| GET | /api/v1/workspaces | workspaces.list | any authenticated |
| POST | /api/v1/workspaces | workspaces.create | system_admin |
| GET | /api/v1/workspaces/{workspace_id} | workspaces.read | workspace_viewer+ |
| PATCH | /api/v1/workspaces/{workspace_id} | workspaces.update | workspace_admin |
| DELETE | /api/v1/workspaces/{workspace_id} | workspaces.delete | system_admin |
| GET | /api/v1/workspaces/{workspace_id}/memberships | workspaces.memberships.list | workspace_viewer+ |
| POST | /api/v1/workspaces/{workspace_id}/memberships | workspaces.memberships.add | workspace_admin |
| DELETE | /api/v1/workspaces/{workspace_id}/memberships | workspaces.memberships.remove | workspace_admin |
| GET | /api/v1/workspaces/{workspace_id}/quota | workspaces.quota.read | workspace_viewer+ |
| PUT | /api/v1/workspaces/{workspace_id}/quota | workspaces.quota.set | workspace_admin |
| GET | /api/v1/workspaces/{workspace_id}/role-bindings | workspaces.role_bindings.list | workspace_viewer+ |
| POST | /api/v1/workspaces/{workspace_id}/role-bindings | workspaces.role_bindings.grant | workspace_admin |
| DELETE | /api/v1/workspaces/{workspace_id}/role-bindings/{user_id} | workspaces.role_bindings.revoke | workspace_admin |
| GET | /api/v1/workspaces/{workspace_id}/role-bindings/audit | workspaces.role_bindings.audit.list | workspace_viewer+ |

## Invariants enforced

1. **SystemAdmin bypass** — `system_admin` bypasses all workspace grant checks.
2. **404 > 403 anti-leakage** — unauthorized workspace access returns
   `ErrWorkspaceNotFound` (HTTP 404), never 403.
3. **Three fixed roles** — only `workspace_admin`, `workspace_editor`,
   `workspace_viewer` are accepted (DB CHECK + service validation).
4. **Owner role fixed** — the owner is always `workspace_admin`; the grant
   cannot be revoked or downgraded while the workspace exists.
5. **WorkspaceGrant is orthogonal** — it does not grant namespace read access;
   the 2D authorization matrix from M35 is unchanged.
6. **Append-only audit** — every role-binding change appends a row; no UPDATE
   or DELETE path.
7. **Soft quota display-only** — the platform does NOT enforce workspace quota
   against cluster ResourceQuota.
8. **SystemAdmin-only create/delete** — workspace self-service creation is
   deferred; deletion requires SystemAdmin to prevent accidental loss of
   cross-cluster attribution.

## Verification

- `go build ./...` — passes.
- `go test ./internal/workspace/ -count=1` — 29 tests pass.
- `go test ./internal/httpserver/ -run TestWorkspaceHandler -count=1` — 10
  tests pass.
- `go test ./internal/httpserver/ -run TestRegisteredRoutesMatchOpenAPI` —
  OpenAPI route contract test passes (14 new routes covered).
- `verify-fast.ps1 -Scope All` — fast gate passed (backend=True frontend=True
  manifests=True, 63.48s).
