# ADR 0046: Controlled Node Maintenance

- Date: 2026-07-30
- Status: Accepted
- Related milestones: M30, M29 (PDB/capacity interpretation), M23 (controlled operations pattern), M19 (controlled operations catalog)

## Context

Kubernetes node maintenance — cordon, uncordon and drain — is a high-frequency
operator workflow that the platform did not expose. Without a controlled path,
operators either skip the platform entirely (`kubectl`) or, worse, run ad-hoc
Pod deletion that bypasses PodDisruptionBudget admission. The roadmap (M30)
calls for a bounded, PDB-aware maintenance operation that reuses the
controlled-operation shape accepted in M23/ADR 0023 and the PDB interpretation
accepted in M29/ADR 0045.

The risk surface is significant: drain is destructive, evictions can time out,
emptyDir deletion loses data, force-deletion bypasses PDB, and an uncordon
after a failed drain silently returns a damaged node to scheduling. The design
must fail closed on every one of these.

## Decision

### 1. Separate `internal/maintenance` package

Create a dedicated `maintenance_plans` table (migration 000022) and
`internal/maintenance` package rather than reusing `remediation_plans` or
`backup_plans`. The maintenance domain has different parameters (Node identity,
Pod classification, PDB evidence, eviction outcomes) that do not fit the
existing plan CHECK constraints. The controlled-operations *pattern* (Preview →
Execute, confirmation token, idempotency, Claim/Complete/Fail) is reused but
the persistence surface is separate.

### 2. Fixed V1 scope — three actions only

The maintenance service supports exactly three actions on a single worker Node:

- **cordon** — patch `spec.unschedulable=true`. Rejected if already cordoned.
- **uncordon** — patch `spec.unschedulable=false`. Rejected if already
  schedulable. Never auto-issued after a failed drain.
- **drain** — bounded PDB-aware eviction. Rejected unless the Node is already
  cordoned (preview-time precondition).

Control-plane Nodes are always rejected (`node-role.kubernetes.io/control-plane`
or `node-role.kubernetes.io/master` labels). Bulk selection, multi-node drain
and arbitrary YAML patches do not exist.

### 3. Pod classification with explicit blocker classes

Every resident Pod is classified into one of three categories:

- **retained** — DaemonSet-managed or mirror/static Pod. Never evicted.
- **evictable** — managed by a reviewed controller, has no real
  `spec.volumes[].emptyDir`, and every selector-matching PDB reports
  `disruptionsAllowed > 0`.
- **blocking** — unmanaged/unknown owner, uses `emptyDir`, has no matching PDB,
  has a matching zero-disruption PDB, or PDB evidence is unavailable.

PDB selectors are decoded as Kubernetes `metav1.LabelSelector` values and
matched with the official selector implementation. Namespace co-location by
itself never associates a PDB with a Pod.

### 4. Drain bounds

- At most 100 resident Pods inspected per Node (`maxResidentPods`)
- At most 20 evictable Pods per drain (`maxEvictablePods`)
- Eviction concurrency = 2 (`evictionConcurrency`)
- Per-eviction timeout = 30 seconds (`evictionTimeout`)
- Total drain deadline = 10 minutes (plan TTL)

Exceeding any bound fails the preview closed. There is no "best-effort" drain
that exceeds these bounds.

### 5. Two-phase confirmation with idempotency

Identical to M19/ADR 0023 and M28/ADR 0044 pattern:

- Preview returns a one-time confirmation token (SHA-256 hash stored, plaintext
  returned once and never persisted)
- Preview performs a server-side dry-run Node patch for cordon/uncordon to
  surface admission errors before confirmation
- Preview captures Node UID + resourceVersion and every resident Pod's UID +
  resourceVersion as immutable evidence
- Execute requires the token + `Idempotency-Key` header (8–128 chars)
- `Claim` transaction uses `SELECT FOR UPDATE` + `constant-time` token
  comparison
- Stale lock recovery (claim TTL = 1 min), plan TTL = 10 min
- `Complete`/`Fail` condition on `status=executing AND idempotency_key=?`
- Same-key replay returns the same plan without re-mutating

### 6. Drain execution: bounded eviction, no force paths

Execution first compares both Node UID and resourceVersion, and Node patches
carry the expected resourceVersion. `executeDrain` re-collects Pod evidence
and verifies it has not widened since preview (`evidenceMatches`). If the Pod set changed, the plan fails with
`ErrStaleTarget` and the Node is NOT mutated. The Node is then ensured cordoned
(preview required it, but the patch is re-applied defensively if the Node was
uncordoned out-of-band). A failed cordon is persisted as a failed plan and no
eviction is attempted.

Eviction uses `policy/v1` Eviction subresource `create` only:

- `POST /api/v1/namespaces/{ns}/pods/{name}/eviction`
- No `delete` verb on Pods, no `--disable-eviction`, no grace-period override
- PDB admission is enforced by the kube-apiserver; the platform surfaces the
  rejection verbatim in the Pod outcome
- Per-Pod timeout uses `context.WithTimeout` — no `--timeout` flag, no client
  library deadline bypass

On partial failure (one or more evictions fail), the plan is marked `failed`
with `ErrPartialDrain`, the result records `partial=true`, and the Node
**remains cordoned**. The platform never auto-uncordons after a partial drain.

### 7. Hard prohibitions (encoded in interface bounds)

The `KubernetesSource` interface consumed by the maintenance service exposes
exactly five methods:

```go
type KubernetesSource interface {
  Node(ctx, clusterID, name) (Node, error)
  Pods(ctx, clusterID, namespace, query) (ListResponse[Pod], error)
  PodDisruptionBudgets(ctx, clusterID, namespace, query) (ListResponse[PDB], error)
  PatchNode(ctx, clusterID, name, body, dryRun) (Node, error)
  CreateResource(ctx, clusterID, path, body, dryRun) ([]byte, error)
}
```

No `DeletePod`, no `UpdatePod`, no `DeleteNode`, no Secret/ConfigMap access, no
arbitrary `Update`. The eviction path is the *only* Pod-mutating surface, and
it is hard-bound to the `/pods/{name}/eviction` subresource via `CreateResource`
with a fixed body shape constructed by `buildEvictionBody`.

### 8. Authorization

- Preview/Execute: `requireRoles(SystemAdmin, OperationsAdmin)`
- List plans: any authenticated user
- Read Node/Pod/PDB: any authenticated user (unchanged from M22/M25)

### 9. Audit

Two new audit actions (registered in `audit.go`):

- `maintenance.preview` → `MaintenancePlan` resource
- `maintenance.execute` → `MaintenancePlan` resource

Audit details record `method`, `path_template`, and `cluster_id`. The
confirmation token, idempotency key and kubeconfig are never recorded.

### 10. Frontend: two-pane NodeMaintenanceView under 分析与治理

The frontend view (`/node-maintenance`, navigated via ConsoleLayout → 分析与治理
→ 节点维护) follows the same two-pane pattern used by the backup and
namespace-posture views:

- **Left pane**: action selector (cordon/uncordon/drain), Node name input,
  preview button, and a plan history list (last 30 plans)
- **Right pane**: preview confirmation (with Pod classification table,
  PDB evidence, one-time token display) or execution result (with per-Pod
  outcome table, partial-failure banner)

Destructive emphasis (red `danger-button`) applies only to drain confirmation.
Cordon and uncordon use the standard primary button but still require explicit
confirmation. The route is role-gated to `system_admin` and `operations_admin`.

## Consequences

- New migration 000022 (`maintenance_plans` table) required
- `httpserver.Service` gains one new constructor-injected dependency
  (`*maintenance.Service`) and three new handlers
- Frontend gains a new `NodeMaintenanceView` and navigation entry under
  分析与治理
- The `KubernetesSource` interface bounds the mutation surface to Node patch
  and Eviction create; no Pod delete or Node delete path exists in the
  maintenance package
- Real-kind E2E is implemented by
  `scripts/e2e-m30-node-maintenance-kind.ps1` using a disposable two-worker
  kind cluster and PDB/emptyDir fixtures; it proves cordon replay, safe
  eviction, blocked unsafe drains, uncordon and complete cleanup
- Real Pod volumes and PDB selectors are part of the bounded read projection;
  there is no annotation-based storage safety assumption
