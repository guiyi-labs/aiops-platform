# M72: Topology Collection Parallelization (Performance)

- Date: 2026-08-02
- Status: Development Complete (perf; committed `da58511`; CI backend 7-gate green)
- Scope: Backend only — no API / frontend contract change

## Summary

Parallelizes topology collection for large clusters (P2-②). The operations
cockpit resource-topology view previously collected per-resource sub-graphs
sequentially; on clusters with thousands of objects this dominated the request
latency. `internal/topology` now fans out collection across a bounded worker
pool and reassembles the results.

Implementation notes:

- A configurable concurrency limit (`MaxConcurrency`) bounds the worker pool;
  the default is **4**, and a value of `0` means one goroutine per resource.
- Workers are coordinated with `sync.WaitGroup`; each worker pulls a resource
  off a shared channel, collects its sub-graph, and writes results to a
  mutex-protected accumulator.
- The public `CollectCluster` signature is unchanged, so the change is
  invisible to callers and to the HTTP layer.

## Files Changed

### Modified Files

- `backend/internal/topology/collector.go` — splits sequential collection into a
  per-resource unit suitable for concurrent dispatch; adds the channel /
  accumulator plumbing.
- `backend/internal/topology/service.go` — introduces the bounded worker pool
  (`MaxConcurrency`, default 4) and the `WaitGroup`-based fan-out in
  `CollectCluster`; preserves deterministic ordering of emitted topology nodes.
- `backend/internal/topology/service_test.go` — +195 lines covering the parallel
  path: correctness vs the sequential baseline, convergence under high resource
  counts, and idempotency of the merged result.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0.

## Notes

- The change is a pure performance / scalability improvement; no `internal/finding`
  codes, OpenAPI schemas, or frontend components are affected.
- Latency now scales with `max(cluster size / concurrency, slowest single
  resource)` instead of the sum of all resources.
