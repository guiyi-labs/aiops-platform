# KRM And Ratel Product Gap Analysis

- Status: Accepted
- Reviewed: 2026-07-28
- Compared baseline: `b1f52e098ca2c6a44891f5e83fbed66e43a651af`
- KRM material: `<local-refs>/krm-main`
- Ratel material: `<local-refs>/ratel-doc-master`

> Superseded for post-M25 planning: use
> [`final-product-gap-analysis.md`](final-product-gap-analysis.md) for the current KRM/Ratel/KubeSphere/
> Kubernetes Guide comparison, M27-M32 route and project-end criteria. This file remains the accepted historical
> record that drove M21-M25.

## Evidence Boundary

The local KRM material contains a README, deployment instructions and images,
but not the application source used by its published image. The local Ratel
material is a documentation and screenshot archive, and its README explicitly
says the project is no longer maintained. Their capabilities below are
therefore documented claims or documented workflows, not newly reproduced
runtime results.

`aiops-platform` is compared from its current source, routes, tests, retained
screenshots and accepted M1-M20 records. A missing generic feature is not
automatically a defect when it would weaken least privilege, evidence capture
or change safety.

## Capability Comparison

| Area | aiops-platform | KRM material | Ratel material | Assessment |
|---|---|---|---|---|
| Multi-cluster access | Encrypted bounded kubeconfig onboarding, health conditions, credential replacement, fault-isolated fleet view and search | Documents non-invasive cluster addition and multi-cluster statistics | Documents Secret-backed cluster configuration and hot reload | Current platform is stronger on credential safety and failure semantics |
| Resource inventory | 17 fixed resource kinds with typed list/detail/event contracts and safe field projection | Claims common resources plus arbitrary core/CRD browsing | Documents classic workload, networking, configuration, storage and account resources | Current fixed coverage is credible but lacks several governance resources and generic discovery |
| Troubleshooting | Current/previous bounded Pod logs, events, metrics, topology and seven deterministic diagnosis rules | Shows topology, node/Pod operations and container file workflows | RBAC/docs include Pod logs and exec-style workflows | Current diagnosis is stronger; multi-container log UX and incident history remain incomplete |
| Resource mutation | Four fixed actions with server-side dry-run, typed diff, one-time confirmation, idempotency and audit | Claims graphical create/edit, update, rollback and one-click scheduling operations | Documents create/edit for workloads, networking, configuration and storage | Current platform is much safer but visibly narrower for daily release work |
| Cross-cluster delivery | Bounded health and fixed-kind search only; no mutation or copy | Claims cross-cluster copy and project migration | Documents Namespace/Deployment copy workflows | This is a material product gap |
| Backup and restore | Strong platform-database logical recovery evidence and recovery-policy admission; no cluster-workload backup console | Claims Velero backup and restore management | No comparable current maintained workflow in the local archive | Cluster workload protection is a material gap; platform DB recovery is a different concern |
| Identity and audit | Platform users/roles, session revocation, encrypted credentials, append-only audit, signed webhook/outbox and signed offline archive | Deployment example uses environment-based fixed account hashes | Includes obsolete Basic auth and broad ServiceAccount examples | Current platform is materially stronger |
| Observability history | Real Node/Pod point-in-time metrics and bounded fleet summaries; no retained time series or alert-window evaluation | Shows aggregate resource statistics | Shows basic cluster/resource lists | Historical evidence is the largest gap relative to the AIOps product position, even though neither reference proves a stronger implementation |
| Engineering evidence | Buildable Go/Vue source, migration tests, real-kind suites, Compose/Kustomize gates and hosted CI | Local material does not permit source-level verification | Documentation project is stopped | Current platform is materially stronger |
| Interface | Quiet task-oriented console with deep links, topology, event/diagnosis/audit views; the resource workbench is dense and action-light | Broad resource navigation and visible action entry points | Broad but dated navigation and table layout | Preserve current visual system; improve information architecture and action discoverability rather than imitate either theme |

## Missing Capabilities Worth Building

### Priority 0: Product-Defining Gaps

1. Bounded historical metrics, missing-sample semantics and alert-window
   evaluation. Without trends, the platform is reactive rather than AIOps.
2. Safe Deployment release lifecycle: rollout status/history, exact image
   update preview and rollback to an immutable ReplicaSet revision.
3. Fixed cross-cluster promotion with target preflight, reference mapping,
   server-side dry-run, conflict handling, typed diff and audit.

### Priority 1: Daily Operations Completeness

1. Explicit multi-container log selection, current/previous mode, bounded
   tail/since controls, timestamps, search and safe download.
2. Fixed read-only coverage for PersistentVolume, PodDisruptionBudget,
   NetworkPolicy, ServiceAccount, Role/ClusterRole and binding metadata.
3. Redacted server-produced manifest inspection for approved non-sensitive
   kinds. Secret/ConfigMap values and StorageClass parameters remain excluded.
4. Optional Velero capability detection, backup inventory and controlled backup
   creation before any restore action is considered.

### Priority 2: Organization-Gated Work

1. Real OIDC login, MFA evidence validation and account linking after approval
   of the accepted Phase 11 policy.
2. Physical/WAL PITR and disposable HA failover/failback after approval of the
   accepted Phase 12 recovery policy.
3. Registry publication, artifact signing identity, protected-branch policy and
   the dedicated real-kind runner.

## Explicit Non-Goals

- Do not add an arbitrary Kubernetes API proxy, generic YAML editor or generic
  CRD mutation merely to match a feature count.
- Do not return Secret values, ConfigMap values, ServiceAccount tokens,
  StorageClass parameters or credentials.
- Do not add unrestricted Pod exec, browser file upload/download, bulk project
  migration or one-click restore. An audited break-glass terminal requires a
  separate threat model and is not on the committed M21-M26 route.
- Do not call logical database restore evidence production PITR/HA, and do not
  call a policy admission result production recovery validation.

## Roadmap Effect

M20 closes after Phase 12. Product-visible operational workflows now precede
further provider-specific hardening: M21 historical observability, M22 daily
troubleshooting and governance coverage, M23 safe Deployment release lifecycle,
M24 fixed cross-cluster promotion and M25 cluster-workload backup integration.
M26 converges organization-approved identity/recovery work and formal release
readiness without blocking M21-M25 on unavailable provider decisions.
