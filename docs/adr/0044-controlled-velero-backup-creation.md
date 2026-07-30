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
object-storage evidence. M31 subsequently added only the isolated rehearsal
path defined by ADR 0047; in-place restore, PV restore, cutover and rollback
remain prohibited.

## Decision

### 1. Separate `internal/backup` package

Create a dedicated `backup_plans` table (migration 000021) and `internal/backup`
package rather than reusing `remediation_plans`. The backup domain has different
parameters (source Namespace identity, storage location and TTL) that do not fit the
remediation action CHECK constraints. The controlled-operations *pattern* (Preview
→ Execute, confirmation token, idempotency, Claim/Complete/Fail) is reused but
the persistence surface is separate.

### 2. Fixed scope — no arbitrary YAML

The caller supplies only one `source_namespace`, one `storage_location` and a
TTL from `24h`, `168h` or `720h`. The service generates the Backup name and
uses the fixed Velero control Namespace `velero`. Preview captures the source
Namespace UID/resourceVersion. `includeClusterResources=false` and
`snapshotVolumes=false` are unconditional; label selectors are not accepted.

No hooks, no schedules, no ordered_resources, no included_resources, no
arbitrary JSON/YAML. The manifest is built by `buildBackupManifest()`.

### 3. Five-gate preflight

Preview runs five checks before persisting a plan:
1. **Velero installed** — `VeleroCapability` probe; `ErrVeleroNotInstalled` if absent
2. **Source identity** — the exact Namespace exists and exposes UID/resourceVersion
3. **Storage location Available** — exact-name BSL exists with `status.phase=Available`
4. **Generated name not taken** — `VeleroBackupExists` check
5. **Server-side dry-run** — fixed manifest admission without persistence

### 4. Two-phase confirmation with idempotency

Identical to M19/ADR 0023 pattern:
- Preview returns a one-time confirmation token (SHA-256 hash stored, plaintext returned once)
- Execute requires the token + `Idempotency-Key` header (8–128 chars)
- `Claim` transaction uses `SELECT FOR UPDATE` + `constant-time` comparison
- Stale lock recovery (claim TTL = 1 min), plan TTL = 10 min
- `Complete`/`Fail` condition on `status=executing AND idempotency_key=?`
- Execute rechecks Namespace UID/resourceVersion, Velero capability, BSL phase
  and name absence before mutation, then persists returned Backup UID/resourceVersion

### 5. Restore boundary

M28 itself adds no restore mutation. The later M31 surface accepts only an
M28-compatible completed Backup and restores its manifest allowlist into a
server-generated, default-deny, zero-Pod quarantine Namespace. In-place
restore, PV/PVC restore, Service/Ingress recreation, traffic cutover and
rollback remain outside the platform contract.

### 6. Authorization

- Preview/Execute: `requireRoles(SystemAdmin, OperationsAdmin)`
- List plans: any authenticated user
- Read backup inventory (M25): any authenticated user (unchanged)

### 7. Audit

Two new audit actions:
- `backup.preview` → `BackupPlan` resource
- `backup.execute` → `BackupPlan` resource

## Consequences

- Migrations 000021 and 000024 persist the plan and exact source/result identities
- `kubernetes.Service` gains `BackupStorageLocations` and `VeleroBackupExists` methods
- Frontend `WorkloadProtectionView` gains a create-backup dialog with two-phase UX
- Real-kind E2E is implemented by
  `scripts/e2e-m28-backup-creation-kind.ps1` using pinned Velero, a configured
  BSL and disposable MinIO; it proves Completed phase, fixed scope, stale
  Namespace rejection, idempotent replay, least-privilege RBAC and cleanup
- The `backup_plans` table is the first persistent state for backup operations;
  M25 was stateless by design
