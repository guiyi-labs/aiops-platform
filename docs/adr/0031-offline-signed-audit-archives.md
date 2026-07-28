# ADR 0031: Offline Signed Audit Archives

- Status: Accepted
- Date: 2026-07-28
- Milestone: M20 Phase 10

## Context

`audit_logs` is append-only through the application and its public model is
sanitized, but a database row or CSV file alone cannot be independently checked
after it leaves PostgreSQL. CSV export is designed for bounded human review;
it is not a durable integrity envelope. Adding a remote archive endpoint would
increase the high-impact HTTP administration surface and encourage long-running
requests. Trusting a public key stored only beside an archive would also let an
attacker replace the payload, signature and key together.

## Decision

Signed archival is an offline `/app/audit-archive` command and does not add an
HTTP route. Creation requires an inclusive positive audit ID range, an explicit
output path and a base64 Ed25519 private-key file. The caller fixes a
`--max-records` bound of 1..10000. A repeatable-read, read-only PostgreSQL
snapshot counts candidates before loading them; an oversized range is rejected
before either output file is created.

The payload is canonical JSON built from the existing sanitized `audit.Entry`
model and ordered by ascending ID. A detached JSON manifest records the format,
payload SHA-256, count, first/last ID, UTC creation time, tool version,
signature algorithm and signer public key. Ed25519 signs the canonical manifest
fields. Creation refuses to overwrite either file and uses owner-only file mode
where the operating system supports it.

Verification requires `--trusted-public-key-file`. It first compares that
external trust anchor with the embedded public key, verifies the manifest
signature, verifies the exact payload bytes against SHA-256, then validates the
payload format, count, range and strict ascending order. The embedded key alone
is never accepted as trust.

Private keys, database URLs, archives and manifests are not persisted by the
application or CI. The isolated test uses an ephemeral seed and synthetic audit
rows, retains only sanitized booleans/counts and deletes every temporary file.

## Consequences

- An exported archive can be verified offline against a separately distributed
  trusted public key, including after transport or storage.
- The signature proves integrity and signer-key possession; it does not prove
  that the signing key was uncompromised or that database administrators never
  removed records before export.
- Gaps in PostgreSQL sequence IDs are valid. Operators must review range and
  count continuity under their retention policy; this phase does not add hash
  chaining, transparency logging, WORM storage or automatic schedules.
- Archives contain sanitized but still security-relevant operational metadata.
  They require encrypted storage, access control, retention and deletion rules
  outside the application.
- No rows are marked archived or deleted, and signed archival is not a database
  backup, PITR mechanism or legal-records policy.
