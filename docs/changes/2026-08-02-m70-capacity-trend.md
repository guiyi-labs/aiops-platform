# M70: Cluster Capacity-Trend Prediction Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `9b2e919`; CI backend 7-gate + frontend green)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest / vite build) green

## Summary

Adds a read-only cluster capacity-trend prediction analyzer to the optimization
center. It answers the question a platform operator asks before a capacity
incident: "at the current growth rate, when will this cluster run out of CPU or
memory?" It aggregates node allocatable capacity and a window of observed node
usage (from the metrics history store) into a per-resource (CPU / memory) time
series, fits a linear trend, and projects utilization forward to a horizon.
Resources predicted to saturate within the horizon are reported as findings.

The analyzer is pure (ADR 0004): `Evaluate` takes only an `Inputs` bundle
(collected read-only from the API server and metrics history) and returns a
`Status`. It never reaches the cluster, never queries a metrics backend, and
never mutates anything.

Rules (`internal/capacity` pure `Evaluate`):

- `CAPACITY_SATURATION_RISK` → critical when a resource's utilization is
  projected to reach a risky level (>= 80%) within the horizon, or to saturate
  (>= 100%) within a short window; otherwise **warning**. `Details` carry the
  projection (current %, slope, days-to-saturation).

Family: `capacity-trend`. `Inputs.HorizonDays` defaults to `DefaultHorizonDays`
(30) when 0.

## Files Changed

### New Files

- `backend/internal/capacity/model.go` — `Sample`, `ResourceTrend` (capacity +
  usage samples), `Inputs` (CPU / Memory trends + horizon), `Status`
  (cpu_capacity_nanocores, mem_capacity_bytes, cpu_current_pct, mem_current_pct,
  cpu_saturation_in_days, mem_saturation_in_days, by_severity, by_family,
  findings) + finding code / severities / family.
- `backend/internal/capacity/service.go` — pure `Evaluate(clusterID, Inputs, at)`
  with linear-trend fit and saturation projection; findings sorted by severity
  then code.
- `backend/internal/capacity/service_test.go` — 229-line table suite covering
  growing / flat / shrinking trends and the critical-vs-warning thresholds.
- `frontend/src/views/OptimizationView.vue` — new "容量预测" tab (CPU / memory
  current % + projected saturation-in-days cards + findings table).

### Modified Files

- `backend/cmd/server/main.go` — wires the capacity collector into startup.
- `backend/internal/optimization/collector.go` — `CollectCapacity` maps node
  allocatable + metrics-history usage windows (M65 read-only List).
- `backend/internal/optimization/service.go` — delegates `CollectCapacity`.
- `backend/internal/httpserver/optimization.go` — `capacityAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/capacity/analyze`
  (audit `optimization.capacity.analyze`).
- `docs/api/openapi.yaml` — `analyzeCapacityPosture` operation +
  `CapacityPostureStatus` schema.
- `frontend/src/types/optimization.ts` — `CapacityPostureStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeCapacity(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — client success / failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest, vite build green.

## Notes

- `-1` in `cpu_saturation_in_days` / `mem_saturation_in_days` means the resource
  is not growing toward saturation within the horizon.
- Linear projection is intentionally simple and explainable; a risky-threshold
  warning lets operators plan ahead before the critical window.
