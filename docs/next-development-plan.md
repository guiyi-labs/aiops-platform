# Next Development Execution Plan

- Status: Ready for execution
- Updated: 2026-07-30
- Repository baseline: `main` at `17d4f6f`
- Accepted implementation baseline: `baseline-m25-20260730` -> `62320fc`
- Default next engineering milestone: M27 Historical Alert Lifecycle
- Planned product closure: M32 Formal Closure
- Audience: the next development Agent and its reviewer

本文是 M21-M25 收口后的后续开发执行合同。`docs/roadmap.md` 继续说明产品方向，
本文负责说明下一位 Agent 应该按什么顺序开发、不得突破哪些边界、每个阶段交付什么，
以及什么证据才允许标记完成。

若本文与较早的阶段叙述冲突，以本文、当前 `docs/roadmap.md` 和任务相关 ADR 为准。
任何改变本文固定范围、上限、权限或非目标的实现，必须先新增或更新 ADR，并在开始
编码前向用户说明影响。

`docs/references/final-product-gap-analysis.md` 是 M25 之后“为什么做/不做”的权威对标结论；
本文是“如何做到项目结束”的权威执行合同。

## 1. Current Accepted Baseline

当前代码已接受 M21-M25：

| Milestone | Accepted capability | Current local evidence |
|---|---|---|
| M21 | 稀疏指标历史、后台采集、精确序列查询、持续窗口评估、趋势与诊断证据 | `.artifacts/m21-history-kind/m21-history-kind-20260730-141646.json` |
| M22 | 有界多容器日志、固定只读资源、服务端脱敏 Manifest、统一详情工作台 | `.artifacts/verification/verify-20260730-124214.json` |
| M23 | Deployment 镜像更新与精确 ReplicaSet 修订回滚 | `.artifacts/m23-release-lifecycle-kind/m23-release-lifecycle-kind-20260730-131353.json` |
| M24 | 固定 Deployment/Service/Ingress 跨集群发布与依赖映射 | `.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-131550.json` |
| M25 | 可选 Velero 能力和只读 Backup 库存 | `.artifacts/m25-workload-protection-kind/m25-workload-protection-kind-20260730-132024.json` |

这些 `.artifacts` 是当前设备的忽略文件，不是跨设备源代码事实。换设备后必须重新生成
本机证据；不得因为文档中存在一个旧路径就宣称新环境已经通过。

当前本机前提：

- Docker Desktop 使用 Linux containers 和 cgroup v2；cgroup v1 会触发 kind 内
  `runc ... unable to freeze` 的已知失败。
- WSL 为 2.7.11，Linux 内核为 6.18.33.2；`C:\Users\33427\.wslconfig` 固定
  `kernelCommandLine=cgroup_no_v1=all`。
- Docker registry mirror 保存在用户级 Docker 配置，不得向 daemon 配置重新加入固定
  `dns`，否则 `host.docker.internal` 解析和 kind API 探测会失败。
- 中国网络下允许使用被忽略的 `.tools/Dockerfile.*-local` 构建本地镜像，但不得只为
  镜像源修改正式 Dockerfile。正式发布仍需托管 CI 使用仓库 Dockerfile 成功构建。

## 2. Priority And Gating

| Priority | Workstream | May start now | Completion gate |
|---|---|---:|---|
| P0 | Baseline audit and contract freeze | Yes | Clean Git state and fast gate pass |
| P0 | M26A release/runner closure | Partially | User authorization plus remote evidence |
| P0 | M26B OIDC/MFA and PITR/HA | No | Named policy, provider and infrastructure decisions |
| P1 | M27 historical alert lifecycle | Yes, default next task | Full local and disposable real-kind acceptance |
| P1 | M28 controlled Velero Backup creation | After M27 | Real Velero controller/object-storage acceptance |
| P1 | M29 Namespace governance and capacity posture | After M28 | Deterministic real-kind posture acceptance |
| P1 | M30 controlled Node maintenance | After M29 | Two-worker kind, PDB and eviction acceptance |
| P1 | M31 isolated workload restore rehearsal | After M30 and accepted M28 | Real Velero quarantine-restore acceptance |
| P2 | M32 formal closure and thesis/demo refresh | Last | Reviewed revision, final gates and explicit external-gate disposition |

The next Agent should start M27 unless the user explicitly supplies the M26 external decisions or
explicitly changes priority. M27-M31 are independently reviewed milestones and must not be collapsed into one
change. M32 must not begin before the intended release revision is frozen.

## 3. First Turn Checklist For The Next Agent

Before changing any file:

```powershell
Set-Location C:\BS\aiops-platform
git status --short --branch
git rev-parse HEAD
git rev-list --left-right --count origin/main...HEAD
git log -5 --oneline --decorate
docker info --format 'server={{.ServerVersion}} cgroup={{.CgroupVersion}} kernel={{.KernelVersion}}'
docker compose ps
kind get clusters
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-fast.ps1 -Scope All
```

The Agent must report:

1. HEAD, ahead/behind counts and whether the worktree already contains user changes.
2. Compose health, Docker cgroup version and any retained kind clusters.
3. The selected milestone and phase, files/contracts expected to change, and explicit non-goals.
4. Whether any external authorization, credential, provider choice or remote mutation is required.

If the worktree contains unrelated user edits, preserve them. If the fast gate fails before any change,
diagnose the baseline failure first and do not hide it inside a feature change.

## 4. Non-Negotiable Development Requirements

### 4.1 Architecture And Scope

- Continue the modular monolith. Add a domain package only when the milestone owns persistent state and
  lifecycle behavior; do not place domain logic in HTTP handlers or Vue components.
- Deterministic rules and stored evidence remain authoritative. AI may explain cited evidence but may not
  schedule, acknowledge, resolve or execute an operation.
- Do not add arbitrary GVK access, YAML editing, generic patching, PromQL, label-expression languages,
  unrestricted Pod exec/file transfer or Secret-value display.
- Every fan-out or background loop must have fixed item, concurrency, timeout, batch and retention bounds.
  Partial failure and truncation must be explicit states, never silently reported as success.
- Reuse established repository, audit, authorization, controlled-operation and error-response patterns.
  Do not create a second generic workflow engine.

### 4.2 Data And Migration

- The next schema change starts at migration `000020`; migrations are append-only and require matching
  `.up.sql` and `.down.sql` files.
- Published migrations `000001` through `000019` must not be edited to accommodate a local database.
- Database constraints must enforce state enums, shape invariants, uniqueness and bounded values, not only
  application validation.
- Multi-step state changes that create a diagnosis, deduplicate an alert or claim an operation must be
  transactional. Race safety must be proven with concurrent repository tests.
- Store only bounded metadata and evidence. Do not persist raw metric payload copies, kubeconfig, tokens,
  Secret data, complete Velero manifests or object-storage credentials.

### 4.3 Public Contract

Any public route change must update in the same milestone:

- backend DTO and validation;
- stable error code mapping;
- router and authorization tests;
- `docs/api/openapi.yaml`;
- frontend API client and TypeScript types;
- API/Vitest contract tests;
- audit action/target mapping for every mutation.

Unknown fields must fail closed on mutation requests. Names, comments and pagination must have explicit
length/range limits. Internal errors must be sanitized before entering API responses or audit details.

### 4.4 Authorization And Audit

- Read operations remain available to authenticated viewer roles unless existing policy is stricter.
- Rule creation/update/disable, alert acknowledgement through diagnosis workflow, backup preview and backup
  execution require `system_admin` or `operations_admin`.
- Every successful, denied and failed mutation must emit the existing low-cardinality audit shape without
  confirmation tokens, idempotency keys, credentials or raw upstream bodies.
- Kubernetes RBAC is least privilege and milestone-specific. New write verbs require a threat-boundary review
  and a real `kubectl auth can-i` assertion for both allowed and denied verbs.

### 4.5 Frontend Quality

- Extend the current operations UI and design system; do not add a marketing page or a parallel shell.
- Use existing Lucide icons, status badges, tables, drawers and confirmation flows.
- Loading, empty, partial, insufficient-data, unavailable, denied, conflict, expired and retry states must be
  represented where applicable.
- Desktop and mobile layouts must not overlap. Wide tables stay inside one horizontal scroll container.
- User workflow changes require component/API tests and Playwright/browser verification at 1280x720 and
  390x844, with no unexpected console warning/error.

### 4.6 Security And Evidence

- `.env`, passwords, tokens, kubeconfig, private keys, webhook secrets and object-store credentials never enter
  source, logs, audit details, screenshots or `.artifacts`.
- Acceptance JSON contains counts, states, timestamps, revisions and hashes only. It must not embed Kubernetes
  credentials or raw manifests with sensitive fields.
- Real-kind scripts use unique names, short-lived ServiceAccount tokens, isolated platform registrations and
  `finally` cleanup. They never reuse or delete a retained `aiops-test` cluster.
- A failed or skipped gate is reported as failed or skipped. It must never be converted to passed by editing the
  evidence file or weakening the assertion.

## 5. M26 - Organization Integration And Formal Release

M26 is split because part of it is normal engineering and part requires organization-owned decisions.

### 5.1 M26A Release Closure

Allowed preparatory work:

- Revalidate pinned Actions, `actionlint`, reusable CI result aggregation and release-package checksums.
- Add a read-only runner preflight that checks Windows PowerShell 5.1, Docker cgroup v2, available memory,
  kubectl/kind versions, absence of production kubeconfig and absence of stale `aiops-*-e2e-*` resources.
- Rehearse package generation with `workflow_dispatch` only after the user authorizes remote workflow execution.

External actions that require explicit user authorization:

- registering or removing a GitHub self-hosted runner;
- changing repository settings, required checks, branch protection or repository visibility;
- pushing a semantic tag, creating a GitHub Release or publishing registry artifacts;
- creating signing, registry, cloud or environment secrets.

M26A acceptance:

- `actionlint` passes and deployment contract tests cover runner/release safety markers.
- A disposable real-kind runner job executes the selected suite and leaves the initial cluster set unchanged.
- A release rehearsal produces source, backend/frontend image archives, OpenAPI, dependency licenses,
  `release-metadata.json` and `SHA256SUMS`; all hashes verify on a separate trusted process.
- No tag/release is claimed until the user approves the exact revision and hosted CI is green.

### 5.2 M26B External Readiness Gates

Do not implement production OIDC/MFA until the identity owner supplies and approves:

- issuer, discovery and JWKS behavior;
- client type, redirect URIs, claims/role mapping and MFA assurance claim;
- secret storage/rotation, break-glass login and provider outage behavior;
- logout, session revocation and audit requirements.

Do not implement production PITR/HA until the infrastructure owner supplies and approves:

- measured RPO/RTO and retention;
- WAL archive destination, encryption, restore identity and key rotation;
- topology, fencing, failover/failback, split-brain prevention and rollback;
- disposable physical restore/failover drill design.

The existing offline readiness reports are admission checks only. Passing them is not evidence that SSO, MFA,
PITR, HA, failover or failback exists.

## 6. M27 - Historical Alert Lifecycle

M27 is the default next implementation milestone. It turns the accepted M21 evaluator into a bounded background
workflow without duplicating the diagnosis lifecycle.

### 6.1 Product Outcome

An operations administrator can create a fixed alert rule for one exact Node CPU or memory series. The backend
evaluates enabled rules in the background, creates exactly one active alert/diagnosis for a sustained breach,
keeps repeated firing evaluations deduplicated, exposes insufficient-data/error state, and marks the alert resolved
only after a complete normal evaluation. Human acknowledgement and ownership reuse the linked diagnosis record.

### 6.2 Fixed V1 Contract

M27 V1 is intentionally limited to:

- resource kind: `Node` only;
- metric: `node_cpu` or `node_memory` only;
- exact `cluster_id` plus exact Node name; no selectors, regex or labels;
- operator: `gte` or `lte`;
- absolute non-negative `int64` threshold in nanocores or bytes;
- `for_seconds`: 60 through 21,600;
- `minimum_points`: 2 through 360;
- one immutable evaluation shape after creation; only display name and enabled state may be patched;
- maximum 20 non-deleted rules per cluster and case-insensitive unique rule names per cluster;
- soft disable/delete metadata so historical alert/diagnosis references are never orphaned.

Changing cluster, Node, metric, operator, threshold, duration or minimum points requires creating a new rule.
Pod rules, percentages, multi-metric correlation, notification routing, silence schedules, escalation policies,
PromQL and arbitrary labels are non-goals.

### 6.3 Persistence And State

Migration `000020` should introduce a dedicated alert domain with these logical records (exact names are chosen in
ADR 0043 before implementation):

- rule: identity, immutable evaluation fields, enabled/deleted state, creator, last evaluation state/time/error code,
  next due time and bounded claim metadata;
- alert instance: rule, linked diagnosis, `firing` or `resolved`, first/last firing time, resolved time and latest
  complete evaluation evidence anchor;
- database uniqueness that permits at most one unresolved instance per rule.

Do not store one row per scheduler tick. The accepted metric collection/history tables remain the source series.
The firing diagnosis preserves the evidence snapshot; the rule stores only the latest bounded scheduler outcome.

State rules:

1. `firing` with no unresolved instance atomically creates one alert instance and one linked diagnosis.
2. Repeated `firing` updates `last_fired_at` and evidence anchor without creating a second diagnosis or notification.
3. `insufficient_data` or evaluator/upstream error updates the rule health but never fires or resolves an alert.
4. A complete `normal` evaluation resolves the unresolved alert instance.
5. A later firing evaluation creates a new instance and diagnosis, preserving the previous resolved history.
6. Acknowledgement, assignment, comments and human status changes reuse the existing diagnosis APIs. The alert API
   derives acknowledgement from its linked diagnosis; do not create a parallel human workflow table.

The create-or-touch path must be transactional and safe under two backend instances evaluating the same due rule.
Use a bounded database claim (`FOR UPDATE SKIP LOCKED` or equivalent expiring lease) and a unique unresolved-rule
constraint. In-memory mutexes alone are not sufficient.

### 6.4 Scheduler Bounds

Default and maximum behavior:

- scheduler poll interval: 15 seconds;
- each rule is evaluated no more often than once per 60 seconds;
- claim batch: at most 20 due rules;
- worker concurrency: at most 4 per backend process;
- per-rule evaluation timeout: 10 seconds;
- claim lease: 30 seconds with expired-claim recovery;
- no overlapping scheduler tick inside one process;
- deterministic rule order by next due time then ID;
- only enabled clusters and enabled, non-deleted rules are evaluated.

All values are configuration with validation only if operational tuning is necessary; defaults and hard maxima must
be documented in `.env.example`, config tests and `docs/development.md`. A zero/invalid value must fail startup or
use a documented safe default, never become unbounded.

### 6.5 API And UI Deliverables

Freeze exact paths in ADR/OpenAPI before coding. The expected surface is:

- cluster-scoped list/create/get for alert rules;
- patch limited to name and enabled state;
- soft delete with conflict behavior when an unresolved instance exists;
- bounded alert list/detail with cluster, rule, state and time filters;
- linked diagnosis ID/status/assignee and latest evaluation health;
- no endpoint that accepts an expression or raw query.

The frontend adds an operator-focused alert view or a clearly separated alert section in the existing diagnosis
workflow. It must support rule creation, enable/disable, active/resolved filtering, coverage/error display and a deep
link to the diagnosis drawer. Acknowledgement uses the existing diagnosis transition to `confirmed`.

### 6.6 M27 Delivery Phases

1. **M27.0 Contract freeze**: ADR 0043, migration design, API/error/state table, threat boundary, test plan.
2. **M27.1 Domain and repository**: migration 000020, models, concurrency-safe claims, deduplication and state tests.
3. **M27.2 Scheduler and diagnosis bridge**: bounded workers, exact evaluator reuse, restart recovery, one-diagnosis rule.
4. **M27.3 API and frontend**: authorization, audit, OpenAPI, typed clients, alert/rule workflow and responsive tests.
5. **M27.4 Real-kind and closure**: disposable Metrics Server outage/recovery/restart acceptance, full gate, docs and
   change record.

Do not combine all phases into one unreviewable change. Each phase must leave tests green and document any deliberate
contract change before the next phase starts.

### 6.7 M27 Acceptance Standard

Unit/repository tests must prove:

- validation of every fixed enum/range and the 20-rule cluster limit;
- case-insensitive naming and immutable evaluation fields;
- two concurrent claims cannot create two unresolved alerts or diagnoses;
- expired claims recover after simulated process death;
- firing -> repeated firing -> normal -> later firing produces two historical instances, not duplicate active rows;
- insufficient data and stable evaluator errors never create or resolve an alert;
- disabled/deleted rules are not scheduled;
- authorization and audit include success, denied, conflict and failure without sensitive material.

The disposable `scripts/e2e-m27-alert-lifecycle-kind.ps1` must prove against real Metrics Server and PostgreSQL:

1. a unique kind cluster and exact Node rule become firing;
2. at least two scheduler passes leave one active alert and one diagnosis;
3. the linked diagnosis can transition to confirmed without creating a second alert workflow;
4. Metrics API outage yields insufficient/error evidence and does not resolve the firing alert;
5. recovery followed by a complete normal threshold resolves it;
6. a later breach creates a new instance;
7. backend restart preserves rules, instances, links and deduplication;
8. temporary registration, image and kind cluster are removed; the initial cluster set is unchanged.

Success evidence goes to `.artifacts/m27-alert-lifecycle-kind/` and contains no credential or raw kubeconfig.

## 7. M28 - Controlled Velero Backup Creation

M28 begins only after M27 is accepted or the user explicitly reprioritizes it. It extends M25 from read-only inventory
to one fixed creation workflow. It does not add restore.

### 7.1 Fixed V1 Scope

- Velero `velero.io/v1` Backup only; no Schedule, Restore, DeleteBackupRequest or arbitrary CRD write.
- One source cluster and exactly one included application Namespace per plan.
- Backup CR is created in one configured Velero control Namespace, default `velero`.
- Backup name is generated server-side from a safe prefix plus unique suffix; callers cannot choose an arbitrary name.
- TTL is selected from the fixed set 24h, 168h or 720h.
- Storage location must be an existing `Available` BackupStorageLocation selected by exact name.
- Volume snapshots, file-system backup, hooks, selectors, excluded resources and cluster-scoped resources are disabled
  in V1. Persistent-volume restore behavior remains outside the claim.
- No object-store endpoint, access key, secret name/value or plugin credential is accepted by the browser/API.

### 7.2 Controlled Operation Contract

Preview must:

- verify Velero API/controller capability, target Namespace UID/resourceVersion and available storage location;
- verify the generated name is absent;
- derive the complete bounded Backup spec server-side;
- perform Kubernetes server-side dry-run;
- return a typed scope/diff, expiring one-time confirmation token and no raw credential-bearing manifest.

Execution must require the preview ID, token and an idempotency key. It rechecks Namespace and storage-location
preconditions, creates exactly one Backup, stores the returned UID/resourceVersion, and returns the same plan on
same-key replay. Conflicting keys or stale preconditions fail closed. Plans expire after 10 minutes.

ADR 0044 and migration `000021` introduce a dedicated workload-protection operation repository. Do not widen the
existing remediation action enum or store this lifecycle in an in-memory token map.

### 7.3 RBAC And UI

- Read/list existing M25 Backup projections and BackupStorageLocation availability.
- Permit `create/get` Backup only in the configured Velero control Namespace.
- Explicitly deny update, patch and delete Backup; deny all Restore and Secret verbs.
- The UI shows capability/storage readiness before enabling preview, then reuses the established preview/diff,
  confirmation, idempotency and result pattern.
- The UI never offers restore, credential entry, arbitrary Namespace lists or raw Backup YAML.

### 7.4 M28 Delivery Phases

1. **M28.0 Contract freeze**: ADR 0044, Backup spec/operation/state/error table, RBAC boundary and test plan.
2. **M28.1 Domain and preview**: migration 000021, repository, Velero/storage preflight, dry-run and concurrency tests.
3. **M28.2 Execution and UI**: idempotent create, polling/result, audit, OpenAPI/types and responsive workflow.
4. **M28.3 Real-Velero closure**: controller/object-store E2E, full gate, docs and dated change record.

### 7.5 M28 Acceptance Standard

Unit and loopback tests must cover fixed scope, generated names, TTL allowlist, unavailable Velero/controller/storage,
dry-run error sanitization, stale preconditions, token expiry, concurrent idempotent execution and forbidden fields.

The disposable `scripts/e2e-m28-backup-creation-kind.ps1` must use a pinned real Velero controller and disposable
S3-compatible object storage, not only the M25 CRD stub. It must prove:

1. one test Namespace with bounded non-sensitive objects is backed up;
2. preview dry-run makes no Backup;
3. confirmed execution creates exactly one Backup that reaches `Completed`;
4. same-key replay returns the same plan/Backup without a second CR;
5. inventory/detail show the completed Backup using the M25 projection;
6. unavailable storage and stale Namespace preconditions fail closed;
7. the platform identity can create/get Backup but cannot delete Backup, create Restore or read Secret values;
8. object store, cluster, registration, images and temporary files are cleaned in `finally`.

Evidence goes to `.artifacts/m28-backup-creation-kind/`. A completed Backup is evidence of this disposable creation
path only; it is not production restore, PV recovery, off-cluster retention, RPO or RTO evidence.

## 8. M29 - Namespace Governance And Capacity Posture

M29 converts the fixed read-only resources accepted in M22 into one source-cited operational posture. It must not
create a second resource inventory or policy engine.

### 8.1 Product Outcome

An authenticated operator can select one cluster and one Namespace and see whether resource policy, disruption
coverage and schedulable capacity require attention. Every conclusion cites the Kubernetes objects and observation
time used to derive it. Missing, denied, unavailable and truncated data remain visible and can never become a green
summary.

### 8.2 Fixed V1 Scope And Bounds

- One exact enabled `cluster_id` and one exact Namespace per request; no fleet-wide policy scan.
- Read ResourceQuota, LimitRange, PodDisruptionBudget, supported workload PodTemplates, current Pods and Nodes only
  through reviewed typed adapters.
- Supported controllers are Deployment, StatefulSet, DaemonSet and CronJob. Jobs, bare Pods and unknown owners are
  reported but not assigned inferred controller policy.
- Inspect at most 100 workloads, 500 Pods, 50 PDBs, 20 quotas, 20 limit ranges and 100 Nodes. Use stable ordering and
  explicit per-source truncation.
- Use Kubernetes Quantity parsing and `requests`/`limits`/`allocatable` fields. Do not derive capacity from live
  usage metrics or claim that aggregate free capacity proves a Pod can schedule.
- Derive only reviewed codes: missing/exhausted quota, missing LimitRange defaults, missing container requests or
  limits, BestEffort workload, no matching PDB, blocked/zero PDB disruptions, Node pressure/unschedulable state,
  requested-capacity threshold and incomplete evidence.
- Freeze severity and the requested-capacity warning threshold in ADR 0045. No user-authored rules or policy DSL.
- The response is computed on demand and is not a historical compliance database. M29 adds no migration.

NetworkPolicy may be linked as inventory evidence, but V1 must not infer reachability or declare a Namespace secure
from selector inspection. Affinity, taints, topology spread, storage binding and scheduler plugin simulation are
outside the capacity claim.

### 8.3 API And UI Deliverables

- Freeze one bounded Namespace posture route and stable error/risk schema in ADR 0045 and OpenAPI.
- Return overall state, source coverage, typed findings, cited `kind/namespace/name/uid/resourceVersion`, observation
  time and truncation counters. Do not return raw manifests.
- Reuse the resource detail drawer/deep links for a cited object when that kind is already approved.
- Present quota pressure, request/limit coverage, PDB coverage and Node/capacity state as scan-friendly sections,
  with loading, empty, partial, unavailable and denied states.
- This surface is read-only for every platform role. It must not offer quota, limit, PDB, Node or workload edits.

### 8.4 M29 Delivery And Acceptance

1. **M29.0 Contract freeze**: ADR 0045, source/risk/severity table, bounds, error and test matrix.
2. **M29.1 Aggregator**: exact typed reads, deterministic joins, Quantity math, truncation and focused tests.
3. **M29.2 API and UI**: authorization, OpenAPI/types, deep links, responsive and partial-state tests.
4. **M29.3 Real-kind closure**: disposable fixture, full gate, docs and dated change record.

Unit and contract tests must cover selector matching, multi-container requests, zero/missing quota fields, Quantity
overflow, PDB overlap, unknown owners, Node pressure, deterministic ordering and each independent source failure.

`scripts/e2e-m29-governance-posture-kind.ps1` must prove:

1. one healthy bounded Namespace yields only the expected informational findings;
2. missing requests/limits, quota exhaustion and BestEffort workloads produce the frozen codes and citations;
3. a PDB with `disruptionsAllowed=0` is blocked while an unmatched workload is separately unprotected;
4. one unschedulable/pressure fixture and one requested-capacity threshold produce exact capacity findings;
5. one denied or unavailable source makes the result partial, never healthy;
6. desktop `1280x720` and mobile `390x844` render all states without overlap or unexpected console errors;
7. the initial cluster/registration set is restored in `finally`.

Evidence goes to `.artifacts/m29-governance-posture-kind/` and contains counts/codes/citations only, not raw
manifests or credentials.

## 9. M30 - Controlled Node Maintenance

M30 provides one disruption-aware alternative to arbitrary Pod deletion. It reuses the controlled-operation shape
accepted in M23 and the PDB/capacity interpretation accepted in M29.

### 9.1 Fixed V1 Scope

- Actions: cordon one worker Node, uncordon one worker Node, and drain one already-cordoned or atomically cordoned
  worker Node. Control-plane Nodes and bulk selection are rejected.
- Preview exactly one Node and at most 100 resident Pods. At most 20 Pods may be classified as evictable.
- DaemonSet-managed and mirror/static Pods are retained and reported. Unmanaged Pods, any Pod using `emptyDir`, an
  unknown owner, unavailable PDB evidence or an over-limit set blocks execution.
- Drain uses `policy/v1` Eviction and honors PDB/admission decisions. No force deletion, direct Pod delete,
  `--disable-eviction`, grace-period override, `emptyDir` deletion or PDB bypass exists.
- Drain execution is deterministic, with concurrency at most 2, 30 seconds per eviction observation and a ten-minute
  total deadline. Timeout or rejection produces an explicit partial result and leaves the Node cordoned.
- Uncordon is a separate confirmed operation; failed/partial drain never automatically uncordons.

### 9.2 Controlled Operation And Persistence

ADR 0046 and migration `000022` define a dedicated maintenance plan; the existing remediation action enum must not
be widened for Node/Pod/PDB evidence. Preview must capture Node UID/resourceVersion and
unschedulable state, classified Pod UID/resourceVersion/owner/storage facts, matching PDB state and a typed result.

Preview performs server-side dry-run for the Node patch, expires after ten minutes and returns a one-time token.
Execution requires token, preview ID and idempotency key, then rechecks Node identity, Pod set and PDB evidence before
mutation. It patches `spec.unschedulable`, invokes bounded evictions, records each low-cardinality outcome and returns
the same result on same-key replay. Stale or widened targets fail closed.

RBAC permits only `get/list/watch` Nodes/Pods/PDB/owners, Node `patch` for unschedulable state and Pod `create` on
the `eviction` subresource. It must not grant Pod delete/patch, Node delete/update, Secret access or arbitrary writes.

### 9.3 UI And Acceptance

The Node detail view shows a maintenance preview with retained, blocking and evictable groups, PDB evidence,
confirmation and the final per-Pod outcome. Destructive emphasis applies only to drain; cordon and uncordon still
require a reviewed preview and explicit confirmation. The UI must explain partial state through results, not hide it
behind a generic success message.

Unit/repository tests must prove role enforcement, token expiry, stale Node/Pod/PDB conflict, exact idempotency,
concurrent execution, all blocker classes, eviction 429/timeout behavior, partial-result persistence and absence of
forbidden Kubernetes verbs.

`scripts/e2e-m30-node-maintenance-kind.ps1` must create a disposable kind cluster with at least two worker Nodes and
prove:

1. a control-plane Node, unmanaged Pod and `emptyDir` Pod are rejected;
2. cordon changes only the selected worker and same-key replay is mutation-free;
3. a protective PDB blocks eviction and records no direct Pod delete;
4. after the fixture permits disruption, drain evicts only the bounded eligible Pods and retains DaemonSet Pods;
5. a forced timeout/rejection records partial state and leaves the Node cordoned;
6. uncordon restores schedulability only after a separate confirmation;
7. audit/evidence contains no token, kubeconfig or raw upstream body and cleanup restores the initial cluster set.

Evidence goes to `.artifacts/m30-node-maintenance-kind/`.

## 10. M31 - Isolated Workload Restore Rehearsal

M31 starts only after M28's real Backup creation contract is accepted. It proves bounded reconstruction, not
production recovery or traffic cutover.

### 10.1 Fixed V1 Scope

- Source: one `Completed`, M28-compatible Velero Backup on the same cluster and exact Backup UID/resourceVersion.
- Destination: one server-generated, previously nonexistent Namespace with an ownership label and stored UID.
- Before Restore creation, the platform creates quarantine controls: default-deny ingress/egress NetworkPolicy and
  ResourceQuota that permits zero Pods and zero LoadBalancer/NodePort Services.
- The Velero Restore uses namespace mapping and a fixed allowlist: Deployment, StatefulSet, DaemonSet, CronJob,
  ConfigMap, Secret and ServiceAccount. The platform never reads or returns Secret/ConfigMap values.
- Pods, Jobs, Services, Ingresses, Endpoints/EndpointSlices, PVCs, PVs, volume snapshots, ResourceQuota,
  LimitRange, NetworkPolicy, RBAC bindings, webhooks and cluster-scoped resources are excluded.
- `restorePVs=false`; no in-place target, overwrite, cross-cluster restore, source deletion, traffic switch or
  production cutover.
- One active restore rehearsal per source Backup and at most 100 projected restored resources. Unknown/truncated
  results are incomplete, never successful.

The V1 platform does not expose generic Namespace deletion. Disposable E2E removes its cluster in `finally`; a real
quarantine Namespace is retained for review and its administrative removal is an explicit documented follow-up.

### 10.2 Controlled Operation And Persistence

ADR 0047 and migration `000023` define restore plans, source Backup identity, generated destination identity,
quarantine status, one-time confirmation, idempotency, Velero Restore identity, bounded item counts and terminal
state. Plans expire after ten minutes before execution; executed history is retained according to a fixed policy.

Preview verifies Velero/controller/storage availability, `Completed` source phase, the M28 scope, destination
absence and supported resource projection, then server-side dry-runs the quarantine resources and Restore CR.
Execution rechecks every precondition, creates quarantine before Restore, creates exactly one Restore and polls with
fixed timeout/backoff. Same-key replay returns the same plan and Restore. Failure after Namespace creation records
the retained quarantine target and never retries into a different Namespace silently.

RBAC permits create/get Restore only in the configured Velero Namespace plus create/get the generated Namespace and
its two fixed quarantine resources. Kubernetes RBAC cannot restrict Namespace `create` by generated-name prefix, so
ADR 0047 must explicitly threat-model this cluster-scoped verb, isolate it to the managed-cluster identity and prove
that every public path rejects caller-owned names. It denies Restore update/delete, arbitrary Namespace delete,
PV/PVC/snapshot mutation and Secret reads.

### 10.3 UI And Acceptance

The Backup detail view offers rehearsal only for an eligible completed Backup. It shows the exact exclusions,
generated target, quarantine controls, preview, confirmation, terminal phase and bounded restored-item names/kinds.
It never shows a Restore action as “recover production,” exposes secret values or offers overwrite/cutover.

Unit/repository tests must cover source phase/scope, destination collision, allowlist/exclusions, quarantine ordering,
token expiry, stale Backup, concurrent idempotency, controller failure, restart recovery, retention and redaction.

`scripts/e2e-m31-isolated-restore-kind.ps1` must use pinned real Velero and disposable S3-compatible storage to prove:

1. an M28-compatible Backup completes and preview creates no Namespace or Restore;
2. confirmed execution creates one new generated Namespace and exactly one Restore;
3. quarantine controls exist before restored controllers and no Pod can become admitted;
4. approved workload/configuration objects appear under the target Namespace, while Service/Ingress/PV/PVC and
   cluster-scoped resources do not;
5. same-key replay, target collision and stale Backup fail or replay exactly as specified;
6. backend restart preserves plan, source/target/Restore identities and terminal state;
7. platform RBAC cannot delete Restore/Namespace, mutate PV/PVC or read Secret values;
8. evidence is sanitized and the disposable cluster/object store/registration/images are removed in `finally`.

Evidence goes to `.artifacts/m31-isolated-restore-kind/`. It is evidence of an isolated logical workload-resource
rehearsal only, not PV recovery, application consistency, production RPO/RTO or cutover.

## 11. M32 - Formal Closure And Thesis/Demo Refresh

M32 runs after M27-M31 are accepted and the user selects one release candidate revision. It is the only milestone
that may declare this development route closed.

Required work:

- perform a final threat-boundary, RBAC, migration, API/OpenAPI parity, dependency/license and generated-file audit;
- run every applicable L0-L3 gate and regress M21-M25 shared contracts; record pass/fail/skip without editing evidence;
- obtain green hosted CI, then run only user-authorized runner, tag, release, registry or signing actions;
- independently verify release metadata and `SHA256SUMS` against the exact reviewed revision;
- update README capability/status/count statements, architecture diagrams, test matrix, defense script, environment
  table, roadmap, handoff and one dated final closure record;
- recapture authenticated desktop and mobile screenshots against the exact revision and record revision, optional
  tag, viewport, timestamp and sanitized fixture identity;
- classify every M26 external item as `completed`, `deferred` with owner/reason/re-entry gate, or `not applicable`.

Acceptance requires valid local links, no stale milestone counts, no UI overlap or unexpected console error, no
credential/private endpoint/personal data in committed assets, and a reviewer-signed project-end checklist. A tag or
public release is not mandatory when authorization is absent; the absence and re-entry condition must be explicit.

M32 may declare **development complete** when all locally implementable requirements and evidence are closed.
It may declare **production ready** only when real organization-approved OIDC/MFA, physical/WAL PITR and HA drills
also exist where required. Readiness admission documents alone never satisfy that claim.

## 12. Dependency And Project-End Rules

| Milestone | Depends on | Must rerun when shared behavior changes |
|---|---|---|
| M27 | M21 evaluator/history and diagnosis | M21 history/diagnosis real-kind suites |
| M28 | M25 Velero read projection and controlled-operation pattern | M25 inventory and M23 operation suites |
| M29 | M22 typed resources and M21 metrics conventions | M22 resource/API tests; M21 only if metrics contracts change |
| M30 | M29 PDB/capacity interpretation and M23 operation pattern | M29 posture and M23 operation suites |
| M31 | M28 real Backup contract and M25 projection | M28 creation and M25 inventory suites |
| M32 | Accepted M27-M31 and selected revision | Every affected suite plus full final gate |

The project ends only when the ten criteria in
`docs/references/final-product-gap-analysis.md#7-project-end-criteria` are reviewed and recorded. Rejected generic
CRUD, terminal, migration and production-cutover features are not hidden backlog and do not block closure.

## 13. Verification Ladder

Every code phase must run the smallest relevant tests during development and the following ladder before milestone
closure.

### L0 - Static And Focused

```powershell
git diff --check
Set-Location .\backend
gofmt -w <changed-go-files>
go test -count=1 ./internal/<changed-domain> ./internal/httpserver ./internal/deployment
Set-Location ..\frontend
pnpm typecheck
pnpm test
pnpm build
```

Do not run `gofmt -w .` because it may rewrite unrelated user files.

### L1 - Fast Repository Gate

```powershell
Set-Location C:\BS\aiops-platform
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-fast.ps1 -Scope All
```

### L2 - Full Local Gate

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

`-SkipComposeBuild` is acceptable only for local diagnosis when current-HEAD images were built through the ignored
mirror Dockerfiles. It is not sufficient as the sole release acceptance; canonical Dockerfiles must build in hosted
CI or another unrestricted clean environment.

### L3 - Milestone Real Environment

Run the new milestone's disposable kind script without functional skip flags. Also rerun every older real-kind suite
whose shared contract changed. Shared metrics/diagnosis changes require M21 regression; shared controlled-operation
changes require M23 and possibly M24 regression; Velero read contracts require M25 regression.

Real kind suites run serially on the current 16 GiB machine. Do not run multiple control-plane-heavy suites in
parallel.

### L4 - Release/Remote Gate

Only after user authorization:

- push the reviewed branch/commit;
- wait for the unified hosted CI result;
- run the self-hosted disposable suite when runner capacity exists;
- rehearse or create a semantic tag from the exact green revision.

Remote success never replaces required local real-kind evidence for a milestone-specific workflow unless the remote
job runs the same assertions and preserves sanitized evidence.

## 14. Definition Of Done

A phase or milestone is complete only when all applicable items are true:

- product outcome and non-goals match the accepted ADR;
- migration, repository, service, HTTP, OpenAPI, frontend types and UI are aligned;
- limits, authorization, audit, redaction, idempotency and concurrency are tested;
- focused tests, fast gate and full gate pass;
- required real-kind/database/browser acceptance passes with redacted evidence;
- failure paths and cleanup are verified, not inferred;
- README/docs/roadmap/handoff/test matrix and a dated `docs/changes/` record are current;
- `kind get clusters` shows no milestone cluster residue;
- Compose services are healthy after restart/durability tests;
- `git status` contains only intentional source/doc changes and no generated or secret file;
- no unrun test or externally gated item is described as passed.

For M32 closure, phase-level Definition of Done is necessary but not sufficient: the project-end criteria in the
final gap analysis must also be recorded against the selected revision, including explicit disposition of every
M26 external gate.

## 15. Required Agent Exit Report

The implementing Agent's final report must include:

1. Outcome and user-visible workflow.
2. Contract and migration changes, including stable errors and bounds.
3. Security/RBAC/audit behavior and explicit non-goals.
4. Tests actually run, exact pass/fail result and evidence paths.
5. Browser viewports checked for UI work.
6. Cleanup state: Compose health, kind clusters and temporary registrations.
7. Git status and any uncommitted/unpushed work.
8. External approvals or follow-up work still required.

## 16. Handoff Prompt

The user may give the following instruction to the next Agent:

```text
Read docs/next-development-plan.md, docs/roadmap.md and the current baseline section at the top of
docs/development-handoff.md, then read docs/references/final-product-gap-analysis.md for the accepted comparison and
project-end boundary. Audit the workspace before editing. Unless I explicitly provide M26 external decisions or
reprioritize the roadmap, start the earliest incomplete milestone in the serial M27-M31 route, beginning with its
contract-freeze phase, and carry only the selected phase through its applicable verification ladder. Preserve every
fixed scope, bound, permission, security invariant and rejected capability. Do not combine milestones, change remote
repository settings, register a runner, push a tag, publish an image or create a release without my explicit
authorization. Report only tests and evidence actually completed; classify unavailable external gates as deferred,
never passed.
```
