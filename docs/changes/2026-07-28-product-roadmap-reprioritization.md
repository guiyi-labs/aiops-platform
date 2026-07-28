# Product Roadmap Reprioritization After KRM/Ratel Review

- Date: 2026-07-28
- Status: Accepted
- Baseline: `b1f52e098ca2c6a44891f5e83fbed66e43a651af`
- Accepted revision: `5cfbf694d52bc114ff8ee567525a290d4b85e4b0`
- Hosted CI: [run 30351531959](https://github.com/guiyi-labs/aiops-platform/actions/runs/30351531959)
- Scope: post-M20 competitive gap review and M21-M26 sequencing

## Evidence Reviewed

- Current frontend routes, backend API routes, accepted M1-M20 records, local
  evidence index and hosted CI baseline.
- KRM README, deployment instructions and screenshots in the local reference
  archive. The local material does not include buildable application source.
- Ratel resource/account/cluster documentation and screenshots. Its README says
  the project is no longer maintained.
- The prior KubeSphere architecture analysis, used only as an engineering and
  platform-boundary reference.

The complete comparison is archived in
`docs/references/krm-ratel-gap-analysis.md`.

## Decision

Close M20 after the accepted Phase 12 recovery-readiness gate. Stop making
provider-specific OIDC/PITR/HA work the only next path because the necessary
organization decisions are still unavailable and those tasks do not close the
largest user-visible gaps.

Adopt the following product sequence:

1. M21: bounded historical observability and alert-window evidence;
2. M22: daily troubleshooting and governance workbench completeness;
3. M23: safe Deployment release history, image update and exact rollback;
4. M24: fixed, dependency-aware cross-cluster promotion;
5. M25: optional cluster-workload backup inventory and controlled creation;
6. M26: organization-approved identity/recovery integration and formal release.

Each mutation phase must retain the existing server-side dry-run, typed diff,
one-time confirmation, idempotency, target precondition, least-privilege RBAC,
audit and real-cluster restoration requirements.

## Why This Is Not Feature-Count Parity

KRM and Ratel make broad resource editing, copy and terminal-style operations
visible, but their local materials do not provide the source/runtime evidence
needed to adopt those security models. `aiops-platform` will close high-value
workflow gaps with fixed contracts. Generic YAML/CRD mutation, sensitive-value
display, unrestricted exec/file transfer, bulk project migration and one-click
restore remain outside the committed route.

## First Implementation Entry

M21 begins with a bounded metrics-history contract: explicit retention,
cluster/series/point/query caps, missing samples represented as missing rather
than zero, deterministic window evaluation, access-scoped query predicates and
retention cleanup evidence. UI work follows the existing quiet console and adds
trend/coverage context without replacing it with competitor styling.
