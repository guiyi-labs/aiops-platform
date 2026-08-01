# ADR 0066: Bounded Event Stream + Alert Inhibits (M51)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M51
- Supersedes: none
- Related: ADR 0004 (bounded read-only Kubernetes gateway), ADR 0008
  (sanitized append-only audit trail), ADR 0049 (route descriptor contract
  and RBAC inventory), ADR 0050 (lightweight cluster and namespace access
  grants), ADR 0065 (monitoring dashboard and log explorer)

## Context

M51 continues Phase 2 (full-stack observability) of the post-M45 roadmap.
The roadmap calls for two backend increments that extend the alert and
event surface without introducing a second alert system or a Kubernetes
Watch dependency:

1. **Event stream** — a live, push-based feed of Kubernetes Events for the
   console. The frontend currently polls `GET /clusters/:cluster_id/events`
   on a timer; M51 adds a Server-Sent Events (SSE) endpoint that pushes
   deduped events as they arrive.
2. **Alert inhibits** — a source_match → target_match suppression rule that
   silences noisy target alerts while a source alert is firing (e.g.
   suppress `app-503` while `db-down` is active). This complements the M37B
   time-bounded silences, which cannot express "suppress while X is firing".

The design space includes a Kubernetes Watch passthrough, an Alertmanager
CRD model, and a time-bounded inhibit. All three are rejected by the
project's hard constraints:

- A Kubernetes Watch passthrough would break the bounded read-only gateway
  model (ADR 0004): Watch holds a server-side connection per client and
  re-introduces the resource-exhaustion surface the gateway was designed to
  close. It also requires a write-capable informer path the gateway does
  not expose.
- An Alertmanager CRD model would introduce a second alert system alongside
  the M27 lifecycle (violating the "no second alert system" hard
  constraint) and would require a dynamic rule controller the platform does
  not run.
- A time-bounded inhibit (start/end window) would not express the
  "suppress while firing" semantics that operators need — the whole point
  of an inhibit is that it is active exactly as long as the source is
  firing, no more, no less.

M51 therefore delivers: a **bounded poller** SSE stream (not a Watch) over
the existing read-only gateway, and a **state-evaluated** inhibit (not a
time window) persisted as a SQL row and re-checked on every
`MatchAndDeliver` call.

## Decision

### 1. Bounded poller SSE stream; no Kubernetes Watch

`internal/eventstream.Service.Subscribe` opens a per-client goroutine that
polls `EventLister.ListEvents` at `PollInterval` (default 5s, min 50ms, max
60s). Each poll calls the existing `kubernetes.Service.Events` list method
through the `kubernetesEventLister` adapter in `cmd/server/eventstream_lister.go`.
The adapter translates the gateway `Event` into an `EventSummary`
projection (UID, name, namespace, kind, type, reason, message, count,
timestamps, cluster_id, occurred_at) — no raw manifest is exposed.

There is no Watch, no informer cache, and no long-lived gateway connection.
The poller is stateless between ticks: it dedupes by UID against a bounded
ring of recently-seen UIDs (size = `BufferCap`, default 256) and pushes
only unseen events.

### 2. Drop-oldest backpressure; bounded per-client memory

The per-client events channel is buffered at `BufferCap` (default 256, min
16, max 1024). When the channel is full, `pushEvent` drains one event
(oldest) before pushing the new one — drop-oldest backpressure. This caps
per-client memory at `BufferCap × EventSummary` size regardless of event
rate. A slow client misses events but never blocks the poller or other
clients.

### 3. M35 namespace scope honoured; empty scope → empty stream (anti-leakage)

The handler resolves the caller's M35 namespace scope via
`ResolvedNamespaceScope(c)`:

- `AllNamespaces == true` → the poller fetches cluster-wide (single
  empty-namespace target).
- `AllNamespaces == false && len(NamespaceGrants) > 0` → the poller
  fetches each authorized namespace.
- `AllNamespaces == false && len(NamespaceGrants) == 0` → the stream
  closes immediately with no events. The client receives a `hello` event
  followed by a final `stream-closed` event.

The empty-scope case returns an empty stream rather than 404 so an
unauthorized namespace's existence is not leaked (404 > 403 anti-leakage,
ADR 0050/0061). The optional `?namespace=` query param is honoured by the
`requireNamespaceQueryAccess` middleware: when present, the scope is
narrowed to that single namespace (404 if unauthorized).

### 4. SSE protocol; hello / event / stream-closed messages

The handler emits three SSE event types:

- `hello` — sent once immediately after the headers, carrying
  `{cluster_id}`. Lets the client detect an immediate close (empty scope)
  without waiting for the first event.
- `event` — one per deduped `EventSummary`.
- `stream-closed` — sent once on termination, carrying `{reason}`:
  `client-disconnect`, `scope-empty-or-poller-stopped`, or
  `events-channel-closed`.

`X-Accel-Buffering: no` disables nginx proxy buffering. A nil flusher
(test recorder) is tolerated — bytes are still written, just not flushed.

### 5. State-evaluated inhibits; not time-bounded

`alert_inhibits` (migration `000036`) stores source and target matchers
(cluster_id nullable, rule_name, severity — empty/nil = match any). An
inhibit is **not** time-bounded: it is active from creation until explicit
deletion (or a future disable flag). Suppression depends on the source
alert's live firing state, re-evaluated on every `MatchAndDeliver` call:

- `IsInhibited` iterates enabled inhibits whose **target** matches the
  incoming alert.
- For each match, it queries `HasFiringSource(source, DefaultInhibitActiveWindow)`
  — true when a non-resolved `alert_route_delivery` exists for the source
  match within the active window (default 5m, `DefaultInhibitActiveWindow`).
- If any source is firing, the target alert is suppressed (no delivery
  created).

This mirrors Prometheus Alertmanager's inhibit semantics but is
persisted as a SQL row (not a CRD) and evaluated against the existing
delivery table (not a separate alert state store), keeping the M27
lifecycle as the single alert system.

`MatchAndDeliver` checks `IsSilenced` first, then `IsInhibited`, then
proceeds to route matching. An inhibited alert produces no delivery record
— it is fully suppressed, not deferred.

### 6. Inhibit validation and limits

`validateInhibit` enforces:

- `reason` mandatory, 1..500 chars (mirrors silences, auditability).
- At least one source matcher AND one target matcher must be non-empty.
  A fully-wildcard inhibit on both sides is rejected — it would suppress
  all alerts whenever any alert fires, which is never the operator's
  intent. The migration enforces the same constraint with CHECK
  constraints on source and target columns.

`MaxInhibitsPerUser = 30` mirrors `MaxSilencesPerUser`. Inhibits are
creator-scoped (reads and deletes fail with `ErrInhibitNotFound` for
non-creators, indistinguishable from a missing row). `enabled` defaults to
`true` on creation (the service sets it explicitly to mirror the migration
`DEFAULT TRUE`, avoiding the Go zero-value false).

### 7. Route registration under existing middleware chains

- **Event stream** (`GET /clusters/:cluster_id/events/stream`) is
  registered under `resourceRoutes` (only when `EventStreamService !=
  nil`), which applies `requireClusterAccess` → `requireNamespaceAccess` →
  `requireNamespaceQueryAccess` (M35). No new middleware.
- **Inhibits** (`GET/POST/DELETE /alert-routes/inhibits`) are registered
  under `alertrouteRoutes`. List is auth-required; create/delete require
  `operations_admin` (matching the silence create/delete roles). No new
  middleware.

When `EventStreamService` is nil, the SSE route is not registered. When
the service is non-nil but the lister is nil (route-contract wiring),
`Subscribe` returns `ErrClusterMissing` → handler returns 503
`EVENTSTREAM_UNAVAILABLE`.

### 8. 404 > 403 anti-leakage preserved

- An unauthorized or missing inhibit on delete returns 404
  `INHIBIT_NOT_FOUND` (indistinguishable from a missing row).
- An unauthorized namespace on the event stream returns an empty stream
  (not 404), so the namespace's existence is not leaked.
- A non-existent cluster follows the existing `requireClusterAccess` 404
  path.

## Consequences

- The event stream is a poller, not a Watch. Event latency is bounded by
  `PollInterval` (default 5s). Sub-second latency would require a Watch,
  which is deliberately rejected (ADR 0004).
- Drop-oldest backpressure means a slow client can miss events. This is
  intentional — the stream is a live console feed, not a durable queue.
  Clients needing durability should poll the list endpoint.
- Inhibits do not produce a delivery record when suppressing. This means
  the audit trail shows the source firing but not the target being
  suppressed. Operators needing suppressed-alert audit should rely on the
  inhibit row itself (which carries the reason and is append-only on
  create/delete).
- Adding a disable-inhibit path (PATCH `enabled`) is a future increment;
  M51 ships create/list/delete only. The `enabled` column and the
  `Enabled` field exist so the schema is forward-compatible.
- The inhibit active window (`DefaultInhibitActiveWindow = 5m`) is a
  compile-time constant. A source alert whose last delivery is older than
  5m is treated as no longer firing. This mirrors the silence evaluation
  cadence; making it per-inhibit configurable is a future increment.
