# ADR 0084 - M96 frontend scale budget and virtual-list evidence

- Date: 2026-08-10
- Status: Accepted
- Milestone: M96
- Related: ADR 0082, ADR 0083, M91 virtual scroll, M93-B2 login performance

## Context

M96 requires browser evidence for a 50,000-Pod workload without allowing the
full response to become a full DOM tree. The existing Pod view already had
fixed-row virtualization, but it did not preserve bottom spacer height, so a
large result set could not be scrolled past the initial window. Global row
entry animation also moved newly mounted virtual rows during scrolling.

## Decision

1. Keep the production Pod response as one typed array of object references;
   an empty name query reuses that array and an active query returns matching
   references without deep-cloning response objects.
2. Render the Pod table with a bounded visible window plus top and bottom
   spacer rows. The window exposes total rows, rendered rows, and start/end
   indexes as non-sensitive DOM metrics for regression probes.
3. Synchronize the virtual viewport after mount and on `ResizeObserver`
   changes. Virtual rows do not use generic enter transforms because geometry
   must remain stable while a window is replaced.
4. Generate a deterministic `m96-pods-v1` browser fixture and run production
   preview sampling for desktop and mobile, three visits per profile. Emit
   versioned JSON and Markdown with machine, browser, commit, fixture hashes,
   first render, interaction, scroll, long-task, DOM, heap, and window data.
5. Keep latency, long-task, DOM and heap budgets in report mode until two
   stable hosted CI cycles establish runner variance. Exact fixture count,
   bounded rendered rows, scroll reachability/stability, filter correctness
   and zero console errors remain hard invariants.

## Consequences

- The list remains usable at 50k rows while rendering only the measured
  window (16 rows in the current environment).
- The benchmark covers browser parsing and production UI behavior, but does
  not claim real kube-apiserver transport or device capacity.
- Changing fixture shape, row height, overscan or measured contracts requires
  a new fixture/baseline version and an archived comparison.
