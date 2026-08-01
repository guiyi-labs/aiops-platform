# ADR 0067: Intelligent Inspection + Service Mesh Read-Only (M52)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M52
- Supersedes: none
- Related: ADR 0004 (bounded read-only Kubernetes gateway), ADR 0035
  (bounded background metrics collection), ADR 0036
  (authenticated exact-series metrics history), ADR 0039
  (unified service identity and signal model), ADR 0049
  (route descriptor contract and RBAC inventory), ADR 0050
  (lightweight cluster and namespace access grants), ADR 0066
  (bounded event stream and alert inhibits)

## Context

M52 closes Phase 2 (full-stack observability) of the post-M45 roadmap.
The roadmap calls for two backend increments that extend the platform
into proactive fleet health and service-mesh evidence — without
introducing a KubeEye dependency, an Istio control-plane controller, or
a write-capable mesh governance path:

1. **Intelligent inspection (KubeEye-style)** — a compile-time catalog of
   8 deterministic inspection rules (node NotReady, Pod restart loop,
   OOMKilled, PVC pending, ImagePullBackOff, CrashLoopBackOff,
   Endpoints orphan, namespace quota near-limit) that a user can run
   on-demand or schedule as a periodic plan. Findings are normalized
   into the M39 `signal_occurrences` table so the full AIOps loop
   (correlation → diagnosis → automation) can consume them as first-class
   signals. The engine must be bounded: per-cluster timeout,
   per-rule timeout, concurrent-cluster cap, and result-row cap.
2. **Service mesh read-only access** — list/detail views of Istio
   `VirtualService` and `DestinationRule` CRs, plus aggregate traffic
   metrics (request volume, error rate, latency p50/p95/p99) sourced
   from the M36 Prometheus history. The surface is strictly read-only:
   no route editing, no subset reweight, no retry/timeouts rewrite. The
   metrics feed the M41 SLO evidence pool and the M40 topology edge
   attributes; they are never an action input.

The design space includes an external KubeEye operator deployment,
a full Istio REST API proxy, and a schedule+write CRD controller. All
three are rejected by the project's hard constraints:

- An external KubeEye operator would violate the single-binary modular
  monolith constraint (ADR 0002), require a second control-plane
  lifecycle, and expose findings through a foreign schema that the M39
  signal model could not consume natively.
- A full Istio REST proxy would break the bounded read-only gateway
  model (ADR 0004): Istio's `istiod` REST surface is unbounded and
  write-capable, and proxying it would re-open policy-escape surfaces
  the gateway was designed to close.
- A schedule+write CRD controller would introduce a write-path into the
  cluster (governance execution) that the roadmap explicitly defers;
  M52 is an evidence-only milestone.

M52 therefore delivers: a **bounded single-binary inspection engine**
with a compile-time rule catalog (no external operator) and normalized
M39 signal output, plus a **read-only CRD list + Prometheus-aggregate**
service-mesh surface (no istiod proxy, no governance writes).

## Decision

### 1. Compile-time inspection catalog (8 rules); no external KubeEye

`internal/inspection.DefaultCatalog()` returns a fixed slice of 8
`RuleDescriptor` entries. Each descriptor carries `Code`,
`SchemaVersion`, `Domain` (node/pod/workload/storage/namespace/network),
`DefaultSeverity` (critical/warning/info), a stable M39 `SignalCode` of
the form `inspect.<domain>.<code>.v1`, a `Description`, a
`Remediation` hint, and a per-rule `Timeout`. Clients cannot add new
rules at runtime (catalog is compile-time); per-cluster overrides of
`enabled`/`severity` are persisted in the `inspection_rules` SQL table
and applied at plan-build time (rule-level override, no schema edit).

The 8 rules are:

| Code | Domain | Signal code |
| ---- | ------ | ----------- |
| node_not_ready | node | inspect.node.not_ready.v1 |
| pod_restart_loop | pod | inspect.pod.restart_loop.v1 |
| pod_oom_killed | pod | inspect.pod.oom_killed.v1 |
| pvc_pending | storage | inspect.storage.pvc_pending.v1 |
| image_pull_backoff | workload | inspect.workload.image_pull_backoff.v1 |
| pod_crash_loop | pod | inspect.pod.crash_loop.v1 |
| endpoints_orphan | network | inspect.network.endpoints_orphan.v1 |
| namespace_quota_high | namespace | inspect.namespace.quota_high.v1 |

### 2. Bounded execution engine; per-cluster + per-rule timeouts

`internal/inspection.Service.RunInspectOnce` is the entry point. It
resolves authorized cluster IDs (via `ClusterLister.List`), creates an
`inspection_tasks` row in state `QUEUED`, then fans out to at most
`MaxConcurrentClusters` (default 4, min 1, max 32) worker goroutines.
Each worker runs the 8 (or user-selected) rules sequentially against a
single cluster. Each rule run is subject to the descriptor's `Timeout`
(capped at the cluster-level `PerClusterTimeout`, default 15s, min 5s,
max 120s). Context cancellation propagates to every gateway call via
the shared per-cluster context. Results are appended to a bounded slice;
when `MaxTaskResults` (default 1000) is reached the worker short-circuits
to `RESULTS_TRUNCATED` summary state (no further rules execute) — a hard
memory and storage cap regardless of fleet size.

The `DefaultExecutor` routes each rule code to a dedicated `ruleXxx`
method. All methods read exclusively through the M33/ADR 0004
read-only Kubernetes gateway (`Nodes`, `Pods`, `PersistentVolumeClaims`,
`Deployments`, `StatefulSets`, `DaemonSets`, `Services`,
`ServiceEndpoints`, `ResourceQuotas`, `Namespaces`). No informer cache,
no Watch, no direct client-go calls.

### 3. Findings normalized to M39 signal model

Every executor `Finding` carries `Severity`, `Summary`, `Remediation`,
a structured `ResourceRef` (cluster_id, namespace, kind, name, uid,
api_version — best-effort uid for nodes), and a free-form `Labels` map
for rule-specific attributes (e.g. `restart_count`, `exit_code`,
`used_percent`). After a task completes, `processFindings` batches them
into `signal_occurrences` via the M39 `signal.Service` upsert path:
`fingerprint = sha256(cluster_id + signal_code + resource_uid +
summary_prefix)` with a 300s dedup window (matching M39). Signal
`source = 'inspection'`, `evidence_id` references the parent
`inspection_results.id`, and `triggered_by` propagates the task's
operator user id. The task rows and signal rows are two linked but
independent surfaces; users can query tasks for operator UI and query
signals for M42 correlation.

### 4. Plan lifecycle (cron-driven but in-process, not K8s CronJob)

`inspection_plans` stores `name`, `cron_expr` (5-field standard cron,
validated client-side via the same parser as ADR 0059 policy schedules),
`cluster_ids` (int8 array, empty = all authorized clusters),
`rule_codes` (text array, empty = full catalog), `severity_floor`, and
`enabled`. The in-process scheduler (`inspection.Scheduler`) is a
single goroutine that ticks at `ScheduleTick` (default 30s) and re-reads
enabled plans from SQL on every tick (no in-memory plan cache — avoids
split-brain under multi-replica deployments; each replica races to
`UPDATE … SET last_run_at = NOW() WHERE id = ? AND last_run_at < ?` via
`clause.OnConflict` to pick exactly one executor per plan tick). The
scheduler calls `RunInspectOnce` with `TriggeredBy = plan.creator_id`
and `TriggerSource = 'schedule'`.

A K8s CronJob runner is explicitly rejected: it would escape the
single-binary boundary (ADR 0002) and require RBAC the platform does
not grant itself.

### 5. Service mesh: list + detail via existing CRD gateway; no istiod proxy

`internal/servicemesh.Service` exposes four read-only methods:

- `ListVirtualServices(ctx, clusterID, namespace, page, limit)` →
  `ListResponse[VirtualServiceView]`
- `GetVirtualService(ctx, clusterID, namespace, name)` →
  `VirtualServiceView`
- `ListDestinationRules(ctx, clusterID, namespace, page, limit)` →
  `ListResponse[DestinationRuleView]`
- `GetDestinationRules(ctx, clusterID, namespace, name)` →
  `DestinationRuleView`
- `TrafficMetrics(ctx, clusterID, namespace, service, window, step)` →
  `TrafficMetrics` (request count, 2xx/4xx/5xx counts, error rate,
  latency p50/p95/p99 as JSON points arrays)

List/detail calls go through the existing `kubernetes.Service.CustomResource`
gateway (M49 ADR 0064) with `group = networking.istio.io`,
`version = v1beta1`, `plural = virtualservices | destinationrules`.
Manifest JSON is projected to a small fixed `VirtualServiceView`
(name, namespace, hosts, gateways, http_routes count, age, cluster_id)
and `DestinationRuleView` (name, namespace, host, subsets count,
traffic_policy summary, age, cluster_id). The raw manifest is never
returned; callers can still fetch it via the M49 generic CRD browser if
authorized (separate route, separate audit).

### 6. Traffic metrics: Prometheus-history aggregate, no new scrape config

`TrafficMetrics` reuses the M36 `metricshistory.Service.QueryRange`
path. It executes six fixed series queries (template-based, no client
PromQL injection) against the M41 Istio SLI metrics:

- `istio_requests_total` by `response_code` (2xx/4xx/5xx) → request
  volume and error rate
- `istio_request_duration_milliseconds` → rate-weighted quantile
  approximation for p50/p95/p99

The `Service` parameter is mandatory; `namespace` defaults to the
caller's M35 resolved scope. Query windows are capped at `MaxWindow`
(24h, matching ADR 0036) and steps are normalized to `step >=
Max(window / 500, 15s)` to keep series cardinality bounded. The handler
returns a pre-aggregated `TrafficMetrics` structure (not raw series) so
the M41 SLO evaluator can feed directly from the same object shape.

### 7. Anti-leakage and authorization

- **Inspection routes**: M50/M47 workspace scope honoured at
  cluster-selection time: a workspace-scoped caller sees only that
  workspace's clusters (via `ClusterLister` adapter). Per-cluster
  results are further filtered to the caller's M35 namespace scope for
  namespace-domain rules (Pod*, PVC, Endpoints, Quota). An empty scope
  yields zero findings for namespace rules, not 403 — anti-leakage per
  ADR 0050.
- **Service mesh routes**: List routes accept `namespace` query param
  and re-check it against the M35 resolved scope; unauthorized
  namespace → `DESTINATION_RULE_NOT_FOUND` / `VIRTUAL_SERVICE_NOT_FOUND`
  (404, not 403). `TrafficMetrics` with out-of-scope service → empty
  series (not 403/404).
- **RBAC**: 4 existing roles reused — `operations_admin` for
  plan/task create/delete + override save, `operations_viewer` for
  list/read; `mesh_admin` / `mesh_viewer` (from M49/M47 role matrix)
  for servicemesh routes. No new roles.
- **Audit**: all 8 routes tagged with a unique audit verb per ADR 0008.
  Plan create/delete, override save, and `RunOnce` record the full
  parameter payload (cron, cluster_ids, rule_codes).

## Consequences

### Positive

- Single-binary deployment: inspection engine + mesh reader compile into
  the existing server binary — zero new operators or sidecars.
- Bounded at every layer: cluster concurrency, per-cluster timeout,
  per-rule timeout, result-row cap, series-window cap, step-floor cap.
- Reuses 4 existing pillars unchanged: ADR 0004 gateway for reads, M35
  namespace scope for anti-leakage, M36 Prometheus history for mesh
  metrics, M39 signal model for findings ingestion.
- Compile-time catalog means the 8 rules are auditable in code review
  and replayable against the M45 golden dataset.

### Negative / Risks

- Rule catalog is not extensible at runtime. Operators wanting a 9th
  rule must ship a release. Mitigation: per-cluster severity/enable
  overrides cover the common "this rule is too noisy on cluster X" case
  without a schema edit.
- Istio CRD projection is shallow. Operators wanting match-condition or
  subset-detail drilldown must use the M49 generic CRD browser + raw
  manifest viewer — not a bug, an intentional shallow-read choice to
  keep the mesh surface bounded.
- In-process scheduler has at-most-once semantics under replica crash
  between the `UPDATE last_run_at` winner and `RunInspectOnce`
  execution. Mitigation: plans have a backstop `RunNow` button in the
  UI, and the 30s tick interval means the next tick recovers. For
  stronger guarantees a future milestone could write a task row
  atomically with the last_run_at update; this is deferred.
- Traffic metrics rely on `istio_requests_total` and
  `istio_request_duration_milliseconds` being present in the M36
  Prometheus adapter's allow-list. If an adapter lacks these series,
  `TrafficMetrics` returns empty (fail-closed, not synthesized).

## Deployment notes

- Migration `000037_inspection_and_servicemesh.up.sql` must run before
  the new binary starts.
- The scheduler goroutine starts unconditionally; it is a no-op when
  `inspection_plans` contains zero enabled rows.
- Service mesh routes return empty lists on clusters that do not have
  the Istio CRDs registered (CRD gateway 404 → projected empty list,
  never an error).
- Re-test `TestRegisteredRoutesMatchOpenAPI` covers all 8 new routes
  plus the M39 signal upsert path integration.
