# M20 Phase 10: Signed Audit Archives

- Date: 2026-07-28
- Status: Implemented; hosted CI acceptance pending
- Accepted revision: pending
- Hosted CI: pending
- Scope: bounded offline audit archival, external trust verification and tamper evidence

## Delivered

- Added ADR 0031 and `docs/database/audit-archive.md`.
- Added `/app/audit-archive` creation, verification and public-key derivation
  modes without adding an HTTP endpoint.
- Added inclusive ID-range selection under a read-only repeatable-read snapshot.
  Candidate count is checked against an explicit 1..10000 bound before records
  are loaded or files are written.
- Added a canonical JSON payload using the existing sanitized `audit.Entry`
  model and a detached, Ed25519-signed manifest containing format, SHA-256,
  count, ID range, UTC time, tool version and public key.
- Verification requires an externally supplied trusted public key and rejects
  an embedded-key mismatch, bad signature, digest mismatch, malformed payload,
  metadata mismatch or non-ascending records.
- Output is explicit, non-overwriting and owner-only where supported. The
  command, local gate, production image, regular CI runtime job and sanitized
  artifact upload now move together under delivery contract tests.

## Verification

Targeted Go 1.25 container tests cover trusted verification, payload and
manifest tampering, untrusted signer rejection and overwrite refusal. Audit,
command and delivery contract packages pass.

The final isolated PostgreSQL run passed at 2026-07-28 15:40 +08:00. Evidence is
`.artifacts/audit-archive/audit-archive-20260728-154047.json`: two synthetic
sanitized rows were archived and verified; a three-row candidate was rejected
against `max-records=2` with neither output file created; a one-byte mutation
failed verification; and the database container, private network, temporary
image, key/archive directory and process environment were all removed.

The complete local gate passed at 2026-07-28 15:30 +08:00 in 361.34 seconds.
Evidence is `.artifacts/verification/verify-20260728-153059.json`: 167 Go
`Test*` entries and all backend packages, all three backend binaries, 14 Vitest
files / 59 tests, frontend production build, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend-proxy HTTP checks passed. Actionlint
1.7.7 returned zero findings. Hosted CI acceptance pending.

The first two hosted attempts correctly failed rather than overstating
acceptance. Run `30338972042` exposed Linux PowerShell null/empty normalization
in the environment-restoration assertion. After that correction, run
`30339580960` exposed that the image's non-root application UID could not write
the runner-owned bind mount. Both attempts removed their containers, network,
image and temporary files. The gate now maps the Linux command container to the
non-root runner UID/GID while retaining the image's default application user in
normal operation; the replacement hosted run is pending.

## Security Boundary

No HTTP endpoint, automatic schedule, key database, archive upload, row deletion
or retention engine is introduced. The embedded public key is informational,
not trusted. Private keys and archives remain outside Git and CI artifacts.
Signatures do not prove source-database completeness before export and do not
replace encrypted WORM storage, key custody, PostgreSQL backup/PITR or legal
records policy.

## Next Route

Complete hosted acceptance for this phase. Then document organization-specific
OIDC/MFA requirements and production backup retention/RPO/RTO before selecting
an identity provider or implementing PITR/HA drills.
