# ADR 0034: Bounded PostgreSQL Metrics History

- Status: Accepted
- Date: 2026-07-28

## Context

M15 and M16 expose real, point-in-time Node and Pod usage from the optional
Kubernetes Metrics API. The platform cannot show a trend, prove a sustained
condition or distinguish a missing interval from zero after the response is
gone. Adding a browser-side cache would lose data on restart and make evidence
depend on which operator had a page open. Treating the existing process-local
`/metrics` endpoint as resource history would also mix two unrelated contracts.

The first M21 slice needs a durable data contract before a collector, trend UI
or alert-window evaluator is allowed to depend on it. It must be small enough
for the existing PostgreSQL deployment and must not become an arbitrary
Prometheus query proxy.

## Decision

Persist collection evidence in `metric_collection_runs` and normalized values
in `metric_samples`. A collection run is always scoped to one registered
cluster and records separate Node and Pod source results, sampled/total/complete
coverage, a stable failure code, start/completion time and expiry. Samples store
only the approved series identity plus CPU nanocores or memory bytes, the source
timestamp, millisecond-precision Metrics API window and platform collection time. Raw Kubernetes
objects, labels, annotations, API Server addresses and upstream error strings
are not stored.

The reviewed hard envelope is:

- seven-day retention by default and no more than 30 days;
- no more than 1,800 normalized samples in one cluster collection;
- one exact Node or Pod-container series per query;
- no more than a 24-hour query window or 1,440 returned points;
- cleanup deletes at most 1,000 runs per transaction by default and never more
  than 5,000.

Node series require an empty Namespace and container. Pod series require both
an exact Namespace and exact container. The only metric names are `cpu` and
`memory`; their only public units are `nanocores` and `bytes`. A unique
constraint permits at most one point for the same series and metric in a
collection. All repository reads include `cluster_id` and the complete exact
series predicate. Cluster deletion cascades its history.

Collection result is derived from the two source results. A complete pair is
`succeeded`; one usable source is `partial`; two absent APIs are `unavailable`;
otherwise timeout and failure remain explicit. Non-successful results accept
only a bounded stable failure code, never a raw upstream message. Query coverage
reports collection outcomes, actual point count and missing count. Missing
samples remain absent; the service does not insert zeroes, carry values forward
or interpolate.

## Consequences

PostgreSQL supplies restart durability, transactions, foreign-key isolation and
deterministic bounded cleanup without introducing another production data
service. A later collector can convert Kubernetes quantities before calling
this contract, and trend/alert consumers can use one sparse evidence model.

This slice does not start background collection, expose an HTTP history route,
draw charts or evaluate alert windows. Those are subsequent M21 phases and must
reuse these caps. Raising retention, collection cardinality, query range or
series breadth requires a new capacity review; adding generic PromQL, arbitrary
labels or user-supplied Kubernetes selectors is outside this decision.
