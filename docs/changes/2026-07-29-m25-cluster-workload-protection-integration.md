# M25: Cluster Workload Protection Integration

- Status: Accepted
- Date: 2026-07-29
- Scope: read-only Velero backup inventory, capability detection, bounded projection, frontend console, real-kind E2E with CRD stub
- Decision: ADR 0042

## Outcome

M25 closes the read-only half of the workload-protection gap identified by
the 2026-07-28 KRM/Ratel review. The platform now detects the Velero API
group as an **optional capability** — never a core dependency — and surfaces a
bounded, read-only backup inventory (phase, scope, storage location, expiry,
errors/warnings, lifecycle timestamps) per cluster.

Clusters without Velero render a read-only empty state; the platform never
fails a health check or readiness probe because Velero is absent. No mutation
surface is introduced: the API exposes three `GET` routes only. There is no
`POST`/`PUT`/`PATCH`/`DELETE` for backups, no persistence, no cache. Every
call hits the target cluster's Velero API live.

Restore remains explicitly disabled. Controlled backup creation is deferred
until read-only inventory and real-kind compatibility are accepted (this
milestone). A future milestone that adds restore must solve destination
isolation, resource conflict, persistent-volume behavior, cutover and
rollback first.

## Product Surface

The `kubernetes.Service` gains three read-only methods:

- `VeleroCapability(ctx, clusterID)` — probes `/apis/velero.io/v1`. A 404
  returns `{Installed: false}` without error; any other failure is
  propagated. Mirrors the metrics-server capability detection pattern.
- `Backups(ctx, clusterID, namespace, query)` — lists Velero Backup CRs via
  `/apis/velero.io/v1/backups` (cluster-wide) or
  `/apis/velero.io/v1/namespaces/{ns}/backups` (namespace-scoped). Returns
  `ErrVeleroUnavailable` when the Velero API group is absent, the generic
  `ListResponse[VeleroBackup]` shape otherwise.
- `Backup(ctx, clusterID, namespace, name)` — reads a single Backup CR.
  Distinguishes `ErrVeleroUnavailable` (API group absent) from
  `ErrResourceNotFound` (Backup CR absent).

The raw Velero Backup CR is decoded into the internal `rawVeleroBackup` type
and projected into the public `VeleroBackup` type. The projection carries
only identity (`name`, `namespace`), state (`phase`, `failure_reason`,
`errors`, `warnings`), scope (`included_namespaces`), retention intent
(`storage_location`, `ttl`) and lifecycle timestamps (`expiration`,
`started_at`, `completed_at`, `created_at`). Volume snapshot locations,
hooks, label selectors, raw object-storage references and node-agent state
are dropped at projection time. The platform never surfaces provider
credentials through this API.

Three HTTP routes are mounted under `v1`:

- `GET /api/v1/clusters/{cluster_id}/velero/capability` — any authenticated
  role. Returns `VeleroCapability`.
- `GET /api/v1/clusters/{cluster_id}/backups` — any authenticated role.
  Accepts the standard `page`, `limit`, `namespace`, `name`, `sort_by`,
  `ascending` query parameters. Returns `VeleroBackupList`.
- `GET /api/v1/clusters/{cluster_id}/backups/{namespace}/{name}` — any
  authenticated role. Returns `VeleroBackup`.

The handler maps `ErrVeleroUnavailable` to `424 Failed Dependency` with code
`VELERO_UNAVAILABLE`, and `ErrResourceNotFound` to `404` with
`KUBERNETES_RESOURCE_NOT_FOUND`. Other gateway errors fall through to the
existing `KUBERNETES_API_ERROR` mapping. Audit target and cluster ID are set
on every call.

The frontend `WorkloadProtectionView` reuses `ConsoleLayout`, the existing
`resource-toolbar`, `resource-panel`, `pod-table` and `resource-empty` styles.
Cluster selection mirrors the workloads view. After cluster selection the
page calls `getVeleroCapability`; when `installed` is false it renders a
read-only banner explaining that Velero is not installed and the platform
does not treat it as a core dependency. No backup list call is made in that
state.

When Velero is installed, the page renders four summary cards (total,
completed, failed/partially-failed, in-progress/pending) and a backup table
with columns: name, namespace, phase, scope, storage location, errors/
warnings, start time, expiration. A detail drawer opens on row click and
renders the full bounded projection as a read-only grid with an explicit
notice that the page is read-only and restore remains disabled. The toolbar
has only cluster selector, namespace filter, name search and refresh — no
create, edit, delete, restore or schedule button.

## Real Kind Evidence

The disposable kind E2E script
`scripts/e2e-m25-workload-protection-kind.ps1` and fixtures under
`deploy/m25-workload-protection-e2e` exercise the read-only inventory
contract against a uniquely named kind cluster. Because installing a full
Velero stack (object storage provider, CSI driver, Velero server) in a
disposable kind cluster is out of scope for an inventory-only milestone, the
E2E applies a **minimal Velero API group stub** (the `backups.velero.io` CRD
with no controller) and two sample Backup CRs, then asserts:

- `velero/capability` reports `{installed: true, version: "v1"}` once the
  CRD is registered on the primary cluster.
- `backups` returns the two sample Backup CRs with the bounded projection
  (phase, included namespaces, storage location, errors/warnings,
  timestamps).
- `backups/{namespace}/{name}` returns the projected single resource for the
  `completed-backup` sample.
- A second cluster without the CRD reports `{installed: false}` and the
  `backups` endpoint returns `424 VELERO_UNAVAILABLE`.
- The observer RBAC allows `get`/`list` on `backups.velero.io`; `create` is
  denied.
- In `finally`, both cluster registrations and the disposable kind cluster
  are removed, and the previous kubectl context is restored. Sanitized
  evidence is written to `.artifacts/m25-workload-protection-kind`.

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `backend/internal/kubernetes/service.go` | Modify | Add `VeleroCapability`, `Backups`, `Backup` methods; `veleroJSON` helper; `VeleroCapability`, `VeleroBackup`, `rawVeleroBackup` types; `ErrVeleroUnavailable` |
| `backend/internal/kubernetes/service_test.go` | Modify | 7 unit tests covering capability detection, Velero-absent, bounded projection, namespace scoping, single-resource read, not-found distinction |
| `backend/internal/httpserver/kubernetes.go` | Modify | Add `veleroCapability`, `backups`, `backup` handlers; map `ErrVeleroUnavailable` to 424 |
| `backend/internal/httpserver/router.go` | Modify | Register `/velero/capability`, `/backups`, `/backups/:namespace/:name` routes |
| `frontend/src/types/kubernetes.ts` | Modify | Add `VeleroCapability`, `VeleroBackup` interfaces |
| `frontend/src/api/kubernetes.ts` | Modify | Add `getVeleroCapability`, `listBackups`, `getBackup` API client functions |
| `frontend/src/views/WorkloadProtectionView.vue` | New | Read-only Velero backup inventory view with cluster selection, capability banner, summary cards, backup table, detail drawer |
| `frontend/src/router/index.ts` | Modify | Add `/workload-protection` route |
| `frontend/src/components/ConsoleLayout.vue` | Modify | Add 工作负载保护 navigation item with `ShieldCheck` icon |
| `frontend/src/styles/base.css` | Modify | `velero-unavailable`, `backup-table`, `backup-detail-drawer`, `detail-grid` styles |
| `docs/api/openapi.yaml` | Modify | `/velero/capability`, `/backups`, `/backups/{namespace}/{name}` routes; `VeleroCapability`, `VeleroBackup`, `VeleroBackupList` schemas |
| `deploy/m25-workload-protection-e2e/primary/namespace.yaml` | New | Primary E2E Namespace |
| `deploy/m25-workload-protection-e2e/primary/velero-crd-stub.yaml` | New | Minimal `backups.velero.io` CRD stub (no controller) |
| `deploy/m25-workload-protection-e2e/primary/sample-backups.yaml` | New | Two sample Velero Backup CRs (completed + failed) |
| `deploy/m25-workload-protection-e2e/primary/kustomization.yaml` | New | Primary Kustomize entry |
| `deploy/m25-workload-protection-e2e/secondary/namespace.yaml` | New | Secondary E2E Namespace (no CRD) |
| `deploy/m25-workload-protection-e2e/secondary/kustomization.yaml` | New | Secondary Kustomize entry |
| `scripts/e2e-m25-workload-protection-kind.ps1` | New | Disposable kind E2E script with Velero CRD stub |
| `docs/adr/0042-cluster-workload-protection-integration.md` | New | Design decision |
| `docs/changes/2026-07-29-m25-cluster-workload-protection-integration.md` | New | Closure change log |
| `docs/roadmap.md` | Modify | Mark M25 ✅ Completed |

## Verification

- **Backend**: `go test ./...` — packages pass, including
  `TestVeleroCapabilityReturnsNotInstalledWhenAPIGroupMissing`,
  `TestVeleroCapabilityReturnsInstalledWhenAPIGroupExists`,
  `TestBackupsReturnsVeleroUnavailableWhenAPIGroupMissing`,
  `TestBackupsProjectsBoundedShapeAndPaginates`,
  `TestBackupsUsesNamespaceScopedPath`,
  `TestBackupReturnsProjectedSingleResource`,
  `TestBackupKeepsResourceNotFoundWhenVeleroInstalledButBackupMissing`.
- **Backend**: `go vet ./...` — no warnings.
- **Frontend**: `vue-tsc -b` — type-check passes clean.
- **OpenAPI parity**: `openapi_route_test` validates that the three new
  routes (`velero/capability`, `backups`, `backups/{namespace}/{name}`) are
  documented; the `VeleroCapability`, `VeleroBackup` and `VeleroBackupList`
  schemas enumerate the bounded projection.

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

The backup inventory is always live. A Velero API outage surfaces as a real
error on the page; operators refresh manually. There is no scheduled refresh
job and no background poller.
