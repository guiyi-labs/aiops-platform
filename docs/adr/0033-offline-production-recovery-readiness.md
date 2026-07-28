# ADR 0033: Offline Production Recovery Readiness Admission

- Status: Accepted
- Date: 2026-07-28
- Milestone: M20 Phase 12

## Context

The isolated PostgreSQL logical restore proves archive compatibility, but it
does not define production retention, RPO/RTO, WAL archival, HA topology,
failover ownership or cutover behavior. Those are organizational and
infrastructure decisions. Treating the existing logical drill as production
disaster-recovery acceptance would hide material gaps.

## Decision

Add `/app/recovery-readiness` as an offline admission command. It strictly reads
an approved recovery policy and sanitized logical-restore evidence, each
bounded to 1 MiB. Unknown fields and trailing data fail closed. The policy
contains no storage credential, database URL, password, backup payload or WAL
material.

The gate evaluates 15 controls covering:

- accountable service, database, security and incident owners;
- explicit RPO, RTO and maximum tolerable downtime;
- encrypted off-cluster storage, retention, immutability and independent copies;
- bounded full-backup frequency, verification, least privilege and credential
  rotation;
- PITR archive frequency, recovery window, encryption, gap monitoring and
  isolated restore, or a named risk acceptance expiring within 180 days;
- HA topology, replicas, automatic failover, ownership, writer fencing and an
  RTO-bound failover target, or a similarly bounded risk acceptance;
- isolated representative logical/PITR/failover drills, cutover ownership,
  writer freeze, rollback and approvals;
- fresh logical evidence proving a non-empty custom archive, source destruction,
  matching source/restored snapshots, zero invalid foreign keys, no retained
  dump and complete cleanup.

The output uses `ready_for_pitr_ha_implementation`. It always returns
`production_recovery_validated: false`; this gate cannot be used to claim that
production-size PITR, failover, storage durability or measured RPO/RTO passed.
The policy must explicitly retain that future validation requirement.

The PostgreSQL drill now versions its sanitized evidence as
`aiops.logical-restore-evidence/v1`. CI runs the recovery gate after producing a
fresh logical restore result, with the command container network disabled.

## Consequences

- Infrastructure owners can review one explicit implementation contract before
  selecting storage, operators or HA technology.
- A passing report means policy and logical evidence are sufficient to begin
  PITR/HA implementation. It is not production disaster-recovery acceptance.
- Risk acceptance is visible, named and time bounded; it cannot silently replace
  PITR or HA forever.
- The next phase must use isolated physical/WAL fixtures and a disposable HA
  topology, then measure recovery against approved objectives before changing
  the production validation flag in any later evidence format.
