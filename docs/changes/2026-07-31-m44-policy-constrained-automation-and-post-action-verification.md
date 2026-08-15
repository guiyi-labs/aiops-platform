# M44: Policy-Constrained Automation And Post-Action Verification

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0059](../adr/0059-policy-constrained-automation-and-post-action-verification.md)
- Fast gate: 67.17s (30 backend packages including `automation`,
  81 frontend tests/18 files, Compose/Kustomize contracts)

## Summary

Introduced the safe-execution ceiling of the AIOps Intelligence Plane. M44A
defines the data model and migration 000033; M44B implements the deterministic
policy gate evaluator (8 gates: `uid_rv_recheck`, `scope`, `pdb_blast_radius`,
`slo_burn`, `freeze_window`, `concurrent_plans`, `attempt_cap`,
`rollback_point`) with action-specific gate sets and preview/recheck
semantics; M44C implements the post-action verifier with evidence comparison
(improved/unchanged/worse/insufficient), outcome classification
(effective/ineffective/failed/unknown) and the server-owned rollback
contract; M44D wires 10 HTTP routes and OpenAPI schemas; M44E adds 66 unit
tests (45 automation package + 21 HTTP handler), ADR 0059 and this change
record.

The action plan lifecycle is `draft → previewed → approved → executing →
succeeded/failed → verified` (plus terminal `expired`/`cancelled`). Every
transition is a repository method that re-checks state under a row lock and
stamps audit metadata. Execution is idempotent via a confirmation token
(issued at preview, hashed at rest) plus an idempotency key (operator-supplied
UUID); two workers and replay produce one business side effect. Human approval
is the default (`L2`); rollback and image-update actions require four-eyes
approval (requester cannot self-approve, enforced at the DB layer). Policy
gates are rechecked immediately before execute — a stale target UID/RV, an
opened freeze window, an exhausted PDB budget, or an exceeded attempt cap all
fail closed.

Post-action verification captures a pre-snapshot at execute time and a
post-snapshot after a configurable cooldown (default 300s, min 60s). The
verifier compares SLO state and resource state deterministically; missing
evidence on either side yields `ComparisonInsufficient` and
`VerificationStatusUnknown` — the verifier never auto-resolves a diagnosis
from missing data. When verification yields `ineffective` or `failed`, the
service evaluates a rollback contract: if a safe rollback exists (target
unchanged, no freeze, no concurrent plan, attempt cap not exceeded), a
rollback plan is drafted automatically; otherwise the case escalates to a
human with the reason recorded.

No public API contract was broken beyond the ten new `aiops/automation`
routes documented in OpenAPI.

## Changes

### M44A — Data model and migration

#### New files

- `backend/internal/automation/model.go`: defines `AutomationVersion =
  "1.0"`, `VerifierVersion = "1.0"`, `AutomationLevel` (4 values:
  L0/L1/L2/L3), `PlanStatus` (9 values: draft/previewed/approved/executing/
  succeeded/failed/expired/cancelled/verified), `ApprovalType` (single,
  four_eyes), `GateStatus` (passed/failed/skipped), `GateCode` (8 codes),
  `PolicyGate`, `ActorRef`, `TargetRef`, `OperationParameters`,
  `OperationChange`, `ActionPlan`, `ActionPlanResponse`,
  `VerificationStatus` (5 values: pending/effective/ineffective/failed/
  unknown), `EvidenceComparison` (4 values: improved/unchanged/worse/
  insufficient), `EvidenceSnapshot`, `SLOSnapshot`, `ActionVerification`,
  `ActionPlanFilter`, `ActionPlanListResponse`, `QualityReport`, and the
  bound constants (`MaxAttemptsPerTarget` = 5, `AttemptWindowSeconds` =
  3600, `MaxPolicyGatesPerPlan` = 16, `MaxPlansPerCase` = 8,
  `MaxRunbookIDLength` = 128, `DefaultPlanTTLSeconds` = 600,
  `DefaultClaimTTLSeconds` = 60, `DefaultCooldownSeconds` = 300,
  `MinCooldownSeconds` = 60).
- `backend/internal/automation/errors.go`: 17 sentinel errors covering
  invalid input, runbook eligibility, operation parameters, target
  staleness, lifecycle state, self-approval, policy gate failure,
  idempotency, execution failure, and verification state.
- `backend/migrations/000033_policy_constrained_automation.up.sql`:
  creates `action_plans` and `action_verifications` tables with CHECK
  constraints on status/approval_type/evidence_comparison/
  verification_status, the four-eyes distinctness CHECK, the
  missing-evidence → insufficient+unknown CHECK, partial unique indexes
  `uq_action_plans_active` (one non-terminal plan per `plan_key`) and
  `uq_action_verifications_active` (one pending verification per plan),
  plus FKs to `correlation_cases(id)` and `ai_investigations(id)` ON
  DELETE SET NULL.
- `backend/migrations/000033_policy_constrained_automation.down.sql`:
  drops the tables, indexes and FKs in the correct order.

### M44B — Policy gate evaluator, catalog, repository

#### New files

- `backend/internal/automation/catalog.go`: defines
  `RunbookDescriptor` and the compiled `catalog` map with 2 V1
  executable runbooks: `rollback_last_rollout` (`deployment.rollback`,
  four-eyes) and `rollout_restart_pods` (`deployment.rollout_restart`,
  single). `LookupRunbook` fails closed; `AllRunbooks` returns the
  catalog. The catalog mirrors the M43 aiinvestigator catalog but only
  includes runbooks with non-empty `ActionCode` (advisory-only runbooks
  cannot be materialized into action plans).
- `backend/internal/automation/gates.go`: defines `GateContext`
  (Now/PreviewSnapshot/CurrentSnapshot/ScopeDecision/PDBEvidence/
  BlastRadius/SLOBurnState/FreezeWindow/ConcurrentPlanCount/
  AttemptCount/AttemptMax/RollbackPoint), `GateEvaluator` (stateless,
  pure), `RequiredGates(actionCode)` returning the action-specific
  gate set, `Evaluate` (preview), `Recheck` (execute-time, stamps
  `Rechecked = true`), `AllPassed` (skipped is non-failure),
  `FailedGates`. The 8 per-gate evaluators implement fail-closed
  semantics: `uid_rv_recheck` compares preview vs current UID/RV;
  `scope` rechecks M35 authorization; `pdb_blast_radius` skips when PDB
  evidence is unavailable; `slo_burn` allows a rollback during a
  breached window (the remedy) but fails image_update/scale_down;
  `freeze_window` fails when a freeze is active; `concurrent_plans`
  fails when another non-terminal plan targets the same UID;
  `attempt_cap` fails when the rolling-window count exceeds the cap;
  `rollback_point` fails when no non-current ReplicaSet revision
  exists.
- `backend/internal/automation/repository.go`: defines the `Repository`
  interface (SavePlan/GetPlan/GetPlanForExecute/ListPlans/
  CountAttemptsSince/CountConcurrentPlans/MarkPreviewed/Approve/Claim/
  Complete/Fail/MarkVerified/Cancel/ExpireStale/SaveVerification/
  GetVerification/GetVerificationByPlan/UpdateVerification),
  `GormRepository` (with `actionPlanRow`/`actionVerificationRow` GORM
  models, `JSONB` wrapper, `clause.Locking{Strength: "UPDATE"}` for
  Claim), `NopRepository`, and 5 lifecycle sentinel errors
  (`ErrPlanNotFound`, `ErrVerificationNotFound`,
  `ErrConfirmationInvalid`, `ErrExpired`, `ErrInProgress`,
  `ErrAlreadyExecuted`, `ErrNotApproved`). `Claim` mirrors the
  remediation/maintenance pattern: idempotent replay returns the
  recorded outcome without re-executing; stale `executing` rows past
  `claimTTL` are reclaimable.

### M44C — Post-action verifier

#### New files

- `backend/internal/automation/verifier.go`: defines `EvidenceProvider`
  interface (CapturePreSnapshot/CapturePostSnapshot),
  `NopEvidenceProvider`, `Verifier` (pure given plan + snapshots),
  `VerifierOption` (`WithVerifierProvider`/`WithVerifierNow`/
  `WithVerifierCooldown`), `CreateVerification` (captures pre-snapshot
  at execute time, computes `verification_key` = SHA-256 over (plan_id
  + verifier_version + evidence_hash)), `Evaluate` (captures
  post-snapshot, runs `compareEvidence` and `classifyStatus`).
  `compareEvidence` is deterministic: SLO state transitions take
  precedence (healthy > burning_slow > burning_fast > breached);
  resource state (replicas/available_replicas/image/suspended) is
  compared for actions without SLO evidence or when SLO state is
  unchanged. Missing evidence yields `ComparisonInsufficient` and
  `VerificationStatusUnknown`. `classifyStatus` maps (comparison,
  missing, action) to effective/ineffective/failed/unknown. Helpers:
  `sloStateRank`, `sloBoundAction`, `resourceInt`/`resourceStr`/
  `resourceBool`, `hashSnapshot`, `computeVerificationKey`.

### M44D — Service, HTTP routes, OpenAPI

#### New files

- `backend/internal/automation/service.go`: defines `Service` (the only
  writer), `CaseReader`/`NopCaseReader`, `CaseContext`,
  `KubernetesSource` interface, `ServiceOption` (`WithNow`/
  `WithPlanTTL`/`WithClaimTTL`/`WithCooldown`/`WithEvidenceProvider`),
  `CreatePlanInput`. `CreatePlan` validates runbook + eligibility,
  materializes operation parameters from the case context, captures
  the target snapshot, computes `plan_key` = SHA-256 over (case_id +
  runbook_id + target_uid + automation_version), issues a
  confirmation token (32 bytes, base64; hashed at rest), and persists
  a draft. `Preview` refreshes the snapshot, evaluates gates, stores
  results, transitions to previewed. `Approve` enforces four-eyes
  distinctness. `Execute` rechecks gates, takes an idempotent claim,
  builds and applies the Kubernetes patch, transitions to
  succeeded/failed, and schedules verification. `Verify` runs the
  verifier, evaluates the rollback contract on
  ineffective/failed outcomes, and marks the plan verified. `Cancel`
  transitions non-terminal plans to cancelled. `ListPlans`/`GetPlan`/
  `GetVerification` are read paths.
- `backend/internal/httpserver/automation.go`: defines
  `automationHandler` with 10 handlers: `listRunbooks`, `listPlans`,
  `createPlan`, `getPlan`, `previewPlan`, `approvePlan`,
  `executePlan`, `cancelPlan`, `verifyPlan`, `getVerification`.
  `isValidPlanID` enforces UUID v4 format. `writeError` maps 25
  sentinel errors to stable HTTP status codes (404 for not-found, 409
  for state conflicts, 410 for expired, 403 for confirmation/self-
  approval failures, 400 for invalid input, 502 for execution
  failures, 503 for disabled). `operationParametersFromPlan` and
  `buildChangePreview` produce the operator-facing diff. Actor
  identity is derived from `requestctx.MetadataFrom`. The Idempotency-
  Key header is read by `executePlan`.

#### Modified files

- `backend/internal/httpserver/router.go`: adds `AutomationService`
  to `Options`, registers the 10 automation routes under
  `/api/v1/aiops/automation` when the service is non-nil. Write
  routes (create/preview/approve/execute/cancel/verify) require
  `rolesSystemOpsAdmin`; read routes (list runbooks, list/get plan,
  get verification) require only authentication.
- `backend/internal/httpserver/openapi_route_test.go`: adds
  `AutomationService` wiring with `NopRepository` + `NopCaseReader`
  so route registration is covered by the OpenAPI parity test.
- `docs/api/openapi.yaml`: adds 9 automation routes and 9 schemas
  (`AutomationRunbookList`, `AutomationRunbook`,
  `CreateActionPlanRequest`, `ApproveActionPlanRequest`,
  `ExecuteActionPlanRequest`, `ActionPlanListResponse`,
  `ActionPlanResponse`, `ActionVerification`, `PolicyGate`) under the
  `automation` tag.

### M44E — Unit tests, ADR, docs

#### New files

- `backend/internal/automation/gates_test.go`: 11 tests —
  `TestRequiredGates` (per-action gate sets, 6 subtests),
  `TestEvaluateUIDRV` (5 subtests: missing preview, target gone, UID
  changed, RV changed, match), `TestEvaluateScope` (allowed/denied/
  empty reason), `TestEvaluateFreezeWindow` (active/inactive),
  `TestEvaluateConcurrentPlans` (0/1+), `TestEvaluateAttemptCap`
  (under/over/default), `TestEvaluateRollbackPoint` (no revision/
  current/valid), `TestEvaluateSLOBurn` (breached+rollback/
  breached+image_update/burning_fast/healthy/unavailable),
  `TestEvaluatePDBBlastRadius` (unavailable/exceeds cap/negative
  allowed/within), `TestAllPassed` (all passed/one failed/skipped ok),
  `TestRecheck` (stamps Rechecked=true, preserves order).
- `backend/internal/automation/verifier_test.go`: 17 tests —
  `TestCreateVerification` (success/clamp cooldown/propagate error),
  `TestEvaluatePostSnapshotCaptureFailed` (yields failed +
  missing_evidence), `TestCompareEvidenceSLOImproved`,
  `TestCompareEvidenceSLOWorse`, `TestCompareEvidenceMissingPre`,
  `TestCompareEvidenceMissingPost`,
  `TestCompareEvidenceResourceScaleImproved`,
  `TestCompareEvidenceResourceScaleUnchanged`,
  `TestCompareEvidenceRolloutRestartImproved`,
  `TestCompareEvidenceRolloutRestartUnchangedWhenPodsNotReady`,
  `TestClassifyStatus` (improved→effective, worse→ineffective,
  insufficient→unknown, missing→unknown),
  `TestHashSnapshot` (determinism), `TestSloStateRank` (order),
  `TestSloBoundAction` (deployment.* are SLO-bound, cronjob is not),
  `TestVerifierEvaluateEndToEndMissingEvidenceIsUnknown`.
- `backend/internal/automation/service_test.go`: 17 tests —
  `TestCreatePlanRejectsEmptyRunbook`,
  `TestCreatePlanRejectsUnknownRunbook`,
  `TestCreatePlanRejectsAdvisoryRunbook`,
  `TestCreatePlanRejectsIneligibleRunbook`,
  `TestCreatePlanRejectsWhenCaseNotFound`,
  `TestApproveRejectsNonPreviewed`,
  `TestApproveRejectsSelfApprovalFourEyes`,
  `TestApproveAcceptsDifferentApproverFourEyes`,
  `TestExecuteRejectsEmptyConfirmationToken`,
  `TestExecuteRejectsInvalidIdempotencyKey`,
  `TestVerifyRejectsNonVerifiablePlan`,
  `TestCancelDisabledService`, `TestApprovalTypeFor` (rollback→four_eyes,
  others→single), `TestComputePlanKey` (determinism + version bump),
  `TestListPlansReturnsResponseWithTruncated`,
  `TestListPlansNotTruncatedWhenItemsEqualTotal`,
  `TestListPlansNotTruncatedWhenEmpty`. Uses `fakeRepository`
  (in-memory), `fakeCaseReader`, `stubKubernetesSource`.
- `backend/internal/httpserver/automation_test.go`: 21 handler tests
  — list runbooks 200/503, list plans 200/invalid limit/invalid
  case_id, create plan missing fields/unknown runbook/case not-found/
  ineligible runbook, get plan invalid id/404, preview invalid id/404,
  approve invalid id, execute missing confirmation, cancel 404, verify
  404, get verification 404, writeError maps 25 sentinel errors (25
  subtests), isValidPlanID (valid/invalid), buildChangePreview
  (scale/rollback/image_update/suspend).
- `docs/adr/0059-policy-constrained-automation-and-post-action-verification.md`:
  8 decisions covering lifecycle with deterministic transitions, gate
  evaluator with action-specific gate sets, confirmation token +
  idempotency key + lease, human approval with four-eyes for rollback,
  post-action verifier with evidence comparison, server-owned rollback
  contract, bounded HTTP surface, audit and failure persistence.
- `docs/changes/2026-07-31-m44-policy-constrained-automation-and-post-action-verification.md`:
  this change record.

#### Modified files

- `docs/roadmap.md`: M44 status section added.
- `docs/testing/test-matrix.md`: M44 addendum with 66 test counts.
- `docs/development-handoff.md`: updated to M44 baseline.
- `CHANGELOG.md`: M44 entry.

## Test counts

| Package | Tests |
|---|---|
| `internal/automation` (gates) | 11 |
| `internal/automation` (verifier) | 17 |
| `internal/automation` (service) | 17 |
| `internal/httpserver` (automation handler) | 21 |
| **Total** | **66** top-level (plus 50+ subtests) |

## Deferred

- Background verification worker (cooldown-based scheduling of
  `Verifier.Evaluate` for pending verifications)
- Stale `executing` plan reclaim worker (auto-reclaim after `claimTTL`)
- `ExpireStale` background worker (TTL-based expiration of awaiting
  plans)
- Real Kubernetes integration tests for the patch path
  (`PatchDeployment`/`PatchCronJob`/`RolloutHistory` via `client-go`)
- Real Prometheus/SLO integration for the `EvidenceProvider`
- Real PostgreSQL integration test for `GormRepository` (requires full
  Compose stack)
- Real-kind E2E for the preview → approve → execute → verify path
- Frontend UI (plan list, plan detail with gate timeline, verification
  panel, runbook stepping)
- `L3` pre-authorized automatic execution level (requires separate ADR
  with shadow mode, narrow policy, canary, kill switch)
- Rollback-plan auto-execution path (M44 drafts the rollback plan; a
  future milestone may auto-execute it under stricter gates)
- M42 `ActionCandidate` → M44 plan auto-suggestion (currently the
  operator picks the runbook by ID)
