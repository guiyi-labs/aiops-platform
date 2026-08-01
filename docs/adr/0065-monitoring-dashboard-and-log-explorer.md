# ADR 0065: Monitoring Dashboard + Log Explorer (M50)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M50
- Supersedes: none
- Related: ADR 0034 (bounded Postgres metrics history), ADR 0036
  (authenticated exact-series metrics history), ADR 0053 (capability plane
  adapters), ADR 0050 (lightweight cluster and namespace access grants),
  ADR 0061 (workspace multi-tenancy), ADR 0062 (three-tier console and
  workspace filter), ADR 0063 (multi-cluster federation)

## Context

M50 opens Phase 2 (full-stack observability) of the post-M45 roadmap. The
roadmap calls for two backend increments:

1. **Monitoring dashboard** — a fixed-template dashboard that returns panel
   descriptors (metric, unit, resource kind) for the frontend to render. The
   dashboard must support both a single-cluster view and a workspace-level
   cross-cluster view.
2. **Log explorer** — a bounded Loki log query endpoint scoped to a cluster,
   reusing the M37A `capability.LogProvider` contract.

The design space includes a PromQL/LogQL passthrough, a user-defined dashboard
builder, and a server-side pre-fetch of all panel time series. All three are
rejected by the project's static-extension hard constraints and the ADR 0053
capability-plane model:

- A PromQL/LogQL passthrough would let clients inject arbitrary queries,
  breaking the "no client-supplied PromQL/LogQL" invariant (ADR 0053 §5) and
  reopening the SSRF / resource-exhaustion surface.
- A user-defined dashboard builder implies client-authored panel definitions,
  which is a contract change per milestone; the project ships fixed templates
  reviewed through the gate.
- A server-side pre-fetch of all panel time series would be O(clusters ×
  resources) for the workspace dashboard, exceeding the bounded fan-out budget
  and duplicating the existing `/metrics/history` endpoint.

M50 therefore delivers the minimum viable observability surface: four
**compile-time-fixed dashboard templates** (node/workload/pod/workspace
overview), a **topology-only** workspace dashboard (no pre-fetch), and a
**bounded** log query that reuses the M37A Loki adapter with M35 namespace
scope re-check.

## Decision

### 1. Compile-time-fixed dashboard templates; no PromQL; no client-authored panels

`fixedTemplates` in `internal/monitoring/model.go` is a package-level
`map[string][]Panel` keyed by template name. Four templates are defined:

- `node_overview` — node CPU + memory (2 panels).
- `workload_overview` — pod CPU + memory grouped by workload (2 panels).
- `pod_overview` — pod CPU + memory (2 panels).
- `workspace_overview` — aggregate node CPU + memory across workspace
  clusters (2 panels).

Each `Panel` carries `Title`, `Metric` (cpu/memory), `Unit` (nanocores/bytes),
`ResourceKind` (Node/Pod), and an optional `Description`. The frontend uses
these descriptors to drive calls to the existing
`GET /clusters/:cluster_id/metrics/history` endpoint (M21, ADR 0036) — the
dashboard endpoint itself does **not** pre-fetch time series.

Adding a template is a code change reviewed through the normal gate. There is
no admin API and no runtime expansion (static-extension hard constraint).

### 2. Topology-only workspace dashboard; bounded fan-out

The workspace dashboard
(`GET /workspaces/:workspace_id/monitoring/dashboard`) returns:

- The fixed `workspace_overview` template panels.
- The workspace's cross-cluster `(cluster_id, namespaces)` topology, fetched
  via `workspace.Service.ListMemberships` (which enforces `workspace_viewer`).

The backend does **not** pre-fetch per-cluster time series. The frontend fans
out per-cluster `/metrics/history` calls using the returned topology. The
fan-out is bounded by `Config.MaxClusters` (default 20, matching the
federation service's `ResourceSummary` bound, ADR 0063). Excess clusters are
silently dropped; cluster IDs are sorted ascending for stable rendering.

### 3. Dashboard time window bounded to 24h

`MaxDashboardWindow = 24 * time.Hour` matches the `metricshistory`
`MaxQueryWindow` (ADR 0034). The `validateWindow` function rejects zero,
inverted, or over-24h windows with `ErrInvalidWindow` (400). This ensures the
dashboard cannot request a range that the underlying metrics-history endpoint
would reject.

### 4. Log explorer reuses M37A LogProvider; M35 namespace scope re-check

`POST /clusters/:cluster_id/logs/query` accepts a JSON body with `namespace`,
`pod`, `container`, `text_filter`, `start`, `end`, `direction`, and `limit`.
It reuses the `capability.LogProvider` (Loki adapter, ADR 0053) — clients
cannot inject LogQL.

Because the namespace arrives in the request body (not a query parameter), the
`requireNamespaceQueryAccess` middleware resolves the caller's **full** scope
rather than validating a specific namespace. The handler therefore re-checks
the body namespace against the resolved `authz.ClusterScope`:

- If `AllNamespaces` is true → any namespace is allowed.
- If `AllNamespaces` is false → the body namespace must be in
  `NamespaceGrants`; otherwise 404 `RESOURCE_NOT_FOUND` (anti-leakage, ADR
  0050/0061).

Provider-side validation errors (`ErrInvalidLogQuery`) surface as 400;
provider-side runtime errors surface as 500 with a sanitized message. When the
log provider is nil (not configured), the endpoint returns 503
`LOG_PROVIDER_UNAVAILABLE`.

### 5. Route registration under existing middleware chains

- **Cluster dashboard** (`GET /clusters/:cluster_id/monitoring/dashboard/:template`)
  and **logs/query** (`POST /clusters/:cluster_id/logs/query`) are registered
  under the `resourceRoutes` group, which applies `requireClusterAccess` →
  `requireNamespaceAccess` → `requireNamespaceQueryAccess` →
  `withWorkspaceNamespaceFilter` (M35 + M47). No new middleware is introduced.
- **Workspace dashboard**
  (`GET /workspaces/:workspace_id/monitoring/dashboard`) is registered under
  the `workspaceRoutes` group. The monitoring service enforces
  `workspace_viewer` via `workspace.Service.ListMemberships` (404 > 403
  anti-leakage).

When the monitoring service is nil, the monitoring routes are not registered.
When the log provider is nil, the logs/query route is not registered (the
cluster dashboard route still works without a log provider).

### 6. 404 > 403 anti-leakage preserved

- An unauthorized or missing workspace returns 404 `WORKSPACE_NOT_FOUND`
  (indistinguishable from a genuinely missing workspace). The monitoring
  service collapses both `workspace.ErrWorkspaceNotFound` and
  `workspace.ErrAccessDenied` into `monitoring.ErrWorkspaceNotFound`.
- An unauthorized namespace in the logs/query body returns 404
  `RESOURCE_NOT_FOUND` (indistinguishable from a missing resource).

## Consequences

- The frontend must fan out per-cluster `/metrics/history` calls using the
  workspace dashboard topology. This is intentional — it keeps the backend
  O(1) for the dashboard endpoint and avoids duplicating the metrics-history
  bounded-query logic.
- Adding a dashboard template or panel is a code change (contract change per
  milestone). No runtime expansion is supported.
- The log explorer's namespace scope re-check is a handler-level check, not a
  middleware check, because the namespace arrives in the body. This is
  consistent with the M37A capability logs handler which also takes namespace
  in the body; the difference is that M50 enforces M35 scope, while M37A
  relies on the provider's own namespace filter.
