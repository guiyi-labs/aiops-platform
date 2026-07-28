# ADR 0018: Compare-and-swap Self-service Password Change

- Status: Accepted
- Date: 2026-07-17

## Context

Users need to rotate their own password without administrator involvement. A simple read-verify-write sequence can race with an administrator reset: both may validate against the same old state, and the slower request could overwrite the newer emergency reset. Successful change must also invalidate every existing session, including the current access token.

## Decision

Expose an authenticated password-change endpoint to every platform role. The service validates new-password length, verifies the submitted current password against the stored bcrypt hash, rejects reuse, and hashes the replacement. The repository update includes both user ID and the previously read password hash in its WHERE clause. Zero updated rows is treated as invalid/stale current credentials.

The successful transaction increments `auth_version` and revokes every active refresh token. The handler clears the refresh Cookie and returns 204; the web console clears its in-memory session and redirects to login. Unified audit records `auth.password.change` after the handler and never receives either password.

## Consequences

- A stale request cannot overwrite an administrator reset or another completed password change.
- All old access and refresh sessions are invalid immediately after commit.
- Users must sign in again on the device that initiated the change.
- Password policy currently enforces length and non-reuse of the current password only; breached-password screening and password history are not included.
- Email recovery, MFA and SSO remain separate capabilities.
