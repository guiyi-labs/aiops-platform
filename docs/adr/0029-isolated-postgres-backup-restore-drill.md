# ADR 0029: Isolated PostgreSQL Backup And Restore Drill

- Status: Accepted
- Date: 2026-07-28
- Milestone: M20 Phase 8

## Context

The platform stores authentication, encrypted cluster credentials, diagnosis
history, audit records, notification state, controlled-operation plans and
saved search filters in PostgreSQL. A successful `pg_dump` command alone does
not prove that an archive can be restored into a fresh PostgreSQL instance or
that the current migration state and representative relationships survive.

Running a destructive restore against the retained Compose database would put
development and demonstration data at risk. A useful automated gate therefore
needs to exercise actual PostgreSQL tools while remaining physically isolated
from every retained database and volume.

## Decision

Add `scripts/e2e-postgres-backup-restore.ps1` as a bounded disaster-recovery
drill using the same PostgreSQL 17 pgvector image as Compose and Kubernetes.
The drill:

1. starts a uniquely named source container without a host port;
2. applies every sorted `*.up.sql` migration and records the same migration
   names used by the application migrator;
3. inserts synthetic rows across identity, RBAC, cluster credential,
   diagnosis, audit and saved-filter relationships;
4. creates a custom-format logical backup with `pg_dump`;
5. destroys the source instance before starting a fresh target instance;
6. restores with `pg_restore --exit-on-error` and verifies migration, row,
   encrypted-byte and foreign-key invariants; and
7. removes both containers, anonymous volumes, the backup, seed SQL and
   temporary process credentials in `finally`.

The script writes only sanitized JSON evidence under the ignored
`.artifacts/postgres-recovery` directory. Database passwords and row contents
must not enter logs or evidence. `*.dump` and `*.backup` are ignored globally.

The regular hosted CI runtime job runs this drill before creating the separate
Compose runtime. This keeps recovery verification independent from the
retained development database and from the CI application's data volume.

## Consequences

- Every accepted revision proves logical backup compatibility with its exact
  migration set and PostgreSQL major version.
- The gate detects archives that cannot be listed/restored, missing migrations,
  lost representative data, changed encrypted bytes and broken relationships.
- The drill does not establish a production RPO or RTO, test object-storage
  retention, validate Kubernetes volume snapshots, or restore a real customer
  dataset. Those require production infrastructure and approved credentials.
- Physical/PITR backup, HA failover and application encryption-key rotation
  remain separate production-hardening decisions.
