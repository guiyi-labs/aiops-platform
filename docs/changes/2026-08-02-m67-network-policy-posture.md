# M67: Network Connectivity & NetworkPolicy Posture Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `a8f4039`; CI backend 7-gate + frontend green)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest / vite build) green

## Summary

Adds a read-only network connectivity and NetworkPolicy posture analyzer to the
optimization center. It answers the two questions a human operator actually
asks: "can this traffic get through?" (Services selecting no backends, ports
whose targetPort resolves to nothing, ports the namespace's own policies would
drop) and "is anything wide open?" (missing default-deny baselines, dead
policies, allow-all rules, world-open ipBlocks, externally exposed Services
with no ingress restriction).

The analyzer is pure and offline (ADR 0004): it reasons statically over an
observation bundle that the caller supplies (or that the M65 collector gathers
via read-only List calls). It never contacts a cluster, never sends a probe
packet, and never mutates anything.

Rules (`internal/netpolicy` pure `Evaluate`), grouped by family:

- **coverage** — is a workload protected at all
  - `NETPOL_NS_NO_DEFAULT_DENY_INGRESS` → warning (demoted to **info** for
    kube-system / kube-public / kube-node-lease): namespace has no default-deny
    ingress baseline.
  - `NETPOL_POD_INGRESS_UNRESTRICTED` → warning: namespace already has policies
    but this Pod is selected by none.
- **policy-hygiene** — defects in the policies themselves
  - `NETPOL_POLICY_SELECTS_NO_PODS` → warning: podSelector matches no Pod, the
    policy is dead.
  - `NETPOL_INGRESS_ALLOW_ALL` → warning: ingress rule with empty `from`
    equals allow from anywhere, cancelling a default-deny baseline.
  - `NETPOL_INGRESS_FROM_ALL_NAMESPACES` → warning: empty `namespaceSelector`
    plus empty `podSelector` allows every namespace.
  - `NETPOL_WIDE_IPBLOCK` → warning (ingress `0.0.0.0/0`) / info (egress to the
    world): ipBlock opens the cluster to / from the internet.
- **reachability** — would traffic actually arrive
  - `NETPOL_SERVICE_NO_BACKENDS` → critical: Service selector matches no Pod,
    requests fail.
  - `NETPOL_SERVICE_PORT_UNMATCHED` → critical (named targetPort unresolvable,
    no Endpoint generated) / info (numeric targetPort not declared on any
    container).
  - `NETPOL_SERVICE_PORT_BLOCKED` → warning: backends covered by an ingress
    policy but no rule allows the Service port, traffic is dropped.
  - `NETPOL_HOSTNETWORK_POLICY_INEFFECTIVE` → info: hostNetwork Pod selected by
    a policy but most CNIs ignore host-network traffic.
- **exposure** — externally reachable entry points
  - `NETPOL_EXPOSED_SERVICE_UNRESTRICTED` → critical: NodePort / LoadBalancer
    whose backends have no ingress policy at all, the largest attack surface.

## Files Changed

### New Files

- `backend/internal/netpolicy/model.go` — `Selector` / `PodInfo` / `Policy` /
  `ServiceInfo` domain types (distinguishing "absent" from "present-but-empty"
  selector, which is load-bearing for NetworkPolicy semantics), `Inputs`,
  `Status` (namespaces_total, pods_total, policies_total, services_total,
  ingress_covered_pods, egress_covered_pods, isolated_namespaces,
  exposed_services, by_severity, by_family, findings) + finding codes /
  severities / families.
- `backend/internal/netpolicy/service.go` — pure
  `Evaluate(clusterID, Inputs, at)` implementing the four families above;
  findings sorted by severity then code.
- `backend/internal/netpolicy/service_test.go` — 622-line table suite covering
  every rule branch (critical / warning / info and the system-namespace
  demotion path).
- `frontend/src/views/OptimizationView.vue` — new "网络连通" tab (coverage
  cards + findings table).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectNetworkPolicy` maps
  Namespaces / Pods / Policies / Services (M65 read-only List).
- `backend/internal/optimization/service.go` — delegates `CollectNetworkPolicy`.
- `backend/internal/httpserver/optimization.go` — `networkPolicyAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/networkpolicy/analyze`
  (audit `optimization.networkpolicy.analyze`).
- `docs/api/openapi.yaml` — `analyzeNetworkPolicyPosture` operation +
  `NetworkPolicyStatus` schema.
- `frontend/src/types/optimization.ts` — `NetworkPolicyStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeNetworkPolicy(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — client success / failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest, vite build green.

## Notes

- The `Selector` distinguishes absent (nil) from present-but-empty (selects all
  pods); `matchExpressions` are not evaluated and are treated conservatively so
  coverage checks err toward silence instead of a false "unprotected" alarm.
- Findings reuse the canonical `internal/finding` contract and render uniformly
  with the namespace-posture, CIS, deprecated-API and FinOps findings.
