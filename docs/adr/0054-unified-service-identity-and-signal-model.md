# ADR 0054: Unified Service Identity and Signal Model (M39)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M39
- Supersedes: none
- Related: ADR 0016 (EndpointSlice-first), ADR 0050 (access grants), ADR 0053 (capability adapters)

## Context

M21-M31 produced high-quality operator workflows (diagnosis, alert lifecycle,
metric evaluation, namespace posture, promotion, backup, maintenance, restore)
but each persisted its own native model. There was no normalized signal
envelope, no shared service identity, and no single API that answered "what is
happening across my fleet right now?" The `docs/kubesphere-optimization-plan.md`
M39 section required normalizing existing outputs before adding more algorithms,
so that M40-M44 (temporal topology, SLO, correlation, AI investigator) can
operate on a stable, deduplicated, scope-filtered signal model rather than
re-reading each producer's native tables.

The key constraints:

1. M39 must not create a second alert, diagnosis, workflow or authorization
   system. It normalizes existing outputs.
2. Native M21-M31 signals must work even when every optional M37 provider is
   disabled.
3. Duplicate/concurrent/restarted producer delivery must yield one stable
   occurrence per fingerprint contract.
4. Late, out-of-order, clock-skewed and expired signals must have deterministic
   behavior.
5. Hidden cluster/Namespace data must never enter results, counts, errors or
   evidence references.
6. Provider failure must remain source-local and incomplete evidence must not
   appear healthy.

## Decisions

### 1. Signal envelope with fail-closed catalog

A compiled `SignalDescriptor` catalog maps each signal code to its schema
version, domain, severity policy, correlation dimensions, required evidence,
allowed action codes and retention. Unregistered signals fail closed at
ingestion — `BuildOccurrence` returns an error and `Service.Ingest` wraps it as
`ErrUnregisteredSignal`. This prevents shadow signals from polluting the model.

The normalized envelope (`Occurrence`) carries: signal_id, producer,
cluster_id, namespace, resource citation (kind/namespace/name/UID/incomplete),
severity, state, fingerprint, coverage, freshness, window start/end, observed_at,
ingested_at, expires_at, safe attributes, evidence refs and ingestion_run_id.
It never contains raw telemetry, full manifests, secret values or complete log
bodies.

### 2. Primary resource key is cluster_id + kind + UID; name-only is incomplete

A service identity is derived from exact observed relationships. The primary
key is `cluster_id + kind + UID`. When a producer cannot supply a UID (e.g.,
M27 alert rules only persist ResourceName), the occurrence is still accepted
but `ResourceCitation.Incomplete` is true, so downstream correlation (M40/M42)
can downgrade confidence. This avoids blocking ingestion while keeping the
identity contract honest.

### 3. Fingerprint deduplication over identity fields, not observed_at

The fingerprint is a SHA256 over `signal_id + cluster_id + resource
(kind/namespace/name/uid) + window_start + window_end`. `ObservedAt` is
intentionally excluded: a re-delivery of the same event at a later time must
not create a new row. The database enforces this with a unique index on
`(signal_id, fingerprint)` and an `ON CONFLICT DO UPDATE` that refreshes
state, severity, freshness, observed_at, evidence and expires_at without
touching identity fields.

### 4. SourceReader interface keeps the overview bounded and scope-safe

The `Overview` endpoint aggregates source completeness, active diagnoses, top
signals, recent changes and action outcomes. Rather than importing every
producer package into the signal service, a `SourceReader` interface is defined
and implemented as a thin adapter in the HTTP layer. The service itself only
reads from `signal_occurrences` and the `SourceReader`; it never directly
queries diagnosis, alert, promotion, backup, maintenance or restore tables.

M35 scope filtering is applied by the middleware chain, not by the signal
service. The service accepts an optional `clusterID` and `namespace` for
query bounding, but the authorization decision is upstream.

### 5. PostgreSQL append-only with TTL-bound retention

`signal_occurrences` is append-only with a TTL-bound `expires_at` column. A
periodic `CleanupRetention` worker deletes expired rows in bounded batches.
There is no raw telemetry warehouse, full manifest or complete log body — the
table stores only the normalized envelope and redacted evidence refs.

### 6. Disabled by default; native signals work without M37

The signal service is disabled by default (`SIGNAL_ENABLED=false`). When
disabled, the `SignalService` option is nil and no aiops routes are registered.
Native M21-M31 signals work independently — they do not depend on M37 optional
providers. When enabled, the service uses `NopRepository` and
`NopSourceReader` by default, so it can be wired without a database for testing.

## Consequences

- Adding a new signal requires: a `SignalDescriptor` entry in the catalog, a
  normalizer adapter, an OpenAPI schema update, and unit tests. This is a
  contract change, not a runtime configuration.
- The `SourceReader` interface means the overview's recent-changes and
  active-diagnoses sections are only populated when the adapter is wired. With
  `NopSourceReader`, the overview still returns top signals from
  `signal_occurrences` but reports zero active diagnoses and empty source
  completeness.
- M40 temporal topology will consume signal occurrences as evidence nodes; the
  `EvidenceRef` and `ResourceCitation` types are designed for that reuse.
- The `SignalDescriptor.AllowedActions` field is intentionally a list of codes,
  not an execute endpoint. M42 will propose candidates; execution stays in the
  existing M23-M31 plan/confirmation/audit path.

## Deferred

- Concrete `SourceReader` implementation that reads from diagnosis, alert,
  promotion, backup, maintenance and restore services (the interface and
  `NopSourceReader` are in place; the adapter is deferred to M40 when
  temporal topology needs it).
- Batch ingestion worker that periodically normalizes M21-M31 outputs into
  signal occurrences (the `IngestBatch` API is ready; the worker is deferred).
- Real PostgreSQL integration test for `GormRepository` (needs full Compose
  stack).
- Frontend UI for the AIOps overview and signal list.
