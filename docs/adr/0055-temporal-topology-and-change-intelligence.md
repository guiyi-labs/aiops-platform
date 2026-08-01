# ADR 0055: Temporal Topology And Change Intelligence (M40)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M40
- Supersedes: none
- Related: ADR 0016 (EndpointSlice-first), ADR 0045 (namespace posture),
  ADR 0050 (access grants), ADR 0053 (capability adapters), ADR 0054 (signal model)

## Context

M39 normalized M21-M31 outputs into a unified `signal_occurrences` table so
M40-M44 (temporal topology, SLO, correlation, AI investigator) can operate on a
stable, deduplicated, scope-filtered signal model. Before M40 there was no
persisted relationship graph and no unified change timeline: each plan/audit
record lived in its own M23-M31 table, and topology was recomputed on every
read from raw Kubernetes objects. That blocked M42 correlation (which needs
reviewed edges with validity intervals), M41 SLO views (which need stable
service identity) and the evidence-graph API required by
`docs/kubesphere-optimization-plan.md` §14.

The key constraints:

1. M40 must not persist full Kubernetes objects or raw manifests. Only reviewed
   relationship edges and normalized change references are stored.
2. Edges are derived only from exact observed evidence (OwnerReference, label
   selector, EndpointSlice endpoints, Ingress/Gateway backend, nodeName, PVC
   volume mount, HPA scaleTargetRef, PDB selector). Same-name or temporal
   proximity alone never creates an edge.
3. Edges are append-only with time validity. When a selector changes or a Pod
   is recreated, the old edge is closed (`valid_to` set) and a new edge is
   created. Historical queries return only edges valid at the requested time.
4. Platform-controlled changes are high-confidence observations. Kubernetes
   Events or external delivery adapters are lower-confidence context until
   exact identity and result are verified.
5. Hidden cluster/Namespace data must never enter graph results, counts,
  evidence or remaining-edge counts. M35 scope filtering is applied before
  aggregation.
6. Graph/time/node/edge/byte limits and cleanup retention are deterministic
  and disclosed in every response.

## Decisions

### 1. Eight reviewed edge kinds with distinct derivation methods

The catalog of edge kinds is fixed and enumerated in code:

- `Owns` (OwnerReference: Deployment→ReplicaSet, ReplicaSet→Pod)
- `Selects` (label selector: Service→Pod)
- `RoutesTo` (backend config: Ingress/Gateway→Service)
- `BackedBy` (EndpointSlice endpoints: Service→Pod)
- `RunsOn` (`spec.nodeName`: Pod→Node)
- `Mounts` (PVC volume mount: Pod→PVC)
- `Scales` (`scaleTargetRef`: HPA→Deployment/ReplicaSet)
- `ProtectedBy` (PDB selector match: Workload→PDB)

The derivation method is recorded per edge and never merged into a single
"inferred" bucket. Unknown relationships are never persisted. The CHECK
constraints on `topology_edges` enforce the enumeration at the database level.

### 2. Resource identity is cluster_id + kind + UID; name-only is incomplete

Mirroring ADR 0054 §2, an edge endpoint cites `cluster_id + kind + UID` as its
primary key. When a UID is not available in the snapshot (e.g., Node UIDs are
not in the namespace-scoped snapshot, or a referenced Service was not listed),
the citation is still recorded but `Incomplete = true`, so M42 correlation can
downgrade confidence. This avoids blocking collection while keeping the
identity contract honest.

### 3. Append-only edges with time-validity intervals

Edges are persisted in `topology_edges` with `valid_from` and `valid_to`
columns. The unique index
`uq_topology_edges_active ON (cluster_id, kind, source_uid, target_uid,
derivation) WHERE valid_to IS NULL` enforces at-most-one-active-edge per
identity. Collection works as:

1. Snapshot the namespace (bounded Kubernetes reads with paging).
2. Derive edges deterministically from the snapshot.
3. Upsert each derived edge (refreshes `last_observed_at` on conflict).
4. List active edges for the namespace and close any not present in the
   derived set (`valid_to = now`).

Historical queries use `valid_from <= t AND (valid_to IS NULL OR valid_to > t)`.
Default queries return only active edges (`valid_to IS NULL`).

### 4. Change timeline normalizes M23-M31 outcomes into one table

`change_events` stores normalized platform-operation outcomes from M23-M31:
`promotion`, `backup`, `maintenance`, `restore`, `rollout`, `audit`. Each event
links the domain plan_id, audit_id and request_id and records target
resource, action, safe diff hash, revision, actor, start/end, result
(`succeeded`/`failed`/`pending`/`partial`), confidence (`high`/`low`) and
source (`platform`/`k8s_event`/`delivery_adapter`).

The `ChangeNormalizer` is a pure mapping function: it accepts a typed
`ChangePlanInput` (built by the caller from the domain-specific Plan struct)
so the topology package stays independent of the remediation/promotion/backup/
maintenance/restore packages. Domain statuses are normalized:
`succeeded`→`succeeded`, `failed`/`expired`→`failed`, `partial`→`partial`,
`awaiting_confirmation`/`executing`/`""`→`pending`.

Confidence is `high` for platform-sourced events with an audit ID and `low`
otherwise. Audit-sourced events are `high` when they carry a non-empty
`request_id`. The unique index `uq_change_events_plan ON (kind, plan_id) WHERE
plan_id != ''` makes ingestion idempotent.

### 5. Read-only HTTP surface with bounded limits and completeness indicators

The HTTP API exposes two read-only endpoints:

- `GET /api/v1/aiops/topology/graph` — returns the active topology graph for a
  namespace: nodes (derived from edge endpoints with edge counts), edges and a
  completeness indicator (`complete`/`partial`/`unavailable`/`truncated` with
  `truncated` and `remaining` fields).
- `GET /api/v1/aiops/topology/changes` — returns normalized change events
  filtered by cluster, namespace, kind and time range.

Edge collection and change-event ingestion are internal: the collector and
normalizer are called by background workers or plan-completion hooks, not by
HTTP clients. Both endpoints require authentication; M35 scope filtering is
applied by the middleware chain. Limits are bounded (graph max 500 edges,
timeline max 200 events) and truncation is disclosed in every response.

### 6. Disabled by default; NopRepository for testing

The topology service is wired into `Options.TopologyService`. When nil, the
topology routes are not registered. The `NopRepository` implementation allows
the service to be instantiated without a database for testing and for the
route-contract parity test. The collector can also be nil (query-only mode):
`GetTopologyGraph` and `GetChangeTimeline` work without a collector, but
`CollectNamespace`/`CollectCluster` return an error.

## Consequences

- Adding a new edge kind requires: a new `EdgeKind` constant, a new
  `DerivationMethod` constant, a derivation function, a CHECK constraint
  update in the migration, a catalog/test fixture and an OpenAPI schema
  update. This is a contract change, not a runtime configuration.
- The collector reads eight resource types per namespace per collection pass.
  This is bounded by the existing `apiquery` paging limits and the 1000-page
  safety cap per resource type. Collection is serial per namespace; callers
  control cluster-wide concurrency.
- Stale edges are closed lazily during collection. A namespace that is never
  collected again will retain active edges from its last snapshot. A future
  retention worker (deferred) can close edges whose `last_observed_at` is
  older than a threshold.
- The `ResourceCitation` and `EvidenceRef` types mirror the M39 signal model
  so M42 correlation can consume topology edges without a translation layer.
- The change timeline is a read-only projection of M23-M31 plan/audit records.
  It does not replace the native plan/audit tables; it normalizes them for
  cross-domain timeline queries.

## Deferred

- Background collection worker that periodically snapshots namespaces and
  persists edges (the `CollectNamespace`/`CollectCluster` API is ready; the
  worker is deferred).
- Plan-completion hook that ingests change events when M23-M31 plans finish
  (the `IngestChangeEvent` API is ready; the hook is deferred).
- Real PostgreSQL integration test for `GormRepository` (needs full Compose
  stack).
- Real-kind E2E for topology edge derivation and change timeline (needs a
  multi-worker kind cluster with the full resource set).
- Frontend UI for the topology graph and change timeline.
- Retention worker that closes stale edges and prunes old change events.
