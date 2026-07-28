# M20 Phase 12: Production Recovery Readiness Gate

- Date: 2026-07-28
- Status: Accepted
- Accepted revision: `0baf8583956e1e987ef5043b5fd70ce33aba90e4`
- Hosted CI: [run 30348664880](https://github.com/guiyi-labs/aiops-platform/actions/runs/30348664880)
- Scope: explicit recovery objectives, policy consistency and logical-restore evidence admission

## Delivered

- Added ADR 0033, the recovery readiness runbook and an intentionally unresolved
  policy template.
- Added `/app/recovery-readiness`, which strictly reads bounded policy and
  logical-restore evidence files without network or database access.
- Added 15 fail-closed checks covering owners, RPO/RTO/MTD, encrypted
  off-cluster immutable copies, backup scheduling, PITR, HA/risk acceptance,
  drills, cutover/rollback, approvals, evidence age, logical restore, digest and
  snapshot integrity, cleanup and the mandatory production-validation boundary.
- Added bounded 180-day risk acceptance for explicitly deferred PITR/HA. It is
  named and expiring, and a no-PITR full backup must still satisfy RPO.
- Versioned the existing sanitized logical drill evidence and made the recovery
  gate consume the newest real result after the PostgreSQL restore CI step.
- Added production-image downgrade testing with networking disabled. Only
  sanitized booleans/counts are retained.

## Verification

The versioned logical drill passed against PostgreSQL 17 with all 16 migrations,
source destruction before fresh-target restore, matching source/restored
snapshots, zero invalid foreign keys and all four cleanup assertions. Evidence
is `.artifacts/postgres-recovery/postgres-recovery-20260728-174419.json`.

The network-disabled production-image readiness gate passed all 15 controls,
rejected inadequate copy count, stale evidence, retained dump material and
incomplete cleanup, and removed its temporary image and copied inputs. Evidence
is `.artifacts/recovery-readiness/recovery-readiness-20260728-174509.json`.

Actionlint 1.7.7 returned zero findings. The 199.35-second full local gate used
the Go 1.25 Docker toolchain and passed vet, all packages and 175 Go `Test*`
entries, all five backend build targets, 14 Vitest files / 59 tests, frontend
production build, three healthy Compose services, Kustomize 16/5/22/3 and
direct/proxied HTTP readiness. Evidence is
`.artifacts/verification/verify-20260728-175233.json`.

Hosted CI run `30348664880` passed all four jobs for revision `0baf858`. The
Ubuntu Compose job accepted the network-disabled recovery contract after a
fresh PostgreSQL logical restore, then passed random-production-config Compose
health, direct/proxied HTTP checks, sanitized artifact upload and unconditional
teardown.

## Boundary

The accepted report is named `ready_for_pitr_ha_implementation` and always says
`production_recovery_validated: false`. This phase does not select a cloud
storage service, create a WAL archive, restore to a point in time, operate an HA
cluster, replace a PVC, fail over traffic or measure production RPO/RTO.

## Next Step

After infrastructure-owner approval, implement an isolated PostgreSQL physical
backup/WAL archive and selected-timestamp PITR drill. Then create a disposable
HA topology to validate writer fencing, failover/failback, split-brain
prevention and measured objectives before any production recovery claim.
