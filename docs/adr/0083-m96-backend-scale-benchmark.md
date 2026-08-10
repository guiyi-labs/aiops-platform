# ADR 0083 - M96 backend scale benchmark methodology

- Date: 2026-08-10
- Status: Accepted
- Milestone: M96
- Related: ADR 0082, ADR 0080, M50 metrics history, M91 virtual scroll

## Context

The deterministic `m96-v1` fixture establishes a shared 500 Node / 50k Pod /
100k Event input, but fixture counts alone do not prove bounded backend
behavior. M96 requires repeatable latency distributions, memory and goroutine
observations, plus explicit timeout, cancellation, pagination and backpressure
evidence. A single `go test -bench` line is insufficient because it omits
machine, runtime, commit and fixture identity.

## Decision

1. Add a fixture-backed benchmark runner that verifies the manifest before
   loading data and calls existing production contracts where available:
   `topology.Collector.DeriveEdges`, `globalsearch.Service.Search` and
   `metricshistory.EvaluateWindow`.
2. Measure topology derivation across all namespaces, global search, bounded
   Pod/Event pagination, history lookup/evaluation and an eight-slot bounded
   Pod stream. Each operation records 30 samples after three warmups and emits
   min/mean/P50/P95/P99/max.
3. Sample heap and goroutine counts during the run. Hard semantic invariants
   cover exact fixture counts, cancellation, deadline propagation, global
   search timeout reporting, queue capacity and goroutine return to baseline.
4. Emit versioned JSON plus Markdown with OS, architecture, Go version, CPU
   count, GOMAXPROCS, source commit, fixture config hash and dataset hash.
5. Keep latency and memory thresholds in report mode. Correctness invariants
   fail the command; observed performance values do not become fail-closed
   until at least two stable CI cycles establish runner variance.

## Consequences

- Results are attributable to a commit and exact dataset rather than terminal
  text. CI retains reports while discarding generated record streams.
- The in-memory fixture adapters benchmark deterministic backend logic and
  bounded query behavior; they do not replace a later database/API transport
  benchmark or claim production fleet capacity.
- New operations or threshold changes require a report schema/version review
  and an archived baseline comparison.
