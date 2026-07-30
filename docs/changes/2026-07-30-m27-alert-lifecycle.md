# M27: Historical Alert Lifecycle

- Date: 2026-07-30
- Status: Accepted (unit/contract gates and disposable Metrics Server real-kind E2E passed)
- ADR: [0043-historical-alert-lifecycle.md](../adr/0043-historical-alert-lifecycle.md)

## Summary

M27 implements a bounded background alert lifecycle over the accepted M21 sustained-window evaluation engine. Operators can create fixed Node CPU/memory alert rules that are evaluated automatically in the background, with deduplication, state management, and integration with the existing diagnosis workflow.

## Product Outcome

An operations administrator can:

1. Create a fixed alert rule for one exact Node CPU or memory series
2. The backend evaluates enabled rules in the background every 60+ seconds
3. Creates exactly one active alert/diagnosis for a sustained breach
4. Keeps repeated firing evaluations deduplicated
5. Surfaces insufficient-data/error state without creating alerts
6. Marks alerts resolved only after a complete normal evaluation
7. Human acknowledgement reuses the linked diagnosis record

## Implementation

### Phase 1: Domain and Repository

- Created `internal/alert` package with AlertRule and AlertInstance models
- Migration `000020` introduces `alert_rules` and `alert_instances` tables
- Immutable evaluation fields (cluster_id, resource_kind, resource_name, metric_name, operator, threshold, for_seconds, minimum_points)
- Soft delete with conflict behavior when unresolved alert exists
- At most 20 non-deleted rules per cluster, case-insensitive unique names
- Partial unique index enforces at most one unresolved instance per rule

### Phase 2: Service and Scheduler

- `internal/alert/service.go` implements state machine:
  - `firing` → creates alert instance and linked diagnosis
  - Repeated `firing` → updates timestamp/evidence, no new diagnosis
  - `insufficient_data`/error → updates rule health only
  - `normal` → resolves alert instance
  - Later `firing` → creates new instance and diagnosis
  - Scheduler queries a recent bounded window derived from `for_seconds`,
    minimum points and evaluation interval (with timestamp-jitter slack, capped
    at 24h), so historical breaches do not delay recovery for six hours
- `internal/alert/scheduler.go` implements bounded workers:
  - Poll interval: 15s
  - Claim batch: 20
  - Worker concurrency: 4
  - Evaluation timeout: 10s
  - Claim lease: 30s with expired-claim recovery
  - No overlapping ticks
- Configuration via environment variables (ALERT_ENABLED, ALERT_POLL_INTERVAL, etc.)
- Transactional create-or-touch path with SKIP LOCKED for race safety

### Phase 3: API and Frontend

- HTTP routes under `/api/v1/clusters/:cluster_id/alert-rules` and `/api/v1/clusters/:cluster_id/alerts`
- Authorization: any authenticated user can read, system/operations admin required for mutations
- Audit events: `alert_rule.create`, `alert_rule.update`, `alert_rule.delete`
- Frontend `AlertsView.vue` with:
  - Cluster selection and state filter
  - Alert rules table (name, node, metric, condition, status, evaluation state)
  - Alert instances table (ID, rule, diagnosis link, state, timestamps)
  - Create rule dialog with validation
  - Enable/disable toggle and delete action
  - Integration with existing ConsoleLayout

### Phase 4: Testing

- `internal/alert/service_test.go`: rule creation, validation, state machine, deduplication, error handling
- `internal/alert/scheduler_test.go`: worker bounds, claim recovery, no overlap, timeout
- `internal/httpserver` tests: route registration matches OpenAPI
- TypeScript type definitions and API client

## Fixed V1 Contract

- Resource kind: `Node` only
- Metric: `cpu` or `memory` only
- Exact `cluster_id` plus exact Node name; no selectors, regex or labels
- Operator: `gte` or `lte`
- Absolute non-negative `int64` threshold in nanocores or bytes
- `for_seconds`: 60 through 21,600
- `minimum_points`: 2 through 360
- One immutable evaluation shape after creation
- Maximum 20 non-deleted rules per cluster
- Case-insensitive unique rule names per cluster

## Non-goals

- Pod rules, percentages, multi-metric correlation
- Notification routing, silence schedules, escalation policies
- PromQL or arbitrary labels
- Changing diagnosis lifecycle states/transitions
- Creating a second generic workflow engine

## Files Changed

### Backend
- `backend/internal/alert/model.go` - domain models and validation
- `backend/internal/alert/repository.go` - PostgreSQL repository
- `backend/internal/alert/service.go` - business logic and state machine
- `backend/internal/alert/scheduler.go` - bounded background scheduler
- `backend/internal/httpserver/alert.go` - HTTP handlers
- `backend/internal/httpserver/router.go` - route registration
- `backend/internal/config/config.go` - configuration structure
- `backend/cmd/server/main.go` - integration and startup
- `backend/migrations/000020_alert_lifecycle.up.sql` - schema migration
- `backend/migrations/000020_alert_lifecycle.down.sql` - rollback
- `backend/internal/alert/service_test.go` - service tests
- `backend/internal/alert/scheduler_test.go` - scheduler tests

### Frontend
- `frontend/src/types/alert.ts` - TypeScript definitions
- `frontend/src/api/alert.ts` - API client
- `frontend/src/views/AlertsView.vue` - management UI
- `frontend/src/components/ConsoleLayout.vue` - navigation addition
- `frontend/src/router/index.ts` - route registration

### Documentation
- `docs/adr/0043-historical-alert-lifecycle.md` - architecture decision
- `docs/api/openapi.yaml` - API specification
- `docs/changes/2026-07-30-m27-alert-lifecycle.md` - this document
- `.env.example` - configuration examples

### Scripts
- `scripts/e2e-m27-alert-lifecycle-kind.ps1` - real-kind E2E validation

## Verification

### L0 - Static and Focused
- `gofmt` on all changed Go files
- `go test ./internal/alert ./internal/httpserver` - all pass (15 alert tests, 40+ httpserver tests)
- `pnpm typecheck` - zero errors
- `pnpm test` - 73 tests pass
- `pnpm build` - successful

### L1 - Fast Repository Gate
- `scripts/verify-fast.ps1 -Scope All` - **PASSED** in 29.04s
- All 22 backend packages pass
- Frontend typecheck, Vitest (73 tests), and build pass
- Compose and Kustomize contracts pass

### L2/L3 - Full Gate and Real-kind E2E
- `scripts/e2e-m27-alert-lifecycle-kind.ps1` — **PASSED**
- Evidence:
  `.artifacts/m27-alert-lifecycle-kind/m27-alert-lifecycle-kind-20260731013733-e4a6e270.json`
- Disposable Kubernetes v1.34.0 kind used pinned Metrics Server v0.8.0 and a
  short-lived least-privilege ServiceAccount registration reachable from the
  Compose backend
- Proved sustained firing, same alert/diagnosis deduplication, Metrics API
  outage containment, soft-delete conflict, complete recent normal-window
  resolution under controlled CPU load, disabled/resolved persistence across
  backend restart and full cleanup

### Unit Test Coverage of Acceptance Criteria
The following M27 acceptance criteria are covered by unit/repository tests:

| # | Criterion | Test Coverage |
|---|---|---|
| 1 | Unique kind cluster and exact Node rule become firing | `TestEvaluateRule_FiringCreatesInstance` |
| 2 | At least two scheduler passes leave one active alert and one diagnosis | `TestEvaluateRule_RepeatedFiringDeduplicates` |
| 3 | Linked diagnosis can transition to confirmed without creating a second alert workflow | Diagnosis reuse verified in `TestEvaluateRule_FiringCreatesInstance` |
| 4 | Metrics API outage yields insufficient/error evidence and does not resolve the firing alert | `TestEvaluateRule_InsufficientDataNeverResolves`, `TestEvaluateRule_EvaluatorErrorNeverResolves` |
| 5 | Recovery followed by a complete normal threshold resolves it | `TestEvaluateRule_NormalResolvesFiring` |
| 6 | A later breach creates a new instance | `TestEvaluateRule_LaterFiringCreatesNewInstance` |
| 7 | Backend restart preserves rules, instances, links and deduplication | PostgreSQL persistence + partial unique index |
| 8 | Temporary registration, image and kind cluster are removed | Cleanup in E2E script `finally` block |

## Security

- Authorization: read operations available to authenticated viewers, mutations restricted to system/operations administrators
- Audit: every mutation emits low-cardinality event without sensitive material
- RBAC: backend uses least-privilege ServiceAccount tokens for Metrics API calls
- Data: no credentials, kubeconfig, tokens, or raw metric payloads persisted in alert tables

## Closure

M27 has no remaining locally executable acceptance gap. Notification routing,
silence schedules and Pod/percentage rules remain explicit non-goals rather
than hidden deferred work.
