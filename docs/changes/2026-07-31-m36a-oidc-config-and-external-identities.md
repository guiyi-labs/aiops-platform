# 2026-07-31 M36A — OIDC Configuration and External Identities

- Milestone: M36 (Production OIDC and MFA), phase A
- ADR: 0052 (Production OIDC and MFA)
- Scope: configuration layer, immutable subject-prelinking table, model and
  contract tests. No login endpoint, no HTTP route, no discovery/JWKS runtime,
  no session change. OIDC remains disabled by default.

## What changed

### Configuration (`backend/internal/config/config.go`)

- Added `OIDCConfig` and sub-types (`OIDCClaimMapping`, `OIDCMFAConfig`,
  `OIDCSessionConfig`, `OIDCBreakGlassConfig`, `OIDCJWKSConfig`) to the
  `Config` struct.
- Added `loadOIDCConfig` and a `validate` method that fail closed when
  `OIDC_ENABLED=true`: canonical HTTPS issuer (no trailing slash), non-empty
  client id, absolute fragment-free HTTPS redirect URI, `subject = "sub"`,
  explicit username/display-name/groups claims, `openid`+`profile` scopes,
  approved asymmetric signing algorithms (`RS256`/`PS256`/`ES256`/`EdDSA`),
  at least one group-to-role mapping targeting the four platform roles,
  required identity-provider-enforced MFA with `acr`/`amr` evidence, bounded
  session max age (5m..24h) and reauthentication (1m..max age), mandatory
  `revoke_on_disable`, enabled break-glass (1..2 accounts), bounded JWKS cache
  TTL (1m..24h) and refresh timeout (1s..1m).
- Production requires `OIDC_CLIENT_SECRET` (confidential client). Development
  permits a public PKCE-only client. PKCE S256 is mandatory in every
  environment.
- Added helpers `stringListFromEnv`, `groupToRolesFromEnv` (JSON map with role
  validation and deduplication) and `containsAllStrings`.

### Database migration (`backend/migrations/000026_external_identities`)

- Added the `external_identities` table for administrator-owned immutable
  subject prelinking. Columns: `id`, `user_id` (FK to `users`), `issuer`,
  `subject`, `created_at`.
- Unique constraints: `(issuer, subject)` (one provider subject maps to at
  most one local user) and `(user_id, issuer, subject)` (a user is prelinked
  to a provider at most once). Automatic email linking is forbidden; rows are
  created only by explicit administrator action.
- Paired down migration drops the table.

### Model (`backend/internal/oidc/model.go`)

- New `oidc` package with `ExternalIdentity` GORM model. `TableName` is pinned
  to `external_identities` so GORM inference never drifts from the migration.

### Tests

- `backend/internal/config/config_test.go`: added a valid-OIDC env helper and
  tests covering disabled-by-default, valid load, validation-ignored-when-
  disabled, 21 invalid-configuration cases, production secret requirement,
  group-to-role deduplication and empty-mapping rejection.

## Invariants preserved

- OIDC is disabled by default; local authentication (ADR 0014, ADR 0019) is
  unchanged.
- No public API contract changed. No HTTP route added. No audit behaviour
  changed.
- The provider client secret never enters the browser, audit trail, logs or
  policy file.
- The four platform roles remain the only accepted role targets; `SystemAdmin`
  bypass (ADR 0050) is unchanged.

## Deferred

- M36B: OIDC discovery fetcher and bounded JWKS cache with rotation.
- M36C: Authorization Code + PKCE S256 flow.
- M36D: session/logout management and break-glass drill audit.
- M36E: synthetic IdP end-to-end gate.
- Real organization IdP run (externally gated).
