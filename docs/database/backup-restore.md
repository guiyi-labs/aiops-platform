# PostgreSQL Backup And Restore

## Automated Isolated Drill

Run the repository gate from the project root:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-postgres-backup-restore.ps1
```

PowerShell 7 can run the script directly:

```powershell
pwsh ./scripts/e2e-postgres-backup-restore.ps1
```

The drill does not connect to `localhost:15432`, the Compose project, a
Kubernetes Service or an existing Docker volume. It creates two unique
containers without host ports. The source is deleted before a fresh target is
started, so successful verification cannot accidentally read the source data
directory.

Successful evidence is written to `.artifacts/postgres-recovery` and contains:

- PostgreSQL image and archive format;
- backup byte size and SHA-256 digest;
- applied migration count and latest migration name;
- table-level synthetic row counts and foreign-key consistency; and
- source/target/temp-file/environment cleanup assertions.

The actual dump is always deleted and is never copied into the evidence
directory. The evidence intentionally omits passwords, password hashes,
cluster credential bytes, API servers and business row contents.

## Failure Handling

The script performs cleanup in `finally`. A failed migration, dump, archive
parse, restore or invariant check still removes both containers and their
anonymous data volumes, deletes the system-temporary working directory and
restores the caller's PostgreSQL process environment.

If a run is interrupted by terminating the PowerShell process or Docker
daemon, identify only drill-owned containers before removal:

```powershell
docker ps -a --filter label=io.guiyi.aiops.purpose=postgres-recovery-drill
```

Do not remove Compose containers or named volumes as part of drill cleanup.

## Production Runbook Boundary

The automated drill proves logical compatibility, not a production backup
service. Before production use, an operator-approved design must additionally
define:

- encrypted off-cluster storage, retention and immutability;
- backup identity, least privilege and credential rotation;
- scheduled full backups and, if required, WAL archiving/PITR;
- restore into an isolated validation namespace before promotion;
- RPO/RTO targets measured with production-size data;
- Kubernetes StatefulSet/PVC replacement and application cutover; and
- monitoring for backup age, size anomalies and restore rehearsal failures.

Never rehearse by dropping the active `aiops` database. Restore to a new
instance/database, validate it, stop writers, take a final backup, and use an
explicitly reviewed cutover plan. A Kubernetes PVC snapshot is not a substitute
for a tested logical backup, and a logical backup is not a substitute for HA or
point-in-time recovery.
