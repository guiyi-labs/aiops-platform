# Product Optimization Roadmap

- Updated: 2026-07-30
- Baseline: M21-M25 are accepted as one aligned local baseline. The fresh
  full gate is `.artifacts/verification/verify-20260730-080851.json`; fresh
  M21, M24 and M25 real-kind evidence plus the accepted M23 lifecycle run are
  recorded in `docs/changes/2026-07-30-m21-m25-baseline-alignment.md`.
  M20 Phase 12 and the earlier hosted M21 verification remain archived.
- Principle: close high-frequency operator workflows with fixed, evidence-based contracts; do not chase generic Kubernetes CRUD parity
- Governance: new milestones must satisfy the prioritization, Definition of
  Ready, risk-based verification, Definition of Done and evidence rules in
  `docs/project-vision-and-delivery-standards.md`.

## Baseline Closure Audit (2026-07-30)

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

## M26: Organization Integration And Formal Release

- Status: Gated; work that does not require provider decisions may proceed in
  parallel with M21-M25.
- Implement isolated-provider OIDC/MFA only after Phase 11 policy approval;
  readiness admission alone is not production SSO.
- Implement physical/WAL PITR and disposable HA failover/failback only after
  Phase 12 policy approval; readiness admission alone is not production
  recovery.
- Register the dedicated real-kind runner, decide registry/signing identity,
  enable required checks when the repository plan permits, rehearse a semantic
  release and recapture screenshots bound to the reviewed revision.

## Post-Baseline Development Plan

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
5. **M29 — Release-bound thesis/demo refresh (P2).** Re-capture screenshots,
   architecture/test matrices and demo evidence against one tagged revision;
   remove stale milestone counts and keep generated evidence out of Git.
