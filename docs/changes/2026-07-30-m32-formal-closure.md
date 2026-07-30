# M32: Formal Closure And Final Local Revalidation

- Initial closure: 2026-07-30
- Final revalidation: 2026-07-31
- Status: Local development baseline accepted; external production gates remain explicit
- ADRs: 0043-0047

## Outcome

M27-M31 are implemented against their fixed contracts and now have disposable
real-environment acceptance evidence. The repository-level fast/full gates,
route/OpenAPI parity, migration parity, RBAC bounds, PowerShell syntax,
responsive browser workflow and cleanup checks are green. The final local
archive is bound by tag `baseline-m32-20260731`; no push or remote release is
part of this closure.

## Accepted Evidence

| Gate | Result | Evidence |
|---|---|---|
| Fast repository gate | Passed in 26.17s | `scripts/verify-fast.ps1 -Scope All` |
| Full repository gate | Passed in 97.68s; backend/proxy ready, frontend 200, three healthy services | `.artifacts/verification/verify-20260731-015255.json` |
| M27 alert lifecycle | Passed firing, deduplication, outage containment, complete normal-window resolution, restart durability and cleanup | `.artifacts/m27-alert-lifecycle-kind/m27-alert-lifecycle-kind-20260731013733-e4a6e270.json` |
| M28 Backup creation | Passed pinned Velero/MinIO creation, stale-source rejection, replay, RBAC and cleanup | `.artifacts/m28-backup-creation-kind/summary.json` |
| M29 Namespace posture | Passed deterministic critical findings and cleanup | `.artifacts/m29-governance-posture-kind/summary.json` |
| M30 Node maintenance | Passed two-worker cordon/replay, safe eviction, blocker rejection, uncordon and cleanup | `.artifacts/m30-node-maintenance-kind/summary.json` |
| M31 isolated restore | Passed pinned Velero/MinIO quarantine restore, mapping, stale-source rejection, replay and cleanup | `.artifacts/m31-isolated-restore-kind/summary.json` |
| Responsive UI | Passed at 390x844 and 1280x720; no page overflow, title/button overflow or browser warning/error | In-app browser acceptance on Namespace governance view |

Pinned M28/M31 environment: Velero v1.15.2,
`velero/velero-plugin-for-aws:v1.11.1` and disposable MinIO. The downloaded
Velero Windows package SHA-256 was
`1FA7C2448A5751DD3FDFD86AD9C49472D677B97237A25390E7727088ED82D668`.

## Closure Audit

- 24 migration up/down pairs exist; migration 000024 is applied in the
  development PostgreSQL instance.
- `TestRegisteredRoutesMatchOpenAPI` passes with all conditional services.
- Compose config and all four Kustomize roots render successfully (16/5/22/3).
- All 27 PowerShell scripts parse with zero AST errors.
- Managed-cluster RBAC permits only reviewed mutations: Node patch,
  Pod eviction create, fixed Backup/Restore/quarantine creates; generic
  update/delete and unrelated writes remain denied.
- `git diff --check` passes.
- No kind cluster, temporary registration, kubeconfig, MinIO credential file
  or controlled browser tab remains after acceptance.
- `go test -race` is not claimed: this Windows environment has no `gcc`, so
  the race gate is recorded as environment-blocked rather than passed.

## Material Fixes Found During Revalidation

- Alert scheduling now evaluates a recent bounded window with collection-jitter
  slack instead of a fixed six-hour query that delayed recovery.
- M27 registration now uses a short-lived, least-privilege, container-reachable
  ServiceAccount kubeconfig and waits for the Metrics API, not only its
  Deployment.
- M28 persists PostgreSQL `text[]` with the standard pq array contract,
  captures exact Namespace/Backup identities and returns stale-source HTTP 409.
- M30 recognizes empty-value control-plane/master label keys and uses real
  Pod volume/PDB selector evidence.
- M31 uses feasible dry-run locations while preserving the strict execute
  order: Namespace, quarantine controls, Restore.
- Managed-cluster RBAC includes exact BSL reads and reviewed maintenance,
  Backup and restore mutations; the security test verifies the allowlist.
- Shared console panel/table styles and mobile layout prevent the governance
  view from collapsing or causing page-level overflow.

## External Gates

The local baseline is complete, but it does not claim production readiness.
The following remain organization-owned prerequisites:

- hosted CI on the archived revision and any remote release publication;
- organization-approved OIDC/MFA integration;
- physical/WAL PITR with measured RPO/RTO;
- HA failover and failback drills.

These items require new authority or infrastructure. They do not invalidate
the local development baseline and must not be described as completed.

## Final Declaration

M32 is locally complete. M27-M31 have executable real-environment evidence,
the repository gates are green, the UI workflow was revalidated, and the
workspace is suitable for archival. The authoritative archive record is
`docs/changes/2026-07-31-final-baseline-archive.md`.
