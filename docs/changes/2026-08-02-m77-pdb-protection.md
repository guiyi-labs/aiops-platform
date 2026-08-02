# M77: PodDisruptionBudget Protection Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `40877fc`; CI `30737114826` success)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest 116 / vite build) green

## Summary

Adds a read-only PodDisruptionBudget protection analyzer. It evaluates PDBs
and their associated workloads (already collected) and emits
`internal/finding`-shaped findings — no cluster mutation, per ADR 0004.

Rules (`internal/pdb` pure `Evaluate`):
- **no PDB for a controlled workload** → `info` (voluntary disruptions can
  evict freely).
- **`minAvailable` / `maxUnavailable` missing** → `warning` (no eviction
  protection floor).
- **`maxUnavailable = 100%`** → `warning` (workload can be fully evicted).
- **`kubernetes.io/allow-voluntary-disruptions: "true"` annotation** → `info`
  (PDB intentionally bypassed).

`minAvailable` / `maxUnavailable` are Kubernetes `IntOrString` (number or
string); the model decodes them via `json.RawMessage` + `rawToText` so both
forms parse without dropping the whole object.

## Files Changed

### New Files

- `backend/internal/pdb/model.go` — `Status` (pdb_total, protected_total,
  by_severity, findings) + `Inputs` (per-PDB `DisruptionsAllowed`,
  `minAvailable`/`maxUnavailable` raw, allow-voluntary flag, associated
  workload kind).
- `backend/internal/pdb/service.go` — pure `Evaluate(clusterID, Inputs,
  observedAt)`; findings sorted stably by severity then code.
- `backend/internal/pdb/service_test.go` — 11 table groups covering each rule
  branch (incl. numeric vs string `IntOrString`).
- `frontend/src/views/OptimizationView.vue` — new "PDB 保护" tab (ShieldAlert
  icon; metric cards + findings table).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectPDB` maps PDB
  `Status.DisruptionsAllowed` + associated workload type.
- `backend/internal/optimization/service.go` — delegates `CollectPDB`.
- `backend/internal/httpserver/optimization.go` — `pdbAnalyze` handler.
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/pdb/analyze`
  (audit `optimization.pdb.analyze`).
- `docs/api/openapi.yaml` — `analyzePDBPosture` operation + `PDBPostureStatus`
  schema.
- `frontend/src/types/optimization.ts` — `PDBStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzePDB(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — PDB client success/failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage 56.6%,
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest 116,
vite build 9.2s.

## Notes

- First implementation decoded `IntOrString` into `string`, which failed on
  JSON numbers (`1`) and silently skipped the object; switched to
  `json.RawMessage` + `rawToText`.
- Optimization center now exposes 10 analyzer tabs (adds PDB to the M76 set).
