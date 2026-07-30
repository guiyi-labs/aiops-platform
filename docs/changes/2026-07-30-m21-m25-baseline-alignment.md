# M21-M25 Baseline Alignment

- Status: Accepted locally
- Date: 2026-07-30
- Scope: integration review, correctness hardening, real-kind acceptance,
  repository gates and release-candidate documentation

## Reviewed Baseline

The audit reviewed all uncommitted M21-M25 backend, frontend, migration, RBAC,
OpenAPI, CI and acceptance assets as one release candidate. Public contracts
remain fixed and typed; no arbitrary Kubernetes mutation or expression
language was introduced.

Key corrections include complete PodTemplate rollback, explicit GORM mappings
for migration 18, clean operation-history queries, bundle-level promotion
dependency deduplication, mapped dependency rewrites, Service runtime-field
stripping, per-item promotion persistence, response manifest hiding,
Namespace-scoped write RBAC, Windows PowerShell 5 native-command handling and
M25 CRD establishment ordering.

## Acceptance Ledger

| Area | Evidence | Result |
|---|---|---|
| M21 history and sustained windows | `.artifacts/m21-history-kind/m21-history-kind-20260730-080558.json` | Passed real Metrics API collection, sparse outage, recovery, deterministic three-state evaluation, PostgreSQL restart durability and cleanup |
| M22 troubleshooting workbench | `.artifacts/verification/verify-20260730-080851.json` | Backend/frontend/production-build and delivery contracts passed |
| M23 release lifecycle | `.artifacts/m23-release-lifecycle-kind/m23-release-lifecycle-kind-20260729-234238.json` | Image update, exact revision rollback, idempotency, restoration and cleanup passed |
| M24 cross-cluster promotion | `.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-074812.json` | Two items, one deduplicated dependency, mapped reference, idempotency and cleanup passed |
| M25 workload protection | `.artifacts/m25-workload-protection-kind/m25-workload-protection-kind-20260730-075311.json` | Velero installed/unavailable paths, two backups, 424 fallback, read-only RBAC and cleanup passed |
| Fast gate | `scripts/verify-fast.ps1` | Passed in 23.73 seconds after documentation alignment |
| Full gate | `.artifacts/verification/verify-20260730-080851.json` | Passed in 121.79 seconds; all three Compose services healthy |

Generated evidence remains ignored. It is referenced by path for local audit
and must be uploaded only through the sanitized CI artifact contract.

## Release Boundary

The baseline does not claim production OIDC/MFA, PITR/HA, cross-cluster
rollback, workload restore, arbitrary CRD promotion, arbitrary metrics queries
or unrestricted Kubernetes write access. M26 provider decisions and formal
release work remain gated as recorded in `docs/roadmap.md`.
