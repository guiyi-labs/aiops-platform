# 2026-07-31 M36D — OIDC Session, Logout and Break-Glass Drill Management

- Milestone: M36 (Production OIDC and MFA), phase D
- ADR: 0052 (Production OIDC and MFA)
- Scope: bridge from a verified OIDC callback to a local session, RP-initiated
  provider logout, and break-glass drill audit tracking. No HTTP route, no
  public API change. OIDC remains disabled by default.

## What changed

### Session manager (`backend/internal/oidc/session.go`)

- `LocalUser` is the resolved local account for an OIDC subject.
- `Session` mirrors `auth.Session` (access token, token type, expiry, refresh
  token, user view) so OIDC login produces a local session through the same
  shape as password login.
- `IdentityResolver` interface resolves an `(issuer, subject)` pair to a
  prelinked local user via `external_identities`. Automatic email linking is
  forbidden; the resolver only returns users an administrator explicitly
  prelinked.
- `SessionIssuer` interface issues a local session for a prelinked user. The
  implementation delegates to `auth.Service` so refresh-token rotation,
  `auth_version` revocation, per-request status checks and audit semantics are
  preserved.
- `SessionManager.CompleteLogin` orchestrates: `Provider.HandleCallback` →
  `IdentityResolver.ResolveBySubject` → status check → `SessionIssuer.IssueSession`.
  It fails closed when the ID token is invalid, the subject is not prelinked,
  the prelinked user is disabled, or the session issuer fails. The local
  account's effective roles are re-derived from the prelinked local user on
  every protected request (via `auth.Authenticate`), not from the ID token
  alone.
- `SessionManager.Reauthentication` exposes the configured reauthentication
  interval so the HTTP handler can decide when to redirect back to the provider
  for fresh MFA evidence rather than silently refreshing.
- `ErrSubjectNotPrelinked` and `ErrUserDisabled` are sentinel errors the
  caller can branch on for audit and user-facing messaging.

### Provider logout (`backend/internal/oidc/session.go`)

- `Provider.LogoutURL` builds the provider `end_session_endpoint` URL for
  RP-initiated logout. It accepts an optional `id_token_hint` (so the provider
  can identify the session) and an optional `post_logout_redirect_uri` (which
  must be HTTPS). Empty parameters are omitted per OIDC spec. It fails closed
  when the provider does not advertise an `end_session_endpoint`.
- `StripBearerPrefix` is a small helper so handlers can pass an Authorization
  header value directly as the `id_token_hint`.

### Break-glass drill audit (`backend/internal/oidc/breakglass.go`)

- `BreakGlassEvent` records a single break-glass login (user id, username,
  user agent, IP address, reason, timestamp).
- `BreakGlassAuditor` interface forwards break-glass events to the platform
  audit trail at high priority (the production implementation is wired by the
  HTTP handler).
- `BreakGlassDrillTracker` records break-glass login events and reports
  whether the drill interval is current (`IsCurrent`, `LastDrillAt`). The
  readiness gate uses `IsCurrent` to flag a stale fallback. The tracker
  requires a non-empty reason (drill or outage) for every event and caps the
  maximum number of break-glass accounts at two (ADR 0052). It does not mark
  itself current when the auditor fails, so a broken audit trail cannot mask a
  missing drill.
- `BreakGlassDrillConfig` bounds the required drill interval (default 7 days)
  and max accounts (default 2, capped at 2).

### Local session revocation on identity disable

- The existing `auth.GormRepository.UpdateUser` already proactively revokes all
  non-revoked refresh tokens for a user when their status transitions to
  `disabled` or when their roles change (the `securityChanged` branch). This
  satisfies the `revoke_local_sessions_on_identity_disable = true` config
  invariant: a disabled user's refresh tokens are revoked immediately, not
  merely blocked on the next use. Access tokens are short-lived (15m) and
  `auth.Authenticate` checks status on every protected request, so a disabled
  user's session takes effect on the next request at most.

### Tests (`backend/internal/oidc/session_test.go`)

- 6 session-manager tests: `CompleteLogin` happy path (asserts resolver and
  issuer receive the correct issuer/subject/user id/agent/IP), failure when the
  ID token is invalid (resolver/issuer not called), failure when the subject is
  not prelinked, failure when the prelinked user is disabled, propagation of
  session-issuer errors, and the reauthentication interval (configured + default).
- 3 provider-logout tests: end-session URL with both parameters, empty
  parameters produce no query string, HTTP `post_logout_redirect_uri` rejected.
- 1 `StripBearerPrefix` test (5 cases).
- 7 break-glass drill tests: event recording + auditor forwarding, required
  fields (reason/user id/username), auditor error propagation (tracker stays
  stale), stale-after-interval reporting, config defaults and max-accounts cap,
  and auditor-less operation.

## Invariants preserved

- OIDC is disabled by default; no HTTP route added, no public API contract
  changed, no audit behaviour changed.
- OIDC login produces a local session through the same `auth.Service` path as
  password login, so existing revocation, grant and audit machinery applies
  uniformly.
- The local account's effective roles are re-derived from the prelinked local
  user on every protected request, not from the ID token alone.
- Automatic email linking is forbidden; only administrator-prelinked subjects
  resolve to a local session.
- Break-glass remains local and accountable: every break-glass use requires a
  reason and produces a high-priority audit record.
- The provider client secret never enters this layer.

## Deferred

- M36E: synthetic IdP end-to-end gate (login, authorization, rotation,
  disable, logout, break-glass) and the OIDC HTTP route wiring.
- GORM-based `IdentityResolver` implementation (queries `external_identities`
  joined with `users`).
- Real organization IdP run (externally gated).
