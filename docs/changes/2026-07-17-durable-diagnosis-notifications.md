# Change Record: Durable diagnosis notifications

- Date: 2026-07-17
- Scope: Diagnosis event outbox, webhook delivery and delivery operations UI

## Delivered

- Added migration `000012_diagnosis_notification_outbox` with a singleton
  runtime setting, durable delivery rows, due/stale claim indexes and an
  allowlisted diagnosis-event trigger.
- Added `diagnosis.created`, `diagnosis.status_changed` and
  `diagnosis.assigned` events without diagnosis evidence, workflow comments or
  credentials.
- Added a background webhook worker with `FOR UPDATE SKIP LOCKED` claims,
  request timeout, HMAC-SHA256 body signatures, redirect rejection, bounded
  exponential retry and terminal `dead` state.
- Added delivery list and dead-delivery retry APIs. The list is restricted to
  system administrators and security auditors and omits stored payloads;
  manual retry is restricted to system administrators and audited as
  `notification.delivery.retry`.
- Added the Event Center frontend view with delivery filters, safe metadata and
  system-administrator retry controls.
- Added disabled-by-default Compose and Kubernetes configuration. Production
  enables notifications only with an HTTPS endpoint and a secret of at least
  32 characters.

## Verification

- Unit tests cover signed success delivery, non-2xx retry, retry delay cap,
  disabled behavior, redirect rejection, URL/error redaction, HTTP handlers,
  configuration validation and frontend API behavior.
- PostgreSQL migration and trigger verification produced exactly one event for
  diagnosis creation, status change and assignment change.
- A single SQL update that changed both status and assignee produced both
  `diagnosis.status_changed` and `diagnosis.assigned`; the transaction was
  rolled back and left no QA data.
- A loopback webhook receiver accepted all three events and database rows
  reached `delivered` with one attempt. Signatures and event envelopes were
  inspected; payloads contained no evidence or secret.
- Real authenticated API checks returned safe delivery metadata, rejected
  anonymous access, returned `NOTIFICATIONS_DISABLED` when appropriate, and
  recorded the retry failure audit event.
- Temporary receiver processes, QA clusters, diagnoses and delivery rows were
  removed after verification, and the persisted notification setting was
  restored to disabled.

The final repository-wide command results for this stage are recorded in
`docs/development-handoff.md`.
