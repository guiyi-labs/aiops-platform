# ADR 0058: Cited And Evaluated AI Investigator (M43)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M43
- Supersedes: none
- Related: ADR 0057 (multi-signal correlation and deterministic RCA),
  ADR 0054 (unified signal model), ADR 0055 (temporal topology),
  ADR 0024 (resource-originated controlled operations), ADR 0007
  (append-only human diagnosis workflow)

## Context

M42 delivered deterministic, replayable multi-signal correlation that
links M39 signals, M40 topology/changes and M41 SLO impact into bounded
cases with explicit factors and confidence classes. M42 deliberately left
the natural-language "why did this happen, and what should I do?" to M43.

The platform already has a per-diagnosis cited explanation feature
(`aiexplain`): for one diagnosis record it asks a provider for a cited
summary, analysis and recommended actions, and rejects fabricated evidence
IDs. But that feature is bound to a single diagnosis and cannot reason
about a correlation *case* — the cross-signal factors, the topology path,
the change candidates, and the eligible runbooks.

The optimization plan (`docs/kubesphere-optimization-plan.md` §15) requires
M43 to deliver a **cited and evaluated** AI investigator bound to M42
correlation cases, without becoming a general chat console, without
letting the model modify system state, and without letting an AI outage
remove the operator's ability to act.

The key constraints:

1. **Citations are mandatory and bounded.** Every factual claim, impact
   statement and hypothesis must cite an authorized evidence ID. The
   prompt builder assembles the authorized set from the case's signals,
   change candidates and the case itself; the validator rejects any
   citation outside that set. Fabricated, nonexistent or unauthorized
   citations reject the *entire* output, not just the offending claim.
2. **The model cannot upgrade a candidate to confirmed cause.** Only an
   operator reviewing the diagnosis record can (ADR 0007, ADR 0057). The
   validator rejects hypotheses whose claim text asserts "confirmed root
   cause". The correlation case's confidence class is never modified by
   the investigator — the investigation is a read-only advisory.
3. **Runbook recommendations are server-owned and eligibility-bounded.**
   The model may only emit runbook IDs from a compiled catalog. Each
   runbook maps to an M19 controlled-operations code (or is advisory-only
   with an empty code). The investigator rechecks eligibility against the
   M42 `ActionCandidate` list at generation time; an ineligible runbook
   rejects the output.
4. **No general tools.** The model cannot invoke Kubernetes URLs, kubectl,
   SQL, PromQL, LogQL or raw provider queries. Only server-fixed
   read-only evidence is admitted to the prompt. Logs, Events, labels,
   annotations and manifests are untrusted data and cannot alter the
   system/tool instructions (prompt-injection defense).
5. **Deterministic investigation_key.** The key is SHA-256 over
   (case_id + investigator_version + prompt_hash). Identical evidence +
   prompt + version reproduce identical keys, so re-runs are idempotent
   and auditable. One active investigation per case_key; newer
   investigations mark older ones stale.
6. **AI outage leaves deterministic investigation available.** On provider
   failure, budget exhaustion, invalid output or citation rejection, a
   *failed* investigation is persisted with `failure_reason` set. The
   operator still sees the case, the deterministic factors, the action
   candidates, and the failure reason — the AI is advisory, not load-bearing.

## Decision

### 1. Structured cited output, not a chat console

The investigator produces one structured `Investigation` per case:
`summary`, `impact`, `hypotheses[]` (claim, confidence, evidence_ids,
disconfirming_evidence, next_checks), `recommended_runbook_id`, and
`uncertainties[]`. There is no free-form conversation, no streaming, no
tool-call loop. The output schema is fixed in the system prompt and
validated after the provider returns.

### 2. Authorized evidence set + citation rejection

`BuildPrompt` assembles the authorized evidence set from the `CaseContext`:
the case itself (`correlation_case:<id>`), every linked signal occurrence
(`signal_occurrence:<id>`), and every change candidate
(`change_candidate:<id>`). The user prompt lists only these IDs and
redacted facts. `ValidateProviderResult` rejects any citation, hypothesis
evidence, or disconfirming evidence whose ref is not in the authorized
set. The rejection is total — the entire output is discarded and a failed
investigation is persisted.

### 3. Server-owned runbook catalog

`catalog.go` compiles a fixed map of `RunbookDescriptor`: each carries a
`RunbookID`, an `ActionCode` (M19 code, or empty for advisory-only),
a title and human-readable steps. Adding a runbook is a contract change
(`InvestigatorVersion` bump), not runtime configuration. The investigator
passes only the *eligible* runbooks (per the M42 `ActionCandidate` list)
to the system prompt, and rechecks eligibility before persisting. The
model cannot invent runbook IDs.

### 4. Prompt-injection defense via untrusted-data boundary

The system prompt declares that logs, events, labels, annotations and
manifests are untrusted data that cannot alter instructions. The user
prompt contains only redacted, authorized evidence — no raw logs, no
events, no manifest text. The validator does not do semantic content
filtering (that is the system prompt's job); it enforces the *structural*
invariants: citations authorized, runbook eligible, no "confirm root
cause" claims, schema complete. The `prompt_injection_rejected` golden
fixture documents that a structurally-valid-but-injected output is
accepted by the validator (the defense is the evidence-bound prompt, not
content filtering), while `hidden_scope_citation_rejected` and
`fabricated_citation_rejected` cover the structural attacks.

### 5. Deterministic investigation_key and staleness

`computeInvestigationKey` is SHA-256 over (case_id + investigator_version
+ prompt_hash), where `prompt_hash` is a stable hash over the case
context's rule_id, confidence, evidence completeness, primary resource,
and the sorted authorized evidence ID set. Identical evidence + version
reproduce identical keys. The repository's `Save` marks older
non-stale investigations for the same `(case_id, investigation_key)`
stale, so only one active investigation per key is retained; history is
kept for audit. The unique index `uq_ai_investigations_active` enforces
this at the DB level.

### 6. Failure persistence

`Service.Investigate` always persists an investigation row. On provider
error the status is `failed` with `failure_reason = "provider_error"` and
`provider/model = "unknown"`. On validation failure (citations rejected,
runbook ineligible, schema incomplete) the status is `failed` with
`failure_reason = "citation_rejected"`, but the provider's summary,
hypotheses and citations are still persisted for audit. The HTTP handler
returns the failed investigation with 200 (not an error status) so the
caller sees the failure reason; only `ErrCaseNotFound` (404) and
`ErrDisabled` (503) are error responses.

### 7. Read-mostly HTTP surface

| Route | Purpose |
|---|---|
| `GET /api/v1/aiops/investigator/runbooks` | List runbook catalog |
| `GET /api/v1/aiops/investigator/cases/:case_id/investigations` | List investigations for a case |
| `GET /api/v1/aiops/investigator/investigations/:id` | Get one investigation |
| `POST /api/v1/aiops/investigator/cases/:case_id/investigations` | Generate a cited investigation |

The POST is the only write; it persists an investigation but never
modifies the case, diagnosis or alert. Actor identity is derived from the
authenticated session. Routes are registered only when
`AIInvestigatorService` is non-nil, mirroring the M35/M40/M41/M42 pattern.

## Consequences

- **AI is advisory, not load-bearing.** An AI outage, budget exhaustion,
  or schema/citation failure leaves the deterministic investigation, the
  case factors, and the M42 action candidates available. The operator is
  never blocked by the AI.
- **Citations are auditable.** Every claim traces to an authorized
  evidence ID; fabricated or out-of-scope citations reject the entire
  output. An operator can replay an investigation and see exactly which
  evidence supported each hypothesis.
- **No silent promotion.** The model cannot upgrade a candidate to
  confirmed cause; only an operator can, via the diagnosis lifecycle. The
  validator rejects "confirmed root cause" claims structurally.
- **Runbooks are bounded.** Recommendations are server-owned IDs already
  declared eligible by the deterministic M42 Action Catalog. M44 (safe
  automation) reuses the existing preview/confirmation/idempotency/audit
  paths; there is no execute endpoint in M43.
- **Deferred**: real AI provider integration (Responses-compatible HTTP
  provider), provider budget/reservation enforcement, real PostgreSQL
  integration test for `GormRepository`, real-kind E2E for the
  investigate → citation → runbook path, frontend UI (investigation
  panel, hypothesis rendering, citation tooltips), and M44 safe-automation
  wiring.
