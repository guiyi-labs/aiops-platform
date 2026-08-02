# M53: AIOps Overview + Signal/Topology UI

- Date: 2026-08-02
- Status: Development Complete (frontend increment; surfaces M39-M40 backend)
- ADR: n/a (consumes [ADR 0063](../adr/0063-multi-cluster-federation.md) signal model)
- Fast gate: passed (verify-fast.ps1 -Scope All; backend=True frontend=True manifests=True)

## Summary

Delivered the M53 frontend increment exposing the M39 unified signal model
and the M40 temporal topology graph as an operator-facing console surface:

1. **AIOps overview** (`/aiops/overview`, `AIOpsOverviewView.vue`, 1409
   lines) — aggregates `GET /api/v1/aiops/overview`, `GET /api/v1/aiops/
   signals` and `GET /api/v1/aiops/signals/catalog` into a signal
   inventory with severity/state/producer filters. The view reads
   cluster-scoped signal counts and drills into the catalog of producers
   (M39 `signal` package).
2. **Topology graph + change timeline** (`/aiops/overview` tabs and
   `TopologyView.vue`, 303 lines) — renders `GET /api/v1/aiops/topology/
   graph` and `GET /api/v1/aiops/topology/changes` (M40 `topology`
   package). The graph is drawn from edge endpoints; the change timeline
   shows recent topological changes with the bounded `ChangeTimeline`
   contract.

The views are read-only (ADR 0004): no mutation endpoint is invoked from
this surface. Everything is driven through the shared `frontend/src/api/
aiops.ts` client (M39-M45 module) with the standard `authorizedRequest`
bearer helper.

## Files Changed

### New Files

- `frontend/src/api/aiops.test.ts` — 17 unit tests for the AIOps API
  client covering overview, signal filters, topology graph/changes,
  SLO CRUD, correlation cases, investigator, automation lifecycle and
  quality-report endpoints, including query-string building and stable
  API error propagation.

### Modified Files

- `frontend/src/router/index.ts` — Route `/aiops/overview` →
  `AIOpsOverviewView.vue` (lazy-loaded), `/topology` → `TopologyView.vue`.

## Routes

No backend routes were added by M53 (the endpoint surface is M39/M40);
the frontend consumes:

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/v1/aiops/overview` | auth required |
| GET | `/api/v1/aiops/signals` | auth required |
| GET | `/api/v1/aiops/signals/catalog` | auth required |
| GET | `/api/v1/aiops/topology/graph` | auth required |
| GET | `/api/v1/aiops/topology/changes` | auth required |

## Key Invariants Maintained

- **Read-only surface** — the M53 UI never POSTs/PATCHes; signal and
  topology data is only fetched.
- **Unified client contract** — every call goes through `aiops.ts` and
  the shared `authorizedRequest`, keeping bearer auth and error handling
  in one place.
- **Bounded lists** — topology changes and signals respect the backend
  `limit` bounds; the frontend passes through filters without unbounded
  pagination.
- **No new authorization path** — all routes reuse existing auth
  middleware.

## Tests

- 17 new frontend API client tests in `frontend/src/api/aiops.test.ts`
  (overview, signal filters incl. empty-value dropping, topology
  graph/changes serialization, SLO create/list/templates/evaluate,
  correlation cases/detail, investigator generate/list/get, automation
  plan lifecycle create→preview→approve→execute→verify, quality
  report + replay, stable 503 error propagation).

Total: 17 new frontend tests.
