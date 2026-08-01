# ADR 0059: Policy-Constrained Automation And Post-Action Verification (M44)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M44
- Supersedes: none
- Related: ADR 0058 (cited and evaluated AI investigator), ADR 0057
  (multi-signal correlation and deterministic RCA), ADR 0056 (SLO, error
  budget and impact), ADR 0053 (capability plane adapters), ADR 0024
  (resource-originated controlled operations), ADR 0023 (confirmed
  idempotent remediation), ADR 0008 (sanitized append-only audit trail)

## Context

M43 delivered a cited, advisory-only AI investigator that recommends
server-owned runbooks bounded by the M42 Action Catalog. The optimization
plan (`docs/kubesphere-optimization-plan.md` §16) requires M44 to close
the loop: take an eligible runbook, materialize it into an executable
action plan, gate it through deterministic policy checks, require human
approval, execute idempotently against the Kubernetes source, and verify
the outcome against captured evidence.

The platform already has a per-resource controlled-operations path
(`remediation`, ADR 0024) for direct operator-initiated patches, and a
confirmed idempotent remediation pattern (ADR 0023) that uses request
keys to deduplicate side effects. M44 is **not** a general-purpose
automation runner: it is a tightly-scoped, policy-constrained plane that
turns M43 recommendations into auditable, reversible, verifiable actions
on Deployments and CronJobs.

The key constraints:

1. **Policy gates are deterministic and rechecked.** Every plan carries
   a fixed set of policy gates evaluated at preview time and *re-evaluated*
   immediately before execution. A gate that passed at preview may fail at
   recheck (target UID/RV changed, freeze window opened, PDB budget
   exhausted, attempt cap reached); a single failed gate fails the plan.
   The recheck uses the same evaluator and the same `GateContext` shape,
   so identical inputs produce identical decisions.
2. **Human approval is the default.** All actions default to `L2`
   (human-in-the-loop). Four-eyes approval is required for irreversibility-
   adjacent actions (rollback). The confirmation token issued at preview
   must be supplied at execute time, and self-approval is forbidden for
   four-eyes plans. There is no `L4` (fully autonomous) path in M44.
3. **Execution is idempotent and lease-bound.** `Claim` atomically
   transitions `approved → executing` under a row lock and stamps an
   idempotency key. Re-execute with the same key returns the recorded
   outcome; re-execute with a different key after `succeeded`/`failed`
   returns `ErrAlreadyExecuted`. Stale `executing` rows are reclaimable
   after a bounded TTL.
4. **Post-action verification is evidence-bound.** The verifier captures
   a pre-snapshot at execute time and a post-snapshot after a cooldown.
   It compares SLO state and resource state and classifies the outcome
   (improved, unchanged, worse, insufficient). Missing evidence never
   resolves a diagnosis automatically — `insufficient` yields
   `VerificationStatusUnknown`, not `effective`.
5. **Rollback is server-owned and safe-only.** When verification yields
   `ineffective` or `failed`, the service evaluates a rollback contract:
   if a safe rollback exists (target unchanged since execute, no freeze,
   no concurrent plan, attempt cap not exceeded), a rollback plan is
   drafted automatically; otherwise the case escalates to a human with
   the reason recorded.
6. **Attempt cap prevents runaway loops.** A rolling window caps the
   number of non-cancelled plans per target UID. The cap is enforced by
   the `attempt_cap` gate at preview *and* recheck, so a flapping target
   cannot be hammered by repeated automation.
7. **Audit is append-only.** Every transition (draft → previewed →
   approved → executing → succeeded/failed → verified) is stamped with
   actor, time, idempotency key and gate results. The full gate history
   is preserved as JSONB on the plan; recheck gates are stored alongside
   preview gates. Verification rows are append-only within a plan.
8. **Failure does not lose state.** On execution failure the plan is
   marked `failed` with a safe (redacted) error message; a pending
   verification is still created so the operator can see the pre-snapshot
   and reason about what went wrong. On verification capture failure the
   verification is `failed` with `missing_evidence = true`; the plan
   retains its execution status.

## Decision

### 1. Action plan lifecycle with deterministic transitions

`ActionPlan` carries a fixed state machine:
`draft → previewed → approved → executing → (succeeded | failed) → verified`,
plus terminal `expired` and `cancelled`. Each transition is a repository
method that re-checks the current state under a row lock and stamps
`updated_at`. The HTTP layer never writes state directly; it calls the
service, which calls the repository.

- `SavePlan` (draft) — creates the plan with `status = draft`, a fresh
  UUID, a SHA-256 `plan_key` over (case_id + runbook_id + target_uid +
  automation_version), and a cryptographically random confirmation token
  (hashed at rest).
- `MarkPreviewed` — stores the previewed gate set and the target
  snapshot; transitions `draft → previewed`.
- `Approve` — transitions `previewed → approved`; enforces four-eyes
  distinctness at the DB layer.
- `Claim` — transitions `approved → executing` under a row lock and
  stamps the idempotency key; idempotent re-claim returns the recorded
  outcome without re-executing.
- `Complete` / `Fail` — terminal execution transitions.
- `MarkVerified` — transitions `succeeded/failed → verified` and stores
  the verification summary.

`plan_key` deduplicates drafts for the same (case, runbook, target,
version); a second `SavePlan` for the same key returns the existing draft.

### 2. Policy gate evaluator with action-specific gate sets

`GateEvaluator` is stateless and pure. `RequiredGates(actionCode)` returns
the gate set for an action code; adding a gate is a contract change
(`AutomationVersion` bump). All actions share the core gates
(`uid_rv_recheck`, `scope`, `freeze_window`, `concurrent_plans`,
`attempt_cap`). Pod-affecting actions add `pdb_blast_radius`. SLO-bound
actions add `slo_burn`. Rollback actions add `rollback_point`.

`Evaluate` runs the gates at preview time and stores the results on the
plan. `Recheck` runs the same gates at execute time with `Rechecked = true`
and fresh `GateContext`; a gate that passed at preview may fail at recheck.
`AllPassed` treats `skipped` as non-failure (e.g. missing PDB evidence
skips `pdb_blast_radius`); a single `failed` gate fails the plan.

The `uid_rv_recheck` gate is the staleness defence: the target UID and
resourceVersion captured at preview are compared against the current
snapshot at execute. A changed UID (resource replaced) or a changed
resourceVersion (resource mutated) fails the gate. An empty preview
snapshot (draft plan that was never previewed) skips the gate — but
`Preview` always captures the snapshot, so an `approved` plan always has
one.

### 3. Confirmation token, idempotency key, and lease

`Preview` issues a cryptographically random confirmation token (32 bytes,
base64-encoded) to the operator. The token's SHA-256 hash is stored on
the plan; the plaintext is returned once and never persisted. `Execute`
requires the plaintext token; `Claim` re-hashes and compares. A wrong
token yields `ErrConfirmationInvalid` (403).

`Execute` also requires an idempotency key (operator-supplied UUID).
`Claim` stamps the key on the plan; re-execute with the same key returns
the recorded outcome. Re-execute with a different key after a terminal
status yields `ErrAlreadyExecuted` (409). Stale `executing` rows (older
than `claimTTL`) are reclaimable by a new `Claim` — but M44 does not
auto-reclaim; a background worker (deferred) will handle that.

### 4. Human approval with four-eyes for rollback

`approvalTypeFor(actionCode)` returns `single` for most actions and
`four_eyes` for rollback. Four-eyes requires `approver_user_id !=
requested_by_user_id`; the DB CHECK constraint enforces this at write
time, and the service re-checks before transition. Self-approval of a
four-eyes plan yields `ErrSelfApprovalForbidden` (403).

The default `Level` is `L2` (human-in-the-loop). M44 does not define
`L4` (fully autonomous); a future milestone may add it with a stricter
gate set and a separate approval type.

### 5. Post-action verifier with evidence comparison

`Verifier` is constructed with an `EvidenceProvider` that captures
redacted, hash-stamped snapshots. `CapturePreSnapshot` is called at
execute time; `CapturePostSnapshot` is called by the verifier worker
after `cooldown` elapses. The default provider reads from the SLO service
and the Kubernetes source; tests inject a fake.

`compareEvidence(pre, post, plan)` is deterministic: identical snapshots
+ identical plan → identical comparison. SLO state transitions take
precedence when both snapshots have SLO state (healthy > burning_slow >
burning_fast > breached). Resource state (replicas, available_replicas,
image, suspended) is compared for actions without SLO evidence or when
SLO state is unchanged. Missing evidence on either side yields
`ComparisonInsufficient` and `VerificationStatusUnknown` — the verifier
never auto-resolves a diagnosis from missing data.

`classifyStatus` maps (comparison, missing, action) to
`effective`/`ineffective`/`failed`/`unknown`. `failed` is reserved for
capture failures; `unknown` is reserved for insufficient evidence.

### 6. Server-owned rollback contract

When verification yields `ineffective` or `failed`, `evaluateRollbackContract`
checks whether a safe rollback exists:

- The target UID is unchanged since execute (no replacement).
- No freeze window is active.
- No concurrent plan targets the same UID.
- The attempt cap is not exceeded.
- The original action has a defined rollback action code
  (`deployment.rollback` for `deployment.image_update` and
  `deployment.rollout_restart`; `deployment.rollout_restart` for itself;
  no rollback for `cronjob.suspend`/`resume` — they are their own inverse).

If safe, a rollback plan is drafted automatically (status `draft`,
`rollback_of_plan_id` set) and the operator is notified. If unsafe, the
verification records `reason = "unsafe_rollback_escalated_to_human"` and
the case is surfaced for human review. M44 never auto-executes the
rollback plan; it requires the same preview → approve → execute path.

### 7. Bounded HTTP surface

| Route | Purpose |
|---|---|
| `GET /api/v1/aiops/automation/runbooks` | List executable runbook catalog |
| `GET /api/v1/aiops/automation/plans` | List action plans (filterable) |
| `POST /api/v1/aiops/automation/plans` | Create draft plan |
| `GET /api/v1/aiops/automation/plans/{plan_id}` | Get one plan |
| `POST /api/v1/aiops/automation/plans/{plan_id}/preview` | Evaluate gates, transition to previewed |
| `POST /api/v1/aiops/automation/plans/{plan_id}/approve` | Transition to approved |
| `POST /api/v1/aiops/automation/plans/{plan_id}/execute` | Transition to executing, apply patch, verify |
| `POST /api/v1/aiops/automation/plans/{plan_id}/cancel` | Transition to cancelled |
| `POST /api/v1/aiops/automation/plans/{plan_id}/verify` | Run post-action verifier |
| `GET /api/v1/aiops/automation/plans/{plan_id}/verification` | Get latest verification |

Routes are registered only when `AutomationService` is non-nil, mirroring
the M35/M40/M41/M42/M43 pattern. Actor identity is derived from the
authenticated session; the M35 scope decision is re-checked at execute
time by the `scope` gate (defence-in-depth).

### 8. Audit and failure persistence

Every plan stores its full gate history (preview + recheck) as JSONB.
Every verification stores its pre/post snapshots, comparison, status and
reason. Execution errors are redacted via `safeExecutionError` before
persistence — the raw error is logged server-side but never exposed to
the client, preventing log/manifest injection into the audit trail (ADR
0008).

On execution failure, the service still creates a pending verification so
the operator can see the pre-snapshot and reason about what went wrong.
On verification capture failure, the verification is `failed` with
`missing_evidence = true`; the plan retains its execution status and the
operator can re-run verification.

## Consequences

- **Automation is policy-bound, not autonomous.** Every action passes
  through deterministic gates, human approval, and post-action
  verification. M44 does not introduce a fully-autonomous path; a future
  milestone may add `L4` with a stricter gate set.
- **Stale targets fail closed.** The `uid_rv_recheck` gate ensures that
  a resource replaced or mutated between preview and execute fails the
  plan, preventing the automation from acting on the wrong target.
- **Runaway loops are bounded.** The `attempt_cap` gate caps the number
  of non-cancelled plans per target UID within a rolling window, so a
  flapping target cannot be hammered by repeated automation.
- **Verification is evidence-bound.** Missing evidence never resolves a
  diagnosis automatically; the verifier returns `unknown` and the
  operator must intervene. This preserves the M41 fail-closed invariant
  in the automation layer.
- **Rollback is safe-only.** When verification yields ineffective/failed
  and a safe rollback exists, a rollback plan is drafted automatically;
  otherwise the case escalates to a human. M44 never auto-executes
  rollbacks.
- **Audit is complete.** Every transition, every gate result, every
  verification is persisted. The full history is available for replay
  and forensic analysis.
- **Deferred**: background verification worker (cooldown-based
  scheduling), real Kubernetes integration tests for the patch path,
  real Prometheus/SLO integration for the verifier, real-kind E2E for
  the preview → approve → execute → verify path, frontend UI (plan list,
  plan detail with gate timeline, verification panel), `L4` autonomous
  level, and the rollback-plan auto-execution path.
