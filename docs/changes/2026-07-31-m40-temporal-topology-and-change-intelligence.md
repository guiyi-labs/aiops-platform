# M40: Temporal Topology And Change Intelligence

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0055](../adr/0055-temporal-topology-and-change-intelligence.md)
- Fast gate: 73.46s (29 backend packages including `topology`, 81 frontend
  tests/18 files, Compose/Kustomize contracts)

## Summary

Introduced a persisted temporal topology graph and a unified change timeline
that normalizes M23-M31 platform-operation outcomes. M40A defines the eight
reviewed edge kinds and the change-event data model; M40B implements the
topology edge collector and service; M40C implements the change-timeline
normalizer; M40D wires the read-only HTTP routes and OpenAPI schemas; M40E
adds unit tests, ADR 0055 and this change record.

The native M21-M31 signal path is unchanged; M40 is an evidence-graph and
change-timeline normalizer, not a second alert/diagnosis/workflow system.
M40 begins the temporal-correlation foundation that M41 (SLO), M42
(correlation/RCA) and M43 (AI investigator) build on.

No public API contract was broken beyond the two new `aiops/topology` routes
documented in OpenAPI.

## Changes

### M40A — Data model and migration

#### New files

- `backend/internal/topology/model.go`: defines `EdgeKind` (8 values: Owns,
  Selects, RoutesTo, BackedBy, RunsOn, Mounts, Scales, ProtectedBy),
  `DerivationMethod` (8 values), `ResourceCitation` (kind/namespace/name/
  UID/incomplete), `Edge`, `EvidenceRef`, `EdgeFilter`, `EdgeListResponse`,
  `TopologyGraph`, `GraphNode`, `GraphCompleteness`, `ChangeEvent`,
  `ChangeTimelineFilter`, `ChangeTimelineResponse`.
- `backend/migrations/000029_topology_edges_and_change_events.up.sql`:
  creates `topology_edges` (with partial unique index
  `uq_topology_edges_active` for at-most-one-active-edge, CHECK constraints
  on `kind` and `derivation`, query indexes on cluster/namespace/source/
  target/validity) and `change_events` (with unique index
  `uq_change_events_plan` for idempotent ingestion, CHECK constraints on
  `kind`/`result`/`confidence`/`source`, query indexes on cluster/namespace/
  kind/target/started_at).
- `backend/migrations/000029_topology_edges_and_change_events.down.sql`:
  drops both tables.

### M40B — Topology edge collector and service

#### New files

- `backend/internal/topology/collector.go`: defines `ResourceReader` interface
  (bounded Kubernetes reads for 8 resource types), `CollectorSnapshot`,
  `Collector` with `Snapshot` (paged reads with 1000-page safety cap) and
  `DeriveEdges` (deterministic derivation of all 8 edge kinds). Each edge
  carries a `SourceHash` so the service can detect unchanged edges. Helpers:
  `buildOwnerIndex`, `citationFromMeta`, `mapSelectorMatches`,
  `computeEdgeHash`, `SortEdges`.
- `backend/internal/topology/repository.go`: defines `Repository` interface
  (`UpsertEdge`, `CloseEdge`, `ListEdges`, `UpsertChangeEvent`,
  `ListChangeEvents`), `GormRepository` (with `ON CONFLICT DO UPDATE` for
  edge refresh and change-event idempotency), `NopRepository` for
  disabled/testing mode. Row conversion helpers preserve incomplete flags.
- `backend/internal/topology/service.go`: defines `Service` with
  `CollectNamespace` (snapshot → derive → upsert → close stale),
  `CollectCluster` (iterate visible namespaces), `GetTopologyGraph` (build
  nodes from edge endpoints, compute completeness), `GetChangeTimeline`,
  `IngestChangeEvent` (validated persistence). `NamespaceLister` interface
  for cluster-wide collection. `CollectionResult` and
  `ClusterCollectionResult` report persisted/closed counts and partial flag.

### M40C — Change timeline normalizer

#### New files

- `backend/internal/topology/normalizer.go`: defines `ChangeNormalizer` (pure
  mapping function), `ChangePlanInput` (typed input independent of domain
  packages), `AuditChangeInput`, `FromPlan` (normalizes domain status to
  change_events result), `FromAudit` (audit-sourced events). Helpers:
  `normalizePlanStatus`, `normalizeAuditResult`, `defaultActionForKind`,
  `HashSafeDiff`, `FormatPlanIDHash`, `JoinEvidenceKinds`. Validation:
  `isValidChangeKind`, `isValidChangeResult`, `isValidConfidence`,
  `isValidChangeSource`.

### M40D — HTTP routes and OpenAPI

#### New files

- `backend/internal/httpserver/topology.go`: defines `topologyHandler` with
  `getTopologyGraph` (GET /api/v1/aiops/topology/graph) and
  `listChangeEvents` (GET /api/v1/aiops/topology/changes). Both return 503
  when the service is unconfigured, 400 on invalid query params, 200 with
  the bounded result otherwise.

#### Modified files

- `backend/internal/httpserver/router.go`: added `TopologyService` to
  `Options`; registered the two topology routes under the `aiopsRoutes`
  group (when the service is non-nil) via `RouteDescriptor` with
  `AuditAction` and `AuditResource` tags.
- `backend/internal/httpserver/openapi_route_test.go`: wired
  `topology.NewService(nil, topology.NopRepository{}, nil)` into the
  route-contract test so the M40 routes are registered and bidirectional
  parity is verified.
- `docs/api/openapi.yaml`: added the two topology paths and the
  `TopologyGraph`, `GraphNode`, `GraphCompleteness`, `TopologyEdge`,
  `TopologyResourceCitation`, `TopologyEvidenceRef`,
  `ChangeTimelineResponse`, `ChangeEvent` schemas.

### M40E — Unit tests, ADR and docs

#### New files

- `backend/internal/topology/collector_test.go`: 13 tests covering each edge
  kind derivation (Owns with/without unknown owner, Selects exact match/empty
  selector, RoutesTo complete/incomplete target, BackedBy, RunsOn, Mounts
  dedupe, Scales, ProtectedBy), the all-kinds integration test, edge hash
  determinism and selector match helper.
- `backend/internal/topology/normalizer_test.go`: 11 tests covering
  `FromPlan` (succeeded/pending/expired→failed/partial/default action/
  validation errors), `FromAudit` (succeeded/denied→failed),
  `normalizePlanStatus` table test, `HashSafeDiff` determinism,
  `IngestChangeEvent` validation and default-filling.
- `backend/internal/topology/service_test.go`: 5 tests covering
  `CollectNamespace` (disabled collector/empty namespace/success),
  `GetTopologyGraph` (empty/with edges), `GetChangeTimeline`.
- `docs/adr/0055-temporal-topology-and-change-intelligence.md`: records the
  six M40 decisions.
- `docs/changes/2026-07-31-m40-temporal-topology-and-change-intelligence.md`:
  this change record.

#### Modified files

- `docs/roadmap.md`: added M40 status section.
- `docs/testing/test-matrix.md`: added M40 addendum.
- `docs/development-handoff.md`: updated baseline to M40, added M40 highlights
  and closure summary row.

## Verification

- Fast gate `scripts/verify-fast.ps1 -Scope All` passed in 73.46s: 29 backend
  packages vet/test green (including `topology`), 81 frontend tests / 18
  files green, Compose and Kustomize contracts green.
- `TestRegisteredRoutesMatchOpenAPI` verifies bidirectional route↔OpenAPI
  parity for both M40 routes.
- 29 topology-package unit tests pass (13 collector + 11 normalizer + 5
  service).

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
