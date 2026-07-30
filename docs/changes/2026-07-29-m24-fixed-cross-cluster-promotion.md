# M24: Fixed Cross-Cluster Promotion

- Status: Accepted
- Date: 2026-07-29
- Scope: cross-cluster promotion bundle, namespace mapping, dependency inventory, destination preflight, dry-run/confirm/execute contract, frontend wizard, real-kind E2E
- Decision: ADR 0041

## Outcome

M24 closes the cross-cluster promotion gap identified by the 2026-07-28
KRM/Ratel review. The platform now promotes a fixed bundle of
Deployment/Service/Ingress from a source cluster to a distinct destination
cluster through the same dry-run, typed diff, one-time confirmation,
idempotency, target preconditions and audit contract that bounds scale,
CronJob suspend/resume, Deployment image update and Deployment rollback.

The platform never accepts a client-owned manifest for execution. The
promoted manifest is fetched from the source cluster at preview time via
`kubernetes.RawManifest`, stripped of server-owned fields (`status`,
`metadata.uid`, `metadata.resourceVersion`, `metadata.creationTimestamp`,
`metadata.generation`, `metadata.managedFields`, `metadata.selfLink`,
`metadata.ownerReferences`, `metadata.finalizers`, `metadata.deletionTimestamp`,
`metadata.deletionGracePeriodSeconds`), and rewritten to the destination
namespace. The source UID and resourceVersion are captured before stripping
and recorded on the persisted bundle item as source evidence.

The platform never copies ConfigMap or Secret values. The operator provides
a `DependencyMapping` for each scanned ConfigMap/Secret reference; the
platform verifies the destination object exists by the mapped name and
fails closed on unresolved dependencies. References are scanned from the
Deployment pod template (`containers`/`initContainers` `envFrom`/`env`,
`volumes.configMap`, `volumes.secret.secretName`,
`volumes.projected.sources`, `imagePullSecrets`).

Migration `000019_cross_cluster_promotion` adds three tables:
`promotion_plans`, `promotion_bundle_items` and
`promotion_dependency_mappings`. CHECK constraints bind bundle `kind` to
`Deployment`/`Service`/`Ingress`, dependency `kind` to `ConfigMap`/`Secret`,
plan `status` to the six-state lifecycle, item `status` to the four-state
per-item lifecycle, bundle `ordinal` to `0-9`, and source/destination
cluster IDs to be distinct. Indexes cover source/destination/claim lookups.

## Product Surface

The promotion service `KubernetesSource` interface gains `RawManifest`,
`NamespaceExists`, `ConfigMapExists`, `SecretExists`, `ResourceExists` and
`CreateResource` methods so the business layer can read source evidence and
write destination resources without depending on the gateway HTTP surface.
`CreateResource` reuses the existing gateway `Create` path with
`?dryRun=All` support, content-type validation and a 256 KiB body bound.

Four HTTP routes are mounted under `v1`:

- `GET /api/v1/promotions` — list plans by `source_cluster_id` (required)
  and optional `namespace`. Any authenticated role.
- `POST /api/v1/promotions/preview` — dry-run and persist a plan.
  `system_admin` and `operations_admin` only. Uses `decodeStrictJSON` so
  unknown fields are rejected.
- `GET /api/v1/promotions/:promotion_id` — read a plan by ID. Any
  authenticated role.
- `POST /api/v1/promotions/:promotion_id/execute` — confirm and execute.
  `system_admin` and `operations_admin` only. Requires `Idempotency-Key`
  header (8-128 chars) and `confirmation_token` body.

The handler maps typed service errors to stable HTTP status codes and error
codes: `PROMOTION_NOT_FOUND` (404), `INVALID_PROMOTION` (400),
`PROMOTION_BUNDLE_EMPTY` (400), `PROMOTION_SOURCE_UNAVAILABLE` (409),
`PROMOTION_DESTINATION_UNAVAILABLE` (409), `PROMOTION_NAMESPACE_MISSING`
(409), `PROMOTION_DEPENDENCY_UNRESOLVED` (409), `PROMOTION_CONFLICT` (409),
`PROMOTION_PREVIEW_FAILED` (400), `PROMOTION_CONFIRMATION_INVALID` (403),
`INVALID_IDEMPOTENCY_KEY` (400), `PROMOTION_EXPIRED` (410),
`PROMOTION_IN_PROGRESS` (409), `PROMOTION_ALREADY_USED` (409),
`PROMOTION_FAILED` (502). Audit target and destination cluster ID are set
on every preview and execute call.

`Preview` runs preflight (destination namespace exists, no destination
resource already exists), fetches each source manifest, strips runtime
fields, rewrites the namespace, scans dependencies, verifies operator
mappings against the destination cluster, dry-run creates each item on the
destination cluster, and persists the plan with a one-time confirmation
token (SHA-256 hash stored, plaintext returned once). Plan TTL is 15
minutes; claim TTL is 2 minutes.

`Execute` requires a valid plan ID, confirmation token and idempotency
key. `repository.Claim` is the single atomic gateway that decides whether
to execute or replay: first confirmation transitions `awaiting_confirmation`
→ `executing` and records the idempotency key; same-key replay returns the
persisted plan without re-applying; different-key replay after execution is
rejected with `ErrAlreadyExecuted`. Execution applies bundle items in
ordinal order with per-item status tracking (`applied`/`skipped`/`failed`);
a conflict on the destination marks the item `skipped` and the plan
`partial`, any other failure marks the item `failed` and the plan `failed`,
and remaining items are still attempted.

The frontend `PromotionView` is a four-step wizard:

1. **Clusters & namespaces** — select source/destination cluster and
   source/destination namespace. Validates clusters are distinct and
   namespaces are non-empty.
2. **Bundle assembly** — add Deployment/Service/Ingress items by name; add
   ConfigMap/Secret dependency mappings. Validates at least one bundle item.
3. **Preflight confirmation** — call `previewPromotion`, render the typed
   bundle summary (item count, kind counts, dependency records) and require
   operator confirmation.
4. **Execution result** — call `executePromotion` with the confirmation
   token and a generated idempotency key, render per-item status and
   plan-level status.

The wizard reuses the existing `authorizedRequest` API client, `ConsoleLayout`
shell, and `primary-button`/`secondary-button` styles. Navigation is gated
to `system_admin` and `operations_admin` via `adminOnly` in `ConsoleLayout`.
New `wizard-*` and `promotion-*` CSS classes keep the layout aligned with
the existing `operation-*` styles.

## Real Kind Evidence

The disposable kind E2E script
`scripts/e2e-m24-cross-cluster-promotion-kind.ps1` and fixtures under
`deploy/m24-cross-cluster-promotion-e2e` exercise the full promotion flow
against two uniquely named kind clusters. The run:

- Creates two disposable kind clusters with unique names
  (`aiops-m24-source-<runID>`, `aiops-m24-dest-<runID>`), applies the
  `aiops-m24-source` fixture (ConfigMap `app-config` + Deployment
  `promote-target` + Service `promote-target`) on the source and the
  `aiops-m24-dest` fixture (ConfigMap `app-config` only) on the
  destination, installs the least-privilege observer RBAC, and registers
  both clusters with 1h ServiceAccount tokens.
- Previews a bundle of `Deployment/promote-target` and
  `Service/promote-target` with a `ConfigMap/app-config` dependency
  mapping; asserts the plan is `awaiting_confirmation`, the bundle summary
  carries `item_count=2`, `deployment_count=1`, `service_count=1`, the
  dependency record is marked `resolved`, and a confirmation token is
  returned.
- Executes the plan with a unique `Idempotency-Key`
  (`m24-promote-<runID>`); asserts the plan transitions to `succeeded`,
  both items are `applied`, and the promoted Deployment and Service exist
  on the destination cluster by name (verified via `kubectl get` on the
  destination context).
- Asserts idempotent replay with the same `Idempotency-Key` returns the
  same plan ID without re-applying.
- Verifies the observer RBAC: `get deployments`/`get services` is allowed
  on both clusters; `create deployments` is denied in `kube-system`.
- In `finally`, deletes both platform cluster registrations, deletes both
  disposable kind clusters, restores the previous kubectl context and
  refuses unsafe cleanup targets. Sanitized evidence is written to
  `.artifacts/m24-cross-cluster-promotion-kind`.

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `backend/internal/cluster/registry.go` | Modify | Add `Create` gateway method with content-type and body-size bounds |
| `backend/internal/kubernetes/service.go` | Modify | Add `CreateResource`, `RawManifest`, `ResourceExists`, `NamespaceExists`, `ConfigMapExists`, `SecretExists` to `KubernetesSource` |
| `backend/internal/promotion/model.go` | New | Plan, BundleItem, DependencyRecord models, typed errors, request DTOs |
| `backend/internal/promotion/repository.go` | New | GORM repository with `Save`, `Get`, `List`, `Claim`, `Complete`, `Fail` |
| `backend/internal/promotion/service.go` | New | `Preview` and `Execute` with preflight, stripping, namespace rewrite, dependency scan, dry-run, idempotent execution |
| `backend/internal/promotion/service_test.go` | New | 14 unit tests covering validation, preflight, stripping, dependency scan, dry-run failure, execution, conflict, idempotency |
| `backend/internal/httpserver/promotion.go` | New | `promotionHandler` with `preview`, `execute`, `get`, `list`; strict JSON decoding; error mapping |
| `backend/internal/httpserver/router.go` | Modify | Register promotion routes with role gating |
| `backend/cmd/server/main.go` | Modify | Wire promotion service into HTTP server |
| `backend/migrations/000019_cross_cluster_promotion.up.sql` | New | Schema for `promotion_plans`, `promotion_bundle_items`, `promotion_dependency_mappings` with CHECK constraints and indexes |
| `backend/migrations/000019_cross_cluster_promotion.down.sql` | New | Rollback migration |
| `frontend/src/types/promotion.ts` | New | TypeScript interfaces for promotion API requests/responses |
| `frontend/src/api/promotion.ts` | New | API client for `previewPromotion`, `executePromotion`, `getPromotion`, `listPromotions` |
| `frontend/src/api/promotion.test.ts` | New | Unit tests for promotion API client |
| `frontend/src/views/PromotionView.vue` | New | Four-step promotion wizard |
| `frontend/src/router/index.ts` | Modify | Add `/promotions` route with role-based access control |
| `frontend/src/components/ConsoleLayout.vue` | Modify | Add `跨集群 Promotion` navigation item (admin only) |
| `frontend/src/styles/base.css` | Modify | `wizard-*` and `promotion-*` styles |
| `docs/api/openapi.yaml` | Modify | `/api/v1/promotions/*` routes, `PromotionID` parameter, `PromotionPreviewRequest`/`PromotionExecuteRequest` schemas |
| `deploy/m24-cross-cluster-promotion-e2e/source/namespace.yaml` | New | Source E2E Namespace |
| `deploy/m24-cross-cluster-promotion-e2e/source/workloads.yaml` | New | Source ConfigMap, Deployment, Service fixtures |
| `deploy/m24-cross-cluster-promotion-e2e/source/kustomization.yaml` | New | Source Kustomize entry |
| `deploy/m24-cross-cluster-promotion-e2e/destination/namespace.yaml` | New | Destination E2E Namespace |
| `deploy/m24-cross-cluster-promotion-e2e/destination/configmap.yaml` | New | Destination ConfigMap fixture (dependency target) |
| `deploy/m24-cross-cluster-promotion-e2e/destination/kustomization.yaml` | New | Destination Kustomize entry |
| `scripts/e2e-m24-cross-cluster-promotion-kind.ps1` | New | Disposable two-cluster kind E2E script |
| `docs/adr/0041-fixed-cross-cluster-promotion.md` | New | Design decision |
| `docs/changes/2026-07-29-m24-fixed-cross-cluster-promotion.md` | New | Closure change log |
| `docs/roadmap.md` | Modify | Mark M24 ✅ Completed |

## Verification

- **Backend**: `go test ./...` — packages pass, including
  `TestPreviewRejectsSameSourceAndDestinationCluster`,
  `TestPreviewRejectsEmptyBundle`,
  `TestPreviewFailsWhenDestinationNamespaceMissing`,
  `TestPreviewFailsWhenDestinationResourceExists`,
  `TestPreviewStripsRuntimeFieldsAndRewritesNamespace`,
  `TestPreviewRequiresDependencyMappings`,
  `TestPreviewFailsWhenDestinationDependencyMissing`,
  `TestPreviewSucceedsWithResolvedDependencies`,
  `TestPreviewScansSecretRefsInVolumesAndEnv`,
  `TestPreviewDryRunFailureReturnsPreviewFailed`,
  `TestExecuteAppliesBundleItemsInOrder`,
  `TestExecuteConflictMarksItemSkippedAndPlanPartial`,
  `TestExecuteInvalidIdempotencyKeyRejected`,
  `TestExecuteMissingConfirmationTokenRejected`.
- **Backend**: `go vet ./...` — no warnings.
- **Frontend**: `vue-tsc -b` — type-check passes clean.
- **Frontend**: `vitest run` — API tests pass for `previewPromotion`,
  `executePromotion`, `getPromotion`, `listPromotions`.
- **OpenAPI parity**: `openapi_route_test` validates that all registered
  routes (including the four promotion routes) are documented; the
  `PromotionPreviewRequest` schema enumerates the bundle and
  dependency_mappings shapes.
- **Migration**: `000019` applies cleanly and the CHECK constraints reject
  invalid `kind`, `status`, `ordinal` and same-cluster plans.

## 2026-07-30 Baseline Alignment

The baseline audit added four hardening corrections found during integration:

- Dependency evidence is deduplicated at bundle scope, so two promoted items
  referencing the same ConfigMap produce one dependency record.
- Mapped ConfigMap/Secret names are rewritten in the destination manifest;
  Service cluster-assigned fields are removed before dry-run and persistence.
- Execution persists each item's final status/error independently and public
  responses do not expose the internal stripped manifest.
- Destination writes use a Namespace-scoped promotion manager Role and
  RoleBinding instead of widening the observer role.

Fresh evidence
`.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-074812.json`
passed with two bundle items, one dependency, the mapped
`app-config-promoted` reference, idempotent replay and complete cleanup.

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

The source manifest is fetched at preview time and persisted (stripped) on
the plan. If the source resource changes between preview and execution, the
persisted manifest is what gets applied; the platform does not re-fetch at
execution time. This matches the M19/M23 contract: the plan records what
was dry-run.
