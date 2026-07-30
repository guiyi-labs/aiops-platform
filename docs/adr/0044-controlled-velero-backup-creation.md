# ADR 0044: Controlled Velero Backup Creation

- Date: 2026-07-30
- Status: Accepted
- Related milestones: M28, M25 (read-only baseline), M19 (controlled operations pattern)

## Context

M25 established a read-only Velero Backup inventory with bounded projections and
optional capability detection. Operators can view existing Backup CRs but cannot
create new ones through the platform.

The roadmap (M28) calls for controlled Velero Backup creation with fixed scope,
server-side preflight, one-time confirmation, idempotency, audit and disposable
object-storage evidence. Restore remains explicitly disabled until M31 designs
conflict/PV/cutover/rollback policies.

## Decision

### 1. Separate `internal/backup` package

Create a dedicated `backup_plans` table (migration 000021) and `internal/backup`
package rather than reusing `remediation_plans`. The backup domain has different
parameters (namespaces, storage location, TTL, snapshots) that do not fit the
remediation action CHECK constraints. The controlled-operations *pattern* (Preview
→ Execute, confirmation token, idempotency, Claim/Complete/Fail) is reused but
the persistence surface is separate.

### 2. Fixed scope — no arbitrary YAML

The Backup CR spec is constructed server-side from a fixed set of fields:
- `backup_name`, `backup_namespace` (required, validated DNS-1123 names)
- `included_namespaces` (1–10 explicit names, no wildcard `*`)
- `storage_location` (required, must exist as a BSL CR)
- `ttl` (duration pattern `^[0-9]+(h|m|s)$`, default `720h`)
- `include_cluster_resources` (boolean, default false)
- `snapshot_volumes` (boolean, default false)
- `label_selector` (optional, matchLabels only, max 10 entries)

No hooks, no schedules, no ordered_resources, no included_resources, no
arbitrary JSON/YAML. The manifest is built by `buildBackupManifest()`.

### 3. Four-gate preflight

Preview runs four checks before persisting a plan:
1. **Velero installed** — `VeleroCapability` probe; `ErrVeleroNotInstalled` if absent
2. **Storage location exists** — `BackupStorageLocations` list; `ErrStorageLocationNotFound`
3. **Name not taken** — `VeleroBackupExists` check; `ErrBackupNameConflict`
4. **Server-side dry-run** — `CreateResource(..., dryRun=true)`; admission validation without persistence

### 4. Two-phase confirmation with idempotency

Identical to M19/ADR 0023 pattern:
- Preview returns a one-time confirmation token (SHA-256 hash stored, plaintext returned once)
- Execute requires the token + `Idempotency-Key` header (8–128 chars)
- `Claim` transaction uses `SELECT FOR UPDATE` + `constant-time` comparison
- Stale lock recovery (claim TTL = 1 min), plan TTL = 10 min
- `Complete`/`Fail` condition on `status=executing AND idempotency_key=?`

### 5. Restore disabled

No restore endpoints, no restore UI. The detail drawer explicitly states restore
is disabled pending M31. This is a hard boundary, not a feature flag.

### 6. Authorization

- Preview/Execute: `requireRoles(SystemAdmin, OperationsAdmin)`
- List plans: any authenticated user
- Read backup inventory (M25): any authenticated user (unchanged)

### 7. Audit

Two new audit actions:
- `backup.preview` → `BackupPlan` resource
- `backup.execute` → `BackupPlan` resource

## Consequences

- New migration 000021 (`backup_plans` table) required
- `kubernetes.Service` gains `BackupStorageLocations` and `VeleroBackupExists` methods
- Frontend `WorkloadProtectionView` gains a create-backup dialog with two-phase UX
- Real-kind E2E requires a Velero controller with a configured BSL; the CRD-stub
  approach from M25 is insufficient for creation testing
- The `backup_plans` table is the first persistent state for backup operations;
  M25 was stateless by design
