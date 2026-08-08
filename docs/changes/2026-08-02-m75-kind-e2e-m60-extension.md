# M75: Extend kind E2E Suite to M60 Provider Registry

- Date: 2026-08-02
- Status: Development Complete (test infra; committed `6482ede`)
- Scope: Test / CI only — no product code change

## Summary

Extends the disposable kind E2E suite from M46–M58 to M46–M60 and renames the
driver to `scripts/e2e-m46-m60-kind.ps1`. Adds M60 provider-registry fixtures
so the multi-cluster / workspace / provider-registry surfaces have a
real-cluster gate alongside the earlier milestones.

## Files

- `scripts/e2e-m46-m60-kind.ps1` — renamed + extended driver.
- `.github/workflows/real-kind-e2e.yml` — updated to invoke the renamed suite.

## Notes

Companion to M73 (initial suite) and M74 (AIOps surface API tests).