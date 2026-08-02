# M71: Policy-as-Code Workload Posture Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `15b8f12`; CI backend 7-gate + frontend green)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest / vite build) green

## Summary

Adds a read-only "policy-as-code" posture evaluator of workload manifests to the
optimization center. Instead of shipping a Rego / OPA engine, the analyzer
evaluates a small, opinionated rule set (resource requests / limits, security
context, host access, and probes) that mirrors what KubeSphere and similar
consoles gate on by default. The rule set lives in `service.go` and is
trivially extendable.

The analyzer is pure and offline (ADR 0004): `Evaluate` takes only an
observation bundle (collected read-only from the API server) and returns a
`Status`. It never reaches the cluster, never applies anything, and never
mutates any resource.

Rules (`internal/policy` pure `Evaluate`), grouped by family:

- **resources** (requests / limits presence)
  - `POLICY_CONTAINER_NO_CPU_REQUEST` → warning
  - `POLICY_CONTAINER_NO_MEMORY_REQUEST` → warning
  - `POLICY_CONTAINER_NO_RESOURCE_LIMITS` → warning (missing limits is the most
    common drift from a production baseline; missing requests silently degrades
    QoS)
- **security** (privileged / escalation / run-as-root)
  - `POLICY_CONTAINER_PRIVILEGED` → critical
  - `POLICY_CONTAINER_ALLOW_PRIVILEGE_ESCALATION` → warning
  - `POLICY_CONTAINER_RUN_AS_ROOT` → info
- **host-access** (hostNetwork / hostPID / hostIPC)
  - `POLICY_WORKLOAD_HOST_NETWORK` → warning
  - `POLICY_WORKLOAD_HOST_PID_OR_IPC` → warning
- **probes** (liveness / readiness / startup)
  - `POLICY_CONTAINER_NO_LIVENESS_PROBE` → warning
  - `POLICY_CONTAINER_NO_READINESS_PROBE` → warning
  - `POLICY_CONTAINER_NO_STARTUP_PROBE` → info (matters for slow-booting
    containers)

A workload is `compliant` only when every checked container passes every rule.

## Files Changed

### New Files

- `backend/internal/policy/model.go` — `ContainerPolicy` (pointer booleans
  distinguish "unset" from an explicit false for privileged /
  allowPrivilegeEscalation / runAsNonRoot), `WorkloadPolicy`, `Inputs`,
  `Status` (workloads_total, containers_total, compliant_workloads, by_severity,
  by_family, findings) + finding codes / severities / families.
- `backend/internal/policy/service.go` — pure `Evaluate(clusterID, Inputs, at)`;
  findings sorted by severity then code.
- `backend/internal/policy/service_test.go` — 203-line table suite covering each
  rule branch (pointer-bool defaults included).
- `frontend/src/views/OptimizationView.vue` — new "策略合规" tab (compliant /
  total cards + findings table grouped by family).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectPolicy` maps workload
  controllers and their container security contexts / probes (M65 read-only
  List).
- `backend/internal/optimization/service.go` — delegates `CollectPolicy`.
- `backend/internal/httpserver/optimization.go` — `policyAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/policy/analyze`
  (audit `optimization.policy.analyze`).
- `docs/api/openapi.yaml` — `analyzePolicyPosture` operation +
  `PolicyPostureStatus` schema.
- `frontend/src/types/optimization.ts` — `PolicyPostureStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzePolicy(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — client success / failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest, vite build green.

## Notes

- Pointer booleans in `ContainerPolicy` preserve the Kubernetes defaults (e.g.
  `privileged` defaults to false, so only an explicit `true` is a finding).
- Findings reuse the canonical `internal/finding` contract and render uniformly
  with the other optimization analyzers.
