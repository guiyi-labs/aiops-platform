# M61: FinOps Right-sizing Advisor

- Date: 2026-08-01
- Status: Development Complete
- Commit: `9c81927`（与 M62/M63 合并提交）

## Summary

Adds the first read-only optimization analyzer: a FinOps right-sizing +
cost-waste advisor (`backend/internal/finops`). It turns the already-collected
M21 metrics (CPU/memory in nanocores/bytes) into suggested requests/limits
(p95 × headroom, rounded) and a monthly dollar waste estimate. Pure
`Recommend` function, configurable `CostRate`, in-memory `Repository`,
`QuantityFromResourceMap` for k8s quantity parsing.

## Files

- `backend/internal/finops/` — new package: recommend/waste/quantity logic + tests.
- `docs/adr/0073-*`（如适用）— read-only analyzer contract notes.

## Notes

Pure read-only; no cluster writes. Part of the M61–M66 optimization-center
arc completed by M64/M65/M66.