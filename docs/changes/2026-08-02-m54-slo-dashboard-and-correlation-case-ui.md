# M54: SLO Dashboard + Correlation Case UI

- Date: 2026-08-02
- Status: Development Complete (frontend increment; surfaces M41-M42 backend)
- ADR: n/a (consumes M41 SLO error-budget and M42 correlation contracts)
- Fast gate: passed (verify-fast.ps1 -Scope All; backend=True frontend=True manifests=True)

## Summary

Delivered the M54 frontend increment exposing the M41 SLO error-budget
and the M42 multi-signal correlation case model as console surfaces:

1. **SLO dashboard** (`/aiops/slo`, `SLODashboardView.vue`, 605 lines) —
   lists SLO definitions via `GET /api/v1/aiops/slos` with
   cluster/namespace/enabled filters, evaluates an SLO on demand via
   `POST /api/v1/aiops/slos/:id/evaluate`, and shows the evaluation
   history via `GET /api/v1/aiops/slos/:id/evaluations`. The SLI
   template catalog (`GET /api/v1/aiops/slos/templates`) backs the
   creation form (`POST /api/v1/aiops/slos`).
2. **Correlation cases** (`/aiops/correlation`, `CorrelationCasesView.vue`,
   684 lines) — lists correlated cases (`GET /api/v1/aiops/correlation/
   cases`) with status/confidence filters, renders the case detail
   (`GET .../cases/:id`), the resource graph (`GET .../cases/:id/graph`)
   and the candidate actions (`GET .../cases/:id/actions`), plus the
   timeline (`GET /api/v1/aiops/correlation/cases/timeline`).

Both surfaces are read-only for browsing plus the explicit, audited
SLO-evaluate action; no mutation beyond the SLO evaluate lifecycle is
invoked from the UI.

## Files Changed

### New Files

- (covered under M53's `frontend/src/api/aiops.test.ts` — SLO and
  correlation client tests added there)

### Modified Files

- `frontend/src/router/index.ts` — Route `/aiops/slo` →
  `SLODashboardView.vue`, `/aiops/correlation` →
  `CorrelationCasesView.vue` (both lazy-loaded).
- `frontend/src/views/SLODashboardView.vue` — SLO list/evaluate/history
  surface (605 lines).
- `frontend/src/views/CorrelationCasesView.vue` — case list/detail/graph/
  actions/timeline surface (684 lines).

## Routes

No backend routes were added by M54 (the endpoint surface is M41/M42);
the frontend consumes:

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/v1/aiops/slos` | auth required |
| POST | `/api/v1/aiops/slos` | auth required |
| GET | `/api/v1/aiops/slos/templates` | auth required |
| POST | `/api/v1/aiops/slos/:id/evaluate` | auth required |
| GET | `/api/v1/aiops/slos/:id/evaluations` | auth required |
| GET | `/api/v1/aiops/correlation/cases` | auth required |
| GET | `/api/v1/aiops/correlation/cases/timeline` | auth required |
| GET | `/api/v1/aiops/correlation/cases/:id` | auth required |
| GET | `/api/v1/aiops/correlation/cases/:id/graph` | auth required |
| GET | `/api/v1/aiops/correlation/cases/:id/actions` | auth required |

## Key Invariants Maintained

- **Single audit path** — SLO evaluation is the only mutating call and it
  reuses the existing audited route; the dashboard never patches or
  deletes SLO definitions.
- **Bounded evaluation history** — the evaluations list passes the
  backend `limit` bound through unchanged.
- **Case actions are candidates only** — the correlation UI lists
  candidate actions but does not execute them (execution belongs to the
  M55 automation console).

## Tests

- SLO client tests in `frontend/src/api/aiops.test.ts` (create via POST
  with JSON body, list with cluster/enabled filters, templates fetch,
  evaluate POST, evaluations bound).
- Correlation client tests (case list filters, case detail, investigation
  generate/list/get).

Total: part of the 17 new frontend tests delivered under M53's client
suite; M54 adds the SLO + correlation view surfaces.
