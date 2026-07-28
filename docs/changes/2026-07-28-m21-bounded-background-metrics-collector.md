# M21 Phase 2: Bounded Background Metrics Collector

- Date: 2026-07-28
- Scope: enabled-cluster scheduling, canonical quantity conversion, failure
  isolation and expiry cleanup
- Decision: ADR 0035

## Delivered

1. `internal/metricshistory.Collector` runs an immediate collection and cleanup,
   then uses independent fixed tickers for sampling and bounded expiry work.
2. Enabled clusters are sorted by ID and capped. Four clusters run concurrently
   by default, each with an independent timeout and parallel Node/Pod reads.
3. Node and Pod bundles are sorted and admitted round-robin under the existing
   1,800-sample hard limit. Truncation remains explicit in source coverage.
4. Kubernetes `resource.Quantity` converts CPU to nanocores and memory to bytes;
   negative, malformed and overflowing inputs fail the source atomically.
5. API absence, timeout, request, quantity, payload and limit outcomes map to six
   stable failure codes. No raw upstream error is stored.
6. The server starts and stops the collector with the notification dispatcher,
   waits for both during graceful shutdown and keeps PostgreSQL open until they
   have exited.
7. `.env.example`, Compose and Kubernetes ConfigMap assets expose the same
   reviewed defaults: enabled, 168-hour retention, 60-second collection,
   10-second cluster timeout, hourly cleanup, 20 clusters and four-way
   concurrency.

## Verification

Package and full-backend tests cover exact decimal/binary/nano conversion,
overflow rejection, enabled-cluster filtering, stable sorting, Node/Pod fair
allocation, the 1,800-point cap, partial and mixed failures, request concurrency,
immediate cleanup and cancellation.

The 782.71-second repository-wide gate passed with Go 1.25.12: `go vet`, all
195 Go `Test*` entries, five backend build targets, frontend typecheck, 14
Vitest files / 59 tests, the production frontend build, both Docker image
builds, three healthy Compose services, Kustomize `16/5/22/3` and direct plus
proxied readiness checks. Evidence is
`.artifacts/verification/verify-20260728-223526.json`.

- Implementation revision: `62ed5a4d94622787e516a12502ef51e78b47479a`.
- Hosted CI: [run 30369559322](https://github.com/guiyi-labs/aiops-platform/actions/runs/30369559322)
  passed Backend, Frontend and Manifests plus the 6m11s Compose runtime job.
  The runtime job repeated credential, signed-audit, identity, PostgreSQL
  backup/restore and recovery-readiness drills, then passed random-configuration
  startup, service health, direct/proxied HTTP, sanitized upload and teardown
  with the collector enabled.

## Boundary

This phase does not add an HTTP history route, trend charts, sustained-window
alerts or multi-replica leader election. Those remain Phase 3 and later work.
