# ADR 0030: Controlled Application Credential Key Re-encryption

- Status: Accepted
- Date: 2026-07-28
- Milestone: M20 Phase 9

## Context

Cluster kubeconfigs are encrypted with AES-256-GCM and each row records its
key version. Before this phase the application could encrypt and decrypt only
with one configured key. Changing that key would make every existing cluster
credential unreadable, while retaining one key forever would prevent routine
rotation after exposure, personnel change or cryptoperiod expiry.

An online HTTP rotation endpoint would enlarge the remote administrative
surface and make request timeout, retries and authorization part of a rare
high-impact operation. A single unbounded database transaction would hold too
many row locks. Logging row contents or cryptographic material as evidence
would defeat the protection being rotated.

## Decision

The online backend has one active encryption key and a bounded legacy
decryption keyring. `CREDENTIAL_ENCRYPTION_KEY` and
`CREDENTIAL_KEY_VERSION` identify the active key. The Secret-only
`CREDENTIAL_DECRYPTION_KEYS` is a JSON object mapping at most eight different
legacy versions to base64-encoded 32-byte keys. The active version cannot be
duplicated in the legacy map. New and replaced credentials always use the
active key; reads select a decryption key from the row's stored version.

Bulk conversion is an offline `/app/credential-reencrypt` command, not an HTTP
route. It defaults to dry-run and requires `--apply` for writes. Batch size is
bounded to 1..100 and the reviewed candidate maximum to 1..10000. A PostgreSQL
advisory lock admits only one command. Apply batches use `FOR UPDATE SKIP LOCKED`;
each batch decrypts and parses every kubeconfig before re-encrypting,
and any error rolls back the complete batch. Plaintext buffers are cleared
after use.

Migration 000016 adds `credential_reencryption_runs`. A run stores only key
version names, dry-run/status, bounded counts, timestamps and stable sanitized
error codes. It never stores keys, plaintext, ciphertext, database URLs or raw
errors. A failed command uses a separate bounded audit context to recount
remaining rows and close the run even when the main context was canceled.

## Consequences

- Operators can deploy an active-plus-legacy overlap, validate it, convert in
  batches and remove the retired key only after the remaining count is zero.
- Unknown versions, corrupt ciphertext and invalid kubeconfigs fail closed;
  committed earlier batches remain valid and the failed batch remains intact.
- The database is briefly mixed-version between committed batches, so every
  application replica must retain all required keys until conversion and
  verification finish.
- PostgreSQL advisory locking prevents two command instances but does not
  replace change approval, a tested backup, Secret-manager controls or
  operator monitoring.
- This phase does not introduce envelope encryption, a cloud KMS/HSM,
  automatic schedules or key material in the database.
