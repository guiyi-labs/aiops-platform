# M22: Daily Troubleshooting And Governance Workbench

- Status: Accepted
- Date: 2026-07-30 baseline audit
- Decision: ADR 0039

## Outcome

M22 closes the daily read-only troubleshooting gap without introducing a
generic Kubernetes proxy. The resource gateway and UI now provide fixed
PersistentVolume, PodDisruptionBudget, NetworkPolicy and ServiceAccount
list/detail contracts; bounded multi-container Pod logs; and redacted manifest
inspection for an explicit kind allowlist.

Pod log requests require an exact container, bounded time/tail/byte limits and
explicit current-versus-previous selection. Multi-container aggregation keeps
per-container failures local and discloses truncation. The frontend offers
container tabs, search and download without storing log payloads.

Manifest inspection strips or replaces sensitive values server-side and fails
closed for kinds outside the reviewed allowlist. Secret values are never
returned. The resource detail workbench separates overview, spec, status,
events, logs, manifest, rollout and task surfaces while reusing authenticated,
cluster-scoped clients.

## Baseline Review Corrections

- PodDisruptionBudget uses the correct policy API route and typed projection.
- Container enumeration includes init containers without mixing them into the
  running-container state contract.
- Log window parameters remain mutually exclusive and are covered by tests.
- The manifest viewer production template parses cleanly and passed the Vite
  production build.

## Verification

The 2026-07-30 fast and full gates passed all backend packages, 17 Vitest
files/73 tests, the production frontend build, Compose health and delivery
contracts. The full evidence is
`.artifacts/verification/verify-20260730-080851.json`.

## Boundary

M22 adds no Pod exec, file transfer, arbitrary GVK access, YAML editing,
generic patching, Secret value display or unbounded log streaming.
