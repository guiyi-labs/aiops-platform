# M48: Multi-Cluster Federation (Host/Member Model)

- Date: 2026-08-01
- Status: Development Complete (backend increment only; local development deliverables)
- ADR: [0063](../adr/0063-multi-cluster-federation.md)
- Fast gate: passed (verify-fast.ps1 -Scope All, 56.89s; backend=True frontend=True manifests=True)

## Summary

Delivered the M48 backend increment implementing a KubeSphere-style
multi-cluster federation model (host/member clusters). The federation layer is
a **SQL aggregation view** over the existing `clusters` table — there is no
new CRD, no Cluster Agent / Tower side channel, no inter-cluster resource
sync controller, and no transparent K8s API proxy. Cross-cluster operations
still go through the existing explicit `cluster_id` + fixed GVR whitelist
pattern (ADR 0004).

Three capabilities are delivered:

1. **Host/Member topology** — clusters can be registered as host or member.
   At most one host is permitted (enforced at both the service layer via
   `CountHost` and the database layer via a partial unique index). Standalone
   clusters (the default) are not part of the federation but appear in the
   overview.
2. **Federation health dimension** — `federation_status`
   (registered/healthy/degraded/disconnected) is orthogonal to the existing
   `clusters.status` (enabled/disabled/unreachable). The existing cluster
   probe remains the source of truth for `clusters.status`; federation status
   is operator- or heartbeat-driven.
3. **Cross-cluster resource summary** — a bounded fan-out (20 clusters, 4s
   per-cluster timeout) that aggregates resource counts across visible
   clusters for a fixed 9-entry GVR whitelist. Missing/unreachable clusters
   contribute zero counts with an error code (`TIMEOUT` / `QUERY_FAILED`);
   partial results are always returned.

Every federation state transition (registration, deregistration, role change,
heartbeat, status change) is recorded in an append-only
`cluster_federation_events` table — no UPDATE/DELETE path is exposed by the
repository. This mirrors the platform audit pattern (ADR 0008).

Anti-leakage (404 > 403) is preserved: `ErrClusterNotFound` surfaces as 404
so a missing cluster is indistinguishable from an unauthorized one. Federation
read operations require authentication only (visible clusters are narrowed by
the caller's authz scope); write operations require `operations_admin` role.

M48 production gates (hosted CI, production OIDC/MFA, HA PostgreSQL, signed
releases, real-kind E2E) remain external and are not closed by this
development deliverable.

## Changes

### New files

- `backend/migrations/000035_cluster_federation.up.sql` — extends `clusters`
  with `cluster_role`, `federation_status`, `registered_at`,
  `last_heartbeat_at` columns (CHECK-constrained enums); creates
  `cluster_federation_events` append-only table; adds partial unique index
  `clusters_single_host_uq` for the single-host invariant.
- `backend/migrations/000035_cluster_federation.down.sql` — rollbacks for the
  above.
- `backend/internal/federation/model.go` — data models
  (`FederationEvent`, `ClusterSummary`, `Overview`, `ResourceSummary`,
  `ResourceSummaryEntry`, `ClusterCount`), event-type constants, sentinel
  errors.
- `backend/internal/federation/repository.go` — `Repository` interface
  (context-aware, append-only events), `GormRepository` production
  implementation, `nopRepository` for disabled-federation deployments.
- `backend/internal/federation/service.go` — `Service` with
  `RegisterCluster`, `DeregisterCluster`, `PromoteToHost`, `DemoteHost`,
  `RecordHeartbeat`, `UpdateFederationStatus`, `Overview`, `ListEvents`,
  `ListEventsByCluster`, `ResourceSummary`. `ClusterLister` interface decouples
  the service from the kubernetes package. `FixedGVRWhitelist` (9 entries).
- `backend/internal/federation/service_test.go` — 38 unit tests covering
  register/deregister/promote/demote/heartbeat/status/overview/events/
  resource-summary, single-host invariant, idempotency, anti-leakage,
  timeout error mapping, and helper functions.
- `backend/internal/httpserver/federation.go` — HTTP handlers for all 9
  federation routes; `writeFederationError` maps sentinel errors to stable
  HTTP status codes.
- `backend/internal/httpserver/federation_test.go` — 20 handler tests
  covering all routes with 200/400/404/409/503 paths.
- `backend/cmd/server/federation_lister.go` — `kubernetesClusterLister`
  adapter that translates `kubernetes.Service` typed list methods into
  `federation.CountResult`.
- `docs/adr/0063-multi-cluster-federation.md` — ADR documenting the 7 key
  decisions: extend clusters table, append-only events, service invariants,
  overview aggregation, resource summary fan-out, authorization/anti-leakage,
  route/OpenAPI contract.

### Modified files

- `backend/internal/cluster/model.go` — added `ClusterRole`,
  `FederationStatus`, `RegisteredAt`, `LastHeartbeatAt` fields and
  `ClusterRole*` / `FederationStatus*` constants.
- `backend/internal/httpserver/router.go` — registers 9 federation routes
  under `/api/v1/federation` (conditional on `FederationService` + `Auth`
  being non-nil).
- `backend/internal/httpserver/openapi_route_test.go` — added
  `FederationService` to the route-contract test setup.
- `backend/cmd/server/main.go` — initializes `federationService` and wires it
  into `httpserver.Options`.
- `docs/api/openapi.yaml` — added 9 federation paths, request/response schemas
  (`FederationOverview`, `FederationEvent`, `FederationEventList`,
  `FederationResourceSummary`, `FederationCluster`,
  `RegisterFederationClusterRequest`, `DemoteFederationClusterRequest`,
  `FederationHeartbeatRequest`, `UpdateFederationStatusRequest`).

## Verification

- `verify-fast.ps1 -Scope All`: passed in 56.89s (backend=True,
  frontend=True, manifests=True).
- `go test ./internal/federation/...`: 38 tests, all pass.
- `go test ./internal/httpserver/ -run TestFederationHandler`: 20 tests,
  all pass.
- `TestRegisteredRoutesMatchOpenAPI`: pass — all 9 federation routes are
  registered and documented.
- `go vet ./...`: clean.

## Key invariants maintained

- **Single-host invariant**: enforced at service layer (`CountHost`) and
  database layer (partial unique index `clusters_single_host_uq`).
- **Append-only events**: `cluster_federation_events` has no UPDATE/DELETE
  path in the repository interface.
- **404 > 403 anti-leakage**: `ErrClusterNotFound` → 404; unauthorized
  clusters are indistinguishable from missing ones.
- **federation_status orthogonal to clusters.status**: the existing probe
  updates `clusters.status`; the federation heartbeat updates
  `federation_status`. A cluster can be `ready` but `degraded`.
- **Bounded resource summary fan-out**: 20 clusters max, 4s per-cluster
  timeout, partial results always returned.
- **Static extension model**: no new CRD, no control plane, no side channel.
  Cross-cluster operations use the existing `cluster_id` + fixed GVR
  whitelist pattern (ADR 0004).
