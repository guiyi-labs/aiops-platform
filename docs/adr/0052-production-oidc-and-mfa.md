# ADR 0052: Production OIDC and MFA

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M36 (Production OIDC and MFA), ADR 0014 (live authorization and safe user lifecycle), ADR 0019 (cookie-bound session device management), ADR 0032 (offline OIDC and MFA readiness gate)

## Context

ADR 0032 shipped a provider-neutral, offline admission gate that evaluates a
proposed OIDC contract (issuer, discovery, JWKS, claim mapping, MFA ownership,
account-linking behaviour, session bounds and break-glass policy) without
enabling login or contacting the provider. A passing report is a prerequisite
for SSO; it is not SSO. The platform still authenticates local accounts only.

`docs/kubesphere-optimization-plan.md` (M36) requires one organization-approved
OIDC provider behind a small compiled interface, with:

- Authorization Code + PKCE S256, state, nonce, issuer, audience and HTTPS
  discovery validation;
- bounded JWKS cache and rotation;
- immutable subject mapping and a fixed claim/group-to-role allowlist;
- required MFA evidence for privileged roles;
- current-user status, role/grant version and session revocation checked on
  every protected request;
- a local break-glass account retained with high-priority audit/notification;
- provider secret supplied by external Secret/configuration, never
  browser-managed.

M36 is split into five phases (A-E). This ADR covers phase A (configuration,
the immutable subject-prelinking table and the contract) and authorizes the
later phases to implement discovery/JWKS caching, the Authorization Code +
PKCE flow, session/logout management and a synthetic IdP end-to-end gate.

## Decision

### 1. OIDC is an optional, externally gated capability

OIDC is disabled by default (`OIDC_ENABLED=false`). When disabled, the
platform authenticates local accounts exactly as before; no OIDC code path is
registered and the `external_identities` table is unused. Enabling OIDC is an
operational decision that requires the offline readiness gate (ADR 0032) to
have passed for the same issuer and contract.

### 2. Configuration reuses the readiness policy contract and fails closed

The runtime `OIDCConfig` (in `internal/config`) mirrors the offline readiness
policy. When `OIDC_ENABLED=true` the configuration must satisfy the same
fail-closed rules:

- canonical HTTPS issuer with no trailing slash, exactly matching discovery;
- non-empty client identifier and an absolute fragment-free HTTPS redirect URI;
- claim mapping with `subject = "sub"` and explicit username, display-name and
  groups claim names;
- required scopes include `openid` and `profile`;
- a non-empty set of allowed signing algorithms drawn from `RS256`, `PS256`,
  `ES256`, `EdDSA` (symmetric algorithms are rejected);
- at least one group-to-role mapping, where every role code is one of the four
  platform roles (`system_admin`, `operations_admin`, `security_auditor`,
  `viewer`);
- MFA is required, identity-provider enforced, with an `acr` or `amr`
  evidence claim and at least one accepted value;
- session max age in 5m..24h, reauthentication in 1m..max age, and
  `revoke_local_sessions_on_identity_disable = true`;
- break-glass enabled with one or two accounts;
- bounded JWKS cache TTL (1m..24h) and refresh timeout (1s..1m).

In production a confidential client is required: `OIDC_CLIENT_SECRET` must be
supplied. In development a public PKCE-only client may omit it. PKCE S256 is
mandatory in every environment.

The provider client secret is loaded exclusively from environment
configuration. It never enters the browser request, the audit trail, logs, the
policy file or any report.

### 3. Immutable subject prelinking; no automatic email linking

A new `external_identities` table (migration 000026) binds an OIDC provider
subject to a local user. The `(issuer, subject)` pair is unique: one provider
subject maps to at most one local user. Rows are created explicitly by an
administrator before the bound user can authenticate through OIDC. An incoming
ID token never creates a row, and an email match never creates a link. This is
the `admin_prelinked_subject` mode from the readiness policy.

Deleting a row (or disabling the local user) severs the link. Re-linking a
subject to a different local user requires an explicit administrative delete
and insert; there is no in-place reassignment.

### 4. Claim/group-to-role is a fixed compiled allowlist

The `OIDC_GROUP_TO_ROLES` configuration maps provider group claim values to
platform role codes. Only the four platform roles are valid targets; unknown
codes fail configuration load. OIDC login never grants a role that is not in
this allowlist, and the local account's effective roles are re-derived from
the prelinked local user on every protected request (not from the ID token
alone). This keeps the existing `auth_version` revocation and M35 grant
checking authoritative.

### 5. MFA evidence is verified, not asserted

MFA is identity-provider enforced. The platform verifies that the accepted
`acr` or `amr` value is present in the ID token for every OIDC login. A token
without accepted MFA evidence fails closed, regardless of the user's local
role. Privileged roles do not relax this check; they require it.

### 6. Break-glass remains local and accountable

One or two local break-glass accounts remain available so that operator
access survives a provider outage. Their credentials are stored in an offline
secret manager (never in the OIDC policy file) and are exercised on a bounded
drill interval. Break-glass login produces high-priority audit records. The
local authentication path (ADR 0014, ADR 0019) is unchanged and remains the
fallback.

### 7. Phased delivery

- **M36A** (this ADR): configuration contract, `external_identities` migration
  and model, and the validation that fails closed when OIDC is enabled.
- **M36B**: OIDC discovery fetcher and bounded JWKS cache with rotation,
  keyed by the approved issuer and algorithms.
- **M36C**: Authorization Code + PKCE S256 flow with state, nonce, issuer,
  audience, signature, expiry and MFA evidence validation.
- **M36D**: session and logout management, including local session revocation
  on identity disable, provider logout propagation and break-glass drill
  audit.
- **M36E**: synthetic IdP end-to-end gate proving login, authorization,
  rotation, disable, logout and break-glass without a real organization IdP.

A real organization IdP run is still required for production acceptance; the
synthetic IdP only accepts the local implementation.

## Consequences

- Enabling OIDC is now a configuration decision gated by the same contract as
  the offline readiness gate. A misconfiguration fails fast at startup rather
  than at login time.
- The `external_identities` table is the single source of truth for the
  provider-to-local-user link. Migrating an existing userbase requires
  explicit administrator action; there is no auto-link migration.
- Later phases (M36B-E) must not weaken the invariants in this ADR: PKCE S256,
  immutable subject mapping, MFA evidence verification, per-request status/
  version/session checks, break-glass availability and external-only secret
  supply.
- The local authentication path, refresh-token rotation, session device
  management and audit semantics (ADR 0014, ADR 0019) are preserved. OIDC
  login produces a local session through the same `auth.Service` so that
  existing revocation, grant and audit machinery applies uniformly.
