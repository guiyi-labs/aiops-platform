# Product Optimization Roadmap

- Updated: 2026-07-28
- Baseline: M20 Phase 11 identity readiness accepted at `216eb81e92b9ccc009c62c9d03ec58f9116bfd33`; Phase 12 recovery readiness is accepted locally and awaiting hosted CI; private remote remains active
- Principle: deepen observable, evidence-based operations before adding generic Kubernetes CRUD

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

- Status: In progress. Phase 1 bounded health fan-out, Phase 2 disposable fleet
  E2E, Phase 3 fixed-kind global search, Phase 4 user-owned saved filters,
  Phase 5 disposable search E2E and Phase 6 versioned CI/release pipeline were
  accepted on 2026-07-27 and 2026-07-28; evidence is archived in
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
  gate is accepted locally and hosted CI is pending.
- Continue with organization approval and isolated OIDC login integration,
  then isolated PITR and HA validation against approved infrastructure.
- Define bounded metrics retention and missing-sample semantics before storing
  historical time series.
