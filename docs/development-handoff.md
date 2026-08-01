# Development Handoff

> **安全重写说明**：Git 历史已执行安全脱敏重写（清除个人邮箱），所有 commit hash 均已变更。本文档中引用的旧 commit hash（如 `2d46588`、`cf20c66`、`0baf858`、`b1f52e0` 等）仅供历史归档参考，无法直接 checkout。请使用 tag 名称或提交信息搜索定位里程碑。文档中的 `<repo-root>`、`<local-refs>`、`<docker-data>` 为路径占位符，需替换为实际路径。

## Current Baseline (2026-08-01 M36+M37+M38+M39+M40+M41+M42+M43+M44+M45+M46+M47+M48+M49+M50+M51+M52+M56+M57+M58+M59+M60 Complete)

M60 (Static Provider Registry + Lifecycle) closes Phase 5 of the post-M45
roadmap with a single centralized `capability.Registry` that unifies the ten
previously ad-hoc capability surfaces wired into `cmd/server/main.go` and the
HTTP router. Every provider now carries a stable `ProviderDescriptor` (name,
kind, dependencies, cluster-role eligibility, configured flag, optional
Lifecycle and HealthChecker), starts in deterministic topological order
(dependencies before dependents) and stops in reverse, honors cluster-role
gating (`federation`, `inspection_scheduler`, `copyops_cross_cluster` only
start under host/standalone roles — member clusters keep them `disabled` with
`InactiveReason = "cluster role"`), and reports cached health (1s cache,
refreshable via `?refresh=true`). Two new OpenAPI routes
(`GET /api/v1/capability/providers` and `/:name`) expose the registry under
the `system_ops_admin` scope using a 1:1 `ProviderInfo` schema. 12 table-driven
registry tests cover Register/Start/Stop/Health/ClusterRoles/Cache/Cycle
branches at 84.2% statement coverage.
M59 (Structural) extends `.github/workflows/release.yml` with Cosign v2.4.1
keyless signing of SHA256SUMS + release-metadata.json, an in-toto SLSA v1
provenance placeholder bound to a bundle aggregate digest, and a
`cosign attest-blob` placeholder (`|| true` fail-open for rehearsal runs).
HA and PITR are *not* implemented in M59; ADR 0075 marks their place in the
signing chain so they can reuse the same workflow identity when they land.
Fast gate: `verify-fast.ps1 -Scope All` passed in 76.71s (backend=True
frontend=True manifests=True); helm lint skipped locally (helm not installed,
CI-enforced).

M58 (DevOps Read-Only + Cross-Cluster Copy + Backup/Restore GUI) closes
Phase 3 of the post-M45 roadmap with three DevOps-facing backend
enhancements: (a) GitOps (ArgoCD Application) read-only integration —
`argoproj.io/v1alpha1 Application` CRs are read directly via the ADR
0004 bounded Kubernetes gateway, projected into GUI-friendly
sync/health + repo/destination envelopes with a capability probe that
gracefully returns `available=false` when ArgoCD is not installed on
the cluster; (b) interactive cross-cluster copy (copyops) — a
lightweight primitive built on the same M19 controlled-operation state
machine as promotion/backup/restore, supporting preview → execute flow
with a 20-item bundle cap, operator-curated kind whitelist, source
namespace identity CAS gate, per-item scrubbing and dry-run validation,
and idempotency-key replay; (c) Velero backup/restore GUI browse
extensions — list/get endpoints for `velero.io/v1 Backup` and
`Restore` CRs so the GUI can render existing cluster backups before
guiding users through M22/M23 plan creation. 11 routes total; copyops
writes audit targets into the unified audit trail. 000039 migration
adds one JSONB-heavy `copy_plans` table. GitOps + copyops service
tests use stubbed dependencies (no CGO).
M57 (Helm Application Catalog + Controlled Deploy Plans) opens Phase 4
(delivery & ops integration) of the post-M45 roadmap with a simplified
Helm application catalog and M19 controlled-operation deploy plans.
M57 delivers (a) Helm repository CRUD with optional basic-auth
credentials stored in a `credentials_json` JSONB column — credentials
are structurally never returned in API responses (the `RepositoryView`
projection has no credentials field; only a `has_auth` boolean is
exposed); (b) read-only chart listing/detail via live `index.yaml`
HTTP fetch (10 MiB body limit, 15s timeout) — no Helm SDK dependency,
no caching, metadata always fresh; (c) M19 controlled-operation deploy
plans that reuse the exact confirmation-token + idempotency-key + Claim
state machine from promotion/backup/restore/maintenance — preview
builds a Flux `HelmRelease` CR manifest (`helm.toolkit.fluxcd.io/
v2beta1`, already in the M49 CRD whitelist), validates it via a
server-side dry-run, and persists the plan with a one-time confirmation
token (SHA-256 hashed); execute claims the plan (row-level lock +
constant-time token compare + idempotency key check), creates the
HelmRelease CR via the M49 generic `CreateResource` path, and marks the
plan succeeded/failed (a 409 conflict during execute is treated as
success — the HelmRelease already exists from a previous timed-out
attempt). The manifest is built once at preview and applied verbatim at
execute — deterministic, no re-rendering. 10 routes under
`/api/v1/app-catalog` (4 repository CRUD, 2 chart read, 2 plan read, 2
plan write); writes require `system_ops_admin`, reads are any-auth. 32
appcatalog service tests + 24 handler tests.
M52 (Intelligent Inspection + Service Mesh Read-Only) closes Phase 2 of the
post-M45 roadmap. M52 delivers (a) a KubeEye-style single-binary inspection
engine over an 8-rule compile-time catalog (node_not_ready, pod_restart_loop,
pod_oom_killed, pvc_pending, image_pull_backoff, pod_crash_loop,
endpoints_orphan, namespace_quota_high) with per-cluster enabled/severity
overrides, bounded execution at every layer (MaxConcurrentClusters=4,
PerClusterTimeout=15s, per-rule descriptor timeout, MaxTaskResults=1000
short-circuit truncation), one-shot trigger + standard-cron periodic plans,
and findings normalized into the M39 signal_occurrences table so M42
correlation and M44 automation consume them natively; plus (b) a strictly
read-only Istio service-mesh surface covering VirtualService/DestinationRule
list+detail via the M49 CustomResource gateway (shallow projections; raw
manifest not returned — operators use M49 generic CRD browser for drilldown)
and traffic metrics (request volume, 2xx/4xx/5xx, error rate, latency
p50/p95/p99) aggregated from the M36 Prometheus history via six fixed
template queries (no client PromQL, window capped at 24h, step normalized
floor 15s). Everything evidence-only, zero governance write paths.
M51 (Bounded Event Stream + Alert Inhibits), M50 (Monitoring Dashboard + Log
Explorer), M49 (CRD Discovery + Read-Only
Custom Resource Browsing), M48 (Multi-Cluster Federation), M47 (Three-Tier
Console Navigation + Workspace Resource Filter), M46 (Workspace
Multi-Tenancy), M45 (Versioned AIOps Golden Dataset And
Quality Report), M44
(Policy-Constrained Automation and Post-Action Verification), M43
(Cited and Evaluated AI Investigator), M42 (Multi-Signal Correlation
and Deterministic RCA), M41 (SLO, Error Budget and Impact), M40 (Temporal
Topology and Change Intelligence), M39 (Unified Service Identity and
Signal Model), M38 (Engineering, Delivery and Supply-Chain Hardening),
M37 (Capability Plane Adapters) and M36 (Production OIDC and MFA) are
development complete. M51 extends Phase 2 (full-stack observability)
with a bounded SSE event stream over Kubernetes Events (poller, not
Watch) and alert inhibit rules (source_match → target_match suppression
while the source is firing, not time-bounded). The event stream reuses
the M35 namespace scope (empty scope → empty stream, anti-leakage) and
the read-only gateway (ADR 0004); drop-oldest backpressure bounds
per-client memory. Inhibits complement the M37B time-bounded silences
and are re-evaluated on every `MatchAndDeliver` call against the M27
delivery table (no second alert system). M50 opens Phase 2 (full-stack observability) with
a fixed-template monitoring dashboard (single-cluster + workspace
cross-cluster topology) and a bounded Loki log explorer reusing the
M37A `capability.LogProvider`. Clients cannot inject PromQL/LogQL; the
dashboard returns panel descriptors that the frontend uses to drive
existing `/metrics/history` calls, and the workspace dashboard returns
only the cross-cluster `(cluster_id, namespaces)` topology (no
per-cluster time-series pre-fetch). The log explorer re-checks the body
namespace against the M35 resolved namespace scope (anti-leakage 404).
M45 introduces the versioned AIOps golden dataset
and quality report as the replayable contract for the full AIOps loop
(M39-M44); the dataset contains 3 scenarios (mandatory 10-step end-to-end
+ 2 negative companions: misattribution prevention, partial/unknown
fail-closed) and the quality report records before/after comparison per
scenario with aggregated summary metrics. M44 closes the AIOps loop: an
eligible M43 runbook is materialized into an action plan, gated through
deterministic policy checks (8 gates), approved by a human (L2 default;
four-eyes for rollback/image_update), executed idempotently against the
Kubernetes source, and verified against captured pre/post evidence;
missing evidence never resolves a diagnosis automatically (yields
`unknown`), and a server-owned rollback contract drafts a rollback plan
when safe or escalates to a human. M43 introduces a cited and evaluated
AI investigator bound to M42 correlation cases; the investigation is a
read-only advisory that never modifies the case, diagnosis or alert, and
every factual claim cites an authorized evidence ID (fabricated or
out-of-scope citations reject the entire output); on provider failure or
citation rejection a failed investigation is persisted so deterministic
investigation remains available. M42 introduces a deterministic,
replayable multi-signal correlation engine that links M39 signals, M40
topology/changes and diagnosis records into bounded cases; the diagnosis
record remains the human status/SLA/feedback source of truth. M41
introduces server-owned SLI templates, versioned SLO definitions,
deterministic evaluation with explicit missing-data handling, and
burn-alert transitions that feed the existing M27 alert lifecycle; the
native signal path is unchanged. M40 persists reviewed relationship edges
(8 kinds with validity intervals) and normalizes M23-M31 platform-operation
outcomes into a unified change timeline; the native signal path is
unchanged. M39 normalizes existing M21-M31 outputs into a unified
`signal_occurrences` table with fingerprint-based deduplication; the
native signal path is unchanged. M37 adds bounded Prometheus/Loki
capability adapters and alert routing with bounded silences; all adapters
are disabled by default. The OIDC provider supports Authorization Code +
PKCE S256, JWKS caching with key rotation, MFA evidence, GORM identity
resolver and session management; OIDC remains disabled by default. The
pull-request gate now runs the race detector, golangci-lint, ESLint, a 50%
coverage baseline and an OpenAPI breaking-change check; the real-kind E2E
workflow covers M23-M31; an official Helm 3 chart ships under
`deploy/helm/aiops-platform/`; releases produce multi-architecture OCI
images and SPDX SBOMs; a license allowlist is enforced at gate time;
`SECURITY.md` and `CHANGELOG.md` are tracked delivery assets. M35
(lightweight cluster and namespace access grants) and M34 (RouteDescriptor
contract and RBAC inventory) remain the contract and authorization
foundation. M33 (restricted `client-go` migration) remains the
transport-layer foundation. Continue from the current commit on local
`main` (ahead of `baseline-m32-20260731`).

The approved post-M32 optimization route is `docs/kubesphere-optimization-plan.md`.
Later historical "Recommended Next Work" and "Next Priorities" sections in this file are not current instructions.

- Last updated: 2026-08-01
- Repository: `C:\BS\aiops-platform`
- Git baseline: local `main`, tag `baseline-m32-20260731`;
  M33+M34+M35+M36+M37+M38+M39+M40+M41+M42+M43+M44+M45+M46+M47+M48+M49+M50+M51+M52+M56+M57+M58 commits pending; push/release not authorized
- Current milestone: M32 local archive accepted; M33 transport swap, M34
  contract-debt closure, M35 resource-scope authorization, M36 production
  OIDC/MFA, M37 capability plane adapters, M38 engineering/delivery/
  supply-chain hardening, M39 unified signal model, M40 temporal topology
  and change intelligence, M41 SLO/error budget/impact, M42 multi-signal
  correlation/deterministic RCA, M43 cited/evaluated AI investigator,
  M44 policy-constrained automation/post-action verification, M45
  versioned golden dataset/quality report, M46 workspace
  multi-tenancy, M47 three-tier console navigation/workspace resource
  filter, M48 multi-cluster federation, M49 CRD discovery + read-only
  custom resource browsing, M50 monitoring dashboard + log explorer,
  M51 bounded event stream + alert inhibits,
  M52 intelligent inspection + service mesh read-only,
  M56 golden dataset replay + quality dashboard,
  M57 Helm application catalog + controlled deploy plans,
  and M58 DevOps readonly + cross-cluster copy + backup/restore GUI
  complete; only hosted CI/release,
  organization OIDC/MFA production run, PITR and HA remain external gates.
  The committed development route ends at M32; M33-M52 are post-M32
  optimization milestones per `docs/kubesphere-optimization-plan.md`.

### M27-M58 Closure Summary

| Milestone | Status | Closure record |
|---|---|---|
| M27 Historical Alert Lifecycle | ✅ Accepted 2026-07-30 | `docs/changes/2026-07-30-m27-alert-lifecycle.md` |
| M28 Controlled Velero Backup Creation | ✅ Accepted 2026-07-30 | `docs/changes/2026-07-30-m28-controlled-backup-creation.md` |
| M29 Namespace Governance and Capacity Posture | ✅ Accepted 2026-07-31 | `docs/changes/2026-07-31-m29-namespace-posture.md` |
| M30 Controlled Node Maintenance | ✅ Accepted 2026-07-30 | `docs/changes/2026-07-30-m30-controlled-node-maintenance.md` |
| M31 Isolated Workload Restore Rehearsal | ✅ Accepted 2026-07-30 | `docs/changes/2026-07-30-m31-isolated-workload-restore-rehearsal.md` |
| M32 Formal Closure | ✅ Final local archive 2026-07-31 | `docs/changes/2026-07-31-final-baseline-archive.md` |
| M33 Restricted client-go Migration | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m33-restricted-client-go-migration.md` |
| M34 Route Descriptor Contract and RBAC Inventory | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m34-route-descriptor-and-rbac-inventory.md` |
| M35 Lightweight Cluster and Namespace Access Grants | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m35-lightweight-cluster-and-namespace-access-grants.md` |
| M36 Production OIDC and MFA | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m36f-oidc-http-wiring-and-gorm-resolver.md` |
| M37 Capability Plane Adapters | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m37-capability-plane-adapters.md` |
| M38 Engineering, Delivery and Supply-Chain Hardening | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md` |
| M39 Unified Service Identity and Signal Model | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m39-unified-signal-model.md` |
| M40 Temporal Topology and Change Intelligence | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m40-temporal-topology-and-change-intelligence.md` |
| M41 SLO, Error Budget and Impact | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m41-slo-error-budget-and-impact.md` |
| M42 Multi-Signal Correlation and Deterministic RCA | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m42-multi-signal-correlation-and-deterministic-rca.md` |
| M43 Cited and Evaluated AI Investigator | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m43-cited-and-evaluated-ai-investigator.md` |
| M44 Policy-Constrained Automation and Post-Action Verification | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m44-policy-constrained-automation-and-post-action-verification.md` |
| M45 Versioned AIOps Golden Dataset and Quality Report | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m45-versioned-aiops-golden-dataset-and-quality-report.md` |
| M46 Workspace Multi-Tenancy | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m46-workspace-multi-tenancy.md` |
| M47 Three-Tier Console Navigation + Workspace Resource Filter | ✅ Development complete 2026-07-31 | `docs/changes/2026-07-31-m47-three-tier-console-and-workspace-filter.md` |
| M48 Multi-Cluster Federation (Host/Member Model) | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m48-multi-cluster-federation.md` |
| M49 CRD Discovery + Read-Only Custom Resource Browsing | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m49-crd-discovery-and-browsing.md` |
| M50 Monitoring Dashboard + Log Explorer | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m50-monitoring-dashboard-and-log-explorer.md` |
| M51 Bounded Event Stream + Alert Inhibits | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m51-bounded-event-stream-and-alert-inhibits.md` |
| M52 Intelligent Inspection + Service Mesh Read-Only | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m52-inspection-and-servicemesh.md` |
| M56 Golden Dataset Replay + Quality Dashboard | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m56-golden-replay-and-quality-dashboard.md` |
| M57 Helm Application Catalog + Controlled Deploy Plans | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m57-helm-app-catalog.md` |
| M58 DevOps Read-Only + Cross-Cluster Copy + Backup/Restore GUI | ✅ Development complete 2026-08-01 | `docs/changes/2026-08-01-m58-devops-readonly-copyops-backup-gui.md` |

### M58 Highlights

- **GitOps (ArgoCD Application) read-only (M58):** New `internal/gitops` package
  with `Service.Capability` / `List` / `Get`. `Capability` probes for
  `argoproj.io/v1alpha1 Application` CRDs first; absent CRD →
  `{available: false, mode: "none"}` with probe-level counts. List/Get
  read Application manifests via the existing ADR 0004 bounded Kubernetes
  gateway and project into GUI-friendly rows (name, namespace, project,
  sync_status, health_status, repo_url/path/revision, destination) plus
  `raw_manifest`. No ArgoCD SDK, no proxy credentials.
- **Interactive cross-cluster copy (copyops) (M58):** New `internal/copyops`
  package reusing the M19 controlled-operation state machine.
  `Service.Preview` captures source-namespace identity (CAS gate), scrubs
  manifests with fixed label/annotation prefix strips + Secret scrub
  toggle + nodeName drops, runs per-item "already exists on destination"
  skip + server-side dry-run create. Bundle cap MaxBundle=20 enforced
  BEFORE normalize/dedup. `Service.Execute` re-checks CAS (M28 backup
  style), re-checks destination namespace, applies pending items as
  create-only. `Repository.ClaimAndLoad` rewritten for idempotency-replay
  short-circuit: terminal status + matching idempotency-key → returns
  completed plan (not ErrAlreadyExecuted). Migration 000039 adds one
  JSONB-heavy `copy_plans` table (single-row, no child tables;
  char_length(id)=36 CHECK; TTL expires_at; locked_at lease).
- **Velero backup/restore GUI browse extensions (M58):** `backup.go` and
  `restore.go` in `httpserver` extended with `listBackups`/`getBackup`
  and `listRestores`/`getRestore` reading `velero.io/v1 Backup`/
  `Restore` CRs via existing cluster dynamic client. Rows project
  phase/errors/warnings/started/completed/storage_location/schedule/
  included/excluded namespaces; raw_manifest available for detail views.
  Existing M22/M23 Plan CRUD is untouched.
- **OpenAPI + route-contract (M58):** 11 new paths + 13 new schemas in
  `openapi.yaml` (GitOpsCapability, GitOpsApplication[List],
  VeleroBackupList, VeleroRestoreList, CopyBundleItem,
  CopyPlanPreviewRequest, CopyPlanExecuteRequest, CopyItemDiff,
  CopyPlanResourceItem, CopyPlanDiffSummary, CopyPlan[List]).
  `TestRegisteredRoutesMatchOpenAPI` passes for all 11 routes.
- **Authorization + tests (M58):** All 11 routes protected by bearerAuth
  + per-cluster + workspace tenancy gates. copyops preview/execute write
  audit targets into the unified audit trail. GitOps service tests cover
  capability-none/present, list filters, get hit/miss with stubbed Raw
  client. Copyops service tests use thread-safe `inmemRepo` (no CGO) +
  `fakeKubernetes` covering invalid inputs, bundle-too-large pre-dedup,
  disallowed kind, success preview+write, destination missing, Execute
  idempotency replay SAME/DIFFERENT keys, Execute CAS drift → failPlan.
- **Service DI (M58):** `cmd/server/main.go` wires `gitops.NewService(k8sSvc)`
  and `copyops.NewService(k8sSvc, copyops.NewGormRepository(db))` into the
  HTTP server options; all handlers reuse existing auth/session/aikit
  middleware.

### M57 Highlights

- **Helm repository CRUD (M57):** New `internal/appcatalog` package
  with `Service.CreateRepository` / `GetRepository` /
  `ListRepositories` / `DeleteRepository`. Credentials stored in
  `helm_repositories.credentials_json` JSONB column; structurally
  never returned in API responses (the `RepositoryView` projection
  has no credentials field; only a `has_auth` boolean is exposed).
  Repository writes (create/delete) require `system_ops_admin`;
  reads are any-auth. See ADR 0069.
- **Read-only chart listing/detail (M57):** `Service.ListCharts` and
  `Service.GetChart` fetch chart metadata live from each repository's
  `index.yaml` over HTTP via `HTTPIndexSource` (10 MiB body limit,
  15s timeout, basic-auth from `credentials_json`). No Helm SDK
  dependency — plain HTTP + YAML parse. No caching; metadata is
  always fresh.
- **M19 controlled-operation deploy plans (M57):** `Service.Preview`
  validates the request, checks the target namespace exists, fetches
  chart metadata, checks no existing HelmRelease with the same name,
  builds a Flux `HelmRelease` CR manifest
  (`helm.toolkit.fluxcd.io/v2beta1`, already in the M49 CRD
  whitelist), runs a server-side dry-run, and persists the plan with
  a one-time confirmation token (SHA-256 hashed). `Service.Execute`
  claims the plan via `ClaimPlan` (row-level lock +
  `subtle.ConstantTimeCompare` for token + idempotency key check),
  creates the HelmRelease CR via the M49 generic `CreateResource`
  path, and marks the plan succeeded/failed. A 409 conflict during
  execute is treated as success (HelmRelease already exists from a
  previous timed-out attempt). The manifest is built once at preview
  and applied verbatim at execute — deterministic, no re-rendering.
  Plan TTL 30 minutes; claim TTL 5 minutes. Mirrors the exact M19
  state machine from promotion/backup/restore/maintenance.
- **No Helm SDK dependency (M57):** Chart metadata comes from
  `index.yaml` HTTP fetch; deployment targets a Flux HelmRelease CR
  (already in the M49 CRD whitelist). The Flux helm-controller
  handles reconciliation, retries, and rollbacks — the platform just
  creates the CR. Binary stays small; attack surface minimal.
- **Authorization + anti-leakage (M57):** 10 routes under
  `/api/v1/app-catalog` registered on `v1` (not `resourceRoutes`,
  mirroring promotion routes — app-catalog endpoints don't take a
  `:cluster_id` path parameter). Write operations (create repo,
  delete repo, preview, execute) require `system_ops_admin`; reads
  are any-auth. All tagged with audit verbs
  (`app_catalog.repositories.{list,create,read,delete}`,
  `app_catalog.charts.{list,read}`,
  `app_catalog.plans.{list,read,preview,execute}`) per ADR 0008.
  No new roles, no new middleware — the 2D authorization matrix is
  intact.
- **Migration 000038 (M57):** 2 new tables — `helm_repositories`
  (id, name UNIQUE, display_name, url, credentials_json JSONB,
  created_by, timestamps) and `app_catalog_plans` (id VARCHAR(36)
  PK, status, repo_id FK, chart snapshot, target cluster/namespace,
  release_name, values_yaml, chart_metadata JSONB, release_manifest
  JSONB, deploy_diff JSONB, confirmation_token_hash BYTEA,
  idempotency_key, locked_at, executed_at, last_error, expires_at,
  timestamps). Indexes on `helm_repositories(name)`,
  `app_catalog_plans(status)`, `app_catalog_plans(target_cluster_id,
  target_namespace)`, `app_catalog_plans(idempotency_key)`.
- **Tests (M57):** 32 appcatalog service tests (repository CRUD,
  chart listing/detail, preview success + 4 error paths, execute
  success + invalid-token + invalid-idempotency + plan-not-found +
  idempotent-replay, list plans, manifest building, HelmRelease
  path resolution, validation) + 24 handler tests (each route's
  success + error path, sentinel error mapping).
  `TestRegisteredRoutesMatchOpenAPI` covers all 10 new paths.

### M52 Highlights

- **8-rule compile-time inspection catalog (M52):** New `internal/inspection`
  package with `DefaultCatalog()` returning 8 fixed `RuleDescriptor`
  entries (node_not_ready, pod_restart_loop, pod_oom_killed, pvc_pending,
  image_pull_backoff, pod_crash_loop, endpoints_orphan,
  namespace_quota_high). Each rule carries a stable M39 signal code of
  the form `inspect.<domain>.<code>.v1`, default severity, per-rule
  timeout, description, and remediation. No runtime rule injection (no
  KubeEye operator, no external schema); per-cluster `enabled` and
  `severity_override` stored in `inspection_rules` SQL table. See ADR
  0067.
- **Bounded inspection engine (M52):** `Service.RunInspectOnce` fans out
  to at most `MaxConcurrentClusters=4` workers; each cluster subject to
  `PerClusterTimeout=15s` (5s..120s) and each rule subject to the
  descriptor timeout. When `MaxTaskResults=1000` is reached the task
  short-circuits to `RESULTS_TRUNCATED` summary state — hard memory and
  storage cap regardless of fleet size. All reads go exclusively
  through the ADR 0004 read-only gateway.
- **Findings normalized to M39 signal model (M52):** Each `Finding` is
  batch-upserted into `signal_occurrences` via the M39 signal.Service
  path: `source='inspection'`, `evidence_id → inspection_results.id`,
  fingerprint `sha256(cluster_id + signal_code + resource_uid +
  summary_prefix)` with 300s dedup window. M42 correlation and M44
  automation therefore consume inspection findings as first-class
  signals with zero new wiring.
- **Cron-driven plan scheduler (M52):** In-process `Scheduler` ticks
  every 30s, loads enabled plans from SQL each tick (no in-memory plan
  cache → no multi-replica split-brain), and elects exactly one runner
  per plan tick via `UPDATE … SET last_run_at = NOW() WHERE id = ? AND
  last_run_at < ?` using `clause.OnConflict`. Not a K8s CronJob
  (single-binary ADR 0002).
- **Service mesh read-only surface (M52):** New `internal/servicemesh`
  package provides 4 list/detail views via M49 CustomResource gateway
  (`networking.istio.io/v1beta1` VirtualService/DestinationRule) —
  shallow projection (hosts/gateways/http_routes_count; host/subsets_count/
  traffic_policy_summary) only. Cluster not running Istio → empty list,
  never 5xx. Raw manifest not returned; operators use M49 generic CRD
  browser for drilldown.
- **Traffic metrics from Prometheus history (M52):** `TrafficMetrics`
  aggregates 6 fixed-template series from M36 history (istio_requests_total
  by response_code, istio_request_duration_milliseconds quantile
  approximation) → returns request_count, 2xx/4xx/5xx, error_rate, and
  latency p50/p95/p99 points arrays. Window capped at 24h; step
  normalized to `max(window/500, 15s)`. Zero client PromQL injection.
  Feeds M41 SLO evaluator + M40 topology edges — never an action input.
- **Authorization + anti-leakage (M52):** `operations_admin` for plan
  create/delete / override save / run-once; `operations_viewer` for
  reads; `mesh_admin`/`mesh_viewer` for mesh routes. M35 namespace
  scope honoured: out-of-scope VirtualService/DestinationRule → 404
  (not 403); empty scope → empty task-results list + empty metrics.
- **Migration 000037 (M52):** 4 new tables — `inspection_rules`
  (UNIQUE(cluster_id, rule_code)), `inspection_plans`
  (cron_expr CHECK not-empty), `inspection_tasks` (QUEUED → RUNNING →
  SUCCEEDED/PARTIAL/FAILED + RESULTS_TRUNCATED summary state; trigger
  snapshot; counts; timestamps), `inspection_results` (task_id FK,
  resource_ref JSON, labels JSON). Indexes + CHECK constraints as per
  ADR 0067 §Deployment notes.
- **Tests (M52):** 27 inspection service tests (~97% coverage); 16
  servicemesh service tests; 38 handler tests (21 inspection + 17
  servicemesh). `TestRegisteredRoutesMatchOpenAPI` covers all 13 new
  paths. verify-fast.ps1 -Scope Backend passes.

### M51 Highlights

- **Bounded SSE event stream (M51):** New `internal/eventstream` package
  with `Service.Subscribe` opening a per-client poller goroutine over the
  read-only Kubernetes gateway (ADR 0004). The stream is a bounded poller
  (default 5s, min 50ms, max 60s), NOT a Kubernetes Watch — no long-lived
  gateway connection, no informer cache. Events are deduped by UID against
  a bounded ring (256 entries) and pushed with drop-oldest backpressure
  (BufferCap 256, min 16, max 1024). The stream emits hello / event /
  stream-closed SSE messages. See ADR 0066.
- **M35 namespace scope honoured (M51):** The handler resolves the
  caller's M35 namespace scope. All-namespaces polls cluster-wide; an
  authorized namespace set polls each namespace; an empty scope (no
  grants, not all-namespaces) yields an immediately-closed empty stream
  so unauthorized namespaces are not leaked (empty stream, not 404 —
  anti-leakage, ADR 0066 §3). The optional `?namespace=` query param
  narrows the scope (404 when unauthorized).
- **State-evaluated alert inhibits (M51):** `GET/POST/DELETE
  /api/v1/alert-routes/inhibits` manage source_match → target_match
  suppression rules. An inhibit is NOT time-bounded; suppression depends
  on the source alert's live firing state (a non-resolved
  `alert_route_delivery` within the 5m active window), re-checked on
  every `MatchAndDeliver` call. `MatchAndDeliver` checks `IsInhibited`
  after `IsSilenced`, before route matching — an inhibited alert produces
  no delivery record (fully suppressed). This complements the M37B
  time-bounded silences; the M27 lifecycle remains the single alert
  system.
- **Inhibit validation and limits (M51):** `reason` mandatory (1..500
  chars, mirrors silences). At least one source AND one target matcher
  required (rejects fully-wildcard inhibits). `MaxInhibitsPerUser = 30`
  mirrors `MaxSilencesPerUser`. Inhibits are creator-scoped (non-creator
  deletes return 404 `INHIBIT_NOT_FOUND`, indistinguishable from
  missing). Migration `000036_alert_inhibits` enforces the matcher
  constraints with CHECK constraints.
- **Authorization reused from M35 + M37B (M51):** Event stream registered
  under `resourceRoutes` (M35 cluster/namespace scope); inhibits
  registered under `alertrouteRoutes` (auth-required for list;
  `operations_admin` for create/delete). No new authorization path; the
  2D matrix is intact.
- **404 > 403 anti-leakage (M51):** Unauthorized inhibit delete → 404
  `INHIBIT_NOT_FOUND`; unauthorized namespace on event stream → empty
  stream (not 404).
- **Tests (M51):** 38 unit tests (23 service + 15 handler) covering
  eventstream config/subscribe/dedup/backpressure/poll-error/cancel,
  inhibit create/list/delete/validation/limit/non-creator, IsInhibited
  firing semantics, MatchAndDeliver suppression, SSE handler
  (503/empty scope/delivery/headers/cancel).

### M50 Highlights

- **Fixed-template monitoring dashboard (M50):** New `internal/monitoring`
  package with 4 compile-time-fixed dashboard templates
  (`node_overview`, `workload_overview`, `pod_overview`,
  `workspace_overview`), each carrying 2 `Panel` descriptors (metric,
  unit, resource_kind). Clients cannot inject PromQL; the dashboard
  returns panel descriptors that the frontend uses to drive existing
  `/metrics/history` calls (ADR 0036). Adding a template is a code
  change — no admin API, no runtime expansion (static-extension hard
  constraint). See ADR 0065.
- **Topology-only workspace dashboard (M50):** The workspace dashboard
  (`GET /api/v1/workspaces/:workspace_id/monitoring/dashboard`) returns
  the fixed `workspace_overview` template plus the workspace's
  cross-cluster `(cluster_id, namespaces)` topology. The backend does
  NOT pre-fetch per-cluster time series; the frontend fans out using
  the topology. Fan-out bounded by `MaxClusters` (20, matching
  federation). Cluster IDs sorted ascending for stable rendering.
- **Bounded Loki log explorer (M50):** `POST /api/v1/clusters/:cluster_id/logs/query`
  reuses the M37A `capability.LogProvider` (Loki adapter, ADR 0053).
  Clients cannot inject LogQL. The namespace arrives in the body, so
  the handler re-checks it against the M35 resolved namespace scope
  (anti-leakage 404). Provider errors map to 400 (`ErrInvalidLogQuery`)
  or 500 (runtime); nil provider → 503 `LOG_PROVIDER_UNAVAILABLE`.
- **Authorization reused from M35 + M46 + M47 (M50):** Cluster
  dashboard + logs/query registered under `resourceRoutes` (M35
  cluster/namespace scope + M47 workspace filter); workspace dashboard
  registered under `workspaceRoutes` with `workspace_viewer` enforced
  via `workspace.Service.ListMemberships`. No new authorization path;
  the 2D matrix is intact.
- **24h window bound (M50):** `MaxDashboardWindow = 24h` matches
  metricshistory `MaxQueryWindow` (ADR 0034). `validateWindow` rejects
  zero, inverted, or over-24h windows with `ErrInvalidWindow` (400).
- **404 > 403 anti-leakage (M50):** Unauthorized/missing workspace →
  404 `WORKSPACE_NOT_FOUND`; unauthorized namespace in logs/query →
  404 `RESOURCE_NOT_FOUND`.
- **Tests (M50):** 28 unit tests (11 service + 17 handler) covering
  template validation, window validation, panel cloning, workspace
  topology (sorted + bounded), nil service/provider (503), logs query
  (200/400/404/500 + scope allowed/denied + default direction +
  provider error).

### M49 Highlights

- **Read-only CRD browser (M49):** 2 read-only HTTP routes under
  `/api/v1/clusters/:cluster_id/custom-resources`:
  `GET /:group/:version/:resource` (list) and
  `GET /:group/:version/:resource/:name` (detail). Only `GET` is registered —
  no write routes and no write service methods; the read-only contract is
  structural. See ADR 0064.
- **Compile-time-fixed GVR whitelist (M49):** `customResourceWhitelist` in
  `internal/kubernetes/service.go` covers 22 operator CRD GVRs across Velero
  (3), Prometheus operator (8), Flux Helm/source (5), and cert-manager (6).
  One entry (`cert-manager.io/v1/clusterissuers`) is cluster-scoped; the rest
  are namespaced. Adding an entry is a code change — no admin API, no runtime
  expansion (static-extension hard constraint).
- **Manifest redaction reused from M22 (M49):** Both endpoints run the raw
  CRD manifest through `redactManifest` (M22 Secret/ConfigMap redaction);
  sensitive fields (`password`, `secret`, `token`, `key`, ...) are recursively
  redacted to `"<redacted>"`.
- **Authorization reused from M35 + M47 (M49):** Namespaced CRDs fan out
  across the caller's `ClusterScope` via `authorizedNamespaceLists`; the
  `?workspace_id` filter (M47) narrows the namespace set. Cluster-scoped CRDs
  are listed cluster-wide. No new authorization path; the 2D matrix is intact.
- **404 > 403 anti-leakage (M49):** A non-whitelisted GVR returns 404
  `RESOURCE_NOT_FOUND` before the gateway is contacted (indistinguishable from
  a missing resource). An empty authorized scope yields 200 `items: []`.
- **Tests (M49):** 34 unit tests (17 service + 17 handler) covering
  whitelist allow/deny, namespaced vs cluster-scoped path building, list +
  detail with redaction, name filter + sort, selector forwarding,
  cluster-disabled (409) and not-found (404) propagation, namespace-grant
  fan-out, empty-scope empty list, only-GET registered, and the
  `writeServiceError` whitelisted mapping.

### M48 Highlights

- **Multi-cluster federation (M48):** New `backend/internal/federation`
  package implementing a KubeSphere-style host/member cluster model as a SQL
  aggregation view over the existing `clusters` table. No new CRD, no
  Cluster Agent, no inter-cluster sync controller. Migration `000035`
  extends `clusters` with `cluster_role` (host/member/standalone) and
  `federation_status` (registered/healthy/degraded/disconnected) — both
  CHECK-constrained enums. A partial unique index enforces the single-host
  invariant at the database level. See ADR 0063.
- **Append-only federation events (M48):** New `cluster_federation_events`
  table records every federation state transition (registered/deregistered/
  heartbeat/status_change/role_change). The repository exposes no UPDATE or
  DELETE path, mirroring the platform audit pattern (ADR 0008).
- **Federation routes (M48):** 9 HTTP routes under `/api/v1/federation`:
  overview (topology + health), events, resource summary, cluster
  register/deregister/promote/demote/heartbeat/status, and per-cluster
  events. Write operations require `operations_admin`; reads require
  authentication only (visible clusters narrowed by authz scope).
- **Cross-cluster resource summary (M48):** Bounded fan-out (20 clusters,
  4s per-cluster timeout) over a fixed 9-entry GVR whitelist.
  Missing/unreachable clusters contribute zero counts with `TIMEOUT` /
  `QUERY_FAILED` error codes; partial results are always returned.
- **federation_status orthogonal to clusters.status (M48):** The existing
  cluster probe updates `clusters.status`; the federation heartbeat updates
  `federation_status`. A cluster can be `ready` but `degraded`.
- **Anti-leakage (M48):** `ErrClusterNotFound` surfaces as 404 so a missing
  cluster is indistinguishable from an unauthorized one.
- **Tests (M48):** 58 unit tests (38 service + 20 handler) covering
  register/deregister/promote/demote/heartbeat/status/overview/events/
  resource-summary, single-host invariant, idempotency, anti-leakage,
  timeout error mapping, and all HTTP status paths (200/400/404/409/503).

### M47 Highlights

- **CRD discovery preview (M47):** New
  `GET /api/v1/clusters/:cluster_id/api-resources` endpoint returning the
  union of a fixed operator-curated GVR whitelist (27 core resource kinds)
  and the cluster's dynamically discovered API resources. Discovery failures
  degrade gracefully (whitelist-only fallback); the endpoint never 500s due
  to discovery unavailability. Subresources are skipped; whitelist entries
  are deduplicated against discovery. Output is sorted by group/version/
  resource on all return paths. This is the M47 preview of M49's full CRD
  browsing. See ADR 0062.
- **Workspace resource filter (M47):** New optional `workspace_id` query
  parameter on namespace-scoped resource list endpoints, implemented by the
  `withWorkspaceNamespaceFilter` middleware + `narrowScopeByWorkspace` pure
  function. The filter is a **pure visibility narrowing**, NOT an
  authorization decision — it runs after `requireClusterAccess` +
  `requireNamespaceQueryAccess` and only narrows the already-authorized
  scope. The 2D authorization matrix (ADR 0050) and WorkspaceGrant
  orthogonality (ADR 0061) are unchanged.
- **Anti-leakage (M47):** Unauthorized cluster returns 404 before the
  filter; non-existent/empty workspace returns 200 with `items: []` (not
  404) so workspace existence is not leaked.
- **Tests (M47):** 23 unit tests (7 discovery + 5 workspace-filter service +
  11 httpserver-filter) covering whitelist-only fallback, CRD merge/dedup,
  discovery/credential error fallback, sorted output, zero-ID short-circuit,
  cross-workspace/cluster isolation, unknown-workspace empty result,
  AllNamespaces narrowing, namespace-grant intersection, anti-leakage
  empty-scope collapse, invalid `workspace_id` 400, repository-error 500,
  and nil-service pass-through.

### M46 Highlights

- **Workspace multi-tenancy (M46):** New `backend/internal/workspace`
  package with 5 SQL tables (`workspaces`, `workspace_memberships`,
  `workspace_quotas`, `user_workspace_grants`,
  `workspace_role_bindings_audit`), 3 fixed workspace roles
  (`workspace_admin` / `workspace_editor` / `workspace_viewer`), and 14
  HTTP routes under `/api/v1/workspaces`. WorkspaceGrant is orthogonal to
  ClusterGrant/NamespaceGrant — it does NOT grant namespace read access.
  Anti-leakage (404 > 403) is preserved. SystemAdmin bypasses all
  workspace grant checks. Owner is always `workspace_admin` and cannot be
  downgraded or revoked. Audit trail is append-only. Quota is display-only.
- **Tests (M46):** 39 tests (29 service-level + 10 handler-level)
  covering create/get/list/update/delete, membership, quota, role
  bindings, anti-leakage, role hierarchy, owner protection and metadata
  normalization.

### M45 Highlights

- **Golden dataset (M45):** New `backend/internal/golden` package with
  `DatasetVersion = "1.0"`, `ScenarioVersion = "1.0"`, 10 `StepID`
  constants, `AllSteps` ordered list, 3 `ScenarioID` constants,
  `StepOutcome` with expected signal/topology/SLO/correlation/
  investigation/action plan/verification/alert recovery flags, `Scenario`,
  `Dataset`, `DefaultDataset()` returning 3 scenarios.
- **Mandatory 10-step scenario (M45):** Maps each step of the AIOps loop
  to an expected outcome: establish_healthy_service (M41), publish_bad_image
  (M23), capture_signals (M39+M41), build_impact_graph (M40),
  rank_cause_candidate (M42), generate_investigation (M43),
  preview_approve_rollback (M44 approved), execute_verify (M44
  verified+effective), recover_alert (M27), cleanup.
- **Negative companions (M45):** `negative_misattribution` — unrelated
  change in another Namespace must NOT be attributed to the primary case
  (no action plan expected). `negative_partial_evidence` — when one
  provider is stopped, the case must be partial/unknown, not falsely
  healthy (valid advisory investigation expected, but no alert recovery).
  Preserves the M41 fail-closed invariant.
- **Quality report (M45):** `QualityReport` with before/after dataset
  versions, engine versions (M39-M44), per-scenario `ScenarioQuality`
  (passed_before/after, delta, steps_passed_before/after, notes),
  `QualitySummary` aggregation, `ClassifyDelta` (preserved/improved/
  regressed/unchanged), `Summarize`. JSON-serializable. Generated offline;
  never self-modifies rules, prompts or policy online.
- **Tests (M45):** 9 tests (dataset version, integrity, mandatory step
  coverage, negative misattribution, negative partial evidence, determinism,
  ClassifyDelta, Summarize, quality report end-to-end).
- **Production gates (M45, external/deferred):** Hosted CI with Linux race
  detector and full real-kind matrix, production OIDC/MFA and break-glass
  evidence, multi-replica deployment with PDB/topology spread/rolling-
  update evidence, external HA PostgreSQL with WAL/PITR and measured
  RPO/RTO, multi-instance no-duplicate-business-effect evidence, signed
  multi-arch release with SBOM/provenance/support matrix, real-kind E2E
  for the full 10-step scenario, real Prometheus/Loki/AI-provider replay
  in CI, frontend quality dashboard, CI integration that generates the
  quality report on every PR.

### M44 Highlights

- **Data model (M44):** New `backend/internal/automation` package with
  `AutomationVersion = "1.0"`, `VerifierVersion = "1.0"`, 4
  `AutomationLevel` values (L0/L1/L2/L3; L2 is the default, L3 not
  enabled in M44), 9 `PlanStatus` values (draft/previewed/approved/
  executing/succeeded/failed/expired/cancelled/verified), 2
  `ApprovalType` values (single/four_eyes), 3 `GateStatus` values
  (passed/failed/skipped), 8 `GateCode` values (uid_rv_recheck/scope/
  pdb_blast_radius/slo_burn/freeze_window/concurrent_plans/attempt_cap/
  rollback_point), `ActionPlan` with deterministic `plan_key` (SHA-256
  over case_id + runbook_id + target_uid + automation_version),
  `ActionVerification` with deterministic `verification_key` (SHA-256
  over plan_id + verifier_version + evidence_hash), `EvidenceSnapshot`,
  `SLOSnapshot`. Bound constants (`MaxAttemptsPerTarget` = 5,
  `AttemptWindowSeconds` = 3600, `DefaultPlanTTLSeconds` = 600,
  `DefaultClaimTTLSeconds` = 60, `DefaultCooldownSeconds` = 300,
  `MinCooldownSeconds` = 60).
- **Policy gate evaluator (M44):** `GateEvaluator` is stateless and pure.
  `RequiredGates(actionCode)` returns the action-specific gate set
  (core: uid_rv_recheck/scope/freeze_window/concurrent_plans/attempt_cap;
  Pod-affecting add pdb_blast_radius; SLO-bound add slo_burn; rollback
  adds rollback_point). `Evaluate` runs at preview; `Recheck` runs at
  execute with `Rechecked = true` and fresh `GateContext`. `AllPassed`
  treats `skipped` as non-failure. Adding a gate is a contract change
  (AutomationVersion bump). Stale UID/RV, opened freeze window,
  exhausted PDB budget, or exceeded attempt cap all fail closed.
- **Confirmation + idempotency (M44):** `Preview` issues a 32-byte
  confirmation token (base64; SHA-256 hashed at rest). `Execute` requires
  the plaintext token plus an operator-supplied idempotency key (UUID).
  `Claim` atomically transitions `approved → executing` under a row lock
  and stamps the key; replay returns the recorded outcome; re-execute
  with a different key after a terminal status yields `ErrAlreadyExecuted`.
  Stale `executing` rows past `claimTTL` are reclaimable.
- **Human approval (M44):** `approvalTypeFor(actionCode)` returns
  `four_eyes` for rollback and image_update, `single` otherwise.
  Four-eyes requires `approver_user_id != requested_by_user_id`;
  enforced at the DB layer (CHECK constraint) and re-checked by the
  service. Self-approval of a four-eyes plan yields
  `ErrSelfApprovalForbidden` (403).
- **Post-action verifier (M44):** `Verifier` is pure given (plan, pre,
  post). `CapturePreSnapshot` at execute time; `CapturePostSnapshot`
  after cooldown. `compareEvidence` is deterministic: SLO state
  transitions take precedence (healthy > burning_slow > burning_fast >
  breached); resource state (replicas/available_replicas/image/
  suspended) is compared for actions without SLO evidence or when SLO
  state is unchanged. Missing evidence yields `ComparisonInsufficient`
  and `VerificationStatusUnknown`. `classifyStatus` maps to
  effective/ineffective/failed/unknown.
- **Server-owned rollback contract (M44):** When verification yields
  ineffective/failed, `evaluateRollbackContract` checks target UID
  unchanged, no freeze, no concurrent plan, attempt cap not exceeded.
  If safe, a rollback plan is drafted automatically (status `draft`,
  `rollback_of_plan_id` set). If unsafe, verification records
  `reason = "unsafe_rollback_escalated_to_human"`. M44 never
  auto-executes rollback plans — they require the same preview →
  approve → execute path.
- **Repository (M44):** `GormRepository` with `SavePlan`/`GetPlan`/
  `GetPlanForExecute` (row lock)/`ListPlans`/`CountAttemptsSince`/
  `CountConcurrentPlans`/`MarkPreviewed`/`Approve`/`Claim`/`Complete`/
  `Fail`/`MarkVerified`/`Cancel`/`ExpireStale`/`SaveVerification`/
  `GetVerification`/`GetVerificationByPlan`/`UpdateVerification`.
  `NopRepository` for testing/disabled mode. 7 lifecycle sentinel
  errors.
- **HTTP routes (M44):** 10 routes under `/api/v1/aiops/automation`
  (runbooks, list/create/get plans, preview/approve/execute/cancel/
  verify plan, get verification). Write routes require
  `rolesSystemOpsAdmin`; read routes require only authentication. Actor
  derived from the authenticated session. Idempotency-Key header read
  by execute.
- **Migration 000033 (M44):** `action_plans` and `action_verifications`
  tables with CHECK constraints on status/approval_type/
  evidence_comparison/verification_status, the four-eyes distinctness
  CHECK, the missing-evidence → insufficient+unknown CHECK, partial
  unique indexes `uq_action_plans_active` (one non-terminal plan per
  `plan_key`) and `uq_action_verifications_active` (one pending
  verification per plan), FKs to `correlation_cases(id)` and
  `ai_investigations(id)` ON DELETE SET NULL.
- **Tests (M44):** 66 tests (11 gates + 17 verifier + 17 service + 21
  handler). Fast gate passed in 67.17s (30 backend packages including
  `automation`, 81 frontend tests/18 files).
- **Deferred (M44):** background verification worker (cooldown-based
  scheduling), stale `executing` reclaim worker, `ExpireStale`
  background worker, real Kubernetes integration tests for the patch
  path, real Prometheus/SLO integration for the `EvidenceProvider`,
  real PostgreSQL integration test, real-kind E2E, frontend UI, `L3`
  pre-authorized automatic execution level, rollback-plan auto-execution
  path, M42 `ActionCandidate` → M44 plan auto-suggestion.

### M43 Highlights

- **Data model (M43):** New `backend/internal/aiinvestigator` package with
  `InvestigatorVersion = "1.0"`, 3 `InvestigationStatus` values
  (completed/failed/stale), 3 `HypothesisConfidence` values
  (high/medium/low), 7 `EvidenceKind` values, `Investigation` with
  deterministic `investigation_key` (SHA-256 over case_id +
  investigator_version + prompt_hash), `Hypothesis`, `Citation`,
  `EvidenceRef`, `Prompt`, `ProviderResult`. Bound constants
  (`MaxHypothesesPerInvestigation` = 8, `MaxCitationsPerInvestigation` =
  64, etc.).
- **Runbook catalog (M43):** 4 V1 runbooks — `rollback_last_rollout`
  (`deployment.rollback`), `rollout_restart_pods`
  (`deployment.rollout_restart`), `inspect_pvc_capacity` (advisory),
  `inspect_node_maintenance` (advisory). `LookupRunbook` fails closed;
  advisory runbooks always eligible; adding a runbook is a contract
  change.
- **Prompt + validator (M43):** `BuildPrompt` assembles the system prompt
  (role, output schema, citation rules, runbook rules, prohibitions,
  prompt-injection defense) and the user prompt (redacted authorized
  evidence only — no raw logs/events/manifests). `ValidateProviderResult`
  enforces 8 rules; rejection is total — fabricated, out-of-scope or
  unauthorized citations discard the entire output. The AI cannot upgrade
  a candidate to confirmed cause, and cannot recommend a runbook outside
  the eligible M42 Action Catalog.
- **Golden fixtures (M43):** 10 validation scenarios (correct,
  insufficient, conflicting, prompt-injection, hidden-scope,
  fabricated-citation, ineligible-runbook, confirm-root-claim,
  empty-summary, no-citations). Each is a deterministic (provider result,
  authorized evidence, eligible codes, expected valid/invalid) pair.
- **Service (M43):** `Investigate` (read case + eligible codes, build
  prompt, call provider, validate, persist completed/failed),
  `GetInvestigation`, `ListByCase`, `ListRunbooks`. On provider failure →
  `failed`/`provider_error`; on validation failure →
  `failed`/`citation_rejected` (provider summary retained for audit).
- **HTTP routes (M43):** 4 routes under `/api/v1/aiops/investigator`
  (runbooks, list investigations, get investigation, generate
  investigation). The POST is the only write; it persists an
  investigation but never modifies the case/diagnosis/alert. Actor derived
  from the authenticated session.
- **Migration 000032 (M43):** `ai_investigations` table with CHECK
  constraints (status/tokens, completed-summary/completed-citations/
  failed-reason invariants), partial unique index
  `uq_ai_investigations_active`, and a FK to `correlation_cases(id)` ON
  DELETE CASCADE.
- **Tests (M43):** 44 tests (5 catalog + 4 provider/fixtures/18 subtests
  + 8 prompt + 15 service + 12 handler). Fast gate passed in 37.47s (31
  backend packages including `aiinvestigator`, 81 frontend tests/18
  files).
- **Deferred (M43):** real AI provider integration (Responses-compatible
  HTTP provider), provider budget/reservation enforcement, real PostgreSQL
  integration test, real-kind E2E, frontend UI, M44 safe-automation
  wiring.

### M42 Highlights

- **Data model (M42):** New `backend/internal/correlation` package with 4
  `ConfidenceClass` values (confirmed/candidate/contradicted/unknown), 3
  `CaseStatus` values (active/resolved/stale), `Case` with deterministic
  `case_key` (SHA-256 over cluster_id+resource_uid+rule_id+correlation_version),
  `SignalLink`, `ResourceLink`, `ChangeCandidate`, `ActionCandidate` (fixed
  codes from M19 catalog). `CorrelationVersion = "1.0"`.
- **Catalog (M42):** 6 V1 rules covering golden replay scenarios
  (rollout→pod_failure, rollout→unavailable_deployment, rollout→no_endpoints,
  maintenance→node_failure, pvc_pending→pod_failure, rollout→metric_breach).
  Fail-closed lookup; adding a rule is a contract change.
- **Engine (M42):** Pure, stateless `Correlate` — identical inputs + identical
  rule/correlation versions yield identical results. Explicit factors:
  `same_uid`, `topology_distance` (bidirectional BFS over M40 edges),
  `time_distance`, `change_symptom_rule`, `signal_freshness`,
  `signal_completeness`, `diagnosis_match`, `contradicting_signal`.
  `classifyConfidence` is a pure function; temporal proximity alone is never
  causality.
- **Golden fixtures (M42):** 9 replay scenarios + cold-start, each a
  deterministic (inputs, expected) pair. Replaying produces byte-identical
  case_keys and confidence.
- **Service (M42):** `CorrelateNamespace` (bounded lookback, idempotent
  persist), `GetCase`, `ListCases`, `ListTimeline`, `GetCaseGraph`,
  `ListActionCandidates` (derives `deployment.rollback` /
  `deployment.rollout_restart` — no execute endpoint).
- **HTTP routes (M42):** 6 read-only routes under
  `/api/v1/aiops/correlation` (rules, cases, timeline, case detail, graph,
  actions). Case correlation is internal, not HTTP-triggered.
- **Migration 000031 (M42):** `correlation_cases`,
  `correlation_signal_links`, `correlation_resource_links`,
  `correlation_change_candidates` with CHECK constraints and unique indexes.
- **Tests (M42):** 36 tests (5 catalog + 3 fixtures/10 subtests + 10 service
  + 9 handler). Fast gate passed in 63.26s (31 backend packages including
  `correlation`, 81 frontend tests/18 files).
- **Deferred (M42):** background correlation worker, signal-ingestion hook,
  real PostgreSQL integration test, real-kind E2E, frontend UI, M43/M44
  integration.

### M41 Highlights

- **Data model (M41):** New `backend/internal/slo` package with 3 `SLITemplate`
  values (`request_success_ratio`, `request_latency_target_ratio`,
  `workload_readiness`), 2 `MissingDataPolicy` values (`unavailable`,
  `fail_open`), 5 `EvaluationState` values (`healthy`, `burning_slow`,
  `burning_fast`, `breached`, `unavailable`), 3 `EvaluationCoverage` values
  (`complete`, `partial`, `unavailable`), `Definition` (versioned, enabled,
  bounded burn windows), `Evaluation` (append-only, deterministic). 31
  slo-package unit tests (14 evaluator + 13 service + 4 catalog).
- **Catalog:** `TemplateDescriptor` + compiled `catalog` map is the single
  source of truth for which templates exist, what they require and which
  missing-data policies they admit. `ValidateDefinition` is the only
  validation entry point. Adding a template is a contract change.
- **Evaluator:** `Evaluator.Evaluate` is pure: same Definition + same
  MetricsSource output → same Evaluation. Counter resets detected as
  monotonicity violations and handled as "counter went to 0". Sparse data
  → `CoveragePartial`; no samples → `CoverageUnavailable`. Clock boundaries
  inclusive `window_start`, exclusive `window_end`. Missing data fail-closed
  by default; only `workload_readiness` may fail-open with explicit operator
  opt-in, and even then `Coverage` remains `Unavailable` (auditable).
  `classifyState` precedence: breached > burning_fast > burning_slow >
  healthy. Zero error budget (objective == 1.0) handled explicitly.
- **Repository:** `GormRepository` with `ON CONFLICT DO NOTHING` for
  idempotent evaluation inserts, partial unique index
  `uq_slo_definitions_active` for at-most-one-active-definition.
  `NopRepository` for testing/disabled mode.
- **Service:** `CreateDefinition` stamps version=1. `PatchDefinition`
  requires actor and increments Version. `DeleteDefinition` marks
  `enabled=false` (row retained). `EvaluateSLO` looks up definition first
  (404 > 503 precedence), runs evaluator, persists even on unavailable
  (auditable fact), emits `BurnTransition` to `BurnAlertSink` only on state
  change. Sink is best-effort: failure does not rollback.
- **HTTP routes:** 8 routes under `/api/v1/aiops/slos`: `GET /templates`,
  `GET /`, `POST /` (SystemOpsAdmin), `GET /:id`, `PATCH /:id`
  (SystemOpsAdmin), `DELETE /:id` (SystemOpsAdmin),
  `POST /:id/evaluate` (SystemOpsAdmin), `GET /:id/evaluations`. Read-only
  routes open to any authenticated user; M35 scope enforced via cluster_id
  binding at create time and middleware on underlying Kubernetes resources.
- **OpenAPI:** Adds `slo` tag, 8 paths, 10 schemas. Enums, bounds and
  required fields match migration CHECK constraints and `ValidateDefinition`
  rules. Route contract test verifies bidirectional OpenAPI consistency.
- **Migration 000030:** `slo_definitions` table with CHECK constraints on
  template/policy/objective/window/burn bounds, partial unique active index,
  query indexes; `slo_evaluations` table with CHECK constraints on
  state/coverage/window/event-count/ratio bounds, query indexes. Paired
  down migration.

### M40 Highlights

- **Edge model (M40):** New `backend/internal/topology` package with 8
  `EdgeKind` values (Owns/Selects/RoutesTo/BackedBy/RunsOn/Mounts/Scales/
  ProtectedBy), 8 `DerivationMethod` values, `ResourceCitation` (cluster_id +
  kind + UID primary key; name-only marked incomplete), `Edge` with validity
  interval, `ChangeEvent` with confidence/source. 29 topology-package unit
  tests (13 collector + 11 normalizer + 5 service).
- **Collector:** `Snapshot` reads 8 resource types with bounded paging
  (1000-page safety cap); `DeriveEdges` deterministically derives all 8 edge
  kinds from exact observed evidence (OwnerReference, label selector,
  EndpointSlice, Ingress backend, nodeName, PVC mount, HPA scaleTargetRef,
  PDB selector). Same-name/temporal proximity never creates an edge.
- **Repository:** `GormRepository` with `ON CONFLICT DO UPDATE` for edge
  refresh and change-event idempotency. Partial unique index
  `uq_topology_edges_active` enforces at-most-one-active-edge. `NopRepository`
  for disabled/testing mode.
- **Service:** `CollectNamespace` (snapshot → derive → upsert → close stale),
  `CollectCluster`, `GetTopologyGraph` (nodes from edge endpoints +
  completeness), `GetChangeTimeline`, `IngestChangeEvent` (validated).
- **Change normalizer:** Pure mapping from `ChangePlanInput`/`AuditChangeInput`
  to `ChangeEvent`. Domain statuses normalized (succeeded/failed/expired/
  partial/awaiting_confirmation/executing → succeeded/failed/failed/partial/
  pending/pending). Confidence high for platform+audit_id, low otherwise.
- **Migration 000029:** `topology_edges` (partial unique active index, query
  indexes, CHECK constraints) + `change_events` (idempotent plan_id index,
  CHECK constraints). Paired down migration.
- **HTTP routes:** `GET /api/v1/aiops/topology/graph` and
  `GET /api/v1/aiops/topology/changes`. Read-only; require authentication;
  M35 scope filtering by middleware. Bounded limits (graph 500, timeline 200)
  with truncation disclosed.
- **ADR:** ADR 0055 records the six temporal-topology decisions.
- **Deferred:** background collection worker, plan-completion ingestion hook,
  real PostgreSQL integration test, real-kind E2E, frontend UI, retention
  worker.

### M39 Highlights

- **Signal model (M39):** New `backend/internal/signal` package with
  `Occurrence` envelope and `SignalDescriptor` catalog (28 signal codes across
  7 domains). Primary resource key is cluster_id + kind + UID; name-only is
  marked Incomplete. Fail-closed for unregistered signals. 22 signal-package
  unit tests + 9 HTTP handler tests.
- **Fingerprint dedup:** SHA256 over identity fields (excluding ObservedAt) +
  unique DB index + ON CONFLICT DO UPDATE ensures duplicate producer delivery
  yields one row.
- **Normalizers:** DiagnosisNormalizer (11 rules), AlertNormalizer,
  MetricBreachNormalizer, PostureNormalizer (4 codes), ChangeNormalizer
  (promotion/backup/maintenance/restore × succeeded/failed). Pure functions.
- **Service:** `Ingest`/`IngestBatch`/`List`/`Overview`/`CleanupRetention`.
  `SourceReader` interface for overview aggregation; `NopSourceReader` default.
- **Migration 000028:** `signal_occurrences` table with unique fingerprint
  index, query indexes, CHECK constraints.
- **HTTP routes:** `GET /api/v1/aiops/overview`, `GET /api/v1/aiops/signals`,
  `GET /api/v1/aiops/signals/catalog`. M35 scope filtering by middleware.
- **Configuration:** `SignalConfig` disabled by default.
- **ADR:** ADR 0054 records the six signal-model decisions.
- **Deferred:** concrete `SourceReader` adapter (M40), batch ingestion worker,
  real PostgreSQL integration test, frontend UI.

### M38 Highlights

- **CI (M38A):** `go test -race`, `golangci-lint@v2.12.2` with
  `.golangci.yml`, `pnpm lint` with the ESLint flat config, 50.0% coverage
  baseline and `oasdiff breaking --fail-on ERR` are now mandatory on every
  pull request. The real-kind E2E workflow covers M23-M31 in addition to
  diagnosis, fleet, search and M21-history.
- **Helm (M38B):** Official Helm 3 chart at `deploy/helm/aiops-platform/`
  with `Chart.yaml`, `values.yaml`, `values.schema.json` and nine templates.
  The chart never renders a Secret; ten Go contract tests guard its structure,
  values, schema and security baseline.
- **Supply chain (M38C):** Releases build `linux/amd64` + `linux/arm64` OCI
  images with `docker buildx`/QEMU, generate SPDX SBOMs with `syft v1.27.0`,
  and bundle the Helm chart, license allowlist and SHA256 manifest. The
  license allowlist (`docs/security/license-allowlist.json`) admits
  `MIT`/`ISC`/`BSD-2-Clause`/`BSD-3-Clause`/`Apache-2.0` only.
- **Docs:** `SECURITY.md` and `CHANGELOG.md` are tracked delivery assets;
  ADR 0051 records the seven decisions.

### M37 Highlights

- **Capability providers (M37A):** New `backend/internal/capability` package
  with `MetricsProvider` and `LogProvider` interfaces, Prometheus and Loki
  adapters, and `Nop*` defaults. Public APIs accept fixed template/query AST
  fields only — they never accept PromQL, LogQL or arbitrary labels. Provider
  endpoints and credentials are server-configured. 18 provider tests + 8
  HTTP handler tests.
- **Alert routing (M37B):** New `backend/internal/alertroute` package with
  route priority (1..100), exact cluster/rule/severity match, dedupe key,
  group/repeat interval, HTTPS webhook receiver, time-bounded silences
  (5m..7d, reason required, permanent forbidden), idempotent delivery with
  retry and dead-letter. Migration `000027` adds four tables. 40 service
  tests + 27 HTTP handler tests.
- **Configuration:** `CapabilityConfig` and `AlertRouteConfig` in
  `backend/internal/config/config.go` with fail-closed validation (HTTPS
  endpoints, bounded timeouts). Both disabled by default.
- **HTTP routes:** `GET /api/v1/capability/metrics` and
  `POST /api/v1/capability/logs` for M37A; 10 alert-route endpoints under
  `/api/v1/alert-routes/` for M37B. SystemOpsAdmin role required for
  mutations; deliveries restricted to SystemSecurityAudit.
- **ADR:** ADR 0053 records the six capability-plane decisions.
- **Deferred:** M37C (Gateway API evidence) and M37D (delivery metadata)
  deferred per ADR 0053 §4 until M40 demonstrates concrete need. Real
  Prometheus/Loki provider integration and real-kind E2E deferred pending
  external provider access.

### M36 Highlights

- **OIDC provider (M36A-M36C):** Full Authorization Code + PKCE S256 flow
  with HTTPS discovery validation, JWKS cache with TTL-based refresh and
  key rotation, ID token verification (signature, issuer, audience, nonce,
  expiry, MFA evidence), group-to-role mapping and browser-flow leak guard.
  OIDC remains disabled by default; when disabled, no OIDC route is
  registered.
- **Session management (M36D):** `AuthSessionIssuer` adapter delegates to
  `auth.Service.IssueSessionForUser`, sharing refresh-token rotation,
  `auth_version` revocation and audit with password login. Provider
  RP-initiated logout and break-glass drill recording with staleness
  tracking.
- **Synthetic IdP E2E (M36E):** `TestSyntheticIdPEndToEndLifecycle` (6
  ordered subtests) exercises discovery, JWKS, PKCE, ID-token verification,
  MFA evidence, session issuance and break-glass audit through real
  implementations against a synthetic HTTPS IdP.
- **HTTP wiring (M36F):** `GormIdentityResolver` resolves (issuer, subject)
  to prelinked local user; automatic email linking forbidden. OIDC HTTP
  handlers (`GET /login`, `GET /callback`, `POST /logout`) with fail-closed
  error mapping. `OIDC_AUTH_SESSION_SIGNING_KEY` (≥32 bytes) added to
  configuration. OpenAPI bidirectional parity preserved.
- **Migration:** `000026_external_identities` adds `external_identities`
  table with `(issuer, subject)` unique constraint.
- **ADR:** ADR 0052 records the production OIDC and MFA decisions.
- **Deferred:** Real organization IdP run, GORM `IdentityResolver`
  PostgreSQL integration test and frontend OIDC login button (externally
  gated).

### Final Gates

- L1 fast gate: `scripts/verify-fast.ps1 -Scope All` passed in 64s (M45
  baseline; 31 backend packages including `golden`, 81 frontend
  tests/18 files, Compose/Kustomize contracts)
- L2 full gate: `scripts/verify.ps1` passed in 97.68s
  (`.artifacts/verification/verify-20260731-015255.json`)
- L3 real-kind E2E: M27-M31 all passed with disposable clusters and cleanup;
  M38 adds M23-M31 to the hosted real-kind matrix
- Browser: 390x844 and 1280x720 passed without page overflow or warning/error logs
- Race: not run locally because `gcc` is unavailable; CI workflow now
  includes `go test -race` for hosted runs
- L4 remote CI: deferred (requires user-authorized push)

### M32 Audit Findings (Fixed)

- OpenAPI gap: 11 M28-M31 routes were missing from `docs/api/openapi.yaml`; fixed in this revision
- Contract test blind spot: `TestRegisteredRoutesMatchOpenAPI` did not inject Backup/Maintenance/NamespacePosture/Restore service stubs; fixed in this revision
- Migration 000024, bounded managed-cluster mutations, alert recovery window,
  real dry-run feasibility and responsive shared styles were aligned and tested

### Deferred External Gates (M26)

All M26 external gates are `deferred` with owner/reason/re-entry condition:

- Hosted CI green + tag/release: requires user-authorized push to private remote
- OIDC/MFA production run: requires organization-approved identity provider inputs
  (M36 implementation is complete; only real IdP validation remains externally gated)
- PITR: requires physical/WAL PITR infrastructure
- HA failover/failback: requires HA infrastructure

See `docs/changes/2026-07-30-m32-formal-closure.md` §M26 External Gate Disposition.

### Migrations Applied

Migrations 000020-000026 are applied in the development PostgreSQL instance.
There are 26 matching up/down pairs; 000026 is the latest applied migration.

### Stable Baseline (M21-M25)

The M21-M25 baseline remains accepted at
`.artifacts/verification/verify-20260730-080851.json`. Fresh real-kind evidence
for M21/M23/M24/M25 is archived under `.artifacts/`.

Everything below this point is retained only as historical phase narrative;
the 2026-07-31 section above is authoritative and supersedes older current/
deferred statements.
Local `main` contains the reviewed M21-M25 implementation and tracks
`https://github.com/guiyi-labs/aiops-platform.git`. The release candidate has
passed the fresh full repository gate at
`.artifacts/verification/verify-20260730-080851.json` (121.79 seconds), with
all Go packages, 17 Vitest files/73 tests, the production frontend build,
current backend/frontend images, three healthy Compose services, Kustomize
contracts and direct/proxied readiness.

Fresh real-kind evidence is:

- M21 history/window/outage/recovery/restart:
  `.artifacts/m21-history-kind/m21-history-kind-20260730-080558.json`.
- M23 image update/exact rollback lifecycle:
  `.artifacts/m23-release-lifecycle-kind/m23-release-lifecycle-kind-20260729-234238.json`.
- M24 two-cluster fixed promotion, including `dependencies=1`:
  `.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-074812.json`.
- M25 installed/unavailable Velero inventory and read-only RBAC:
  `.artifacts/m25-workload-protection-kind/m25-workload-protection-kind-20260730-075311.json`.

Migrations 18 and 19 are applied in the development PostgreSQL instance. The
current public surface, OpenAPI, typed frontend clients, least-privilege RBAC
and acceptance scripts are aligned. Generated `.artifacts` and
`frontend/dist` remain ignored and must not be committed. See
`docs/changes/2026-07-30-m21-m25-baseline-alignment.md` for the review fixes
and evidence ledger.

- Last updated: 2026-07-29
- Repository: `<repo-root>/aiops-platform`
- Git state: local `main` tracks private remote `https://github.com/guiyi-labs/aiops-platform.git`; M21 Phase 3 is accepted at `cf20c66c588e35b9a29d492661bc99a8e1cb498b` with hosted CI run `30411146049`; Phases 1 and 2 plus M20 Phase 12 remain accepted and archived
- Current milestone: M21 bounded historical observability and alert evidence is in progress; Phase 3 exposes the accepted sparse PostgreSQL history contract through one authenticated exact-series route, while provider-specific identity/recovery work remains organization-gated

The 2026-07-28 KRM/Ratel reassessment found that the platform is already
stronger in diagnosis evidence, credential safety, controlled mutation, audit
and delivery verification, but is visibly narrower in historical operations,
Deployment release lifecycle, cross-cluster promotion and cluster-workload
backup. The accepted M21-M26 sequence and explicit non-goals are archived in
`docs/references/krm-ratel-gap-analysis.md` and
`docs/changes/2026-07-28-product-roadmap-reprioritization.md`.

M21 Phase 1 adds ADR 0034, migration 17 and `internal/metricshistory`. The
accepted envelope is seven-day default/30-day maximum retention, at most 1,800
canonical samples per collection, one exact cluster/resource/container/metric
series per query, a 24-hour window, 1,440 returned points and bounded expiry
cleanup. Collection runs retain Node/Pod result and coverage independently;
queries report sparse points plus explicit missing/unavailable/timeout/failure
counts and never manufacture zeroes. Local Go full-package tests, a real
PostgreSQL migration, database constraints/index inspection, backend image
rebuild and readiness HTTP check passed. Hosted Backend, Frontend, Manifests
and the 7m11s Compose runtime job also passed, including isolated PostgreSQL
backup/restore with migration 17. The phase is archived in
`docs/changes/2026-07-28-m21-bounded-metrics-history-foundation.md`.

M21 Phase 2 adds ADR 0035 and `internal/metricshistory.Collector`. The API
process now samples enabled clusters every minute by default, with stable ID
ordering, a 20-cluster cap, four-way cluster concurrency, a ten-second
per-cluster timeout and parallel Node/Pod reads. Official Kubernetes Quantity
parsing stores exact CPU nanocores and memory bytes; malformed, negative or
overflowing values fail one source atomically. Round-robin Node/Pod bundle
allocation preserves both sources under the 1,800-point cap, and six stable
codes record API absence, timeout, request, quantity, payload and limit outcomes
without persisting raw upstream errors. Cleanup runs immediately and hourly,
one bounded repository batch per tick. The server cancels and waits for both
background services before closing PostgreSQL. The phase is archived in
`docs/changes/2026-07-28-m21-bounded-background-metrics-collector.md`.
The 782.71-second local repository gate passed 195 Go `Test*` entries and five
backend targets, 14 Vitest files / 59 tests, production frontend and both
Docker image builds, three healthy Compose services, Kustomize `16/5/22/3`
and direct/proxied readiness. Evidence is
`.artifacts/verification/verify-20260728-223526.json`. Hosted Backend,
Frontend and Manifests plus the 6m11s Compose runtime job passed in run
`30369559322`, including all isolated drills, random-configuration startup,
service health, direct/proxied HTTP, sanitized upload and teardown with the
collector enabled.

M21 Phase 3 adds ADR 0036 and authenticated
`GET /api/v1/clusters/{cluster_id}/metrics/history`. The route accepts only one
exact Node or Pod CPU/memory series, explicit RFC3339 bounds, at most 24 hours
and 1,440 points. It preserves sparse points and explicit collection coverage,
never fills gaps with zero and hides repository failures behind stable HTTP
codes. OpenAPI and Gin route parity are mechanically checked. The real
PostgreSQL E2E at
`.artifacts/metrics-history-e2e/metrics-history-e2e-20260729-081759.json`
proved cross-cluster and exact-series isolation, two ordered points across
three collections with one missing sample, backend restart durability and
complete fixture cleanup. The 115.83-second full local gate passed Go vet, 199
Go `Test*` entries, five backend build targets, 14 Vitest files / 59 tests,
the frontend production build, both Docker images, three healthy Compose
services, Kustomize `16/5/22/3` and direct/proxied readiness. Evidence is
`.artifacts/verification/verify-20260729-082024.json`. The implementation
revision is `cf20c66c588e35b9a29d492661bc99a8e1cb498b`. Hosted CI run
`30411146049` passed Backend, Frontend and Manifests plus the 8m21s Compose
runtime, including the authenticated history isolation/restart drill,
direct/proxied HTTP, sanitized evidence upload and unconditional teardown.

M20 Phase 12 adds ADR 0033, `/app/recovery-readiness`, an unresolved policy
template and a network-disabled gate consuming the newest real logical-restore
evidence. Fifteen checks cover owners, RPO/RTO/MTD, encrypted off-cluster
immutable copies, backup frequency, PITR, HA or named 180-day risk acceptance,
drills, cutover/rollback, approvals, evidence freshness/integrity/cleanup and a
mandatory production-validation boundary. Inputs are strict and limited to 1
MiB. No backup credential, payload, WAL material, database URL or HTTP endpoint
is introduced.

The final logical drill at 2026-07-28 17:44 +08:00 applied 16 migrations,
created a custom archive, destroyed the source before a fresh-target restore,
preserved all synthetic snapshots with zero invalid foreign keys and passed all
four cleanup assertions. Evidence is
`.artifacts/postgres-recovery/postgres-recovery-20260728-174419.json`. The
recovery gate then accepted all 15 controls, rejected one-copy storage, stale
evidence, a retained dump and incomplete cleanup, and removed its image/copied
inputs. Evidence is
`.artifacts/recovery-readiness/recovery-readiness-20260728-174509.json`.
`production_recovery_validated` remains false by design. Actionlint 1.7.7
returned zero findings. The 199.35-second full local gate passed 175 Go `Test*`
entries and five backend targets, 14 Vitest files / 59 tests, production
frontend build, three healthy services, Kustomize 16/5/22/3 and runtime HTTP
checks. Evidence is `.artifacts/verification/verify-20260728-175233.json`.
Hosted CI run `30348664880` passed all four jobs at `0baf858`, including the
network-disabled recovery gate after a fresh PostgreSQL logical restore, all
existing isolated drills, random-production-config Compose health, sanitized
evidence upload and unconditional teardown.

M20 Phase 11 adds ADR 0032, the strict offline `/app/identity-readiness`
command, a policy template and a network-disabled synthetic drill. Fourteen
checks cover accountable ownership, canonical HTTPS issuer/endpoints/redirects,
Authorization Code + PKCE S256, scopes, asymmetric signing/JWKS structure,
explicit claims, identity-provider MFA evidence, immutable-subject prelinking,
bounded sessions/logout and offline break-glass controls. Unknown fields and
files over 1 MiB fail closed; the schema has no client-secret field and the
command adds no HTTP route, database state or network request.

The final drill passed at 2026-07-28 16:54 +08:00, accepted the complete 14-check
synthetic contract, rejected issuer/PKCE and MFA/email-linking downgrades and
deleted its temporary image and snapshots. Evidence is
`.artifacts/identity-readiness/identity-readiness-20260728-165405.json`.
Actionlint 1.7.7 returned zero findings. The 300.97-second full local gate then
passed 171 Go `Test*` entries and four backend targets, 14 Vitest files / 59
tests, production frontend build, three healthy services, Kustomize 16/5/22/3
and runtime HTTP checks. Evidence is
`.artifacts/verification/verify-20260728-165939.json`. Hosted CI run
`30345051371` passed all four jobs at `216eb81`, including the network-disabled
identity gate, all three isolated database drills, random-production-config
Compose health, sanitized evidence upload and unconditional teardown.

M20 Phase 10 adds ADR 0031, the offline `/app/audit-archive` command and an
isolated PostgreSQL drill. Archive creation requires an explicit ID range,
output and Ed25519 private-key file, checks a reviewed 1..10000 maximum before
writing, and emits canonical JSON plus a detached signed manifest. Verification
requires a separately supplied trusted public key and checks signer identity,
signature, exact payload SHA-256, metadata and ordering. The isolated run at
2026-07-28 15:40 +08:00 passed two-row signing/verification, three-row overflow
refusal with no output, one-byte tamper rejection and all five cleanup
assertions. Evidence is
`.artifacts/audit-archive/audit-archive-20260728-154047.json`. The 361.34-second
full local gate passed all backend packages and three binaries, 167 Go `Test*`
entries, 14 Vitest files / 59 tests, production build, three healthy services,
Kustomize 16/5/22/3 and runtime HTTP checks. Evidence is
`.artifacts/verification/verify-20260728-153059.json`. Hosted CI run
`30340088789` passed all four jobs at `c144957`, including all three isolated
database drills, random-production-config Compose health, sanitized evidence
upload and unconditional teardown.

Hosted runs `30338972042` and `30339580960` passed Backend, Frontend, Manifests
and the existing credential drill but intentionally remain failed evidence.
The first exposed Linux PowerShell null/empty behavior in the process-environment
cleanup assertion; the second then exposed the non-root image UID versus
runner-owned bind-mount permission boundary. Actual disposable resources were
removed in both attempts. Cleanup comparison is now null/empty-normalized and
Linux command containers use the non-root runner UID/GID for the temporary
mount. The accepted replacement is run `30340088789`.

M20 Phase 9 adds ADR 0030, migration 000016, an active-plus-legacy AES-GCM
keyring and the default-dry-run `/app/credential-reencrypt` command. Apply is
explicit, bounded to 100 rows per transaction and 10000 reviewed candidates,
serialized by a PostgreSQL advisory lock and audited with versions, counts and
sanitized error codes only. The isolated 2026-07-28 run created two real v1
credentials through the API, proved no-write dry-run and whole-batch rollback,
converted both to v2, then proved a v2-only backend decrypt path. Evidence is
`.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`;
all dedicated containers, network, image and process environment were cleaned.
The full local gate passed in 288.9 seconds with 163 Go `Test*` entries, both
backend binaries, 14 Vitest files / 59 tests, frontend production build, three
healthy Compose services, Kustomize 16/5/22/3 and runtime HTTP checks. Evidence
is `.artifacts/verification/verify-20260728-141111.json`. Hosted CI run
`30334216631` passed all four jobs at revision `151bc7e`, including the isolated
re-encryption and PostgreSQL recovery drills, random-production-config Compose
health, sanitized evidence upload and unconditional teardown.

M20 Phase 8 adds ADR 0029, the recovery runbook and
`scripts/e2e-postgres-backup-restore.ps1`. The script starts an isolated
PostgreSQL 17 source with no host port, applies all 15 migrations, inserts
synthetic relational fixtures, creates a custom-format dump, destroys the
source and restores a fresh target. The 2026-07-28 local run preserved all
expected migration/table/encrypted-byte invariants, found zero invalid foreign
keys and removed both containers and temporary backup material. The regular CI
runtime job now runs this drill and uploads only sanitized JSON evidence. This
does not claim production retention, PITR, RPO/RTO, PVC recovery or HA.
The post-change full local gate passed in 278.81 seconds with all backend
packages, 14 Vitest files / 59 tests, production build, three healthy Compose
services, Kustomize 16/5/22/3 and runtime HTTP checks. Evidence is
`.artifacts/verification/verify-20260728-125500.json`; actionlint 1.7.7 also
returned zero findings.
Hosted CI run `30331346283` passed all four jobs for the final Phase 8 archive,
including the PostgreSQL source-to-fresh-target restore on Ubuntu PowerShell,
the independent Compose runtime health checks, sanitized evidence upload and
unconditional teardown.

M20 Phase 7 reviewed Dependabot PRs #1, #2, #5 and #6. The Actions and Go
updates were merged after all four hosted checks passed; the Vue and vue-tsc
patch update was then merged and the combined `main` revision passed run
`30328283896`. The multi-major frontend PR #3 was closed without merge, and the
duplicate pnpm PR #4 was superseded by the reviewed `pnpm/action-setup` v6
commit. `.github/dependabot.yml` now groups only minor/patch updates and the
contract suite verifies all three ecosystem policies.

M20 Phase 6 adds ADR 0028, `.github/workflows/ci.yml`, `release.yml` and
`real-kind-e2e.yml`, plus grouped weekly Dependabot updates. Pull requests use
read-only permission and no repository secret; backend, frontend and manifest
jobs gate an ephemeral Compose runtime with random credentials and guaranteed
teardown. Release rehearsals validate `vX.Y.Z` and only package; tagged runs
reuse the complete CI, produce checksummed versioned image/source/API/license
assets and require `gh release create --verify-tag`. Physical kind suites run
only weekly or manually on a dedicated `[self-hosted, windows, x64,
aiops-kind]` runner and include the disposable diagnosis, fleet and search
scripts. All marketplace actions are pinned to commit SHAs. The YAML/security
contract test passed and actionlint 1.7.7 returned zero findings. At Phase 6
acceptance no commit, remote workflow, tag, release or registry push was
created. See
`docs/changes/2026-07-28-versioned-ci-release-pipeline.md` and
`docs/ci-release.md`.

The M20 Phase 6 post-archive gate passed at 2026-07-28 10:07:52 +08:00 in
180.85 seconds. Evidence is
`.artifacts/verification/verify-20260728-100752.json`: 152 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

The reviewed local root baseline was then created on `main` as
`2d46588f8c15ab626703e92eccc35b4de8b53ab2` with author and committer
`guiyi-labs <277616126+guiyi-labs@users.noreply.github.com>`. It contains 368 files and excludes `.env`, local
tools, evidence, dependency/build output, `backend/server.exe` and frontend
TypeScript build metadata. The commit-bound full gate passed at 2026-07-28
10:21:10 +08:00 in 177.39 seconds with evidence at
`.artifacts/verification/verify-20260728-102110.json`. No remote, tag or
release was created.

The private GitHub remote was subsequently created at
`https://github.com/guiyi-labs/aiops-platform`. The first push exposed one
pre-existing `gofmt` mismatch in the workflow contract test; revision
`648aea6c94fbc29fbf21d1f799df29880099d454` corrected it. Hosted CI run
`30325194933` then passed on 2026-07-28 at 11:14:24 +08:00: Backend,
Frontend, Manifests and the ephemeral Compose runtime all succeeded, including
runtime health checks, sanitized artifact upload and guaranteed teardown. The
initial grouped Dependabot pull requests were reviewed; major frontend
migrations remain intentionally deferred as separate future work.

M20 Phase 5 adds `scripts/e2e-global-search-kind.ps1` without changing ADR 0026
or the search API. The accepted run created two physically distinct Kubernetes
v1.34.0 kind clusters plus isolated PostgreSQL/backend, returned nine stable
Pod/Deployment/Service/Ingress matches with complete 2/2 cluster coverage,
verified canonical kind selection, `cluster_limit=1` and global truncation,
then produced four localized `TIMEOUT` failures, recovered to nine complete
results and produced four localized `QUERY_FAILED` failures after stopping the
second control plane. The healthy peer retained four usable results in both
fault states. All fixed-kind reads were allowed and creates denied on both
clusters. All eight cleanup assertions passed and the pre-existing
`aiops-test` cluster was preserved. Evidence is
`.artifacts/search-e2e/search-e2e-20260727-225358.json`; see
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`.

The M20 Phase 5 final gate passed at 2026-07-27 23:02:04 +08:00 in
158.94 seconds. Evidence is
`.artifacts/verification/verify-20260727-230204.json`: 151 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

M20 Phase 4 adds ADR 0027, migration 000015 and authenticated CRUD under
`/api/v1/fleet/resources/search/filters`. Records belong to the current actor,
are capped at 20 under a per-user PostgreSQL advisory lock, use
case-insensitive names and persist only the Phase 3 query, Namespace and fixed
kind subset. List projects stale schema/query records as incompatible; they may
be renamed, completely overwritten or deleted, but not applied. The search UI
supports save/apply/rename/overwrite/delete and keeps incompatible repair and
the 20-item limit explicit. Source coverage is now 151 Go `Test*` entries and
14 Vitest files / 59 tests. PostgreSQL/API acceptance produced exactly 20
successes and two 409 conflicts from 22 concurrent creates, then removed all
test rows. Browser save, rename, overwrite, apply and URL linkage passed at
desktop and mobile widths; document widths were 1265/1265 and 375/375, and the
mobile 760px table remained inside its 279px scroller with no warning/error
logs. The browser controller reached the native delete confirmation but timed
out while accepting it, so final cleanup used the independently accepted
DELETE API and the UI confirmation is not overstated. See
`docs/changes/2026-07-27-user-owned-global-search-filters.md`.

The M20 Phase 4 final gate passed at 2026-07-27 22:27:53 +08:00 in
351.1 seconds. Evidence is
`.artifacts/verification/verify-20260727-222753.json`: 151 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

M20 Phase 3 remains accepted with 140 Go test entries and 14 Vitest files / 58
tests. Its final gate passed at 2026-07-27 21:03:08 +08:00 in 168.62 seconds;
evidence is `.artifacts/verification/verify-20260727-210308.json`. See
`docs/changes/2026-07-27-bounded-global-resource-search.md`.

The M20 Phase 2 gate passed at 2026-07-27 19:37:11 +08:00 with evidence at
`.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json`. It created two distinct
Kubernetes v1.34.0 kind clusters plus an isolated PostgreSQL/backend runtime,
matched direct and fleet Node/Pod/Deployment/Event totals, verified ID ordering,
`limit=1`, 401/400 and read-only RBAC, then proved `timed_out` at 4003ms,
recovery and `unavailable` isolation. All eight cleanup checks passed and the
pre-existing `aiops-test` cluster was preserved. No retained password, database
or platform record was used.

The post-archive full gate passed at 2026-07-27 19:47:24 +08:00 with evidence
at `.artifacts/verification/verify-20260727-194724.json` (223.18 seconds). Go
vet/all packages/server build, frontend typecheck plus 13 Vitest files / 57
tests and production build passed. Kustomize remained 16/5/22/3 and all three
Compose services plus backend/frontend/proxy runtime checks were healthy.

The M20 Phase 1 full gate passed at 2026-07-27 19:01:33 +08:00 with evidence
at `.artifacts/verification/verify-20260727-190133.json` (104.53 seconds, 133
Go test entries, 13 Vitest files / 57 tests, Kustomize 16/5/22/3 and three
healthy Compose services). Rebuilt Dashboard acceptance used two enabled
platform records over the retained real kind endpoint: one unavailable record
was isolated without hiding the current 1/1 Node, 12/15 Pod, 5/7 Deployment
and 10 Warning result. Desktop 1280x720 and mobile 390x844 had zero document
overflow; the mobile fleet table scrolled only in its 277px container and
browser warning/error logs were empty. This is not claimed as two physically
distinct Kubernetes clusters.

The M19 full gate passed at 2026-07-27 18:04:28 +08:00 with evidence at
`.artifacts/verification/verify-20260727-180428.json` (143.85 seconds, 128 Go
test entries, 12 Vitest files / 56 tests, Kustomize 16/5/22/3 and three healthy
Compose services). The M19 real-kind evidence is
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json`; it passed scale/replay,
CronJob resume/suspend, namespaced RBAC and fixture restoration. The retained demo is
`demo-kind-20260727-165016` (platform cluster ID 39), with Metrics samples and
all seven expected diagnosis IDs. The previous M17 gate remains archived at
`.artifacts/verification/verify-20260727-155239.json`; the M9 real-kind evidence is
`.artifacts/diagnosis-e2e/diagnosis-e2e-20260726-193724.json`, and the change
record is `docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`.
The M10 implementation and responsive browser evidence are archived in
`docs/changes/2026-07-26-event-center-ui-unification.md`.
The M11 cockpit, topology, reference notes and real-data browser evidence are
archived in `docs/changes/2026-07-27-operations-cockpit-resource-topology.md`.
The M12 classified workbench, resource detail contracts and deep-link browser
evidence are archived in `docs/changes/2026-07-27-deep-link-resource-workbench.md`.
The M13 extended resource contracts, sanitization, related-event UI and browser
evidence are archived in `docs/changes/2026-07-27-expanded-read-only-resource-workbench.md`.
The M14 EndpointSlice list contract, complete traffic/workload topology and
responsive real-data evidence are archived in
`docs/changes/2026-07-27-complete-ingress-backend-topology.md`.
The M15 fixed Node/Pod Metrics contracts, optional capability behavior and
responsive Dashboard evidence are archived in
`docs/changes/2026-07-27-real-resource-metrics-foundation.md`.
The M16 Metrics Server fixture, real utilization, consumer ranking and
available-path browser evidence are archived in
`docs/changes/2026-07-27-real-metrics-utilization-consumers.md`.
The M17 fixed workload/policy contracts, Secret threat boundary, real-kind
fixtures and responsive workbench evidence are archived in
`docs/changes/2026-07-27-common-workload-policy-coverage.md`.

## Latest Evidence-Based Diagnosis Expansion

M18 adds `node.pressure.v1`, `persistentvolumeclaim.pending.v1`,
`horizontalpodautoscaler.saturated.v1` and `ingress.backend_unavailable.v1`.
The diagnosis endpoint, OpenAPI, frontend API and Workloads actions use the
same fixed resource contract. Replayable positive/negative fixtures are stored
in `backend/internal/diagnosis/testdata/m18-fixtures.json`.

The real kind run passed all seven diagnoses and retained cluster ID 39. It
created a synthetic Ready+MemoryPressure Node only for the diagnosis call and
deleted it in `finally`; the PVC Warning Event is linked by exact UID and the
HPA status is patched immediately before reading because the controller may
overwrite it. M18 deliberately does not claim sustained restart behavior: a
single Pod snapshot and cumulative restart count are not a time window.
See `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`.

## Stable Baseline

The functional MVP is complete: authentication/RBAC and user management, encrypted cluster onboarding, bounded Kubernetes reads, eleven deterministic diagnosis rules, workflow/SLA/assignment, append-only audit with safe CSV export, and cited AI explanations with runtime guardrails and quality feedback.

M8 extends the deterministic read-only diagnosis surface to Node and Deployment.
`node.not_ready.v1` retains Node Conditions, and
`deployment.replicas_unavailable.v1` retains desired/current readiness counters.
The Workloads page exposes both actions. M9 validates both rules against a
disposable real kind v1.34.0 cluster using only observer RBAC. The final gate passed all Go packages,
frontend typecheck plus 12 Vitest files / 55 tests, production and Docker image
builds, Compose health, Kustomize 16/5/22/3 and runtime HTTP checks.

## Latest Common Workload and Policy Coverage

M17 adds fixed bounded list/detail contracts for StatefulSet, DaemonSet,
ReplicaSet, Job, CronJob, HPA, ResourceQuota, LimitRange and Secret. The API,
OpenAPI document, route drift tests and target-cluster observer RBAC move
together; no arbitrary GVK, API path, YAML or write proxy was introduced. New
Pod templates expose only container names/images, while HPA selectors and
behavior internals are omitted.

Secret has a separate public model that exposes only minimal metadata, type,
immutable and sorted data key names. Raw response verification found none of
the fixture value, annotation bait, `data`, labels or annotations. This is an
application-model boundary, not field-level Kubernetes authorization: the
ServiceAccount can read raw Secret objects because Kubernetes RBAC cannot
grant key-name-only access. Production use therefore requires explicit threat
acceptance and strict protection of the platform identity/runtime.

The retained kind v1.34.0 environment is `demo-kind-20260727-152828`, platform
cluster ID 35, with Metrics Server v0.8.0 and a one-hour credential created
around 15:28 +08:00. All nine list/detail fixtures passed; counts were
1/1/11/1/1/1/1/1/1 at E2E capture time for StatefulSet/DaemonSet/ReplicaSet/
Job/CronJob/HPA/ResourceQuota/LimitRange/Secret. RBAC returned Secret list=yes,
Secret create=no and HPA list=yes. The downstream three diagnoses, remediation
execution/idempotent replay and real Node/Pod metrics also passed.

Desktop 1280x720 validated the four categories, eight workload tabs, exact
deep links, representative details and zero document overflow. Mobile 390x844
kept categories and kinds in two-column grids, contained wide tables inside the
resource panel, and used a 375px full-width drawer. Browser validation exposed
and fixed an M17 Condition DOM/grid mismatch that made HPA messages overlap on
mobile; the corrected cards are 326px wide, messages are 281px wide, and the
browser warning/error log is empty. Screenshots are under ignored
`.artifacts/browser-m17`.

## Latest Metrics Available Path

M16 keeps Metrics Server optional while adding a checksum-pinned v0.8.0 kind
fixture for the real available path. The upstream manifest remains
byte-equivalent and is verified against SHA-256
`ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b`.
The local runtime patch uses the reachable pinned mirror and
`--kubelet-insecure-tls` only for kind; neither change enters normal platform or
managed-cluster Kustomize bases.

Dashboard matches Node Metrics to the same Node's `status.allocatable`, so CPU
and memory percentages are emitted only with valid real denominators. Pod
Metrics are ranked independently by CPU or memory, bounded to five visible
consumers and labeled with loaded/total sample coverage. Every row preserves
cluster, kind, Namespace and name in the `/workloads` deep link.

The retained kind v1.34.0 environment is `demo-kind-20260727-142712`, platform
cluster ID 34, with Metrics Server v0.8.0 and a one-hour credential created
around 14:27 +08:00. Direct and platform reads both returned 1 Node and 12 Pods;
the downstream diagnosis/remediation gate also passed. Desktop 1280x720 showed
CPU 479m/3.0%, memory 911.4 MiB/11.7% and 12/12 Pod coverage. Memory ranking and
the exact Pod detail deep link passed. Mobile 390x844 remained 375/375 with a
single-column ranking, no overflowing elements and no browser warning/error
logs.

## Latest Real Resource Metrics Foundation

M15 adds fixed bounded Node and Pod Metrics routes under each explicit cluster
ID. Public responses retain only metadata, timestamp, window, CPU/memory usage
and Pod container names. The gateway maps an absent metrics.k8s.io API to
`424 METRICS_API_UNAVAILABLE`; it does not return zeros or hide the failure as
an empty list.

Dashboard loads Metrics independently from core Node, Pod, Deployment,
Service, Event and diagnosis data. CPU and memory cards show absolute Node
totals plus Pod totals only when real quantities exist. The parser covers CPU
`n/u/m/core` and binary/decimal memory units. No utilization percentage is
claimed without a real denominator.

The retained kind v1.34.0 environment is `demo-kind-20260727-134215`, platform
cluster ID 33, with a one-hour credential created around 13:42 +08:00. Metrics
Server is absent: core Node returned 200/1 while both Metrics routes returned
424. SubjectAccessReview confirmed get/list=true and create=false for metrics.
Desktop 1280x720 remained 1265/1265 and mobile 390x844 remained 375/375 with no
browser warning/error logs.

## Latest Complete Ingress Backend Topology

M14 adds the fixed bounded `GET /clusters/{cluster_id}/endpointslices` contract.
The public model retains only metadata, address type, port identity, endpoint
conditions/node/targetRef and the Service identity derived from the standard
label. Empty endpoint collections are normalized, and arbitrary discovery
paths or writes remain impossible.

`/topology` now renders Ingress, Service, EndpointSlice, Pod and Deployment.
Relationships require exact same-Namespace backend names, standard Service
labels or Pod targetRefs. Service selector fallback runs only when no matching
EndpointSlice exists. Selecting the real `healthy-nginx` Ingress highlighted
exactly one Ingress, Service, EndpointSlice, Pod and Deployment.

The retained kind v1.34.0 environment is `demo-kind-20260727-130453`, platform
cluster ID 32, with a one-hour credential created around 13:04 +08:00. Desktop
1280 validation had document and topology canvas overflow at zero. At 390x844,
the document remained 375/375 while the 928px topology scrolled only inside its
277px canvas. The browser run exposed and fixed a null EndpointSlice collection
white-screen regression; the current asset has no warning/error logs.

## Latest Expanded Read-Only Resource Workbench

M13 adds fixed list/detail contracts for Ingress, PersistentVolumeClaim,
StorageClass and sanitized ConfigMap metadata/key names. ConfigMap values and
StorageClass parameters cannot enter public response models. `/workloads`
groups eight resource types under workload, network, storage and configuration
categories while retaining the existing path and deep-link query contract.

All eight detail drawers load exact involvedObject Events independently from
the resource detail, so Event failure does not hide the resource. The retained
kind v1.34.0 browser run validated the new four types, PVC
WaitForFirstConsumer and Pod ImagePullBackOff Events, ConfigMap value absence,
StorageClass parameter absence and responsive 390x844 layout with no document
overflow or browser warning/error logs.

The target observer role remains `get/list` only. The demo now renders 10
resources and the retained environment is `demo-kind-20260727-114759` with a
one-hour credential. Real-kind demo preparation also exposed and fixed the
generic Pending rule taking precedence over ImagePullBackOff; all three demo
diagnoses and idempotent remediation passed after the fix.

## Latest Deep-Link Resource Workbench

M12 replaces the long mixed Workloads page with Pod, Deployment, Service and
Node inventory tabs. The server now exposes fixed GET details for Node,
Deployment and Service alongside the existing Pod detail route; all paths are
registered in Gin and OpenAPI and remain within the bounded read-only gateway.

Resource selection is encoded as `cluster/kind/namespace/name` query state.
Refresh restores the selected drawer, closing removes resource selection, and
Dashboard/Topology preserve cluster and resource context when navigating into
the workbench. The retained kind cluster returned 14 Pods, 6 Deployments,
4 Services and 1 Node. All four live detail workflows, topology navigation and
direct-link refresh passed. Desktop and 390x844 checks had no page-level
overflow or browser warning/error logs; the mobile table scrolled only inside
its panel and the detail drawer occupied the complete 375px document width.

## Latest Operations Cockpit and Resource Topology

M11 turns Dashboard into a live selected-cluster cockpit backed by existing
bounded Node, Pod, Deployment, Service and Event reads. It adds six operational
KPIs, health meters, recent Warning signals and direct navigation into topology,
resources, events and diagnoses. `/topology` derives Service -> Pod <- Deployment
relationships only from complete label-selector matches in the same Namespace;
empty selectors and cross-Namespace lookalikes never produce links.

The retained kind v1.34.0 demo showed 1/1 Ready Node, 11/14 Healthy Pods, 4/6
Available Deployments, 4 Services and 19 Warning Events. Selecting the healthy
Service highlighted exactly one Service, one Pod and one Deployment. Desktop
and 390x844 browser checks passed with no page-level horizontal overflow or
browser warning/error logs. The temporary viewport override was reset.

## Latest Event Center and UI Unification

M10 adds `/events` as the real Kubernetes Event Center for every authenticated
role. It supports cluster, Namespace, type, resource kind and resource-name
filters, prioritizes modern Event series timestamps/counts and exposes a safe
detail drawer. `/notifications` remains the diagnosis webhook outbox and is now
named Notification Delivery, preserving its administrator/auditor boundary.

Dashboard now consumes the shared `ConsoleLayout`; a topbar actions slot keeps
its refresh command without duplicating navigation, account security or logout.
The retained kind demo returned 11 Events (5 Warning, 6 Normal, 3 resources).
Desktop 1440x1000 and mobile 390x844 checks passed for both Events and
Dashboard: document-level horizontal overflow remained zero, the wide Event
table scrolled only inside its panel, the mobile drawer was viewport-width with
single-column fields, and browser logs were empty. Focused verification passed
frontend typecheck, 8 Vitest files / 28 tests, production build and backend
Kubernetes/HTTP packages. Exact scope is archived in
`docs/changes/2026-07-26-event-center-ui-unification.md`.

## Latest Node/Deployment kind Validation

M9 adds a separate `deploy/diagnosis-e2e` fixture set and
`scripts/e2e-diagnosis-kind.ps1`. A timestamped kind v0.30.0 cluster running
Kubernetes v1.34.0 produced a synthetic Node with Ready=False and a Deployment
with desired/current/ready/available/unavailable counts of 2/2/0/0/2. The
platform matched `node.not_ready.v1` and
`deployment.replicas_unavailable.v1`, persisted 2 Node Condition evidence rows
and 1 Deployment status evidence row, and confirmed RBAC yes/yes/no/no for
list Nodes, get Deployments, patch Deployments and patch Nodes.

The script deleted the temporary platform cluster and its diagnosis records,
the kind cluster, kubeconfig and status-patch file in `finally`; `aiops-test`
remained present. The retained defense demo fixtures and three diagnosis
scenarios were not modified. Exact results are archived in
`docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`.

## Latest Delivery Packaging

M5 is complete. `scripts/verify.ps1` provides the one-command code/build/runtime
gate and `scripts/e2e-kind.ps1` provides the repeatable real Kubernetes
diagnosis/remediation gate. The latter uses a one-hour ServiceAccount token only
in memory, deletes its temporary platform cluster in `finally`, and writes only
sanitized evidence under ignored `.artifacts`.

Fresh 2026-07-26 results passed all backend packages with Go 1.25, frontend
typecheck plus 8 Vitest files / 26 tests, both production builds, Compose health,
16/5/7 Kustomize resources and backend/frontend/proxy HTTP checks. Real kind
v1.34.0 matched all three rule IDs, executed and idempotently replayed the
allowlisted rollout restart, confirmed the expected RBAC yes/yes/no/no matrix,
and left zero platform cluster/diagnosis/remediation QA rows.

Thesis diagrams, test matrix, environment record, dependency-license report,
reference attribution and the 10-minute defense script are indexed by
`docs/thesis/README.md`. Exact results and compatibility fixes are archived in
`docs/changes/2026-07-26-delivery-packaging.md`.

## Latest Defense Demo Readiness

`scripts/demo-up.ps1` now prepares and retains a fully populated real-kind
environment; `scripts/demo-down.ps1` removes the retained platform record and
optionally the Namespace/RBAC. A from-zero run passed after making Pod polling
tolerant of normal Kubernetes state transitions. The current retained cluster
is `demo-kind-20260726-170601`, Ready on Kubernetes v1.34.0 with three diagnosis
records and one succeeded/idempotently replayed remediation.

`scripts/capture-thesis-screenshots.ps1` uses installed Edge/Chrome through the
standard DevTools Protocol and needs no npm/browser installation. Four 1440x1000
authenticated screenshots were captured and visually checked under
`docs/thesis/screenshots`. The ignored browser profile is removed after every
run. See `docs/changes/2026-07-26-defense-demo-readiness.md`.

The final regression passed at 17:16:02 and the default ephemeral E2E passed
again at 17:16:21. Current intentional demo state is one Ready platform cluster,
three diagnosis rows and one remediation plan. All Compose services are healthy.

## Latest Real kind Validation

The previously blocked Kubernetes environment gate is now complete. A real
`kind-aiops-test` cluster running Kubernetes v1.34.0 accepted the managed-cluster
RBAC and repeatable `aiops-demo` workloads. The platform imported a short-lived
ServiceAccount kubeconfig, reported all three readiness Conditions True, read
core resources and Events, matched all three deterministic diagnosis rules,
and completed confirmed/idempotent Deployment rollout remediation. RBAC denied
Pod deletion and cross-namespace Deployment patch. The unprivileged Nginx fix,
exact results, cleanup contract and apply-manager pitfalls are archived in
`docs/changes/2026-07-17-real-kind-diagnosis-remediation.md`.

The 2026-07-17 user-management stage passed all Go tests and build, frontend typecheck, 19 Vitest tests and production build, PostgreSQL/API concurrency checks, and browser workflow verification. Temporary users and processes were removed after validation.

## Latest Kubernetes Compatibility Work

Service diagnosis now prefers bounded `discovery.k8s.io/v1` EndpointSlice reads, converts ready/not-ready addresses into the existing evidence shape, and falls back to core/v1 Endpoints only on discovery 404. Permission and transport failures remain visible. Focused tests cover ready true/false/nil, multi-address counts, fallback and non-fallback errors. Full verification is recorded in `docs/changes/2026-07-17-endpointslice-compatibility.md` and ADR 0016.

This earlier environment limitation is superseded by the 2026-07-17 real kind validation described above.

## Latest Cluster Security Work

System administrators can replace a registered cluster kubeconfig through `PUT /clusters/{id}/credentials` and the cluster UI. Parsing/encryption happens before an atomic database swap; stale probe fields are cleared, Conditions become Unknown, and the cached client is invalidated only after commit. Invalid input leaves the original API server and credential unchanged. Real API/DB verification confirmed encrypted storage, no audit secret leakage, explicit-probe state, and success/failure/denied audit results. See `docs/changes/2026-07-17-cluster-credential-rotation.md` and ADR 0017.

## Latest Account Security Work

All authenticated roles can change their own password from `/account/security`. Current-password verification, reuse rejection and compare-and-swap storage protect against stale concurrent updates. Success increments `auth_version`, revokes all refresh sessions, clears the cookie and forces re-login. Real API verification confirmed old access/refresh/password rejection, new-password login, success/error audit outcomes and zero password leakage. See `docs/changes/2026-07-17-self-service-password-change.md` and ADR 0018.

The same page now lists the user's active refresh sessions with a current marker and supports revoking one or all other sessions. Repository transactions require the active current Cookie summary and scope every row by user ID. Real two/three-session verification passed; see `docs/changes/2026-07-17-session-device-management.md` and ADR 0019.

## Latest Observability and Contract Work

The service now exposes `GET /metrics` with Prometheus-compatible counters and
duration aggregates. Labels are restricted to method, registered Gin route
template and status class; raw paths, identifiers, query strings and request
bodies are excluded. The scrape endpoint is intentionally unauthenticated and
must be bound or firewalled to a trusted monitoring network. A hand-reviewed
OpenAPI 3.0.3 baseline covers all current public/auth/user/cluster/Kubernetes/
diagnosis/AI/audit route families. Focused HTTP tests and YAML parsing passed.
See `docs/changes/2026-07-17-openapi-and-http-metrics.md`, ADR 0020 and
`docs/api/openapi.yaml`.

This stage's verification passed: `go test ./...`, `go build ./cmd/server`,
OpenAPI route drift and YAML validation, frontend `pnpm typecheck`, frontend
Vitest (8 files / 25 tests), and frontend `pnpm build`.

## Latest Contract and Container Work

`TestRegisteredRoutesMatchOpenAPI` now registers the full conditional Gin
router and compares its method/path set in both directions with the OpenAPI
document. A route changed on only one side fails the normal Go suite. Container
build drift was also corrected: the backend builder now matches `go.mod` at Go
1.25, the frontend explicitly installs `pnpm@11.7.0`, and the workspace policy
file is present before install so the allowed esbuild lifecycle script runs.

The frontend image built and served the SPA with HTTP 200. A real local API
process against PostgreSQL passed live/ready and verified that a concrete user
identifier does not enter route-template metrics. Docker Hub initially reset
OAuth connections, but a later retry pulled the bases and successfully built
the non-root backend image. See
`docs/changes/2026-07-17-contract-and-container-build-gates.md`.

## Latest Completed Work

Administrator password reset with full session invalidation is complete. Migration `000011_user_auth_version` adds a monotonic credential version; reset writes the bcrypt hash, increments the version and revokes refresh tokens in one transaction. Old access tokens, refresh sessions and passwords were rejected in real PostgreSQL/API verification. The system-admin UI exposes reset for other users, and `user.password.reset` records success/failure without the request body.

Verification baseline after this stage: all Go tests and server build, frontend typecheck, 20 Vitest tests and production build. See `docs/changes/2026-07-17-admin-password-reset.md` and ADR 0015.

## Recommended Next Work

The Git baseline, private remote, hosted CI and dependency governance are
already archived. Next, register the isolated `aiops-kind` runner, evaluate
OIDC/MFA, then validate signed audit archives, production backup/PITR and HA
behavior. Application-key re-encryption is accepted locally and in hosted CI.
Only after those reviews should the
project choose a registry identity, artifact-signing policy, license and formal
release tag. Keep MFA/SSO as a separate identity-provider project.

## Controlled Remediation Contract (Archived)

Started after the durable-notification milestone. The intended first action is
a bounded Deployment rollout restart linked to a confirmed Pod diagnosis. The
preview must use Kubernetes server-side dry-run, persist an expiring plan and
return a one-time confirmation token whose hash alone is stored. Execution must
require the token plus an idempotency key, enforce the captured target identity
and resource version, use an exact allowlisted patch, and append an audit
result. No arbitrary manifest, path, verb or patch body may cross the API
boundary. Implementation and isolated verification are complete; the next
environment gate is applying the target-cluster RBAC and platform deployment
to a real kind or safe Kubernetes context.

## Latest Kubernetes Deployment Work

`deploy/kubernetes/` now contains a Kustomize baseline with namespace, service
accounts, PostgreSQL StatefulSet/PVC, backend and frontend Deployments,
ClusterIP Services, TLS Ingress and default-deny NetworkPolicies. The backend
and database are not Ingress targets; only frontend pods may call the API,
while a labeled in-cluster monitoring namespace may scrape `/metrics`. The
Secret template is deliberately excluded from the default Kustomization and
contains only replacement markers. Application pods run non-root with probes,
resource limits, dropped capabilities and read-only roots. See ADR 0021 and
`docs/changes/2026-07-17-kubernetes-deployment-baseline.md`.

Offline Kustomize rendering produced 16 resources and deployment-manifest
checks pass in `go test ./...`. The frontend image was rebuilt and served HTTP
200 as UID/GID 101 with read-only root plus writable tmpfs mounts. Actual kind
apply remains unverified because kind is not installed and kubectl has no
current context. Both application images now build successfully. A complete
Compose smoke run reached healthy PostgreSQL/backend/frontend containers and
verified direct health, frontend API proxy, backend metrics, and that the
frontend does not proxy `/metrics`; application containers were removed after
verification while the development PostgreSQL container was retained.

Final regression for this stage passed `go vet ./...`, `go test ./...`,
`go build ./cmd/server`, frontend typecheck, 8 Vitest files / 25 tests,
frontend production build, `docker compose config --quiet`, both Docker image
builds, Kustomize rendering (16 resources) and the Compose runtime smoke checks
above. With notifications disabled by default, the rebuilt containers reached
healthy status; live, ready, SPA, frontend API proxy, backend metrics and
frontend non-proxy behavior all returned the expected HTTP results.

The controlled-remediation regression then passed the same Go vet/test/build
gate, frontend typecheck, 8 Vitest files / 26 tests, frontend production build,
`docker compose config --quiet`, both rebuilt application images, platform
Kustomize rendering (16 resources), managed-cluster RBAC Kustomize rendering
(5 resources), and a Compose health smoke with backend/frontend healthy and
live/ready/SPA/API-proxy checks returning 200. `kubectl apply --dry-run=client`
was not claimed because this machine has no usable Kubernetes context; offline
Kustomize rendering is the available manifest gate.

## Latest Diagnosis Notification Work

Migration `000012_diagnosis_notification_outbox` adds a singleton enable flag,
durable delivery rows and an after-trigger that emits only
`diagnosis.created`, `diagnosis.status_changed` and `diagnosis.assigned`. The
worker claims rows with `FOR UPDATE SKIP LOCKED`, signs the exact JSON body with
HMAC-SHA256, rejects redirects, retries with capped exponential backoff and
marks terminal failures `dead`. Payloads are an explicit allowlist and never
contain evidence, workflow comments, credentials or response bodies.

The Event Center exposes safe delivery metadata to system administrators and
security auditors. Only system administrators can retry a dead row; the retry
is audited as `notification.delivery.retry`. Notification configuration is
disabled by default, requires an HTTPS URL and a 32-character secret in
production, and is wired into Compose/Kubernetes templates without putting the
secret in ConfigMap.

Verification included focused Go/config/HTTP tests, frontend API/UI tests,
OpenAPI route drift coverage, real PostgreSQL trigger checks for all three
events, authenticated API checks (including disabled retry and audit outcome),
and a loopback webhook smoke in which all three rows reached `delivered` with
valid signatures. QA rows, the receiver, generated binary and enabled setting
were cleaned up after the smoke. See ADR 0022 and
`docs/changes/2026-07-17-durable-diagnosis-notifications.md`.

An archive review caught and fixed the combined-update edge case: when one SQL
statement changes both diagnosis status and assignee, the trigger now appends
both event types instead of prioritizing one. A real PostgreSQL transaction
confirmed one created, one status-changed and one assigned event, then rolled
back to leave the database clean. The full Go suite and backend image build
passed again after this correction.

## Latest Controlled Remediation Work

ADR 0023 and migration `000013_controlled_remediation` define the first
mutation primitive: `deployment.rollout_restart`. It is available only for a
confirmed Pod diagnosis and a Deployment in the same namespace whose selector
matches the current Pod labels. Preview captures UID/resourceVersion, creates
the exact server-generated annotation patch and submits Kubernetes
`dryRun=All`; only an accepted dry-run creates an expiring plan.

Execution stores only a SHA-256 confirmation-token hash, requires an
`Idempotency-Key`, atomically claims the plan and reuses the captured
resourceVersion. Same-key replay returns the stored result without another
PATCH; a stale lease can recover after a process failure. Different keys,
expired plans, invalid tokens and changed targets are rejected. The API never
accepts a Kubernetes path, verb, raw patch or manifest. System/operations
administrators can write; all logged-in roles can read safe plan metadata.

The target-cluster RBAC example under `deploy/managed-cluster/` keeps the
observer role read-only and grants Deployment `get`/`patch` only in an
explicitly approved namespace.

Real isolated verification completed against PostgreSQL and a TLS Kubernetes
stub: the primary scenario produced one dry-run and one real PATCH; same-key
replay produced no second PATCH; different-key/invalid-token/expired-plan
returned 409/403/410; stale same-key recovery succeeded; viewer read/write
authorization and success/failure/denied audit entries were confirmed. QA
resources and processes were removed afterward. See ADR 0023 and
`docs/changes/2026-07-17-controlled-remediation.md`.

## M19 Controlled Operations Catalog

ADR 0024 and migration `000014_controlled_operations_catalog` preserve the
diagnosis-bound `deployment.rollout_restart` flow and add exactly three
resource-originated actions: `deployment.scale`, `cronjob.suspend` and
`cronjob.resume`. Resource preview accepts only action, Namespace, target name
and the typed desired replica count required by scale. Unknown or irrelevant
fields, replica counts outside 0..1000 and no-change requests are rejected.

Every plan captures the current UID/resourceVersion and typed before/after
value, uses a complete server-generated patch, and must pass Kubernetes
server-side dry-run before persistence. Execution reuses the one-time token,
idempotency, stale-claim recovery, audit and sanitized-result controls from ADR
0023. Resource operation history is bounded to 50 and readable by authenticated
roles; only system and operations administrators may preview or execute. The
remediator Role grants namespaced Deployment and CronJob `get`/`patch`, while
the observer remains read-only and `kube-system` mutation remains denied.

The full M19 gate passed with 128 Go test entries, 12 Vitest files / 56 tests,
Kustomize 16/5/22/3 and three healthy Compose services. Real kind v1.34.0
verified Deployment scale plus same-key replay and restoration, CronJob
resume/suspend plus restoration, all seven diagnoses and the positive/negative
RBAC matrix. Desktop and 390x844 browser checks passed with one overlay
scrollbar and no warning/error logs. Evidence is archived in
`.artifacts/verification/verify-20260727-180428.json`,
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json` and
`docs/changes/2026-07-27-controlled-operations-catalog.md`.

Deployment rollback was explicitly deferred at M19. The gateway did not yet
expose exact ReplicaSet revision and Pod-template history, so preview could not
bind to an immutable selected revision without accepting unsafe client-owned
patch content. M23 (ADR 0040) resolved this by binding preview and confirmation
to the exact ReplicaSet revision captured at preview time; see ADR 0024 for the
original admission requirements and ADR 0040 for the accepted implementation.

## M20 Bounded Fleet Health

ADR 0025 adds authenticated `GET /api/v1/fleet/health` with hard limits of 20
enabled clusters, four concurrent cluster workers, four seconds per cluster and
100 sampled Nodes, Pods, Deployments and Events. Reads remain fixed and
sequential inside each worker. Each cluster reports health counts, explicit
sample coverage, Warning count, latency and sanitized failure scopes/codes.
Timeout, truncation or one resource failure stays local to that item; only the
platform cluster-directory read can fail the whole request.

Dashboard renders the response as one compact comparison table and can switch
the existing selected-cluster cockpit from a row. It does not move Kubernetes
fan-out into the browser, introduce arbitrary GVK/path input or widen target
RBAC. The retained runtime validates one real kind data path; deterministic
tests validate ordering, two-worker concurrency, timeouts and partial failure.
Phase 2 now adds physically distinct real-cluster evidence:
`scripts/e2e-fleet-kind.ps1` creates two kind clusters and an isolated platform
runtime, verifies direct resource counts, ordering, limits, timeout, recovery,
unavailable isolation and RBAC, then requires complete cleanup. See
`docs/changes/2026-07-27-bounded-multi-cluster-health.md` and
`docs/changes/2026-07-27-two-cluster-fleet-e2e.md`.

Phase 3 adds ADR 0026 and `GET /api/v1/fleet/resources/search`. It searches only
Pod, Deployment, Service and Ingress by bounded name substring and optional
Namespace, reports known-result and enabled-cluster coverage separately, and
localizes fixed-kind failures. `/search` keeps the reviewed query shape in the
URL and opens matches in the existing Workloads drawer. Phase 4 now adds the
owner-scoped persistence contract from ADR 0027 without expanding the query
shape. See `docs/changes/2026-07-27-bounded-global-resource-search.md` and
`docs/changes/2026-07-27-user-owned-global-search-filters.md`.

Phase 5 adds physically distinct real-cluster evidence through
`scripts/e2e-global-search-kind.ps1`. Its isolated runtime validates fixed
kinds, stable cluster/kind/Namespace/name ordering, result and cluster
coverage, truncation, timeout/recovery/query-failure isolation, observer RBAC
and complete cleanup. See
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`.

## Latest Diagnosis Rule Expansion

M7 adds two deterministic Pod rules without widening the Kubernetes write
boundary: `pod.pending.v1` matches the Pending phase with scheduling conditions
and Warning Events, while `pod.oom_killed.v1` matches current or previous
container termination with reason `OOMKilled`. The rule chain evaluates these
specific conditions before ImagePullBackOff and CrashLoopBackOff, preserving
the most actionable root cause when a container is both OOM-killed and in a
restart backoff. The Workloads view now exposes diagnosis actions for Pending
and OOMKilled Pods. Unit coverage and documentation are archived in
`docs/changes/2026-07-26-diagnosis-rule-expansion.md`.

## Latest Node and Deployment Diagnosis Expansion

M8 adds `node.not_ready.v1` and `deployment.replicas_unavailable.v1`. Node
diagnosis treats a missing or non-True Ready Condition as a match and preserves
the complete Condition set. Deployment diagnosis compares the Kubernetes
default-aware desired replica count with Ready and Available replicas while
retaining all rollout counters. The shared diagnosis API accepts Node without a
Namespace and still requires Namespace for Pod, Service and Deployment. No new
mutation path was added. Full verification and scope are archived in
`docs/changes/2026-07-26-node-deployment-diagnosis.md`.

## Important Invariants

- Deterministic diagnosis remains usable when AI is disabled or fails.
- Sensitive values never enter responses, audit details, logs or Git.
- ConfigMap values and StorageClass parameters never enter public Kubernetes resource responses.
- Secret values, labels and annotations never enter public responses; Secret observer RBAC still requires explicit threat acceptance because Kubernetes cannot field-filter the raw object.
- User disable/role removal is effective on the next authenticated request.
- At least one active `system_admin` must always remain.
- Password reset must invalidate both refresh sessions and already-issued access tokens.
- Audit and diagnosis histories are append-only through business APIs.
- Kubernetes mutation is limited to the fixed catalog: diagnosis-bound Deployment rollout restart; resource-originated Deployment scale, image update and exact ReplicaSet-backed rollback; CronJob suspend/resume; and fixed Deployment/Service/Ingress promotion. No arbitrary write proxy exists.
- Every operation preview must pass server-side dry-run, capture target UID/resourceVersion and a typed before/after value, and expire without execution.
- Operation execution requires a one-time token, an idempotency key and a matching target precondition; repeated same-key calls do not repeat the Kubernetes patch.
- Deployment rollback accepts only a server-selected immutable ReplicaSet revision and complete PodTemplate snapshot bound to preview and execution; client-owned templates and arbitrary patches remain forbidden.
- Fleet queries must remain bounded by reviewed cluster concurrency, per-cluster timeout and per-kind sample limits; partial or truncated data cannot be labeled healthy.
- Global search must remain limited to the reviewed name/Namespace/four-kind query shape; omitted clusters, truncated results and fixed-kind failures cannot be labeled complete.

## Local Verification

PostgreSQL development container is `k8s-aiops-postgres-1`, exposed on `localhost:15432`. Before handoff, run:

```powershell
.\scripts\verify.ps1
.\scripts\e2e-fleet-kind.ps1
.\scripts\e2e-global-search-kind.ps1
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
.\scripts\e2e-kind.ps1
```

The verify script uses a compatible host Go when available and otherwise runs
Go 1.25 in Docker with the repository mounted read-only. `frontend/dist` and
`.artifacts` are generated outputs and must not be treated as source or
committed. The development PostgreSQL container remains healthy on port 15432;
saved-filter acceptance rows are zero and migration 000017 is the latest
applied version in the current development database.

## Next Priorities After Current Work

The authoritative phase requirements and acceptance standards are in
`docs/next-development-plan.md`. The default next engineering task is M27.0;
M26 remote/organization actions remain externally gated. M27-M31 are separate,
serially reviewed milestones for alert lifecycle, Backup creation, governance
posture, Node maintenance and isolated restore. M32 starts only after those
milestones are accepted and one release candidate revision is selected. Missing
organization input must be recorded as deferred, never presented as completed
OIDC/MFA, PITR/HA or production readiness.

## Real kind Final Verification

Fresh verification on 2026-07-17 passed backend format/vet, all Go packages
with cache disabled, and server build. Frontend typecheck passed, Vitest passed
8 files / 26 tests, and the production Vite build completed. Kustomize rendered
16 platform resources, 5 managed-cluster RBAC resources and 7 demo resources;
Compose configuration and real-cluster server-side dry-runs passed. The final
kind state retained two Ready control workloads plus the intended
CrashLoopBackOff and ImagePullBackOff workloads. The temporary platform cluster
row is zero, the generated credential file contains no credential material,
and the validation API process is stopped.
