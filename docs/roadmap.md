# Product Optimization Roadmap

- Updated: 2026-07-28
- Baseline: M21-M26 reprioritization accepted at `5cfbf694d52bc114ff8ee567525a290d4b85e4b0`; hosted CI run `30351531959`; M20 Phase 12 remains accepted and archived; private remote remains active
- Principle: close high-frequency operator workflows with fixed, evidence-based contracts; do not chase generic Kubernetes CRUD parity

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

- Status: In progress. Phase 1 accepted locally on 2026-07-28; hosted CI is
  pending the implementation commit.
- Phase 1 adds ADR 0034, migration 17 and a PostgreSQL-backed exact-series
  domain contract with seven-day default retention, 1,800-sample collection,
  24-hour query, 1,440-point and batch-cleanup caps. Missing samples remain
  missing, never zero.
- Retain Node/Pod CPU and memory samples with explicit source timestamp,
  collection result, coverage and expiry; clean expired data deterministically.
- Add trend views and deterministic sustained-window evaluation linked to the
  existing event, diagnosis and controlled-operation records.
- Validate sampling, retention, gaps, restart recovery and query isolation
  against real kind. Do not build a Prometheus replacement or allow unbounded
  label cardinality.

## M22: Daily Troubleshooting And Governance Workbench

- Status: Planned after M21.
- Make Pod logs explicitly container-aware and add bounded tail/since,
  timestamp, current/previous, search and download controls with truncation
  disclosed to the user.
- Add fixed read-only contracts for PersistentVolume, PodDisruptionBudget,
  NetworkPolicy, ServiceAccount, Role/ClusterRole and binding metadata.
- Add redacted server-produced manifest inspection only for approved
  non-sensitive kinds. Secret/ConfigMap values, ServiceAccount tokens and
  StorageClass parameters remain excluded.
- Split the dense resource workbench into predictable inventory/detail/task
  surfaces while preserving deep links and keyboard/mobile behavior.

## M23: Safe Deployment Release Lifecycle

- Status: Planned after M22.
- Derive rollout history from exact Deployment and ReplicaSet revision/template
  evidence; never accept a client-owned rollback template.
- Add fixed container-image update and exact revision rollback actions through
  server-side dry-run, typed diff, one-time confirmation, idempotency, target
  UID/resourceVersion preconditions and audit.
- Expose rollout progress, failure reason and operation history in the resource
  detail workflow, then prove update/rollback/restoration against disposable
  real-kind fixtures.

## M24: Fixed Cross-Cluster Promotion

- Status: Planned after M23.
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

- Status: Planned after M24.
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
