# 2026-07-17 Session Device Management

## Scope

- Add authenticated session list, revoke-one-other and revoke-all-others endpoints.
- Return only non-sensitive client metadata and a server-computed current marker.
- Require an active current refresh Cookie before revocation and protect the current session from ID-based removal.
- Scope every lookup/update by authenticated user ID.
- Add session controls to the account-security page and audit both mutation routes.

## Verification

- Go tests cover current-marker projection, hash-only repository calls, missing-current rejection and audit route mapping.
- Frontend typecheck and 23 Vitest tests pass, including list/single/bulk request contracts.
- Real API verification with two sessions listed exactly one current row; current ID revocation returned 409; other ID revocation returned 204 and its refresh returned 401.
- A third session was revoked by the bulk endpoint while current refresh remained 200; anonymous bulk revocation returned 401.
- Audit contained failure/409, success/204, success/200 and denied/401 for session mutations.
- Final `go test ./...`, server build, frontend typecheck, 23 Vitest tests and production build all passed.

## Boundaries

- Session metadata uses stored User-Agent and IP strings without device fingerprinting or geolocation.
- The current session is ended through logout, not the session-ID endpoint.
- MFA, SSO and suspicious-login notifications remain outside this stage.
