# 2026-07-17 Self-service Password Change

## Scope

- Add authenticated `POST /api/v1/auth/password-change` for all platform roles.
- Verify current bcrypt password, require a different 12–128 character replacement, and use compare-and-swap persistence.
- Increment `auth_version`, revoke every refresh session and clear the current refresh Cookie on success.
- Add `auth.password.change` audit mapping without request-body capture.
- Add `/account/security`, a top-bar shortcut, validation feedback and forced redirect to login after success.

## Verification

- Go tests cover wrong current password, unchanged password, bcrypt replacement hash and expected-hash compare-and-swap input.
- Frontend typecheck and 22 Vitest tests pass, including the access-token and body contract.
- Real PostgreSQL/API verification: wrong current password returned 401; successful change returned 204 and incremented `auth_version` from 1 to 2.
- The previous access token, refresh session and password all returned 401; the replacement password logged in successfully; reusing it as the new value returned 409; anonymous access returned 401.
- Audit records included success/204, failure/409 and denied/401 outcomes, with zero replacement-password matches in audit details.
- Final `go test ./...`, server build, frontend typecheck, 22 Vitest tests and production build all passed.

## Boundaries

- No email recovery, forced first-login change, password history, breached-password lookup, MFA or SSO.
- Password change intentionally signs out every device rather than preserving the initiating session.
