# M20 Phase 9: Application Credential Key Re-encryption

- Date: 2026-07-28
- Status: Accepted
- Accepted revision: `151bc7ee848391e37b74d59f489bbe804d9234ff`
- Hosted CI: [run 30334216631](https://github.com/guiyi-labs/aiops-platform/actions/runs/30334216631)
- Scope: versioned keyring, controlled offline re-encryption, rollback and sanitized evidence

## Delivered

- Added ADR 0030 and `docs/database/credential-key-rotation.md`.
- Added a version-aware AES-256-GCM keyring with one active key and up to eight
  Secret-only legacy keys from `CREDENTIAL_DECRYPTION_KEYS`.
- Added `/app/credential-reencrypt`, default dry-run, explicit `--apply`,
  1..100 batch bounds, 1..10000 reviewed-record bounds and one PostgreSQL
  advisory lock.
- Added migration 000016 for sanitized run status/version/count/error metadata.
- Apply uses `FOR UPDATE SKIP LOCKED` with one transaction per batch. Unknown
  versions, authentication failures or invalid kubeconfigs roll back the whole
  current batch; plaintext buffers are cleared after parsing/encryption.
- Added the command to the production image, regular backend build, local
  verification and hosted runtime job.
- Added an isolated PostgreSQL/backend gate that never touches retained Compose
  data and always removes its containers, network, image and process secrets.

## Verification

Targeted Go 1.25 container tests passed for cluster, config and command
packages. The new service tests cover dry-run no-write behavior, v1-to-v2
plaintext preservation, unknown-version sanitization, full-batch rollback,
preflight bounds and concurrent dry-run candidates.

The final isolated run at 2026-07-28 14:13 +08:00 passed with evidence at
`.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`:

- two API-created v1 credentials remained v1 after dry-run (2 examined, 0
  modified, 2 remaining);
- a corrupt second row produced `REENCRYPTION_FAILED`, left both rows on v1 and
  preserved the first row's ciphertext digest;
- the repaired batch converted both rows to v2 (2 modified, 0 remaining);
- a restarted v2-only backend decrypted the rotated credential before the
  expected synthetic Kubernetes connection failure; and
- all database/backend containers, the private network, temporary image and
  process environment were cleaned.

The complete local release gate passed at 2026-07-28 14:11 +08:00 in 288.9
seconds. Evidence is
`.artifacts/verification/verify-20260728-141111.json`: 163 Go `Test*` entries,
all backend packages, both backend binaries, 14 Vitest files / 59 tests,
frontend production build, three healthy Compose services, Kustomize
16/5/22/3 and backend/frontend/proxy HTTP checks passed.

Hosted CI run `30334216631` passed all four jobs at revision `151bc7e`.
Backend formatting/vet/test and both binaries, frontend typecheck/test/build,
manifest/Compose rendering, the isolated credential re-encryption drill, the
independent PostgreSQL restore drill, random-production-config Compose health,
HTTP checks, sanitized evidence upload and unconditional teardown all
succeeded on Ubuntu 24.04.

## Security Boundary

No HTTP rotation endpoint, automatic schedule, KMS/HSM integration or key
database is introduced. Keys, plaintext kubeconfigs, ciphertext, database URLs
and raw errors do not enter command JSON, audit rows or retained evidence.
Operators must preserve active-plus-legacy overlap until every row and replica
has been verified, and must pair database recovery with separately protected
historical keys.

## Next Route

Continue M20 production hardening with OIDC/MFA evaluation, then signed audit
archives, production backup retention/PITR and HA/failover validation. Registry
identity, artifact signing and a formal release tag remain release-governance
decisions.
