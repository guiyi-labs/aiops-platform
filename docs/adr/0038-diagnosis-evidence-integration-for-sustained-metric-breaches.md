# ADR 0038: Diagnosis Evidence Integration for Sustained Metric Breaches

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend and platform operations

## Context

ADR 0037 established a deterministic sustained-window evaluation engine that
produces `EvaluationResponse` with `SustainedWindow` evidence across the
full metrics series. The diagnosis package (ADR M18) evaluates rules against
Kubernetes resources and produces `Record` objects with typed `Evidence`.
Consumers need a deterministic bridge between the metrics evaluation and
diagnosis evidence so that sustained CPU or memory breaches on Nodes and
Pods surface as first-class diagnosis records in the existing workflow.

Without this bridge, the evaluation results remain isolated at the API layer
and cannot participate in the diagnosis lifecycle (state transitions, SLA,
assignment, feedback, audit). Operators would have to manually correlate
metrics history with diagnosis records, defeating the purpose of the
bounded evaluation primitive.

## Decision

1. **New diagnosis rule**: Add `RuleNodeSustainedMetricBreach`
   (`node.metric_sustained_breach.v1`) that consumes an `EvaluationResponse`
   and produces a `Record` with structured `Evidence`. The rule fires only
   when the evaluation state is `firing` with at least one sustained window.

2. **Evidence shape**: Each `SustainedWindow` maps to an `Evidence` entry
   with `type: "metric_sustained_breach"`, `source` identifying the metric
   series, and `content` capturing `threshold`, `operator`, `for_seconds`,
   `breaching_points`, `span_seconds`, `start_collected_at`, and
   `end_collected_at`. A summary evidence entry captures the overall
   evaluation state and coverage.

3. **Service extension**: Add `DiagnoseNodeMetrics(ctx, clusterID, name,
   metric, rule)` to the diagnosis `Service`. It takes a `metricshistory.Service`
   reference, queries the last 6 hours of series data for the named Node,
   runs the evaluation, and returns a persisted `Record` if the state is
   `firing`. The method never fabricates a record from `insufficient_data`
   or `normal` states.

4. **Source interface**: Extend the diagnosis `Source` interface with
   `NodeMetricsHistory(ctx, clusterID, name, metric, from, to)` that returns
   a `metricshistory.SeriesResponse`. The existing Kubernetes gateway
   implementation will delegate to the metrics history service.

5. **No background evaluation**: The rule is evaluated synchronously when
   invoked through the diagnosis API. Background alert pipelining is left
   to a future phase. This keeps the boundary clean: evaluation is a
   function of explicit queries, not an unbounded watch loop.

6. **Severity mapping**: Sustained CPU breaches at ≥80% of allocatable
   capacity map to `severity: "high"`. Memory breaches at ≥85% map to
   `severity: "medium"`. These thresholds align with the frontend Dashboard
   defaults from M21 Phase 4.

## Consequences

- Sustained metric breaches become first-class diagnosis records with full
  lifecycle support (state transitions, SLA, assignment, feedback).
- The diagnosis evidence model gains a new `metric_sustained_breach` type
  that future rules (NodePressure, OOMKilled correlation) can compose with.
- No new storage, indexes, or background workers are introduced. The
  integration is purely a synchronous bridge between existing packages.
- The `Source` interface gains a metrics history method, establishing a
  clean seam for future diagnosis rules that require historical evidence.
- Alert pipelining (background evaluation, multi-metric correlation,
  deduplication) remains a future phase. This ADR only covers the
  evidence mapping and synchronous diagnosis entry point.