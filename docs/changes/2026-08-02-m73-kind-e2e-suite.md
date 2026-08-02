# M73: Disposable kind E2E Suite for M46–M58 Milestones

- Date: 2026-08-02
- Status: Development Complete (test infra; committed `dd82e54`; CI `real-kind-e2e` green)
- Scope: Test / CI only — no product code change

## Summary

Adds a disposable kind-based end-to-end test suite covering the M46–M58
milestones (P2-③). Each run spins up a throwaway `kind` cluster, applies the
fixtures those milestones exercise, and verifies the read-only surfaces behave
as specified — then tears the cluster down. This gives the large-cluster /
multi-tenant / workspace features a real-cluster gate instead of unit-only
coverage.

Implementation notes:

- `scripts/e2e-m46-m58-kind.ps1` (159 lines) orchestrates cluster creation,
  fixture apply, the verification steps per milestone, and cleanup. It is
  PowerShell so it runs unmodified on the Windows CI runner and locally.
- `.github/workflows/real-kind-e2e.yml` is extended to invoke the suite as part
  of the `real-kind-e2e` workflow.

## Files Changed

### New / Modified Files

- `scripts/e2e-m46-m58-kind.ps1` — new disposable kind E2E driver for the M46–M58
  surface set.
- `.github/workflows/real-kind-e2e.yml` — `+3` lines wiring the new script into
  the workflow.

## Follow-up

This suite was later extended from M46–M58 to M46–M60 and renamed
`scripts/e2e-m46-m60-kind.ps1` in M75 (`6482ede`), which also added the M60
provider-registry fixtures.

## Verification

`real-kind-e2e` workflow green on push.

## Notes

- kind clusters are created with `--wait 60s` and deleted in a `finally` block so
  a failed assertion never leaks a cluster on the runner.
- The suite is intentionally read-only against any cluster it is pointed at; it
  only asserts on observed state, never mutates tenant workloads.
