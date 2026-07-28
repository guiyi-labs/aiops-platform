# M20 Phase 6 Versioned CI And Release Pipeline

- Status: Accepted
- Date: 2026-07-28
- Scope: bounded continuous integration, package-only rehearsal, tagged release and scheduled disposable real-kind validation

## Outcome

M20 Phase 6 adds ADR 0028 and three separate GitHub Actions workflows. Normal
pull requests run backend, frontend, manifest and real Compose runtime checks
without a repository secret. Release tags reuse that complete gate before
building checksummed versioned archives. Physical kind suites stay weekly or
manual on an explicitly labeled self-hosted Windows runner and retain only
sanitized JSON evidence.

The workflows add these safeguards:

1. GitHub-hosted jobs use Ubuntu 24.04 and exact action commit SHAs;
2. default permissions are `contents: read`, with `contents: write` scoped only
   to the release packaging job;
3. no workflow uses `pull_request_target`, a PR secret, `docker push`, generic
   Kubernetes access or retained-demo mode;
4. the Compose runtime uses random ephemeral credentials, waits for all three
   health checks, verifies backend/frontend/proxy HTTP and always removes its
   containers, volume and `.env`;
5. manual release dispatch validates the semantic version and produces a
   package only; `gh release create --verify-tag` runs only for a tag;
6. release assets include two versioned image archives, exact-revision source,
   OpenAPI, dependency licenses, metadata and `SHA256SUMS`;
7. real-kind concurrency does not cancel an active cleanup path and runs only
   the disposable diagnosis, fleet and fixed-kind search scripts;
8. Dependabot groups weekly Actions, Go and frontend dependency updates.

`backend/internal/deployment/ci_workflows_test.go` parses all four YAML
configuration surfaces and enforces required/prohibited markers in the normal
Go suite. The accepted local review also ran actionlint 1.7.7 with zero
warnings or errors.

## Verification

The post-archive full gate passed at 2026-07-28 10:07:52 +08:00 in 180.85
seconds. Evidence is
`.artifacts/verification/verify-20260728-100752.json`: backend format/vet, 152
Go `Test*` entries, server build, frontend typecheck, 14 Vitest files / 59
tests, production build, three healthy Compose services, Kustomize
16/5/22/3 and backend/frontend/proxy HTTP checks all passed. The phase does
not claim a hosted GitHub run because the repository still lacks its
human-approved initial commit and remote baseline.

## Boundaries And Follow-Up

No initial commit, remote branch protection, release tag, GitHub Release,
registry push, signing key or production runner was created. Those are external
state changes and remain human-controlled. The next production-hardening slice
should evaluate OIDC/MFA and application credential-key rotation before adding
registry identity, artifact signing or provenance attestation.
