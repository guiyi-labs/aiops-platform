# M20 Phase 4 User-Owned Global Search Filters

- Status: Accepted
- Date: 2026-07-27
- Scope: private saved filters over the bounded global-search query

## Outcome

M20 Phase 4 adds authenticated CRUD at
`/api/v1/fleet/resources/search/filters`. Every record is owned by the current
actor and stores only a trimmed name, the Phase 3 name query, an optional exact
Namespace, a canonical subset of Pod/Deployment/Service/Ingress and
`schema_version=1`. It does not store search results, selectors, arbitrary GVK,
API paths, raw Kubernetes objects, credentials, sharing or schedules.

Migration `000015_saved_global_search_filters` adds the user foreign key,
validation checks, UTC timestamps, a case-insensitive `(user_id, lower(name))`
unique expression index and a user/order index. Creation takes a per-user
PostgreSQL advisory transaction lock before count and insert, making the
20-record cap deterministic under concurrent requests. List, update and delete
always include `user_id`; another user's ID has the same 404 behavior as a
missing row.

The service normalizes names, query and Namespace and orders kinds by the fixed
catalog. A persisted schema version or query shape that is no longer current is
returned with `compatible=false` and `SCHEMA_VERSION` or `QUERY_SHAPE`, without
breaking the list. It cannot be applied, but may be renamed, overwritten with
one complete current query or deleted. Patch rejects an empty body and requires
query, Namespace and kinds together for overwrite.

The Vue search page adds a dense saved-filter work area with count/limit,
inline creation and rename, apply, complete overwrite, delete and explicit
incompatible presentation. Applying restores the fixed form and URL and runs a
fresh bounded search. The mobile layout keeps actions inside each repeated row
and keeps the resource table in its existing local horizontal scroller.

## Security And Audit

All authenticated roles may manage only their own preferences because this
surface adds no target-cluster verb. Create/update/delete use strict JSON and
are audited as `global_search_filter.create`, `.update` and `.delete`. Audit
details contain only method, route template, cluster ID zero and filter
identity where available; they never capture the query body. Database rows
contain no Kubernetes response or credential material.

## Automated Verification

- `backend/internal/globalsearch` covers normalization, Unicode name bounds,
  canonical kinds, invalid and partial updates, owner forwarding, repository
  conflict propagation and stale schema/query characterization.
- `backend/internal/httpserver` covers authenticated actor projection, strict
  JSON, CRUD status codes, stable 400/404/409/500 errors, audit mappings and
  bidirectional OpenAPI route drift.
- Frontend typecheck passes. Fourteen Vitest files / 59 tests pass, including
  exact list/create/update/delete payloads alongside the bounded search query.
- Source currently contains 151 Go `Test*` entries. The final full gate artifact
  is recorded in the final verification section below.

## PostgreSQL And API Acceptance

The rebuilt Compose runtime applied migration 000015. PostgreSQL reported the
unique expression index on `(user_id, lower(name))`. API create returned kinds
in canonical `Pod,Service` order; a case-variant duplicate returned 409; rename
and complete overwrite succeeded; a second delete returned 404; cleanup left
zero filters.

The 22 concurrent creates for one empty user produced exactly 20 HTTP 201
responses and two HTTP 409 cap conflicts. The subsequent list reported exactly
20 rows and `limit=20`; all 20 acceptance rows were deleted. Audit inspection
showed create/update/delete success and failure outcomes with no query text in
details.

## Browser Acceptance

At desktop width, a real `nginx` query loaded the four fixed resource kinds
from retained cluster ID 39 while the expired peer remained localized. The
page created `浏览器验收筛选器`, renamed it, overwrote it with
`api / prod / Pod,Service`, and applied it. The form and URL both changed to
`/search?q=api&namespace=prod&kinds=Pod,Service`. The document measured
1265/1265 with no page-level horizontal overflow.

At the 390x844 breakpoint, the browser viewport/document width was 375/375 and
the saved-filter panel was 279px wide. After refreshing the one-hour observer
credential for cluster ID 39, two fixed-kind results rendered in a 760px table
contained by a 279px local scroller. Browser warning/error logs were empty.

The delete control opened its native confirmation dialog, but the browser
controller timed out while accepting that dialog. Therefore this record does
not claim a complete UI delete-confirmation pass. DELETE behavior already
passed handler and live API acceptance, and the browser-created row was removed
through that API, leaving zero saved filters.

## Final Verification

Full gate `.artifacts/verification/verify-20260727-222753.json` passed at
2026-07-27 22:27:53 +08:00 in 351.1 seconds. It passed Go format/vet/all
packages/server build with 151 `Test*` entries, frontend typecheck plus 14
Vitest files / 59 tests and production build, Compose rebuild with all three
services healthy, Kustomize 16/5/22/3, OpenAPI/delivery-contract checks and
backend/frontend/proxy runtime health.

## Boundaries And Follow-Up

This phase intentionally excludes team sharing, cross-user administration,
pinning/order, alerts, schedules and persisted results. A later schema/catalog
change must explicitly migrate or characterize stale filters before release.
The disposable two-cluster search follow-up was completed in M20 Phase 5; see
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`. Production
hardening remains next, and saved filters must not widen the Kubernetes query
or mutation surface.
