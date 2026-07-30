# ADR 0047: Isolated Workload Restore Rehearsal

- Date: 2026-07-30
- Status: Accepted
- Related milestones: M31, M28 (Velero Backup creation), M23 (controlled operations pattern), M19 (controlled operations catalog)

## Context

M28 (ADR 0044) added controlled Velero Backup creation but explicitly left
restore disabled until a separate conflict/PV/cutover/rollback design was
approved. The roadmap (M31) calls for an isolated workload restore rehearsal
that proves an M28-compatible Backup is restorable without touching production
state.

The risk surface of restore is higher than backup: a naive restore can
overwrite live resources, re-attach PersistentVolumes, recreate Services/Ingress
that hijack traffic, recreate RBAC that widens access, or recreate webhooks
that intercept admission. The design must fail closed on every one of these
and must never restore into a namespace the operator names.

## Decision

### 1. Separate `internal/restore` package

Create a dedicated `restore_plans` table (migration 000023) and
`internal/restore` package. The restore domain has different parameters
(source Backup identity, destination Namespace, quarantine controls, restored
item projection) that do not fit the existing plan CHECK constraints. The
controlled-operations *pattern* (Preview → Execute, confirmation token,
idempotency, Claim/Complete/Fail) is reused but the persistence surface is
separate.

### 2. Fixed V1 scope — rehearsal into a quarantine Namespace only

The restore service supports exactly one operation: rehearse the restore of a
single M28-compatible Velero Backup into a **server-generated** quarantine
Namespace. There is no operator-supplied destination name, no restore into an
existing Namespace, no in-place restore, no cross-cluster restore, and no
cutover/rollback path.

The destination Namespace name is derived from the source Backup name
(truncated, sanitized) and the requesting actor ID:
`restore-{shortBackupName}-{actorID}`. This makes audit tracing deterministic
without exposing caller-controlled names.

### 3. M28-compatible source Backup preconditions

The source Backup must:

- Exist in the target cluster (Velero `backups.velero.io`)
- Be in `Completed` phase
- Have exactly one included namespace, `includeClusterResources=false`,
  `snapshotVolumes=false`, and no label selector

Any other phase, scope, or missing identity fails preview closed. Backup UID
and resourceVersion are captured and compared exactly at execute time.

### 4. Quarantine controls established before Restore

Before the Velero Restore CR is created, the destination Namespace is created
with two quarantine controls:

- **NetworkPolicy** (`quarantine-default-deny`): default-deny all ingress and
  egress. No exceptions. Restored workloads cannot reach any network.
- **ResourceQuota** (`quarantine-zero-pods`): `pods=0`,
  `services.loadbalancers=0`, `services.nodeports=0`. No restored workload can
  schedule a Pod, expose a LoadBalancer, or bind a NodePort.

The Namespace is labeled `k8s-aiops.local/restore-rehearsal=true` and
`k8s-aiops.local/quarantine=true` for easy identification and cleanup.

Creation order is strict: **Namespace → NetworkPolicy + ResourceQuota →
Restore**. If any quarantine control fails after the Namespace is created, the
plan is marked `failed` with `ErrQuarantineFailed` and the Namespace is
**retained** (not auto-deleted) so the operator can inspect the partial state.
The execution result records `quarantine_established=false` and the failure
reason.

### 5. Fixed restore resource allowlist

The Velero Restore CR is constructed with a fixed `includedResources` allowlist
(lowercased plural):

- `deployments`, `statefulsets`, `daemonsets`, `cronjobs`, `configmaps`,
  `secrets`, `serviceaccounts`

And fixed `includeClusterResources=false`, `restorePVs=false`. The explicit
exclusion list (documented in `ExcludedKinds` and surfaced in the API response)
covers: Pod, Job, Service, Ingress, Endpoints, EndpointSlice, PVC, PV,
VolumeSnapshot, VolumeSnapshotContent, ResourceQuota, LimitRange, NetworkPolicy,
RoleBinding, ClusterRoleBinding, Role, ClusterRole, MutatingWebhookConfiguration,
ValidatingWebhookConfiguration.

No operator-supplied include/exclude list is accepted. The allowlist is the
only way resources enter the quarantine Namespace.

### 6. Two-phase confirmation with idempotency

Identical to M19/ADR 0023, M28/ADR 0044 and M30/ADR 0046 pattern:

- Preview returns a one-time confirmation token (SHA-256 hash stored, plaintext
  returned once and never persisted)
- Preview dry-runs the destination Namespace using its final generated name.
  NetworkPolicy and ResourceQuota schemas/RBAC are dry-run validated in the
  existing Velero control Namespace because Kubernetes rejects namespaced
  dry-runs when the not-yet-created destination Namespace does not exist. Their
  final manifests are otherwise identical except for `metadata.namespace`.
  The Velero Restore CR is dry-run in `velero` with the final destination
  `namespaceMapping`. This separates feasible admission checks without
  weakening the execute-time creation order or quarantine contract.
- Preview captures source Backup name, control Namespace, UID, resourceVersion,
  phase and included source Namespace as immutable evidence
- Execute requires the token + `Idempotency-Key` header (8–128 chars)
- `Claim` transaction uses `SELECT FOR UPDATE` + constant-time token comparison
- Stale lock recovery (claim TTL = 1 min), plan TTL = 10 min
- `Complete`/`Fail` condition on `status=executing AND idempotency_key=?`
- Same-key replay returns the same plan without re-mutating

### 7. Execute: re-verify, quarantine, restore, poll, project

`Execute` re-verifies all preconditions (the exact source Backup identity and
M28 scope are unchanged,
destination still absent, Restore name still available) before any mutation.
The execution sequence is:

1. Create quarantine Namespace (capture UID)
2. Create NetworkPolicy and ResourceQuota (quarantine controls)
3. Create exactly one Velero Restore CR whose `namespaceMapping` key is the
   Backup's included source Namespace, never the Backup CR name (capture UID)
4. Poll the Restore CR until terminal phase (`Completed`,
   `PartiallyFailed`, `Failed`) or bounded timeout (60 attempts × 5s = 5 min)
5. Project restored items by listing the allowlisted kinds in the destination
   Namespace (bounded to 100 items; Secrets listed by name only, no values)

On poll timeout the plan is marked `failed` with `ErrRestorePollTimeout` and
the quarantine Namespace is retained. On `PartiallyFailed` the plan is marked
`failed` with `ErrPartialRestore` and the Namespace is retained. On `Failed`
the plan is marked `failed` with `ErrExecutionFailed` and the Namespace is
retained. In all failure cases `execution_result` records the phase, UID,
restored items, and failure reason.

### 8. Hard prohibitions (encoded in interface bounds)

The `KubernetesSource` interface consumed by the restore service exposes
exactly these methods:

```go
type KubernetesSource interface {
  VeleroCapability(ctx, clusterID) (VeleroCapability, error)
  Backup(ctx, clusterID, namespace, name) (VeleroBackup, error)
  NamespaceExists(ctx, clusterID, namespace) (bool, error)
  VeleroRestoreExists(ctx, clusterID, namespace, name) (bool, error)
  VeleroRestore(ctx, clusterID, namespace, name) (VeleroRestore, error)
  CreateResource(ctx, clusterID, path, body, dryRun) ([]byte, error)
  Deployments/StatefulSets/DaemonSets/CronJobs/ConfigMaps/Secrets/ServiceAccounts
    (ctx, clusterID, namespace, query) (ListResponse[T], error)
}
```

No `DeleteResource`, no `UpdateResource`, no `Patch`, no Pod/Service/Ingress
mutation, no PV/PVC mutation, no Secret value reads, no Restore delete/update.
The only mutating surface is `CreateResource`, bound to fixed manifest bodies
constructed by `buildNamespaceManifest`, `buildQuarantineManifests`, and
`buildRestoreManifest`.

### 9. Authorization

- Preview/Execute: `requireRoles(SystemAdmin, OperationsAdmin)`
- List plans: any authenticated user
- Read Backup/Restore: any authenticated user (unchanged from M25/M28)

### 10. Audit

Two new audit actions (registered in `audit.go`):

- `restore.preview` → `RestorePlan` resource
- `restore.execute` → `RestorePlan` resource

Audit details record `method`, `path_template`, and `cluster_id`. The
confirmation token, idempotency key and kubeconfig are never recorded.

### 11. Frontend: two-pane RestoreRehearsalView under 分析与治理

The frontend view (`/restore-rehearsal`, navigated via ConsoleLayout →
分析与治理 → 恢复演练) follows the same two-pane pattern used by the
maintenance view:

- **Left pane**: source backup namespace/name inputs, preview button, and a
  plan history list (last 30 plans)
- **Right pane**: preview confirmation (with quarantine control summary,
  allowed/excluded resource kind tags, one-time token display, warning banner)
  or execution result (with restore phase, restored item count, restored item
  table, partial-failure banner)

The route is role-gated to `system_admin` and `operations_admin`. The
destructive `danger-button` is used for the execute confirmation.

## Consequences

- New migration 000023 (`restore_plans` table) required
- `httpserver.Service` gains one new constructor-injected dependency
  (`*restore.Service`) and three new handlers
- `kubernetes.Service` gains `VeleroRestore` and `VeleroRestoreExists` methods
  (read-only Velero Restore CR access)
- Frontend gains a new `RestoreRehearsalView` and navigation entry under
  分析与治理
- The `KubernetesSource` interface bounds the mutation surface to
  `CreateResource` only; no delete, update, patch, or Secret-value path exists
  in the restore package
- Real-kind E2E uses a disposable kind cluster, pinned Velero and disposable
  S3-compatible storage through `scripts/e2e-m31-isolated-restore-kind.ps1`
- Quarantine Namespace cleanup is operator-driven; the platform does not
  auto-delete quarantine Namespaces. The `k8s-aiops.local/quarantine` label
  enables future cleanup tooling
- The PV/PVC restore path is intentionally disabled (`restorePVs=false`);
  stateful workload restore rehearsal is limited to manifests only
