# Final Product Gap Analysis And Closure Route

- Status: Superseded for post-M32 work by `../kubesphere-optimization-plan.md`
- Reviewed: 2026-07-30
- Compared baseline: `main` at `17d4f6f`, accepted M21-M25 implementation
- Execution contract: [`../next-development-plan.md`](../next-development-plan.md)
- Product roadmap: [`../roadmap.md`](../roadmap.md)

> This document remains the historical decision record for M27-M32. Those milestones are complete;
> current optimization scope and acceptance criteria are defined in `../kubesphere-optimization-plan.md`.

本文回答三个问题：当前平台与本机可用的参考项目相比还缺什么，哪些差距值得补齐，
以及做到什么程度可以正式结束本轮项目。它取代早期
[`krm-ratel-gap-analysis.md`](krm-ratel-gap-analysis.md) 对 M25 之后工作的判断；旧文档保留为
M21-M25 决策历史。

## 1. Evidence Boundary

| Material | Local evidence | Evidence level | Permitted conclusion |
|---|---|---|---|
| `aiops-platform` | Source, tests, migrations, API contract, retained screenshots and accepted real-kind evidence | Source and runtime verified | May describe implemented behavior within the exact accepted test boundary |
| KRM | 10 local files: README, deployment document and screenshots; no application source | Documentation claim only | May identify advertised workflows, but not implementation quality, safety or current runtime behavior |
| Ratel | 86 documentation/screenshot files; README states that the project is no longer maintained | Historical documentation only | May identify operator workflow demand, but must not copy obsolete APIs, authentication or broad RBAC |
| KubeSphere | Previously accepted architecture analysis; its source is not present on this device | Accepted secondary analysis | May reuse already recorded patterns, but must not add new source/runtime claims |
| Kubernetes Guide | Teaching chapters and manifests, not an operations product | Domain coverage checklist | May reveal Kubernetes operational topics, but not prove a competing product capability |

Comparison labels used below:

- **Implemented** means the current repository and accepted evidence support the exact statement.
- **Documented** means a reference README or guide describes the workflow; it was not reproduced here.
- **Derived gap** means the need follows from operator workflow and domain coverage, not feature-count parity.
- **Rejected** means the capability conflicts with the platform's least-privilege and evidence-based product boundary.

No reference document is evidence that a feature is secure, production-ready or compatible with the current
Kubernetes version. A missing generic feature is not automatically a defect.

Local material inspected in this review:

- KRM: `C:\BS\krm-main\krm-main`
- Ratel: `C:\BS\ratel-doc-master\ratel-doc-master`
- Kubernetes Guide: `C:\BS\kubernetes-guide-main\kubernetes-guide-main`
- KubeSphere: no source checkout is present; only the accepted repository analysis is reused

## 2. Comparison Summary

| Area | Current platform after M25 | Reference signal | Decision |
|---|---|---|---|
| Multi-cluster access | Encrypted bounded onboarding, health conditions, fault-isolated fleet queries and fixed search | KRM and Ratel document cluster addition and cross-cluster workflows; KubeSphere analysis emphasizes cluster-aware context | Keep the current explicit `cluster_id` model; no transparent API proxy |
| Resource inventory | Typed, bounded list/detail routes for approved workload, policy, storage and RBAC metadata; redacted manifests | KRM claims arbitrary core/CRD browsing; Ratel documents broad classic resources | Existing coverage is sufficient; generic GVK/YAML parity is rejected |
| Troubleshooting | Metrics, history, logs, events, topology and deterministic diagnosis are implemented | KRM/Ratel expose more direct action entry points | Add alert lifecycle, then keep diagnosis as the human workflow source of truth |
| Release operations | Controlled Deployment restart, scale, image update, exact rollback and fixed cross-cluster promotion | KRM/Ratel document broad graphical editing/copy | Current narrow mutation model is preferable; no generic create/edit surface |
| Namespace governance | ResourceQuota, LimitRange, PDB, NetworkPolicy and workload details are individually readable, but there is no joined posture or risk summary | Ratel documents quota editing; Kubernetes Guide covers quota, limits, QoS, policy and scheduling | Add deterministic read-only governance and capacity posture in M29 |
| Node maintenance | Node inventory/diagnosis exists; no cordon/drain workflow | KRM documents node management and Pod offlining; Kubernetes operations require disruption-aware maintenance | Add controlled cordon/uncordon and PDB-aware drain in M30; reject arbitrary Pod delete |
| Workload backup | Optional Velero detection and read-only Backup inventory are implemented | KRM documents visual backup/restore | Add controlled Backup creation in M28 and isolated restore rehearsal in M31 |
| Workload restore | Deliberately disabled after M25 | KRM claims restore; Kubernetes Guide includes storage recovery topics | Add only quarantine Namespace restore without PV, overwrite or cutover |
| Identity and audit | Local roles, revocable sessions, encrypted credentials, append-only audit, signed delivery/archive paths | KRM/Ratel examples are weaker or obsolete; KubeSphere separates authentication, authorization and audit | Preserve current layering; production OIDC/MFA remains organization-gated |
| Platform database recovery | Logical destroy/restore evidence and readiness admission exist | Kubernetes Guide covers storage and recovery topics | Do not call logical restore PITR/HA; physical/WAL and HA remain externally gated |
| Engineering evidence | Buildable Go/Vue source, migrations, Compose/Kustomize, kind suites and hosted CI are present | KRM source is unavailable; Ratel is stopped; guide is instructional | Quality gates remain a product requirement, not optional release work |

## 3. Gaps Already Closed

The earlier comparison identified five high-value gaps. M21-M25 close them within fixed boundaries:

| Milestone | Closed gap | Boundary retained |
|---|---|---|
| M21 | Sparse historical metrics, exact-series queries and sustained-window evaluation | No PromQL, arbitrary selectors or manufactured zero samples |
| M22 | Multi-container logs, fixed troubleshooting resources and redacted server-produced manifests | No Secret/ConfigMap value display, exec or arbitrary manifest access |
| M23 | Deployment image update and exact ReplicaSet-backed rollback | No client-owned PodTemplate or generic patch |
| M24 | Fixed Deployment/Service/Ingress cross-cluster promotion with dependency mapping | No Namespace migration or arbitrary object copy |
| M25 | Optional Velero capability and bounded read-only Backup inventory | No create, restore, Secret access or recovery claim |

These capabilities must be regression-tested when a later milestone changes their shared contracts. They are not
reopened merely to imitate a broader reference UI.

## 4. Remaining Product Work To Adopt

### 4.1 M27 - Historical Alert Lifecycle

Turn M21's deterministic evaluator into a bounded background lifecycle: exact Node CPU/memory rules, database
claims, one unresolved instance per rule, linked diagnosis, acknowledgement reuse, insufficient-data state,
recovery and restart durability. This closes the remaining gap between historical evidence and an actionable
operations loop.

It must not introduce PromQL, label expressions, notification routing, silence schedules or an AI-owned state
transition. The detailed contract and real-kind acceptance are in the execution plan.

### 4.2 M28 - Controlled Velero Backup Creation

Extend M25 through one server-derived Backup workflow: one application Namespace, fixed TTL choices, existing
available storage location, dry-run, one-time confirmation, idempotency, audit and real Velero plus S3-compatible
object-storage evidence. Restore remains unavailable during M28.

### 4.3 M29 - Namespace Governance And Capacity Posture

Join already approved read-only data into one deterministic Namespace posture instead of adding more disconnected
resource tables. The V1 result should cover:

- ResourceQuota hard/used pressure and missing quota state;
- LimitRange defaults/min/max coverage and containers missing requests or limits;
- workload QoS/resource-request posture using Kubernetes-defined fields;
- PDB coverage and disruption budget state for supported controllers;
- unschedulable or pressure-affected Nodes and bounded requested-versus-allocatable capacity;
- stable risk codes, cited source resources, partial/truncated/unavailable states and no guessed health.

This is read-only. It does not edit policies, infer complete NetworkPolicy semantics, run a scheduler simulator or
claim that aggregate free capacity guarantees a Pod can schedule.

### 4.4 M30 - Controlled Node Maintenance

Provide a safe alternative to KRM's documented node/Pod operations: single-Node cordon, uncordon and a PDB-aware
drain workflow. Preview must classify DaemonSet, mirror/static, unmanaged, local-storage and evictable Pods; execution
must use the eviction API, fixed bounds, preconditions, confirmation, idempotency and audit.

Force deletion, ignoring PDB, deleting `emptyDir` data, arbitrary Pod delete, browser terminals and multi-Node bulk
drain remain prohibited. A partial drain leaves the Node cordoned and reports exact bounded results.

### 4.5 M31 - Isolated Workload Restore Rehearsal

Complete the Backup lifecycle without presenting a production cutover button. Restore one M28-compatible completed
Backup into a server-generated, new quarantine Namespace on the same cluster. Establish isolation before creating
the Velero Restore, disable PV recovery, reject target conflicts and retain a bounded inventory of restored items.

V1 does not restore in place, overwrite a Namespace, restore PV/PVC data, cross clusters, switch traffic or delete
the source. It proves only that approved namespaced resources can be reconstructed in a disposable isolated target.

### 4.6 M32 - Formal Closure

Bind code, evidence and presentation material to one reviewed revision. Run the final security/quality audit,
hosted CI and authorized runner/release gates; verify the release package and checksums; refresh README, architecture,
test matrix, thesis/demo text and sanitized desktop/mobile screenshots; record every external gate as completed or
formally deferred.

M32 is not permission to change repository settings, register a runner, push a tag, publish an image or create a
release. Those actions still require explicit user authorization.

## 5. Capabilities Explicitly Rejected

The following reference features are not part of this project end state:

| Rejected capability | Reason |
|---|---|
| Arbitrary core/CRD browsing and generic YAML CRUD | Makes authorization, redaction, validation and stable contracts unbounded |
| Broad Deployment/StatefulSet/DaemonSet/Service/Ingress/PV/PVC editors | Duplicates Kubernetes administration while bypassing controlled-operation invariants |
| One-click Namespace/project migration | Hides dependency, identity, storage, admission and rollback conflicts |
| Unrestricted Pod exec, file upload/download and arbitrary Pod delete | Creates a remote-command and data-exfiltration surface outside the accepted threat model |
| Browser Secret/credential management | Conflicts with the no-secret-value API and evidence boundary |
| Force drain, ignore-PDB or delete-`emptyDir` options | Converts maintenance into an uncontrolled availability/data-loss action |
| One-click in-place restore or production cutover | Cannot be accepted without PV, conflict, routing, rollback, RPO and RTO policy |
| Workspace multi-tenancy, extension marketplace and application store | Not required by the single-platform operations scope |
| Generic DevOps/CI platform and Service Mesh console | Large independent products with no evidence that they improve the target workflow |
| Transparent multi-cluster API proxy | Weakens explicit cluster routing and per-operation authorization/audit |

Rejected items should not remain as an implied backlog. Adding one requires a new product decision, threat model,
ADR and explicit user approval.

## 6. Final Milestone Sequence

```text
M26A release preparation  ───────────────────────────────────────┐
M26B organization decisions ─────────────────────────────────────┤
                                                               v
M27 alert lifecycle -> M28 backup creation -> M29 posture -> M30 maintenance -> M31 restore -> M32 closure
```

M26 may proceed in parallel only where authorization and organization inputs exist. The default engineering path
is serial from M27 through M31 because each milestone changes security-sensitive contracts and requires independent
review. M31 depends on the real Backup contract accepted in M28. M29 should precede M30 so drain preview can reuse
one accepted capacity/PDB interpretation rather than creating a second policy engine.

| Milestone | Exit evidence | May be declared complete when |
|---|---|---|
| M26A | Hosted CI/runner/release rehearsal and verified package metadata | Each remote action was authorized and the exact revision is recorded |
| M26B | Approved identity/recovery inputs plus physical/provider drills | Real OIDC/MFA and PITR/HA behavior exists; admission documents alone are insufficient |
| M27 | Real Metrics Server outage/recovery/restart lifecycle | Deduplication, diagnosis reuse and persistence pass |
| M28 | Real Velero controller and disposable object store | Exactly one completed Backup, replay safety, denied Restore/Secret verbs and cleanup pass |
| M29 | Disposable kind governance/capacity fixture | Every risk code is source-cited and partial/truncated states cannot appear healthy |
| M30 | Two-worker kind maintenance fixture | Cordon, PDB block, bounded eviction, partial result, uncordon and cleanup pass |
| M31 | Real Velero quarantine restore fixture | New isolated Namespace, no PV/overwrite/cutover, restored inventory and cleanup pass |
| M32 | Final gates, reviewed release metadata and refreshed documentation/assets | All project-end criteria below are signed off |

## 7. Project-End Criteria

The project reaches **development complete** only when all of the following are true:

1. M27-M31 are accepted against their fixed contracts and real-environment suites; no skipped suite is reported as passed.
2. M21-M25 regressions affected by shared changes pass, and the complete fast/full repository gates are green.
3. Public routes, OpenAPI, frontend types, migrations, RBAC, audit actions and UI states agree.
4. Security review finds no generic write proxy, credential exposure, unrestricted execution or unbounded background/fan-out path.
5. Desktop and mobile workflows pass browser verification without overlap or unexpected console errors.
6. Every disposable cluster, object store, registration, image and temporary credential is cleaned; evidence is sanitized.
7. README, architecture, roadmap, handoff, test matrix, change records and thesis/demo material describe the same exact revision.
8. One release candidate has green hosted CI; package hashes verify independently. Tag/release publication occurs only if authorized.
9. Every M26 external item is marked `completed`, `deferred with owner/reason/re-entry gate`, or `not applicable`; none is implied complete.
10. A final reviewer confirms that no accepted requirement is left only as prose without implementation and evidence.

**Production ready** is a separate claim. It additionally requires organization-approved OIDC/MFA, physical/WAL
PITR, HA failover/failback, production infrastructure, operational ownership and measured RPO/RTO where those
capabilities are required. If those inputs are unavailable, M32 may close the software project as development
complete, but release notes must say that production identity/recovery readiness is deferred.

## 8. Change Control For The Next Agent

The next Agent must use [`../next-development-plan.md`](../next-development-plan.md) as the implementation contract.
It may refine names and endpoint shapes during the milestone's contract-freeze phase, but it may not silently widen
resource kinds, permissions, limits or recovery/maintenance semantics. Any widened boundary requires an ADR and user
approval before implementation.

When a reference feature conflicts with this document, this document's accepted/rejected decision wins. When actual
source or runtime evidence later becomes available for a reference project, update the evidence level first and then
reassess scope; do not rewrite the current result as if that evidence had already existed.
