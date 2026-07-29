# ADR 0036: Authenticated Exact-Series Metrics History

- Status: Accepted
- Date: 2026-07-29
- Owners: Backend and platform operations

## Context

ADR 0034 established a durable sparse-series contract and ADR 0035 added the
bounded collector. The domain service already validates one exact series, a
24-hour maximum window and 1,440-point cap, but no authenticated consumer can
read that contract. Exposing arbitrary labels, aggregation or PromQL would
expand cardinality and authorization beyond the accepted M21 boundary.

## Decision

Expose one authenticated read-only route:

`GET /api/v1/clusters/{cluster_id}/metrics/history`

The request must supply `resource_kind`, `name`, `metric`, `from` and `to`.
Pod series also require exact `namespace` and `container`; Node series forbid
both. CPU and memory are the only metrics. `from` is inclusive, `to` is
exclusive, both are RFC3339 timestamps, the window is at most 24 hours and
`limit` is 1 through 1,440 with 1,440 as the default.

All authenticated roles may read the route because it adds no target-cluster
verb. The response returns canonical series identity, ordered sparse points,
explicit succeeded/partial/unavailable/timed-out/failed collection coverage,
missing count, fixed limits and truncation. Missing collections never become
zero-valued points. A deleted cluster returns `CLUSTER_NOT_FOUND`; malformed
input returns `INVALID_QUERY`; repository details are hidden behind
`METRICS_HISTORY_QUERY_FAILED`.

The Compose runtime gate seeds two clusters and multiple series, proves exact
cluster/series isolation and one sparse gap, restarts the backend, repeats the
query against PostgreSQL and deletes every synthetic fixture.

## Consequences

- Trend and incident consumers gain one stable bounded source without a query
  language or unbounded labels.
- Disabled clusters remain historically queryable until deletion; deleting a
  cluster still cascades its history.
- The route does not downsample, aggregate, interpolate, fill gaps, draw charts
  or evaluate sustained alert windows. Those remain later M21 phases.
- Multi-tenant cluster authorization is not invented; the route follows the
  platform's existing authenticated cluster visibility model.
