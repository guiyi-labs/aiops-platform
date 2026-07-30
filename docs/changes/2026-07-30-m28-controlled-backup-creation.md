# M28: Controlled Velero Backup Creation

- Date: 2026-07-30
- Status: Accepted (fast gate passed; real-kind E2E deferred to environment with Velero controller)
- ADR: [0044-controlled-velero-backup-creation.md](../adr/0044-controlled-velero-backup-creation.md)

## Summary

M28 adds controlled Velero Backup creation through a fixed-scope, two-phase
confirmation workflow. Operators with system/operations admin roles can create
Velero Backup CRs through the platform with server-side preflight validation,
one-time confirmation tokens, idempotent execution, and full audit trails.
Restore remains explicitly disabled pending M31.

## Product Outcome

An operations administrator can:

1. Open the Workload Protection page and click "Create Backup"
2. Fill in backup name, namespace, included namespaces, storage location, TTL, and options
3. Preview triggers four-gate preflight (Velero installed, BSL exists, name available, dry-run)
4. Review the confirmed plan parameters and expiration
5. Confirm with a one-time token to create the actual Backup CR
6. The backup appears in the read-only inventory list

## Implementation

### Backend

- `internal/backup/model.go` — Plan, PlanResponse, Request, Parameters types with JSONB/StringArray GORM helpers
- `internal/backup/repository.go` — GORM repository with Claim/Complete/Fail (SELECT FOR UPDATE, constant-time token compare, stale lock recovery)
- `internal/backup/service.go` — Preview (4-gate preflight + dry-run), Execute (claim + create), List, validateRequest, buildBackupManifest, newIdentity, safeExecutionError
- `internal/backup/service_test.go` — 13 unit tests covering all paths
- `internal/httpserver/backup.go` — HTTP handlers (preview, execute, list) with strict JSON decoding and sentinel error mapping
- `internal/httpserver/router.go` — Route registration under `/api/v1/clusters/:cluster_id/backup-plans` and `/api/v1/backup-plans/:plan_id/execute`
- `internal/httpserver/audit.go` — `backup.preview` and `backup.execute` audit mappings
- `internal/kubernetes/service.go` — New `BackupStorageLocations` and `VeleroBackupExists` methods
- `migrations/000021_backup_plans.up.sql` / `.down.sql` — `backup_plans` table with CHECK constraints
- `cmd/server/main.go` — Service wiring

### Frontend

- `frontend/src/types/kubernetes.ts` — `BackupPlan` and `BackupStorageLocation` types
- `frontend/src/api/kubernetes.ts` — `previewBackupPlan`, `executeBackupPlan`, `listBackupPlans` functions
- `frontend/src/views/WorkloadProtectionView.vue` — Create backup dialog with two-phase UX (form → preview → confirm)

## Fixed V1 Contract

- Backup name: DNS-1123, max 253 chars
- Backup namespace: DNS-1123, max 63 chars (typically `velero`)
- Included namespaces: 1–10 explicit names, no wildcard
- Storage location: must exist as a Velero BSL CR
- TTL: duration string `^[0-9]+(h|m|s)$`, default `720h`
- Include cluster resources: boolean, default false
- Snapshot volumes: boolean, default false
- Label selector: matchLabels only, max 10 key-value pairs
- No hooks, schedules, ordered_resources, or arbitrary YAML

## Non-goals

- Restore (deferred to M31)
- Backup schedules (recurring backups)
- Backup deletion through the platform
- BackupStorageLocation creation or management
- Volume snapshot class configuration
- Cross-cluster backup migration

## Verification

### L0 - Static and Focused
- `gofmt` on all changed Go files
- `go test ./internal/backup` — 13 tests pass
- `go test ./...` — all 23 backend packages pass
- `vue-tsc -b` — zero frontend type errors
- `vitest run` — 73 frontend tests pass

### L1 - Fast Repository Gate
- `scripts/verify-fast.ps1 -Scope All` — **PASSED** in 34.92s
- All backend packages pass (including new `backup` package)
- Frontend typecheck, Vitest (73 tests), and build pass
- Compose and Kustomize contracts pass

### L2/L3 - Real-kind E2E
- Deferred: requires a Velero controller with a configured BSL and object storage
- The M25 CRD-stub approach is insufficient for creation testing
- E2E script to be prepared for environments with full Velero installation

### Unit Test Coverage

| Test | Scenario |
|---|---|
| `TestPreviewRejectsInvalidRequest` | Missing name, too many namespaces |
| `TestPreviewFailsWhenVeleroNotInstalled` | Velero absent |
| `TestPreviewFailsWhenStorageLocationNotFound` | BSL does not match |
| `TestPreviewFailsOnBackupNameConflict` | Backup name already exists |
| `TestPreviewPerformsDryRunAndStoresHash` | Dry-run called, token hash stored, plaintext not persisted |
| `TestExecuteCreatesBackupAndCompletes` | Create without dry-run, Complete called |
| `TestExecuteFailsOnClaimError` | Claim error propagated, no create attempted |
| `TestExecuteFailsAndRecordsError` | Create error recorded in Fail |
| `TestExecuteRejectsInvalidIdempotencyKey` | Short key rejected |
| `TestExecuteRejectsEmptyConfirmation` | Empty token rejected |
| `TestValidateRequestAcceptsDefaults` | Valid request passes |
| `TestValidateRequestRejectsBadTTL` | Invalid TTL pattern |
| `TestValidateRequestRejectsEmptyNamespaces` | Empty namespace list |

## Security

- Authorization: mutations restricted to system/operations admin
- Audit: `backup.preview` and `backup.execute` events recorded
- Confirmation: one-time token, SHA-256 hash stored, constant-time comparison
- Idempotency: 8–128 char key, Claim transaction with SELECT FOR UPDATE
- Error boundary: `safeExecutionError` prevents K8s API details from leaking
- Fixed scope: no arbitrary YAML, no client-controlled patch content

## Files Changed

### Backend
- `backend/internal/backup/model.go` — domain models and validation
- `backend/internal/backup/repository.go` — GORM repository with Claim/Complete/Fail
- `backend/internal/backup/service.go` — Preview, Execute, List, manifest builder
- `backend/internal/backup/service_test.go` — 13 unit tests
- `backend/internal/httpserver/backup.go` — HTTP handlers
- `backend/internal/httpserver/router.go` — route registration
- `backend/internal/httpserver/audit.go` — audit mappings
- `backend/internal/kubernetes/service.go` — BSL listing and backup existence check
- `backend/cmd/server/main.go` — service wiring
- `backend/migrations/000021_backup_plans.up.sql` — schema migration
- `backend/migrations/000021_backup_plans.down.sql` — rollback

### Frontend
- `frontend/src/types/kubernetes.ts` — BackupPlan and BackupStorageLocation types
- `frontend/src/api/kubernetes.ts` — backup plan API functions
- `frontend/src/views/WorkloadProtectionView.vue` — create backup dialog

### Documentation
- `docs/adr/0044-controlled-velero-backup-creation.md` — architecture decision
- `docs/changes/2026-07-30-m28-controlled-backup-creation.md` — this document
