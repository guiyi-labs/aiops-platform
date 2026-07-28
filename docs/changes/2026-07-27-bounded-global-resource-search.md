# M20 Phase 3 Bounded Global Resource Search

- Status: Accepted
- Date: 2026-07-27
- Scope: fixed-kind cross-cluster search, explicit coverage and resource deep links

## Outcome

M20 Phase 3 adds authenticated `GET /api/v1/fleet/resources/search`. The query
accepts only a required 2..64 character name substring, one optional valid
Namespace and a unique subset of Pod, Deployment, Service and Ingress. It does
not accept arbitrary GVK, API paths, label/field selectors, raw objects or
client-owned Kubernetes queries.

The service sorts enabled clusters by platform ID, selects at most 20, runs no
more than four cluster workers and gives each worker one four-second budget.
Kinds are read sequentially and contribute at most 100 candidates each. The
global response returns at most 100 normalized items sorted by cluster ID,
fixed kind order, Namespace and name. A fixed-kind failure exposes only
`TIMEOUT` or `QUERY_FAILED` and does not suppress successful peers.

Result coverage and cluster coverage are separate. `total`/`remaining` describe
known matches in searched successful reads, while `clusters_total`,
`clusters_searched` and `clusters_remaining` show enabled-cluster admission.
Truncation, omitted clusters or any failure makes `complete=false`.

The Vue console adds `/search` for every authenticated role. It provides name,
Namespace and four-kind controls, cancels replaced requests, restores the fixed
query from the URL, displays coverage/failure summaries and keeps the result
table inside a local horizontal scroller. A result opens the existing
`/workloads` detail drawer with exact cluster, kind, Namespace and name.

## Verification

- `backend/internal/globalsearch` tests cover validation, stable selection and
  ordering, bounded concurrency, result truncation, omitted-cluster coverage,
  localized query failure and timeout.
- HTTP tests cover bounded parsing, invalid inputs and sanitized directory
  failure. OpenAPI route drift includes the authenticated endpoint and a typed
  response schema.
- Frontend typecheck and 14 Vitest files / 58 tests pass, including fixed query
  serialization. The production Vite build passes.
- Full gate `.artifacts/verification/verify-20260727-210308.json` passed in
  168.62 seconds: Go vet/all packages/server build, frontend typecheck plus 14
  Vitest files / 58 tests and production build, three healthy Compose services,
  Kustomize 16/5/22/3 and backend/frontend/proxy runtime checks all passed.

## Runtime And Browser Acceptance

The rebuilt runtime rejected anonymous search with 401. One retained real kind
record received a new one-hour observer credential and returned four `nginx`
matches: Pod, Deployment, Service and Ingress. The other retained record kept
its expired credential and produced four localized `QUERY_FAILED` entries,
proving that successful matches remain usable during a peer failure. No raw
upstream error appeared in the UI.

At 1280x720 the document was 1265/1265 and the 945px result table fit without
local scrolling. Selecting the Pod result navigated to
`/workloads?cluster=39&kind=Pod&namespace=aiops-demo&name=...` and opened the
exact Running Pod detail drawer. At 390x844 the document was 375/375; the 760px
table was contained by its 279px local scroller. A URL containing duplicate
`Pod` kinds restored only Pod/Service/Ingress once and completed normally.
Browser warning/error logs were empty. The generated kubeconfig file was
deleted after credential rotation; only the encrypted, expiring credential
remains in the local development database.

## Boundary And Next Slice

All authenticated roles may search because the route only reuses observer
reads and adds no Kubernetes verb. Responses contain navigation metadata and a
short health summary, never credentials, API Server addresses, upstream error
text or raw Kubernetes objects.

Saved filters are not part of this slice. A later phase must define per-user
ownership, maximum count, uniqueness, rename/delete, stale-kind migration and
audit behavior while persisting only this reviewed query shape. Search does not
authorize cross-cluster mutation, arbitrary YAML editing or bulk actions.
