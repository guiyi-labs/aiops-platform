# ADR 0086 - M96 Gate B evidence aggregation

- Date: 2026-08-10
- Status: Accepted
- Milestone: M96 / Gate B
- Related: ADR 0082, ADR 0083, ADR 0084, ADR 0085

## Context

M96 produces evidence in separate backend and frontend CI jobs. The fixture
manifest, backend report, Pod browser samples and active CSS audit must be
checked as one versioned scale claim, while generated streams remain too large
to pass between jobs as ordinary source artifacts.

## Decision

1. Add `scripts/m96-gate-b.mjs` as the canonical report-mode aggregator. It
   discovers the uploaded evidence by versioned filenames and emits JSON and
   Markdown results.
2. Lock Gate B to the `m96-v1` fixture identity, exact 500 Node / 5,000
   workload / 50,000 Pod / 100,000 Event counts, config hash and dataset hash.
   Manifest, generation output and verification output must describe the same
   identity; when local generated streams are present, their decompressed
   bytes and SHA-256 are revalidated.
3. Require the backend report to retain its report mode, sample count,
   cancellation, timeout, backpressure and goroutine invariants. Require six
   frontend samples across three desktop and three mobile visits, all virtual
   list, filter, scroll and console-error invariants, and a report-mode budget
   baseline.
4. Require the active CSS audit to retain the four-layer import order,
   per-layer hashes and internally consistent totals. CSS and performance
   thresholds remain report-only until two stable hosted CI cycles establish
   runner variance.
5. CI downloads the backend and frontend artifacts into a clean aggregator
   job and passes the source commit explicitly. A missing artifact, identity
   mismatch or hard invariant failure blocks the Gate B job.

## Consequences

- Gate B evidence is reproducible from structured artifacts rather than a
  terminal transcript or screenshot.
- Hosted CI can validate the manifest and reports without transferring the
  generated fixture streams; the backend verifier remains responsible for
  reading those streams before upload.
- A local Gate B pass is not a hosted CI pass. M97 remains pending until the
  aggregator runs successfully on the committed source in hosted CI.
- The evidence describes fixture-backed behavior and browser rendering only;
  it does not claim production Kubernetes or device capacity.
