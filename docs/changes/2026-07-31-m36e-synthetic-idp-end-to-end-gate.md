# 2026-07-31 M36E — Synthetic IdP End-to-End Gate

- Milestone: M36 (Production OIDC and MFA), phase E (final)
- ADR: 0052 (Production OIDC and MFA)
- Scope: synthetic IdP end-to-end gate proving login, authorization, signing-key
  rotation, identity disable, provider logout and break-glass drill audit without
  a real organization IdP. No HTTP route, no public API change. OIDC remains
  disabled by default.

## What changed

### Synthetic IdP key rotation (`backend/internal/oidc/provider_test.go`)

- `syntheticIdP.RotateKey` generates a fresh signing key with a new kid and
  retires the current key. The JWKS endpoint publishes only the new key
  afterwards, mirroring provider key retirement. The retired key is returned so
  E2E rotation tests can sign tokens with it to assert they fail closed after
  the JWKS cache refresh drops them.
- `syntheticIdP.signIDTokenWithKey` signs an ID token with an explicit signing
  key (the current or a retired one). `signIDToken` now delegates to it so the
  default and rotation paths share one implementation.

### End-to-end lifecycle gate (`backend/internal/oidc/e2e_test.go`)

`TestSyntheticIdPEndToEndLifecycle` is the M36E gate. It shares a single
synthetic IdP, provider, session manager, identity resolver, session issuer and
break-glass drill tracker across six ordered subtests so the lifecycle
narrative accumulates state exactly as a production deployment would:

1. **Login** — drives a full Authorization Code + PKCE flow through
   `SessionManager.CompleteLogin` and asserts the issued local session carries
   the correct access/refresh tokens, user identity, and that the resolver and
   issuer were called with the correct issuer/subject/user-id/agent/IP.
2. **Authorization** — drives `Provider.HandleCallback` and asserts the
   `CallbackResult` carries the verified subject, username, display name,
   group-mapped roles (`oidc-admins` → `system_admin`) and MFA evidence (`mfa`).
   Then proves MFA is enforced: a token whose `acr` evidence is empty fails
   closed.
3. **Rotation** — rotates the IdP signing key; a new login signed with the
   rotated key succeeds (proving the JWKS cache refreshed and picked up the new
   key); a callback where the IdP returns a token signed with the retired key
   fails closed with `ErrUnknownKey` (the cache refresh dropped the retired kid).
4. **Disable** — sets the prelinked user's status to `disabled`; an OIDC login
   attempt fails closed with `ErrUserDisabled` and the session issuer is not
   called (no session for a disabled user). The user is restored afterwards.
5. **Logout** — `Provider.LogoutURL` builds the provider `end_session_endpoint`
   URL with the `id_token_hint` and HTTPS `post_logout_redirect_uri`.
6. **BreakGlass** — the drill tracker is initially stale; recording a drill
   forwards the event to the auditor, marks the tracker current, and the
   recorded event carries the high-priority audit fields. A break-glass event
   without a reason is rejected so every use is attributed to a drill or outage.

`TestSyntheticIdPEndToEndBreakGlassStaleness` proves the break-glass drill
tracker reports stale when the drill interval has elapsed, using a custom clock
so the gate does not depend on real-time waiting.

## Invariants preserved

- OIDC is disabled by default; no HTTP route added, no public API contract
  changed, no audit behaviour changed.
- The E2E gate exercises the same `Provider`, `SessionManager` and
  `BreakGlassDrillTracker` components shipped in M36B-M36D without mocks of the
  OIDC primitives: discovery, JWKS cache, PKCE, ID-token verification, MFA
  evidence, session issuance and break-glass audit all run through their real
  implementations against the synthetic IdP.
- Retired signing keys fail closed after the JWKS cache refresh drops them;
  there is no graceful fallback to an unknown kid.
- A disabled prelinked user fails closed even when the ID token is valid; the
  session issuer is never called.
- Break-glass remains local and accountable: every break-glass use requires a
  reason and produces a high-priority audit record.

## Test evidence

- `TestSyntheticIdPEndToEndLifecycle` (6 subtests: Login, Authorization,
  Rotation, Disable, Logout, BreakGlass) — all pass.
- `TestSyntheticIdPEndToEndBreakGlassStaleness` — passes.
- Fast gate `scripts/verify-fast.ps1 -Scope Backend` — passed in 26.4s; full
  `internal/oidc` suite green.

## Deferred

- OIDC HTTP route wiring (login/callback/logout handlers in `httpserver`).
- GORM-based `IdentityResolver` implementation (queries `external_identities`
  joined with `users`).
- Real organization IdP run (externally gated).
