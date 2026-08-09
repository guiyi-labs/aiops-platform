# W10: OpenAPI Type Sync + Contract Repair (M86 complete)

- Date: 2026-08-09
- Status: Complete — this closes the M86 follow-up opened by
  `2026-08-09-w10-error-code-audit.md`.

## Summary

Completes the W10 contract-governance slice: the frontend now consumes API
shapes generated from `docs/api/openapi.yaml`, and CI enforces that the
generated artifact stays in sync with the spec. Getting there required fixing
latent spec defects that had never been parseable by openapi-typescript:

1. **Duplicated mapping keys** — `VeleroBackupList` (two near-identical
   definitions, one with obsolete `page/limit`) and `EvidenceRef` (duplicate
   enum variant) made the document unparseable. Both were merged:
   - `VeleroBackupList` now matches the live handlers
     (`apiquery.ListResponse` → `{items, total, remaining}` and items are the
     detailed `VeleroBackup` projection used by all backup surfaces).
   - `EvidenceRef` keeps the generic non-enum variant (matches
     `internal/{correlation,topology,signal,aiinvestigator}` `json:"kind"`
     which accepts arbitrary kind strings today).
2. **Missing `components/parameters`** — `QueryPage` / `QueryPageSize` were
   referenced by backup/restore/copy/gitops list endpoints but never defined;
   added the two parameter aliases.
3. **Missing federation schemas (ADR 0063)** — the 10 federation operations
   (overview/events/resources-summary/register/deregister/promote/demote/
   heartbeat/status/cluster-events) referenced undefined schemas. Added the
   `FederationOverview`, `FederationEvent[List]`, `FederationResourceSummary`,
   `FederationCluster`, request-body and `ClusterSummary/ClusterCondition`
   shapes by reading `internal/federation` + `internal/cluster` models, so
   `openapi-typescript` can resolve every `$ref` in the document.

## Generated types & consumption

- `frontend/package.json` — `pnpm typegen` runs
  `openapi-typescript ../docs/api/openapi.yaml -o src/api/openapi.d.ts`.
- `src/api/openapi.d.ts` — committed generated artifact (429 KB; regenerate
  deterministically, no hand edits).
- `src/api/openapi.ts` — facade re-exporting `paths` / `components` /
  `operations` plus `OperationResponse<Op>` helper narrowing the JSON payload
  of a 200 response.
- `src/api/insight.ts` — refactored to consume the generated
  `getInsightRunbook` operation query + `InsightRunbookContract` response
  types (runtime behavior unchanged; existing insights tests pass).
- `.github/workflows/ci.yml` — new frontend job step
  “Contract typegen sync (W10)”: `pnpm typegen` then
  `git diff --exit-code -- src/api/openapi.d.ts`, so spec drift breaks CI.

## Verification

```
cd frontend
npx openapi-typescript ../docs/api/openapi.yaml -o src/api/openapi.d.ts   → success (all $ref resolve)
npx vue-tsc --noEmit                                                        → clean
npx eslint src/api/insight.ts src/api/openapi.ts                            → clean
npx vitest run src/api/insight.test.ts                                      → 2 passed
# idempotency: re-running typegen produces zero diff (W10/sync gate)
```

## Non-goals

- OpenAPI schema depth diff (every response body field match with live
  handlers) remains out of scope for this slice.
- Letting the whole frontend type surface derive from `operations` (full
  migration of hand-written `src/types/*`) stays as future work.