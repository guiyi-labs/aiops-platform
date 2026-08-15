# Product Optimization Roadmap

- Updated: 2026-07-31 (M35, M36, M37, M38, M39, M40 and M41 closure recorded)
- Baseline: M1-M32 local development is archived by
  `baseline-m32-20260731`. M27-M31 have disposable real-environment evidence;
  the fresh full gate is
  `.artifacts/verification/verify-20260731-015255.json`.
  M33-M41 are development complete on local `main`; only hosted
  CI/release, organization OIDC/MFA production run, PITR and HA remain
  external gates. Next development targets are M42 (multi-signal correlation
  and deterministic RCA) or M43-M44 (AIOps differentiation).
- Principle: close high-frequency operator workflows with fixed, evidence-based contracts; do not chase generic Kubernetes CRUD parity

## Baseline Closure Audit (2026-07-30)

This section is historical for the M21-M25 checkpoint. The authoritative final
closure is `docs/changes/2026-07-31-final-baseline-archive.md`.

- M21 sustained-window evaluation, sparse history, trend UI and diagnosis
  evidence passed the full real-kind outage/recovery/restart gate.
- M22 fixed read-only troubleshooting resources, container-aware logs and
  redacted manifest inspection passed backend, frontend and production-build
  gates; its closure record is
  `docs/changes/2026-07-30-m22-daily-troubleshooting-and-governance-workbench.md`.
- M23 exact ReplicaSet-backed image update and rollback retained complete
  PodTemplate state and passed its disposable real-kind lifecycle gate.
- M24 promotion now strips cluster-assigned Service fields, rewrites mapped
  dependencies, persists per-item results and deduplicates bundle-level
  dependency evidence. Fresh dual-cluster evidence reports one dependency.
- M25 remains read-only. Its fixture now waits for the Velero CRD to become
  `Established` before applying Backup objects, and the fresh dual-cluster
  gate proves installed/unavailable behavior and read-only RBAC.

## Release Prerequisite

The human-reviewed baseline, private remote, hosted CI and dependency review
are accepted. GitHub rejected branch protection for this private repository
under the current account plan (HTTP 403); the repository remains private and
no policy bypass is claimed. Before a formal release, register the dedicated
runner, complete security/backup/HA review, tag the release and recapture
screenshots bound to the reviewed revision. No public repository or release is
claimed yet.

## M16: Metrics Available Path

- Status: Accepted on 2026-07-27. Evidence is archived in
  `docs/changes/2026-07-27-real-metrics-utilization-consumers.md`.

- Validate Node and Pod Metrics against a real kind cluster with a pinned Metrics
  Server fixture while keeping Metrics Server optional in normal deployment.
- Calculate Node CPU and memory utilization only from real, name-matched
  `status.allocatable` denominators.
- Add bounded Pod CPU/memory consumer ranking with explicit sample coverage.
- Preserve the M15 unavailable path and independent Dashboard loading behavior.

## M17: Common Workload Coverage

- Status: Accepted on 2026-07-27. Evidence is archived in
  `docs/changes/2026-07-27-common-workload-policy-coverage.md`.

- Add fixed read-only list/detail/event contracts for StatefulSet, DaemonSet,
  ReplicaSet, Job, CronJob and HPA.
- Add ResourceQuota, LimitRange and Secret metadata/key-name contracts without
  exposing Secret values.
- Extend deep links, health summaries and topology only through exact,
  namespace-safe relationships; do not add an arbitrary GVK proxy.

## M18: Evidence-Based Diagnosis Expansion

- Status: Accepted on 2026-07-27. Evidence is archived in
  `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`.

- Add Node pressure, PVC Pending, HPA saturation, Ingress backend and sustained
  restart rules where the required evidence is observable.
- Add replayable diagnosis fixtures, versioned rule tests and quality summaries.
- Keep deterministic rules authoritative; AI remains a cited explanation layer.

## M19: Controlled Operations Catalog

- Status: Accepted on 2026-07-27. Evidence is archived in
  `docs/changes/2026-07-27-controlled-operations-catalog.md`.

- Add Deployment scale and CronJob suspend/resume as fixed resource actions;
  preserve the diagnosis-bound Deployment rollout restart.
- Require server-side dry-run, diff, target snapshot, confirmation, idempotency,
  scoped RBAC, audit and failure-safe retry for every action.
- Do not expose arbitrary YAML editing or generic patch endpoints.
- Defer Deployment rollback until exact ReplicaSet revision/template history can
  be selected, snapshotted and dry-run without client-owned patch content.

## M20: Multi-Cluster Efficiency And Production Hardening

- Status: Accepted through Phase 12 on 2026-07-28. Phase 1 bounded health
  fan-out, Phase 2 disposable fleet E2E, Phase 3 fixed-kind global search,
  Phase 4 user-owned saved filters, Phase 5 disposable search E2E and Phase 6
  versioned CI/release pipeline are archived in
  `docs/changes/2026-07-27-bounded-multi-cluster-health.md` and
  `docs/changes/2026-07-27-two-cluster-fleet-e2e.md`, with the Phase 3 record at
  `docs/changes/2026-07-27-bounded-global-resource-search.md` and Phase 4 at
  `docs/changes/2026-07-27-user-owned-global-search-filters.md`; Phase 5 is at
  `docs/changes/2026-07-27-two-cluster-global-search-e2e.md`; Phase 6 is at
  `docs/changes/2026-07-28-versioned-ci-release-pipeline.md`.

- Phase 1 defines bounded cross-cluster fan-out, per-cluster timeout,
  partial-failure, sample-coverage and authorization semantics and adds a
  compact health comparison table.
- Phase 2 validates physically distinct kind clusters, stable ordering, fixed
  resource counts, timeout/recovery/unavailable isolation, least-privilege RBAC
  and complete cleanup in a platform environment isolated from retained data.
- Phase 3 adds bounded name/Namespace search for Pod, Deployment, Service and
  Ingress only. It fixes 20-cluster, four-worker, four-second, per-kind and
  global-result caps; exposes stable failure codes and enabled-cluster coverage;
  and deep-links normalized results into the existing resource workbench.
- Phase 4 persists only the reviewed Phase 3 query shape as private per-user
  filters. It fixes a 20-item cap, case-insensitive names, ownership-scoped
  CRUD, rename/overwrite/delete, stale-schema compatibility and mutation audit;
  it does not add sharing, selectors, arbitrary kinds or scheduled execution.
- Phase 5 validates the Phase 3 contract against two physically distinct
  Kubernetes v1.34.0 kind clusters and an isolated platform. It proves fixed
  kind/order/coverage/truncation semantics, timeout/recovery/query-failure
  isolation, observer read-only RBAC and complete cleanup without changing the
  API or persistence surface.
- Phase 6 adds a pinned, least-privilege regular CI workflow, semantic-version
  package/tag release workflow, scheduled/manual disposable real-kind workflow
  and grouped dependency updates. The reviewed local baseline was subsequently
  created and verified; registry publication and release credentials remain
  outside this phase.
- Phase 7 reviews grouped dependency updates, limits Dependabot groups to
  minor/patch changes, separates major migrations into independent PRs, moves
  pnpm setup to the signed Node 24 action, and archives the account-plan limit
  on private-repository branch protection. Actions, Go modules, Vue patch
  updates and the final combined CI run were accepted without a release tag.
- Phase 8 adds a two-instance PostgreSQL 17 logical backup/restore drill. It
  applies the complete migration set, inserts synthetic relational data,
  destroys the source, restores a fresh target, verifies migration/data/FK
  invariants and removes all temporary backup material. Production RPO/RTO,
  off-cluster retention, PITR and HA are explicitly not claimed. The local and
  hosted Ubuntu gates were accepted on 2026-07-28.
- Phase 9 adds a bounded active-plus-legacy credential keyring, default dry-run
  and explicit-apply offline command, per-batch transaction rollback, sanitized
  run metadata and an isolated v1-to-v2 physical gate. Local and hosted Ubuntu
  acceptance completed on 2026-07-28.
- Phase 10 adds bounded offline audit selection, canonical JSON plus a detached
  Ed25519 manifest, externally anchored verification, overwrite refusal and an
  isolated PostgreSQL tamper/overflow/cleanup gate. The 2026-07-28 complete
  local gate and Ubuntu hosted CI are accepted.
- Phase 11 adds a provider-neutral OIDC/MFA policy contract, strict offline
  discovery/JWKS validation, 14 fail-closed admission checks and a
  network-disabled downgrade drill. It creates no login endpoint and does not
  claim production SSO; local and hosted Ubuntu acceptance completed on
  2026-07-28.
- Phase 12 adds an explicit recovery-objective and infrastructure policy,
  strict logical-evidence admission, bounded PITR/HA risk acceptance and a
  network-disabled 15-check gate. It marks the policy ready for PITR/HA
  implementation while explicitly refusing a production-recovery claim; local
  and hosted Ubuntu acceptance completed on 2026-07-28.

## Competitive Reassessment

The 2026-07-28 KRM/Ratel review is archived at
`docs/references/krm-ratel-gap-analysis.md`. It found that this platform is
stronger in diagnosis evidence, credential safety, controlled mutation, audit,
recovery admission and delivery verification, but narrower in release
lifecycle, cross-cluster promotion and cluster-workload backup. Historical
observability is also incomplete for a product positioned as AIOps.

Generic YAML/CRD mutation, sensitive-value display, unrestricted Pod exec/file
transfer, bulk project migration and one-click restore are not accepted as
parity targets.

## M21: Historical Observability And Alert Evidence

- Status: Accepted on 2026-07-29. All six phases are complete and
  verified via `go test ./...` (18 backend packages), `vue-tsc -b`
  (zero frontend type errors), and `vitest run` (66 frontend unit tests).
  Frontend trigger buttons on the Dashboard trend consumer invoke the
  synchronous Node metrics diagnosis endpoint with inline result display.
  Future alerting (background pipelining, multi-metric correlation, Pod-level
  diagnosis, deduplication) is explicitly out of scope and will be tracked
  under a future roadmap item.
- Closure change log: `docs/changes/2026-07-29-m21-sustained-window-evaluation-and-trend-consumers.md`
- ADRs: 0034 (storage), 0035 (collector), 0036 (query), 0037 (evaluation engine), 0038 (diagnosis evidence integration)
- Phase 1 adds ADR 0034, migration 17 and a PostgreSQL-backed exact-series
  domain contract with seven-day default retention, 1,800-sample collection,
  24-hour query, 1,440-point and batch-cleanup caps. Missing samples remain
  missing, never zero.
- Phase 2 adds ADR 0035 and an in-process collector over enabled clusters with
  stable ID ordering, 20-cluster/four-way concurrency defaults, per-cluster
  timeout, official Kubernetes quantity conversion, six stable failure codes,
  fair Node/Pod cap allocation and deterministic bounded cleanup scheduling.
- Phase 3 adds ADR 0036 and an authenticated exact-series HTTP contract with
  strict Node/Pod shape, explicit RFC3339 window, sparse coverage, stable
  errors, OpenAPI parity and PostgreSQL restart/isolation E2E.
- Phase 4 adds ADR 0037, a deterministic sustained-window evaluation engine
  that detects all breach windows (not just trailing), an authenticated
  evaluation route, and a frontend Dashboard trend consumer rendering SVG
  charts with evaluation state badges.
- Phase 5 adds ADR 0038, a diagnosis rule `node.metric_sustained_breach.v1`
  that maps sustained-window evaluation evidence into diagnosis records with
  structured `Evidence` entries per window plus a summary, severity mapping
  (CPU → high, memory → medium), and a synchronous `DiagnoseNodeMetrics`
  service method that bridges the metrics history evaluator to the diagnosis
  lifecycle.
- Phase 6 wires the `MetricEvaluator` interface into `main.go`, exposes
  `POST /api/v1/clusters/{cluster_id}/diagnoses/node_metrics` with full
  field validation and error mapping, adds `DiagnoseNodeMetricsRequest`
  OpenAPI schema, and covers the handler with validation, no-match and
  error-branching tests.
- Retain Node/Pod CPU and memory samples with explicit source timestamp,
  collection result, coverage and expiry; clean expired data deterministically.
- Add trend views and deterministic sustained-window evaluation linked to the
  existing event, diagnosis and controlled-operation records.
- Validate sampling, retention, gaps, restart recovery and query isolation
  against real kind. Do not build a Prometheus replacement or allow unbounded
  label cardinality.

## M22: Daily Troubleshooting And Governance Workbench

- Status: ✅ Completed.
- Phase 1 (Container-Aware Pod Logs): Container enumeration (`Containers`),
  bounded `LogsSince` with time-filtering and truncation disclosure,
  `AllContainerLogs` multi-container aggregation, frontend PodLogsViewer
  with container tabs, search, download, and previous-log toggle.
- Phase 2 (Read-Only Resource Contracts): Fixed read-only endpoints for
  PersistentVolume, PodDisruptionBudget, NetworkPolicy, ServiceAccount
  (list + detail) with pagination and typed response envelopes.
- Phase 3 (Redacted Manifest Inspection): Server-side manifest redaction
  for approved kinds (Pod, Deployment, Service, Ingress, PVC, PV, PDB,
  NetworkPolicy, ServiceAccount, Role, ClusterRole). Sensitive fields
  (password, token, secret, data, etc.) replaced with `<redacted>`.
  Allowlist fails closed with 404 for non-approved kinds.
- Phase 4 (Workbench UX Split): New `ResourceDetailView` with tabbed
  interface (overview, spec, status, events, logs, manifest, tasks).
  New `PodLogsViewer` and `ResourceManifestViewer` components.
  Updated WorkloadsView with new resource categories and navigation
  to detail view. Backend manifest endpoint with `GET /resources/:kind/:ns/:name/manifest`.
  All endpoints strictly read-only.


## M23: Safe Deployment Release Lifecycle

- Status: ✅ Completed on 2026-07-29. Verified via `go test ./...` (backend
  unit tests covering image update, rollback, ReplicaSet UID/resourceVersion
  preconditions, idempotent replay and rollback patch derivation), `vue-tsc -b`
  (zero frontend type errors), and `vitest run` (frontend API tests for
  `getRolloutHistory`, `getRolloutStatus`, `image_update` and `rollback`
  operations). The disposable real-kind E2E script
  `scripts/e2e-m23-release-lifecycle-kind.ps1` exercises the full lifecycle
  against a uniquely named kind cluster.
- Closure change log: `docs/changes/2026-07-29-m23-safe-deployment-release-lifecycle.md`
- ADRs: 0040 (release lifecycle contract)
- Derive rollout history from exact Deployment and ReplicaSet revision/template
  evidence; never accept a client-owned rollback template. ReplicaSet
  `metadata.annotations["deployment.kubernetes.io/revision"]` is the canonical
  revision anchor and the platform records the ReplicaSet UID and
  resourceVersion as target preconditions before any patch.
- Add fixed container-image update and exact revision rollback actions through
  server-side dry-run, typed diff, one-time confirmation, idempotency, target
  UID/resourceVersion preconditions and audit. Migration `000018` extends
  `remediation_plans` with `container_name`, `before_image`, `desired_image`,
  `rollback_revision`, `rollback_replicaset_name`, `rollback_replicaset_uid`
  and `rollback_replicaset_resource_version`, and the action/parameter CHECK
  constraint binds each action to its valid parameter shape.
- Expose rollout progress, failure reason and operation history in the
  resource detail workflow through `GET /deployments/:namespace/:name/rollout/history`
  and `GET /deployments/:namespace/:name/rollout/status`. The
  `ResourceDetailView` gains a Rollout tab and `WorkloadsView` adds
  image-update and rollback controls in the Deployment drawer, reusing the
  existing controlled-operation confirmation flow.
- Prove update/rollback/restoration against disposable real-kind fixtures
  in `deploy/m23-release-lifecycle-e2e`. The script never reuses `aiops-test`,
  registers only the disposable cluster, asserts the revision graph advances
  1 → 2 → 3 across image update then rollback, verifies idempotent replay
  returns the same plan, and confirms the restored deployment image matches
  the baseline.

## M24: Fixed Cross-Cluster Promotion

- Status: ✅ Completed on 2026-07-29. Verified via `go test ./...` (backend
  unit tests covering preview validation, preflight, runtime-field
  stripping, namespace rewriting, dependency scanning, dependency mapping
  verification, dry-run failure, ordinal-ordered execution, conflict
  skipping, idempotency key and confirmation token enforcement),
  `vue-tsc -b` (zero frontend type errors), and `vitest run` (frontend
  API tests for `previewPromotion`, `executePromotion`, `getPromotion`,
  `listPromotions`). The disposable real-kind E2E script
  `scripts/e2e-m24-cross-cluster-promotion-kind.ps1` exercises the full
  promotion flow against two uniquely named kind clusters.
- Closure change log: `docs/changes/2026-07-29-m24-fixed-cross-cluster-promotion.md`
- ADRs: 0041 (fixed cross-cluster promotion)
- Start with a reviewed Deployment/Service/Ingress promotion bundle, explicit
  source and target, Namespace mapping, dependency inventory and destination
  capability preflight.
- Strip runtime/server-owned fields, require mappings for referenced
  ConfigMaps/Secrets instead of copying sensitive values, and fail closed on
  unresolved dependencies or conflicts.
- Reuse the controlled-operation contract for target dry-run, typed diff,
  confirmation, idempotency, audit, partial-failure reporting and two-cluster
  cleanup/restoration evidence. Do not add bulk project migration.

## M25: Cluster Workload Protection Integration

- Status: ✅ Completed on 2026-07-29. Verified via `go test ./...` (backend
  unit tests covering Velero capability detection for installed/absent API
  groups, `ErrVeleroUnavailable` when the API group is missing, bounded
  projection and pagination of the backup list, namespace-scoped list path,
  single-resource read, and the not-found vs unavailable distinction),
  `vue-tsc -b` (zero frontend type errors), and the `openapi_route_test`
  (the three new read-only routes and the `VeleroCapability`/`VeleroBackup`/
  `VeleroBackupList` schemas are documented). The disposable real-kind E2E
  script `scripts/e2e-m25-workload-protection-kind.ps1` exercises the
  read-only inventory contract against a uniquely named kind cluster using a
  minimal Velero CRD stub and sample Backup CRs.
- Closure change log: `docs/changes/2026-07-29-m25-cluster-workload-protection-integration.md`
- ADRs: 0042 (cluster workload protection integration)
- Detect an optional reviewed Velero API capability without making it a core
  platform dependency; show bounded backup inventory, scope, phase, expiry and
  failure details.
- Add controlled backup creation only after read-only inventory and real-kind
  compatibility are accepted. Never collect object-storage credentials through
  the browser.
- Keep restore disabled until destination isolation, resource conflict,
  persistent-volume behavior, cutover and rollback policies have separate
  approval and disposable recovery evidence.

## M27: Historical Alert Lifecycle

- Status: ✅ Completed on 2026-07-30. Verified via `go test ./...` (backend
  unit tests covering rule validation, state machine, deduplication, concurrent
  claims, expired-claim recovery, scheduler bounds, and error handling),
  `vue-tsc -b` (zero frontend type errors), `vitest run` (73 frontend tests),
  and final repository gates. Disposable Metrics Server E2E passed firing,
  deduplication, outage containment, complete normal-window resolution,
  restart durability and cleanup; evidence is under
  `.artifacts/m27-alert-lifecycle-kind/`.
- Closure change log: `docs/changes/2026-07-30-m27-alert-lifecycle.md`
- ADRs: 0043 (historical alert lifecycle)
- Build a bounded background evaluator, deduplication and acknowledgement
  lifecycle over the accepted M21 exact-series contract.
- Preserve deterministic diagnosis as the source of truth and avoid arbitrary
  PromQL or unbounded label cardinality.

## M28: Controlled Velero Backup Creation

- Status: ✅ Completed on 2026-07-30. Verified via `go test ./...` (all 23 backend
  packages pass, including 13 new `backup` tests covering request validation,
  Velero capability preflight, storage location validation, name conflict
  detection, dry-run preflight, confirmation token hashing, idempotent claim,
  execution success/failure paths), `vue-tsc -b` (zero frontend type errors),
  `vitest run` (73 frontend tests), and `scripts/verify-fast.ps1 -Scope All`
  plus the final repository gates. Pinned Velero v1.15.2 + disposable MinIO
  E2E passed; evidence is `.artifacts/m28-backup-creation-kind/summary.json`.
- Closure change log: `docs/changes/2026-07-30-m28-controlled-backup-creation.md`
- ADRs: 0044 (controlled Velero backup creation)
- Add Velero Backup creation through fixed scope, server-side preflight (Velero
  installed, exact source identity, Available BSL, generated name and dry-run),
  one-time confirmation, idempotency, and audit. M31 adds only the isolated
  quarantine rehearsal; in-place/PV restore and cutover remain prohibited.

## M29: Namespace Governance and Capacity Posture

- Status: ✅ Completed on 2026-07-31. Verified via `go test ./...` (all 24 backend
  packages pass, including 10 new `namespaceposture` tests covering required
  Namespace metadata read, ErrResourceNotFound propagation, partial-section
  containment, workload+pod aggregation, list count summary, truncation status,
  phase sort, copyMap isolation, stringValue numeric coercion, and namespaced
  API path rendering), `vue-tsc -b` (zero frontend type errors after removing
  unused `BackupStorageLocation` import and rewriting LimitRange row iteration),
  `vitest run` (73 frontend tests, 17 files), and `scripts/verify-fast.ps1
  -Scope All` (passed in 28.56s, backend=True frontend=True manifests=True).
  Disposable real-kind governance E2E passed with deterministic critical
  findings; evidence is `.artifacts/m29-governance-posture-kind/summary.json`.
- Closure change log: `docs/changes/2026-07-31-m29-namespace-posture.md`
- ADRs: 0045 (namespace governance and capacity posture)
- Joins the existing typed ResourceQuota, LimitRange, Workload (5 kinds), Pod,
  PDB and Node reads into a deterministic, source-cited Namespace posture view.
  Every section carries its own EvidenceCitation so RBAC denials, partial
  failures and list truncation are honest. Strictly read-only; 7 categories of
  inference (quota ratios, LR conflicts, PDB coverage, node-share, NetPol
  reachability, ownerRef expansion, scheduler semantics) are explicitly
  refused and documented in ADR 0045 §6.

## M30: Controlled Node Maintenance

- Status: ✅ Completed on 2026-07-30. Verified via `go test ./...` (all 25 backend
  packages pass, including 40+ new `maintenance` tests covering request
  validation, control-plane rejection, cordon/uncordon/drain preconditions,
  unmanaged/emptyDir/PDB-unavailable blocker classes, dry-run patch, token
  hashing, idempotent claim, stale target on UID/Pod-set change, cordon/uncordon
  success, drain success, partial drain with Node-remains-cordoned, unknown
  action, list delegation, Pod classification for all 6 paths, evidence matching,
  block-error classification, node-patch and eviction-body builders, identity
  generation, JSONB round-trips, and isControlPlane label coverage), `vue-tsc -b`
  (zero frontend type errors after removing unused `MaintenancePodEvidence`
  import), `vitest run` (73 frontend tests, 17 files), and
  `scripts/verify-fast.ps1 -Scope All` (passed in 35.01s, backend=True
  frontend=True manifests=True). Disposable two-worker real-kind E2E passed
  safe cordon/drain/uncordon, replay and blocker behavior; evidence is
  `.artifacts/m30-node-maintenance-kind/summary.json`.
- Closure change log: `docs/changes/2026-07-30-m30-controlled-node-maintenance.md`
- ADRs: 0046 (controlled node maintenance)
- Adds single-worker cordon, uncordon and bounded PDB-aware eviction through
  preview, confirmation, preconditions, idempotency and audit. The
  `KubernetesSource` interface bounds the mutation surface to Node patch and
  Eviction create; force deletion, PDB bypass, `emptyDir` deletion, arbitrary
  Pod delete, browser terminals, auto-uncordon after failed drain, and bulk
  multi-node selection are explicitly prohibited and documented in ADR 0046 §7.

## M31: Isolated Workload Restore Rehearsal

- Status: ✅ Completed on 2026-07-30. Verified via `go test ./...` (all 26 backend
  packages pass, including 40+ new `restore` tests covering request validation,
  Velero capability preflight, source backup not-found/incomplete/scope,
  destination exists/collision, restore name conflict, quarantine and restore
  dry-run failures, confirmation token hashing, idempotent claim, stale source
  on phase change, namespace/quarantine-controls/restore creation failures,
  poll timeout, partial restore, failed phase, success projection, list
  delegation, namespace/restore name generation, DNS1123 sanitization, UID
  extraction, identity generation, JSONB round-trips, allowlist/excludelist
  contracts, and response projection), `vue-tsc -b` (zero frontend type
  errors), `vitest run` (73 frontend tests, 17 files), and
  `scripts/verify-fast.ps1 -Scope All` and final gates. Pinned Velero v1.15.2 +
  disposable MinIO E2E passed quarantine, mapping, replay and cleanup; evidence
  is `.artifacts/m31-isolated-restore-kind/summary.json`.
- Closure change log: `docs/changes/2026-07-30-m31-isolated-workload-restore-rehearsal.md`
- ADRs: 0047 (isolated workload restore rehearsal)
- Rehearsable restore of one M28-compatible Velero Backup into a
  server-generated quarantine Namespace with default-deny NetworkPolicy and
  zero-Pod ResourceQuota, fixed resource allowlist, two-phase confirmation,
  idempotent execution, and bounded restored-item projection. The
  `KubernetesSource` interface bounds the mutation surface to `CreateResource`
  only; in-place restore, PV/PVC restore, cross-cluster restore,
  cutover/rollback, operator-supplied destination names, arbitrary
  include/exclude lists, and quarantine Namespace auto-cleanup are explicitly
  prohibited and documented in ADR 0047 §2/§5/§8.

## M32: Formal Closure And Thesis/Demo Refresh

- Status: ✅ Final local archive on 2026-07-31. Fast gate passed in 26.17s;
  full gate passed in 97.68s with evidence at
  `.artifacts/verification/verify-20260731-015255.json`. M27-M31 disposable
  suites and responsive browser checks passed; 24 migration pairs, route/
  OpenAPI parity, reviewed RBAC, script AST and cleanup checks passed. Race is
  environment-blocked because `gcc` is unavailable. Hosted CI/release,
  OIDC/MFA, PITR and HA remain external gates.
- Closure change log: `docs/changes/2026-07-30-m32-formal-closure.md`
- ADRs: 0043-0047 (M27-M31 decisions remain accepted)
- Production ready is a separate claim that additionally requires
  organization-approved OIDC/MFA, physical/WAL PITR and HA drills. Readiness
  admission documents alone never satisfy that claim.

## M26: Organization Integration And Formal Release

- Status: Gated; work that does not require provider decisions may proceed in
  parallel with the serial M27-M31 engineering route.
- Implement isolated-provider OIDC/MFA only after Phase 11 policy approval;
  readiness admission alone is not production SSO.
- Implement physical/WAL PITR and disposable HA failover/failback only after
  Phase 12 policy approval; readiness admission alone is not production
  recovery.
- Register the dedicated real-kind runner, decide registry/signing identity,
  enable required checks when the repository plan permits, rehearse a semantic
  release and recapture screenshots bound to the reviewed revision.

## Archived M26-M32 Development Plan

The route below is complete and retained only as planning history. Current post-M32 work is authoritative in
`docs/kubesphere-optimization-plan.md`.

## M33: Restricted client-go Migration

- Status: ✅ Development Complete on 2026-07-31. Fast gate passed in 85.91s
  (26 backend packages, 73 frontend tests, Compose/Kustomize contracts).
- Closure change log: `docs/changes/2026-07-31-m33-restricted-client-go-migration.md`
- ADR: [0048](adr/0048-restricted-client-go-migration.md)
- Replaced the raw `net/http` `cluster.Registry` with a `k8s.io/client-go`-backed
  `ClusterClientProvider`. The four gateway interfaces (`Prober`, `Gateway`,
  `PatchGateway`, `CreateGateway`) are unchanged; `service.go` and all call
  sites are untouched. Satisfies ADR 0004's requirement to use `client-go`.
- Real-kind E2E deferred; transport-level contract verified by unit tests
  (Probe, Patch method/body/dryRun, no-redirect invariant).

## M34: Route Descriptor Contract and RBAC Inventory

- Status: ✅ Development Complete on 2026-07-31. Fast gate passed in 26.64s
  (3 backend packages vet+test, 73 frontend tests, Compose/Kustomize contracts).
- Closure change log: `docs/changes/2026-07-31-m34-route-descriptor-and-rbac-inventory.md`
- ADR: [0049](adr/0049-route-descriptor-contract-and-rbac-inventory.md)
- **M34A — Route Descriptor Contract.** Replaced the scattered Gin route
  registration, hardcoded audit map, and inline role checks with a single
  `RouteDescriptor` source of truth. The same descriptor drives route
  registration, authentication/role enforcement, and audit classification.
  Five table-driven invariants (`TestRouteTableCoversAllGinRoutes`,
  `TestDescriptorMetadataWellFormed`, `TestDescriptorHTTPMethodsValid`,
  `TestDescriptorFullPathStartsWithAPIV1`, `TestDescriptorNoDuplicateRoutes`)
  guard the contract.
- **M34B — RBAC Inventory.** Delivered the ADR 0039-promised fixed read-only
  contract for `Role`, `ClusterRole`, `RoleBinding`, `ClusterRoleBinding`:
  bounded projections, manifest redaction extension, observer RBAC grant,
  OpenAPI documentation, eight descriptor-registered GET routes, and the
  `TestRegisteredRoutesMatchOpenAPI` bidirectional parity check.
- Real-kind E2E deferred; contract verified by the descriptor parity suite,
  OpenAPI parity test, and RBAC unit tests over the `roundTrip` harness.

## M35: Lightweight Cluster And Namespace Access Grants

- Status: ✅ Development Complete on 2026-07-31. Fast gate passed in 39.44s
  (backend, frontend and manifests all green; 21 `authz/service_test.go`
  cases, 11 `authz_middleware_test.go` cases and 13 `grants_test.go` cases
  added; OpenAPI route parity test extended for grant-management routes).
- Closure change log: `docs/changes/2026-07-31-m35-lightweight-cluster-and-namespace-access-grants.md`
- ADR: [0050](adr/0050-lightweight-cluster-and-namespace-access-grants.md)
- Introduced the platform's first *resource-scope* authorization dimension on
  top of the four global platform roles. Migration `000025_access_grants`
  creates `user_cluster_grants` and `user_namespace_grants`. A single policy
  evaluator (`authz.Service`) answers cluster access, namespace access and
  visible-cluster filtering; `requireClusterAccess` and
  `requireNamespaceAccess` middleware wire the evaluator into fleet, search
  and resource routes carrying `:cluster_id` or `:namespace` path parameters.
- Authorization failures return 404 (not 403) to avoid leaking the existence
  of hidden clusters or namespaces. SystemAdmin bypasses all grants; other
  roles see only granted scope. Fleet and global search silently omit
  unauthorized clusters from results, counts and errors.
- Real-kind E2E and frontend UI deferred; namespace query-param filtering
  outside fleet/search routes deferred. Backend contract, parity and unit
  tests are authoritative.

## M36: Production OIDC And MFA

- Status: ✅ Development Complete on 2026-07-31. All six phases (A–F)
  passed local implementation and fast gate verification. OIDC remains
  disabled by default; when disabled, no OIDC route is registered and the
  server behaves as a local-only deployment. No public API contract changed
  beyond the three new OIDC routes documented in OpenAPI.
- Closure change logs:
  - `docs/changes/2026-07-31-m36a-oidc-config-and-external-identities.md`
  - `docs/changes/2026-07-31-m36b-oidc-discovery-and-jwks-cache.md`
  - `docs/changes/2026-07-31-m36c-oidc-authorization-code-and-pkce-flow.md`
  - `docs/changes/2026-07-31-m36d-oidc-session-logout-and-breakglass.md`
  - `docs/changes/2026-07-31-m36e-synthetic-idp-end-to-end-gate.md`
  - `docs/changes/2026-07-31-m36f-oidc-http-wiring-and-gorm-resolver.md`
- ADR: [0052](adr/0052-production-oidc-and-mfa.md)
- **M36A — Configuration and external identities.** OIDC configuration with
  fail-closed validation (HTTPS issuer, PKCE S256, MFA evidence, claim
  mapping, group-to-role deduplication). Migration `000026` adds
  `external_identities` table with `(issuer, subject)` unique constraint.
  21 invalid-configuration cases rejected in unit tests.
- **M36B — Discovery and JWKS cache.** Automatic OIDC discovery document
  fetch and validation, cached JWKS with TTL-based refresh, key rotation
  (retired keys fail closed), single-flight coalescing, duplicate-kid
  rejection and too-many-keys rejection. 10 discovery + 10 JWKS contract
  tests.
- **M36C — Authorization Code + PKCE flow.** Full Authorization Code flow
  with PKCE S256, state/nonce validation, ID token verification (signature,
  issuer, audience, nonce, expiry, MFA evidence, `amr` claim), group-to-role
  mapping and browser-flow leak guard. 15 provider tests + 21 subtests
  including synthetic HTTPS IdP.
- **M36D — Session management, logout and break-glass.** Session issuance
  through `AuthSessionIssuer` adapter (shares refresh-token rotation,
  `auth_version` revocation and audit with password login), provider
  RP-initiated logout, and break-glass drill recording with staleness
  tracking. 6 session + 3 logout + 7 break-glass tests.
- **M36E — Synthetic IdP end-to-end gate.** `TestSyntheticIdPEndToEndLifecycle`
  (6 ordered subtests: Login, Authorization, Rotation, Disable, Logout,
  BreakGlass) and `TestSyntheticIdPEndToEndBreakGlassStaleness` exercising
  discovery, JWKS, PKCE, ID-token verification, MFA evidence, session
  issuance and break-glass audit through their real implementations against
  a synthetic HTTPS IdP.
- **M36F — HTTP wiring and GORM identity resolver.** `GormIdentityResolver`
  resolves (issuer, subject) to prelinked local user via
  `external_identities` JOIN `users`; automatic email linking forbidden.
  `AuthSessionIssuer` adapts `auth.Service.IssueSessionForUser`. OIDC HTTP
  handlers (`GET /login`, `GET /callback`, `POST /logout`) with fail-closed
  error mapping. `OIDC_AUTH_SESSION_SIGNING_KEY` (≥32 bytes) added to
  configuration. Server bootstrap wires provider when enabled; registers no
  OIDC route when disabled. OpenAPI bidirectional parity preserved.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 46.71s (26
  backend packages, 81 frontend tests/18 files, Compose/Kustomize contracts).
- Real organization IdP run, GORM `IdentityResolver` PostgreSQL integration
  test and frontend OIDC login button are deferred (externally gated).

## M37: Capability Plane Adapters

- Status: ✅ Development Complete on 2026-07-31. M37A (capability providers)
  and M37B (alert routing and silences) passed local implementation and fast
  gate verification. M37C (Gateway API evidence) and M37D (delivery metadata)
  are deferred per ADR 0053 §4 until M40 demonstrates concrete need. All
  adapters are disabled by default; the server runs identically to the
  current deployment when no provider is configured. No public API contract
  changed beyond the new `capability` and `alert-routes` routes documented in
  OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m37-capability-plane-adapters.md`
- ADR: [0053](adr/0053-capability-plane-adapters.md)
- **M37A — MetricsProvider and LogProvider.** New
  `backend/internal/capability` package with `MetricsProvider` and
  `LogProvider` interfaces, Prometheus and Loki adapters, and `Nop*` defaults.
  Public APIs accept fixed template/query AST fields only (service/resource,
  cluster/Namespace/Pod/container, bounded text, start/end, direction, limit);
  they never accept PromQL, LogQL or arbitrary labels. Provider endpoints and
  credentials are server-configured. 18 provider tests + 8 HTTP handler
  tests.
- **M37B — Alert routing and bounded silences.** New
  `backend/internal/alertroute` package with route priority (1..100), exact
  cluster/rule/severity match, dedupe key, group/repeat interval, HTTPS
  webhook receiver, time-bounded silences (5m..7d, reason required, permanent
  forbidden), idempotent delivery with retry and dead-letter. Migration
  `000027` adds four tables. 40 service tests + 27 HTTP handler tests.
- **Configuration.** `CapabilityConfig` and `AlertRouteConfig` in
  `backend/internal/config/config.go` with fail-closed validation (HTTPS
  endpoints, bounded timeouts, webhook URL scheme). Both disabled by default.
- **HTTP routes.** `GET /api/v1/capability/metrics` and
  `POST /api/v1/capability/logs` for M37A; `GET/POST/DELETE
  /api/v1/alert-routes/receivers[/:id]`, `GET/POST/PATCH/DELETE
  /api/v1/alert-routes[/:id]`, `GET/POST/DELETE
  /api/v1/alert-routes/silences[/:id]` and `GET /api/v1/alert-routes/deliveries`
  for M37B. SystemOpsAdmin role required for mutations; deliveries restricted
  to SystemSecurityAudit.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 51.06s (27
  backend packages, 81 frontend tests/18 files, Compose/Kustomize contracts).
- M37C (Gateway API evidence) and M37D (delivery metadata) deferred per
  ADR 0053 §4. Real Prometheus/Loki provider integration and real-kind E2E
  are deferred pending external provider access.

## M38: Engineering, Delivery And Supply-Chain Hardening

- Status: ✅ Development Complete on 2026-07-31. Fast gate passed in 36.5s
  (backend) and 2.47s (manifests); all 25 deployment-package tests green,
  including 10 Helm chart contract tests and 2 license allowlist tests.
- Closure change log: `docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md`
- ADR: [0051](adr/0051-engineering-delivery-and-supply-chain-hardening.md)
- **M38A — CI completeness.** The pull-request gate now runs
  `go test -race -p=1 -count=1 ./...`, `golangci-lint@v2.12.2` with
  `.golangci.yml`, `pnpm lint` with the ESLint flat config, a 50.0%
  coverage baseline and `oasdiff breaking --fail-on ERR`. The real-kind E2E
  workflow covers M23-M31 in addition to diagnosis, fleet, search and
  M21-history.
- **M38B — Helm chart.** Official Helm 3 chart at
  `deploy/helm/aiops-platform/` with `Chart.yaml`, `values.yaml`,
  `values.schema.json` and nine templates. The chart never renders a Secret;
  ten Go contract tests guard its structure, values, schema and security
  baseline.
- **M38C — Supply chain.** Releases build `linux/amd64` + `linux/arm64`
  OCI images with `docker buildx`/QEMU, generate SPDX SBOMs with
  `syft v1.27.0`, and bundle the Helm chart, license allowlist and SHA256
  manifest. The license allowlist (`docs/security/license-allowlist.json`)
  admits `MIT`/`ISC`/`BSD-2-Clause`/`BSD-3-Clause`/`Apache-2.0` only.
  `SECURITY.md` and `CHANGELOG.md` are tracked delivery assets.
- Cosign image signing, `helm lint` in CI and real-kind E2E for the Helm
  upgrade/rollback matrix are deferred pending authorized hosted CI.

## M39: Unified Service Identity and Signal Model

- Status: ✅ Development Complete on 2026-07-31. M39 normalizes existing M21-M31
  outputs into a unified `signal_occurrences` table with fingerprint-based
  deduplication. The native M21-M31 signal path is unchanged; M39 is an evidence
  normalizer, not a second alert/diagnosis/workflow system. Native signals work
  even when every optional M37 provider is disabled. No public API contract
  changed beyond the new `aiops` routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m39-unified-signal-model.md`
- ADR: [0054](adr/0054-unified-service-identity-and-signal-model.md)
- **Signal model.** New `backend/internal/signal` package with `Occurrence`
  envelope, `SignalDescriptor` catalog (28 signal codes across 7 domains),
  `ResourceCitation` (cluster_id + kind + UID primary key; name-only marked
  incomplete), `EvidenceRef` (stable, redacted evidence pointers).
- **Fingerprint dedup.** SHA256 over identity fields (signal_id + cluster_id +
  resource kind/namespace/name/uid + window_start + window_end), excluding
  ObservedAt. Database unique index + ON CONFLICT DO UPDATE ensures duplicate
  producer delivery yields one row.
- **Normalizers.** DiagnosisNormalizer (11 rules), AlertNormalizer (firing/
  resolved), MetricBreachNormalizer (sustained window), PostureNormalizer (4
  finding codes), ChangeNormalizer (promotion/backup/maintenance/restore ×
  succeeded/failed). Pure functions, no DB/K8s access.
- **Service.** `Service.Ingest` (fail-closed for unregistered signals),
  `IngestBatch`, `List` (bounded), `Overview` (source completeness + top
  signals + recent changes + action outcomes), `CleanupRetention` (TTL-bound).
  `SourceReader` interface for overview aggregation; `NopSourceReader` default.
- **Migration 000028.** `signal_occurrences` table with unique fingerprint
  index, query indexes, CHECK constraints. Paired down migration.
- **HTTP routes.** `GET /api/v1/aiops/overview`,
  `GET /api/v1/aiops/signals`, `GET /api/v1/aiops/signals/catalog`.
  All require authentication; M35 scope filtering applied by middleware.
- **Configuration.** `SignalConfig` in `config.go` (Enabled, RetentionBatch,
  ListLimit, OverviewTopN, OverviewWindow, CleanupInterval). Disabled by
  default.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 59.59s (28 backend
  packages including `signal`, 81 frontend tests/18 files, Compose/Kustomize
  contracts).
- Deferred: concrete `SourceReader` adapter (M40), batch ingestion worker,
  real PostgreSQL integration test, frontend UI.

## M40: Temporal Topology And Change Intelligence

- Status: ✅ Development Complete on 2026-07-31. M40 introduces a persisted
  temporal topology graph (8 reviewed edge kinds with validity intervals) and
  a unified change timeline that normalizes M23-M31 platform-operation
  outcomes. The native M21-M31 signal path is unchanged; M40 is an
  evidence-graph and change-timeline normalizer. No public API contract
  changed beyond the two new `aiops/topology` routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m40-temporal-topology-and-change-intelligence.md`
- ADR: [0055](adr/0055-temporal-topology-and-change-intelligence.md)
- **Edge model.** New `backend/internal/topology` package with 8 `EdgeKind`
  values (Owns/Selects/RoutesTo/BackedBy/RunsOn/Mounts/Scales/ProtectedBy),
  8 `DerivationMethod` values, `ResourceCitation` (cluster_id + kind + UID
  primary key; name-only marked incomplete), `Edge` with validity interval,
  `ChangeEvent` with confidence/source.
- **Collector.** `Collector.Snapshot` reads 8 resource types with bounded
  paging (1000-page safety cap); `DeriveEdges` deterministically derives all
  edge kinds from exact observed evidence (OwnerReference, label selector,
  EndpointSlice, Ingress backend, nodeName, PVC mount, HPA scaleTargetRef,
  PDB selector). Same-name/temporal proximity never creates an edge.
- **Repository.** `GormRepository` with `ON CONFLICT DO UPDATE` for edge
  refresh and change-event idempotency. Partial unique index
  `uq_topology_edges_active` enforces at-most-one-active-edge. `NopRepository`
  for disabled/testing mode.
- **Service.** `CollectNamespace` (snapshot → derive → upsert → close stale),
  `CollectCluster` (iterate visible namespaces), `GetTopologyGraph` (nodes
  from edge endpoints, completeness indicator), `GetChangeTimeline`,
  `IngestChangeEvent` (validated persistence).
- **Change normalizer.** Pure mapping function from `ChangePlanInput`/
  `AuditChangeInput` to `ChangeEvent`. Domain statuses normalized:
  succeeded/failed/expired/partial/awaiting_confirmation/executing →
  succeeded/failed/failed/partial/pending/pending. Confidence high for
  platform+audit_id, low otherwise.
- **Migration 000029.** `topology_edges` table with partial unique active
  index, query indexes, CHECK constraints; `change_events` table with
  idempotent plan_id index, CHECK constraints. Paired down migration.
- **HTTP routes.** `GET /api/v1/aiops/topology/graph` and
  `GET /api/v1/aiops/topology/changes`. Read-only; require authentication;
  M35 scope filtering by middleware. Bounded limits (graph 500, timeline 200)
  with truncation disclosed.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 73.46s (29 backend
  packages including `topology`, 81 frontend tests/18 files, Compose/Kustomize
  contracts).
- Deferred: background collection worker, plan-completion ingestion hook,
  real PostgreSQL integration test, real-kind E2E, frontend UI, retention
  worker.

## M41: SLO, Error Budget And Impact

- Status: ✅ Development Complete on 2026-07-31. M41 introduces server-owned
  SLI templates, versioned SLO definitions, deterministic evaluation with
  explicit missing-data handling, and burn-alert transitions that feed the
  existing M27 alert lifecycle. The native M21-M31 signal path is unchanged;
  M41 is a deterministic evaluator that reads from the M37 capability
  providers. No public API contract changed beyond the eight new
  `aiops/slos` routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m41-slo-error-budget-and-impact.md`
- ADR: [0056](adr/0056-slo-error-budget-and-impact.md)
- **Data model.** New `backend/internal/slo` package with 3 `SLITemplate`
  values (`request_success_ratio`, `request_latency_target_ratio`,
  `workload_readiness`), 2 `MissingDataPolicy` values (`unavailable`,
  `fail_open`), 5 `EvaluationState` values (`healthy`, `burning_slow`,
  `burning_fast`, `breached`, `unavailable`), 3 `EvaluationCoverage` values
  (`complete`, `partial`, `unavailable`), `Definition` (versioned, enabled,
  bounded burn windows), `Evaluation` (append-only, deterministic).
- **Catalog.** `TemplateDescriptor` + compiled `catalog` map is the single
  source of truth for which templates exist, what they require and which
  missing-data policies they admit. `ValidateDefinition` is the only
  validation entry point. Adding a template is a contract change.
- **Evaluator.** `Evaluator.Evaluate` is pure: same Definition + same
  MetricsSource output → same Evaluation. Counter resets detected as
  monotonicity violations and handled as "counter went to 0". Sparse data
  → `CoveragePartial`; no samples → `CoverageUnavailable`. Clock boundaries
  inclusive `window_start`, exclusive `window_end`. Missing data fail-closed
  by default; only `workload_readiness` may fail-open with explicit operator
  opt-in, and even then `Coverage` remains `Unavailable` (auditable).
  `classifyState` precedence: breached > burning_fast > burning_slow >
  healthy. Zero error budget (objective == 1.0) handled explicitly.
- **Repository.** `GormRepository` with `ON CONFLICT DO NOTHING` for
  idempotent evaluation inserts, partial unique index
  `uq_slo_definitions_active` for at-most-one-active-definition.
  `NopRepository` for testing/disabled mode.
- **Service.** `CreateDefinition` stamps version=1. `PatchDefinition`
  requires actor and increments Version. `DeleteDefinition` marks
  `enabled=false` (row retained). `EvaluateSLO` looks up definition first
  (404 > 503 precedence), runs evaluator, persists even on unavailable
  (auditable fact), emits `BurnTransition` to sink only on state change.
  Sink is best-effort: failure does not rollback.
- **Migration 000030.** `slo_definitions` table with CHECK constraints on
  template/policy/objective/window/burn bounds, partial unique active index,
  query indexes; `slo_evaluations` table with CHECK constraints on
  state/coverage/window/event-count/ratio bounds, query indexes. Paired
  down migration.
- **HTTP routes.** 8 routes under `/api/v1/aiops/slos`: `GET /templates`,
  `GET /`, `POST /` (SystemOpsAdmin), `GET /:id`, `PATCH /:id`
  (SystemOpsAdmin), `DELETE /:id` (SystemOpsAdmin),
  `POST /:id/evaluate` (SystemOpsAdmin), `GET /:id/evaluations`. Read-only
  routes open to any authenticated user; M35 scope enforced via cluster_id
  binding at create time and middleware on underlying Kubernetes resources.
- **OpenAPI.** Adds `slo` tag, 8 paths, 10 schemas
  (`SLITemplateCatalog`, `SLITemplateDescriptor`, `SLOServiceRef`,
  `SLOActorRef`, `SLODefinition`, `SLODefinitionCreate`,
  `SLODefinitionPatch`, `SLODefinitionList`, `SLOEvaluation`,
  `SLOEvaluationList`). Enums, bounds and required fields match migration
  CHECK constraints and `ValidateDefinition` rules.
- **Tests.** 31 backend tests in `internal/slo` (14 evaluator + 13 service +
  4 catalog) and 11 backend tests in `internal/httpserver` (handler smoke
  tests). Route contract test verifies bidirectional OpenAPI consistency.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 67.02s (30 backend
  packages including `slo`, 81 frontend tests/18 files, Compose/Kustomize
  contracts).
- Deferred: real Prometheus/Loki integration tests, real-kind E2E for
  burn-transition-to-M27, frontend SLO management UI, background evaluation
  worker, multi-window burn rate, production wiring in `cmd/server/main.go`.

## M42: Multi-Signal Correlation And Deterministic RCA

- Status: ✅ Development Complete on 2026-07-31. M42 introduces a
  deterministic, replayable multi-signal correlation engine that links M39
  signal occurrences, M40 topology edges/change events and existing diagnosis
  records into bounded cases. The diagnosis record remains the human
  status/SLA/feedback source of truth; correlation cases are candidates, not
  incidents. No public API contract changed beyond the six new
  `aiops/correlation` routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m42-multi-signal-correlation-and-deterministic-rca.md`
- ADR: [0057](adr/0057-multi-signal-correlation-and-deterministic-rca.md)
- **Data model.** New `backend/internal/correlation` package with 4
  `ConfidenceClass` values (`confirmed`, `candidate`, `contradicted`,
  `unknown`), 3 `CaseStatus` values (`active`, `resolved`, `stale`), 4
  `SignalRelation` values, 4 `ResourceRelation` values, `Case` (deterministic
  case_key via SHA-256), `SignalLink`, `ResourceLink`, `ChangeCandidate`,
  `ActionCandidate` (fixed codes from M19 catalog), `CorrelationResult`.
  `CorrelationVersion = "1.0"`.
- **Catalog.** `RuleDescriptor` + compiled `catalog` map with 6 V1 rules
  covering golden replay scenarios. `LookupRule`, `AllRules`,
  `RulesForTriggerSignal` fail closed for unlisted rules. Adding a rule is a
  contract change.
- **Engine.** `Engine.Correlate` is pure and stateless: identical inputs +
  identical rule/correlation versions yield identical results. Computes
  explicit factors (`same_uid`, `topology_distance`, `time_distance`,
  `change_symptom_rule`, `signal_freshness`, `signal_completeness`,
  `diagnosis_match`, `contradicting_signal`). `edgeIndex` implements
  bidirectional BFS for topology path search. `classifyConfidence` is a pure
  function over (rule, factors, contradicting refs).
- **Golden fixtures.** 9 replay scenarios (ImagePull, CrashLoop, OOM,
  PVC-Pending, NoEndpoints, ReplicasUnavailable, NodeNotReady, MetricBreach,
  BadRollout-contradicted) plus a cold-start scenario. Each fixture is a
  deterministic (inputs, expected) pair.
- **Repository.** `GormRepository` with idempotent `UpsertResult` (ON
  CONFLICT DO NOTHING), unique indexes on case_key (active),
  (case_id, signal_occurrence_id, relation), (case_id, uid, relation),
  (case_id, change_event_id). `NopRepository` for testing/disabled mode.
- **Service.** `CorrelateNamespace` gathers bounded inputs, runs the engine,
  persists results idempotently. `ListActionCandidates` derives fixed action
  candidates (`deployment.rollback`, `deployment.rollout_restart`) — no
  execute endpoint.
- **Migration 000031.** `correlation_cases`, `correlation_signal_links`,
  `correlation_resource_links`, `correlation_change_candidates` tables with
  CHECK constraints and unique indexes.
- **HTTP routes.** 6 read-only routes under
  `/api/v1/aiops/correlation`: `GET /rules`, `GET /cases`,
  `GET /cases/timeline`, `GET /cases/:id`, `GET /cases/:id/graph`,
  `GET /cases/:id/actions`. Case correlation is an internal operation, not
  HTTP-triggered.
- **OpenAPI.** Adds `correlation` tag, 6 paths, schemas for cases, timeline,
  graph, actions, rules.
- **Tests.** 36 tests total: 5 catalog + 3 fixtures (10 subtests) + 10
  service + 9 handler. Route contract test verifies bidirectional OpenAPI
  consistency.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 63.26s (31 backend
  packages including `correlation`, 81 frontend tests/18 files,
  Compose/Kustomize contracts).
- Deferred: background correlation worker, signal-ingestion hook, real
  PostgreSQL integration test, real-kind E2E, frontend UI, M43 AI
  investigator integration, M44 safe automation integration.

## M43: Cited And Evaluated AI Investigator

- Status: ✅ Development Complete on 2026-07-31. M43 introduces a cited
  and evaluated AI investigator bound to M42 correlation cases. The
  investigation is a read-only advisory: it never modifies the case,
  diagnosis or alert. Every factual claim cites an authorized evidence ID;
  fabricated, out-of-scope or unauthorized citations reject the entire
  output. The model cannot upgrade a candidate to confirmed cause, and
  cannot recommend a runbook outside the eligible M42 Action Catalog. On
  provider failure, budget exhaustion or citation rejection, a failed
  investigation is persisted with `failure_reason` set so deterministic
  investigation remains available. No public API contract changed beyond
  the four new `aiops/investigator` routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m43-cited-and-evaluated-ai-investigator.md`
- ADR: [0058](adr/0058-cited-and-evaluated-ai-investigator.md)
- **Data model.** New `backend/internal/aiinvestigator` package with
  `InvestigatorVersion = "1.0"`, 3 `InvestigationStatus` values
  (`completed`, `failed`, `stale`), 3 `HypothesisConfidence` values
  (`high`, `medium`, `low`), 7 `EvidenceKind` values, `Investigation`
  (deterministic `investigation_key` via SHA-256 over case_id +
  investigator_version + prompt_hash), `Hypothesis`, `Citation`,
  `EvidenceRef`, `Prompt`, `ProviderResult`. Bound constants
  (`MaxHypothesesPerInvestigation` = 8, `MaxCitationsPerInvestigation` =
  64, etc.).
- **Runbook catalog.** `RunbookDescriptor` + compiled `catalog` map with
  4 V1 runbooks: `rollback_last_rollout` (`deployment.rollback`),
  `rollout_restart_pods` (`deployment.rollout_restart`),
  `inspect_pvc_capacity` (advisory), `inspect_node_maintenance`
  (advisory). `LookupRunbook` fails closed; advisory runbooks are always
  eligible; adding a runbook is a contract change.
- **Prompt + validator.** `BuildPrompt` assembles the system prompt
  (role, output schema, citation rules, runbook rules, prohibitions,
  prompt-injection defense) and the user prompt (redacted authorized
  evidence only — no raw logs/events/manifests). `ValidateProviderResult`
  enforces 8 rules: non-empty summary/impact, 1..8 hypotheses, authorized
  citations, authorized hypothesis/disconfirming evidence, 1..64
  citations, eligible runbook, no "confirm root cause" claims, bounded
  next_checks/uncertainties. Rejection is total — the entire output is
  discarded.
- **Golden fixtures.** 10 validation scenarios (correct, insufficient,
  conflicting, prompt-injection, hidden-scope, fabricated-citation,
  ineligible-runbook, confirm-root-claim, empty-summary, no-citations).
  Each fixture is a deterministic (provider result, authorized evidence,
  eligible codes, expected valid/invalid + failure substring) pair.
- **Repository.** `GormRepository` with `Save` (insert + `MarkStale`),
  `Get`, `ListByCase`, `ListByFilter`, `MarkStale`. Partial unique index
  `uq_ai_investigations_active` on `(case_id, investigation_key) WHERE
  status != 'stale'`. `NopRepository` for testing/disabled mode.
- **Service.** `Investigate` reads the case + eligible action codes,
  builds the prompt, calls the provider, validates, and persists
  (completed or failed). `GetInvestigation`, `ListByCase`, `ListRunbooks`.
  On provider failure → `failed`/`provider_error`; on validation failure
  → `failed`/`citation_rejected` (provider summary retained for audit).
- **Migration 000032.** `ai_investigations` table with CHECK constraints
  on status/tokens, completed-summary/completed-citations/failed-reason
  invariants, and a FK to `correlation_cases(id)` ON DELETE CASCADE.
- **HTTP routes.** 4 routes under `/api/v1/aiops/investigator`:
  `GET /runbooks`, `GET /cases/:case_id/investigations`,
  `GET /investigations/:id`, `POST /cases/:case_id/investigations`. The
  POST is the only write; it persists an investigation but never modifies
  the case. Actor derived from the authenticated session.
- **OpenAPI.** Adds `aiinvestigator` tag, 4 paths, 8 schemas
  (InvestigatorRunbookList, InvestigatorRunbook, InvestigationListResponse,
  Investigation, InvestigationActor, InvestigationHypothesis,
  InvestigationCitation, EvidenceRef).
- **Tests.** 44 tests total: 5 catalog + 4 provider/fixtures (18 subtests)
  + 8 prompt + 15 service + 12 handler. Route contract test verifies
  bidirectional OpenAPI consistency.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 37.47s (31
  backend packages including `aiinvestigator`, 81 frontend tests/18 files,
  Compose/Kustomize contracts).
- Deferred: real AI provider integration (Responses-compatible HTTP
  provider), provider budget/reservation enforcement, real PostgreSQL
  integration test, real-kind E2E, frontend UI, M44 safe-automation
  wiring.

## M44: Policy-Constrained Automation And Post-Action Verification

- Status: ✅ Development Complete on 2026-07-31. M44 closes the AIOps
  loop: an eligible M43 runbook is materialized into an action plan,
  gated through deterministic policy checks, approved by a human (L2
  default; four-eyes for rollback/image_update), executed idempotently
  against the Kubernetes source, and verified against captured
  pre/post evidence. The action plan lifecycle is `draft → previewed →
  approved → executing → succeeded/failed → verified` (plus terminal
  `expired`/`cancelled`). Every transition is a repository method that
  re-checks state under a row lock and stamps audit metadata. Policy
  gates are rechecked immediately before execute — a stale target
  UID/RV, an opened freeze window, an exhausted PDB budget, or an
  exceeded attempt cap all fail closed. Post-action verification
  compares SLO and resource state deterministically; missing evidence
  never resolves a diagnosis automatically (yields `unknown`). When
  verification yields ineffective/failed, a server-owned rollback
  contract drafts a rollback plan when safe, or escalates to a human.
  No public API contract changed beyond the ten new `aiops/automation`
  routes documented in OpenAPI.
- Closure change log: `docs/changes/2026-07-31-m44-policy-constrained-automation-and-post-action-verification.md`
- ADR: [0059](adr/0059-policy-constrained-automation-and-post-action-verification.md)
- **Data model.** New `backend/internal/automation` package with
  `AutomationVersion = "1.0"`, `VerifierVersion = "1.0"`, 4
  `AutomationLevel` values (L0/L1/L2/L3; L2 is the default, L3 not
  enabled), 9 `PlanStatus` values, 2 `ApprovalType` values (single,
  four_eyes), 3 `GateStatus` values, 8 `GateCode` values
  (uid_rv_recheck/scope/pdb_blast_radius/slo_burn/freeze_window/
  concurrent_plans/attempt_cap/rollback_point), `ActionPlan`
  (deterministic `plan_key` via SHA-256 over case_id + runbook_id +
  target_uid + automation_version), `ActionVerification`
  (deterministic `verification_key` via SHA-256 over plan_id +
  verifier_version + evidence_hash), `EvidenceSnapshot`, `SLOSnapshot`.
  Bound constants (`MaxAttemptsPerTarget` = 5, `AttemptWindowSeconds`
  = 3600, `DefaultPlanTTLSeconds` = 600, `DefaultClaimTTLSeconds` =
  60, `DefaultCooldownSeconds` = 300, `MinCooldownSeconds` = 60).
- **Policy gate evaluator.** `GateEvaluator` is stateless and pure.
  `RequiredGates(actionCode)` returns the action-specific gate set
  (core: uid_rv_recheck/scope/freeze_window/concurrent_plans/
  attempt_cap; Pod-affecting add pdb_blast_radius; SLO-bound add
  slo_burn; rollback adds rollback_point). `Evaluate` runs at preview;
  `Recheck` runs at execute with `Rechecked = true` and fresh
  `GateContext`. `AllPassed` treats `skipped` as non-failure. Adding
  a gate is a contract change (AutomationVersion bump).
- **Confirmation + idempotency.** `Preview` issues a 32-byte
  confirmation token (base64; SHA-256 hashed at rest). `Execute`
  requires the plaintext token plus an operator-supplied idempotency
  key (UUID). `Claim` atomically transitions `approved → executing`
  under a row lock and stamps the key; replay returns the recorded
  outcome; re-execute with a different key after a terminal status
  yields `ErrAlreadyExecuted`. Stale `executing` rows past `claimTTL`
  are reclaimable.
- **Human approval.** `approvalTypeFor(actionCode)` returns `four_eyes`
  for rollback and image_update, `single` otherwise. Four-eyes requires
  `approver_user_id != requested_by_user_id`; enforced at the DB layer
  (CHECK constraint) and re-checked by the service. Self-approval of a
  four-eyes plan yields `ErrSelfApprovalForbidden` (403).
- **Post-action verifier.** `Verifier` is pure given (plan, pre, post).
  `CapturePreSnapshot` at execute time; `CapturePostSnapshot` after
  cooldown. `compareEvidence` is deterministic: SLO state transitions
  take precedence (healthy > burning_slow > burning_fast > breached);
  resource state (replicas/available_replicas/image/suspended) is
  compared for actions without SLO evidence or when SLO state is
  unchanged. Missing evidence yields `ComparisonInsufficient` and
  `VerificationStatusUnknown`. `classifyStatus` maps to
  effective/ineffective/failed/unknown.
- **Server-owned rollback contract.** When verification yields
  ineffective/failed, `evaluateRollbackContract` checks target UID
  unchanged, no freeze, no concurrent plan, attempt cap not exceeded.
  If safe, a rollback plan is drafted automatically (status `draft`,
  `rollback_of_plan_id` set). If unsafe, verification records
  `reason = "unsafe_rollback_escalated_to_human"`. M44 never
  auto-executes rollback plans — they require the same preview →
  approve → execute path.
- **Repository.** `GormRepository` with `SavePlan`/`GetPlan`/
  `GetPlanForExecute` (row lock)/`ListPlans`/`CountAttemptsSince`/
  `CountConcurrentPlans`/`MarkPreviewed`/`Approve`/`Claim`/`Complete`/
  `Fail`/`MarkVerified`/`Cancel`/`ExpireStale`/`SaveVerification`/
  `GetVerification`/`GetVerificationByPlan`/`UpdateVerification`.
  `NopRepository` for testing/disabled mode. 7 lifecycle sentinel
  errors.
- **Migration 000033.** `action_plans` and `action_verifications`
  tables with CHECK constraints on status/approval_type/
  evidence_comparison/verification_status, the four-eyes distinctness
  CHECK, the missing-evidence → insufficient+unknown CHECK, partial
  unique indexes `uq_action_plans_active` (one non-terminal plan per
  `plan_key`) and `uq_action_verifications_active` (one pending
  verification per plan), FKs to `correlation_cases(id)` and
  `ai_investigations(id)` ON DELETE SET NULL.
- **HTTP routes.** 10 routes under `/api/v1/aiops/automation`:
  `GET /runbooks`, `GET /plans`, `POST /plans`, `GET /plans/:plan_id`,
  `POST /plans/:plan_id/preview`, `POST /plans/:plan_id/approve`,
  `POST /plans/:plan_id/execute`, `POST /plans/:plan_id/cancel`,
  `POST /plans/:plan_id/verify`,
  `GET /plans/:plan_id/verification`. Write routes require
  `rolesSystemOpsAdmin`; read routes require only authentication.
  Actor derived from the authenticated session. Idempotency-Key
  header read by execute.
- **OpenAPI.** Adds `automation` tag, 9 paths, 9 schemas
  (AutomationRunbookList, AutomationRunbook, CreateActionPlanRequest,
  ApproveActionPlanRequest, ExecuteActionPlanRequest,
  ActionPlanListResponse, ActionPlanResponse, ActionVerification,
  PolicyGate).
- **Tests.** 66 tests total: 11 gates + 17 verifier + 17 service + 21
  handler. Route contract test verifies bidirectional OpenAPI
  consistency.
- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 67.17s (30
  backend packages including `automation`, 81 frontend tests/18 files,
  Compose/Kustomize contracts).
- Deferred: background verification worker (cooldown-based
  scheduling), stale `executing` reclaim worker, `ExpireStale`
  background worker, real Kubernetes integration tests for the patch
  path, real Prometheus/SLO integration for the `EvidenceProvider`,
  real PostgreSQL integration test, real-kind E2E, frontend UI, `L3`
  pre-authorized automatic execution level, rollback-plan
  auto-execution path, M42 `ActionCandidate` → M44 plan auto-suggestion.

## M45: Versioned AIOps Golden Dataset And Quality Report

- Status: ✅ Development-side deliverable complete on 2026-07-31. M45
  introduces the versioned AIOps golden dataset and quality report as
  the replayable contract for the full AIOps loop (M39-M44). The dataset
  contains 3 scenarios: the mandatory 10-step end-to-end golden scenario
  (healthy service → bad image → signals → impact graph → cause
  candidate → AI investigation → preview/approve rollback → execute/
  verify → recover alert → cleanup) plus 2 negative companions
  (misattribution prevention, partial/unknown fail-closed). The quality
  report structure records before/after comparison per scenario,
  aggregated summary metrics, changed components, and human review state.
  It is JSON-serializable so CI can diff before/after and block
  regressions. M45 production gates (hosted CI, production OIDC/MFA, HA
  PostgreSQL, signed releases, real-kind E2E) remain external.
- Closure change log: `docs/changes/2026-07-31-m45-versioned-aiops-golden-dataset-and-quality-report.md`
- ADR: [0060](adr/0060-versioned-aiops-golden-dataset-and-quality-report.md)
- **Golden dataset.** New `backend/internal/golden` package with
  `DatasetVersion = "1.0"`, `ScenarioVersion = "1.0"`, 10 `StepID`
  constants, `AllSteps` ordered list, 3 `ScenarioID` constants,
  `StepOutcome` (with expected signal/topology/SLO/correlation/
  investigation/action plan/verification/alert recovery flags),
  `Scenario`, `Dataset`, `DefaultDataset()`, and 3 scenario constructors.
- **Mandatory 10-step scenario.** Maps each step of the AIOps loop to
  an expected outcome: establish_healthy_service (M41), publish_bad_image
  (M23), capture_signals (M39, M41), build_impact_graph (M40),
  rank_cause_candidate (M42), generate_investigation (M43),
  preview_approve_rollback (M44), execute_verify (M44), recover_alert
  (M27), cleanup.
- **Negative companions.** `negative_misattribution`: unrelated change
  in another Namespace must NOT be attributed to the primary case (no
  action plan expected). `negative_partial_evidence`: when one provider
  is stopped, the case must be partial/unknown, not falsely healthy
  (valid advisory investigation expected, but no alert recovery).
- **Quality report.** `QualityReport` with before/after dataset versions,
  engine versions (M39-M44), per-scenario `ScenarioQuality`
  (passed_before/after, delta, steps_passed_before/after, notes),
  `QualitySummary` aggregation, `ClassifyDelta` (preserved/improved/
  regressed/unchanged), `Summarize`. Machine-readable, JSON-serializable.
  Generated offline; never self-modifies rules, prompts or policy online.
- **Tests.** 9 tests: dataset version, integrity, mandatory step
  coverage, negative misattribution invariants, negative partial
  evidence invariants, determinism, ClassifyDelta, Summarize, quality
  report end-to-end.
- **Production gates (external, deferred).** Hosted CI with Linux race
  detector and full real-kind matrix, production OIDC/MFA and break-glass
  evidence, multi-replica deployment with PDB/topology spread/rolling-
  update evidence, external HA PostgreSQL with WAL/PITR and measured
  RPO/RTO, multi-instance no-duplicate-business-effect evidence, signed
  multi-arch release with SBOM/provenance/support matrix, real-kind E2E
  for the full 10-step scenario, real Prometheus/Loki/AI-provider replay
  in CI, frontend quality dashboard, CI integration that generates the
  quality report on every PR.

## M46: Workspace Multi-Tenancy

- Status: ✅ Development-side deliverable complete on 2026-07-31. M46
  introduces the lightweight KubeSphere-style Workspace multi-tenancy layer
  as an aggregation dimension that groups cluster namespaces across the
  fleet for UI grouping, quota display and cross-cluster namespace
  attribution. The workspace layer carries its own three-role model
  (`workspace_admin` / `workspace_editor` / `workspace_viewer`) that is
  independent of the four platform roles. The defining invariant:
  WorkspaceGrant does NOT grant namespace read access — the 2D authorization
  matrix from M35 (ADR 0050) is unchanged; WorkspaceGrant is a third,
  orthogonal grant type that only authorizes workspace metadata / membership
  / quota / role-binding edits. Anti-leakage (404 > 403) is preserved.
  SystemAdmin bypasses all workspace grant checks. Workspace creation and
  deletion are SystemAdmin-only. The owner is always `workspace_admin` and
  cannot be downgraded or revoked while the workspace exists. The audit
  trail is append-only. M46 production gates (hosted CI, production
  OIDC/MFA, HA PostgreSQL, signed releases, real-kind E2E) remain external.
- Closure change log: `docs/changes/2026-07-31-m46-workspace-multi-tenancy.md`
- ADR: [0061](adr/0061-workspace-multi-tenancy.md)
- **Data model.** Migration `000034_workspaces_and_grants` adds 5 tables:
  `workspaces`, `workspace_memberships`, `workspace_quotas`,
  `user_workspace_grants`, `workspace_role_bindings_audit`. CHECK
  constraints enforce the three fixed roles, three audit actions, DNS-
  subdomain workspace names and DNS-1123 namespace labels. Unique
  constraints enforce one workspace per name and one workspace per
  (cluster_id, namespace) tuple.
- **Service layer.** `backend/internal/workspace/service.go` enforces all
  authorization invariants: SystemAdmin bypass, 404 > 403 anti-leakage,
  owner role fixed, append-only audit, role validation, quota validation.
  The owner grant is seeded atomically on workspace creation.
- **HTTP API.** 14 routes under `/api/v1/workspaces` covering workspace
  CRUD, memberships, quota, role bindings and audit trail. Authorization is
  enforced inside the service layer; the handler only parses inputs and
  maps errors.
- **Tests.** 39 tests: 29 service-level (create/get/list/update/delete,
  membership, quota, role bindings, anti-leakage, role hierarchy, owner
  protection, metadata normalization) + 10 handler-level (HTTP status
  codes, anti-leakage at HTTP layer, path/query validation).

Detailed execution requirements, phase boundaries, verification commands and
Agent handoff rules are authoritative in `docs/next-development-plan.md`.
The cross-project evidence boundary, adopted/rejected gaps and formal project-end
criteria are authoritative in `docs/references/final-product-gap-analysis.md`.

1. **M26A — Hosted release closure (P0).** Confirm the pushed baseline's
   regular CI, preserve redacted workflow evidence, register the dedicated
   Windows real-kind runner, and rehearse a signed semantic release without
   weakening the current branch-plan limitation.
2. **M26B — Organization readiness decisions (P0, externally gated).** Obtain
   named approvals and provider inputs for OIDC/MFA and PITR/HA. Keep the
   existing readiness gates fail-closed; do not claim production SSO or
   recovery before those approvals and physical drills exist.
3. **M27 — Historical alert lifecycle (P1).** Build a bounded background
   evaluator, deduplication and acknowledgement lifecycle over the accepted
   M21 exact-series contract. Preserve deterministic diagnosis as the source
   of truth and avoid arbitrary PromQL or unbounded label cardinality.
4. **M28 — Controlled backup creation (P1).** Add Velero Backup creation only
   through fixed scope, server-side preflight, one-time confirmation,
   idempotency, audit and disposable object-storage evidence. Restore remains
   disabled until a separate conflict/PV/cutover/rollback design is approved.
5. **M29 — Namespace governance and capacity posture (P1).** Join the existing
   typed ResourceQuota, LimitRange, workload, Pod, PDB and Node reads into one
   deterministic, source-cited Namespace posture. Keep it read-only and report
   partial/truncated evidence explicitly; do not infer complete NetworkPolicy or
   scheduler semantics.
6. **M30 — Controlled Node maintenance (P1).** Add single-worker cordon,
   uncordon and bounded PDB-aware eviction through preview, confirmation,
   preconditions, idempotency and audit. Force deletion, PDB bypass, `emptyDir`
   deletion, arbitrary Pod delete and browser terminals remain prohibited.
7. **M31 — Isolated workload restore rehearsal (P1).** Restore one eligible M28
   Backup into a server-generated quarantine Namespace with PV, overwrite,
   cross-cluster and cutover paths disabled. Prove the workflow against a real
   Velero controller and disposable object store.
8. **M32 — Formal closure and demo refresh (P2).** Bind final gates,
   release metadata, architecture/test matrices and sanitized screenshots to one
   reviewed revision. Record every M26 organization gate as completed, deferred
   with a re-entry condition, or not applicable; do not claim production
   readiness from admission documents alone.

The committed development route ends at M32. Generic Kubernetes CRUD, arbitrary
resource migration, unrestricted exec/file transfer, force drain, in-place
restore, production cutover, workspace multi-tenancy, Service Mesh and a generic
DevOps platform are explicit non-goals rather than hidden post-M32 backlog.
