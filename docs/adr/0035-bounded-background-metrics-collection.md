# ADR 0035: Bounded Background Metrics Collection

- Status: Accepted
- Date: 2026-07-28

## Context

ADR 0034 defines durable, sparse Node and Pod-container history, but no process
produces collection runs. M21 Phase 2 must sample every enabled cluster without
turning one slow or malformed Metrics API into a platform-wide outage. The
collector also needs exact Kubernetes quantity semantics; ad hoc parsing of
decimal SI, binary SI and nano CPU values would corrupt historical evidence.

The existing Metrics API gateway already bounds response bodies and removes
arbitrary labels. The collector should reuse that gateway and the Phase 1
repository instead of introducing another Kubernetes client or storage path.

## Decision

Run one background collector in the API process. It is enabled by default and
can be disabled with `METRICS_HISTORY_ENABLED=false`. A cycle runs immediately
after startup and then every 60 seconds by default. It lists registered
clusters, keeps only enabled entries, sorts them by numeric ID and samples at
most 20 clusters. At most four clusters run concurrently. Each cluster gets an
independent 10-second context, and its Node and Pod Metrics API reads run in
parallel under that context.

The hard runtime envelope is configurable only within these reviewed caps:

- 1..20 selected clusters and 1..4 concurrent clusters, with concurrency no
  greater than the selected-cluster limit;
- a collection interval from 15 seconds through 24 hours;
- a per-cluster timeout from one second through one minute;
- no more than the ADR 0034 limit of 1,800 normalized samples per cluster run;
- cleanup immediately after startup and every one minute through 24 hours, one
  existing bounded repository batch per tick.

Metrics responses are sorted by stable resource identity before conversion.
Node and Pod bundles are admitted in round-robin order so a large Node list
cannot silently consume the entire collection. A Pod bundle is atomic across
all containers and both approved metrics. Coverage counts fully admitted Nodes
or Pods; response paging or sample-cap exhaustion produces incomplete coverage
and `COLLECTION_LIMIT_REACHED` rather than pretending the collection succeeded.

Use `k8s.io/apimachinery/pkg/api/resource` for quantity parsing. CPU is stored
as exact nanocores and memory as bytes. Negative, overflowing or syntactically
invalid quantities fail that complete source atomically. Timestamp and window
parsing, resource identity, duplicate resources and list totals are validated
before any source samples are admitted.

Persist only these stable failure codes:

- `METRICS_API_UNAVAILABLE`;
- `METRICS_API_TIMEOUT`;
- `METRICS_API_REQUEST_FAILED`;
- `METRICS_QUANTITY_INVALID`;
- `METRICS_PAYLOAD_INVALID`;
- `COLLECTION_LIMIT_REACHED`.

Raw upstream errors are never copied into history rows. Node and Pod failures
are isolated, so one usable source still produces a partial collection. Mixed
failures select a deterministic code consistent with the derived collection
status. A shutdown cancellation stops new work, cancels in-flight API calls and
waits for the collector and notification dispatcher before the database closes.

## Consequences

Restarting the API process resumes collection without browser involvement, and
existing PostgreSQL rows remain the only history source of truth. Collection
cadence, coverage, expiry and failure evidence are now durable and bounded.

The in-process scheduler is intentionally single-replica. Running multiple API
replicas would duplicate collection runs and requires a later leader-election
decision. This phase does not expose history over HTTP, draw trends, evaluate
sustained alert windows or claim Prometheus-scale retention/cardinality.
