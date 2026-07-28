# ADR 0032: Offline OIDC and MFA Readiness Gate

- Status: Accepted
- Date: 2026-07-28
- Milestone: M20 Phase 11

## Context

The platform currently authenticates local accounts and deliberately has no
production identity-provider registration. Choosing an issuer, client, claim
mapping, MFA ownership, account-linking behavior, session limits or emergency
access without the organization that owns those controls would create a false
security claim. Fetching an operator-supplied URL from the running application
would also add an unnecessary discovery/SSRF surface before login integration
has been approved.

## Decision

Add `/app/identity-readiness` as an offline, provider-neutral admission gate.
It accepts three explicit JSON files: an approved policy, a captured OIDC
discovery document and its captured JWKS. Inputs are limited to 1 MiB, decoded
strictly and rejected on unknown fields. The policy schema contains a public
client identifier but has no client-secret field; secrets, tokens and private
keys do not belong in any input or report.

The gate fails closed unless all of the following are true:

- a canonical HTTPS issuer exactly matches discovery;
- authorization, token, JWKS, redirect and logout URLs use HTTPS;
- Authorization Code and mandatory PKCE S256 are supported;
- `openid` and `profile` plus every approved scope are advertised;
- ID tokens use an approved asymmetric algorithm and the JWKS has a
  structurally valid verification key with a unique `kid`;
- immutable `sub`, username, display-name and group claim mappings are explicit;
- MFA is identity-provider enforced and produces an approved `acr` or `amr`
  value;
- accounts are administrator-prelinked by immutable subject and never
  automatically linked by email;
- local session revocation, bounded reauthentication, provider logout,
  one-or-two-account break-glass handling and accountable owners are explicit.

The command performs no network request and adds no HTTP endpoint, database
table or authentication behavior. CI evaluates synthetic snapshots with
networking disabled and proves that issuer/PKCE and MFA/linking downgrades are
rejected. Only boolean/count evidence is retained.

## Consequences

- Provider evaluation becomes reproducible and reviewable before code is
  coupled to a vendor.
- A passing report means the supplied contract and snapshots meet the current
  admission rules. It does not prove provider availability, token validation,
  browser login, logout propagation, identity disable latency or MFA operation.
- Discovery and JWKS snapshots can become stale. Production integration must
  define who captures and reviews them and must implement bounded runtime key
  refresh, rollover and outage behavior.
- Local bootstrap authentication remains unchanged. Enabling SSO requires a
  separate approved phase with provider fixtures, token verification, subject
  persistence, migration/account linking, audit and end-to-end browser tests.
