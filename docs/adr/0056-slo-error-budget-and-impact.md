# ADR 0056: SLO, Error Budget And Impact (M41)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M41
- Supersedes: none
- Related: ADR 0053 (capability adapters), ADR 0054 (signal model),
  ADR 0055 (temporal topology), ADR 0004 (bounded Kubernetes gateway)

## Context

M40 left the platform with a temporal topology and a unified change timeline,
but no way to express *what healthy means* for a service. Without an explicit
service-level objective, the AIOps Intelligence Plane cannot:

- distinguish "no signal because everything is fine" from "no signal because
  the metrics source is down";
- quantify how fast a service is burning its error budget during a partial
  outage;
- feed M42 (correlation) a deterministic "is this service currently impacted?"
  answer that M43 (AI investigator) can cite.

The optimization plan (`docs/kubesphere-optimization-plan.md` §12-13) requires
M41 to deliver SLO views that link the current evaluation to authorized
signals, recent changes and diagnoses, while preserving the AIOps invariants:
deterministic evaluation is the source of truth, missing data must be explicit,
and AI never modifies the SLO.

The key constraints:

1. The API must never accept raw PromQL, LogQL or arbitrary query languages.
   Only server-owned SLI templates are admitted. Adding a template is a
   contract change, not a runtime configuration.
2. Missing data cannot be labeled healthy. The default policy is
   `unavailable` — the SLO reports `StateUnavailable` and never fabricates a
   normal state. The only fail-open path is `workload_readiness` with an
   explicit operator opt-in, and even then the `Coverage` field remains
   `Unavailable` so the fail-open is auditable.
3. SLO edits are versioned. Every mutation increments `Version`; historical
   evaluations remain queryable by version. Edits never rewrite history.
4. Evaluations are append-only and deterministic: the same Definition and the
   same MetricsSource output yield the same Evaluation. Counter resets, sparse
   data and clock boundaries are handled deterministically.
5. Burn alerts enter the existing M27 alert lifecycle via a
   `BurnAlertSink` interface — M41 never creates a second alert system. The
   sink is best-effort: a sink failure does not roll back the evaluation.
6. M41 must not change the native M21-M31 signal path. SLO is a deterministic
   evaluator that reads from the M37 capability providers and writes to the
   `slo_*` tables; it does not modify signals, diagnoses or audit records.
7. Scope is enforced by M35: cluster_id binding at create time plus the
   existing requireNamespaceAccess/requireNamespaceQueryAccess middleware on
   the underlying Kubernetes resources. SLO routes themselves do not carry a
   `:namespace` path parameter, so they do not add new middleware.

## Decision

### 1. Server-owned SLI templates only

Three templates are admitted in V1:

- `request_success_ratio` — good = successful requests, total = all requests.
  Requires traffic metrics. Never fails open.
- `request_latency_target_ratio` — good = requests under the configured
  latency threshold, total = all requests. Requires `latency_threshold_ms`
  (1..60000). Never fails open.
- `workload_readiness` — good = ready pods, total = desired pods. Does not
  require traffic metrics. May fail open (explicit operator opt-in). This is
  a platform-health indicator, never a substitute for request availability.

The catalog (`slo.catalog`) is the single source of truth for which templates
exist, what they require and which missing-data policies they admit.
`ValidateDefinition` is the only validation entry point, used by Create, Patch
and the HTTP layer. Adding a template requires updating the catalog, the
migration CHECK constraint, the OpenAPI enum and the ADR — it is a contract
change.

### 2. Deterministic evaluator with explicit missing-data handling

The `Evaluator` consumes a `MetricsSource` (a bounded read interface shaped
for SLO evaluation) and produces a single `Evaluation` per call. The
evaluation is pure with respect to inputs: the same Definition and the same
MetricsSource output yield the same Evaluation.

Counter resets are detected as monotonicity violations and the delta is taken
as the post-reset value (the counter is treated as having reset to 0). Sparse
data produces `CoveragePartial` when at least one sample is present but the
sample count is below the expected minimum; complete absence yields
`CoverageUnavailable` and `StateUnavailable` (fail-closed). Clock boundaries
use inclusive `window_start` and exclusive `window_end` — samples at exactly
`window_start` are counted, samples at `window_end` are excluded.

Coverage is computed from sample count vs expected:

- 0 samples → `CoverageUnavailable`
- 1 sample or `< expected/2` → `CoveragePartial`
- `>= MinSamplesForComplete (2)` and `>= expected/2` → `CoverageComplete`

When samples exist but produce zero deltas (e.g. a single cumulative counter
sample), `State` is `Unavailable` but `Coverage` is preserved as `Partial` so
callers can distinguish "no samples at all" from "had samples but no deltas".

### 3. Error budget, burn rate and state classification

The error budget is `1 - objective`. The remaining budget is
`(ratio - objective) / error_budget`, clamped to `[0, 1]`. The burn rate is
`(1 - ratio) / error_budget`. When `objective == 1.0` the error budget is 0;
remaining is 1.0 when `ratio == 1.0` and 0.0 otherwise, and burn rate is
`+Inf` for any `ratio < 1.0`.

State precedence (highest severity first):

1. `breached` — `ratio < objective` for the full rolling window (budget
   exhausted).
2. `burning_fast` — burn rate `>= fast_burn_rate`.
3. `burning_slow` — burn rate `>= slow_burn_rate`.
4. `healthy` — `ratio >= objective` and no burn threshold exceeded.

V1 compares the single rolling-window burn rate against both thresholds. A
multi-window implementation (separate fast/slow evaluations over their own
windows) is a future enhancement; for V1 the single-window burn rate is
conservative — a single-window burn that exceeds the fast threshold is
unambiguously a fast burn.

### 4. Versioned definitions with append-only evaluations

The `slo_definitions` table stores versioned definitions. `Version` starts at
1 and increments on every patch. `updated_at` is refreshed on every patch.
The unique partial index `uq_slo_definitions_active` admits exactly one active
row per `(cluster_id, service_namespace, service_name, template)`; deleting a
definition sets `enabled=false` and frees the slot for a new active definition.

The `slo_evaluations` table is append-only. `InsertEvaluation` uses
`ON CONFLICT DO NOTHING` so retries are idempotent. Historical evaluations
remain queryable by `(slo_id, version, window)`.

### 5. Burn transitions via BurnAlertSink

The `Service.EvaluateSLO` method reads the previous state from the latest
persisted evaluation (any version). When the state changes
(`previous != current`), it emits a `BurnTransition` to the configured
`BurnAlertSink`. The sink is best-effort: a sink error does not roll back the
evaluation. The default sink is `NopBurnAlertSink`; the M27 integration is
wired at the httpserver layer and translates transitions into alert
instances. The SLO service never creates alert Rules — it only emits lifecycle
transitions.

A fresh SLO's baseline previous state is `healthy`, so a first evaluation
that immediately breaches emits a `healthy → breached` transition. Steady-state
evaluations (same state as previous) do not emit transitions, avoiding alert
churn.

### 6. HTTP routes and OpenAPI contract

Eight routes are registered under `/api/v1/aiops/slos`:

- `GET /templates` — list the SLI template catalog (read-only, no service
  required).
- `GET /` — list definitions (filter by cluster, namespace, template, enabled,
  owner).
- `POST /` — create definition (SystemOpsAdmin).
- `GET /:id` — get definition.
- `PATCH /:id` — patch definition (SystemOpsAdmin); Version increments.
- `DELETE /:id` — disable definition (SystemOpsAdmin); row retained.
- `POST /:id/evaluate` — run one evaluation (SystemOpsAdmin); persists and
  emits burn transition.
- `GET /:id/evaluations` — list evaluations (filter by version, state, time
  range).

Writes require `rolesSystemOpsAdmin`; reads are available to any
authenticated user. Cluster/namespace scope is enforced by M35 on the
underlying Kubernetes resources and by the service's `cluster_id` binding at
create time — the SLO routes themselves do not carry a `:namespace` path
parameter and so do not add new middleware.

The OpenAPI document adds a `slo` tag and eight schemas:
`SLITemplateCatalog`, `SLITemplateDescriptor`, `SLOServiceRef`, `SLOActorRef`,
`SLODefinition`, `SLODefinitionCreate`, `SLODefinitionPatch`,
`SLODefinitionList`, `SLOEvaluation`, `SLOEvaluationList`. The route contract
test (`TestRegisteredRoutesMatchOpenAPI`) verifies bidirectional consistency.

### 7. No new public API accepts a query language

The `MetricsSource.QuerySLI` interface takes a `*Definition` (which carries
the `SLITemplate` enum) and a time range — never PromQL. The concrete adapter
that translates templates into provider queries lives outside the `slo`
package and is wired at httpserver construction. This preserves ADR 0053 §5
(clients cannot inject PromQL) and the optimization plan's invariant that
"AI may rank only server-approved runbooks; it cannot create Kubernetes
commands, patches, URLs or query languages".

## Consequences

- **Positive**: M42 correlation now has a deterministic "is this service
  impacted?" answer it can cite. M43 AI investigator can reference SLO
  evaluations as evidence. Operators can define SLOs without writing PromQL.
- **Positive**: Missing data is explicit — a provider outage produces
  `StateUnavailable` with `CoverageUnavailable`, never false health.
- **Positive**: Burn alerts reuse the M27 lifecycle instead of creating a
  parallel alert system.
- **Negative**: V1 evaluates burn rate over a single rolling window rather
  than separate fast/slow windows. This is conservative (a single-window
  fast burn is unambiguously fast) but may produce false `burning_fast` for
  short windows. Multi-window evaluation is deferred to a future enhancement.
- **Negative**: The `MetricsSource` adapter that translates templates into
  Prometheus/Loki queries is not yet wired in `cmd/server/main.go`. The
  routes are registered and the contract is verified, but production
  deployment requires the adapter. This matches the M37/M39/M40 pattern of
  shipping the contract and tests before the production wiring.
- **Deferred**: Real Prometheus/Loki integration tests, real-kind E2E for
  the burn-transition-to-M27 path, frontend SLO management UI, and the
  background evaluation worker.

## Alternatives Considered

- **User-defined PromQL**: rejected. Violates ADR 0053 §5 and the optimization
  plan's invariant that the API never accepts query languages.
- **Multi-window burn rate in V1**: rejected for complexity. The
  single-window rate is conservative and the data model already stores
  `fast_burn_window_seconds` and `slow_burn_window_seconds` for a future
  multi-window evaluator.
- **Direct alert.Rule creation from SLO**: rejected. Would create a second
  alert system and violate the M27 lifecycle contract. The `BurnAlertSink`
  interface keeps M41 decoupled from alert internals.
- **Rewriting historical evaluations on edit**: rejected. Violates the
  append-only contract and the audit invariant that "SLO edits never rewrite
  historical evaluations".
