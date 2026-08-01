# ADR 0078 — M62 CIS Kubernetes Compliance Posture (read-only)

- Date: 2026-08-01
- Status: Proposed
- Related milestones: M62 (Optimization branch `opt/m61-m63`)
- Supersedes / context: extends the read-only posture philosophy established in
  ADR 0076 (deprecated API) and ADR 0077 (FinOps). Companion to
  `internal/finding`.

## Context

The platform already observes cluster state broadly (metrics M21, namespace
posture, multi-cluster fleet, correlation, topology) but does **not** express
that observation against an external security benchmark. CIS Kubernetes
Benchmark is the de-facto industry baseline (kube-bench / Kubescape implement
it). Adding a read-only CIS posture check lets operators answer "how hardened is
this cluster, and what specifically should I fix?" without leaving the platform.

Two constraints shaped this ADR:

1. **Read-only, observation-only** (per ADR 0004 and the platform's stated
   Non-Goals). The analyzer MUST NOT remediate, mutate objects, install
   admission webhooks, or change any configuration. It only emits findings.
2. **No collision with the in-flight M46–M60 route.** The analyzer is delivered
   as pure functions over already-fetched data, in its own package
   (`internal/cis`), depending only on `internal/finding`. It deliberately does
   NOT add HTTP routes, frontend, or wiring to the live client — those are
   deferred to a later integration step coordinated with the main route.

## Decision

Introduce `backend/internal/cis` as a deterministic, compiled-in CIS control
catalog evaluator, modelled after the `diagnosis` rule pattern and the
`deprecatedapi` (pluto/kubent-style) catalog pattern:

- **`catalog.go`** — compiled-in controls across four domains:
  - *Component flag controls* (CIS 1.2/1.3/1.4/1.5/4.2): kube-apiserver,
    kube-scheduler, kube-controller-manager, etcd, kubelet. Six flag-check
    shapes: `should_be_false`, `must_be_set`, `must_be_absent`,
    `mode_must_include`, `must_not_equal`, `equals`. 26 controls.
  - *Workload security checks* (CIS 5.2 / Pod Security Standards): privileged,
    allowPrivilegeEscalation, run-as-non-root, host namespace sharing,
    hostPath, CAP_NET_RAW drop. 6 checks.
  - *RBAC checks* (CIS 5.1): cluster-admin to non-system subject, wildcard
    verb/resource role. 2 checks.
  - *Namespace Pod Security Admission checks* (CIS 5.2 / PSS): enforce level not
    privileged/unset. 1 check.
- **`model.go`** — read-only input contracts (`ComponentConfig`,
  `WorkloadSecurity`, `ContainerSecurity`, `RBACBinding`, `NamespacePodSecurity`,
  `Inputs`, `Status`). `Finding` is aliased from `internal/finding` so the
  frontend renders CIS findings uniformly with every other posture module.
- **`service.go`** — pure `Evaluate(clusterID, Inputs, observedAt) Status`.
  Only the supplied domains are checked; missing domains are skipped (not
  counted as pass/fail), so partial observations are safe. Each failed control
  emits exactly one `finding.Finding` with `details` carrying flag/value, CIS
  level, rationale, and remediation reference.

The evaluator is deterministic (no time/random in logic; `observedAt` only
stamps the finding) and has zero external dependencies.

## Data model (input contracts)

| Contract | Source in a real cluster |
| --- | --- |
| `ComponentConfig.Flags` | kube-apiserver/scheduler/controller-manager/etcd flag map (from static manifests or node kubelet config) |
| `WorkloadSecurity` | Pod / Deployment / StatefulSet / DaemonSet `.spec` + `.spec.containers[].securityContext` |
| `RBACBinding` (+ `RoleRules`) | RoleBinding/ClusterRoleBinding + resolved referenced Role rules |
| `NamespacePodSecurity` | `pod-security.kubernetes.io/enforce|audit|warn` namespace labels |

The service layer that builds these from the live Kubernetes API is **out of
scope** for this ADR (deferred to integration; see Non-goals).

## Acceptance criteria

- `go build ./internal/cis/` and `go vet ./internal/cis/` pass.
- `go test ./internal/cis/` passes, covering: critical component flag failure
  (anonymous-auth), pass-when-unset, RBAC cluster-admin (non-system flagged,
  system SA not flagged), RBAC wildcard, privileged container (flagged vs. not),
  run-as-root uid 0, namespace PSA (privileged/unset flagged, restricted pass),
  empty input, and every flag-check shape.
- Every emitted finding carries `code`, `severity`, `summary`, `resource`, and
  `details{rationale, remediation}` so it is actionable without further lookup.
- No new dependency; only `internal/finding` is imported.

## Non-goals (explicitly excluded)

- No mutation, remediation, or "fix" action of any kind.
- No admission webhook, OPA/Replica/Policy-as-Code engine, or dynamic rule
  evaluation (that is M68's scope and stays read-only-编目 only).
- No live cluster contact, no client wiring, no HTTP route, no frontend
  (deferred; avoids collision with M46–M60).
- No fix for the baseline `internal/cluster` compile defect (owned by the
  M33/M34+ route); `cis` does not import `cluster`.
- Not a full CIS coverage (≈250 controls). This is a high-value, safe,
  read-only subset; remaining controls are additive later.

## Consequences

- Operators get a benchmark-aligned hardening report inside the platform.
- The analyzer is trivially extensible: add a struct literal to a catalog slice.
- Integration (live data → `Inputs`, API route, frontend panel) is a separate,
  well-bounded follow-up that must be coordinated with the main route owner.
