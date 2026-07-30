# ADR 0040: Safe Deployment Release Lifecycle

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend, Frontend, Security

## Context

M19 ("Controlled Operations Catalog") deliberately deferred Deployment
rollback. The recorded reason was:

> The current gateway and public models do not provide exact ReplicaSet
> revision/template history, so the platform cannot bind a dry-run and
> confirmation to an immutable selected revision. Accepting a client
> template or an arbitrary historical patch would violate the
> fixed-action boundary.

M23 closes that gap. Operators need three things the platform did not
provide:

1. **Rollout evidence**: a read-only view of the Deployment's revision
   graph — current revision, ReplicaSet revisions, replica counts,
   images, and rollout phase — derived from the cluster, not from
   client-supplied state.
2. **Controlled image update**: a fixed action that updates a single
   container image on a Deployment through the same dry-run → typed
   diff → confirmation → idempotent execution → audit contract used by
   scale and CronJob suspend/resume.
3. **Controlled rollback**: a fixed action that rolls a Deployment back
   to an exact historical revision, **without** accepting a
   client-owned template. The platform must derive the rollback patch
   from the live ReplicaSet at execution time and fail closed if the
   ReplicaSet identity has changed since diagnosis.

The hard constraint is the fixed-action boundary from ADR 0024: the
platform never accepts arbitrary YAML, generic patches, or
client-owned mutation content. Every action is a typed, server-derived
operation.

## Decision

### 1. Rollout history is derived from ReplicaSet revision evidence

The Kubernetes service exposes two read-only methods:

- `RolloutHistory(ctx, clusterID, namespace, name)` reads the
  Deployment, parses `metadata.annotations["deployment.kubernetes.io/revision"]`
  to find the current revision, then lists ReplicaSets owned by the
  Deployment's UID. Each ReplicaSet's
  `metadata.annotations["deployment.kubernetes.io/revision"]` is the
  canonical revision anchor. The response is a `RolloutHistory` with
  `deployment`, `namespace`, `current_revision` and a `revisions`
  list. Each `RolloutRevision` carries `revision`, `replicaset_name`,
  `uid`, `resource_version`, `created_at`, `replicas`,
  `ready_replicas`, `available_replicas`, `images` and a `current`
  flag.
- `RolloutStatus(ctx, clusterID, namespace, name)` reads the
  Deployment and derives `current_revision`, `desired_replicas`,
  `updated_replicas`, `ready_replicas`, `available_replicas`,
  `unavailable_replicas` and a `phase` (`complete`, `progressing`,
  `degraded`). The phase is derived from the Deployment conditions and
  replica counts, not from a client-supplied label.

The ReplicaSet UID and resourceVersion are **not** part of the public
history response shape — they are internal evidence used to bind
rollback plans to an immutable target.

### 2. Two new fixed controlled operations

The action catalog grows from five to seven:

| Action | Parameters | Target preconditions |
|--------|------------|---------------------|
| `deployment.image_update` | `container_name`, `desired_image` | Deployment UID + resourceVersion |
| `deployment.rollback` | `rollback_revision` | Deployment UID + resourceVersion **and** ReplicaSet UID + resourceVersion |

Both actions reuse the existing `PreviewOperation` flow:
server-side dry-run, typed `OperationChange` (`before`/`after`), one-time
confirmation token hash, idempotency key, target snapshot, expiry and
bounded outcome.

Migration `000018` extends `remediation_plans` with `container_name`,
`before_image`, `desired_image`, `rollback_revision`,
`rollback_replicaset_name`, `rollback_replicaset_uid` and
`rollback_replicaset_resource_version`. The action CHECK constraint and
the parameter CHECK constraint are rewritten so each action is bound to
exactly its valid parameter shape. `deployment.image_update` requires a
non-empty container name and before/desired images;
`deployment.rollback` requires a positive `rollback_revision` and a
non-empty `rollback_replicaset_uid`.

### 3. The rollback patch is derived at execution time, never at diagnosis time

This is the central safety property of M23.

`buildRollbackPatch(ctx, plan)` fetches the ReplicaSet named in
`plan.RollbackReplicaSetName` from the live cluster, projects its
`spec.template` into a strategic merge patch, and applies it to the
Deployment. The plan stores only the ReplicaSet **name**, **UID** and
**resourceVersion** as preconditions; it never stores the template
content.

At execution time, the service:

1. Fetches the ReplicaSet by name.
2. Compares the fetched ReplicaSet's UID to
   `plan.RollbackReplicaSetUID`. If they differ, returns
   `ErrTargetChanged` → `Kubernetes remediation target changed after
   diagnosis`. The patch is never applied.
3. Otherwise, projects `replicaset.spec.template` into the rollback
   patch and applies it with the Deployment UID/resourceVersion
   preconditions used by every other action.

This means a rollback plan created against revision 2 will fail closed
if revision 2's ReplicaSet has been pruned by
`revisionHistoryLimit`, has been recreated by another controller, or
has otherwise changed identity. The platform never applies a stale
template.

### 4. The rollback patch deliberately does not annotate the source UID

An earlier draft recorded a
`k8s-aiops.local/rollback-replicaset-uid` annotation on the Deployment
so later audits could trace the rollback source. This was removed
because:

- The annotation would survive into the new ReplicaSet created by the
  rollback, leaking stale identity into subsequent revisions.
- A later rollback plan would then read the stale UID as the "current"
  target, breaking the UID-precondition check.
- The plan already records the source ReplicaSet name, UID and
  resourceVersion in PostgreSQL; the audit trail is complete without
  annotating the live object.

### 5. Frontend reuses the existing controlled-operation confirmation flow

`ResourceDetailView` adds a Rollout tab that loads history and status
in parallel via `Promise.allSettled`. `WorkloadsView` extends the
Deployment drawer with a release control section that mirrors the
existing scale control: a container selector plus desired image input
for image update, a revision selector for rollback, and the shared
operation preview/confirm dialog that renders the typed diff. No new
confirmation primitive is introduced.

### 6. Real-kind E2E proves the revision graph advances deterministically

The disposable kind script
`scripts/e2e-m23-release-lifecycle-kind.ps1` asserts the revision graph
advances 1 → 2 → 3 across an image update then a rollback, that
idempotent replay returns the same plan, that the restored Deployment
image matches the baseline, and that the observer RBAC denies
`delete pods` and `patch deployments` in `kube-system`.

## Consequences

- The action catalog is now seven fixed actions. Adding a new action
  still requires a migration, a typed request shape, a server-derived
  patch, and the full dry-run/confirmation/audit lifecycle.
- Rollback to a pruned revision fails closed. Operators must either
  choose a surviving revision or perform a manual image update to
  restore the desired state. This is intentional: applying a stale
  template is worse than refusing.
- The ReplicaSet UID precondition makes rollback plans non-replayable
  across ReplicaSet recreation. A retried rollback after a concurrent
  rollout will return `ErrTargetChanged`, which is the correct safety
  behavior.
- The rollout history endpoint reads ReplicaSets, so the observer
  Role must grant `list`/`watch` on `replicasets.apps`. The
  least-privilege RBAC in `deploy/managed-cluster/observer.yaml`
  already covers this; the E2E script asserts it.
- No new write primitive is introduced. The image-update and rollback
  patches are typed JSON patches generated by the server, not by the
  client.

## Boundary

M23 does not include:

- Generic YAML/CRD editing or arbitrary patch endpoints.
- Rollback to a revision whose ReplicaSet has been pruned.
- Cross-cluster promotion (M24) or workload backup/restore (M25).
- Background rollout status streaming; the Rollout tab refreshes on
  navigation and on demand.
- Pod-level image update or StatefulSet/DaemonSet rollback.
- A `kubectl rollout undo`-style "undo last" shortcut. The operator
  must select an explicit revision; the platform refuses to guess.
