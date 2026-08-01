# 2026-07-31 M36B — OIDC Discovery and JWKS Runtime Cache

- Milestone: M36 (Production OIDC and MFA), phase B
- ADR: 0052 (Production OIDC and MFA)
- Scope: OIDC discovery fetcher with fail-closed validation and a bounded,
  TTL-based JWKS cache with key rotation. No login endpoint, no HTTP route, no
  ID-token verification yet. OIDC remains disabled by default.

## What changed

### Discovery fetcher (`backend/internal/oidc/discovery.go`)

- `DiscoveryFetcher` fetches `/.well-known/openid-configuration` over HTTPS
  with a bounded timeout, redirect rejection and a 1 MiB response cap.
- `Validate` enforces the runtime contract: discovery issuer exactly matches
  the approved issuer; authorization, token, JWKS and end-session endpoints are
  absolute HTTPS; the `code` response type and PKCE `S256` are advertised;
  every required scope and every approved signing algorithm is advertised.
- Fetched discovery is cached for the configured TTL; `Invalidate` forces a
  refresh (used after rotation or in tests).

### JWKS cache (`backend/internal/oidc/jwks.go`)

- `JWKSCache` fetches and parses the provider JWKS, converting each approved
  JWK into a usable `crypto.PublicKey`. Keys are cached keyed by `kid` with a
  TTL.
- **Rotation**: the fast path returns a cached key immediately when the `kid`
  is present and the cache is fresh. When a `kid` is unknown or the cache is
  stale, a single-flight refresh fetches the current JWKS. Retired keys are
  dropped on the next refresh, so tokens signed with a retired key stop
  validating. Concurrent refreshes are coalesced into one HTTP request.
- Bounds: max 16 keys, duplicate `kid` detection, 1 MiB response cap, HTTPS
  only, redirect rejection.

### Key conversion (`backend/internal/oidc/keyset.go`)

- `JWK.publicKey` converts RSA (`RS256`/`PS256`), EC P-256 (`ES256`) and
  Ed25519 (`EdDSA`) keys into `crypto.PublicKey` instances, mirroring the
  structural validation in `identityreadiness.usableSigningKey` but returning
  usable keys for signature verification. Symmetric algorithms and weak keys
  (RSA modulus < 2048 bits, off-curve points, zero Ed25519 keys) are rejected.

### Bounded HTTP client (`backend/internal/oidc/http.go`)

- `newBoundedHTTPClient` enforces a per-request timeout and rejects redirects
  (SSRF control). `fetchJSON` validates that the target is an absolute HTTPS
  URL without userinfo, caps the response body, and decodes a single JSON
  value. Unknown fields are permitted so the runtime interoperates with
  providers that advertise extensions; required fields are validated
  explicitly.

### Tests

- `keyset_test.go`: RSA/EC/Ed25519 key conversion plus 12 rejection cases
  (missing kid, disallowed algorithm, wrong kty, short modulus, off-curve
  point, wrong curve, etc.).
- `discovery_test.go`: fetch+validate, TTL caching, 10 contract-violation
  cases, HTTPS issuer enforcement and redirect rejection.
- `jwks_test.go`: fetch+KeyByID, fast-path no-refetch, unknown-kid fail-closed,
  rotation drops retired keys, TTL expiry forces refresh, single-flight
  coalescing, duplicate-kid rejection, HTTPS JWKS URI enforcement, too-many-
  keys rejection and unusable-key skipping.

## Invariants preserved

- OIDC is disabled by default; no HTTP route, no public API change, no audit
  behaviour change.
- The provider client secret never enters this layer.
- JWKS rotation takes effect without a restart; retired keys stop validating.
- All provider endpoints must be HTTPS; redirects are rejected.

## Deferred

- M36C: Authorization Code + PKCE S256 flow with ID-token signature, issuer,
  audience, nonce, state and MFA evidence verification.
- M36D: session/logout management and break-glass drill audit.
- M36E: synthetic IdP end-to-end gate.
