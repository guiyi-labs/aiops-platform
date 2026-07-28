# ADR 0019: Cookie-bound Session Device Management

- Status: Accepted
- Date: 2026-07-17

## Context

Users need visibility into active login devices and a way to revoke a lost browser without changing their password. Refresh tokens are intentionally stored only as SHA-256 summaries, and a sequential database ID alone must not authorize revocation across users. Bulk revocation must not accidentally remove every valid session when the caller presents an absent or stale Cookie.

## Decision

List only the authenticated user's active, unexpired refresh-token rows and project them to ID, User-Agent, IP address, creation/expiry time and a current flag. The current flag is computed server-side by hashing the HttpOnly Cookie and comparing it with each stored summary; neither raw token nor summary enters the response.

Single revocation is scoped by both user ID and session ID, requires a still-active current Cookie session, and rejects the current row. Bulk revocation uses one transaction to prove the current summary exists and then sets `revoked_at` only on other rows. Current-session termination remains the existing logout operation. Mutation routes use unified audit mappings without token details.

## Consequences

- A user can inspect and revoke other devices without administrator access.
- Guessing another user's session ID returns not found and cannot revoke it.
- Stale or missing cookies cannot trigger bulk revocation.
- User-Agent and source IP are self-visible security metadata and must not be exposed to other users.
- Device naming, geolocation and push notifications are not inferred from untrusted User-Agent/IP values.
