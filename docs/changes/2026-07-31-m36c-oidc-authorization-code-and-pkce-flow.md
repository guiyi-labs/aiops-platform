# 2026-07-31 M36C — OIDC Authorization Code + PKCE S256 Flow

- Milestone: M36 (Production OIDC and MFA), phase C
- ADR: 0052 (Production OIDC and MFA)
- Scope: PKCE S256 code verifier/challenge generation, HMAC-signed short-lived
  auth-session cookie, the Authorization Code flow against an organization-
  approved OIDC provider, and ID-token signature/issuer/audience/nonce/expiry/
  MFA-evidence verification. No HTTP route, no session issuance, no public API
  change. OIDC remains disabled by default.

## What changed

### PKCE and auth-session cookie (`backend/internal/oidc/pkce.go`)

- `GenerateCodeVerifier` produces a 43-character base64url PKCE verifier (32
  random bytes) and its S256 challenge, per RFC 7636.
- `randomString` returns a base64url-encoded random string used for state and
  nonce.
- `authSessionSigner` issues and verifies a short-lived (10-minute) HMAC-SHA256
  signed token carrying `state`, `nonce`, `code_verifier` and `expires_at`. The
  callback verifies the HMAC in constant time and rejects expired, malformed or
  tampered tokens. This lets the server validate state/nonce/PKCE without
  server-side session storage.

### Provider (`backend/internal/oidc/provider.go`)

- `ProviderConfig` adapts `config.OIDCConfig` into runtime parameters and
  validates the fail-closed invariants (subject claim is `sub`, required claim
  mappings, at least one signing algorithm, at least one group-to-role mapping,
  required MFA with `acr`/`amr` evidence, signing key >= 32 bytes).
- `Provider.AuthorizationURL` fetches discovery, generates a PKCE pair plus
  state and nonce, signs an auth-session cookie, and builds the provider
  authorization endpoint URL with `response_type=code`, `client_id`,
  `redirect_uri`, `scope`, `state`, `nonce`, `code_challenge` and
  `code_challenge_method=S256`.
- `Provider.HandleCallback` verifies the auth-session cookie, checks the state
  parameter, exchanges the authorization code at the token endpoint (sending
  `client_secret` only when configured and only over the back channel), and
  verifies the returned ID token.
- `verifyIDToken` validates the signature (against the JWKS cache, restricted
  to the approved algorithms), issuer, audience, nonce, expiry and MFA
  evidence; extracts the configured claims; maps groups to platform roles; and
  fails closed on any mismatch or missing claim. A token whose groups map to no
  platform role is rejected.
- `mapGroupsToRoles` deduplicates and sorts the role codes derived from the
  configured group-to-role allowlist; only the four platform roles are valid
  targets.
- `verifyMFA` / `mfaEvidence` support both `acr` (string) and `amr` (array)
  evidence claims and reject tokens without accepted evidence.

### Bounded HTTP (`backend/internal/oidc/http.go`)

- Extracted `decodeLimitedJSON` so both GET (discovery/JWKS) and POST (token
  exchange) responses share the same 1 MiB body cap, single-JSON-value
  decoding and empty-body rejection.

### Tests

- `pkce_test.go` (8 tests): RFC 7636 verifier/challenge shape and uniqueness,
  `randomString` uniqueness, auth-session signer issue/verify round trip,
  malformed-token rejection (4 cases), wrong-signature rejection, expired-
  session rejection and corrupted-payload rejection.
- `provider_test.go` (15 tests + 21 subtests): `ProviderConfig` validation
  (10 cases), `AuthorizationURL` required parameters and cookie round trip,
  `HandleCallback` happy path through a synthetic HTTPS IdP, callback
  rejection (missing inputs, state mismatch, invalid session token, token
  endpoint 500, missing `id_token`), ID-token contract violations (wrong
  issuer/audience/nonce, expired, missing MFA, unaccepted MFA, missing sub,
  missing username/display name, no role-mapped groups), disallowed signing
  algorithm (HS256 rejected), unknown `kid`, `amr` evidence acceptance,
  group-to-role dedup/sort, claim extraction (string/array/other), string
  claim coercion, discovery/JWKS lazy binding, and a guard that the client
  secret never enters the authorization URL but does reach the token endpoint.
- The synthetic IdP (`syntheticIdP`) is a minimal HTTPS OIDC provider serving
  discovery, JWKS and the token endpoint, with a `tokenBehavior` switch for
  forced failure modes. It signs ID tokens with a 2048-bit RSA key published in
  JWKS so the full signature-verification path is exercised.

## Invariants preserved

- OIDC is disabled by default; no HTTP route added, no public API contract
  changed, no audit behaviour changed.
- PKCE S256 is mandatory in every environment; the client secret never enters
  the browser, authorization URL, audit trail or logs.
- State, nonce and PKCE verifier are bound to a short-lived HMAC-signed cookie
  and verified at the callback; a mismatch or expiry fails closed.
- ID-token verification fails closed on wrong issuer, audience, nonce,
  signature, algorithm, `kid`, expiry, MFA evidence or claim shape.
- The four platform roles remain the only accepted role targets; the local
  account's effective roles are re-derived from the prelinked local user on
  every protected request (M36D responsibility), not from the ID token alone.
- All provider endpoints must be HTTPS; redirects are rejected; response bodies
  are capped at 1 MiB.

## Deferred

- M36D: session and logout management, including local session revocation on
  identity disable, provider logout propagation and break-glass drill audit.
- M36E: synthetic IdP end-to-end gate (login, authorization, rotation,
  disable, logout, break-glass) and the OIDC HTTP route wiring.
- Real organization IdP run (externally gated).
