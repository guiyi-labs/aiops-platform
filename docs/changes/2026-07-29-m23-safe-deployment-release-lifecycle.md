# M23: Safe Deployment Release Lifecycle

- Status: Accepted
- Date: 2026-07-29
- Scope: rollout history/status contracts, controlled image update, exact-revision rollback, frontend release controls, real-kind E2E
- Decision: ADR 0040

## Outcome

M23 closes the release lifecycle gap left open by M19. The platform now
derives rollout history and status from exact Deployment and ReplicaSet
revision/template evidence, and exposes two new fixed controlled
operations — `deployment.image_update` and `deployment.rollback` — through
the same dry-run, typed diff, one-time confirmation, idempotency, target
UID/resourceVersion preconditions and audit contract that bounds scale
and CronJob suspend/resume.

The platform never accepts a client-owned rollback template. The
rollback patch is derived at execution time from the live ReplicaSet
selected by `metadata.annotations["deployment.kubernetes.io/revision"]`,
and the ReplicaSet UID and resourceVersion are recorded as target
preconditions on the persisted plan. A retry that finds the ReplicaSet
UID changed between diagnosis and execution fails closed with
`Kubernetes remediation target changed after diagnosis` and never
applies a stale template.

Migration `000018_deployment_release_lifecycle` extends `remediation_plans`
with `container_name`, `before_image`, `desired_image`,
`rollback_revision`, `rollback_replicaset_name`, `rollback_replicaset_uid`
and `rollback_replicaset_resource_version`, and rewrites the action and
parameter CHECK constraints so each action is bound to its valid
parameter shape. `deployment.image_update` requires a non-empty
container name plus before/desired images; `deployment.rollback`
requires a positive `rollback_revision` and a non-empty
`rollback_replicaset_uid`.

## Product Surface

The Kubernetes service exposes `RolloutHistory` and `RolloutStatus`
methods that read the Deployment, its current revision annotation, and
the ReplicaSets that own the same controller UID. `RolloutHistory`
returns a stable `RolloutRevision` list with `revision`,
`replicaset_name`, `uid`, `resource_version`, `created_at`, replica
counts, images and a `current` flag. `RolloutStatus` returns the
current revision, desired/updated/ready/available/unavailable replicas
and a derived `phase` (`complete`, `progressing`, `degraded`).

Two new read-only HTTP routes are mounted under `resourceRoutes`:

- `GET /api/v1/clusters/{cluster_id}/deployments/:namespace/:name/rollout/history`
- `GET /api/v1/clusters/{cluster_id}/deployments/:namespace/:name/rollout/status`

The existing `POST /api/v1/clusters/{cluster_id}/operations/preview`
endpoint accepts two new action shapes:

- `deployment.image_update` with `namespace`, `target_name`,
  `container_name` and `desired_image`.
- `deployment.rollback` with `namespace`, `target_name` and
  `rollback_revision`.

The remediation service `KubernetesSource` interface gains
`ReplicaSet`, `ReplicaSetsByOwner`, `RolloutHistory` and
`RolloutStatus` methods so the business layer can derive evidence
without depending on the gateway HTTP surface. `patchForOperation`
builds the image-update JSON patch from the typed request, and
`buildRollbackPatch` fetches the selected ReplicaSet at execution time
and projects its `spec.template` into a strategic merge patch. The
generic `ErrTargetChanged` is mapped to the existing
`Kubernetes remediation target changed after diagnosis` execution
error.

The frontend `ResourceDetailView` adds a Rollout tab that loads history
and status in parallel via `Promise.allSettled`, rendering phase cards
(`Phase`, `Current revision`, `Updated/Ready/Available`, `Unavailable`)
and a revision table with replica counts, images and the current
marker. `WorkloadsView` extends the Deployment drawer with a release
control section that mirrors the existing scale control: a container
selector plus desired image input for image update, a revision selector
for rollback, and a shared operation preview/confirm dialog that
renders the typed diff (`spec.template.spec.containers[<name>].image`
for image update, `spec.template (revision rollback)` for rollback).
New `release-control-*` CSS classes keep the layout aligned with the
existing `operation-*` styles.

## Real Kind Evidence

The disposable kind E2E script
`scripts/e2e-m23-release-lifecycle-kind.ps1` and fixtures under
`deploy/m23-release-lifecycle-e2e` exercise the full lifecycle against
a uniquely named kind cluster. The run:

- Creates a disposable kind cluster, applies the `release-target`
  fixture (two replicas, `revisionHistoryLimit: 10`), installs the
  least-privilege observer RBAC and registers only the disposable
  cluster with a 1h ServiceAccount token.
- Reads baseline rollout history and status; asserts `current_revision`
  is 1, the status `phase` is `complete`, and revision 1 is marked
  current.
- Previews and executes `deployment.image_update` from
  `registry.k8s.io/pause:3.10` to `registry.k8s.io/pause:3.9`; asserts
  the typed `change` (`before`/`after`), idempotent replay returns the
  same plan, the Deployment image advances, and the rollout history
  current revision advances to 2 with revision 2 marked current.
- Previews and executes `deployment.rollback` to revision 1; asserts
  idempotent replay returns the same plan, the Deployment image is
  restored to the baseline, the rollout history current revision
  advances to 3, revision 3 is marked current, and revision 3's image
  list contains the restored baseline image.
- Verifies the observer RBAC: `patch deployments` is allowed in the
  fixture Namespace, denied in `kube-system`; `list replicasets` is
  allowed; `delete pods` is denied.
- Reads the operation history endpoint and asserts both
  `deployment.image_update` and `deployment.rollback` plans are
  present for the target Deployment.
- In `finally`, deletes the platform cluster registration, deletes the
  disposable kind cluster, restores the previous kubectl context and
  refuses unsafe cleanup targets. Sanitized evidence is written to
  `.artifacts/m23-release-lifecycle-kind`.

## Files changed

| Path | Kind | Purpose |
|------|------|---------|
| `backend/internal/kubernetes/service.go` | Modify | Add `OwnerReferences`, Deployment conditions, `ReplicaSetsByOwner`, `RolloutHistory`, `RolloutStatus` |
| `backend/internal/remediation/model.go` | Modify | Add image update/rollback action constants, `OperationRequest` and `Plan` fields, response shape |
| `backend/internal/remediation/service.go` | Modify | Extend `KubernetesSource`, add `patchForOperation` and `buildRollbackPatch`, update `PreviewOperation`/`Execute` |
| `backend/internal/remediation/service_test.go` | Modify | Extend `kubernetesStub`, add image update/rollback/rollout history tests |
| `backend/internal/httpserver/remediation.go` | Modify | Add `previewOperation` image/rollback fields, `rolloutHistory` and `rolloutStatus` handlers |
| `backend/internal/httpserver/router.go` | Modify | Register rollout history/status routes |
| `backend/migrations/000018_deployment_release_lifecycle.up.sql` | New | Schema columns and CHECK constraints for M23 actions |
| `backend/migrations/000018_deployment_release_lifecycle.down.sql` | New | Rollback migration |
| `frontend/src/types/diagnosis.ts` | Modify | Extend `RemediationAction`, `ControlledOperationRequest`, `RemediationPlan`, add rollout types |
| `frontend/src/api/diagnosis.ts` | Modify | Add `getRolloutHistory` and `getRolloutStatus` |
| `frontend/src/api/diagnosis.test.ts` | Modify | Tests for image update, rollback, rollout history/status |
| `frontend/src/views/ResourceDetailView.vue` | Modify | Rollout tab with history/status |
| `frontend/src/views/WorkloadsView.vue` | Modify | Image update and rollback controls in Deployment drawer |
| `frontend/src/styles/base.css` | Modify | `release-control-*` styles |
| `docs/api/openapi.yaml` | Modify | Rollout routes and `ControlledOperationPreviewRequest` variants |
| `deploy/m23-release-lifecycle-e2e/namespace.yaml` | New | E2E Namespace |
| `deploy/m23-release-lifecycle-e2e/workloads.yaml` | New | `release-target` Deployment fixture |
| `deploy/m23-release-lifecycle-e2e/kustomization.yaml` | New | Kustomize entry |
| `deploy/m23-release-lifecycle-e2e/README.md` | New | Fixture docs |
| `scripts/e2e-m23-release-lifecycle-kind.ps1` | New | Disposable kind E2E script |
| `docs/adr/0040-safe-deployment-release-lifecycle.md` | New | Design decision |
| `docs/changes/2026-07-29-m23-safe-deployment-release-lifecycle.md` | New | Closure change log |
| `docs/roadmap.md` | Modify | Mark M23 ✅ Completed |

## Verification

- **Backend**: `go test ./...` — packages pass, including
  `TestPreviewImageUpdateStoresTypedDiffAndUsesDryRun`,
  `TestPreviewRollbackDerivesTemplateFromReplicaSetEvidence`,
  `TestExecuteRollbackFailsWhenReplicaSetUIDChanged`,
  `TestExecuteImageUpdateIsIdempotent`,
  `TestRolloutHistoryEnumeratesRevisionsByDeploymentUID`.
- **Backend**: `go vet ./...` — no warnings.
- **Frontend**: `vue-tsc -b` — type-check passes clean.
- **Frontend**: `vitest run` — API tests pass for `getRolloutHistory`,
  `getRolloutStatus`, `image_update` preview and `rollback` preview.
- **OpenAPI parity**: `openapi_route_test` validates that all registered
  routes (including the new rollout history/status routes) are
  documented; the `ControlledOperationPreviewRequest` schema enumerates
  the image_update and rollback variants.
- **Migration**: `000018` applies cleanly and the parameter CHECK
  constraint rejects invalid action/parameter combinations.

## Boundary

M23 does not include:

- Generic YAML/CRD editing or arbitrary patch endpoints.
- Rollback to a revision whose ReplicaSet has been pruned by
  `revisionHistoryLimit`. The platform fails closed with
  `ErrTargetChanged` when the recorded UID no longer matches.
- Cross-cluster promotion (M24) or workload backup/restore (M25).
- Background rollout status streaming; the Rollout tab refreshes on
  navigation and on demand.
- Pod-level image update or StatefulSet/DaemonSet rollback. The action
  catalog remains fixed to Deployment image update and Deployment
  rollback.

The rollback patch deliberately does **not** record a
`k8s-aiops.local/rollback-replicaset-uid` annotation on the Deployment.
Recording the source UID on the live object would leak stale identity
into subsequent revisions and break the UID-precondition check on
later rollbacks. The UID is recorded only on the persisted plan.
