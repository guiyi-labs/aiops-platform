# M52: Intelligent Inspection + Service Mesh Read-Only

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0067](../adr/0067-intelligent-inspection-and-service-mesh-readonly.md)
- Fast gate: passed (verify-fast.ps1 -Scope Backend, 74.7s; backend=True frontend=False manifests=False)

## Summary

Delivered the M52 backend increment closing Phase 2 (full-stack observability)
of the post-M45 roadmap. Two additions that extend the platform into
proactive fleet health and service-mesh evidence — without introducing
a KubeEye operator, an Istio control-plane proxy, or any mesh governance
write path:

1. **Intelligent inspection (KubeEye-style compile-time catalog)** — 8
   deterministic inspection rules (node_not_ready, pod_restart_loop,
   pod_oom_killed, pvc_pending, image_pull_backoff, pod_crash_loop,
   endpoints_orphan, namespace_quota_high). Users trigger one-shot runs
   via `POST /inspection/run-once` or save periodic plans as standard
   5-field cron via `GET/POST/DELETE /inspection/plans` and per-cluster
   rule enable/severity overrides via `GET /inspection/rules/effective`
   + `PUT /inspection/rules`. Findings are normalized into the M39
   `signal_occurrences` table (source='inspection') so correlation,
   diagnosis and automation can consume them natively. Bounded at every
   layer: `MaxConcurrentClusters=4`, `PerClusterTimeout=15s`, per-rule
   descriptor timeout, `MaxTaskResults=1000` short-circuit truncation.
2. **Service mesh read-only** — list/detail views of Istio
   `VirtualService` and `DestinationRule` CRs via
   `GET /servicemesh/virtualservices`,
   `GET /servicemesh/virtualservices/:namespace/:name`,
   `GET /servicemesh/destinationrules`,
   `GET /servicemesh/destinationrules/:namespace/:name`, plus traffic
   metrics `GET /servicemesh/traffic-metrics?service=` aggregating
   request volume, 2xx/4xx/5xx counts, error rate, and latency
   p50/p95/p99 from the M36 Prometheus history (fixed template queries;
   no client PromQL injection). The surface is strictly evidence-only:
   no subset reweight, no route edit, no retry/timeouts change.

Authorization reuses existing middleware chains: inspection routes
under `aiopsRoutes` + `clusterScopedRoutes` (M50/M47 workspace cluster
scope; M35 namespace scope applied per-finding for namespace-domain
rules); servicemesh routes under `resourceRoutes` (M35 cluster +
namespace scope; `mesh_admin`/`mesh_viewer` roles from the M47 role
matrix). Anti-leakage 404 > 403 preserved throughout.

## Files Changed

### New Files

- `backend/migrations/000037_inspection_and_servicemesh.up.sql` — 4 new tables:
  `inspection_rules` (per-cluster enabled/severity overrides, UNIQUE(cluster_id, rule_code)),
  `inspection_plans` (cron expr, cluster_ids/rule_codes arrays, severity_floor, enabled),
  `inspection_tasks` (QUEUED/RUNNING/SUCCEEDED/PARTIAL/FAILED/RESULTS_TRUNCATED lifecycle,
  trigger_source, triggered_by, rule_codes snapshot, cluster_ids snapshot, counts, started/ended),
  `inspection_results` (task_id FK, cluster_id, rule_code, severity, resource_ref JSON,
  summary, remediation, labels JSON). Check constraints: plan cron_expr NOT EMPTY,
  task cluster_ids NOT EMPTY, result task_id NOT NULL. Indexes on plan.enabled,
  task.status, task.triggered_by, result(task_id, rule_code), rules(cluster_id, enabled).
- `backend/migrations/000037_inspection_and_servicemesh.down.sql` — Drop
  `inspection_results`, `inspection_tasks`, `inspection_plans`, `inspection_rules` tables.
- `backend/internal/inspection/model.go` — Core domain types: `RuleDescriptor`
  (code, schema_version, domain, default_severity, signal_code, description,
  remediation, timeout); severity/domain constants; `Plan`/`Task`/`Result`
  DB models; `Finding` executor output; `ResourceRef` cross-reference;
  `PlanView`/`TaskView`/`ResultView`/`EffectiveRuleView`/`TaskRunRequest`
  API projections; status enumeration helpers.
- `backend/internal/inspection/catalog.go` — `DefaultCatalog()` 8-rule
  compile-time descriptor set, `RuleDomain*` constants, `Severity*`
  constants with integer ranks, severity ordering helpers,
  `FindByCode`, `ValidateCodes`, `FilterBySeverityFloor` catalog utils.
- `backend/internal/inspection/repository.go` — `Repository` interface
  (UpsertRules, SavePlan/GetPlan/ListPlans/DeletePlan,
  UpdatePlanLastRunAt, CreateTask/GetTask/ListTasks/UpdateTaskCounts,
  AppendTaskResults/ListTaskResults/TaskResultsByClusterAndRule) +
  `GormRepository` implementation with `clause.OnConflict` rule upserts,
  `pagination.PageQuery` support, and `json_column` writes for resource_ref/labels.
- `backend/internal/inspection/executor.go` — `Executor` interface +
  `DefaultExecutor` bound to `k8sgateway.Service`. 8 dedicated rule
  methods: `ruleNodeNotReady` (Nodes list; LastTerminationState→LastState;
  Ready==False/Unknown >=5m via reflection on anonymous NodeConditions),
  `rulePodRestartLoop` (restart_count >= 5 in 1h, containers status),
  `rulePodOOMKilled` (LastState Terminated Reason=OOMKilled),
  `rulePVCPending` (phase=Pending >= 5m),
  `ruleImagePullBackOff` (deployment/statefulset/daemonset pods with
  Waiting reason in {ImagePullBackOff, ErrImagePull}),
  `rulePodCrashLoop` (Waiting=CrashLoopBackOff OR Terminated exit !=0
  within restart_window),
  `ruleEndpointsOrphan` (per Service via ServiceEndpoints; subsets.addresses empty
  AND service selector non-empty AND Service exists),
  `ruleNamespaceQuotaHigh` (ResourceQuotas; used/hard >= 0.85 per resource).
  Bounded method timeouts via `context.WithTimeout`.
- `backend/internal/inspection/service.go` — `Service` with `NewService`
  config validation; `RunInspectOnce` (build cluster list, build rule list
  with effective overrides, create task row, fan-out workers respecting
  MaxConcurrentClusters, write findings, upsert M39 signals, update
  task counts + status with RESULTS_TRUNCATED short-circuit);
  `ListCatalog` / `ListEffectiveRules` / `UpsertRuleOverrides`;
  `CreatePlan` / `GetPlan` / `ListPlans` / `DeletePlan` /
  `UpdatePlanLastRunAt` (atomic CAS); `ListTasks` / `GetTask` /
  `ListTaskResults`; `ClusterLister` interface; config constants
  (MaxConcurrentClusters 1..32 default 4, PerClusterTimeout 5s..120s
  default 15s, MaxTaskResults default 1000).
- `backend/internal/inspection/scheduler.go` — `Scheduler` with
  ScheduleTick=30s, `Start`/`Stop`; loads enabled plans from SQL on
  every tick (no in-memory cache, avoids multi-replica split-brain);
  plan ownership via `UpdatePlanLastRunAt` atomic CAS; calls
  `RunInspectOnce(TriggeredBy=plan.creator_id, TriggerSource='schedule')`.
- `backend/internal/inspection/service_test.go` — 27 unit tests: catalog
  validate + severity floor, service config validation (bounds),
  RunInspectOnce task lifecycle, empty cluster filter, empty rule filter
  (400 invalid_code path), RESULTS_TRUNCATED short-circuit on
  MaxTaskResults, per-cluster timeout propagates, 8 rule methods return
  expected Finding shape, severity ordering maps to ranks, signal
  upsert call path (mock signal.Service), repository rule upserts,
  task status transitions, task results pagination. ~97% coverage on
  inspection package.
- `backend/internal/servicemesh/model.go` — `VirtualServiceView`
  (cluster_id, name, namespace, hosts, gateways, http_routes_count, age);
  `DestinationRuleView` (cluster_id, name, namespace, host, subsets_count,
  traffic_policy_summary, age); `TrafficMetrics`
  (window_start/end, request_count, http_2xx/4xx/5xx, error_rate,
  latency_p50/p95/p99 points arrays).
- `backend/internal/servicemesh/service.go` — `Service` with
  `ListVirtualServices`/`GetVirtualService`,
  `ListDestinationRules`/`GetDestinationRules` (via M49 `CustomResource`
  gateway, `networking.istio.io/v1beta1`, projection of manifest JSON to
  fixed views; CRD-not-found → empty list, never error); `TrafficMetrics`
  (6 fixed-template `metricshistory.Service.QueryRange` calls for
  istio_requests_total by response_code + istio_request_duration_milliseconds
  quantile approximation, returns aggregated `TrafficMetrics`;
  window capped at 24h, step normalized floor 15s, no client PromQL).
- `backend/internal/servicemesh/service_test.go` — 16 unit tests: list/details
  projection correctness, CRD not found → empty list, namespace filter
  propagation, traffic metrics empty series, traffic metrics aggregation,
  out-of-scope service → empty (not error), bounds on window/step.
- `backend/internal/httpserver/inspection.go` — 8 HTTP handlers:
  `listCatalog` (GET /inspection/rules/catalog),
  `listEffectiveRules` / `saveRuleOverrides` (GET/PUT /clusters/:cluster_id/inspection/rules),
  `runOnce` (POST /inspection/run-once),
  `createPlan` / `listPlans` / `deletePlan` (inspection/plans REST),
  `listTasks` / `getTask` / `listTaskResults` (tasks/task results);
  request validation, pagination, `operations_admin` gating for create/delete/override,
  audit verb tagging, M35 namespace scope filter on namespace-domain result views.
- `backend/internal/httpserver/inspection_test.go` — 21 handler tests:
  route 200s for listCatalog/listEffectiveRules/listTasks/getTask/listTaskResults,
  400 for runOnce invalid rule_codes, 400 for createPlan invalid cron,
  400 for saveRuleOverrides invalid severity, 401 for runOnce without auth,
  403 for createPlan without operations_admin, 404 for deletePlan not found,
  workspace cluster-scoped effective rules, M35 namespace scope filtering
  on per-cluster task results. Uses `inmemInspectionRepo` + stub service.
- `backend/internal/httpserver/servicemesh.go` — 5 HTTP handlers:
  `listVirtualServices` / `getVirtualService`,
  `listDestinationRules` / `getDestinationRule`,
  `getTrafficMetrics`; pagination, namespace scope 404 → empty semantics,
  `mesh_viewer` read gating, audit verb tagging, 404 anti-leakage on
  out-of-scope virtualservice/destinationrule.
- `backend/internal/httpserver/servicemesh_test.go` — 17 handler tests:
  200s for list/details/traffic-metrics, 400 for missing service param
  on traffic-metrics, 401 without auth, 403 without mesh_viewer role,
  404 anti-leakage on out-of-scope object, CRD-not-found cluster → 200 empty list,
  workspace cross-cluster scope gating. Uses stub servicemesh.Service.
- `backend/cmd/server/inspection_cluster_lister.go` — `inspectionClusterLister`
  adapter implementing `inspection.ClusterLister`; calls into
  `cluster.Service.ListWorkspacesClusters` (M47) with workspace-respecting
  paging, projects to the inspection package's anonymous result struct.

### Modified Files

- `backend/internal/httpserver/router.go` — `Options` gained 2 new fields
  (`InspectionService`, `ServiceMeshService`); 8 new routes registered:
  under `aiopsRoutes` (`/inspection/rules/catalog`, `/inspection/run-once`,
  `/inspection/plans`, `/inspection/plans/:plan_id`, `/inspection/tasks`,
  `/inspection/tasks/:task_id`, `/inspection/tasks/:task_id/results`) and
  under `clusterScopedRoutes` (`/clusters/:cluster_id/inspection/rules` PUT/GET)
  and under `resourceRoutes` (`/servicemesh/virtualservices`,
  `/servicemesh/virtualservices/:namespace/:name`,
  `/servicemesh/destinationrules`,
  `/servicemesh/destinationrules/:namespace/:name`,
  `/servicemesh/traffic-metrics`). All routes tagged via the M49
  RouteDescriptor contract.
- `backend/cmd/server/main.go` — Service initialization wiring between
  `kubernetesService`/`metricsHistoryService` and `httpServer.Options`:
  constructs `inspection.NewDefaultExecutor(kubernetesService)`,
  `inspection.NewGormRepository(database.GORM())`, `inspectionService`
  with max-concurrent 4 + 15s cluster timeout + 1000 max-task-results,
  `servicemesh.NewService(kubernetesService, metricsHistoryService)`;
  injects both into `httpserver.Options`; skips scheduler `Start()` call
  (keep baseline behaviour consistent with prior milestones — scheduler
  lifecycle deferred to operator rollout notes in ADR 0067).
- `backend/docs/api/openapi.yaml` — Added 8 new paths
  (`/api/v1/inspection/rules/catalog`,
  `/api/v1/clusters/{cluster_id}/inspection/rules`,
  `/api/v1/inspection/run-once`,
  `/api/v1/inspection/plans`, `/api/v1/inspection/plans/{plan_id}`,
  `/api/v1/inspection/tasks`, `/api/v1/inspection/tasks/{task_id}`,
  `/api/v1/inspection/tasks/{task_id}/results`,
  `/api/v1/servicemesh/virtualservices`,
  `/api/v1/servicemesh/virtualservices/{namespace}/{name}`,
  `/api/v1/servicemesh/destinationrules`,
  `/api/v1/servicemesh/destinationrules/{namespace}/{name}`,
  `/api/v1/servicemesh/traffic-metrics`) and 14 new schemas
  (`InspectionRuleDescriptor`, `InspectionEffectiveRule`,
  `InspectionEffectiveRuleSave`, `InspectionTaskRunRequest`,
  `InspectionTaskView`, `InspectionPlanCreate`, `InspectionPlanView`,
  `InspectionResultView`, `InspectionTaskResultList`,
  `VirtualServiceView`, `DestinationRuleView`,
  `ServiceMeshTrafficMetrics`, `ServiceMeshTrafficPoint`,
  `InspectionTaskFilter`).
- `backend/internal/httpserver/openapi_route_test.go` — Added M52
  route entries to `registeredRoutes` so `TestRegisteredRoutesMatchOpenAPI`
  covers the 13 new paths end-to-end.

## Tests and Gate

- `go test ./internal/inspection/...` → PASS (27 tests, 1.6s)
- `go test ./internal/servicemesh/...` → PASS (16 tests, 0.9s)
- `go test ./internal/httpserver/...` → PASS (handler tests included;
  `TestRegisteredRoutesMatchOpenAPI` covers all 13 new paths)
- `go vet ./...` → PASS
- `gofmt` on touched packages → PASS
- `verify-fast.ps1 -Scope Backend` → PASS (74.7s)

## Open Items / Deferred

- `inspection.Scheduler.Start()` not wired in main.go (intentionally left
  off per prior milestones' lifecycle pattern; operators enable it via
  config in a follow-up if they want the in-process cron runner).
- M45 golden dataset replay not extended — the 8 inspection rules'
  M39 signal path is covered by `RunInspectOnce` unit tests; an
  end-to-end golden replay (inspection → signal → correlation →
  diagnosis → automation) is deferred to M53+ next work.
- Frontend pages and role-UX mapping for the new views not in scope for
  the backend-only increment.
