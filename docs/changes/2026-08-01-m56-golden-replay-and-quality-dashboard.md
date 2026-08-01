# M56: Golden Dataset Replay + Quality Dashboard

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0068](../adr/0068-golden-dataset-replay-and-quality-dashboard.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 71.34s; backend=True frontend=True manifests=True)

## Summary

Delivered the M56 backend increment: the golden dataset replay runner
and quality-report HTTP surface that closes the M45 (ADR 0060) quality
contract. M45 defined the versioned golden dataset (3 scenarios:
mandatory 10-step end-to-end + 2 negative companions) and the quality
report schema, but did not deliver the execution engine or the operator
HTTP surface. M56 delivers both:

1. **Replay runner** (`internal/golden.ReplayRunner`) — a deterministic
   engine-contract verifier that replays the golden dataset against the
   current M39-M44 package versions and status enumerations. Produces
   per-scenario, per-step pass/fail results. Pure Go: no Kubernetes
   client, no Prometheus, no AI provider — identical contracts + dataset
   produce identical results.
2. **Quality report storage** (`internal/golden.FileReportStorage`) —
   timestamped JSON files under `.artifacts/quality-report/`, sorted by
   name to find the latest. Mirrors the audit-archive pattern (ADR 0031):
   artifacts survive database restores, no migration needed.
3. **Async replay service** (`internal/golden.Service`) — in-memory task
   tracking with background goroutine execution. `RunReplay` returns a
   task ID immediately (202); the background goroutine runs the replay
   (5-minute timeout), loads the previous baseline for before/after
   comparison, builds per-scenario `ScenarioQuality` (delta classified
   as preserved/improved/regressed/unchanged), and persists the report.
4. **HTTP surface** — `GET /api/v1/aiops/quality-report` (read latest
   report, any-auth) and `POST /api/v1/aiops/quality-report/run`
   (trigger async replay, `system_ops_admin` only). Both tagged with
   audit verbs per ADR 0008.

Authorization reuses the existing `aiopsRoutes` middleware chain (bearer
auth + audit). No new roles, no new middleware, no new database tables.
The 2D authorization matrix is intact — the quality report is a
platform-level artifact.

## Files Changed

### New Files

- `backend/internal/golden/runner.go` — `ReplayRunner` with
  `EngineContracts` (versions + valid plan/verification statuses),
  `StepResult`, `ScenarioResult` types. `Run` iterates scenarios and
  steps with context cancellation. `ReportVersion = "1.0"` constant.
  Deterministic: no network, no time-dependent logic.
- `backend/internal/golden/storage.go` — `ReportStorage` interface
  (`Save` / `LoadLatest`) + `FileReportStorage` (timestamped JSON files
  in a fixed directory) + `NopReportStorage` (testing). `ErrNoReport`
  sentinel. `NowFunc` clock for test injection.
- `backend/internal/golden/service.go` — `Service` wrapping runner +
  storage with async replay. `RunReplay` (returns task ID, launches
  background goroutine), `GetLatestReport`, `GetTask`. `executeReplay`
  loads baseline, runs replay, builds `ScenarioQuality` per scenario,
  persists report, updates task status. `ReplayTask` / `ReplayTaskView`
  / `ReplayTaskStatus` types. `replayTaskTracker` (in-memory, mutex-
  guarded). 5-minute execution timeout.
- `backend/internal/golden/service_test.go` — 26 unit tests covering
  runner (all-pass, 10-step mandatory, missing signal version, invalid
  plan/verification status, context cancel), storage (save/load, empty,
  non-existent dir, latest-pick, nop), service (no-report, success,
  regression detection, unknown task), quality (classify delta,
  summarize, end-to-end), dataset integrity/determinism.
- `backend/internal/httpserver/golden.go` — `goldenHandler` with
  `getQualityReport` (GET, 200/404/503) and `runQualityReplay` (POST,
  202/503). Nil-service → 503 `GOLDEN_UNAVAILABLE`. No-report → 404
  `QUALITY_REPORT_NOT_FOUND`.
- `backend/internal/httpserver/golden_test.go` — 6 handler tests:
  GET 404 when no report, GET 503 when service nil, GET 200 with report,
  POST 202 accepted, POST 503 when service nil, POST produces report
  (polls GetLatestReport after trigger).
- `backend/cmd/server/golden_contracts.go` — `goldenEngineContracts()`
  adapter that reads live version constants and status enumerations from
  the M39-M44 engine packages (signal, topology, SLO, correlation,
  investigator, automation). Keeps the golden package free of engine
  imports (avoids cycles). Same adapter pattern as M52's
  `inspection_cluster_lister.go`.

### Modified Files

- `backend/internal/httpserver/router.go` — `Options` gained
  `GoldenService *golden.Service`; 2 new routes registered under
  `aiopsRoutes`: `GET /quality-report` (any-auth, audit
  `aiops.quality_report.read`) and `POST /quality-report/run`
  (`system_ops_admin` only, audit `aiops.quality_report.run`). Tagged
  via the M49 RouteDescriptor contract.
- `backend/cmd/server/main.go` — Service initialization: constructs
  `golden.NewFileReportStorage(".artifacts/quality-report")` and
  `golden.NewService(goldenEngineContracts(), storage, logger)`;
  injects into `httpserver.Options`. Import of `golden` package added.
- `backend/internal/httpserver/openapi_route_test.go` — Wired
  `GoldenService` with `golden.NewService(golden.EngineContracts{},
  golden.NopReportStorage{}, logger)` so `TestRegisteredRoutesMatchOpenAPI`
  covers both M56 routes end-to-end.
- `backend/internal/inspection/service_test.go` — Fixed data race in
  `fakeExecutor.Execute` (concurrent append to `calls` slice without
  synchronization). Added `sync.Mutex` to `fakeExecutor` and locked
  around the append. Pre-existing M52 flaky test; surfaced under
  concurrent `MaxConcurrentClusters=2` execution.

### OpenAPI Changes

- `docs/api/openapi.yaml` — 2 new paths
  (`/api/v1/aiops/quality-report` GET,
  `/api/v1/aiops/quality-report/run` POST) and 5 new schemas
  (`QualityReport`, `EngineVersions`, `ScenarioQuality`,
  `QualitySummary`, `QualityReplayResponse`).

## Tests and Gate

- `go test ./internal/golden/...` → PASS (26 tests, 0.6s)
- `go test ./internal/httpserver/...` → PASS (6 golden handler tests;
  `TestRegisteredRoutesMatchOpenAPI` covers both M56 paths)
- `go vet ./...` → PASS
- `gofmt` on touched packages → PASS
- `verify-fast.ps1 -Scope All` → PASS (71.34s; backend=True
  frontend=True manifests=True)

## Open Items / Deferred

- File-based storage has no retention policy. Reports accumulate under
  `.artifacts/quality-report/`. A future milestone could add a retention
  sweep (mirror audit-archive GC).
- In-memory task tracking is non-durable. A server restart during a
  replay loses the in-flight task (but not persisted reports). Durable
  task tracking deferred — replays are short-lived and retriggerable.
- Frontend quality-dashboard page not in scope for the backend-only
  increment.
- The runner verifies engine contracts, not live Kubernetes/AI behavior.
  Live integration testing remains the responsibility of CI E2E tests.
