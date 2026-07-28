# M20 Phase 11: OIDC and MFA Readiness Gate

- Date: 2026-07-28
- Status: Accepted
- Accepted revision: `216eb81e92b9ccc009c62c9d03ec58f9116bfd33`
- Hosted CI: [run 30345051371](https://github.com/guiyi-labs/aiops-platform/actions/runs/30345051371)
- Scope: provider-neutral identity policy, offline metadata validation and downgrade rejection

## Delivered

- Added ADR 0032 and `docs/security/identity-readiness.md`.
- Added a decision template that records public policy metadata but deliberately
  has no field for a client secret, token or private key.
- Added the offline `/app/identity-readiness` command. It strictly reads bounded
  policy, OIDC discovery and JWKS snapshots and emits a machine-readable report.
- Added 14 fail-closed checks for ownership, canonical HTTPS issuer/endpoints,
  redirect URIs, Authorization Code + PKCE S256, scopes, asymmetric signing
  algorithms, structurally valid unique signing keys, claim mapping, MFA,
  immutable-subject linking, sessions/logout and break-glass controls.
- Added unit downgrade coverage and a network-disabled production-image drill.
  The drill accepts complete synthetic inputs, rejects issuer/PKCE and
  MFA/email-linking downgrades, removes temporary inputs and image, and retains
  only sanitized booleans/counts.
- Added the command, drill and sanitized evidence directory to the regular CI
  and delivery contract.

## Verification

The network-disabled production-image drill passed all 14 admission checks and
rejected issuer mismatch, missing PKCE S256, disabled MFA and automatic email
linking. Both cleanup assertions passed. Sanitized evidence is
`.artifacts/identity-readiness/identity-readiness-20260728-165405.json`.

Actionlint 1.7.7 returned zero findings. The 300.97-second final local gate used
the Go 1.25 Docker toolchain and passed vet, all packages and 171 Go `Test*`
entries, all four backend build targets, 14 Vitest files / 59 tests, the
frontend production build, three healthy Compose services, Kustomize
16/5/22/3 and direct/proxied HTTP readiness. Evidence is
`.artifacts/verification/verify-20260728-165939.json`.

Hosted CI run `30345051371` passed all four jobs for revision `216eb81`. The
Ubuntu Compose job accepted the network-disabled identity contract and all
three existing isolated database drills, then passed random-production-config
Compose health, direct/proxied HTTP checks, sanitized artifact upload and
unconditional teardown.

## Boundary

This phase does not register a provider, store a provider/client secret, add a
login endpoint, validate a live token or claim production MFA/SSO. The example
contains unresolved markers because organization-specific identity decisions
have not been authorized. Runtime integration remains a separate phase after
identity, security and application owners approve a real policy.

## Next Step

Complete the organization-specific policy and provider registration, then
implement OIDC login against an isolated provider fixture with state/nonce/PKCE,
strict token and MFA validation, immutable subject persistence, explicit
prelinking, session revocation, logout and audit tests. In parallel, production
backup retention/RPO/RTO still requires infrastructure-owner approval before
PITR or HA claims can be made.
