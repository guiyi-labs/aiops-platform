# ADR 0020: Low-cardinality HTTP metrics

- Status: Accepted
- Date: 2026-07-17

## Context

Operators need request volume and latency visibility across the API, but using
raw URL paths would create unbounded labels containing IDs and potentially
sensitive values. The platform also has no dependency on a full metrics
backend in the application process.

## Decision

Install a process-local Gin middleware that records request count and total
duration keyed only by method, registered route template and HTTP status class.
Expose a small Prometheus text endpoint at `/metrics`. The endpoint is not a
user API and is unauthenticated; deployment configuration must restrict it to
the trusted monitoring network.

## Consequences

The metrics are bounded and safe to aggregate, and the service remains
portable without a collector SDK. The in-memory series reset on process
restart, and production deployments must provide scrape, retention and access
control outside the application. Any new route automatically appears using
its Gin template, while unmatched requests are grouped under `unmatched`.
