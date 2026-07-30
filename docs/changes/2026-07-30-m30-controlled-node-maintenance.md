# M30: Controlled Node Maintenance

- Date: 2026-07-30
- Status: Accepted (unit/contract gates and disposable two-worker real-kind E2E passed)
- ADR: [0046-controlled-node-maintenance.md](../adr/0046-controlled-node-maintenance.md)

## Summary

M30 adds controlled Node maintenance through a fixed-scope, two-phase
confirmation workflow. Operators with system/operations admin roles can cordon,
uncordon, and drain a single worker Node with PDB-aware bounded eviction,
one-time confirmation tokens, idempotent execution, and full audit trails.
Force deletion, PDB bypass, `emptyDir` deletion, arbitrary Pod delete, and
browser terminals remain explicitly prohibited.

## Product Outcome

An operations administrator can:

1. Open the Node Maintenance page under 分析与治理
2. Select cordon, uncordon, or drain and enter the target worker Node name
3. Preview triggers precondition checks (control-plane rejection, already-cordoned,
   already-uncordoned, not-cordoned-before-drain) and classifies all resident
   Pods into retained / evictable / blocking
4. For cordon/uncordon, preview also runs a server-side dry-run Node patch
5. Review the confirmation token, Pod classification table (with PDB evidence),
   and plan expiration
6. Confirm with the one-time token and an Idempotency-Key header
7. For drain, execution evicts eligible Pods with bounded concurrency (2),
   per-Pod timeout (30s), and total deadline (10 min)
8. Partial drain failure leaves the Node cordoned and records per-Pod outcomes

## Implementation

### Backend

- `internal/maintenance/model.go` — Plan, PreviewEvidence, PodEvidence,
  PodOutcome, ExecutionResult types with JSONB GORM helpers; sentinel errors
  for every failure class (control-plane, already-cordoned, not-cordoned,
  unmanaged Pod, emptyDir Pod, PDB unavailable, stale target, partial drain)
- `internal/maintenance/repository.go` — GORM repository with Claim/Complete/Fail
  (SELECT FOR UPDATE, constant-time token compare, stale lock recovery)
- `internal/maintenance/service.go` — Preview (preconditions + dry-run +
  evidence collection + Pod classification), Execute (claim + re-verify +
  cordon/uncordon/drain), List, evictPods (bounded concurrency), evictOne
  (per-Pod timeout), classifyPod, evidenceMatches, classifyBlockError,
  buildNodePatch, buildEvictionBody, newIdentity, safeError
- `internal/maintenance/service_test.go` — 40+ unit tests covering all paths
- `internal/httpserver/node_maintenance.go` — HTTP handlers (preview, execute,
  list) with strict JSON decoding and sentinel error mapping
- `internal/httpserver/router.go` — Route registration under
  `/api/v1/clusters/:cluster_id/maintenance-plans` and
  `/api/v1/maintenance-plans/:plan_id/execute`
- `internal/httpserver/audit.go` — `maintenance.preview` and
  `maintenance.execute` audit mappings
- `migrations/000022_controlled_node_maintenance.up.sql` / `.down.sql` —
  `maintenance_plans` table with CHECK constraints
- `cmd/server/main.go` — Service wiring

### Frontend

- `frontend/src/types/kubernetes.ts` — `MaintenancePlan`,
  `MaintenancePreviewEvidence`, `MaintenancePodEvidence`,
  `MaintenanceExecutionResult`, `MaintenancePodOutcome`,
  `MaintenanceAction`, `MaintenanceStatus`, `PodClassification` types
- `frontend/src/api/kubernetes.ts` — `listMaintenancePlans`,
  `previewMaintenancePlan`, `executeMaintenancePlan` functions
- `frontend/src/views/NodeMaintenanceView.vue` — Two-pane view with preview
  form, plan history, preview confirmation (with Pod classification table and
  one-time token display), and execution result (with per-Pod outcomes and
  partial-failure banner)
- `frontend/src/components/ConsoleLayout.vue` — Hammer icon and 节点维护
  navigation entry under 分析与治理
- `frontend/src/router/index.ts` — `/node-maintenance` route role-gated to
  `system_admin` and `operations_admin`

## Fixed V1 Contract

- Actions: `cordon`, `uncordon`, `drain` only
- Target: single worker Node (control-plane rejected via
  `node-role.kubernetes.io/control-plane` or `.../master` label)
- Node name: trimmed, 1–253 chars
- Resident Pod inspection limit: 100 (`maxResidentPods`)
- Evictable Pod limit per drain: 20 (`maxEvictablePods`)
- Eviction concurrency: 2 (`evictionConcurrency`)
- Per-eviction timeout: 30 seconds (`evictionTimeout`)
- Plan TTL: 10 minutes
- Claim TTL (stale lock recovery): 1 minute
- Idempotency key: 8–128 chars
- Eviction subresource: `POST /api/v1/namespaces/{ns}/pods/{name}/eviction`
  with `policy/v1` `Eviction` body
- No force deletion, no `--disable-eviction`, no grace-period override, no
  `emptyDir` deletion, no PDB bypass, no arbitrary Pod delete

## Pod Classification Rules

| Classification | Criteria |
|---|---|
| `retained` | OwnerKind = `DaemonSet` OR mirror Pod (`kubernetes.io/config.mirror` annotation) |
| `blocking` | Unknown/unmanaged owner, real `spec.volumes[].emptyDir`, no selector-matching PDB, zero-disruption matching PDB, or unavailable PDB evidence |
| `evictable` | Reviewed controller owner, no `emptyDir`, and every selector-matching PDB has `disruptionsAllowed > 0` |

Pod volume projections expose real volume types. PDB association decodes the
Kubernetes `metav1.LabelSelector` and uses the official selector
implementation; Namespace co-location alone never associates a PDB.

## Non-goals

- Multi-node bulk drain
- Browser terminal access to Nodes
- Force deletion or `--disable-eviction`
- PDB bypass or grace-period override
- `emptyDir` deletion
- Arbitrary Pod delete
- Auto-uncordon after failed drain
- Node deletion or Node label mutation
- Drain scheduling or recurring maintenance windows

## Verification

### L0 - Static and Focused

- `gofmt` on all changed Go files
- `go test ./internal/maintenance` — 40+ tests pass
- `go test ./...` — all backend packages pass
- `vue-tsc -b` — zero frontend type errors
- `vitest run` — 73 frontend tests pass

### L1 - Fast Repository Gate

- `scripts/verify-fast.ps1 -Scope All` — **PASSED** in 35.01s
- All backend packages pass (including new `maintenance` package with 40+ tests)
- Frontend typecheck, Vitest (73 tests, 17 files), and build pass
- Compose and Kustomize contracts pass

### L2/L3 - Real-kind E2E

- `scripts/e2e-m30-node-maintenance-kind.ps1` — **PASSED**
- Evidence: `.artifacts/m30-node-maintenance-kind/summary.json`
- A disposable two-worker Kubernetes v1.34.0 kind cluster proved cordon and
  same-key replay, one PDB-authorized eviction, real emptyDir rejection,
  zero-disruption PDB rejection, explicit uncordon and complete cleanup
- Empty-value control-plane/master label keys are covered by focused regression
  tests and rejected independently of their label values

### Unit Test Coverage

| Test | Scenario |
|---|---|
| `TestPreview_InvalidRequest` | Empty action, invalid action, empty node name, too long node name |
| `TestPreview_InvalidClusterID` | Cluster ID < 1 |
| `TestPreview_NodeLookupError` | Kubernetes API unavailable |
| `TestPreview_ControlPlaneRejected` | Control-plane label present |
| `TestPreview_CordonAlreadyCordoned` | Cordon on already-cordoned Node |
| `TestPreview_UncordonAlreadyUncordoned` | Uncordon on schedulable Node |
| `TestPreview_DrainNotCordoned` | Drain without prior cordon |
| `TestPreview_DrainBlockingUnmanagedPod` | Unmanaged Pod blocks drain |
| `TestPreview_DrainBlockingEmptyDirPod` | emptyDir Pod blocks drain |
| `TestPreview_DrainBlockingPDBUnavailable` | Managed Pod without PDB evidence |
| `TestPreview_DrainPDBLookupError` | PDB API unavailable |
| `TestPreview_DrainTooManyEvictable` | Evictable count exceeds 20 |
| `TestPreview_CordonSuccess` | Dry-run patch, token hash stored, plaintext not persisted |
| `TestPreview_DryRunPatchFailure` | Dry-run patch error propagated |
| `TestPreview_DrainSuccess` | DaemonSet retained, managed Pod evictable |
| `TestExecute_InvalidInputs` | Empty plan ID, empty token, short/long idempotency key |
| `TestExecute_ClaimError` | Claim error propagated |
| `TestExecute_ClaimNoExecute` | Successful replay returns same plan |
| `TestExecute_NodeUIDMismatch` | Stale target on UID change |
| `TestExecute_NodeLookupError` | Node API down during execute |
| `TestExecute_CordonSuccess` | Non-dry-run patch, Complete called |
| `TestExecute_UncordonSuccess` | Non-dry-run patch, unschedulable_now=false |
| `TestExecute_CordonPatchFailure` | Patch error → Fail called |
| `TestExecute_DrainStalePodEvidence` | Pod set changed since preview |
| `TestExecute_DrainSuccess` | One eviction call, evicted_count=1 |
| `TestExecute_DrainPartialFailure` | Eviction rejected → partial=true, Node stays cordoned |
| `TestExecute_DrainCollectEvidenceError` | Pods API down during execute |
| `TestExecute_UnknownAction` | Invalid action in plan |
| `TestList_InvalidClusterID` | Cluster ID < 1 |
| `TestList_DelegatesToRepository` | List returns plans |
| `TestClassifyPod_*` | DaemonSet retained, mirror retained, unmanaged blocking, emptyDir blocking, no-PDB blocking, with-PDB evictable |
| `TestEvidenceMatches` | UID mismatch, count mismatch, missing Pod, UID change, match |
| `TestClassifyBlockError` | Unmanaged, emptyDir, PDB unavailable, no-blocking fallback |
| `TestBuildNodePatch` | JSON shape with spec.unschedulable |
| `TestBuildEvictionBody` | policy/v1 Eviction with metadata |
| `TestNewIdentity` | UUID length, token non-empty, hash 32 bytes, uniqueness |
| `TestPreviewEvidenceJSON_RoundTrip` | Value/Scan round-trip |
| `TestExecutionResultJSON_RoundTrip` | Value/Scan round-trip |
| `TestIsControlPlane` | Empty/control-plane/master/worker labels |
| `TestEvictablePods` | Filter to evictable only |
| `TestSafeError` | Non-empty message for generic error |
| `TestValidateRequest_TrimsWhitespace` | Trimmed valid request passes |

## Security

- Authorization: mutations restricted to system/operations admin
- Audit: `maintenance.preview` and `maintenance.execute` events recorded
- Confirmation: one-time token, SHA-256 hash stored, constant-time comparison
- Idempotency: 8–128 char key, Claim transaction with SELECT FOR UPDATE
- Error boundary: `safeError` prevents K8s API details from leaking
- Fixed scope: no arbitrary YAML, no client-controlled patch content
- Interface bounds: `KubernetesSource` exposes only Node/Pod/PDB read, Node
  patch, and Eviction create — no Pod delete, no Node delete, no Secret access
- Token/idempotency key never persisted in audit logs

## Files Changed

### Backend

- `backend/internal/maintenance/model.go` — domain models, sentinel errors, JSONB helpers
- `backend/internal/maintenance/repository.go` — GORM repository with Claim/Complete/Fail
- `backend/internal/maintenance/service.go` — Preview, Execute, List, Pod classification, eviction
- `backend/internal/maintenance/service_test.go` — 40+ unit tests
- `backend/internal/httpserver/node_maintenance.go` — HTTP handlers
- `backend/internal/httpserver/router.go` — route registration
- `backend/internal/httpserver/audit.go` — audit mappings
- `backend/cmd/server/main.go` — service wiring
- `backend/migrations/000022_controlled_node_maintenance.up.sql` — schema migration
- `backend/migrations/000022_controlled_node_maintenance.down.sql` — rollback

### Frontend

- `frontend/src/types/kubernetes.ts` — maintenance types
- `frontend/src/api/kubernetes.ts` — maintenance API functions
- `frontend/src/views/NodeMaintenanceView.vue` — two-pane maintenance view
- `frontend/src/components/ConsoleLayout.vue` — navigation entry
- `frontend/src/router/index.ts` — `/node-maintenance` route

### Documentation

- `docs/adr/0046-controlled-node-maintenance.md` — architecture decision
- `docs/changes/2026-07-30-m30-controlled-node-maintenance.md` — this document
