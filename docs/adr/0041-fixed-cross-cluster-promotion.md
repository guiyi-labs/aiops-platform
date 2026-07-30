# ADR 0041: Fixed Cross-Cluster Promotion

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend, Frontend, Security

## Context

The 2026-07-28 KRM/Ratel gap review
(`docs/references/krm-ratel-gap-analysis.md`) found that this platform was
narrower than the reference product in **cross-cluster promotion**: moving a
reviewed Deployment/Service/Ingress bundle from one cluster to another with
explicit source/target, namespace mapping, dependency inventory and destination
capability preflight.

M19 ("Controlled Operations Catalog") and M23 ("Safe Deployment Release
Lifecycle") established the fixed-action boundary: the platform never accepts
arbitrary YAML, generic patches, or client-owned mutation content. Every
mutation is a typed, server-derived operation bound to dry-run, typed diff,
one-time confirmation, idempotency, target preconditions and audit.

M24 applies that same boundary to cross-cluster promotion. Operators need
three things the platform did not provide:

1. **Bundle assembly**: a fixed bundle of Deployment/Service/Ingress selected
   from a source cluster, with runtime/server-owned fields stripped and the
   destination namespace rewritten.
2. **Dependency inventory**: a scanned list of ConfigMap/Secret references in
   the bundle, resolved against operator-provided destination mappings. The
   platform never copies sensitive values; it only verifies the destination
   object exists by the mapped name.
3. **Controlled execution**: a server-side dry-run on the destination cluster,
   one-time confirmation, idempotent execution with per-item status tracking,
   and partial-failure reporting — never a bulk project migration.

The hard constraint is the same as M19/M23: the platform never accepts a
client-owned manifest for execution. The promoted manifest is **fetched from
the source cluster** at preview time, stripped of server-owned fields, and
dry-run against the destination before the operator confirms.

## Decision

### 1. The promotion bundle is fixed to Deployment, Service and Ingress

`promotion.validPromotionKind` accepts only `Deployment`, `Service` and
`Ingress`. Other kinds — including ConfigMap, Secret, StatefulSet, DaemonSet,
Job, CronJob, PVC, IngressClass and any CRD — are rejected at validation.
ConfigMap and Secret appear only as **dependencies**, not as promotable items.

Migration `000019` enforces this in PostgreSQL: the
`promotion_bundle_items_kind_check` CHECK constraint binds `kind` to those
three values, and `promotion_dependency_mappings_kind_check` binds dependency
`kind` to `ConfigMap` and `Secret`.

The bundle is bounded to 10 items per plan
(`promotion_bundle_items_ordinal_check` enforces `ordinal BETWEEN 0 AND 9`).
Bulk project migration is explicitly out of scope.

### 2. Source and destination clusters must be distinct

`validateRequest` rejects `SourceClusterID == DestinationClusterID` with
`ErrInvalidRequest`. Migration `000019` mirrors this in PostgreSQL with
`promotion_plans_cluster_distinct_check`. A plan that targets the same cluster
cannot be persisted.

### 3. The promoted manifest is fetched from the source, never client-supplied

`Preview` calls `kubernetes.RawManifest(ctx, SourceClusterID, kind, namespace,
name)` for each requested bundle item. The raw manifest returned by the
Kubernetes API server is the only input to bundle assembly. The HTTP request
body carries only the **selector** (`kind`, `namespace`, `name`), never the
manifest content.

`RawManifest` is added to the `KubernetesSource` interface so the promotion
service can read source evidence without depending on the gateway HTTP
surface. The gateway path is the same read-only path used by M22 manifest
inspection; no new write primitive is introduced at the source.

### 4. Runtime/server-owned fields are stripped before dry-run

`stripRuntimeFields` decodes the raw manifest, captures the source `uid` and
`resourceVersion` for the plan record, and drops:

- Top-level `status` (server-owned).
- `metadata.uid`, `metadata.resourceVersion`, `metadata.creationTimestamp`,
  `metadata.generation`, `metadata.managedFields`, `metadata.selfLink`,
  `metadata.ownerReferences`, `metadata.finalizers`,
  `metadata.deletionTimestamp`, `metadata.deletionGracePeriodSeconds`.

User-defined `metadata.labels` and `metadata.annotations` are preserved. The
stripped manifest is the only manifest persisted on the plan and the only
manifest sent to the destination cluster.

`rewriteNamespace` then rewrites `metadata.namespace` to the destination
namespace. It does **not** rewrite selectors, ingress backends, or
cross-namespace references; the operator is responsible for namespace-scoped
consistency. This is intentional: silently rewriting selectors would mask
operator errors.

### 5. Dependency inventory is scanned, not copied

`scanDependencies` walks the stripped Deployment manifest's pod template
(`spec.template.spec`) and collects every ConfigMap and Secret reference it
can find:

- `containers[].envFrom[].configMapRef` and `.secretRef`
- `containers[].env[].valueFrom.configMapKeyRef` and `.secretKeyRef`
- `initContainers[]` (same paths as `containers`)
- `volumes[].configMap`
- `volumes[].secret.secretName`
- `volumes[].projected.sources[].configMap` and `.secret`
- `imagePullSecrets[].name` (treated as a Secret reference)

Service and Ingress manifests carry no pod template and return no references.
References without an explicit namespace are treated as source-namespace
scoped. The scanned references are deduplicated by
`kind/namespace/name`.

The platform **never copies** ConfigMap or Secret values. The operator
provides a `DependencyMapping` for each scanned reference, naming the
destination object that already exists on the destination cluster.
`verifyDependencyMappings` then:

1. For each operator-provided mapping, looks up the destination object by
   `ConfigMapExists` or `SecretExists`. Missing destination object →
   `ErrDependencyUnresolved`.
2. For each scanned reference, requires an operator-provided mapping. No
   mapping for a scanned reference → `ErrDependencyUnresolved`.

Both directions fail closed. An unmapped scanned reference is rejected even if
the operator provided extra mappings; an unresolved mapping is rejected even
if every scanned reference is covered.

### 6. The destination cluster is dry-run before confirmation

After stripping, namespace rewriting and dependency verification, `Preview`
calls `kubernetes.CreateResource(ctx, DestinationClusterID, path, manifest,
dryRun=true)` for each bundle item. The dry-run uses the Kubernetes API
server's `?dryRun=All` semantics: the request is validated, admitted and
rejected on conflict, but no object is persisted.

A dry-run failure returns `ErrPreviewFailed`. A dry-run conflict (destination
resource already exists) returns `ErrConflict`. The plan is not persisted
until every bundle item dry-runs successfully.

`preflight` runs before the dry-run: it verifies the destination namespace
exists (`NamespaceExists`) and that no destination resource already exists
(`ResourceExists`). The dry-run is the second line of defense; preflight is
the first.

### 7. The confirmation token is one-time, hashed and bound to idempotency

`newIdentity` generates a UUIDv4 plan ID and a 32-byte random confirmation
token. The token is base64url-encoded for the API response; only the SHA-256
hash is persisted (`confirmation_token_hash`). The plan TTL is 15 minutes;
the claim TTL (the window during which an idempotency key can resume an
in-progress execution) is 2 minutes.

`Execute` requires:

- A valid plan ID (36 characters).
- A non-empty confirmation token.
- An `Idempotency-Key` header between 8 and 128 characters.

`repository.Claim` is the single atomic gateway that decides whether to
execute or replay:

- If the plan is `awaiting_confirmation` and the token hash matches, the plan
  is transitioned to `executing`, the idempotency key is recorded, and
  execution proceeds.
- If the plan is `executing` and the idempotency key matches, the persisted
  plan is returned without re-applying. This is idempotent replay.
- If the plan is `executing` and the idempotency key differs, the request is
  rejected with `ErrInProgress`.
- If the plan is `succeeded`/`failed`/`partial` and the idempotency key
  matches, the persisted plan is returned. This is idempotent replay of a
  completed plan.
- If the plan is `succeeded`/`failed`/`partial` and the idempotency key
  differs, the request is rejected with `ErrAlreadyExecuted`.
- If the plan is expired, the request is rejected with `ErrExpired`.

This mirrors the `Claim` pattern from remediation (ADR 0023) and M23. No new
concurrency primitive is introduced.

### 8. Execution applies bundle items in ordinal order with per-item status

`Execute` iterates `plan.Items` in `Ordinal` order. For each item:

- If `ItemStatus == ItemStatusApplied`, the item is skipped (already applied
  in a prior partial run).
- Otherwise, `CreateResource(ctx, DestinationClusterID, path, manifest,
  dryRun=false)` is called.
- On success, the item is marked `applied`.
- On `ErrResourceConflict`, the item is marked `skipped` with
  `destination resource already exists`. The plan continues.
- On any other error, the item is marked `failed` with a sanitized error
  message. The plan continues; remaining items are still attempted.

After all items are attempted:

- If any item failed, the plan is transitioned to `failed` with
  `one or more promotion items failed`. The persisted plan records per-item
  status and error.
- Otherwise, the plan is transitioned to `succeeded` (or `partial` if any
  item was skipped due to conflict).

`safeExecutionError` strips Kubernetes API error details to a bounded HTTP
status or truncated message, preventing credential or internal-state leakage
in the API response and audit trail.

### 9. The HTTP surface is fixed and role-bound

Four routes are mounted under `v1`:

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| `GET` | `/api/v1/promotions` | any authenticated | list plans by `source_cluster_id` and optional `namespace` |
| `POST` | `/api/v1/promotions/preview` | `system_admin`, `operations_admin` | dry-run and persist a plan |
| `GET` | `/api/v1/promotions/:promotion_id` | any authenticated | read a plan by ID |
| `POST` | `/api/v1/promotions/:promotion_id/execute` | `system_admin`, `operations_admin` | confirm and execute |

`preview` uses `decodeStrictJSON` so unknown fields are rejected. `execute`
requires the `Idempotency-Key` header and a `confirmation_token` body. The
handler maps typed service errors to stable HTTP status codes and error
codes (`PROMOTION_NOT_FOUND`, `PROMOTION_BUNDLE_EMPTY`,
`PROMOTION_SOURCE_UNAVAILABLE`, `PROMOTION_NAMESPACE_MISSING`,
`PROMOTION_DEPENDENCY_UNRESOLVED`, `PROMOTION_CONFLICT`,
`PROMOTION_PREVIEW_FAILED`, `PROMOTION_CONFIRMATION_INVALID`,
`INVALID_IDEMPOTENCY_KEY`, `PROMOTION_EXPIRED`, `PROMOTION_IN_PROGRESS`,
`PROMOTION_ALREADY_USED`, `PROMOTION_FAILED`).

Audit target and destination cluster ID are set on every preview and execute
call.

### 10. The frontend wizard reuses the existing controlled-operation flow

`PromotionView` is a four-step wizard:

1. **Clusters & namespaces**: select source cluster, destination cluster,
   source namespace and destination namespace. Validates that clusters are
   distinct and namespaces are non-empty.
2. **Bundle assembly**: add Deployment/Service/Ingress items by name; add
   ConfigMap/Secret dependency mappings. Validates that at least one bundle
   item is present.
3. **Preflight confirmation**: call `previewPromotion`, render the typed
   bundle summary (item count, kind counts, dependency records) and require
   the operator to confirm before executing.
4. **Execution result**: call `executePromotion` with the confirmation token
   and a generated idempotency key, render per-item status
   (`applied`/`skipped`/`failed`) and plan-level status
   (`succeeded`/`partial`/`failed`).

The wizard reuses the existing `authorizedRequest` API client, the existing
`ConsoleLayout` shell, and the existing `primary-button`/`secondary-button`
styles. No new confirmation primitive is introduced.

### 11. Real-kind E2E proves the promotion contract against two clusters

The disposable kind E2E script
`scripts/e2e-m24-cross-cluster-promotion-kind.ps1` and fixtures under
`deploy/m24-cross-cluster-promotion-e2e` exercise the full promotion flow
against two uniquely named kind clusters. The run:

- Creates two disposable kind clusters with unique names, applies the
  `aiops-m24-source` fixture (ConfigMap + Deployment + Service) on the
  source and the `aiops-m24-dest` fixture (ConfigMap only) on the
  destination, installs the least-privilege observer RBAC, and registers
  both clusters with 1h ServiceAccount tokens.
- Previews a bundle of `Deployment/promote-target` and
  `Service/promote-target` with a `ConfigMap/app-config` dependency mapping;
  asserts the plan is `awaiting_confirmation`, the bundle summary carries
  the expected kind counts, and a confirmation token is returned.
- Executes the plan with a unique `Idempotency-Key`; asserts the plan
  transitions to `succeeded`, both items are `applied`, and the promoted
  Deployment and Service exist on the destination cluster by name.
- Asserts idempotent replay with the same `Idempotency-Key` returns the
  same plan without re-applying.
- Verifies the observer RBAC: `get deployments` is allowed on both clusters,
  `create deployments` is denied in `kube-system`.
- In `finally`, deletes both platform cluster registrations, deletes both
  disposable kind clusters, restores the previous kubectl context and
  refuses unsafe cleanup targets. Sanitized evidence is written to
  `.artifacts/m24-cross-cluster-promotion-kind`.

## Baseline Alignment Note (2026-07-30)

The final baseline review tightened the implementation without changing this
decision's public contract. Dependency mappings are summarized once per bundle
dependency instead of once per referencing item; Service cluster-assigned
fields are stripped; mapped ConfigMap/Secret names are rewritten in the
persisted execution manifest; and per-item execution results are persisted
while internal manifests remain absent from HTTP responses. The destination
write Role/RoleBinding is Namespace-scoped.

Fresh dual-kind evidence at
`.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-074812.json`
records `bundle_items=2`, `dependencies=1`, the rewritten
`app-config-promoted` reference, a succeeded plan and complete cleanup.

## Consequences

- The promotion bundle is fixed to Deployment/Service/Ingress. Adding a new
  promotable kind requires a migration, a typed request shape, a stripping
  rule, a dependency scan rule, a dry-run path and the full
  dry-run/confirmation/audit lifecycle. CRDs are explicitly out of scope.
- The platform never copies ConfigMap or Secret values. Operators must
  pre-provision destination dependencies and provide mappings. A promotion
  that depends on a missing destination dependency fails closed at preview.
- Namespace rewriting is metadata-only. Selectors, ingress backends and
  cross-namespace references are not rewritten. Operators are responsible
  for namespace-scoped consistency.
- The confirmation token is one-time. A replay with a different
  `Idempotency-Key` after execution is rejected with `ErrAlreadyExecuted`.
- The source manifest is fetched at preview time. If the source resource
  changes between preview and execution, the persisted (stripped) manifest
  is what gets applied; the platform does not re-fetch at execution time.
  This matches the M19/M23 contract: the plan records what was dry-run.
- Execution is partial-failure tolerant: a failed item does not abort the
  plan. Remaining items are still attempted, and the plan is marked
  `failed` if any item failed. Operators must inspect per-item status and
  reconcile manually.
- The observer RBAC must grant `get` on `deployments`, `services` and
  `ingresses` for `RawManifest`, and the platform's management RBAC must
  grant `create` on those kinds on the destination cluster. The
  least-privilege RBAC in `deploy/managed-cluster/observer.yaml` covers the
  read side; the E2E script asserts the write side is scoped to the
  fixture Namespace.

## Boundary

M24 does not include:

- Generic YAML/CRD promotion or arbitrary manifest editing.
- Promotion of ConfigMap, Secret, StatefulSet, DaemonSet, Job, CronJob, PVC
  or any CRD. The bundle is fixed to Deployment/Service/Ingress.
- Copying ConfigMap or Secret values across clusters. The platform only
  verifies destination dependencies exist by mapped name.
- Namespace selector or ingress backend rewriting. The operator is
  responsible for namespace-scoped consistency.
- Bulk project migration, namespace cloning, or "promote all" operations.
- Cross-cluster rollback. Once a plan is executed, the destination resources
  are independent; the platform does not track or reverse promotions.
- Background promotion status streaming. The wizard polls on demand.
- Workload backup/restore (M25) or organization integration (M26).
