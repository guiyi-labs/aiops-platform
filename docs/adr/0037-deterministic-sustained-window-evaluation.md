# ADR 0037: Deterministic Sustained-Window Evaluation

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend and platform operations

## Context

ADR 0034 established the bounded metrics history contract, ADR 0035 added the
background collector, and ADR 0036 exposed the authenticated read-only API.
Consumers can now fetch exact Node/Pod CPU and memory series, but they have
no deterministic way to answer whether a metric has been breachingsustainedly
across a time window. Prior logic only checked trailing consecutive breaches,
which missed non-trailing and multiple breach windows.

The platform needs a deterministic, reproducible evaluation layer that consumes
the exact-series contract and produces unambiguous state transitions (`firing`,
`normal`, `insufficient_data`) along with all detected sustained windows. This
enables trend consumers, diagnosis evidence, and future alert pipelines to
build on the same bounded evaluation primitive without inventing a query
language or unbounded label space.

## Decision

Add a deterministic sustained-window evaluation engine in
`backend/internal/metricshistory/evaluator.go` with the following contract:

1. **EvaluationRule**: A bounded rule with `operator` (`gte`/`lte`),
   `threshold`, `for_seconds`, and `minimum_points`. All fields are
   required; no defaulting occurs at the evaluation layer.
2. **SustainedWindow**: A struct recording `start_collected_at`,
   `end_collected_at`, `breaching_points`, and `span_seconds` for every
   contiguous breach sequence that meets or exceeds `minimum_points` and
   `for_seconds`.
3. **EvaluationResponse**: Returns `state` (firing/normal/insufficient_data),
   `breaching_points`, `observed_span_seconds`, `sustained_windows` (all
   detected windows), and `latest_firing_window` (the most recent firing
   window or null).
4. **Full-series scan**: The evaluator scans the entire point series, not
   just the trailing edge. It detects every breach window, sorts them
   chronologically, and determines the overall state from all windows.
5. **Bounded coverage gating**: Insufficient points (below `minimum_points`
   or sparse coverage) produce `insufficient_data`, never a false `normal`.
6. **No interpolation**: Missing collections remain missing. The evaluator
   never manufactures zero-valued points or interpolates gaps.

A companion authenticated route:

`GET /api/v1/clusters/{cluster_id}/metrics/history/evaluate`

accepts the same exact-series query parameters as the history read plus
`operator`, `threshold`, `for_seconds`, and `minimum_points`. It returns the
evaluation response with the same stable error mapping as ADR 0036.

The frontend Dashboard adds a trend consumer that fetches 6-hour CPU and
memory history for the selected cluster's top Node, renders SVG trend
charts, and displays the evaluation state badge alongside peak values and
coverage percentages.

## Consequences

- Every authenticated consumer gains one deterministic, bounded evaluation
  primitive without a query language or aggregation pipeline.
- Non-trailing and multiple breach windows are detected, enabling accurate
  incident post-mortems and sustained alerting.
- Trend consumers can render historical charts with evaluation state directly
  from the exact-series contract, eliminating ad-hoc threshold checks.
- The evaluator remains a pure function over the existing series contract;
  it does not add storage, indexing, or background evaluation.
- Future alert pipelines and diagnosis evidence can reuse the same
  evaluation response structure without inventing parallel logic.
- No PromQL, label selection, or downsampling is introduced. The exact-series
  boundary from ADR 0036 is preserved.