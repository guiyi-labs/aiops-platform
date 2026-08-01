# 2026-07-31 M36F — OIDC HTTP Wiring and GORM Identity Resolver

- Milestone: M36 (Production OIDC and MFA), phase F (final wiring)
- ADR: 0052 (Production OIDC and MFA)
- Scope: wire the OIDC provider, GORM identity resolver and session manager
  into the HTTP server and bootstrap so OIDC login is reachable end-to-end.
  OIDC remains disabled by default; when disabled, no OIDC route is registered
  and the server behaves as a local-only deployment. No public API contract
  changed beyond the three new OIDC routes documented in OpenAPI.

## What changed

### GORM IdentityResolver (`backend/internal/oidc/gorm_resolver.go`)

- `GormIdentityResolver` resolves an (issuer, subject) pair to a prelinked
  local user by joining `external_identities` with `users`. Automatic email
  linking is forbidden: only an explicit administrator-created row resolves.
- `ResolveBySubject` fails closed with `ErrSubjectNotPrelinked` when no row
  exists, when the joined user is missing, or when the database errors. The
  returned `LocalUser` carries the local account's status and role codes so
  the session manager can enforce the active-status check and so per-request
  role re-derivation (via `auth.Authenticate`) stays authoritative.

### AuthSessionIssuer adapter (`backend/internal/oidc/auth_issuer.go`)

- `AuthSessionIssuer` adapts `auth.Service` to the `oidc.SessionIssuer`
  interface by delegating to `auth.Service.IssueSessionForUser`. OIDC-issued
  local sessions flow through the same refresh-token rotation,
  `auth_version` revocation and audit semantics as password login.
- The adapter projects `auth.Session` onto the OIDC-local `Session` shape so
  the session manager stays independent of the auth package's concrete types.
  A disabled user fails closed with `ErrUserDisabled`.

### auth.Service.IssueSessionForUser (`backend/internal/auth/service.go`)

- New exported method `IssueSessionForUser(ctx, userID, userAgent, ipAddress)`
  issues a local session for an already-authenticated user identified by
  `userID`. It looks up the user by ID, enforces the active-status check,
  and delegates to the internal `newSession` so refresh-token rotation,
  access-token issuance and persistence match password login exactly.
- A disabled or missing user fails closed with `ErrUserDisabled` so the OIDC
  callback surfaces a single, consistent error.

### OIDC HTTP handlers (`backend/internal/httpserver/oidc.go`)

- `oidcHandler` exposes three routes under `/api/v1/auth/oidc`:
  - `GET /login` — calls `Provider.AuthorizationURL`, sets the short-lived
    `oidc_auth_session` cookie (HttpOnly, Secure, SameSite=Lax, 10-minute TTL)
    and redirects (302) to the provider authorization endpoint.
  - `GET /callback` — reads the auth-session cookie, calls
    `SessionManager.CompleteLogin` (which verifies the ID token, resolves the
    prelinked user, issues a local session), clears the auth-session cookie,
    sets the refresh-token cookie (same channel as password login), calls
    `setAuditActor` and returns the session JSON. Fail-closed error mapping:
    disabled user → 403 `USER_DISABLED`; not-prelinked subject → 403
    `OIDC_SUBJECT_NOT_PRELINKED`; missing code/state → 400
    `OIDC_CALLBACK_INVALID`; expired auth session → 400
    `OIDC_SESSION_EXPIRED`; other OIDC failures → 502 `OIDC_LOGIN_FAILED`.
  - `POST /logout` — builds the provider `end_session_endpoint` URL for
    RP-initiated logout, using the optional `X-OIDC-ID-Token-Hint` header and
    the configured post-logout redirect URI. Returns `{logout_url}` JSON.
- The handler embeds `authHandler` so it reuses the shared refresh-token cookie
  channel (`setRefreshCookie`/`clearRefreshCookie`) and keeps cookie attributes
  consistent with password login.

### Configuration (`backend/internal/config/config.go`)

- New `OIDC_AUTH_SESSION_SIGNING_KEY` environment variable and
  `OIDCConfig.AuthSessionSigningKey` field (≥32 bytes). The key signs the
  short-lived auth-session cookie that carries PKCE state/nonce between the
  authorization redirect and the callback. It is supplied exclusively from
  environment configuration, never from the browser, audit trail or logs.
- `validate()` fails closed when OIDC is enabled and the signing key is
  missing or shorter than 32 bytes.

### Server bootstrap (`backend/cmd/server/main.go`)

- When `cfg.OIDC.Enabled` is true, the bootstrap constructs the OIDC
  `Provider` from `OIDCConfig`, calls `provider.Init(ctx)` (fetches discovery
  + binds JWKS cache, 30-second timeout), constructs the
  `GormIdentityResolver` and `AuthSessionIssuer`, and builds the
  `SessionManager`. The manager and post-logout URI are injected into
  `httpserver.Options`.
- When OIDC is disabled (default), no provider is constructed and
  `httpserver.Options.OIDC` is nil, so no OIDC route is registered.

### Route registration (`backend/internal/httpserver/router.go`)

- `Options` gained `OIDC *oidc.SessionManager` and `OIDCPostLogout string`
  fields. When `OIDC` is non-nil, three routes are registered under
  `/api/v1/auth/oidc` with audit actions `auth.oidc.login`,
  `auth.oidc.callback` and `auth.oidc.logout` (resource `Session`). The
  callback route is intentionally unauthenticated (the user arrives from the
  IdP redirect); `/login` is unauthenticated; `/logout` requires
  authentication.

### OpenAPI contract (`docs/api/openapi.yaml`)

- Added `/api/v1/auth/oidc/login` (GET, 302),
  `/api/v1/auth/oidc/callback` (GET, 200/400/403) and
  `/api/v1/auth/oidc/logout` (POST, 200) under the `auth` tag. Bidirectional
  route↔OpenAPI parity preserved by `TestRegisteredRoutesMatchOpenAPI`.

## Invariants preserved

- OIDC is disabled by default; no public API contract changed unless OIDC is
  explicitly enabled. When disabled, no OIDC route is registered and the
  server behaves as a local-only deployment (proven by
  `TestOIDCRoutesAbsentWhenDisabled`).
- Automatic email linking is forbidden: the GORM resolver only returns users
  an administrator explicitly prelinked via `external_identities`.
- A disabled or missing prelinked user fails closed even when the ID token is
  valid; the session issuer is never called for a disabled user.
- OIDC-issued local sessions flow through the same refresh-token rotation,
  `auth_version` revocation, per-request role re-derivation and audit
  semantics as password login.
- The provider client secret and auth-session signing key never enter the
  browser, audit trail, logs or policy file.
- MFA evidence is identity-provider enforced and checked in
  `Provider.HandleCallback`; the HTTP layer does not bypass it.

## Test evidence

- `TestIssueSessionForUserIssuesSessionForActiveUser`,
  `TestIssueSessionForUserFailsClosedForDisabledUser`,
  `TestIssueSessionForUserFailsClosedForMissingUser` (`internal/auth`) — the
  new `IssueSessionForUser` method issues a session for an active user and
  fails closed for disabled/missing users.
- `TestOIDCRoutesAbsentWhenDisabled` (`internal/httpserver`) — the three OIDC
  routes are absent when `Options.OIDC` is nil (local-only deployment).
- `TestOIDCCallbackRejectsMissingInputs` — 3 subtests: callback fails closed
  with 400 `OIDC_CALLBACK_INVALID` for missing code, missing state, or both.
- `TestOIDCCallbackRejectsExpiredSession` — callback fails closed with 400
  `OIDC_SESSION_EXPIRED` when the auth-session cookie is missing (replay
  guard).
- `TestRegisteredRoutesMatchOpenAPI` — bidirectional route↔OpenAPI parity
  preserved; the three new OIDC routes are present in both.
- Fast gate `scripts/verify-fast.ps1 -Scope Backend` — passed in 44.3s; all
  26 backend packages green.

## Deferred

- Real organization IdP run (externally gated).
- GORM-based `IdentityResolver` integration test against a real PostgreSQL
  instance (the unit test path is covered by the synthetic IdP E2E gate in
  M36E; a live DB integration test requires the full Compose stack).
- Frontend OIDC login button and callback handling.
