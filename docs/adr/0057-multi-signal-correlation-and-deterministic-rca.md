# ADR 0057: Multi-Signal Correlation And Deterministic RCA (M42)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M42
- Supersedes: none
- Related: ADR 0054 (unified signal model), ADR 0055 (temporal topology),
  ADR 0056 (SLO and error budget), ADR 0007 (append-only human diagnosis
  workflow), ADR 0024 (resource-originated controlled operations)

## Context

M39 (unified signal model), M40 (temporal topology and change intelligence)
and M41 (SLO and error budget) left the platform with three isolated data
planes: signals, topology/changes, and SLO impact. Without a correlation
layer, the platform cannot answer:

- "A pod started crash-looping 8 minutes after a promotion — is the rollout
  the root cause, or a coincidence?"
- "A Service lost its ready endpoints right after a rollout — which
  Deployment is the candidate for rollback?"
- "A node went NotReady during a maintenance window — is the maintenance the
  cause?"

The optimization plan (`docs/kubesphere-optimization-plan.md` §14) requires
M42 to deliver deterministic, replayable multi-signal correlation that links
M39 signals, M40 topology/changes and existing diagnosis records into bounded
cases — without introducing a black-box anomaly score or a second incident
workflow.

The key constraints:

1. Correlation must be **deterministic and replayable**: identical inputs +
   identical rule/correlation versions yield identical cases, factors,
   confidence classes and reason codes. There is no random seed, no
   probabilistic model, no black-box score.
2. **Temporal proximity alone is never causality.** A change event that
   started 5 minutes before a symptom is a *candidate*, not a confirmed root
   cause. Confirmation requires explicit factors: same UID, bounded topology
   distance, bounded time distance, reviewed change-symptom rule, and no
   contradicting signal.
3. The **diagnosis record remains the human status/SLA/feedback source of
   truth** (ADR 0007). Correlation cases are *candidates*, not incidents. A
   case never auto-closes; it is retained until the operator dismisses the
   underlying diagnosis.
4. **AI (M43) cannot promote a candidate to confirmed.** Only an operator
   reviewing the diagnosis record can do that. The confidence class never
   upgrades itself; AI may rank candidates but never promote them.
5. The **case_key is deterministic**: SHA-256 over (cluster_id, resource_uid,
   rule_id, correlation_version). N duplicate symptoms form one active case
   only when case_key matches. Different UID, authorization scope or
   unrelated topology never merges.
6. **Action candidates are fixed and read-only.** They carry a fixed code
   from the M19 controlled-operations catalog (e.g.
   `deployment.rollback`, `deployment.rollout_restart`). There is no execute
   endpoint — M44 reuses existing preview/confirmation/idempotency/audit
   paths.
7. Scope is enforced by M35: the `cluster_id` query parameter and
   `:namespace` path parameters are subject to the existing
   `requireNamespaceAccess`/`requireNamespaceQueryAccess` middleware.
   Correlation routes themselves do not carry a `:namespace` path parameter,
   so they do not add new middleware.

## Decision

### 1. Deterministic rule catalog, not a model

Correlation rules are server-owned, versioned and compiled into a Go map
(`catalog.go`). Each `RuleDescriptor` carries: trigger signal IDs, change
kinds, primary resource kind, time window, required factors, contradicting
factors and a stable reason code. Adding a rule is a contract change
(`CorrelationVersion` bump), not a runtime configuration.

Six rules are admitted in V1, covering the golden replay scenarios:
`rollout_causes_pod_failure`, `rollout_causes_unavailable_deployment`,
`rollout_causes_no_endpoints`, `maintenance_causes_node_failure`,
`pvc_pending_causes_pod_failure`, `rollout_causes_metric_breach`.

### 2. Explicit factors, not a score

The engine computes explicit, named factors for each (signal, change) pair:

| Factor | Meaning |
|---|---|
| `same_uid` | Signal resource UID == change target UID |
| `topology_distance` | Shortest path in the topology edge graph (bidirectional BFS) |
| `time_distance` | Bounded seconds between change start and symptom observation |
| `change_symptom_rule` | The change kind matches the rule's reviewed change-symptom mapping |
| `signal_freshness` | Signal is active and within the lookback window |
| `signal_completeness` | Signal coverage is complete or partial |
| `diagnosis_match` | A diagnosis record matches the signal's resource |
| `contradicting_signal` | A resolution signal or different UID contradicts the candidate |

Factors carry an evidence ref array pointing at the signals/topology/diagnosis
rows that support them. The weight is fixed by the catalog; the engine never
adjusts it at runtime.

### 3. Confidence classification is deterministic

Confidence is classified by a pure function over (rule, factors,
contradicting refs):

- **Confirmed**: all required factors present, no contradicting factor. The
  linked change event is the asserted root cause.
- **Candidate**: factors partially match. The linked change is a plausible
  cause but evidence is incomplete.
- **Contradicted**: a contradicting factor is observed (e.g. no topology
  path between signal resource and change target). Retained for audit but
  not ranked as a cause.
- **Unknown**: insufficient evidence (cold-start, no change in window).
  Retained so M43 can disclose uncertainty.

### 4. Case is the aggregation unit

One active case per deterministic `case_key`. The engine is pure and
stateless; the service is the only writer to `correlation_*` tables. Cases
are idempotent — running `CorrelateNamespace` twice with the same inputs
produces the same persisted rows (deduplicated by `case_key` and unique
indexes). N duplicate symptoms merge into one active case; all signal
occurrences are preserved as `SignalLink` rows.

### 5. Golden replay fixtures are the contract

Nine golden replay scenarios validate the engine: ImagePull, CrashLoop, OOM,
PVC-Pending, NoEndpoints, ReplicasUnavailable, NodeNotReady, MetricBreach,
and a contradicted BadRollout. Each fixture is a deterministic
(inputs, expected) pair; replaying the same fixture must produce the same
case_key, confidence and candidate count. A cold-start scenario covers the
"no change in window" path.

### 6. Read-only HTTP surface

The HTTP surface is query-only:

| Route | Purpose |
|---|---|
| `GET /api/v1/aiops/correlation/rules` | List rule catalog |
| `GET /api/v1/aiops/correlation/cases` | List cases (cluster_id required) |
| `GET /api/v1/aiops/correlation/cases/timeline` | Case timeline |
| `GET /api/v1/aiops/correlation/cases/:id` | Full case view |
| `GET /api/v1/aiops/correlation/cases/:id/graph` | Impact graph (resource links) |
| `GET /api/v1/aiops/correlation/cases/:id/actions` | Fixed action candidates |

Case correlation is an internal operation (background worker or
signal-ingestion hook), not an HTTP-triggered write.

### 7. Bidirectional topology path search

The engine builds a bidirectional edge index (forward: parent→child, reverse:
child→parent) from M40 topology edges. `findPath` performs a BFS in both
directions so a Pod can find its owning Deployment (reverse Owns) and a
Service can find its backing Pod (forward BackedBy). The path is recorded as
a JSON array of edge kinds on the `ResourceLink` so callers can render the
impact graph without re-querying topology.

## Consequences

- **Deterministic RCA is auditable**: every factor, evidence ref and
  confidence class is explicit and versioned. An operator can replay a case
  and see exactly why the engine ranked a candidate as confirmed or
  contradicted.
- **No black-box promotion**: AI (M43) may rank candidates and generate
  natural-language explanations, but cannot promote a candidate to
  confirmed. Only an operator can, via the diagnosis lifecycle.
- **M44 action path is safe**: action candidates are fixed codes with exact
  target identity. M44 rechecks UID/resourceVersion at preview time; there
  is no execute endpoint in M42.
- **Deferred**: background correlation worker, signal-ingestion hook,
  real PostgreSQL integration test, real-kind E2E, and frontend UI.
