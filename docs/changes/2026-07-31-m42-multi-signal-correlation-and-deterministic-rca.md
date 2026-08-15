# M42: Multi-Signal Correlation And Deterministic RCA

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0057](../adr/0057-multi-signal-correlation-and-deterministic-rca.md)
- Fast gate: 63.26s (31 backend packages including `correlation`, 81
  frontend tests/18 files, Compose/Kustomize contracts)

## Summary

Introduced a deterministic, replayable multi-signal correlation engine that
links M39 signal occurrences, M40 topology edges/change events and existing
diagnosis records into bounded cases. M42A defines the data model and
migration 000031; M42B implements the correlation engine with explicit
factors, bidirectional topology path search and deterministic confidence
classification; M42C adds 9 golden replay fixtures plus a cold-start scenario
as the replayable contract; M42D wires the 6 read-only HTTP routes and
OpenAPI schemas; M42E adds 36 unit tests (5 catalog + 12 engine/fixtures +
10 service + 9 handler), ADR 0057 and this change record.

The diagnosis record remains the human status/SLA/feedback source of truth;
correlation cases are candidates, not incidents. AI (M43) may rank candidates
but cannot promote them — only an operator can, via the diagnosis lifecycle.

No public API contract was broken beyond the six new
`aiops/correlation` routes documented in OpenAPI.

## Changes

### M42A — Data model and migration

#### New files

- `backend/internal/correlation/model.go`: defines `ConfidenceClass` (4
  values: confirmed, candidate, contradicted, unknown), `CaseStatus` (3
  values: active, resolved, stale), `EvidenceCompleteness` (3 values),
  `SignalRelation` (4 values), `ResourceRelation` (4 values),
  `ResourceCitation`, `EvidenceRef`, `Factor`, `Case`, `SignalLink`,
  `ResourceLink`, `ChangeCandidate`, `CaseFilter`, `CaseListResponse`,
  `CaseView`, `CaseTimelineResponse`, `ActionCandidate`,
  `ActionCandidateListResponse`, `CorrelationResult`, and the bound
  constants (`MaxCaseFactors`, `MaxChangeCandidates`, `MaxTopologyPathLen`,
  etc.). `CorrelationVersion = "1.0"`.
- `backend/migrations/000031_diagnosis_correlation.up.sql`: creates
  `correlation_cases`, `correlation_signal_links`,
  `correlation_resource_links`, `correlation_change_candidates` tables with
  unique indexes on `case_key` (active), `(case_id, signal_occurrence_id,
  relation)`, `(case_id, uid, relation)`, and `(case_id, change_event_id)`.

### M42B — Correlation engine

#### New files

- `backend/internal/correlation/catalog.go`: defines `RuleDescriptor` and
  the compiled `catalog` map with 6 V1 rules. `LookupRule`, `AllRules`,
  `RulesForTriggerSignal` fail closed for unlisted rules.
- `backend/internal/correlation/engine.go`: defines `Engine` (stateless,
  pure), `EngineInputs`, `SignalOccurrenceInput`, `ChangeEventInput`,
  `TopologyEdgeInput`, `DiagnosisRef`. `Correlate` processes trigger
  signals in stable order, evaluates each matching rule, computes explicit
  factors (`same_uid`, `topology_distance`, `time_distance`,
  `change_symptom_rule`, `signal_freshness`, `signal_completeness`,
  `diagnosis_match`, `contradicting_signal`), classifies confidence, and
  merges duplicate case_keys. `edgeIndex` implements bidirectional BFS for
  topology path search. `changeIndex` and `diagIndex` provide bounded
  lookups.
- `backend/internal/correlation/repository.go`: defines `Repository`
  interface, `GormRepository` (with `caseRow`/`signalLinkRow`/
  `resourceLinkRow`/`changeCandidateRow` GORM models, `JSONB` wrapper,
  idempotent `UpsertResult` with ON CONFLICT DO NOTHING, `GetCase`,
  `ListCases`, `ListTimeline`, `ListSignalLinks`, `ListResourceLinks`,
  `ListChangeCandidates`, `ResolveCaseStatus`, `applyCaseFilter`) and
  `NopRepository`.
- `backend/internal/correlation/service.go`: defines `Service` (the only
  writer), `InputProvider` interface, `NopInputProvider`,
  `CorrelateResult`, `ServiceOption` (`WithNow`, `WithLookback`).
  `CorrelateNamespace` gathers bounded inputs, runs the engine and
  persists results idempotently. `GetCase`, `ListCases`, `ListTimeline`,
  `GetCaseGraph`, `ListActionCandidates` (derives fixed action candidates
  from the M19 catalog: `deployment.rollback`,
  `deployment.rollout_restart`).

### M42C — Golden replay fixtures

#### New files

- `backend/internal/correlation/fixtures.go`: defines `GoldenFixture`,
  `GoldenFixtures` (9 scenarios: image_pull_backoff, crash_loop_backoff,
  oom_killed, pvc_pending, no_ready_endpoints, replicas_unavailable,
  node_not_ready, metric_breach, bad_rollout_contradicted) and
  `coldStartFixture`. Each fixture is a deterministic (inputs, expected)
  pair with expected confidence, candidate count, signal/resource links.

### M42D — HTTP routes and OpenAPI

#### New files

- `backend/internal/httpserver/correlation.go`: defines `correlationHandler`
  with 6 read-only handlers: `listCorrelationRules`,
  `listCorrelationCases`, `listCorrelationTimeline`, `getCorrelationCase`,
  `getCorrelationCaseGraph`, `listCorrelationActions`. `parseCaseFilter`
  enforces `cluster_id` required, bounded `limit` (max 200), RFC3339
  time parsing. `writeCorrelationError` maps `ErrCaseNotFound` → 404.

#### Modified files

- `backend/internal/httpserver/router.go`: adds `CorrelationService` to
  `Options`, registers the 6 correlation routes under
  `/api/v1/aiops/correlation` when the service is non-nil.
- `backend/internal/httpserver/openapi_route_test.go`: adds
  `CorrelationService` wiring with `NopRepository` so route registration is
  covered by the OpenAPI parity test.
- `backend/cmd/server/main.go`: no production wiring yet (correlation is
  test-only; production wiring deferred to the background worker milestone).
- `docs/api/openapi.yaml`: adds 6 correlation routes and schemas under the
  `correlation` tag.

### M42E — Unit tests, ADR, docs

#### New files

- `backend/internal/correlation/fixtures_test.go`: 3 tests —
  `TestGoldenFixtures` (10 subtests covering all 9 scenarios + cold-start),
  `TestGoldenFixturesDeterminism` (replay produces byte-identical
  case_keys), `TestGoldenFixturesCaseKeyStability` (stable across engine
  instances).
- `backend/internal/correlation/catalog_test.go`: 5 tests —
  `TestCatalogAllRules`, `TestCatalogLookupRule`,
  `TestCatalogRulesForTriggerSignal`, `TestCatalogCorrelationVersion`,
  `TestCatalogRequiredFactors`.
- `backend/internal/correlation/service_test.go`: 10 tests covering
  `CorrelateNamespace` (with fake input provider + real engine),
  `CorrelateNamespace` (NopInputProvider), `GetCase`, `ListCases`,
  `ListTimeline`, `GetCaseGraph`, `ListActionCandidates` (not-found,
  rollback, pod-rollout-restart, service-backing). Uses `fakeRepository`
  (in-memory) and `fakeInputProvider`.
- `backend/internal/httpserver/correlation_test.go`: 9 handler tests —
  list rules 200, list cases missing cluster_id 400, list cases 200,
  timeline 200, get case not-found 404, graph not-found 404, actions
  not-found 404, invalid id 400, service unavailable 503.
- `docs/adr/0057-multi-signal-correlation-and-deterministic-rca.md`: 7
  decisions covering deterministic catalog, explicit factors, confidence
  classification, case aggregation, golden fixtures, read-only HTTP
  surface, bidirectional topology search.
- `docs/changes/2026-07-31-m42-multi-signal-correlation-and-deterministic-rca.md`:
  this change record.

#### Modified files

- `docs/roadmap.md`: M42 status section added.
- `docs/testing/test-matrix.md`: M42 addendum with 36 test counts.
- `docs/development-handoff.md`: updated to M42 baseline.
- `CHANGELOG.md`: M42 entry.

## Test counts

| Package | Tests |
|---|---|
| `internal/correlation` (catalog) | 5 |
| `internal/correlation` (fixtures) | 3 (10 subtests) |
| `internal/correlation` (service) | 10 |
| `internal/httpserver` (correlation handler) | 9 |
| **Total** | **36** (27 top-level + 10 golden subtests, counted as 37 cases) |

## Deferred

- Background correlation worker (periodic `CorrelateNamespace` per cluster)
- Signal-ingestion hook (trigger correlation on new signal occurrence)
- Real PostgreSQL integration test for `GormRepository`
- Real-kind E2E for the full correlation pipeline
- Frontend UI (case list, timeline, impact graph, action candidates)
- M43 AI investigator integration (rank candidates, generate explanations)
- M44 safe automation integration (preview/confirm/execute action candidates)
