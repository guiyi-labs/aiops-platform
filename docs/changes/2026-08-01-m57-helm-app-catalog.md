# M57: Helm Application Catalog + Controlled Deploy Plans

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0069](../adr/0069-helm-application-catalog-and-controlled-deploy.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 76.27s; backend=True frontend=True manifests=True)

## Summary

Delivered the M57 backend increment: a simplified Helm application
catalog with M19 controlled-operation deploy plans. M57 opens Phase 4
(delivery & ops integration) of the post-M45 roadmap. Before M57 the
platform could browse Kubernetes resources (M49 CRD discovery) and
promote workloads across clusters (M19 cross-cluster promotion), but
had no way for operators to discover, browse, and deploy Helm charts
in a controlled manner. M57 delivers:

1. **Helm repository CRUD** (`internal/appcatalog.Service.
   CreateRepository` / `GetRepository` / `ListRepositories` /
   `DeleteRepository`) — registers Helm chart repositories with
   optional basic-auth credentials. Credentials are stored in a
   `credentials_json` JSONB column on `helm_repositories` and are
   NEVER returned in API responses (structurally impossible — the
   `RepositoryView` projection has no credentials field; only a
   `has_auth` boolean is exposed). Repository write operations
   (create/delete) require the `system_ops_admin` role; reads are
   any-auth.
2. **Chart listing/detail** (read-only `index.yaml` fetch) —
   `Service.ListCharts` and `Service.GetChart` fetch chart metadata
   live from each repository's `index.yaml` over HTTP. The
   `HTTPIndexSource` implementation imposes a 10 MiB body limit and a
   15-second timeout. No Helm SDK dependency — chart metadata is plain
   HTTP + YAML parse. No caching; metadata is always fresh.
3. **M19 controlled-operation deploy plans** — preview builds a Flux
   `HelmRelease` CR manifest (`helm.toolkit.fluxcd.io/v2beta1`),
   validates it via a server-side dry-run on the target cluster, and
   persists the plan with a one-time confirmation token (SHA-256
   hashed). Execute claims the plan (row-level lock + constant-time
   token compare + idempotency key check), applies the HelmRelease CR
   via the generic M49 `CreateResource` path, and marks the plan
   succeeded/failed. A 409 conflict during execute is treated as
   success (HelmRelease already exists from a previous timed-out
   attempt). The manifest is built once at preview and applied
   verbatim at execute — deterministic, no re-rendering.

Authorization reuses the existing `RouteDescriptor` pattern and the
M35 namespace scope. No new roles, no new middleware. The 2D
authorization matrix is intact — the app-catalog is a platform-level
resource, not a per-cluster or per-namespace resource (the target
cluster/namespace is in the request body for preview, or in the plan
record for execute).

## Files Changed

### New Files

- `backend/internal/appcatalog/model.go` — `Repository` (helm repo
  with `CredentialsJSON JSON` tagged `json:"-"` so credentials are
  never serialized), `Plan` (deploy plan with M19 state machine
  fields: `Status`, `ConfirmationTokenHash`, `IdempotencyKey`,
  `LockedAt`, `ExecutedAt`, `LastError`, `ExpiresAt`), request/
  response types (`CreateRepositoryRequest`, `DeployPreviewRequest`,
  `ActorRef`), status constants (`StatusAwaitingConfirmation`,
  `StatusExecuting`, `StatusSucceeded`, `StatusFailed`,
  `StatusExpired`), `RepositoryView` projection (no credentials
  field), `ChartSummary` / `ChartDetail` types, `JSON` type alias
  for `json.RawMessage`.
- `backend/internal/appcatalog/repository.go` — `DataStore`
  interface (Helm repo CRUD + plan lifecycle: `SavePlan`, `GetPlan`,
  `ListPlans`, `ClaimPlan`, `CompletePlan`, `FailPlan`,
  `ExpireStalePlans`) and `GormRepository` implementation.
  `ClaimPlan` uses `clause.Locking{Strength: "UPDATE"}` for row-level
  locking and `subtle.ConstantTimeCompare` for token comparison.
  Sentinels: `ErrRepoNotFound`, `ErrRepoNameExists`,
  `ErrPlanNotFound`, `ErrConfirmationInvalid`, `ErrIdempotencyMismatch`,
  `ErrPlanNotClaimable`.
- `backend/internal/appcatalog/service.go` — `Service` with
  `KubernetesSource` (namespace check + HelmRelease dry-run + create)
  and `ChartIndexSource` (index.yaml fetch) interfaces.
  `Preview` validates the request, checks the target namespace
  exists, fetches chart metadata, checks no existing HelmRelease
  with the same name, builds the CR manifest, runs a server-side
  dry-run, and persists the plan with a one-time confirmation token.
  `Execute` claims the plan, creates the HelmRelease CR, and
  completes/fails the plan. `NewService(k8s, store)` constructor;
  `NewTestService` for tests. `HTTPIndexSource` implementation (10
  MiB limit, 15s timeout, basic-auth from `credentials_json`).
  `buildHelmReleaseManifest` constructs the Flux HelmRelease CR YAML.
  Plan TTL default 30 minutes; claim TTL default 5 minutes.
- `backend/internal/appcatalog/service_test.go` — 32 unit tests
  covering repository CRUD (create valid/invalid name/invalid URL/
  invalid actor, delete not-found/success, view redacts credentials,
  has-credentials), chart listing (success/repo-not-found/repo-
  unreachable), chart detail (success/not-found/empty-name), preview
  (success/namespace-missing/chart-not-found/invalid-request/dry-
  run-fails), execute (success/invalid-token/invalid-idempotency/
  plan-not-found/idempotent-replay), list plans (success/invalid-
  cluster-id), manifest building (valid/invalid-YAML), HelmRelease
  path resolution, repo request validation, chart entry lookup.
- `backend/internal/httpserver/appcatalog.go` — `appCatalogHandler`
  with 10 methods: `listRepositories`, `createRepository`,
  `getRepository`, `deleteRepository`, `listCharts`, `getChart`,
  `listPlans`, `getPlan`, `previewDeploy`, `executeDeploy`. Uses
  `decodeStrictJSON` for write bodies (rejects unknown fields),
  `requestctx.MetadataFrom` for actor identity, `setAuditTarget`
  for audit context, and a `writeError` helper that maps sentinel
  errors to HTTP status codes (404 for not-found, 409 for name-
  conflict, 410 for expired plan, 400 for invalid request, 503 for
  upstream kubernetes/index failures).
- `backend/internal/httpserver/appcatalog_test.go` — 24 handler
  tests covering each route's success + error path: create 201/400-
  invalid-json/400-invalid-name, list 200, get 200/404/400-invalid-id,
  delete 204/404, list-charts 200, get-chart 200/404, preview 201/
  400-invalid-request/400-namespace-missing, execute 200/400-invalid-
  plan-id/400-missing-token, get-plan 200/404, list-plans 200/400-
  missing-cluster-id/400-invalid-cluster-id, write-error sentinel
  mapping.
- `backend/migrations/000038_app_catalog.up.sql` — 2 new tables:
  `helm_repositories` (id, name UNIQUE, display_name, url,
  credentials_json JSONB, created_by, created_at, updated_at) and
  `app_catalog_plans` (id VARCHAR(36) PK, status, repo_id FK,
  chart_name, chart_version, target_cluster_id, target_namespace,
  release_name, values_yaml TEXT, chart_metadata JSONB,
  release_manifest JSONB, deploy_diff JSONB, confirmation_token_hash
  BYTEA, requested_by_user_id, requested_by_name, expires_at,
  idempotency_key, locked_at, executed_at, last_error, created_at,
  updated_at). Indexes on `helm_repositories(name)`,
  `app_catalog_plans(status)`, `app_catalog_plans(target_cluster_id,
  target_namespace)`, `app_catalog_plans(idempotency_key)`.
- `backend/migrations/000038_app_catalog.down.sql` — `DROP TABLE IF
  EXISTS app_catalog_plans; DROP TABLE IF EXISTS helm_repositories;`
  (order matters: plans reference repos via FK).
- `docs/adr/0069-helm-application-catalog-and-controlled-deploy.md`
  — 6 key decisions: (1) no Helm SDK — index.yaml HTTP fetch only;
  (2) Flux HelmRelease CR as deploy target (already in M49 CRD
  whitelist); (3) M19 controlled-operation contract reuse (mirrors
  promotion/backup/restore/maintenance); (4) credentials never in
  API responses (structurally impossible via `RepositoryView`); (5)
  authorization: SystemOpsAdmin for writes, any-auth for reads; (6)
  10 routes registered on `v1` (not `resourceRoutes`) — same
  pattern as promotion routes, since app-catalog endpoints don't
  take a `:cluster_id` path parameter.

### Modified Files

- `backend/internal/httpserver/router.go` — `Options` gained
  `AppCatalogService *appcatalog.Service`; 10 new routes registered
  on `v1` (not `resourceRoutes`, mirroring promotion routes): 4
  repository CRUD (`GET/POST /app-catalog/repositories`,
  `GET/DELETE /app-catalog/repositories/:repo_id`), 2 chart read
  (`GET /app-catalog/repositories/:repo_id/charts`,
  `GET /app-catalog/repositories/:repo_id/charts/:chart_name`), 2
  plan read (`GET /app-catalog/plans`,
  `GET /app-catalog/plans/:plan_id`), 2 plan write
  (`POST /app-catalog/plans/preview`,
  `POST /app-catalog/plans/:plan_id/execute`). Write operations
  (create repo, delete repo, preview, execute) require
  `rolesSystemOpsAdmin`; reads are any-auth. All routes tagged with
  audit verbs (`app_catalog.repositories.{list,create,read,delete}`,
  `app_catalog.charts.{list,read}`, `app_catalog.plans.{list,read,
  preview,execute}`) per ADR 0008.
- `backend/cmd/server/main.go` — Service initialization: constructs
  `appcatalog.NewService(kubernetesService, appcatalog.NewGormRepository(database.GORM()))`
  and injects into `httpserver.Options.AppCatalogService`. Import
  of `appcatalog` package added.
- `backend/internal/httpserver/openapi_route_test.go` — Wired
  `AppCatalogService` with `appcatalog.NewService(nil, nil)` so
  `TestRegisteredRoutesMatchOpenAPI` covers all 10 M57 routes
  end-to-end. Nil dependencies are acceptable because the test only
  inspects route registration.

### OpenAPI Changes

- `docs/api/openapi.yaml` — 10 new paths under `/api/v1/app-catalog`
  and new schemas: `HelmRepositoryView`, `HelmRepositoryList`,
  `CreateHelmRepositoryRequest`, `ChartSummary`, `ChartDetail`,
  `ChartList`, `AppCatalogPlan`, `AppCatalogPlanList`,
  `DeployPreviewRequest`, `ExecuteDeployRequest`,
  `DeployPreviewResponse`, `ExecuteDeployResponse`. The
  `HelmRepositoryView` schema explicitly documents that credentials
  are never included (`has_auth` boolean is the only credential-
  related field).

## Tests and Gate

- `go test ./internal/appcatalog/...` → PASS (32 service tests)
- `go test ./internal/httpserver/...` → PASS (24 app-catalog handler
  tests; `TestRegisteredRoutesMatchOpenAPI` covers all 10 M57 paths)
- `go vet ./...` → PASS
- `gofmt` on touched packages → PASS (fixed
  `openapi_route_test.go` formatting before gate)
- `verify-fast.ps1 -Scope All` → PASS (76.27s; backend=True
  frontend=True manifests=True)

## Open Items / Deferred

- **No values schema validation**: the platform validates that
  `values_yaml` is valid YAML but does not validate it against the
  chart's `values.schema.json`. The server-side dry-run catches
  admission-time errors, but semantic validation errors surface
  only at reconciliation time. Documented in ADR 0069 §Negative.
- **Repo index freshness**: chart metadata is fetched live on each
  request — no caching. For repos with large indexes this may add
  latency. The 15-second timeout bounds the worst case. Documented
  in ADR 0069 §Negative.
- **Single HelmRelease per plan**: the plan deploys exactly one
  chart to one namespace. Multi-chart bundles are not supported
  (by design — the roadmap calls for a "simplified" catalog).
- **Frontend app-catalog page not in scope** for the backend-only
  increment. The `AppCatalogView.vue` and `OperationWizard.vue`
  components are deferred to a frontend increment.
- **No Helm SDK dependency**: by design. Chart metadata comes from
  `index.yaml` HTTP fetch; deployment targets a Flux HelmRelease CR
  (already in the M49 CRD whitelist). The Flux helm-controller
  handles reconciliation, retries, and rollbacks — the platform
  just creates the CR.
- **Plan expiry sweep**: `ExpireStalePlans` is implemented on the
  `DataStore` but no background goroutine calls it yet. A future
  milestone could add a periodic sweep (mirrors the M19 promotion
  plan expiry pattern).
