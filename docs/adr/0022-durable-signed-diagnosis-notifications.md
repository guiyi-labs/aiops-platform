# ADR 0022: Durable signed diagnosis notifications

- Status: Accepted
- Date: 2026-07-17

## Context

Diagnosis creation, status changes and assignment changes need to reach an
external event receiver. Sending HTTP inside the diagnosis transaction would
couple database availability to a remote service and could either lose an
event after commit or hold business locks during a slow request. Forwarding
the complete diagnosis object would also expose evidence and operator data
outside the platform boundary.

## Decision

PostgreSQL creates a notification delivery row in the same transaction as the
diagnosis mutation. A background worker claims due rows with
`FOR UPDATE SKIP LOCKED`, sends a bounded JSON envelope, and records delivery
state independently. Events cover diagnosis creation, status change and
assignment change. The payload is an explicit allowlist and excludes evidence,
comments, credentials and user contact data.

Webhook requests use HMAC-SHA256 over the exact request body in
`X-AIOps-Signature`. Redirects are not followed. A 2xx response marks delivery
complete; other outcomes use capped exponential backoff and eventually enter
`dead`. Only a system administrator can manually requeue a dead delivery.
System administrators and security auditors may inspect delivery metadata, but
the API never returns the stored payload.

## Consequences

Committed diagnosis changes remain deliverable across process restarts and
multiple workers can claim disjoint batches. Receivers must deduplicate by the
stable event ID and verify the signature before processing. Database triggers
are now part of the diagnosis write contract and must be maintained when the
diagnosis schema changes. This stage only sends notifications; it does not
authorize or execute remediation.
