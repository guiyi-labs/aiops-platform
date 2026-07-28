# Production Recovery Readiness Runbook

This runbook turns infrastructure-owner decisions and the latest logical
restore drill into a reproducible admission report. It does not run PITR or HA
and cannot approve production disaster recovery.

## Required Decisions

Service, database and security owners must approve:

- data classification, RPO, RTO and maximum tolerable downtime;
- backup interval, retention, immutable period, copy/region count, encryption,
  storage identity and rotation;
- whether WAL/PITR is required and its archive interval/recovery window;
- whether HA is required and its topology, replica count, fencing, failover
  owner and maximum failover time;
- drill intervals, representative-data criteria, isolated restore target,
  cutover writer freeze, validation ownership and rollback plan.

Start from `docs/database/recovery-readiness-policy.example.json`. Every
`REQUIRED_*` marker and zero objective is unresolved. Never put object-storage
keys, database URLs, passwords, dumps, WAL segments or customer data in this
file or Git.

When PITR or HA is not yet approved, the policy requires a named risk owner and
an RFC3339 expiry no more than 180 days away. Without PITR, the full-backup
interval must itself satisfy RPO. An expired acceptance fails closed.

## Produce Logical Evidence

Run the isolated logical restore first:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-postgres-backup-restore.ps1
```

The result is versioned `aiops.logical-restore-evidence/v1`. It contains only
sanitized counts, digest and cleanup state; the actual dump is deleted.

## Evaluate

Use the production image with networking disabled and read-only inputs:

```powershell
docker run --rm --network none `
  --mount type=bind,source=C:\secure\recovery-review,target=/review,readonly `
  --entrypoint /app/recovery-readiness aiops-platform-backend:reviewed `
  --policy-file=/review/policy.json `
  --logical-restore-evidence=/review/postgres-recovery.json
```

Exit code `0`, `ready_for_pitr_ha_implementation: true` and
`production_recovery_validated: false` are the expected pre-implementation
result. Exit code `1` means policy/evidence is malformed, stale or fails a
control. Exit code `2` means required arguments are missing.

The repository acceptance gate consumes the newest real logical evidence:

```powershell
.\scripts\e2e-recovery-readiness.ps1
```

It accepts 15 checks, rejects inadequate copies, stale evidence, retained dump
material and incomplete cleanup, removes its image/copied inputs and retains
only sanitized booleans/counts under `.artifacts/recovery-readiness`.

## Production Validation Boundary

Before claiming recovery readiness, a later phase must test production-size
data and approved infrastructure: object-storage retention/immutability, WAL
continuity and a selected timestamp restore, Kubernetes storage replacement,
writer fencing, failover and failback, split-brain prevention, application
validation and measured RPO/RTO. Restore and failover targets must remain
isolated from active production until an explicit cutover review.
