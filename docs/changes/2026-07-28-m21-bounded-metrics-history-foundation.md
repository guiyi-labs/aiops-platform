# M21 Phase 1: Bounded Metrics History Foundation

- Date: 2026-07-28
- Scope: durable collection evidence, canonical samples, sparse query semantics and bounded cleanup

## Delivered

1. ADR 0034 fixes retention, collection, query-window, point and cleanup caps
   before any background sampler or chart is introduced.
2. Migration 17 adds cluster-scoped collection runs and CPU/memory samples with
   coverage, expiry, exact-series uniqueness and cascading deletion.
3. `internal/metricshistory` validates collection outcomes, derives overall
   status, assigns canonical units, rejects duplicate or malformed series and
   persists a collection transactionally.
4. Exact-series queries are limited to one cluster, one Node or Pod container,
   one metric, 24 hours and 1,440 points. Coverage retains unavailable, partial,
   timeout, failure and missing counts without manufacturing zero-valued points.
5. Expiry cleanup is ordered and batch bounded so retention work cannot turn
   into an unbounded transaction.

## Verification Boundary

The phase has service-level tests for normalization, caps, sparse gaps,
duplicate rejection, failure-code admission and cleanup batching.

- Full local gate: `.artifacts/verification/verify-20260728-193305.json`, passed
  in 281.66 seconds with Docker Go 1.25 full-package vet/tests/build, 14 Vitest
  files and 59 tests, frontend production build, three healthy Compose
  services, Kustomize 16/5/22/3 and direct/proxied readiness checks.
- The development PostgreSQL instance applied and recorded migration 17. Manual
  catalog inspection confirmed both tables, the composite run/cluster foreign
  key, result/coverage/value/window checks, exact-series uniqueness and the
  three collection/query/expiry indexes.
- The rebuilt backend returned HTTP 200 from `/api/v1/health/ready` after the
  migration.

Hosted PostgreSQL/Compose migration verification is recorded after the
implementation commit is accepted by CI.

Background collection, Kubernetes quantity conversion, authenticated history
HTTP routes, trend views and sustained-window evaluation remain for the next
M21 phases.
