# M81: AIOps Closed-Loop Insight — finding-to-runbook correlation (W5)

- Date: 2026-08-09
- Status: Development Complete (local)
- ADR: docs/adr/0079-m81-insight-loop.md
- Fast gate: PASSED — backend go vet/build/test green; frontend typecheck /
  eslint / vitest (22 files, 124 tests) green.

## Summary

Delivers the closed loop polish-plan W5 asks for: a posture or optimization
finding now resolves to a deterministic runbook that connects the previously
isolated pages:

    finding → deterministic diagnosis (M18/M43) → inspection corroboration
    (M52) → cited-AI explanation entry (M55) → dry-run operation preview
    (M19/M44)

The runbook is a pure read-only mapping (ADR 0079; ADR 0004). Live diagnosis,
inspection, explanation and remediation execution still go through the
existing guarded APIs; the new layer performs zero cluster access and zero
state mutation.

## What Changed

### Backend (new)

- `backend/internal/insight/runbook.go` — catalog + `Resolve`: maps resource
  kinds (Pod / Deployment / Service / Node / Ingress / PVC / HPA) to diagnosis
  routes, analyzer domains to corroborating M52 rules, and kinds to dry-run
  operation candidates.
- `backend/internal/insight/runbook_test.go` — 5 table-driven tests covering
  pod/deployment/node resolution plus safe degradation for unknown kinds and
  domains.
- `backend/internal/httpserver/insight.go` — `GET /api/v1/aiops/insight`
  (auth-only, read-only) resolving the runbook from query parameters.
- `backend/internal/httpserver/insight_test.go` — 3 handler tests: 200 with a
  runbook body, 400 on missing required fields, 400 on invalid cluster_id.

### Backend (wired)

- `backend/internal/httpserver/router.go` — route registered on the /aiops
  group with audit action `aiops.insight.runbook.read`.
- `docs/api/openapi.yaml` — new path + `InsightRunbook` schema; the OpenAPI
  parity test stays green.

### Frontend

- `frontend/src/types/insight.ts` — TypeScript mirror of the runbook contract.
- `frontend/src/api/insight.ts` + `insight.test.ts` — client (2 vitest tests).
- `frontend/src/views/PostureView.vue` — each finding now has a "查看闭环"
  toggle; expanding reveals diagnosis / inspection / AI / operation candidates,
  each deep-linking to the corresponding console page.

### Docs

- `docs/adr/0079-m81-insight-loop.md` — new ADR (read-only closed loop).

## Verification (local)

- `go test ./internal/insight/... ./internal/httpserver/...` — green.
- `go vet ./internal/...`, `go build ./...` — green.
- `pnpm vue-tsc --noEmit`, `pnpm eslint` (changed files), `pnpm vitest`
  (22 files · 124 tests) — green.

## Notes

- Any authenticated role can call the endpoint because it is pure mapping; the
  write-bearing preview/execute operations stay behind the existing
  SystemOpsAdmin gates.
- Next workstream: W6 — bring aggregate posture findings into the M56 golden
  replay + quality report (polish-plan).