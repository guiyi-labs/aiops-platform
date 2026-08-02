# M55: AI Investigator + Security Automation Console

- Date: 2026-08-02
- Status: Development Complete (frontend increment; surfaces M43-M44 backend)
- ADR: n/a (consumes M43 cited-and-evaluated investigator and M44
  policy-constrained automation contracts)
- Fast gate: passed (verify-fast.ps1 -Scope All; backend=True frontend=True manifests=True)

## Summary

Delivered the M55 frontend increment exposing the M43 AI investigator and
the M44 policy-constrained automation engine as operator consoles:

1. **AI investigator** (`/aiops/investigator`, `AIInvestigatorView.vue`,
   678 lines) — picks a correlated case (`GET /api/v1/aiops/correlation/
   cases/:id`), lists prior investigations (`GET /api/v1/aiops/
   investigator/cases/:case_id/investigations`) and generates a new
   cited-and-evaluated investigation (`POST .../investigations` with
   optional `runbook_id`/`provider`), then streams the result
   (`GET /api/v1/aiops/investigator/investigations/:id`). The runbook
   catalog (`GET /api/v1/aiops/investigator/runbooks`) drives the
   runbook selector.
2. **Security automation console** (`/aiops/automation`,
   `AutomationConsoleView.vue`, 990 lines) — lists automation plans
   (`GET /api/v1/aiops/automation/plans`) with status filter, walks the
   full controlled-operation lifecycle:
   `POST /plans` (create) → `POST /plans/:id/preview` →
   `POST /plans/:id/approve` → `POST /plans/:id/execute`
   (`confirmation_token` + idempotency) → `POST /plans/:id/verify` and
   `GET /plans/:id/verification`. The runbook catalog
   (`GET /api/v1/aiops/automation/runbooks`) backs plan creation.

The automation console implements the M19 controlled-operation
two-phase pattern end-to-end: every execution requires an explicit
confirmation token and an idempotency key, and each plan carries
post-action verification. No background execution is triggered from the
UI without the operator's approve + execute + token flow.

## Files Changed

### New Files

- (covered under M53's `frontend/src/api/aiops.test.ts` — investigator
  and automation client tests added there)

### Modified Files

- `frontend/src/router/index.ts` — Route `/aiops/investigator` →
  `AIInvestigatorView.vue`, `/aiops/automation` →
  `AutomationConsoleView.vue` (both lazy-loaded).
- `frontend/src/views/AIInvestigatorView.vue` — investigation console
  (678 lines).
- `frontend/src/views/AutomationConsoleView.vue` — automation plan
  lifecycle console with approval + verification (990 lines).

## Routes

No backend routes were added by M55 (the endpoint surface is M43/M44);
the frontend consumes:

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/v1/aiops/investigator/runbooks` | auth required |
| GET | `/api/v1/aiops/investigator/cases/:case_id/investigations` | auth required |
| POST | `/api/v1/aiops/investigator/cases/:case_id/investigations` | auth required |
| GET | `/api/v1/aiops/investigator/investigations/:id` | auth required |
| GET | `/api/v1/aiops/automation/runbooks` | auth required |
| GET | `/api/v1/aiops/automation/plans` | auth required |
| POST | `/api/v1/aiops/automation/plans` | auth required |
| POST | `/api/v1/aiops/automation/plans/:id/preview` | auth required |
| POST | `/api/v1/aiops/automation/plans/:id/approve` | auth required |
| POST | `/api/v1/aiops/automation/plans/:id/execute` | auth required |
| POST | `/api/v1/aiops/automation/plans/:id/verify` | auth required |
| GET | `/api/v1/aiops/automation/plans/:id/verification` | auth required |

## Key Invariants Maintained

- **Controlled-operation end-to-end** — execute requires the
  `confirmation_token` returned by preview/approve plus an idempotency
  key header; the UI threads both through the M19 two-phase flow.
- **Post-action verification** — every executed plan surfaces its
  verification result (`verify` + `verification`) rather than
  fire-and-forget.
- **Read-only until operator commits** — listing/runbook/preview are
  non-mutating; mutation is gated behind the approve/execute token flow.
- **No new authorization path** — all routes reuse existing auth
  middleware; no client-side bypass of backend roles.

## Tests

- Investigator client tests in `frontend/src/api/aiops.test.ts`
  (generate with optional runbook query, list for a case, get single
  investigation).
- Automation client tests (list with status filter; full lifecycle
  create → preview → approve → execute → verify with body/idempotency
  assertions).

Total: part of the 17 new frontend tests delivered under M53's client
suite; M55 adds the investigator + automation console surfaces.
