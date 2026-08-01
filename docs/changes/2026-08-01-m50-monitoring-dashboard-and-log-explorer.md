# M50: Monitoring Dashboard + Log Explorer

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0065](../adr/0065-monitoring-dashboard-and-log-explorer.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 107.18s; backend=True frontend=True manifests=True)

## Summary

Delivered the M50 backend increment implementing the first Phase 2
(full-stack observability) milestone: a fixed-template monitoring dashboard
(single-cluster + workspace cross-cluster) and a bounded Loki log explorer.
The design is bounded by three structural invariants:

1. **Compile-time-fixed templates** — `fixedTemplates` in
   `internal/monitoring/model.go` is a package-level map with four templates
   (node_overview, workload_overview, pod_overview, workspace_overview).
   Adding a template is a code change — no admin API, no runtime expansion
   (static-extension hard constraint). Clients cannot inject PromQL; the
   dashboard returns panel descriptors that the frontend uses to drive
   existing `/metrics/history` calls.
2. **Topology-only workspace dashboard** — the workspace dashboard returns the
   fixed `workspace_overview` template plus the workspace's cross-cluster
   `(cluster_id, namespaces)` topology. The backend does NOT pre-fetch
   per-cluster time series; the frontend fans out using the topology. The
   fan-out is bounded by `MaxClusters` (default 20, matching federation).
3. **Log explorer reuses M37A LogProvider** — `POST /logs/query` reuses the
   `capability.LogProvider` (Loki adapter, ADR 0053). Clients cannot inject
   LogQL. The namespace arrives in the body, so the handler re-checks it
   against the M35 resolved namespace scope (anti-leakage 404).

Authorization reuses existing middleware chains without introducing a new
authorization path. Cluster dashboard + logs/query are registered under
`resourceRoutes` (M35 cluster + namespace scope, M47 workspace filter).
Workspace dashboard is registered under `workspaceRoutes`; the monitoring
service enforces `workspace_viewer` via `workspace.Service.ListMemberships`.

Anti-leakage (404 > 403) is preserved: an unauthorized or missing workspace
returns 404 `WORKSPACE_NOT_FOUND`; an unauthorized namespace in the logs/query
body returns 404 `RESOURCE_NOT_FOUND`.

## Files Changed

### New Files

- `backend/internal/monitoring/model.go` — Template constants, Panel,
  ClusterDashboardRequest/Response, WorkspaceDashboardRequest/Response,
  WorkspaceClusterEntry, and the `fixedTemplates` map (4 templates, 2 panels
  each).
- `backend/internal/monitoring/service.go` — `Service` with
  `ClusterDashboard` (returns fixed template panels) and `WorkspaceDashboard`
  (returns fixed template + cross-cluster topology via
  `WorkspaceMembershipLister`). Bounded fan-out constants (MaxClusters=20,
  MaxConcurrent=4, PerClusterTimeout=4s). `validateWindow` enforces 24h max.
- `backend/internal/monitoring/service_test.go` — 11 unit tests covering
  NewService defaults, cluster dashboard (valid/invalid template/invalid
  window/panel cloning), workspace dashboard (nil lister/lister error/valid
  topology/bounded clusters/invalid window/empty memberships).
- `backend/internal/httpserver/monitoring.go` — `monitoringHandler` with
  `clusterDashboard`, `workspaceDashboard`, and `queryLogs` methods.
  `parseMonitoringTime` helper. `writeMonitoringError` error mapper.
  Namespace scope re-check for logs/query (anti-leakage 404).
- `backend/internal/httpserver/monitoring_test.go` — 17 handler tests covering
  cluster dashboard (200/400 invalid template/400 invalid window/400 missing
  from/503 nil service), workspace dashboard (200/404 not found/503 nil
  service), logs query (200/503 nil provider/400 invalid body/400 invalid
  timestamp/404 namespace scope denied/200 namespace scope allowed/400
  invalid log query/200 default direction forward/500 provider error).

### Modified Files

- `backend/cmd/server/main.go` — Wire capability providers (Prometheus + Loki)
  and monitoring service into `httpserver.Options`.
- `backend/internal/httpserver/router.go` — Extend `Options` with `Monitoring
  *monitoring.Service`. Register 3 M50 routes: cluster dashboard under
  `resourceRoutes`, workspace dashboard under `workspaceRoutes`, logs/query
  under `resourceRoutes` (guarded by `CapabilityLogProvider != nil`).
- `backend/internal/httpserver/openapi_route_test.go` — Add `Monitoring`
  service to the route-contract test's `Options` so the 3 M50 routes are
  covered.
- `docs/api/openapi.yaml` — 3 new paths
  (`/clusters/{cluster_id}/monitoring/dashboard/{template}`,
  `/workspaces/{workspace_id}/monitoring/dashboard`,
  `/clusters/{cluster_id}/logs/query`) and 4 new schemas (`MonitoringPanel`,
  `ClusterDashboardResponse`, `WorkspaceDashboardResponse`,
  `MonitoringLogQuery`).

## Routes

| Method | Path | Audit Action | Auth |
|--------|------|-------------|------|
| GET | `/api/v1/clusters/:cluster_id/monitoring/dashboard/:template` | `monitoring.dashboard.read` | cluster access (M35) |
| GET | `/api/v1/workspaces/:workspace_id/monitoring/dashboard` | `monitoring.dashboard.read` | workspace_viewer (service-enforced) |
| POST | `/api/v1/clusters/:cluster_id/logs/query` | `monitoring.logs.query` | cluster access (M35) + namespace scope re-check |

## Key Invariants Maintained

- **No client-supplied PromQL/LogQL** — only 4 fixed dashboard templates and
  the bounded LogProvider query shape (ADR 0053).
- **Static-extension** — dashboard templates are compile-time-fixed; no
  runtime expansion.
- **404 > 403 anti-leakage** — unauthorized workspace → 404
  `WORKSPACE_NOT_FOUND`; unauthorized namespace in logs/query → 404
  `RESOURCE_NOT_FOUND`.
- **Bounded fan-out** — workspace dashboard topology capped at `MaxClusters`
  (20); excess clusters dropped.
- **24h window bound** — dashboard time window cannot exceed
  `MaxDashboardWindow` (24h, matching metricshistory).
- **2D authorization matrix intact** — no new authorization path; monitoring
  reuses M35 cluster/namespace scope + M46 workspace roles + M47 workspace
  filter.

## Tests

- 11 monitoring service tests (NewService defaults, cluster dashboard
  valid/invalid template/invalid window/panel cloning, workspace dashboard
  nil lister/lister error/valid topology/bounded clusters/invalid
  window/empty memberships).
- 17 httpserver handler tests (cluster dashboard 200/400/503, workspace
  dashboard 200/404/503, logs query 200/400/404/503/500 + scope allowed/denied
  + default direction + provider error).
- `TestRegisteredRoutesMatchOpenAPI` covers all 3 M50 routes (route-contract
  consistency, ADR 0049).
