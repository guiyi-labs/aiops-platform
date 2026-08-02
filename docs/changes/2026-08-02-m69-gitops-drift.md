# M69: GitOps Configuration-Drift Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `f014afa`; CI backend 7-gate + frontend green)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest / vite build) green

## Summary

Adds a read-only GitOps configuration-drift analyzer to the optimization center.
It answers the question a platform operator asks between reconciles: "has
anything in the cluster diverged from what GitOps last applied?" The signal is
the standard `kubectl.kubernetes.io/last-applied-configuration` annotation,
which records the exact manifest a GitOps tool (kubectl apply / Kustomize /
Helm / Flux / Argo CD) wrote. When the live object no longer matches that
record, the resource has drifted and GitOps can no longer cleanly reconcile it.

The analyzer is pure and offline (ADR 0004): it only reasons over an
observation bundle (M65 collector via read-only List) and never mutates cluster
state, never talks to a Git provider, and never re-applies anything.

Rules (`internal/gitopsdrift` pure `Evaluate`):

- `GITOPS_DRIFT_DETECTED` → warning: the live object no longer matches the
  `last-applied-configuration` annotation, so it has diverged from what GitOps
  last applied.
- `GITOPS_UNMANAGED_RESOURCE` → info: a resource lives in a GitOps-managed
  namespace but carries no `last-applied-configuration` annotation, so drift
  cannot be reconciled or even detected for it.

Family: `gitops-drift`. Detected managers reported in `Details["manager"]`:
`kubectl` / `flux` / `argocd`.

## Files Changed

### New Files

- `backend/internal/gitopsdrift/model.go` — `ManagedResource` (kind / namespace
  / name / manager / raw `AppliedConfig` / raw `LiveBody`), `Inputs`
  (resources + managed namespaces), `Status` (resources_total,
  drifted_resources, unmanaged_resources, by_severity, by_family, findings) +
  finding codes / severities / family.
- `backend/internal/gitopsdrift/service.go` — pure
  `Evaluate(clusterID, Inputs, at)`; findings sorted by severity then code.
- `backend/internal/gitopsdrift/service_test.go` — 212-line table suite covering
  drift detection and the unmanaged-resource path.
- `frontend/src/views/OptimizationView.vue` — new "GitOps 漂移" tab (drift /
  unmanaged cards + findings table with manager).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectGitOps` maps managed
  resources and detects GitOps-managed namespaces (M65 read-only List).
- `backend/internal/optimization/service.go` — delegates `CollectGitOps`.
- `backend/internal/httpserver/optimization.go` — `gitopsAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/gitops/analyze`
  (audit `optimization.gitops.analyze`).
- `docs/api/openapi.yaml` — `analyzeGitOpsPosture` operation +
  `GitOpsPostureStatus` schema.
- `frontend/src/types/optimization.ts` — `GitOpsPostureStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeGitOps(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — client success / failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest, vite build green.

## Notes

- Drift is detected by comparing the live `spec` (or `data` for ConfigMap /
  Secret) against the `last-applied-configuration` JSON; the comparison is
  structural, not a simple string equality.
- Findings reuse the canonical `internal/finding` contract and render uniformly
  with the other optimization analyzers.
