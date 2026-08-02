# M76: HPA Scaling-Posture Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `b405730`; CI `30735594845` success)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest 114 / vite build) green

## Summary

Adds a read-only HPA scaling-posture analyzer to the optimization center. It
evaluates `autoscaling/v2` HorizontalPodAutoscalers already collected from a
cluster and emits `internal/finding`-shaped findings — no cluster mutation,
per ADR 0004.

Rules (`internal/hpa` pure `Evaluate`):
- **missing target metric** → `warning` (Kubernetes falls back to an implicit
  80% CPU target, often surprising operators).
- **current replicas at `maxReplicas`** → `warning` (no headroom to scale out
  under load).
- **`maxReplicas` ≤ 2** → `info` (thin burst margin).
- **current utilization over target** → `warning`; **under half the target** →
  `info` (over-provisioned). Percentage comparison is skipped for `pods` /
  `external` / `object` metrics (no utilization ratio available).

## Files Changed

### New Files

- `backend/internal/hpa/model.go` — `Status` (hpa_total, at_max_total,
  over_target_total, by_severity, findings) + `Inputs` (per-HPA observed
  target/current/max replicas + metric kind + current utilization ratio).
- `backend/internal/hpa/service.go` — pure `Evaluate(clusterID, Inputs,
  observedAt)`; findings sorted stably by severity then code.
- `backend/internal/hpa/service_test.go` — 7 table groups covering each rule
  branch and the pods/custom-metric skip path.
- `frontend/src/views/OptimizationView.vue` — new "HPA 扩缩容" tab (metric
  cards: HPA 数 / 触顶数 / 超目标数 / 预警数 + findings table with
  `max_replicas`).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectHPA` maps
  `autoscaling/v2` HPA (min/max/currentReplicas, first metric's resource
  utilization or pods average, `status.currentMetrics` utilization).
- `backend/internal/optimization/service.go` — delegates `CollectHPA`.
- `backend/internal/httpserver/optimization.go` — `hpaAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/hpa/analyze`
  (audit `optimization.hpa.analyze`).
- `docs/api/openapi.yaml` — `analyzeHPAPosture` operation + `HPAPostureStatus`
  schema.
- `frontend/src/types/optimization.ts` — `HPAStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeHPA(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — HPA client success/failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage 56.5%,
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest 114,
vite build 9.30s.

## Notes

- `json.Number` utilization cannot be cast directly to `float64`; use
  `strconv.ParseFloat(string(b), 64)`.
- Optimization center now exposes 9 analyzer tabs (cost / CIS / deprecated API /
  network / image / GitOps / capacity / policy / HPA).
