# M51: Bounded Event Stream + Alert Inhibits

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0066](../adr/0066-bounded-event-stream-and-alert-inhibits.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 58.31s; backend=True frontend=True manifests=True)

## Summary

Delivered the M51 backend increment extending the Phase 2 (full-stack
observability) alert and event surface with two additions that do not
introduce a second alert system or a Kubernetes Watch dependency:

1. **Bounded SSE event stream** — `GET /clusters/:cluster_id/events/stream`
   opens a Server-Sent Events stream of recent Kubernetes Events. The
   stream is a bounded poller (default 5s, min 50ms) over the read-only
   gateway (ADR 0004), not a Kubernetes Watch. Events are deduped by UID
   against a bounded ring (256 entries) and pushed with drop-oldest
   backpressure. The M35 namespace scope is honoured: all-namespaces polls
   cluster-wide; an authorized namespace set polls each namespace; an empty
   scope yields an immediately-closed empty stream (anti-leakage, not
   404). The stream emits hello / event / stream-closed SSE messages.
2. **Alert inhibits** — `GET/POST/DELETE /alert-routes/inhibits` manage
   source_match → target_match suppression rules. An inhibit is not
   time-bounded; suppression depends on the source alert's live firing
   state (a non-resolved delivery within the 5m active window), re-checked
   on every `MatchAndDeliver` call. This complements the M37B
   time-bounded silences, which cannot express "suppress while X is
   firing".

Authorization reuses existing middleware chains without introducing a new
authorization path. The event stream is registered under `resourceRoutes`
(M35 cluster + namespace scope); inhibits are registered under
`alertrouteRoutes` (auth-required for list; `operations_admin` for
create/delete).

Anti-leakage (404 > 403) is preserved: an unauthorized or missing inhibit
on delete returns 404 `INHIBIT_NOT_FOUND`; an unauthorized namespace on
the event stream yields an empty stream (not 404).

## Files Changed

### New Files

- `backend/migrations/000036_alert_inhibits.up.sql` — `alert_inhibits`
  table (id, creator_id, source/target cluster_id+rule_name+severity,
  reason, enabled, timestamps). CHECK constraints require at least one
  source matcher and one target matcher. Indexes on creator_id and
  enabled=TRUE.
- `backend/migrations/000036_alert_inhibits.down.sql` — Drop
  `alert_inhibits` table.
- `backend/internal/eventstream/service.go` — `Service` with `Subscribe`
  (per-client poller goroutine), `Stream` (Events/Done channels),
  `EventSummary` projection, `EventLister` interface, `seenRing` UID
  dedup, `pushEvent` drop-oldest backpressure. Bounded constants
  (DefaultPollInterval=5s, Min=50ms, Max=60s; DefaultBufferCap=256,
  Min=16, Max=1024; DefaultListLimit=500, Max=1000).
- `backend/internal/eventstream/service_test.go` — 11 unit tests covering
  NewService defaults/invalid config, Subscribe nil lister (error), empty
  scope (immediate close), all-namespaces delivery, namespaced delivery,
  dedup, poll error does not terminate, context cancel closes stream,
  drop-oldest backpressure.
- `backend/cmd/server/eventstream_lister.go` — `kubernetesEventLister`
  adapter translating `k8sgateway.Service.Events` into
  `eventstream.EventSummary`. Bounded page limit (DefaultListLimit, capped
  at MaxListLimit).
- `backend/internal/httpserver/eventstream.go` — `eventstreamHandler`
  with `streamEvents`. SSE headers (text/event-stream, no-cache,
  keep-alive, X-Accel-Buffering: no). hello/event/stream-closed messages
  via `writeSSEEvent`. 503 when service nil or lister nil.
- `backend/internal/httpserver/eventstream_test.go` — 9 handler tests
  covering 503 nil service, empty scope (immediate close), all-namespaces
  delivery, SSE headers, hello event, stream-closed on cancel.

### Modified Files

- `backend/internal/alertroute/model.go` — Add `Inhibit` struct,
  `InhibitView`, `ErrInhibitNotFound`, `ErrInvalidInhibit`,
  `ErrInhibitLimit`, `MaxInhibitsPerUser = 30`.
- `backend/internal/alertroute/repository.go` — Add `InhibitListFilter`,
  `CreateInhibit`, `ListInhibits`, `DeleteInhibit`, `ListEnabledInhibits`,
  `HasFiringSource` repository methods. GORM implementation + test mock.
- `backend/internal/alertroute/service.go` — Add `CreateInhibit`,
  `ListInhibits`, `DeleteInhibit`, `IsInhibited`,
  `DefaultInhibitActiveWindow = 5m`, `validateInhibit`,
  `inhibitTargetMatches`. Wire `IsInhibited` into `MatchAndDeliver`
  (checked after `IsSilenced`, before route matching).
- `backend/internal/alertroute/service_test.go` — 12 inhibit service
  tests (create valid/rejects empty source/rejects empty target/rejects
  missing reason/limit, list, delete/rejects non-creator,
  IsInhibited suppresses when source firing/not suppressed when not
  firing/target does not match, MatchAndDeliver blocked by inhibit).
- `backend/internal/httpserver/alertroute.go` — Add `inhibitCreateRequest`,
  `listInhibits`, `createInhibit`, `deleteInhibit` handlers,
  `inhibitViewFromInhibit`, error mappings (`INHIBIT_NOT_FOUND` 404,
  `INVALID_INHIBIT` 400, `INHIBIT_LIMIT_REACHED` 409).
- `backend/internal/httpserver/alertroute_test.go` — Add inhibit repo
  mock methods + 6 handler tests (create valid/invalid/limit, list,
  delete/not-found).
- `backend/internal/httpserver/router.go` — Extend `Options` with
  `EventStreamService *eventstream.Service`. Register 4 M51 routes: event
  stream under `resourceRoutes` (guarded by `EventStreamService != nil`);
  inhibit list/create/delete under `alertrouteRoutes` (create/delete
  require `operations_admin`).
- `backend/internal/httpserver/openapi_route_test.go` — Add
  `EventStreamService: mustEventStreamService(t)` to the route-contract
  test's `Options` so the 4 M51 routes are covered.
- `backend/cmd/server/main.go` — Construct `eventStreamService` and wire
  into `httpserver.Options`.
- `docs/api/openapi.yaml` — 4 new paths
  (`/clusters/{cluster_id}/events/stream`, `/alert-routes/inhibits`,
  `/alert-routes/inhibits/{id}`) and 4 new schemas (`AlertInhibitView`,
  `AlertInhibitCreate`, `EventStreamMessage`, `EventStreamEvent`).

## Routes

| Method | Path | Audit Action | Auth |
|--------|------|-------------|------|
| GET | `/api/v1/clusters/:cluster_id/events/stream` | `kubernetes.events.stream` | cluster access (M35) + namespace scope |
| GET | `/api/v1/alert-routes/inhibits` | — | auth required |
| POST | `/api/v1/alert-routes/inhibits` | `alert_route.inhibit.create` | operations_admin |
| DELETE | `/api/v1/alert-routes/inhibits/:id` | `alert_route.inhibit.delete` | operations_admin |

## Key Invariants Maintained

- **No Kubernetes Watch** — the event stream is a bounded poller over the
  read-only gateway (ADR 0004). No long-lived gateway connection, no
  informer cache.
- **No second alert system** — inhibits are persisted as SQL rows and
  evaluated against the existing M27 delivery table; the M27 lifecycle
  remains the single alert system.
- **Bounded per-client memory** — events channel buffered at 256
  (drop-oldest backpressure); UID dedup ring bounded at 256.
- **404 > 403 anti-leakage** — unauthorized inhibit delete → 404
  `INHIBIT_NOT_FOUND`; unauthorized namespace on event stream → empty
  stream (not 404).
- **Auditability** — inhibit `reason` mandatory (1..500 chars, mirrors
  silences); create/delete audited via `AuditAction`/`AuditResource`.
- **2D authorization matrix intact** — no new authorization path; event
  stream reuses M35 cluster/namespace scope; inhibits reuse M37B
  alertroute roles.

## Tests

- 11 eventstream service tests (NewService defaults/invalid config,
  Subscribe nil lister/empty scope/all-namespaces/namespaced delivery,
  dedup, poll error tolerance, context cancel, drop-oldest backpressure).
- 12 alertroute inhibit service tests (create valid/rejects empty
  source/rejects empty target/rejects missing reason/limit, list, delete/
  rejects non-creator, IsInhibited suppresses when firing/not suppressed
  when not firing/target mismatch, MatchAndDeliver blocked by inhibit).
- 6 alertroute inhibit handler tests (create valid/invalid/limit, list,
  delete/not-found).
- 9 eventstream handler tests (503 nil service, empty scope immediate
  close, all-namespaces delivery, SSE headers, hello event, stream-closed
  on cancel).
- `TestRegisteredRoutesMatchOpenAPI` covers all 4 M51 routes (route-contract
  consistency, ADR 0049).

Total: 38 new unit tests (23 service + 15 handler).
