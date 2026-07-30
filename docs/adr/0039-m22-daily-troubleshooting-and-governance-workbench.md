# ADR 0039: M22 Daily Troubleshooting And Governance Workbench

- Status: Accepted (Phase 1–2 implementation in progress)
- Date: 2026-07-29
- Owners: Backend, Frontend, Security

## Context

The platform currently supports basic Pod log retrieval
(`GET /clusters/:id/pods/:ns/:name/logs`) with `tail_lines` and `previous`
query parameters, but operators hit several hard limits:

1. **No container enumeration**: When a Pod has multiple containers, the
   caller must know the container name in advance. There is no way to list
   containers or fetch logs for all containers in one call.
2. **No bounded `since`**: The API does not support `sinceTime` or
   `sinceSeconds`, forcing operators to rely only on `tailLines` for
   time-based filtering.
3. **No truncation disclosure**: If the log output is truncated (e.g.,
   due to the 1 MiB body cap), the caller has no way to know whether
   they saw the complete tail or a truncated sample.
4. **No download/search**: Operators cannot download log output or search
   within the returned lines.

Additionally, the platform's resource contracts are incomplete. Several
resources that operators frequently need for governance and
troubleshooting — PersistentVolume, PodDisruptionBudget, NetworkPolicy,
ServiceAccount, Role/ClusterRole — are not exposed through fixed
read-only endpoints. The existing workbench UI is a single dense page
that does not scale as more resource kinds are added.

## Decision

M22 is split into four phases:

### Phase 1 — Container-Aware Pod Logs

Enhance the existing Pod logs endpoint with:

1. **Container enumeration**: A new endpoint
   `GET /clusters/:id/pods/:ns/:name/containers` returns the list of
   container names (both regular and init containers) for a Pod,
   along with status indicators (running, waiting, terminated).
2. **Bounded `since`**: Accept `since_time` (RFC3339) or `since_seconds`
   query parameters on the logs endpoint, bounded to a maximum of 24
   hours. The server rejects `since` values outside the window.
3. **Truncation disclosure**: The response gains a `truncated` boolean
   and a `truncation_reason` string (e.g., "body_limit", "time_limit")
   so the frontend can warn operators.
4. **Download and search**: The frontend renders log lines with
   timestamps, supports in-browser search, and offers a download
   button that issues the same authenticated request and triggers a
   file save.
5. **Response envelope**: The log response shape changes from
   `{ logs: string, previous: bool }` to
   `{ containers: [...], logs: [{container, lines, timestamps, truncated, truncation_reason}], previous: bool }`.
   The old shape is retained for backward compatibility via a `?format=v1`
   query parameter until the frontend fully migrates.

### Phase 2 — Read-Only Resource Contracts

Add fixed read-only list/detail endpoints for:

- **PersistentVolume**: `GET /clusters/:id/persistentvolumes` and
  `GET /clusters/:id/persistentvolumes/:name`
- **PodDisruptionBudget**: `GET /clusters/:id/poddisruptionbudgets` and
  `GET /clusters/:id/poddisruptionbudgets/:ns/:name`
- **NetworkPolicy**: `GET /clusters/:id/networkpolicies` and
  `GET /clusters/:id/networkpolicies/:ns/:name`
- **ServiceAccount**: `GET /clusters/:id/serviceaccounts` and
  `GET /clusters/:id/serviceaccounts/:ns/:name`
- **Role / ClusterRole / Binding metadata**: list-only endpoints for
  Role, ClusterRole, RoleBinding, ClusterRoleBinding — without token
  or sensitive field exposure.

All endpoints are strictly read-only (GET only), paginated, and follow
the established `{ items, total, remaining }` list response envelope.
The frontend types mirror the Kubernetes API shape with explicit
`kind`/`apiVersion`/`metadata`/`spec`/`status` fields.

### Phase 3 — Redacted Manifest Inspection

Add a `GET /clusters/:id/resources/:kind/:ns/:name/manifest` endpoint
that returns a server-redacted manifest for approved non-sensitive kinds
only. The allowlist:

- Approved: Pod, Deployment, Service, Ingress, PersistentVolumeClaim,
  PersistentVolume, PodDisruptionBudget, NetworkPolicy, ServiceAccount
  (metadata only), Role/ClusterRole (rules only)
- Excluded: Secret, ConfigMap, ServiceAccount tokens (`.secrets`,
  `.automountServiceAccountToken`), StorageClass parameters, any
  resource containing `.data` or `.stringData` fields

The server performs redaction — removing or replacing sensitive fields
with `"<redacted>"` — before returning the manifest. The UI never
requests excluded kinds; the server enforces the allowlist as a safety
net.

### Phase 4 — Workbench UX Split

Restructure the workload/resource workbench into three predictable
surfaces:

1. **Inventory view**: List-all resources with filtering, sorting, and
   pagination. Sidebar navigation by resource kind.
2. **Detail view**: Single resource with tabs for metadata, spec, status,
   events, logs (for Pods), manifest (redacted).
3. **Task view**: Actions that can be performed on a resource (diagnose,
  restart, scale, etc.) with pre-condition checks.

## Consequences

- Pod logs become a first-class troubleshooting primitive with
  container awareness, time bounding, truncation disclosure, and
  frontend search/download.
- Read-only resource contracts close a major gap for governance and
  troubleshooting, enabling operators to inspect PVs, PDBs,
  NetworkPolicies, and RBAC without `kubectl`.
- Manifest inspection with server-side redaction prevents sensitive
  data leakage. The allowlist approach fails closed: if a kind is not
  explicitly approved, the endpoint returns 404.
- The workbench split improves information architecture but requires
  updating the frontend routing and component structure.
- No write operations are introduced. All new endpoints are strictly
  read-only.
- The existing log endpoint remains backward-compatible via `?format=v1`.

## Boundary

M22 does not include:

- Write operations on any resource kind
- Secret or ConfigMap value exposure (ever)
- ServiceAccount token generation or viewing
- Multi-cluster aggregation views (M26)
- RBAC binding editing
- Direct kubectl-equivalent manifest editing