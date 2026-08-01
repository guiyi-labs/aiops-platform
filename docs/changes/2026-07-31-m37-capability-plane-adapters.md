# M37: Capability Plane Adapters

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0053](../adr/0053-capability-plane-adapters.md)
- Baseline: `baseline-m32-20260731` (M33/M34/M35/M36/M38 layered on top; no new baseline tag cut)
- Fast gate: 51.06s (backend vet+test, 81 frontend tests, Compose/Kustomize contracts)

## Summary

Introduced bounded provider contracts that normalize external monitoring
evidence (Prometheus metrics, Loki logs) and alert routing with bounded
silences into the platform's existing evidence model. The native M21–M31
signal path is unchanged; M37 adapters are evidence sources, not a parallel
alert/diagnosis/workflow system. `docs/kubesphere-optimization-plan.md` (M37)
required bounded provider contracts for the later AIOps differentiation route
(M39–M44).

M37A adds `MetricsProvider` and `LogProvider` interfaces with Prometheus and
Loki adapters. Public APIs accept fixed template/query AST fields only — they
never accept PromQL, LogQL or arbitrary labels. M37B adds exact-match alert
route priority, HTTPS webhook receivers, time-bounded silences (permanent
forbidden) and idempotent delivery with retry and dead-letter.

M37C (Gateway API evidence) and M37D (delivery metadata) are deferred per
ADR 0053 §4 until M40 demonstrates concrete need.

No public API contract was broken. All adapters are disabled by default; the
server runs identically to the current deployment when no provider is
configured.

## Changes

### M37A — Capability providers

#### New files

- `backend/internal/capability/model.go`: defines `MetricsQuery`,
  `MetricsResult`, `MetricsSeries`, `MetricsCoverage`, `LogQuery`,
  `LogResult`, `LogEntry`, `State` constants and the fixed template
  enumeration (`request_rate`, `error_rate`, `latency_p99`, `cpu_usage`,
  `memory_usage`). All fields are bounded: namespace/pod/container max
  length 63, limit 1..500, direction `forward`/`backward`, default 1h,
  hard-stop 7d.
- `backend/internal/capability/provider.go`: defines the `MetricsProvider`
  and `LogProvider` interfaces (`QueryMetrics`, `QueryLogs`, `Name`), the
  `NopMetricsProvider` and `NopLogProvider` defaults that return
  `StateUnavailable`, and the shared query validation helpers
  (`validateMetricsQuery`, `validateLogQuery`).
- `backend/internal/capability/prometheus.go`: Prometheus adapter that maps
  each fixed SLI template to a concrete PromQL query, calls the configured
  HTTPS endpoint with a bounded timeout, and normalizes the response into
  `MetricsResult` with explicit `coverage`, `state` and `freshness`. Provider
  URL and credentials are server-configured; request input cannot redirect
  the query.
- `backend/internal/capability/loki.go`: Loki adapter that maps the bounded
  `LogQuery` to a LogQL query, enforces the 1h default / 7d hard-stop /
  timeout / result / byte bounds, and normalizes the response into
  `LogResult`.
- `backend/internal/capability/provider_test.go`: 18 tests covering query
  validation (template, cluster, namespace, time range, step, limit,
  direction), Nop provider behavior, and the normalized result shape.
- `backend/internal/httpserver/capability.go`: HTTP handler that wires the
  providers to `GET /api/v1/capability/metrics` and
  `POST /api/v1/capability/logs`. Returns 503 when the provider is unset;
  400 on validation error; 200 with the normalized result otherwise.
- `backend/internal/httpserver/capability_test.go`: 8 handler tests
  covering 503 when unconfigured, 400 on missing/invalid inputs, 200 on
  valid queries, and the masked-error invariant.

### M37B — Alert routing and bounded silences

#### New files

- `backend/internal/alertroute/model.go`: defines `Receiver`, `Route`,
  `Silence`, `Delivery`, `PatchRouteInput`, `SilenceListFilter`,
  `DeliveryListFilter`, `ListResponse[T]`, status constants and sentinel
  errors. Route priority is 1..100 (lower = higher); silence duration is
  5m..7d with a non-empty reason (1..500 chars); delivery status is
  `pending`/`delivering`/`delivered`/`dead`.
- `backend/internal/alertroute/repository.go`: defines the `Repository`
  interface with 18 methods (receiver/route/silence/delivery CRUD plus
  `ListEnabledRoutes`, `ListActiveSilences`, `FindActiveDelivery`,
  `ClaimDeliveries`, `MarkDelivered`, `MarkFailed`) and the GORM
  implementation. Active-route and active-silence queries use partial
  indexes.
- `backend/internal/alertroute/service.go`: business logic for
  `MatchAndDeliver` (route matching, dedupe-key rendering, idempotent
  delivery creation), `IsSilenced` (active-silence check), receiver/route/
  silence CRUD with ownership and in-use guards, and the bounded delivery
  dispatcher with retry, dead-letter and audit. `maxAttempts`,
  `retryBase`, `batchSize` and `requestTimeout` are server-configured.
- `backend/internal/alertroute/service_test.go`: 40 tests covering route
  matching, dedupe-key rendering, silence enforcement (permanent forbidden,
  max duration, reason required), receiver CRUD with in-use guard, route
  CRUD with ownership, delivery lifecycle (pending → delivering →
  delivered/dead), retry backoff, batch claiming, webhook dispatch (success,
  4xx, 5xx, timeout), and the masked-URL invariant.
- `backend/internal/httpserver/alertroute.go`: HTTP handlers wiring the
  service to the 10 alert-route endpoints. SystemOpsAdmin role required for
  mutations; deliveries restricted to SystemSecurityAudit. Receiver URL is
  masked in every response; the secret is never returned.
- `backend/internal/httpserver/alertroute_test.go`: 27 handler tests
  covering the role matrix, ownership (404 for other users' resources),
  receiver in-use conflict, route priority bounds, silence duration bounds,
  delivery listing filter, and the masked-URL invariant.

#### Migration

- `backend/migrations/000027_alert_routes_and_silences.up.sql`: creates
  `alert_route_receivers` (with `UNIQUE(creator_id, name)`),
  `alert_routes` (with priority `CHECK 1..100`, partial index on
  `enabled = TRUE`), `alert_silences` (with `CHECK ends_at > starts_at` and
  `CHECK duration <= 7d`) and `alert_route_deliveries` (with a partial index
  on `(route_id, dedupe_key, event_type) WHERE status != 'dead'` for
  idempotent delivery lookup).
- `backend/migrations/000027_alert_routes_and_silences.down.sql`: drops all
  four tables in reverse dependency order.

### Configuration

#### Modified files

- `backend/internal/config/config.go`: added `CapabilityConfig` (Prometheus/
  Loki endpoints and timeouts) and `AlertRouteConfig` (webhook timeout,
  max attempts, retry base, batch size, request timeout) with fail-closed
  validation — HTTPS endpoints required, bounded timeouts, empty endpoints
  allowed (disabled by default).

### HTTP wiring and OpenAPI

#### Modified files

- `backend/internal/httpserver/router.go`: added
  `CapabilityMetricsProvider`, `CapabilityLogProvider` and
  `AlertRouteService` to `Options`; registered the two capability routes
  (when either provider is non-nil) and the ten alert-route routes (when
  the service is non-nil) via `RouteDescriptor` with `AuditAction` and
  `AuditResource` tags.
- `backend/internal/httpserver/openapi_route_test.go`: wired
  `capability.NopMetricsProvider{}`, `capability.NopLogProvider{}` and
  `alertroute.NewService(nil, nil)` into the route-contract test so the
  M37 routes are registered and bidirectional parity is verified.
- `docs/api/openapi.yaml`: added `capability` and `alert-routes` tags, the
  two capability paths, the ten alert-route paths, and the
  `CapabilityMetricsResult`, `CapabilityLogResult`, `AlertReceiverView`,
  `AlertReceiverCreate`, `AlertRouteView`, `AlertRouteCreate`,
  `AlertRouteUpdate`, `AlertSilenceView`, `AlertSilenceCreate` and
  `AlertDeliveryView` schemas.

## Verification

- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 51.06s: 27
  backend packages vet/test green (including `capability` and
  `alertroute`), 81 frontend tests / 18 files green, Compose and Kustomize
  contracts green.
- `TestRegisteredRoutesMatchOpenAPI` verifies bidirectional route↔OpenAPI
  parity for all 12 M37 routes.
- Fixed two YAML parsing errors in `openapi.yaml` (unquoted colons in
  `description` fields at the `createAlertRoute` and `createAlertSilence`
  operations) before the gate passed.

## Deferred

- M37C (Gateway API evidence) and M37D (delivery metadata) deferred per
  ADR 0053 §4 until M40 demonstrates concrete need.
- Real Prometheus/Loki provider integration test deferred pending external
  provider access.
- Real-kind E2E for alert-route delivery deferred pending a multi-worker
  kind cluster with a synthetic webhook receiver.
- Frontend UI for capability metrics/logs and alert-route management
  deferred.
