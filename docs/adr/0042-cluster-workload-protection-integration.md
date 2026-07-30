# ADR 0042: Cluster Workload Protection Integration

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend, Frontend, Security

## Context

The 2026-07-28 KRM/Ratel gap review identified workload protection
(backup/restore inventory) as the next high-frequency operator workflow the
platform did not cover. Operators running multi-cluster fleets need visibility
into Velero Backup state — phase, scope, expiry, errors — without leaving the
platform console, but the platform must not make Velero a hard dependency:
many clusters do not run Velero, and the platform must keep working when it is
absent.

M25 covers the **read-only inventory** half of the workload-protection
contract. The roadmap explicitly defers controlled backup creation until
read-only inventory and real-kind compatibility are accepted, and keeps
restore disabled until destination isolation, resource conflict,
persistent-volume behavior, cutover and rollback policies have separate
approval and disposable recovery evidence.

The hard constraints are inherited from M19/M23/M24:

- The platform never accepts a client-owned manifest for execution. M25 is
  read-only, so no mutation surface is introduced at all.
- Every server-derived response is bounded and projected. The Velero Backup
  CR carries fields the platform has no reason to expose (volume snapshot
  locations, hooks, label selectors, raw object storage references); only a
  bounded projection is returned.
- Optional capabilities must degrade gracefully. A cluster without Velero is
  normal, not an error.

## Decision

### 1. Velero is detected as an optional capability, never a core dependency

`Service.VeleroCapability` probes `/apis/velero.io/v1` on the target cluster.
A 404 means Velero is not installed; the method returns
`{Installed: false}` **without error**. Any other probe failure (auth,
connectivity, throttle) is propagated as a real error. This mirrors the
metrics-server capability detection pattern: absence is a normal state, not a
failure.

The frontend calls `getVeleroCapability` immediately after cluster selection.
When `installed` is false, the page renders a read-only empty state explaining
that Velero is not installed and the platform does not treat it as a core
dependency. No backup list call is made.

### 2. The backup inventory is read-only and bounded

`Service.Backups` lists Velero Backup CRs via
`/apis/velero.io/v1/backups` (cluster-wide) or
`/apis/velero.io/v1/namespaces/{ns}/backups` (namespace-scoped). The raw CR is
decoded into `rawVeleroBackup` (an internal type) and projected into
`VeleroBackup` (the public type). The projection carries only:

- `name`, `namespace` (identity)
- `phase` (Velero status phase)
- `included_namespaces` (backup scope)
- `storage_location`, `ttl` (retention intent)
- `expiration`, `started_at`, `completed_at` (lifecycle timestamps)
- `failure_reason` (populated only when phase is `Failed`)
- `errors`, `warnings` (bounded integer counts)
- `created_at` (metadata creation timestamp)

Fields not in this list — volume snapshot locations, hooks, label selectors,
raw object storage keys, node-agent state, backup result details — are
**dropped** at projection time. The platform never surfaces object-storage
credentials or snapshot provider secrets through this API.

`Service.Backup` returns the same projection for a single named Backup CR.

### 3. Velero-absent is distinguished from backup-not-found

`Service.Backups` and `Service.Backup` first distinguish "Velero is not
installed" from "the specific Backup CR was not found". When the Velero API
group itself returns 404, the service returns `ErrVeleroUnavailable`. When the
API group exists but the named Backup CR returns 404, `Service.Backup`
returns `ErrResourceNotFound`.

This split is important: `ErrVeleroUnavailable` tells the operator "install
Velero to use this page", while `ErrResourceNotFound` tells the operator "the
backup you named does not exist". The HTTP handler maps
`ErrVeleroUnavailable` to `424 Failed Dependency` with code
`VELERO_UNAVAILABLE`, and `ErrResourceNotFound` to `404` with
`KUBERNETES_RESOURCE_NOT_FOUND`.

### 4. No database migration, no persistence

M25 is read-only inventory. There is no `workload_protection` table, no
cached backup state, no scheduled refresh job. Every call to `Backups` or
`Backup` hits the target cluster's Velero API live. This keeps the contract
honest — operators see current Velero state, not a stale snapshot — and avoids
introducing a new persistence surface that would need its own TTL, refresh
and conflict policy.

If a future milestone adds scheduled backup creation or backup policy, that
milestone owns its own migration and persistence. M25 does not pre-create it.

### 5. The HTTP surface is fixed and read-only

Three routes are mounted under `v1`:

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| `GET` | `/api/v1/clusters/{cluster_id}/velero/capability` | any authenticated | probe Velero API group presence |
| `GET` | `/api/v1/clusters/{cluster_id}/backups` | any authenticated | list bounded Velero Backup CRs |
| `GET` | `/api/v1/clusters/{cluster_id}/backups/{namespace}/{name}` | any authenticated | read a single Velero Backup CR |

All three are `GET`. No `POST`, `PUT`, `PATCH` or `DELETE` is mounted for
backups. The handler maps `ErrVeleroUnavailable` to `424` and
`ErrResourceNotFound` to `404`; other gateway errors fall through to the
existing `KUBERNETES_API_ERROR` mapping.

`backups` accepts the standard `page`, `limit`, `namespace`, `name`,
`sort_by`, `ascending` query parameters and reuses the existing
`parseKubernetesListQuery` helper. The list response is the generic
`apiquery.ListResponse[VeleroBackup]` shape used by every other resource
list endpoint.

### 6. The frontend reuses the existing resource-console shell

`WorkloadProtectionView` reuses `ConsoleLayout`, the existing
`resource-toolbar`, `resource-panel`, `pod-table` and `resource-empty` styles.
Cluster selection mirrors the workloads view. The backup table columns are:
name, namespace, phase, scope, storage location, errors/warnings, start time,
expiration. A detail drawer opens on row click and renders the full bounded
projection as a read-only grid.

The page has no create, edit, delete, restore or schedule button. The toolbar
has only cluster selector, namespace filter, name search and refresh. This is
intentional: M25 is inventory, not operations.

### 7. Real-kind E2E proves the read-only contract without a live Velero install

The disposable kind E2E script
`scripts/e2e-m25-workload-protection-kind.ps1` and fixtures under
`deploy/m25-workload-protection-e2e` exercise the read-only inventory contract
against a uniquely named kind cluster. Because installing a full Velero stack
(object storage provider, CSI driver, Velero server) in a disposable kind
cluster is out of scope for an inventory-only milestone, the E2E applies a
**minimal Velero API group stub** (the `backups.velero.io` CRD with no
controller) and two sample Backup CRs, then asserts:

- `velero/capability` reports `{installed: true, version: "v1"}` once the
  CRD is registered.
- `backups` returns the two sample Backup CRs with the bounded projection
  (phase, included namespaces, storage location, errors/warnings, timestamps).
- `backups/{namespace}/{name}` returns the projected single resource.
- A second cluster without the CRD reports `{installed: false}` and the
  `backups` endpoint returns `424 VELERO_UNAVAILABLE`.
- The observer RBAC allows `get`/`list` on `backups.velero.io`; `create` is
  denied.
- In `finally`, the cluster registration and disposable kind cluster are
  removed. Sanitized evidence is written to
  `.artifacts/m25-workload-protection-kind`.

## Consequences

- Velero is an optional capability. Clusters without Velero render an empty
  state; the platform never fails a health check or a readiness probe because
  Velero is absent.
- The backup inventory is always live. There is no cache, so a Velero API
  outage surfaces as a real error on the page. Operators refresh manually.
- The projection is bounded. Adding a new exposed field requires a service
  change, a type change and an OpenAPI schema change. Sensitive fields
  (object-storage keys, snapshot provider secrets) are never surfaced.
- No mutation surface is introduced. M25 cannot create, delete, restore or
  schedule backups. A future milestone that adds controlled backup creation
  must reuse the M19/M23/M24 dry-run/confirmation/audit contract and own its
  own persistence.
- Restore remains disabled. The platform does not expose a restore button or
  API. A future milestone that adds restore must solve destination isolation,
  resource conflict, persistent-volume behavior, cutover and rollback first.

## Boundary

M25 does not include:

- Controlled backup creation, scheduling or deletion. The API is read-only.
- Restore of any kind. Restore is explicitly disabled and deferred.
- Backup policy management, retention enforcement or TTL cleanup.
- Cross-cluster backup replication or backup migration.
- Object-storage credential management. The platform never collects provider
  credentials through the browser and never surfaces them in the API.
- Volume snapshot detail, restic repo status, or node-agent state.
- A persistent cache of backup state. Every call hits the Velero API live.
- Organization integration (M26) or SSO.
