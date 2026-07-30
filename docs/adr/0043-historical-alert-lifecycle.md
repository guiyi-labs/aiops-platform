# ADR 0043: Historical Alert Lifecycle

- Status: Accepted
- Date: 2026-07-30
- Owners: Backend and platform operations

## Context

ADR 0037 established a deterministic sustained-window evaluation engine. ADR
0038 bridges evaluation results into the diagnosis lifecycle synchronously.
Operators currently must invoke `POST /diagnoses/node_metrics` manually or
observe the Dashboard trend consumer to detect sustained metric breaches.

M27 turns the accepted M21 evaluator into a bounded background workflow that
automatically creates exactly one active alert per rule when a sustained breach
is detected, deduplicates repeated firings, surfaces insufficient-data and
error states, and resolves only after a complete normal evaluation. Human
acknowledgement reuses the linked diagnosis record.

Without this milestone, sustained metric breaches remain invisible until an
operator explicitly checks the Dashboard or invokes the synchronous endpoint.
The existing notification pipeline cannot fire because no diagnosis record is
created automatically.

## Decision

### 1. New alert domain package

Add `internal/alert` with two persistent records:

**AlertRule** — immutable evaluation shape, mutable display name and enabled
state:

| Field | Type | Constraints |
|---|---|---|
| id | BIGSERIAL | PK |
| cluster_id | BIGINT | NOT NULL, FK clusters(id) |
| display_name | VARCHAR(128) | NOT NULL, case-insensitive unique per (cluster_id, NOT deleted) |
| resource_kind | VARCHAR(16) | NOT NULL, CHECK = 'Node' |
| resource_name | VARCHAR(253) | NOT NULL |
| metric_name | VARCHAR(16) | NOT NULL, CHECK IN ('cpu','memory') |
| operator | VARCHAR(4) | NOT NULL, CHECK IN ('gte','lte') |
| threshold | BIGINT | NOT NULL, CHECK >= 0 |
| for_seconds | INTEGER | NOT NULL, CHECK BETWEEN 60 AND 21600 |
| minimum_points | INTEGER | NOT NULL, CHECK BETWEEN 2 AND 360 |
| enabled | BOOLEAN | NOT NULL, DEFAULT TRUE |
| deleted | BOOLEAN | NOT NULL, DEFAULT FALSE |
| last_evaluation_state | VARCHAR(24) | DEFAULT '' |
| last_evaluation_at | TIMESTAMPTZ | |
| last_error_code | VARCHAR(32) | DEFAULT '' |
| next_due_at | TIMESTAMPTZ | NOT NULL |
| claim_expires_at | TIMESTAMPTZ | |
| creator_user_id | BIGINT | NOT NULL |
| creator_name | VARCHAR(128) | NOT NULL |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |

Immutability: cluster_id, resource_kind, resource_name, metric_name, operator,
threshold, for_seconds, minimum_points are frozen after creation. Only
display_name and enabled may be patched.

Soft delete: `deleted = TRUE` preserves historical alert/diagnosis references.
A rule with an unresolved alert instance may not be deleted (409 conflict).

Per-cluster limit: at most 20 non-deleted rules per cluster_id.

Case-insensitive uniqueness: one active (not deleted) rule name per cluster,
enforced by a unique index on `LOWER(display_name)` where `deleted = FALSE`.

**AlertInstance** — at most one unresolved instance per rule:

| Field | Type | Constraints |
|---|---|---|
| id | BIGSERIAL | PK |
| rule_id | BIGINT | NOT NULL, FK alert_rules(id) |
| diagnosis_id | BIGINT | NOT NULL, FK diagnosis_records(id) |
| state | VARCHAR(16) | NOT NULL, CHECK IN ('firing','resolved') |
| first_fired_at | TIMESTAMPTZ | NOT NULL |
| last_fired_at | TIMESTAMPTZ | NOT NULL |
| resolved_at | TIMESTAMPTZ | |
| latest_evidence_anchor | JSONB | NOT NULL DEFAULT '{}' |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |

Uniqueness: partial unique index `ONE_UNRESOLVED_PER_RULE` on `(rule_id)`
where `state = 'firing'`. This enforces at most one unresolved alert per rule
at the database level.

### 2. State machine

1. `firing` evaluation with no unresolved instance → atomically create one
   AlertInstance (state=firing) and one linked diagnosis Record.
2. Repeated `firing` → update `last_fired_at` and evidence anchor on the
   existing instance. No new diagnosis or notification.
3. `insufficient_data` or evaluator/upstream error → update rule health
   fields only. Never fire or resolve an alert.
4. Complete `normal` evaluation → resolve the unresolved alert instance.
5. Later `firing` → create a new instance and diagnosis.

The create-or-touch path is transactional. Two backend instances evaluating
the same due rule must not create two unresolved alerts. The scheduler uses
`SELECT ... FOR UPDATE SKIP LOCKED` on the rule row to claim due rules, and
the partial unique index prevents duplicate unresolved instances.

### 3. Scheduler bounds

| Parameter | Default | Maximum |
|---|---|---|
| Scheduler poll interval | 15s | — |
| Per-rule minimum evaluation interval | 60s | — |
| Claim batch | 20 | 20 |
| Worker concurrency | 4 | 4 |
| Per-rule evaluation timeout | 10s | 30s |
| Claim lease | 30s | 60s |
| Overlapping ticks | Prohibited | — |
| Rule ordering | next_due_at ASC, id ASC | — |

Expired claim recovery: after `claim_expires_at`, another scheduler tick may
re-claim the rule. A claim extension updates `claim_expires_at` within the
same transaction.

### 4. Configuration

New environment variables (following existing naming conventions):

```
ALERT_ENABLED=true
ALERT_POLL_INTERVAL=15s
ALERT_CLAIM_BATCH=20
ALERT_WORKER_CONCURRENCY=4
ALERT_EVALUATION_TIMEOUT=10s
ALERT_CLAIM_LEASE=30s
ALERT_MIN_EVALUATION_INTERVAL=60s
ALERT_MAX_RULES_PER_CLUSTER=20
```

All values have safe defaults. A zero or invalid value fails startup or uses
the documented safe default. `ALERT_ENABLED=false` disables the scheduler
without affecting the rule CRUD API.

### 5. API surface

**Alert rules** (cluster-scoped):

| Method | Path | Authorization |
|---|---|---|
| GET | /api/v1/clusters/:cluster_id/alert-rules | Any authenticated |
| POST | /api/v1/clusters/:cluster_id/alert-rules | system_admin, operations_admin |
| GET | /api/v1/clusters/:cluster_id/alert-rules/:rule_id | Any authenticated |
| PATCH | /api/v1/clusters/:cluster_id/alert-rules/:rule_id | system_admin, operations_admin |
| DELETE | /api/v1/clusters/:cluster_id/alert-rules/:rule_id | system_admin, operations_admin |

PATCH accepts only `display_name` and `enabled`. DELETE is soft.

**Alert instances** (cluster-scoped):

| Method | Path | Authorization |
|---|---|---|
| GET | /api/v1/clusters/:cluster_id/alerts | Any authenticated |
| GET | /api/v1/clusters/:cluster_id/alerts/:alert_id | Any authenticated |

Filter parameters: `state` (firing/resolved), `rule_id`, `limit` (max 100,
default 50).

Alert detail includes: linked diagnosis ID/status/assignee/severity, rule
reference, state, time fields and latest evaluation health.

No endpoint accepts an expression, raw query, PromQL or arbitrary labels.

### 6. Diagnosis integration

Each `firing` alert creates one diagnosis record with rule ID
`node.metric_sustained_breach.v1`, using the same evidence shape from ADR
0038. The alert instance stores the diagnosis ID. Acknowledgement,
assignment, comments and status transitions use the existing diagnosis APIs.
The alert API derives acknowledgement from its linked diagnosis record; no
parallel human workflow table is created.

The scheduler reuses the existing `MetricEvaluator` interface from the
diagnosis package. It calls `Evaluate` with the rule's immutable evaluation
fields and the last 6 hours of series data, exactly as
`DiagnoseNodeMetrics` does synchronously.

### 7. Audit

Every rule mutation (create, patch, delete) emits an audit event with action
`alert_rule.create`, `alert_rule.update`, or `alert_rule.delete`. The target
is the rule's cluster-scoped reference. Audit entries contain no threshold,
token, kubeconfig or raw upstream body — only the rule ID, display name and
changed field names.

### 8. Non-goals

- Pod rules, percentages, multi-metric correlation, notification routing,
  silence schedules, escalation policies, PromQL and arbitrary labels.
- Changing the diagnosis lifecycle (states, transitions, SLA) — alerts
  delegate to the existing workflow.
- Creating a second generic workflow engine or parallel acknowledgement table.

### 9. Migration

Migration `000020` introduces `alert_rules` and `alert_instances` tables with
the columns, constraints and indexes described above. The down migration
drops both tables.

## Consequences

- Operators can create fixed Node CPU/memory alert rules that are evaluated
  in the background, deduplicated, and linked to the existing diagnosis
  lifecycle.
- At most one active alert per rule prevents alert storms. Repeated firings
  update metadata without creating new diagnosis records or notifications.
- The scheduler is bounded: fixed poll interval, claim batch, worker
  concurrency, evaluation timeout and claim lease. No unbounded loops.
- Insufficient data and evaluator errors are surfaced as rule health
  indicators without creating or resolving alerts.
- The alert API is read-only for viewers and write-restricted to system/operations
  administrators, matching the existing controlled-operation authorization
  pattern.
- Acknowledgement and assignment reuse the diagnosis record; no parallel
  human workflow table is needed.
