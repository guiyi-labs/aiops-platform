# Signed Audit Archive Runbook

This runbook creates and verifies bounded offline archives from `audit_logs`.
It does not delete rows, replace PostgreSQL backup, or establish a retention
policy.

## Safety Preconditions

- Approve the inclusive `from-id`/`to-id` range and candidate count without
  selecting raw request bodies or unrelated tables.
- Run the reviewed application image from a controlled maintenance host with
  database access. The command uses a read-only repeatable-read transaction.
- Generate and retain the Ed25519 signing key in the approved secret manager.
  The key file must contain base64 for a 32-byte seed or 64-byte private key.
- Distribute the corresponding base64 32-byte public key through a different,
  authenticated channel. Never treat the manifest's embedded key as trusted.
- Use a new output filename in an encrypted, access-controlled location outside
  the repository. Never commit private keys, archives or manifests.

## Create And Verify

The production image contains `/app/audit-archive`. Mount the reviewed private
key and output directory explicitly, then run:

```text
/app/audit-archive \
  --archive=/secure/audit-2026-07-28-1-5000.json \
  --private-key-file=/run/secrets/audit-ed25519.key \
  --from-id=1 --to-id=5000 --max-records=5000 --timeout=5m
```

The command creates the payload and
`audit-2026-07-28-1-5000.json.manifest.json`, prints only path/hash/count/range
metadata, and refuses existing paths. Preserve the pair together.

On a separate verification host, obtain the trusted public key independently
and run without database credentials:

```text
/app/audit-archive \
  --verify \
  --archive=/evidence/audit-2026-07-28-1-5000.json \
  --trusted-public-key-file=/trust/audit-ed25519.pub
```

Use `--manifest=/reviewed/path.json` only when the detached manifest was
renamed. `--print-public-key --private-key-file=...` derives the public key for
an approved provisioning workflow; its JSON output must still be delivered
through the separate trust channel.

## Acceptance And Failure Handling

Accept an archive only when verification exits zero and its payload SHA-256,
record count and first/last IDs match the reviewed archive register. Reject and
quarantine the pair if the signature, digest, format, count, range, ordering or
trusted-key comparison fails.

`audit archive record limit exceeded` means the candidate count exceeded the
reviewed bound before output. Split the ID range or approve a larger bound, but
never above 10000. An empty range, invalid key, database failure or existing
output path also fails closed. Do not overwrite evidence in place; create a new
versioned path after correcting the cause.

After acceptance, remove the private-key mount from the maintenance runtime and
store the archive pair under the approved encryption, access, replication,
retention and destruction controls. Maintain a separate register of exported
ranges and hashes because the current format is not hash chained.

## Isolated Verification

Run from the repository root:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-audit-archive.ps1
```

The gate starts isolated PostgreSQL, seeds synthetic sanitized audit rows,
creates and verifies a two-record archive, proves an oversized selection leaves
no output, mutates one payload byte and proves rejection. It then deletes the
database container, private network, temporary image, archive/key directory and
process environment. Only sanitized evidence under ignored
`.artifacts/audit-archive` remains.
