# M96 Gate B evidence aggregation

- Date: 2026-08-10
- Status: Complete
- Scope: Aggregate and validate M96 scale, browser and CSS evidence.

## Context

M96-A through M96-D produced separate versioned artifacts. Without a single
check, a correct fixture could be paired with a report from another dataset or
with incomplete frontend evidence. Gate B must also preserve the report-mode
policy: observed performance thresholds are evidence, not fail-closed claims.

## What Changed

- Added `scripts/m96-gate-b.mjs`, which validates fixture identity, counts,
  hashes, backend invariants, frontend profile/repeat counts and hard browser
  invariants, plus active CSS layer order, hashes and totals.
- The reader accepts UTF-8 and UTF-16 JSON so local PowerShell evidence and
  hosted CI evidence use the same verification path.
- Local runs revalidate all five generated gzip streams when present. Hosted
  CI consumes the verified manifest and structured reports without uploading
  the large generated streams between jobs.
- Added an `M96 Gate B` CI job that downloads the backend/frontend evidence,
  passes the source commit to the aggregator and uploads the combined report.
- The frontend CI job now runs the existing Desktop/Mobile Playwright suite
  before the M96 performance probes.

## Verification

- `node scripts/m96-gate-b.mjs --root . --output .artifacts/m96-gate-b/m96-gate-b.json`: PASS.
- Local Gate B checks: exact `m96-v1` identity, five stream revalidations,
  backend `30` samples / `7` invariants, frontend `6` samples / `0` failures /
  `0` invariant failures, and four active CSS layers.
- CI-shaped evidence directory without generated streams: PASS; stream
  revalidation is explicitly reported as skipped.
- `pnpm test:e2e`: 56 passed.
- `pnpm test -- --run`: 137 passed.
- `pnpm lint`, `pnpm typecheck`, `pnpm build` and `pnpm style:audit`: passed.
- Hosted CI verification is queued by the new workflow job and remains
  pending until the next reachable remote run.
- `node --check scripts/m96-gate-b.mjs` and `git diff --check`: passed.

## Risks / Notes

- Latency, heap, long-task and CSS drift remain report-mode observations.
- M97 must wait for a successful hosted Gate B run; M89/M90 remain external
  authorization tracks, so the release state stays RC.
