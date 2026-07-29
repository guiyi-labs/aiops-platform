# M21 Phase 3: Authenticated Exact-Series History API

- Date: 2026-07-29
- Scope: authenticated history reads, strict query parsing, OpenAPI and durable
  PostgreSQL acceptance
- Decision: ADR 0036

## Delivered

1. `GET /api/v1/clusters/{cluster_id}/metrics/history` exposes one exact Node
   or Pod CPU/memory series to every authenticated platform role.
2. Required RFC3339 `from`/`to`, a 24-hour maximum window and a 1,440-point cap
   preserve the Phase 1 bounded query contract at the HTTP edge.
3. Node requests forbid Namespace/container; Pod requests require both. No
   arbitrary label, GVK, aggregation, selector, raw SQL or PromQL is accepted.
4. Responses preserve canonical nanocore/byte units, stable chronological
   ordering, sparse points, explicit collection coverage, missing count,
   limits and truncation. Missing samples are never manufactured as zeroes.
5. Stable errors distinguish invalid input, missing cluster and internal query
   failure without exposing PostgreSQL details.
6. OpenAPI documents the exact request/response schema and route registration
   remains mechanically checked against Gin.
7. `scripts/e2e-metrics-history.ps1` seeds isolated cross-cluster and
   cross-series fixtures, verifies two real points across three collections
   with one gap, restarts the backend, repeats the result and proves cascaded
   cleanup. Hosted Compose CI runs the same drill.

## Verification

Handler tests cover exact parsing, Node/Pod shapes, RFC3339 windows, point caps,
authentication and stable error mapping. The real PostgreSQL E2E evidence is
`.artifacts/metrics-history-e2e/metrics-history-e2e-20260729-081759.json`:
cross-cluster isolation, exact-series isolation, chronological ordering,
one sparse gap, restart durability and cleanup all passed.

The 115.83-second repository-wide gate passed with evidence at
`.artifacts/verification/verify-20260729-082024.json`: Go vet, 199 Go `Test*`
entries, five backend build targets, 14 Vitest files / 59 tests, the frontend
production build, both Docker images, three healthy Compose services,
Kustomize `16/5/22/3` and direct/proxied readiness all succeeded.

The implementation revision and hosted CI run are appended after the
implementation push.

## Boundary

This phase does not add charts, downsampling, interpolation, aggregation,
sustained-window alerts or multi-tenant cluster ACLs. The next M21 slice builds
deterministic consumers over this exact-series contract.
