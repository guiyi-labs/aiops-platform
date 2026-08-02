# M78: Ingress Exposure-Surface Audit Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `675ca54`; CI `30744717375` success)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest 118 / vite build) green

## Summary

Adds a read-only Ingress exposure-surface audit analyzer — the final analyzer
in the M76–M78 trio requested via "加吧，先把这些内容做完做好做齐全先". It
evaluates Ingresses (already collected) and emits `internal/finding`-shaped
findings — no cluster mutation, per ADR 0004.

Rules (`internal/ingressposture` pure `Evaluate`):
- **no TLS** → `warning` (cleartext traffic to the service).
- **backend Service does not exist** → `warning` (dead backend / `dead_backend_count`).
- **wildcard host (`*`)** → `info` (over-broad exposure).
- **no explicit `ingressClassName`** → `info` (controller resolved implicitly).

## Files Changed

### New Files

- `backend/internal/ingressposture/model.go` — `Status` (ingresses_total,
  no_tls_count, dead_backend_count, by_severity, findings) + `Inputs`
  (per-Ingress namespace/name/ingressClassName/`HasTLS` + resolved backend
  Service existence).
- `backend/internal/ingressposture/service.go` — pure `Evaluate(clusterID,
  Inputs, observedAt)`; findings sorted stably by severity then code.
- `backend/internal/ingressposture/service_test.go` — table groups covering
  each rule branch (TLS coverage, wildcard host, missing class, dead backend).
- `frontend/src/views/OptimizationView.vue` — new "Ingress 暴露面" tab (Globe
  icon; metric cards + findings table; `v-else` closes the tab chain).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectIngress` maps
  `networking.k8s.io/v1` Ingress (`Namespace`, `Name`, `*string`
  `ingressClassName`, `HasTLS`) and validates the referenced backend Service.
- `backend/internal/optimization/service.go` — delegates `CollectIngress`.
- `backend/internal/httpserver/optimization.go` — `ingressAnalyze` handler.
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/ingress/analyze`
  (audit `optimization.ingress.analyze`).
- `docs/api/openapi.yaml` — `analyzeIngressPosture` operation +
  `IngressPostureStatus` schema.
- `frontend/src/types/optimization.ts` — `IngressStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeIngress(token, clusterId)` with
  findings / by_severity / by_family null-safety.
- `frontend/src/api/optimization.test.ts` — Ingress client success/failure
  cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage 56.8%
(ingressposture 91.7%), 5 binaries built, golangci-lint 0 (per-package on
Windows; see note), eslint 0, vue-tsc 0, vitest 118, vite build success.

## Notes

- `golangci-lint v2` on Windows mis-parses `./...` ("no go files to analyze")
  due to `GOTOOLCHAIN` auto-switch failing offline; run per-package with
  `GOTOOLCHAIN=local GOCACHE=<dir> golangci-lint run --config ../.golangci.yml
  ./internal/<pkg>/...` (CI on Linux is unaffected).
- Optimization center now exposes **11 analyzer tabs**: cost / CIS / deprecated
  API / network / image / GitOps / capacity / policy / HPA / PDB / Ingress.
