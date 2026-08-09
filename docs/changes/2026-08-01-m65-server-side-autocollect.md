# M65: Server-Side Auto-Collection Layer (P1-①)

- Date: 2026-08-01
- Status: Development Complete
- Commit: `d3b8029`（与 `632b302` 重复提交）

## Summary

Adds `backend/internal/optimization/collector.go` — a `Collector` that turns
live cluster data into the exact observation bundles the M61–M63 analyzers
consume. Read-only (ADR 0004): only reads and maps, never mutates cluster
state. `ClusterLister` is the only cluster access needed (`List(ctx, clusterID,
path)`), and a fake can be supplied in tests without a real cluster.

The M64 analyze endpoints now auto-collect when the request body carries no
bundle and a collector is configured.

## Files

- `backend/internal/optimization/collector.go` — new collector.
- `internal/optimization/` — collector tests with fake ClusterLister.

## Notes

Completes the M61–M64 arc; M66 adds the console UI.