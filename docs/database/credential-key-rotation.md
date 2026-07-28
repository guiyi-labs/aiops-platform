# Application Credential Encryption-Key Rotation

This runbook rotates the application key used for encrypted cluster
kubeconfigs. It does not replace a cluster's kubeconfig; that separate API
operation is documented as cluster credential replacement.

## Safety Preconditions

- Take and validate a fresh PostgreSQL backup using the isolated restore
  procedure in `docs/database/backup-restore.md`.
- Inventory `cluster_credentials.encryption_key_version` counts without
  selecting `encrypted_kubeconfig`.
- Generate the new 32-byte key in the approved secret manager. Never place it
  in Git, tickets, shell history, logs or evidence.
- Confirm every backend replica receives the same active key/version and the
  same legacy key map before any row is changed.
- Keep the old key recoverable under the approved retention policy until both
  database conversion and post-rotation application checks pass.

## Rotation Sequence

Assume `v1` is current and `v2` is new.

1. Update the runtime Secret so `CREDENTIAL_ENCRYPTION_KEY` contains the new
   key, `CREDENTIAL_KEY_VERSION=v2`, and `CREDENTIAL_DECRYPTION_KEYS` is a JSON
   object containing the old `v1` key. Roll every backend replica and confirm
   readiness.
2. Run a dry-run from exactly one reviewed backend container or pod:

   ```powershell
   docker compose exec backend /app/credential-reencrypt --batch-size=50 --max-records=1000 --timeout=15m
   ```

   For Kubernetes, use the same command through `kubectl exec` against one
   ready backend pod. The PostgreSQL advisory lock rejects a second command.
3. Review only the JSON metadata: target/source versions, examined count,
   remaining count and status. Increase `--max-records` only after reviewing
   the actual candidate count, never above 10000.
4. Apply the reviewed conversion:

   ```powershell
   docker compose exec backend /app/credential-reencrypt --apply --batch-size=50 --max-records=1000 --timeout=15m
   ```

5. Require `status=succeeded`, `remaining_count=0` and matching sanitized rows
   in `credential_reencryption_runs`. Probe representative enabled clusters
   and check application health with the active-plus-legacy keyring still
   present.
6. Remove `v1` from `CREDENTIAL_DECRYPTION_KEYS`, roll every replica again and
   repeat readiness and representative cluster checks. Retire the old key only
   under the approved retention/destruction policy.

The command is intentionally not exposed through the API. Omit `--apply` for
dry-run. Valid bounds are `--batch-size=1..100` and
`--max-records=1..10000`.

## Failure And Rollback

- If dry-run fails, do not apply. Restore the missing version/key mapping or
  repair the identified data under a separate approved procedure.
- If apply fails, do not remove either key. The current batch is rolled back;
  earlier committed batches may already be on `v2`. Correct the cause and
  rerun toward `v2`, or explicitly switch the active key back to `v1`, retain
  `v2` as a legacy decryption key and run the same reviewed process in reverse.
- `UNKNOWN_KEY_VERSION` means the deployed keyring cannot decrypt at least one
  stored version. `INVALID_KUBECONFIG` means authenticated plaintext failed the
  strict parser. `REENCRYPTION_FAILED` covers corrupt ciphertext or a database
  failure. `RECORD_LIMIT_EXCEEDED` requires a new reviewed record bound.
- If a restored backup contains old-version rows, restore the corresponding
  legacy key before starting the backend. A database backup without its
  separately protected key material is not recoverable.

Never query, export or paste `encrypted_kubeconfig` while diagnosing a run.
The audit table and command JSON are the only approved evidence surfaces.

## Isolated Verification

Run from the repository root:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-credential-reencryption.ps1
```

The gate creates an isolated PostgreSQL/backend runtime, proves dry-run,
transaction rollback, successful v1-to-v2 conversion and v2-only backend
decryption, then removes its containers, network and temporary image. It never
connects to the retained Compose database. Sanitized evidence is written under
the ignored `.artifacts/credential-reencryption` directory.
