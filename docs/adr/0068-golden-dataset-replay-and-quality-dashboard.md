# ADR 0068: Golden Dataset Replay + Quality Dashboard (M56)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M56
- Supersedes: none
- Related: ADR 0060 (versioned AIOps golden dataset and quality report),
  ADR 0059 (policy-constrained automation), ADR 0058 (cited AI
  investigator), ADR 0057 (multi-signal correlation), ADR 0056 (SLO and
  error budget), ADR 0055 (temporal topology), ADR 0054 (unified signal
  model), ADR 0049 (route descriptor contract and RBAC inventory)

## Context

M45 (ADR 0060) introduced the versioned AIOps golden dataset and the
quality report schema as an offline evaluation contract. The dataset
contains 3 scenarios (mandatory 10-step end-to-end + 2 negative
companions: misattribution prevention, partial/unknown fail-closed) and
the quality report records before/after comparison per scenario with
aggregated summary metrics. However, M45 delivered only the dataset
definition and report types — it did not deliver the replay runner that
executes the dataset against live engine contracts, nor the HTTP surface
that exposes the report to operators.

M56 closes this gap. The roadmap requires that "rule, correlation,
prompt, model, provider or evidence-schema changes must produce a
machine-readable before/after quality report rather than silently
replacing the baseline." To enforce this, M56 delivers:

1. **Replay runner** — a deterministic engine-contract verifier that
   replays the golden dataset against the current M39-M44 package
   versions and status enumerations, producing per-scenario, per-step
   pass/fail results.
2. **Quality report storage** — timestamped JSON files under
   `.artifacts/quality-report/` with before/after baseline comparison.
3. **Async replay service** — in-memory task tracking with background
   execution, so a POST trigger returns immediately (202) and the caller
   polls GET for the latest report.
4. **HTTP surface** — `GET /api/v1/aiops/quality-report` (read latest)
   and `POST /api/v1/aiops/quality-report/run` (trigger replay,
   SystemOpsAdmin only).

The design space includes a CI-only runner (no HTTP), a database-backed
report store, and a live Kubernetes integration test runner. All three
are rejected:

- A CI-only runner would require operators to shell into the build
  pipeline to view reports, breaking the single-platform operational
  model. The HTTP surface lets an operator trigger a replay from the
  console after a component upgrade.
- A database-backed store would couple the quality report to the
  operational database lifecycle. Quality reports are artifacts, not
  transactional records — they belong in the file system (like audit
  archives, ADR 0031) so they survive database restores and can be
  diffed across releases.
- A live Kubernetes integration test runner would re-introduce the
  flakiness and environment-dependence that the golden dataset was
  designed to eliminate. The runner verifies engine contracts (version
  constants, status enumerations, step expectations), not live cluster
  state — this keeps the replay deterministic and reproducible.

## Decision

### 1. Deterministic replay runner; verifies engine contracts, not live state

`internal/golden.ReplayRunner` executes the golden dataset against the
current `EngineContracts` snapshot. The contracts struct captures:

- `EngineVersions` — the package-level version constants from each
  AIOps package (signal, topology, SLO, correlation, investigator,
  automation, verifier).
- `ValidPlanStatuses` — the set of valid `automation.ActionPlan` status
  strings.
- `ValidVerificationStatuses` — the set of valid
  `automation.ActionVerification` status strings.

The runner iterates each scenario's steps, checking that the expected
outcomes are still supported by the current contracts. For example, the
mandatory scenario expects that the automation plan lifecycle covers all
9 statuses (draft → previewed → approved → executing → succeeded/failed/
expired/cancelled → verified); if a future engine change removes or
renames a status, the step fails and the scenario is marked as
regressed.

The runner is deterministic: identical `EngineContracts` + identical
`Dataset` produce identical `ScenarioResult` slices. No random input,
no network calls, no time-dependent logic (timestamps are injected by
the service layer, not the runner).

### 2. Engine contracts adapter in cmd/server; no import cycles

The `golden` package is pure — it defines the dataset, runner, and
report types without importing any engine package. The
`EngineContracts` struct is populated by `cmd/server/golden_contracts.go`,
which reads the live version constants and status enumerations from the
M39-M44 packages. This adapter pattern (same as M52's
`inspection_cluster_lister.go`) keeps the golden package free of engine
dependencies and avoids potential import cycles.

### 3. File-based report storage under .artifacts/quality-report/

`internal/golden.FileReportStorage` writes each report as a timestamped
JSON file: `<YYYYMMDD-HHMMSS>-<dataset_version>.json`. Files are sorted
by name (chronological due to the timestamp prefix) to find the latest.
`LoadLatest` returns `ErrNoReport` when the directory does not exist or
contains no `.json` files.

This mirrors the audit-archive pattern (ADR 0031): artifacts are
file-based, survive database restores, and can be diffed across
releases. The directory is created on first `Save`; no migration is
needed.

A `NopReportStorage` is provided for tests and route-registration
checks.

### 4. Async replay service with in-memory task tracking

`internal/golden.Service` wraps the runner and storage. `RunReplay`
creates a task ID (`qr-<8-byte-hex>`), stores it in an in-memory
`replayTaskTracker`, launches a background goroutine, and returns the
task ID immediately (202 Accepted). The background goroutine:

1. Loads the default dataset (`DefaultDataset()`).
2. Runs the replay via `ReplayRunner.Run` with a 5-minute timeout.
3. Loads the previous baseline report (if any) for before/after
   comparison.
4. Builds per-scenario `ScenarioQuality` (passed_before/after, delta
   classification, step counts).
5. Persists the report via `ReportStorage.Save`.
6. Updates the task status (succeeded/failed).

`GetLatestReport` delegates to `storage.LoadLatest`. `GetTask` returns
the current task status for polling.

Task state is in-memory and non-durable: a server restart loses
in-flight task tracking (but not persisted reports). This is acceptable
because replays are short-lived (seconds) and the operator can retrigger
if needed. Durable task tracking would require a database table for a
transient operational artifact — over-engineering for the current scope.

### 5. HTTP surface: GET (any-auth) + POST (SystemOpsAdmin only)

Two routes under `aiopsRoutes`:

- `GET /api/v1/aiops/quality-report` — returns the latest persisted
  quality report JSON. Any authenticated user can read; the report
  contains no sensitive data (engine versions, pass/fail counts, step
  results). Returns 404 `QUALITY_REPORT_NOT_FOUND` when no report exists
  yet, 503 `GOLDEN_UNAVAILABLE` when the service is nil.
- `POST /api/v1/aiops/quality-report/run` — triggers an async replay.
  Requires `system_ops_admin` role (the same role that manages
  platform-level operations). Returns 202 with `{task_id, status,
  message}`. The caller polls GET for the latest report once the
  background goroutine completes.

Both routes are tagged with audit verbs
(`aiops.quality_report.read` / `aiops.quality_report.run`) per ADR 0008.

### 6. Before/after baseline comparison

When a replay completes, the service loads the previous baseline report
(via `storage.LoadLatest`). If a baseline exists:

- `DatasetVersionBefore` / `EngineVersionsBefore` are copied from the
  baseline.
- Each `ScenarioQuality` records `PassedBefore` (from baseline) and
  `PassedAfter` (from the current replay), with `Delta` classified as
  `preserved` / `improved` / `regressed` / `unchanged`.

If no baseline exists (first run), `PassedBefore` is false for all
scenarios and `Delta` is `improved` (if the scenario passes) or
`unchanged` (if it fails). The `QualitySummary` aggregates the
per-scenario deltas into total counts.

### 7. No new authorization path; no new roles

The quality-report routes reuse the existing `aiopsRoutes` middleware
chain (bearer auth + audit). GET requires only authentication; POST
requires `system_ops_admin` (the platform-operations role from the M34
role matrix). No new roles, no new middleware, no new database tables.
The 2D authorization matrix is intact — the quality report is a
platform-level artifact, not a per-cluster or per-namespace resource.

## Consequences

### Positive

- Operators can trigger a golden replay from the console after any
  engine component change (rule, correlation, prompt, model, provider,
  evidence schema) and see a machine-readable before/after report —
  no CI pipeline access required.
- The replay is deterministic and fast (seconds, not minutes): it
  verifies engine contracts, not live cluster state, so it is
  reproducible across environments.
- File-based storage means reports survive database restores and can be
  diffed across releases in git or a file browser.
- The runner is pure Go with no external dependencies — no Kubernetes
  client, no Prometheus, no AI provider — so it runs in any environment
  where the server binary runs.
- Engine contracts adapter pattern keeps the golden package decoupled
  from engine packages, avoiding import cycles.

### Negative / Risks

- The runner verifies engine contracts (versions, status enumerations,
  step expectations), not live Kubernetes/AI behavior. A component
  change that preserves the contract but breaks runtime behavior (e.g.
  a Prometheus query bug) will not be caught by the replay. Mitigation:
  the golden dataset's step expectations are detailed enough to catch
  most contract violations; live integration testing remains the
  responsibility of CI E2E tests.
- In-memory task tracking is non-durable. A server restart during a
  replay loses the task (but not any already-persisted report).
  Mitigation: replays are short-lived; the operator can retrigger.
- File-based storage has no retention policy. Reports accumulate
  indefinitely under `.artifacts/quality-report/`. Mitigation: the
  timestamped naming makes manual cleanup straightforward; a future
  milestone could add a retention sweep.
- `EngineContracts` is a snapshot at construction time. If the server
  binary is upgraded mid-run, the contracts reflect the new binary.
  This is the intended behaviour — the replay should verify the current
  engine, not a stale snapshot.

## Deployment notes

- No database migration is required. The quality report is file-based.
- The `.artifacts/quality-report/` directory is created on first
  `POST /quality-report/run`. In containerized deployments, mount a
  persistent volume at `.artifacts/` if reports should survive pod
  restarts.
- The golden service is initialized in `cmd/server/main.go` with
  `goldenEngineContracts()` (live engine versions + statuses) and
  `FileReportStorage` rooted at `.artifacts/quality-report/`.
- `TestRegisteredRoutesMatchOpenAPI` covers both M56 routes
  (route-contract consistency, ADR 0049).
