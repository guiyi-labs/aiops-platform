# ADR 0045: Namespace Governance and Capacity Posture

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M29, M27-M28 (post-baseline governance track), M25 (read-only resource workbench)

## Context

Operators navigating the multi-cluster workbench can inspect individual ResourceQuota,
LimitRange, PodDisruptionBudget, workload and Pod resources through the existing
read-only routes (M25). However, the per-Namespace governance posture is scattered
across six separate kind pages and the global search. There is no deterministic,
side-by-side view that answers:

1. What ResourceQuota + LimitRange bounds govern this Namespace?
2. How many user-owned workloads and Pods are actually running there?
3. Is there a PodDisruptionBudget protecting any of them?
4. What cluster-wide node capacity is present as denominator context?

The roadmap (M29) calls for joining these reads into a single Namespace posture
view while staying read-only, citing every source explicitly, and refusing to
infer scheduler semantics (QoS share, preemption, topology-spread) or
NetworkPolicy reachability.

## Decision

### 1. Separate `internal/namespaceposture` package

Create a dedicated `internal/namespaceposture` package with its own `Service`
and bounded `KubernetesSource` interface. The posture service is strictly
read-only; it never writes, patches or deletes anything. Bounding the consumed
interface keeps the posture package auditable:

```go
type KubernetesSource interface {
  Namespaces(context.Context, int64, apiquery.ListQuery) (...)
  Nodes(context.Context, int64, apiquery.ListQuery) (...)
  ResourceQuotas(context.Context, int64, string, apiquery.ListQuery) (...)
  LimitRanges(context.Context, int64, string, apiquery.ListQuery) (...)
  PodDisruptionBudgets(context.Context, int64, string, apiquery.ListQuery) (...)
  Pods(context.Context, int64, string, apiquery.ListQuery) (...)
  Deployments/StatefulSets/DaemonSets/Jobs/CronJobs(...)
}
```

No access to Services, Ingresses, EndpointSlices, PVCs, Secrets, ConfigMaps or
NetworkPolicies. Those resources are intentionally out of scope for the
Namespace posture.

### 2. Deterministic evidence citations, not guesses

Every posture section carries an `EvidenceCitation`:

```go
type EvidenceCitation struct {
  APIPath     string       // exact group-version-namespaced-kind path
  Status      SourceStatus // complete | partial | truncated | unavailable
  Total       int          // total items upstream reported
  Returned    int          // items returned into posture
  Remaining   int          // items not returned (truncation signal)
  Error       string       // non-nil upstream error verbatim, if any
  CollectedAt string       // RFC3339 UTC
}
```

- `Status = unavailable` when the upstream call returns an error
- `Status = truncated` when `remaining > 0` despite no error
- `Status = partial` when a multi-kind aggregate (workloads) succeeds for some
  kinds but fails for others
- `Status = complete` otherwise

`NamespacePosture.PartialSections` additionally lists section names whose
evidence is NOT complete so frontends can surface warnings without inspecting
every citation.

### 3. Concurrent bounded fan-out per-Namespace in `Get`

For the single-Namespace full posture (`Service.Get`), fan out six concurrent
goroutines: ResourceQuotas, LimitRanges, Workloads (5-kind sub-fan-out), Pods,
PodDisruptionBudgets, and NodeCapacity. Each goroutine independently records
its own `EvidenceCitation`. A single Namespace metadata failure propagates as
an error (the posture is meaningless without the Namespace itself); every
other section's failure is contained to that section only.

Per-section call limit = 100 items. This keeps the posture a compact rollup,
not an unbounded inventory dump. The posture is for governance at a glance;
the existing kind-level pages remain authoritative for full inventory.

### 4. Compact list summary, not full posture per-Namespace, in `List`

For the Namespace list view (`Service.List`), return a compact
`PostureListEntry` per Namespace:

- `workload_count`, `pod_count`, `quota_count`, `limit_range_count`, `pdb_count`
- `partial_sections` — same semantics as the full posture
- NO per-kind breakdown, NO ResourceQuota hard/used map, NO per-Pod phase/node
  spread

Avoiding the full six-way fan-out per Namespace in `List` keeps response
bounded to ~`namespaces × 6 lightweight count queries` rather than
`namespaces × 6 full-list payloads`.

### 5. Reviewed workload and risk contract

Aggregate Deployment, StatefulSet, DaemonSet and CronJob as reviewed
controllers. Jobs remain visible in counts but their Pods are not assigned
inferred controller policy. The fixed finding codes are:
`MISSING_QUOTA`, `EXHAUSTED_QUOTA`, `QUOTA_PRESSURE`,
`MISSING_LIMIT_RANGE_DEFAULTS`, `MISSING_CONTAINER_REQUESTS`,
`MISSING_CONTAINER_LIMITS`, `BEST_EFFORT_WORKLOAD`, `NO_MATCHING_PDB`,
`BLOCKED_PDB_DISRUPTIONS`, `NODE_UNSCHEDULABLE`, `NODE_PRESSURE`,
`REQUESTED_CAPACITY_THRESHOLD`, and `INCOMPLETE_EVIDENCE`.

Quota quantities use Kubernetes Quantity parsing. PDB coverage uses the
official LabelSelector implementation. Namespace Pod requests are compared
with schedulable, non-pressure Node allocatable capacity at a frozen 80%
warning threshold; this is a pressure indicator, not a schedulability claim.

### 6. Explicit non-inferences

The posture still does not compute:

- LimitRange **conflict ordering** across multiple LimitRanges — Kubernetes
  applies them in object-creation order and that order is not observable
- exact scheduler placement, affinity, taints, preemption or storage binding
- NetworkPolicy **reachability** — requires full set-based selector
  evaluation and is not posture-level information
- CronJob → Job → Pod owner-chain expansion

### 7. Read-only HTTP surface, no audit actions

Two new routes under the existing cluster-scoped API:

- `GET /api/v1/clusters/:cluster_id/namespace-postures` — list (authenticated,
  any role, consistent with other Kubernetes read routes)
- `GET /api/v1/clusters/:cluster_id/namespace-postures/:namespace` — single
  Namespace full posture (same auth)

No POST/PATCH/DELETE routes. No audit actions because there are no mutations
to audit.

### 8. Frontend: two-pane NamespacePostureView under 分析与治理

The frontend view (`/namespace-posture`, navigated via ConsoleLayout → 分析与治理
→ 命名空间治理) follows the same two-pane pattern used by diagnosis and
governance views:

- **Left pane**: Namespace list with phase badge, quick-count chips
  (workloads/pods/quotas/LR/PDB), and a section-completeness badge
  (`完整` / `N 段不完整`)
- **Right pane**: Full posture for the selected Namespace, section by
  section, each section head carrying its evidence badge (完整/部分/已截断/
  不可用 + returned/total counts where applicable)

## Consequences

- No new DB migrations required (the posture is stateless, computed from
  live Kubernetes reads)
- `httpserver.Service` gains one new constructor-injected dependency
  (`*namespaceposture.Service`) and two new handlers
- `namespaceposture/service_test.go` covers source containment, deterministic
  aggregation, Quantity/risk derivation, selector coverage and incomplete
  evidence behavior
- Fast-verification gate: 28.56s with all backend packages (10 new tests),
  frontend typecheck, 73 frontend Vitest tests, compose/kustomize manifests
- Real-kind acceptance is defined in
  `scripts/e2e-m29-governance-posture-kind.ps1`
