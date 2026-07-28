# M20 Phase 1 Bounded Multi-Cluster Health

- Status: Accepted
- Date: 2026-07-27
- Scope: bounded fleet fan-out contract, partial failure semantics and Dashboard health comparison

## Outcome

M20 Phase 1 adds `GET /api/v1/fleet/health`. The service reads only enabled
clusters, sorts them by platform ID and applies a hard 20-cluster response cap,
four concurrent cluster workers, a four-second per-cluster timeout and a
100-object sample per fixed resource kind. Node, Pod, Deployment and Event
reads are sequential inside each worker, so upstream request concurrency is
bounded by the worker count.

Every cluster item reports health counts, sampled/total coverage, completion,
Warning count, duration and stable failure scopes/codes. Truncation is
`partial`; all-query timeout is `timed_out`; query failure never returns raw
Kubernetes errors. The request itself fails only if the platform cluster
directory cannot be read. The endpoint reuses existing authenticated cluster
visibility and adds no Kubernetes verb, arbitrary GVK or write capability.

Dashboard now renders a compact fleet comparison above the selected-cluster
metrics. Rows show health status, resource ratios, Warning samples and latency,
and can switch the existing single-cluster workbench. The table has a stable
780px layout and scrolls only inside its own container on narrow viewports.

## Verification

- Fleet service tests cover disabled-cluster filtering, stable ordering,
  response truncation, two-worker concurrency, partial resource failure,
  per-cluster timeout, invalid limit, directory failure and incomplete samples.
- OpenAPI route drift and the production server wiring compile with the new
  authenticated route.
- Frontend typecheck passed; 13 Vitest files / 57 tests passed, including the
  authenticated bounded fleet request.
- Full release gate, rebuilt runtime endpoint and responsive browser evidence
  are recorded in the final verification section below.

## Evidence Boundary

The retained runtime contains one real kind cluster, so browser/API acceptance
validates the real Kubernetes data path and responsive presentation but does
not claim a two-cluster kind environment. Concurrency, timeout, ordering and
partial-failure behavior are deterministic service-test evidence. A disposable
two-cluster environment remains an explicit next-stage gate before claiming
real multi-cluster E2E.

## Final Verification

- Full gate: `.artifacts/verification/verify-20260727-190133.json`, 104.53
  seconds. Go vet, 133 Go `Test*` entries and server build passed; frontend
  typecheck, 13 Vitest files / 57 tests and production build passed.
- Compose rebuilt the backend and frontend and left PostgreSQL/backend/frontend
  healthy. Kustomize rendered 16/5/22/3 resources and runtime backend,
  frontend and proxy checks passed.
- The rebuilt Dashboard returned two enabled platform cluster records. The
  older record was isolated as `unavailable` with four sanitized failure
  scopes, while the current retained kind path returned 1/1 Ready Nodes,
  12/15 healthy Pods, 5/7 available Deployments and 10 Warning Events. Row
  selection switched the single-cluster cockpit and was restored afterward.
- At 1280x720 the document was 1265/1265 and the 943px fleet table required no
  local scroll. At 390x844 the document was 375/375 with no overflowing
  elements; the 780px table was contained by a 277px local scroller. Browser
  warning/error logs were empty.
- The two platform records point at the same retained real kind endpoint, so
  this evidence validates aggregation and failure isolation across records but
  is not claimed as two physically distinct Kubernetes clusters.
