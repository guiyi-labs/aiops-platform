# ADR 0077: FinOps Right-sizing Advisor

- Date: 2026-08-01
- Status: Accepted
- Related milestones: M61, ADR 0004 (bounded read-only Kubernetes gateway),
  M21 (metrics history collection)
- Supersedes: none

## Context

M21 already collects precise CPU/memory time series per Pod/container, and M29
produces a capacity situation. These signals are under-exploited: the platform
reports *that* capacity exists but does not turn it into **cost-reducing
action**. Industry tooling (Crane, OpenCost, Goldilocks) shows the high-value,
low-risk pattern: compare requested vs observed usage and suggest right-sized
requests/limits, plus a dollar estimate of reclaimable waste.

Design constraints (same as ADR 0076):

1. Developed in the isolated `opt/m61-m63` worktree; does not touch the M46–M60
   main working tree.
2. Read-only, no VPA, no autopilot, no admission control — consistent with
   ADR 0004. The advisor only *suggests*; it never changes a workload.
3. Reuses existing data. Units are nanocores/bytes, identical to
   `metricshistory`, so no new collector or data source is required.

## Decision

1. **New package `internal/finops`.** Pure, read-only right-sizing advisor.
   - `model.go`: `Quantity{CPURequest,CPULimit,MemRequest,MemLimit}` in
     nanocores/bytes (`Unset = -1`); `ContainerInput` (spec requests/limits +
     observed p50/p95/max usage + replicas); `CostRate{PerCoreMonth,PerGBMonth}`;
     `Recommendation`; `WasteSummary` with `ToFindings()` for uniform rendering.
   - `advisor.go`: `Recommend(clusterID, []ContainerInput, rate) → WasteSummary`.
     Per container: suggested request = observed p95 × headroom (CPU 1.15,
     memory 1.20, rounded to friendly steps); suggested limit echoed or gently
     suggested (never tightened). Waste = idle requested − p95, aggregated across
     replicas, priced by `CostRate`. Severity: request ≥ 4× p95 → critical,
     ≥ 2× → warning; missing request → info `MISSING_*_REQUEST`.
   - `service.go`: `QuantityFromResourceMap` bridges Kubernetes resource strings
     (`"1000m"`, `"256Mi"`) to `Quantity`, so callers build `ContainerInput`
     without re-implementing parsing. `NewService(rate, repo)` + `Evaluate`
     orchestrates and (optionally) persists via `Repository`.
   - `repository.go`: `Repository` interface + in-memory implementation.
2. **Read-only, suggestion-only.** Nothing is written back to the cluster.
   `CostRate` defaults are illustrative; operators must supply real cloud rates.
3. **Decoupling from the broken baseline.** Like ADR 0076, `finops` depends only
   on `internal/finding` (and `k8s.io/apimachinery` for quantity parsing), not on
   `cluster`/`kubernetes`, so it builds and tests independently of the M33
   `internal/cluster` defect.

## Non-goals

- No Vertical Pod Autoscaler, no Prometheus, no in-cluster agent.
- No automatic mutation of requests/limits or rollout of changes.
- No cross-cluster billing aggregation beyond the per-cluster rollup.
- No fix for the baseline `internal/cluster` compile defect (out of scope).

## Verification

- `go test ./internal/finops/` passes: over-provisioned CPU/memory (critical,
  waste ≈ $81 / ≈ $6.44), missing request (info, zero waste, suggested value
  produced), right-sized (no recommendation), and no-usage (skipped).
- `QuantityFromResourceMap` round-trips `"1000m"`/`"256Mi"` and treats missing
  keys as `Unset`.
