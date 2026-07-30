# M31: Isolated Workload Restore Rehearsal

- Date: 2026-07-30
- Status: Accepted (fast gate passed in 28.81s; real-kind E2E deferred to environments with Velero and a completed M28-compatible Backup)
- ADR: [0047-isolated-workload-restore-rehearsal.md](../adr/0047-isolated-workload-restore-rehearsal.md)

## Summary

M31 adds isolated workload restore rehearsal through a fixed-scope, two-phase
confirmation workflow. Operators with system/operations admin roles can rehearse
the restore of a single M28-compatible Velero Backup into a server-generated
quarantine Namespace with default-deny network policy and zero-Pod resource
quota, one-time confirmation tokens, idempotent execution, full audit trails,
and bounded restored-item projection. In-place restore, PV restore,
cross-cluster restore, cutover/rollback, operator-supplied destination names,
and arbitrary resource include/exclude lists remain explicitly prohibited.

## Product Outcome

An operations administrator can:

1. Open the Restore Rehearsal page under 分析与治理
2. Enter the source Backup namespace and name
3. Preview triggers precondition checks (Velero installed, source Backup
   Completed and M28-compatible single-namespace scope, destination Namespace
   absent, no active plan for the same source, Restore name available) and
   server-side dry-runs of the quarantine controls and Restore CR
4. Review the quarantine control summary (NetworkPolicy + ResourceQuota), the
   allowed/excluded resource kind tags, the server-generated destination
   Namespace name, and the plan expiration
5. Confirm with the one-time token and an Idempotency-Key header
6. Execution creates the quarantine Namespace, NetworkPolicy, ResourceQuota,
   then the Velero Restore CR, polls until terminal, and projects the restored
   items by listing allowlisted kinds in the destination Namespace
7. On partial failure, the quarantine Namespace is retained and the execution
   result records the phase, restored items, and failure reason

## Implementation

### Backend

- `internal/restore/model.go` — Plan, QuarantineStatus, ExecutionResult,
  RestoredItem, PlanResponse, SourceSummary types with JSONB GORM helpers;
  sentinel errors for every failure class (Velero not installed, source backup
  not found/incomplete/scope, destination exists/collision, restore name
  conflict, quarantine dry-run failed, restore dry-run failed, confirmation
  invalid, expired, in progress, already executed, stale source, quarantine
  failed, execution failed, poll timeout, partial restore)
- `internal/restore/repository.go` — GORM repository with Claim/Complete/Fail
  (SELECT FOR UPDATE, constant-time token compare, stale lock recovery) and
  ActiveBySource
- `internal/restore/service.go` — Preview (7 preflight checks + dry-runs +
  evidence capture), Execute (claim + re-verify + quarantine + restore + poll +
  project), List, pollRestore, projectRestoredItems, buildNamespaceManifest,
  buildQuarantineManifests, buildRestoreManifest, generateDestinationNamespace,
  sanitizeDNS1123, newIdentity, safeError
- `internal/restore/service_test.go` — 40+ unit tests covering all paths
- `internal/kubernetes/service.go` — VeleroRestore and VeleroRestoreExists
  methods (read-only Velero Restore CR access)
- `internal/httpserver/restore.go` — HTTP handlers (preview, execute, list)
  with strict JSON decoding and sentinel error mapping
- `internal/httpserver/router.go` — Route registration under
  `/api/v1/clusters/:cluster_id/restore-plans` and
  `/api/v1/restore-plans/:plan_id/execute`
- `internal/httpserver/audit.go` — `restore.preview` and `restore.execute`
  audit mappings
- `migrations/000023_isolated_workload_restore_rehearsal.up.sql` / `.down.sql`
  — `restore_plans` table with CHECK constraints
- `cmd/server/main.go` — Service wiring

### Frontend

- `frontend/src/types/kubernetes.ts` — `RestorePlan`, `RestoreQuarantineStatus`,
  `RestoreExecutionResult`, `RestoreRestoredItem`, `RestoreSourceSnapshot`,
  `RestoreRehearsalStatus` types
- `frontend/src/api/kubernetes.ts` — `listRestorePlans`,
  `previewRestorePlan`, `executeRestorePlan` functions
- `frontend/src/views/RestoreRehearsalView.vue` — Two-pane view with preview
  form, plan history, preview confirmation (with quarantine control summary,
  allowed/excluded kind tags, one-time token display, warning banner), and
  execution result (with restore phase, restored item count, restored item
  table, partial-failure banner)
- `frontend/src/components/ConsoleLayout.vue` — LifeBuoy icon and 恢复演练
  navigation entry under 分析与治理
- `frontend/src/router/index.ts` — `/restore-rehearsal` route role-gated to
  `system_admin` and `operations_admin`

## Fixed V1 Contract

- Operation: rehearse restore of a single M28-compatible Backup only
- Source Backup: must exist, be `Completed`, have exactly one included namespace
- Destination Namespace: server-generated `restore-{shortBackupName}-{actorID}`,
  must not pre-exist, must not collide with an active plan
- Quarantine controls: `quarantine-default-deny` NetworkPolicy (deny all
  ingress + egress) + `quarantine-zero-pods` ResourceQuota (`pods=0`,
  `services.loadbalancers=0`, `services.nodeports=0`)
- Creation order: Namespace → NetworkPolicy + ResourceQuota → Restore
- Restore CR: fixed `includedResources` allowlist (deployments, statefulsets,
  daemonsets, cronjobs, configmaps, secrets, serviceaccounts),
  `includeClusterResources=false`, `restorePVs=false`, `namespaceMapping` to
  destination
- Restore poll: 60 attempts × 5s = 5 min bounded wait
- Restored item projection: bounded to 100 items; Secrets by name only
- Plan TTL: 10 minutes
- Claim TTL (stale lock recovery): 1 minute
- Idempotency key: 8–128 chars
- No in-place restore, no PV restore, no cross-cluster restore, no
  cutover/rollback, no operator-supplied destination name, no
  operator-supplied include/exclude list

## Resource Kind Allowlist

| Allowed (restored) | Excluded (refused) |
|---|---|
| Deployment | Pod |
| StatefulSet | Job |
| DaemonSet | Service |
| CronJob | Ingress |
| ConfigMap | Endpoints, EndpointSlice |
| Secret | PersistentVolumeClaim, PersistentVolume |
| ServiceAccount | VolumeSnapshot, VolumeSnapshotContent |
| | ResourceQuota, LimitRange, NetworkPolicy |
| | RoleBinding, ClusterRoleBinding, Role, ClusterRole |
| | MutatingWebhookConfiguration, ValidatingWebhookConfiguration |

## Non-goals

- In-place restore into an existing Namespace
- PersistentVolume or PersistentVolumeClaim restore
- Cross-cluster restore
- Cutover, rollback, or traffic switching
- Operator-supplied destination Namespace name
- Operator-supplied resource include/exclude lists
- Quarantine Namespace auto-cleanup
- Restore deletion or update
- Browser terminal access to restored workloads
- Service/Ingress recreation (would hijack traffic)

## Verification

### L0 - Static and Focused

- `gofmt` on all changed Go files
- `go test ./internal/restore` — 40+ tests pass
- `go test ./...` — all backend packages pass
- `vue-tsc -b` — zero frontend type errors
- `vitest run` — 73 frontend tests pass

### L1 - Fast Repository Gate

- `scripts/verify-fast.ps1 -Scope All` — **PASSED** in 28.81s (backend=True
  frontend=True manifests=True)
- All backend packages pass (26 packages including new `restore` package with
  40+ tests)
- Frontend typecheck, Vitest (73 tests, 17 files), and build pass
- Compose and Kustomize contracts pass

### L2/L3 - Real-kind E2E

- Deferred: requires a kind cluster with Velero installed and a completed
  M28-compatible single-namespace Backup
- Default kind clusters do not have Velero installed, which is insufficient for
  restore acceptance

### Unit Test Coverage

| Test | Scenario |
|---|---|
| `TestValidateRequest_*` | Invalid cluster ID, empty backup name, empty namespace, too long name, success |
| `TestPreview_InvalidRequest` | Cluster ID < 1 |
| `TestPreview_VeleroNotInstalled` | Velero capability reports not installed |
| `TestPreview_VeleroCapabilityError` | Velero capability API error |
| `TestPreview_SourceBackupNotFound` | Backup returns ErrResourceNotFound |
| `TestPreview_SourceBackupIncomplete` | Backup phase not Completed |
| `TestPreview_SourceBackupScope` | Backup has multiple included namespaces |
| `TestPreview_DestinationExists` | Destination Namespace already exists |
| `TestPreview_DestinationCollision` | Active plan for same source backup |
| `TestPreview_RestoreNameConflict` | Velero Restore name already exists |
| `TestPreview_QuarantineDryRunFailed` | Dry-run create of quarantine resource fails |
| `TestPreview_RestoreDryRunFailed` | Dry-run create of Restore CR fails |
| `TestPreview_Success` | All preflight pass, plan persisted, token returned, token not persisted |
| `TestPreview_SaveError` | Repository Save error propagated |
| `TestExecute_EmptyID` | Empty plan ID |
| `TestExecute_EmptyToken` | Empty confirmation token |
| `TestExecute_ShortIdempotencyKey` | Idempotency key < 8 chars |
| `TestExecute_ClaimNotFound` | Claim returns ErrNotFound |
| `TestExecute_ClaimConfirmationInvalid` | Claim returns ErrConfirmationInvalid |
| `TestExecute_ClaimExpired` | Claim returns ErrExpired |
| `TestExecute_ClaimNoExecute` | Replay returns same plan without mutation |
| `TestExecute_BackupLookupError` | Source Backup API error during re-verify |
| `TestExecute_StaleSource` | Source Backup phase changed since preview |
| `TestExecute_DestinationExists` | Destination Namespace appeared since preview |
| `TestExecute_RestoreNameConflict` | Restore name taken since preview |
| `TestExecute_NamespaceCreationFails` | Namespace create error → ErrQuarantineFailed |
| `TestExecute_QuarantineControlsFail` | NetworkPolicy/ResourceQuota create fails |
| `TestExecute_RestoreCreationFails` | Restore CR create fails, quarantine retained |
| `TestExecute_PollTimeout` | Restore never reaches terminal phase |
| `TestExecute_PartialRestore` | Restore PartiallyFailed → ErrPartialRestore |
| `TestExecute_FailedPhase` | Restore Failed → ErrExecutionFailed |
| `TestExecute_Success` | Restore Completed, Complete called, items projected |
| `TestList_InvalidClusterID` | Cluster ID < 1 |
| `TestList_Success` | List returns plans |
| `TestGenerateDestinationNamespace_*` | Basic name, long name truncation |
| `TestGenerateRestoreName` | Name derivation |
| `TestSanitizeDNS1123` | Lowercase/digits/hyphens retained, others dropped |
| `TestExtractUID` | Valid JSON, nil, bad JSON |
| `TestNewIdentity` | UUID, token, hash non-empty, hash length |
| `TestQuarantineStatusJSON_Scan` | nil, bytes, invalid type |
| `TestExecutionResultJSON_Scan` | nil, bytes, invalid type |
| `TestExecutionResultJSON_EmptyItemsOmittedByOmitempty` | Empty items omitted |
| `TestExecutionResultJSON_NonEmptyItemsSerialized` | Non-empty items serialized |
| `TestAllowedKinds_ContainsExpectedSet` | Exactly 7 allowlisted kinds |
| `TestExcludedKinds_ContainsPodAndPVC` | Pod, PVC, NetworkPolicy, Service excluded |
| `TestResponse_Projection` | SourceSnapshot, DestinationName, AllowedKinds, RequestedBy |

## Security

- Authorization: mutations restricted to system/operations admin
- Audit: `restore.preview` and `restore.execute` events recorded
- Confirmation: one-time token, SHA-256 hash stored, constant-time comparison
- Idempotency: 8–128 char key, Claim transaction with SELECT FOR UPDATE
- Error boundary: `safeError` prevents K8s API details from leaking
- Fixed scope: no operator-supplied destination name, no operator-supplied
  include/exclude list, no PV restore, no in-place restore
- Interface bounds: `KubernetesSource` exposes only read methods and
  `CreateResource` — no delete, update, patch, or Secret-value path
- Quarantine: default-deny NetworkPolicy + zero-Pod ResourceQuota established
  before Restore; retained on failure for inspection
- Token/idempotency key never persisted in audit logs

## Files Changed

### Backend

- `backend/internal/restore/model.go` — domain models, sentinel errors, JSONB helpers
- `backend/internal/restore/repository.go` — GORM repository with Claim/Complete/Fail/ActiveBySource
- `backend/internal/restore/service.go` — Preview, Execute, List, poll, project, manifest builders
- `backend/internal/restore/service_test.go` — 40+ unit tests
- `backend/internal/kubernetes/service.go` — VeleroRestore, VeleroRestoreExists methods
- `backend/internal/httpserver/restore.go` — HTTP handlers
- `backend/internal/httpserver/router.go` — route registration + Options field
- `backend/internal/httpserver/audit.go` — audit mappings
- `backend/cmd/server/main.go` — service wiring
- `backend/migrations/000023_isolated_workload_restore_rehearsal.up.sql` — schema migration
- `backend/migrations/000023_isolated_workload_restore_rehearsal.down.sql` — rollback

### Frontend

- `frontend/src/types/kubernetes.ts` — restore types
- `frontend/src/api/kubernetes.ts` — restore API functions
- `frontend/src/views/RestoreRehearsalView.vue` — two-pane restore rehearsal view
- `frontend/src/components/ConsoleLayout.vue` — navigation entry
- `frontend/src/router/index.ts` — `/restore-rehearsal` route

### Documentation

- `docs/adr/0047-isolated-workload-restore-rehearsal.md` — architecture decision
- `docs/changes/2026-07-30-m31-isolated-workload-restore-rehearsal.md` — this document
