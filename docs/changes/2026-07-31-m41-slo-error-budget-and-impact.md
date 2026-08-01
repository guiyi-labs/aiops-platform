# M41: SLO, Error Budget And Impact

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0056](../adr/0056-slo-error-budget-and-impact.md)
- Fast gate: 67.02s (30 backend packages including `slo`, 81 frontend
  tests/18 files, Compose/Kustomize contracts)

## Summary

Introduced server-owned SLI templates, versioned SLO definitions, deterministic
evaluation with explicit missing-data handling, and burn-alert transitions that
feed the existing M27 alert lifecycle. M41A defines the data model and
migration 000030; M41B implements the deterministic SLI evaluator with counter
reset, sparse-data and clock-boundary handling; M41C implements the SLO service
with versioned CRUD and burn-transition emission; M41D wires the HTTP routes
and OpenAPI schemas; M41E adds unit tests, ADR 0056 and this change record.

The native M21-M31 signal path is unchanged; M41 is a deterministic evaluator
that reads from the M37 capability providers and writes to the `slo_*` tables.
M41 begins the impact layer that M42 (correlation/RCA) and M43 (AI
investigator) build on.

No public API contract was broken beyond the eight new `aiops/slos` routes
documented in OpenAPI.

## Changes

### M41A — Data model and migration

#### New files

- `backend/internal/slo/model.go`: defines `SLITemplate` (3 values:
  `request_success_ratio`, `request_latency_target_ratio`,
  `workload_readiness`), `MissingDataPolicy` (2 values: `unavailable`,
  `fail_open`), `EvaluationState` (5 values: `healthy`, `burning_slow`,
  `burning_fast`, `breached`, `unavailable`), `EvaluationCoverage` (3 values:
  `complete`, `partial`, `unavailable`), `ServiceRef`, `ActorRef`,
  `Definition`, `Evaluation`, `DefinitionFilter`, `DefinitionListResponse`,
  `EvaluationFilter`, `EvaluationListResponse`, `SLOView`,
  `CreateDefinitionInput`, `PatchDefinitionInput`, and the bound constants
  (`MinObjective`, `MaxObjective`, `MinRollingWindowSeconds`, etc.).
- `backend/internal/slo/catalog.go`: defines `TemplateDescriptor`,
  the compiled `catalog` map, `LookupTemplate`, `AllTemplates`,
  `ValidateDefinition` (the single validation entry point),
  `ValidateCreate`, `DefaultMissingDataPolicy`. The catalog is the source of
  truth for which templates exist, what they require and which missing-data
  policies they admit.
- `backend/internal/slo/repository.go`: defines the `Repository` interface,
  `GormRepository` (with `definitionRow`/`evaluationRow` GORM models,
  `ON CONFLICT DO NOTHING` for idempotent evaluation inserts, partial unique
  index `uq_slo_definitions_active` for at-most-one-active-definition) and
  `NopRepository` (testing/disabled mode).
- `backend/migrations/000030_slo_definitions_and_evaluations.up.sql`: creates
  `slo_definitions` (CHECK constraints on `template`,
  `missing_data_policy`, `objective`, `rolling_window_seconds`,
  `fast_burn_rate`, `slow_burn_rate`, burn window bounds, and
  `fast_burn_window_seconds <= slow_burn_window_seconds`; partial unique index
  `uq_slo_definitions_active` for at-most-one-active-definition; query indexes
  on cluster/namespace, owner, template) and `slo_evaluations` (CHECK
  constraints on `state`, `coverage`, `window_end > window_start`,
  `total_events >= 0`, `good_events <= total_events`, `ratio` and
  `target_ratio` in `[0,1]`, `error_budget` in `[0,1]`; query indexes on
  `(slo_id, version, evaluated_at DESC)`, `(state, evaluated_at DESC)`,
  `(slo_id, window_start, window_end)`).
- `backend/migrations/000030_slo_definitions_and_evaluations.down.sql`: drops
  both tables and indexes.

### M41B — SLI template evaluator

#### New files

- `backend/internal/slo/evaluator.go`: defines `MetricsSource` interface,
  `SLISeries`, `Sample`, `Evaluator`, `BurnAlertSink`, `BurnTransition`,
  `NopBurnAlertSink`, sentinel errors (`ErrEvaluationInvalidInput`,
  `ErrEvaluationSourceUnavailable`), `MinSamplesForComplete`. The evaluator is
  pure: the same Definition and MetricsSource output yield the same
  Evaluation. Counter resets are detected as monotonicity violations and
  handled as "counter went to 0 and started again". Sparse data produces
  `CoveragePartial` (some samples) or `CoverageUnavailable` (no samples).
  Clock boundaries use inclusive `window_start` and exclusive `window_end`.
  Missing data is fail-closed by default; the only fail-open path is
  `workload_readiness` with explicit operator opt-in, and even then
  `Coverage` remains `Unavailable` so the fail-open is auditable.
  `classifyState` derives the state from ratio, burn rate and the definition's
  fast/slow thresholds (precedence: breached > burning_fast > burning_slow >
  healthy). `computeRemainingBudget` and `computeBurnRate` handle the
  zero-error-budget (objective == 1.0) case explicitly.

### M41C — SLO service

#### New files

- `backend/internal/slo/service.go`: defines `Service`, `ServiceOption`,
  `WithBurnAlertSink`, `WithNow`, sentinel errors
  (`ErrEvaluatorUnavailable`, `ErrDefinitionDisabled`). The service is the
  only writer to `slo_evaluations` and the only caller of
  `Evaluator.Evaluate`. `CreateDefinition` stamps version=1 and persists.
  `PatchDefinition` requires an actor ID and increments Version via the
  repository. `DeleteDefinition` marks `enabled=false` (row retained).
  `EvaluateSLO` looks up the definition first (404 takes precedence over
  503), checks enabled, checks evaluator availability, runs the evaluation,
  persists even on source-unavailable (auditable fact), reads the previous
  state, and emits a `BurnTransition` to the sink only on state change
  (steady-state healthy or unavailable does not churn the sink). The sink is
  best-effort: a sink failure does not roll back the evaluation.
  `ListDefinitions`/`ListEvaluations` clamp limit to `[1, 200]` (default 100)
  and set `Truncated` when `len(items) < total`.

### M41D — HTTP routes and OpenAPI

#### New files

- `backend/internal/httpserver/slo.go`: defines `sloHandler` with eight
  handlers (`listSLITemplates`, `listSLODefinitions`, `createSLODefinition`,
  `getSLODefinition`, `patchSLODefinition`, `deleteSLODefinition`,
  `evaluateSLO`, `listSLOEvaluations`) and `writeSLOError` (stable error
  mapping: 404 for not found, 409 for disabled/duplicate, 503 for evaluator
  unavailable, 400 for invalid input, 500 otherwise). Create/Patch extract
  the actor from `requestctx.MetadataFrom`; `firstNonEmpty` resolves the
  display name. List handlers parse query params with explicit 400 on bad
  values.

#### Modified files

- `backend/internal/httpserver/router.go`: adds `slo` import, `SLOService`
  option, and registers eight routes under `/api/v1/aiops/slos` inside the
  existing `aiopsRoutes` group (gated by `options.SignalService != nil` and
  `options.SLOService != nil`). Writes require `rolesSystemOpsAdmin`; reads
  are open to any authenticated user. Audit actions and resources are
  registered for each write.
- `backend/internal/httpserver/openapi_route_test.go`: adds `slo` import and
  wires `SLOService: slo.NewService(slo.NopRepository{}, nil)` so the route
  contract test covers the eight new routes.
- `docs/api/openapi.yaml`: adds `slo` tag, eight paths
  (`/api/v1/aiops/slos/templates`, `/api/v1/aiops/slos`,
  `/api/v1/aiops/slos/{id}`, `/api/v1/aiops/slos/{id}/evaluate`,
  `/api/v1/aiops/slos/{id}/evaluations`) and ten schemas
  (`SLITemplateCatalog`, `SLITemplateDescriptor`, `SLOServiceRef`,
  `SLOActorRef`, `SLODefinition`, `SLODefinitionCreate`,
  `SLODefinitionPatch`, `SLODefinitionList`, `SLOEvaluation`,
  `SLOEvaluationList`). All enums, bounds and required fields match the
  migration CHECK constraints and the `ValidateDefinition` rules.

### M41E — Unit tests, ADR, change record, docs

#### New files

- `backend/internal/slo/evaluator_test.go`: 14 test functions covering the
  healthy path, breach on ratio below objective, counter reset handling,
  missing-data fail-closed, missing-data fail-open for workload_readiness,
  fail-open rejection for request templates, nil source produces unavailable,
  source error produces unavailable, disabled definition rejected,
  burn-rate fast, burn-rate slow, partial coverage with state unavailable,
  samples outside window excluded, zero error budget (objective == 1.0),
  `computeRemainingBudget` table tests, `classifyCoverage` table tests,
  `chooseStep` bounds.
- `backend/internal/slo/service_test.go`: 13 test functions (with
  `memoryRepository` and `captureSink` helpers) covering create success,
  create invalid input (6 subtests), patch increments version, patch
  requires actor, delete disables, delete not found, evaluate no evaluator,
  evaluate disabled definition, evaluate persists and emits transition,
  evaluate no transition on steady state, evaluate sink failure does not
  rollback, list pagination, list limit clamping, list evaluations version
  filter, nop sink.
- `backend/internal/slo/catalog_test.go`: 4 test functions covering
  `ValidateDefinition` (28 subtests), `ValidateCreate` requires creator/owner,
  `LookupTemplate`, `AllTemplates`, `DefaultMissingDataPolicy`.
- `backend/internal/httpserver/slo_test.go`: 11 test functions covering list
  templates returns 200, list definitions returns 200, list definitions
  invalid cluster_id 400, get definition not found 404, get definition
  invalid ID 400, create invalid body 400, evaluate requires evaluator 503,
  evaluate definition not found 404, list evaluations returns 200, list
  evaluations invalid version 400, delete returns 204, nil service returns
  503 (7 subtests for each endpoint).
- `docs/adr/0056-slo-error-budget-and-impact.md`: 7 decisions documented.

## Verification

- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 67.02s: 30 backend
  packages vet/test green (including `slo` at 0.555s), 81 frontend tests / 18
  files green, Compose and Kustomize contracts green.
- `go test ./internal/slo/ -count=1` passes (14 evaluator + 13 service +
  4 catalog = 31 test functions).
- `go test ./internal/httpserver/ -run TestSLOHandler -count=1` passes (11
  handler test functions).
- `go test ./internal/httpserver/ -run TestRegisteredRoutesMatchOpenAPI
  -count=1` passes (bidirectional route/OpenAPI consistency for the eight new
  SLO routes).

## Deferred

- Real Prometheus/Loki integration tests for the `MetricsSource` adapter
  (needs a running provider or a recorded-fixture harness).
- Real-kind E2E for the burn-transition-to-M27 path (needs a multi-worker
  kind cluster with metrics).
- Frontend SLO management UI (list/create/patch/delete definitions, view
  evaluations, burn alert badge).
- Background evaluation worker (cron-driven `EvaluateSLO` for all enabled
  definitions).
- Multi-window burn rate (separate fast/slow evaluations over their own
  windows; the data model already stores the window lengths).
- Production wiring in `cmd/server/main.go` (the routes are registered and
  the contract is verified, but production deployment requires constructing
  the SLO service with a real repository and evaluator).

## Next steps

- M42 (multi-signal correlation and deterministic RCA) per
  `docs/kubesphere-optimization-plan.md` §5.
- Or M43 (cited AI investigator) if M42 is deferred.
- Or M44 (safe automation and post-check) once M42/M43 land.
