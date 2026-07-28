# M20 Phase 7: Dependency Governance And Node 24 CI

- Accepted: 2026-07-28
- Main revision: `acbdccaecaafc6eac96987367c5e118071508fb1`
- Hosted CI: [run 30328283896](https://github.com/guiyi-labs/aiops-platform/actions/runs/30328283896)

## Scope

This phase reviewed the first Dependabot wave after the hosted CI baseline.
Actions, Go modules and frontend packages were not merged as an unchecked bulk
update:

- PR #1 updated signed Action SHAs and the workflow contract test.
- PR #2 updated the reviewed same-major Go module set.
- PR #5 updated `golang.org/x/crypto` and its aligned `x/*` modules.
- PR #6 updated Vue 3.5 and `vue-tsc` patch releases.
- PR #3 was closed because it combined TypeScript, Vite, Vitest, Pinia, Vue
  Router, lucide and Node type major migrations and violated the lockfile
  minimum-release-age policy.
- PR #4 was superseded after the signed `pnpm/action-setup` v6.0.9 commit was
  applied directly with its contract test.

`.github/dependabot.yml` now groups only `minor` and `patch` updates for
GitHub Actions, Go modules and npm packages. Major upgrades remain separate
PRs with their own compatibility and browser verification scope.

## CI Currency

`pnpm/action-setup` moved from the Node 20-based v4 SHA to signed v6.0.9
(`0ebf47130e4866e96fce0953f49152a61190b271`), whose `action.yml` uses
`node24`. The final combined CI passed Backend, Frontend, Manifests and Compose
runtime without the prior Node 20 deprecation annotation.

## Governance Constraint

The GitHub branch-protection API returned HTTP 403 because this private
repository requires GitHub Pro or a public repository for that feature. The
repository remains private; branch protection is deferred rather than weakened
or replaced by an unreviewed workaround.

## Verification

- Local Go 1.25 container: `go test -p=1 -count=1 ./internal/deployment`
- Hosted CI: Backend, Frontend, Manifests and Compose runtime all passed
- Final hosted revision: `acbdccaecaafc6eac96987367c5e118071508fb1`
