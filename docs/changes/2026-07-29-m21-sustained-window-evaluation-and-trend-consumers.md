# M21 Phase 4: Deterministic Sustained-Window Evaluation and Trend Consumers

- Date: 2026-07-29
- Scope: sustained-window detection, evaluation API, frontend trend consumer
- Decision: ADR 0037

## Delivered

1. **Sustained-window evaluation engine** (`backend/internal/metricshistory/evaluator.go`):
   - `EvaluationRule` struct with bounded `operator` (gte/lte), `threshold`,
     `for_seconds`, and `minimum_points`.
   - `SustainedWindow` struct capturing `start_collected_at`, `end_collected_at`,
     `breaching_points`, and `span_seconds` for every detected breach window.
   - `EvaluationResponse` returning `state` (firing/normal/insufficient_data),
     all `sustained_windows`, and `latest_firing_window`.
   - Full-series scan detects non-trailing and multiple breach windows,
     replacing the previous trailing-only check.
   - Bounded coverage gating prevents false `normal` states on sparse data.

2. **Evaluation route** (`GET /api/v1/clusters/{cluster_id}/metrics/history/evaluate`):
   - Reuses the exact-series query contract from ADR 0036 plus evaluation
     parameters (`operator`, `threshold`, `for_seconds`, `minimum_points`).
   - Stable error mapping: `INVALID_QUERY`, `CLUSTER_NOT_FOUND`,
     `METRICS_HISTORY_QUERY_FAILED`.

3. **Frontend trend consumer** (`DashboardView.vue`):
   - 6-hour CPU and memory history for the selected cluster's top Node.
   - SVG trend charts with area fill and line stroke.
   - Peak value display, coverage percentage, and evaluation state badges.
   - Graceful fallback when history or evaluation data is unavailable.

4. **Test coverage** (`evaluator_test.go`):
   - `TestEvaluateWindowDetectsSustainedWindowsAcrossFullSeries`: verifies
     that non-trailing breach windows are detected.
   - `TestEvaluateWindowDetectsNonTrailingSustainedWindow`: confirms firing
     state when the sustained window is not at the series tail.
   - `TestEvaluateWindowFiltersShortOrSparseWindows`: ensures windows below
     minimum points or duration are filtered out.

## Verification

- **Backend**: `go test ./...` — 20 packages all pass (cached on second run).
- **Backend**: `go vet ./...` — no warnings.
- **Frontend**: `vue-tsc -b` — type-check passes clean.
- **Frontend**: `vitest run` — 16 test files, 66 tests all pass.

## Follow-up Fixes (same day)

1. **TypeScript alignment**: Fixed `topNodeName` computed to use `nodes.value[0]?.metadata.name`
   instead of the non-existent `.name` property on `NodeResource`.
2. **Meaningful evaluation thresholds**: CPU threshold set to 80% of node allocatable
   nanocores; memory threshold set to 85% of node allocatable bytes. Previously used
   `threshold: 0` which trivially fired for all points.
3. **Minimum `for_seconds`**: Updated evaluation calls from `forSeconds: 0` to
   `forSeconds: 60` to satisfy backend validation (minimum 60s).
4. **Responsive CSS**: Added trend-grid breakpoint for 900px (single column) and 720px
   (reduced padding/chart height).
5. **OpenAPI spec parity**: Updated `docs/api/openapi.yaml` with `SustainedWindow` schema,
   `sustained_windows` and `latest_firing_window` fields in the evaluation response,
   and corrected the endpoint description from trailing-only to full-series scan.
6. **Type definitions**: Added `MetricSustainedWindow` interface to frontend
   `types/metrics-history.ts` and extended `MetricHistoryEvaluationResponse`.

## Boundary

This phase does not add background evaluation, alert persistence, multi-metric
correlation, or PromQL. The evaluator remains a synchronous, bounded function
over the existing exact-series contract. Future M21 phases may add alert
pipelining and diagnosis evidence integration on top of this primitive.

---

# M21 Phase 5 — Diagnosis Evidence Integration

## Deliverable

- **ADR 0038**: Design for integrating sustained-window evaluation evidence
  into the diagnosis lifecycle (`docs/adr/0038-diagnosis-evidence-integration-for-sustained-metric-breaches.md`).
- **New diagnosis rule**: `node.metric_sustained_breach.v1` consumes an
  `EvaluationResponse` and produces a structured `Record` with typed
  `Evidence` for each `SustainedWindow` plus a summary evidence entry.
- **Service extension**: `DiagnoseNodeMetrics(ctx, clusterID, name, metric, rule)`
  on the diagnosis `Service` queries a 6-hour window through the
  `MetricEvaluator` interface, evaluates the sustained-breach rule, and
  persists the record if the state is `firing`.
- **Interface extension**: `MetricEvaluator` interface (`Evaluate(ctx, query)`)
  added to the diagnosis package so the service can consume the metrics
  history evaluator without tight coupling. The `Source` interface gains a
  metrics history method for future rules.
- **Evidence shape**: Each sustained window maps to an `Evidence` entry with
  `type: "metric_sustained_breach"`, capturing threshold, operator,
  `for_seconds`, `minimum_points`, breaching points, span seconds, and
  time bounds. A summary entry captures overall evaluation state, coverage,
  and breach statistics.
- **Severity mapping**: CPU breaches → `high`, memory breaches → `medium`.
  These align with the Dashboard default thresholds (80% CPU, 85% memory).
- **Rule content**: The rule emits metric-specific root causes and
  recommendations for CPU- and memory-driven breaches, with generic
  fallbacks for future metric types.

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `backend/internal/diagnosis/metric_breach.go` | New | Rule evaluation for sustained metric breach → `Record` |
| `backend/internal/diagnosis/metric_breach_test.go` | New | Unit tests for the new rule |
| `backend/internal/diagnosis/service.go` | Modify | Added `MetricEvaluator` interface, `WithMetricEvaluator` setter, `DiagnoseNodeMetrics` method |
| `docs/adr/0038-diagnosis-evidence-integration-for-sustained-metric-breaches.md` | New | Design decision |
| `docs/changes/2026-07-29-m21-sustained-window-evaluation-and-trend-consumers.md` | Modify | Phase 5 deliverable logging |

## Test coverage

All 18 backend packages pass (`go test ./...`). The new rule adds
`TestEvaluateSustainedMetricBreachFiring`, `TestEvaluateSustainedMetricBreachMemorySeverity`,
`TestEvaluateSustainedMetricBreachNoMatchNormal`,
`TestEvaluateSustainedMetricBreachNoMatchInsufficientData`,
`TestEvaluateSustainedMetricBreachMultipleWindows`, and
`TestEvaluateSustainedMetricBreachSummaryIncludesDetails`.

## Boundary

This phase does not add background evaluation, alert pipelining, multi-metric
correlation, deduplication, or automatic rule triggering. The integration is
a synchronous bridge: diagnosis records are created only when explicitly
requested through the diagnosis API. Background alert generation and
correlation remain future M21 phases.

---

# M21 Phase 6 — Node Metrics Diagnosis HTTP Endpoint

## Deliverable

- **Dependency injection wiring**: `main.go` chains `.WithMetricEvaluator(metricsHistoryService)`
  onto the diagnosis service so `DiagnoseNodeMetrics` can query historical
  evaluation through the `MetricEvaluator` interface.
- **HTTP handler**: `diagnoseNodeMetrics(c *gin.Context)` parses a typed JSON
  request (`name`, `metric ∈ {node_cpu, node_memory}`, `operator`, `threshold`,
  `for_seconds`, optional `minimum_points`), validates each field, maps the
  public metric names (`node_cpu`/`node_memory`) to internal constants, and
  invokes the diagnosis service.
- **Error mapping**: The handler maps `diagnosis.ErrNoRuleMatch` to HTTP 422
  `NO_RULE_MATCH` ("no sustained metric breach was detected"), `ErrDisabled`
  to 409, `ErrClusterNotFound` and `metricshistory.ErrClusterNotFound` to
  404, `ErrInvalidQuery` / `ErrInvalidEvaluation` to 400, and all other
  errors to 502.
- **Route registration**: `POST /api/v1/clusters/{cluster_id}/diagnoses/node_metrics`
  mounted under `resourceRoutes` with cluster context, alongside the
  existing generic `/diagnoses` endpoint.
- **OpenAPI parity**: `DiagnoseNodeMetricsRequest` schema and the new
  operation with `operationId: diagnoseNodeMetrics` added to
  `docs/api/openapi.yaml`. The request requires `name`, `metric`,
  `operator`, `threshold`, `for_seconds`; `minimum_points` defaults to 2.
- **Handler tests**: `diagnosis_node_metrics_test.go` covers:
  `TestDiagnoseNodeMetricsHandlerParsesRequestAndCreatesRecord`,
  `TestDiagnoseNodeMetricsHandlerDefaultMinimumPoints`,
  `TestDiagnoseNodeMetricsHandlerRejectsInvalidInput` (9 sub-cases),
  `TestDiagnoseNodeMetricsHandlerNoRuleMatchForNormalState`, and
  `TestDiagnoseNodeMetricsHandlerMapsEvaluationErrors` (4 sub-cases).
  Uses stubs `diagnosisRepositoryStub`, `fakeMetricEvaluator`.

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `backend/cmd/server/main.go` | Modify | Wire `WithMetricEvaluator(metricsHistoryService)` |
| `backend/internal/httpserver/diagnosis.go` | Modify | Add `diagnoseNodeMetricsRequest`, `diagnoseNodeMetrics` handler, public→internal metric name mapping, metricshistory imports |
| `backend/internal/httpserver/router.go` | Modify | Register `POST /diagnoses/node_metrics` under cluster routes |
| `backend/internal/httpserver/diagnosis_node_metrics_test.go` | New | Handler tests with repository + evaluator stubs |
| `docs/api/openapi.yaml` | Modify | New operation + `DiagnoseNodeMetricsRequest` schema |
| `docs/changes/2026-07-29-m21-sustained-window-evaluation-and-trend-consumers.md` | Modify | Phase 6 deliverable logging |

## Test coverage

All 18 backend packages pass. The new handler file adds ~15 test cases
including sub-tests, exercising field validation, defaulting, 422 on
`normal` evaluation state, and all five error branches.

## Follow-up fixes applied during test

1. **Public→internal metric name mapping**: The handler accepts
   `node_cpu` / `node_memory` (OpenAPI contract) and maps to internal
   `metricshistory.MetricCPU` / `MetricMemory` before forwarding to the
   diagnosis service. Without this mapping, every metric query returned
   `ErrInvalidQuery`.
2. **JSON numeric type**: Test assertions on `evidence.Content["window_index"]`
   compare against `float64` because `json.Unmarshal` into `map[string]any`
   yields float64 for numbers.
3. **OpenAPI YAML**: Removed stray `}` that survived the one-line →
   multi-line route conversion, so the `openapi_route_test` route
   registration parity gate continues to pass.

## Boundary

This phase only exposes the synchronous Node CPU/memory breach diagnosis
entry point. Pod-level metric diagnosis, recurring/background evaluation,
deduplication, alert grouping, multi-metric correlation, and frontend
trigger buttons remain future phases.

---

# M21 Closure — Frontend Integration and Final Acceptance

## Deliverable

- **Frontend TypeScript types**: Added `DiagnoseNodeMetricsRequest` to
  `frontend/src/types/diagnosis.ts` with fields `name`, `metric`
  (`'node_cpu' | 'node_memory'`), `operator` (`'gte' | 'lte'`),
  `threshold`, `for_seconds`, and optional `minimum_points`.
- **Frontend API function**: `diagnoseNodeMetrics(token, clusterID, request)`
  in `frontend/src/api/diagnosis.ts` wraps
  `POST /api/v1/clusters/{id}/diagnoses/node_metrics` and returns
  `DiagnosisRecord`.
- **DashboardView trigger buttons**: In the trend consumers header,
  conditionally renders "诊断 CPU 突破" / "诊断内存突破" buttons only
  when the corresponding evaluation state is `firing`. Clicking triggers
  the synchronous diagnosis, and the result is displayed inline with a
  summary, rule ID, severity, and a click-to-navigate link to the
  diagnosis queue. Error handling covers `NO_RULE_MATCH` ("过去 6 小时内
  未检测到持续突破阈值的指标") and generic errors.
- **Loading and result states**: New refs `diagnoseMetricsLoading`,
  `diagnoseMetricsError`, and `diagnoseMetricsRecord` manage the
  lifecycle. The button shows "诊断中..." while loading and the result
  panel shows a success card or an inline error.
- **Responsive CSS**: `.trend-actions`, `.trend-action-btn`, and
  `.trend-diagnosis-result` styles added. Responsive adjustments at the
  720px breakpoint wrap buttons and results for narrow viewports.
- **Verification**: `vue-tsc -b` passes with zero errors; `vitest run`
  passes all 66 tests across 16 test files (including 11 diagnosis
  tests and 3 metrics-history tests).

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `frontend/src/types/diagnosis.ts` | Modify | Added `DiagnoseNodeMetricsRequest` interface |
| `frontend/src/api/diagnosis.ts` | Modify | Added `diagnoseNodeMetrics` API function |
| `frontend/src/views/DashboardView.vue` | Modify | Import new types/functions, add state, handler, UI buttons and result display |
| `frontend/src/styles/base.css` | Modify | Added `.trend-actions`, `.trend-action-btn`, `.trend-diagnosis-result` styles and 720px responsive rules |
| `docs/changes/2026-07-29-m21-sustained-window-evaluation-and-trend-consumers.md` | Modify | Closure entry |

## Test coverage

| Layer | Command | Result |
|-------|---------|--------|
| Backend | `go test ./... -count=1` | 18 packages pass |
| Frontend type-check | `npm run typecheck` (vue-tsc -b) | Zero errors |
| Frontend unit | `npm run test` (vitest run) | 66 tests across 16 files pass |

## M21 Final Status: ACCEPTED

### What M21 delivers

M21 ("Historical Observability and Alert Evidence") is complete with
six accepted phases:

1. **Phase 1** — Bounded PostgreSQL time-series storage with sparse-series
   contract, retention policy, and automatic cleanup.
2. **Phase 2** — In-process metrics collector that scrapes Node CPU/memory
   and Pod CPU/memory with configurable sampling and graceful shutdown.
3. **Phase 3** — Authenticated `GET /metrics/history` query endpoint with
   exact series, coverage metadata, and pagination.
4. **Phase 4** — Deterministic sustained-window evaluation engine (ADR 0037)
   that scans the full series for all breach windows, plus Dashboard
   trend consumer with SVG charts and evaluation state badges.
5. **Phase 5** — Diagnosis evidence integration (ADR 0038) with rule
   `node.metric_sustained_breach.v1`, mapping `EvaluationResponse` to
   structured `DiagnosisRecord` with typed `Evidence` per window.
6. **Phase 6** — HTTP endpoint `POST /diagnoses/node_metrics` with full
   field validation, metric name mapping, error mapping, OpenAPI parity,
   handler tests, and frontend trigger buttons on the Dashboard.

### Explicit out-of-scope (future phases)

The following capabilities are intentionally NOT part of M21 and will
be addressed in future roadmap items:

- Background / periodic evaluation with alert pipelining
- Multi-metric correlation and deduplication
- Pod-level metric sustained-breach diagnosis (same primitive, different resource kind)
- Automatic diagnosis rule triggering on a schedule
- Alert grouping and aggregation across clusters
- SLO/SLA enforcement with automatic escalation
- PromQL or flexible query language support
- Long-term historical trend analysis (beyond 6h window)

### Boundary

M21 establishes the deterministic, synchronous foundation for sustained
metric breach detection and diagnosis evidence. The human-in-the-loop
trigger model (operator clicks "诊断 CPU 突破" when the UI shows a
firing evaluation) is a deliberate design choice that keeps the system
simple, testable, and safe before adding any background automation. All
future alerting extensions will build on the contracts, interfaces, and
evidence shape defined here.