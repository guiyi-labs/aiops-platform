# M20 Phase 8: Isolated PostgreSQL Backup And Restore

- Date: 2026-07-28
- Status: Accepted
- Accepted revision: `24ed4af7b74ec85438c0c8cc005f27ecf6e74886`
- Hosted CI: [run 30331048635](https://github.com/guiyi-labs/aiops-platform/actions/runs/30331048635)
- Scope: repeatable logical backup, fresh-instance restore, invariants and cleanup

## Delivered

- Added ADR 0029 and `docs/database/backup-restore.md` to define the automated
  drill, production boundary and future RPO/RTO/PITR work.
- Added `scripts/e2e-postgres-backup-restore.ps1` using the pinned
  `pgvector/pgvector:0.8.1-pg17` image.
- Applied all 15 current migrations to an isolated source database and seeded
  synthetic identity, RBAC, cluster credential, diagnosis, audit and saved
  filter relationships.
- Exported a compressed custom-format archive, destroyed the source container,
  restored into a fresh target container and compared source/target invariants.
- Added unconditional cleanup for source/target containers, anonymous volumes,
  system-temporary backup/seed files and process-scoped PostgreSQL credentials.
- Added `.dump` and `.backup` ignore rules and kept sanitized evidence under the
  already ignored `.artifacts` root.
- Added the drill to the hosted `Compose runtime` job before the independent
  application runtime starts; the JSON evidence joins the short-lived CI
  artifact without containing credentials or row contents.

## Local Verification

The isolated drill passed on Docker Engine 29.6.1 with PostgreSQL 17:

- 15 migrations restored, latest
  `000015_saved_global_search_filters.up.sql`;
- roles/users/user roles/clusters/credential blobs/diagnosis/audit/saved filter
  counts matched before and after restore;
- invalid foreign-key count was zero;
- the latest 81,270-byte custom archive was recognized by `pg_restore --list`;
- the source was destroyed before restore; and
- both containers, the temporary backup and process credentials were cleaned.

Sanitized local evidence:
`.artifacts/postgres-recovery/postgres-recovery-20260728-131325.json`.

The complete local quality gate also passed in 278.81 seconds with all backend
packages, 14 Vitest files / 59 tests, the production frontend build, three
healthy Compose services, Kustomize resource counts 16/5/22/3 and backend,
frontend and proxy health checks. Evidence:
`.artifacts/verification/verify-20260728-125500.json`. Actionlint 1.7.7 returned
zero findings after correcting the pre-existing release checksum glob warning.

A controlled negative run with `-ReadyTimeoutSeconds 0` failed as expected and
still left zero drill-owned containers and temporary directories while
preserving the caller's PostgreSQL environment.

Hosted CI run `30331048635` passed Backend, Frontend, Manifests and Compose
runtime at revision `24ed4af7b74ec85438c0c8cc005f27ecf6e74886`. The Ubuntu
PowerShell recovery step completed source migration/backup/destruction,
fresh-target restore, invariant verification and cleanup before the independent
Compose runtime was built and health checked. The first hosted attempt exposed
an unset-versus-empty environment comparison difference on Linux; revision
`24ed4af` normalized that comparison while preserving exact non-empty values.

## Not Claimed

This phase does not claim a production backup schedule, off-cluster retention,
WAL/PITR, production-size RPO/RTO, Kubernetes snapshot recovery, cross-region
replication or HA failover. It also does not decrypt restored cluster
credentials; exact synthetic encrypted bytes are compared without exposing
their contents.
