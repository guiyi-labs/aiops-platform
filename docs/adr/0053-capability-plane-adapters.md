# ADR 0053: Capability Plane Adapters

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M37 (Capability Plane Adapters), ADR 0034 (metrics history
  storage), ADR 0035 (bounded background metrics collection), ADR 0043 (historical
  alert lifecycle), ADR 0022 (durable diagnosis notifications)

## Context

The platform has mature M21 metrics history, M27 alert lifecycle and M20
notification outbox systems. These are native infrastructure defaults — they
work without any external provider. However, the AIOps differentiation route
(M39–M44) needs richer evidence sources: service-level metrics from Prometheus,
historical logs from Loki, alert routing with bounded silences, Gateway API
traffic evidence and delivery metadata.

`docs/kubesphere-optimization-plan.md` (M37) requires bounded provider
contracts that are evidence sources for later AIOps phases. They are not
separate product centers and must not delay the native M21–M31 signal path.

## Decision

### 1. Compiled provider interfaces, not runtime plugins

M37 adapters are compiled-time interfaces registered in `main.go`. No dynamic
code download, JS injection, arbitrary proxy or unsigned Go plugin exists.
Each adapter implements a small interface defined in its package.

### 2. M37A — MetricsProvider and LogProvider

Introduce `MetricsProvider` and `LogProvider` interfaces in a new
`backend/internal/capability` package. The existing Metrics API/PostgreSQL
history remains the infrastructure default. V1 adds one Prometheus-compatible
metrics adapter for fixed service SLI templates and one read-only historical
log adapter (Loki).

Public APIs accept fixed template/query AST fields only: authorized
service/resource, exact optional cluster/Namespace/Pod/container, bounded
text, start/end, direction and limit. They never accept PromQL, LogQL,
OpenSearch DSL, arbitrary labels or provider URLs.

Provider endpoints and credentials are server-configured; request input cannot
redirect a query. Provider outage affects only its capability and returns
explicit `partial`/`unavailable` state.

### 3. M37B — Alert routing and bounded silences

Build on the existing M27 lifecycle and transactional outbox. V1 supports
exact-match route priority and an HTTPS webhook receiver. Persist route
priority, exact cluster/rule/severity match, dedupe key, group/repeat interval
and delivery timeline. Require silence reason, creator, start/end and a hard
maximum duration; permanent silence is forbidden. Store receiver credentials
as encrypted backend references and return metadata only.

### 4. M37C/D — Optional adapters deferred

Gateway API evidence (M37C) and delivery metadata (M37D) are optional and
deferred until change correlation (M40) demonstrates concrete need. They are
not blocking M39.

### 5. Security invariants

- Provider endpoints and credentials are server-configured; request input
  cannot redirect a query.
- Logs default to 1 hour, hard-stop at 7 days and enforce timeout,
  concurrency, result, byte and export bounds.
- Metrics expose template/version, sample coverage, missing-data policy and
  freshness.
- HTTPS, redirect rejection, DNS/IP SSRF policy, timeout, capped response
  and sanitized error are enforced for all outbound provider calls.
- Credentials and provider internals never enter API, audit, logs, evidence
  or Git.
- M35 scope isolation applies to all adapter data.

### 6. No second signal system

M37 adapters are evidence sources, not a parallel alert/diagnosis/workflow
system. They normalize external data into bounded contracts that the existing
M21–M31 infrastructure and the future M39 signal model consume. No adapter
creates diagnosis records, alert instances or remediation plans directly.

## Consequences

- New `backend/internal/capability` package with `MetricsProvider`,
  `LogProvider` interfaces and Prometheus/Loki adapters.
- New `backend/internal/alertroute` package with route priority, silences and
  HTTPS webhook receiver.
- New migrations for alert routes, silences and delivery records.
- New configuration blocks for Prometheus, Loki and alert routing.
- HTTP routes under `/api/v1/capability/` for metrics/logs and under
  `/api/v1/alert-routes/` for routing/silences.
- OpenAPI contracts for all new routes.
- All adapters are disabled by default; the server runs identically to the
  current deployment when no provider is configured.
