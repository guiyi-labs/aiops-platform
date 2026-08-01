# M39: Unified Service Identity and Signal Model

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0054](../adr/0054-unified-service-identity-and-signal-model.md)
- Fast gate: 59.59s (28 backend packages including `signal`, 81 frontend tests, Compose/Kustomize contracts)

## Summary

Introduced a normalized signal model that converts existing M21-M31 outputs
(diagnosis hits, alert transitions, metric breaches, posture findings, change
outcomes) into a single append-only, TTL-bound `signal_occurrences` table with
fingerprint-based deduplication. The native M21-M31 signal path is unchanged;
M39 is an evidence normalizer, not a second alert/diagnosis/workflow system.

M39 begins the AIOps differentiation route (M39-M44). It normalizes existing
outputs before adding more algorithms, so M40 (temporal topology), M41 (SLO),
M42 (correlation/RCA) and M43 (AI investigator) can operate on a stable,
deduplicated, scope-filtered signal model.

## Changes

### New files

- `backend/internal/signal/model.go`: defines `Occurrence`, `SignalDescriptor`,
  `IngestRequest`, `ListFilter`, `ListResponse[T]`, `Overview`,
  `OverviewSignal`, `OverviewChange`, `OverviewOutcomes`, `ResourceCitation`,
  `EvidenceRef`, `Producer`/`State`/`Coverage`/`Severity` constants.
- `backend/internal/signal/catalog.go`: compiled `SignalDescriptor` catalog
  with 28 signal codes covering diagnosis (11), alert (2), metric (1), posture
  (4) and change (8) domains. `Lookup`, `All`, `MapSeverity` functions.
- `backend/internal/signal/repository.go`: `Repository` interface,
  `ComputeFingerprint` (SHA256 over identity fields, excluding ObservedAt),
  `BuildOccurrence` (fail-closed validation against catalog).
- `backend/internal/signal/gorm_repository.go`: `GormRepository` with
  `ON CONFLICT DO UPDATE` dedup, `List`, `CountBySignal`, `DeleteExpired`;
  `NopRepository` for disabled/testing mode.
- `backend/internal/signal/service.go`: `Service` with `Ingest`, `IngestBatch`,
  `List`, `Overview`, `CleanupRetention`; `SourceReader` interface;
  `NopSourceReader`.
- `backend/internal/signal/diagnosis_normalizer.go`: maps M17/M18
  `diagnosis.Record` to `IngestRequest` (11 rules → 11 signal ids).
- `backend/internal/signal/alert_metric_normalizer.go`: maps M27 alert
  `Rule`+`Instance` transitions and M21 sustained-window evaluations.
- `backend/internal/signal/posture_change_normalizer.go`: maps M29 posture
  `Finding` and M23-M31 change outcomes (promotion/backup/maintenance/restore).
- `backend/internal/signal/service_test.go`: 10 tests (fingerprint stability,
  fail-closed, incomplete UID, retention expiry, severity fallback, catalog
  invariants, service dedup, list clamping, overview partial flag, cleanup).
- `backend/internal/signal/normalizer_test.go`: 12 tests covering all five
  normalizer types (diagnosis mapped/unmapped/resolved, alert firing/resolved,
  metric firing/non-firing, posture mapped/unmapped, change succeeded/failed/
  pending).
- `backend/internal/httpserver/signal.go`: HTTP handler for
  `GET /api/v1/aiops/overview`, `GET /api/v1/aiops/signals`,
  `GET /api/v1/aiops/signals/catalog`.
- `backend/internal/httpserver/signal_test.go`: 9 handler tests (200/503/400
  paths, cluster scope, catalog, integration with ingest).

### Migration

- `backend/migrations/000028_signal_occurrences.up.sql`: creates
  `signal_occurrences` table with unique index on `(signal_id, fingerprint)`
  for idempotent ingestion, query indexes on cluster/namespace/state/producer/
  expires_at, and CHECK constraints on schema_version, producer, state,
  coverage, severity.
- `backend/migrations/000028_signal_occurrences.down.sql`: drops the table.

### Modified files

- `backend/internal/config/config.go`: added `SignalConfig` (Enabled,
  RetentionBatch, ListLimit, OverviewTopN, OverviewWindow, CleanupInterval)
  with `loadSignalConfig` and `validate`. Disabled by default.
- `backend/internal/httpserver/router.go`: added `SignalService` and
  `SignalSourceReader` to `Options`; registered 3 aiops routes when the
  service is non-nil.
- `backend/internal/httpserver/openapi_route_test.go`: wired
  `signal.NewService(signal.ServiceOptions{})` so the M39 routes are
  registered and covered by route-contract parity.
- `docs/api/openapi.yaml`: added `aiops` tag, 3 paths
  (`/api/v1/aiops/overview`, `/api/v1/aiops/signals`,
  `/api/v1/aiops/signals/catalog`) and 8 schemas (`AIOpsOverview`,
  `AIOpsOverviewSignal`, `AIOpsOverviewChange`, `AIOpsOverviewOutcomes`,
  `SignalOccurrenceList`, `SignalOccurrence`, `SignalResourceCitation`,
  `SignalEvidenceRef`, `SignalDescriptor`).

## Verification

- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 59.59s: 28 backend
  packages (including `signal`), 81 frontend tests / 18 files, Compose and
  Kustomize contracts green.
- `TestRegisteredRoutesMatchOpenAPI` verifies bidirectional route↔OpenAPI
  parity for all 3 M39 routes.
- 22 signal-package unit tests + 9 HTTP handler tests pass.

## Deferred

- Concrete `SourceReader` implementation reading from diagnosis/alert/promotion/
  backup/maintenance/restore services (interface and NopSourceReader in place;
  adapter deferred to M40).
- Batch ingestion worker that periodically normalizes M21-M31 outputs (API
  ready; worker deferred).
- Real PostgreSQL integration test for `GormRepository` (needs full Compose
  stack).
- Frontend UI for AIOps overview and signal list.
