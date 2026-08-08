# M83: Topology Deepening — Gateway API read-only browse + collapse view (W7)

- Date: 2026-08-09
- Status: Development Complete (local)
- ADR: docs/adr/0080-m83-topology-deepening.md
- Fast gate: PASSED — backend go vet/build/test green; OpenAPI parity green.

## Summary

Two increments for polish-plan W7:

1. **Gateway API read-only browse**: `gateway.networking.k8s.io/v1` Gateways,
   HTTPRoutes and GatewayClasses are now browsable through the operator-curated
   CRD whitelist with correct scoping (GatewayClass cluster-scoped; Gateway and
   HTTPRoute namespaced). All browse stays read-only (ADR 0064 / ADR 0004).
2. **Collapse view**: `GET /api/v1/aiops/topology/graph?collapse=1` collapses
   duplicate edges between the same source/target pair+kind into a single
   representative edge carrying an advisory `aggregate_count`, bounding
   large-graph payloads without changing data or the default 200-edge limit.

## Files Changed

- Backend:
  - `backend/internal/kubernetes/service.go` — Gateway API whitelist entries.
  - `backend/internal/kubernetes/gateway_whitelist_test.go` — browsability +
    scoping test (incl. negative anti-leakage tuples).
  - `backend/internal/topology/model.go` — `Edge.AggregateCount` + `SetEdgeCount`/
    `IncEdgeCount` advisory helpers.
  - `backend/internal/httpserver/topology.go` — `collapse` query param +
    `collapseEdges` read-only view transform.
  - `backend/internal/httpserver/topology_collapse_test.go` — collapse behavior,
    empty-input, JSON projection and query-param tests.
- Contract / Docs:
  - `docs/api/openapi.yaml` — `collapse` query param on `/aiops/topology/graph`.
  - `docs/adr/0080-m83-topology-deepening.md` — new ADR.

## Verification (local)

- `go vet ./internal/...`, `go build ./...` — green.
- `go test ./internal/kubernetes/... ./internal/topology/... ./internal/httpserver/...`
  — green (incl. OpenAPI route parity).

## Notes

- The frontend topology view renders a client-side graph and independently
  exercises Ingress/Service/EndpointSlice/pod data; the backend graph endpoint
  + collapse view serve the bounded server-side path. Real-cluster kind E2E
  (Gateway object present in a live cluster) remains a post-push follow-up
  because Docker is not running locally tonight.