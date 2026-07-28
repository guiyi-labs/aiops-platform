# Development Handoff

- Last updated: 2026-07-28
- Repository: `E:\k8s\毕设\aiops-platform`
- Git state: local `main` tracks private remote `https://github.com/guiyi-labs/aiops-platform.git`; M20 Phase 10 signed audit archives are implemented locally but not yet committed or accepted in hosted CI
- Current milestone: Phase 10 full local acceptance is complete; create/push the implementation revision and archive hosted CI; branch protection is unavailable on the current private-repository plan, while runner registration and release publication remain pending

M20 Phase 10 adds ADR 0031, the offline `/app/audit-archive` command and an
isolated PostgreSQL drill. Archive creation requires an explicit ID range,
output and Ed25519 private-key file, checks a reviewed 1..10000 maximum before
writing, and emits canonical JSON plus a detached signed manifest. Verification
requires a separately supplied trusted public key and checks signer identity,
signature, exact payload SHA-256, metadata and ordering. The isolated run at
2026-07-28 15:08 +08:00 passed two-row signing/verification, three-row overflow
refusal with no output, one-byte tamper rejection and all five cleanup
assertions. Evidence is
`.artifacts/audit-archive/audit-archive-20260728-154047.json`. The 361.34-second
full local gate passed all backend packages and three binaries, 167 Go `Test*`
entries, 14 Vitest files / 59 tests, production build, three healthy services,
Kustomize 16/5/22/3 and runtime HTTP checks. Evidence is
`.artifacts/verification/verify-20260728-153059.json`; hosted evidence remains
pending before the phase is accepted.

Hosted run `30338972042` passed Backend, Frontend, Manifests and the existing
credential drill but failed the new drill's process-environment cleanup
assertion because Linux PowerShell represents a restored unset variable as
empty rather than `$null`. The actual resources were removed. The assertion is
now null/empty-normalized and passed the final local rerun; push and archive the
replacement hosted run before accepting Phase 10.

M20 Phase 9 adds ADR 0030, migration 000016, an active-plus-legacy AES-GCM
keyring and the default-dry-run `/app/credential-reencrypt` command. Apply is
explicit, bounded to 100 rows per transaction and 10000 reviewed candidates,
serialized by a PostgreSQL advisory lock and audited with versions, counts and
sanitized error codes only. The isolated 2026-07-28 run created two real v1
credentials through the API, proved no-write dry-run and whole-batch rollback,
converted both to v2, then proved a v2-only backend decrypt path. Evidence is
`.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`;
all dedicated containers, network, image and process environment were cleaned.
The full local gate passed in 288.9 seconds with 163 Go `Test*` entries, both
backend binaries, 14 Vitest files / 59 tests, frontend production build, three
healthy Compose services, Kustomize 16/5/22/3 and runtime HTTP checks. Evidence
is `.artifacts/verification/verify-20260728-141111.json`. Hosted CI run
`30334216631` passed all four jobs at revision `151bc7e`, including the isolated
re-encryption and PostgreSQL recovery drills, random-production-config Compose
health, sanitized evidence upload and unconditional teardown.

M20 Phase 8 adds ADR 0029, the recovery runbook and
`scripts/e2e-postgres-backup-restore.ps1`. The script starts an isolated
PostgreSQL 17 source with no host port, applies all 15 migrations, inserts
synthetic relational fixtures, creates a custom-format dump, destroys the
source and restores a fresh target. The 2026-07-28 local run preserved all
expected migration/table/encrypted-byte invariants, found zero invalid foreign
keys and removed both containers and temporary backup material. The regular CI
runtime job now runs this drill and uploads only sanitized JSON evidence. This
does not claim production retention, PITR, RPO/RTO, PVC recovery or HA.
The post-change full local gate passed in 278.81 seconds with all backend
packages, 14 Vitest files / 59 tests, production build, three healthy Compose
services, Kustomize 16/5/22/3 and runtime HTTP checks. Evidence is
`.artifacts/verification/verify-20260728-125500.json`; actionlint 1.7.7 also
returned zero findings.
Hosted CI run `30331346283` passed all four jobs for the final Phase 8 archive,
including the PostgreSQL source-to-fresh-target restore on Ubuntu PowerShell,
the independent Compose runtime health checks, sanitized evidence upload and
unconditional teardown.

M20 Phase 7 reviewed Dependabot PRs #1, #2, #5 and #6. The Actions and Go
updates were merged after all four hosted checks passed; the Vue and vue-tsc
patch update was then merged and the combined `main` revision passed run
`30328283896`. The multi-major frontend PR #3 was closed without merge, and the
duplicate pnpm PR #4 was superseded by the reviewed `pnpm/action-setup` v6
commit. `.github/dependabot.yml` now groups only minor/patch updates and the
contract suite verifies all three ecosystem policies.

M20 Phase 6 adds ADR 0028, `.github/workflows/ci.yml`, `release.yml` and
`real-kind-e2e.yml`, plus grouped weekly Dependabot updates. Pull requests use
read-only permission and no repository secret; backend, frontend and manifest
jobs gate an ephemeral Compose runtime with random credentials and guaranteed
teardown. Release rehearsals validate `vX.Y.Z` and only package; tagged runs
reuse the complete CI, produce checksummed versioned image/source/API/license
assets and require `gh release create --verify-tag`. Physical kind suites run
only weekly or manually on a dedicated `[self-hosted, windows, x64,
aiops-kind]` runner and include the disposable diagnosis, fleet and search
scripts. All marketplace actions are pinned to commit SHAs. The YAML/security
contract test passed and actionlint 1.7.7 returned zero findings. At Phase 6
acceptance no commit, remote workflow, tag, release or registry push was
created. See
`docs/changes/2026-07-28-versioned-ci-release-pipeline.md` and
`docs/ci-release.md`.

The M20 Phase 6 post-archive gate passed at 2026-07-28 10:07:52 +08:00 in
180.85 seconds. Evidence is
`.artifacts/verification/verify-20260728-100752.json`: 152 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

The reviewed local root baseline was then created on `main` as
`2d46588f8c15ab626703e92eccc35b4de8b53ab2` with author and committer
`zjw <3342773648@qq.com>`. It contains 368 files and excludes `.env`, local
tools, evidence, dependency/build output, `backend/server.exe` and frontend
TypeScript build metadata. The commit-bound full gate passed at 2026-07-28
10:21:10 +08:00 in 177.39 seconds with evidence at
`.artifacts/verification/verify-20260728-102110.json`. No remote, tag or
release was created.

The private GitHub remote was subsequently created at
`https://github.com/guiyi-labs/aiops-platform`. The first push exposed one
pre-existing `gofmt` mismatch in the workflow contract test; revision
`648aea6c94fbc29fbf21d1f799df29880099d454` corrected it. Hosted CI run
`30325194933` then passed on 2026-07-28 at 11:14:24 +08:00: Backend,
Frontend, Manifests and the ephemeral Compose runtime all succeeded, including
runtime health checks, sanitized artifact upload and guaranteed teardown. The
initial grouped Dependabot pull requests were reviewed; major frontend
migrations remain intentionally deferred as separate future work.

M20 Phase 5 adds `scripts/e2e-global-search-kind.ps1` without changing ADR 0026
or the search API. The accepted run created two physically distinct Kubernetes
v1.34.0 kind clusters plus isolated PostgreSQL/backend, returned nine stable
Pod/Deployment/Service/Ingress matches with complete 2/2 cluster coverage,
verified canonical kind selection, `cluster_limit=1` and global truncation,
then produced four localized `TIMEOUT` failures, recovered to nine complete
results and produced four localized `QUERY_FAILED` failures after stopping the
second control plane. The healthy peer retained four usable results in both
fault states. All fixed-kind reads were allowed and creates denied on both
clusters. All eight cleanup assertions passed and the pre-existing
`aiops-test` cluster was preserved. Evidence is
`.artifacts/search-e2e/search-e2e-20260727-225358.json`; see
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`.

The M20 Phase 5 final gate passed at 2026-07-27 23:02:04 +08:00 in
158.94 seconds. Evidence is
`.artifacts/verification/verify-20260727-230204.json`: 151 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

M20 Phase 4 adds ADR 0027, migration 000015 and authenticated CRUD under
`/api/v1/fleet/resources/search/filters`. Records belong to the current actor,
are capped at 20 under a per-user PostgreSQL advisory lock, use
case-insensitive names and persist only the Phase 3 query, Namespace and fixed
kind subset. List projects stale schema/query records as incompatible; they may
be renamed, completely overwritten or deleted, but not applied. The search UI
supports save/apply/rename/overwrite/delete and keeps incompatible repair and
the 20-item limit explicit. Source coverage is now 151 Go `Test*` entries and
14 Vitest files / 59 tests. PostgreSQL/API acceptance produced exactly 20
successes and two 409 conflicts from 22 concurrent creates, then removed all
test rows. Browser save, rename, overwrite, apply and URL linkage passed at
desktop and mobile widths; document widths were 1265/1265 and 375/375, and the
mobile 760px table remained inside its 279px scroller with no warning/error
logs. The browser controller reached the native delete confirmation but timed
out while accepting it, so final cleanup used the independently accepted
DELETE API and the UI confirmation is not overstated. See
`docs/changes/2026-07-27-user-owned-global-search-filters.md`.

The M20 Phase 4 final gate passed at 2026-07-27 22:27:53 +08:00 in
351.1 seconds. Evidence is
`.artifacts/verification/verify-20260727-222753.json`: 151 Go `Test*` entries,
14 Vitest files / 59 tests, production builds, three healthy Compose services,
Kustomize 16/5/22/3 and backend/frontend/proxy health all passed.

M20 Phase 3 remains accepted with 140 Go test entries and 14 Vitest files / 58
tests. Its final gate passed at 2026-07-27 21:03:08 +08:00 in 168.62 seconds;
evidence is `.artifacts/verification/verify-20260727-210308.json`. See
`docs/changes/2026-07-27-bounded-global-resource-search.md`.

The M20 Phase 2 gate passed at 2026-07-27 19:37:11 +08:00 with evidence at
`.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json`. It created two distinct
Kubernetes v1.34.0 kind clusters plus an isolated PostgreSQL/backend runtime,
matched direct and fleet Node/Pod/Deployment/Event totals, verified ID ordering,
`limit=1`, 401/400 and read-only RBAC, then proved `timed_out` at 4003ms,
recovery and `unavailable` isolation. All eight cleanup checks passed and the
pre-existing `aiops-test` cluster was preserved. No retained password, database
or platform record was used.

The post-archive full gate passed at 2026-07-27 19:47:24 +08:00 with evidence
at `.artifacts/verification/verify-20260727-194724.json` (223.18 seconds). Go
vet/all packages/server build, frontend typecheck plus 13 Vitest files / 57
tests and production build passed. Kustomize remained 16/5/22/3 and all three
Compose services plus backend/frontend/proxy runtime checks were healthy.

The M20 Phase 1 full gate passed at 2026-07-27 19:01:33 +08:00 with evidence
at `.artifacts/verification/verify-20260727-190133.json` (104.53 seconds, 133
Go test entries, 13 Vitest files / 57 tests, Kustomize 16/5/22/3 and three
healthy Compose services). Rebuilt Dashboard acceptance used two enabled
platform records over the retained real kind endpoint: one unavailable record
was isolated without hiding the current 1/1 Node, 12/15 Pod, 5/7 Deployment
and 10 Warning result. Desktop 1280x720 and mobile 390x844 had zero document
overflow; the mobile fleet table scrolled only in its 277px container and
browser warning/error logs were empty. This is not claimed as two physically
distinct Kubernetes clusters.

The M19 full gate passed at 2026-07-27 18:04:28 +08:00 with evidence at
`.artifacts/verification/verify-20260727-180428.json` (143.85 seconds, 128 Go
test entries, 12 Vitest files / 56 tests, Kustomize 16/5/22/3 and three healthy
Compose services). The M19 real-kind evidence is
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json`; it passed scale/replay,
CronJob resume/suspend, namespaced RBAC and fixture restoration. The retained demo is
`demo-kind-20260727-165016` (platform cluster ID 39), with Metrics samples and
all seven expected diagnosis IDs. The previous M17 gate remains archived at
`.artifacts/verification/verify-20260727-155239.json`; the M9 real-kind evidence is
`.artifacts/diagnosis-e2e/diagnosis-e2e-20260726-193724.json`, and the change
record is `docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`.
The M10 implementation and responsive browser evidence are archived in
`docs/changes/2026-07-26-event-center-ui-unification.md`.
The M11 cockpit, topology, reference notes and real-data browser evidence are
archived in `docs/changes/2026-07-27-operations-cockpit-resource-topology.md`.
The M12 classified workbench, resource detail contracts and deep-link browser
evidence are archived in `docs/changes/2026-07-27-deep-link-resource-workbench.md`.
The M13 extended resource contracts, sanitization, related-event UI and browser
evidence are archived in `docs/changes/2026-07-27-expanded-read-only-resource-workbench.md`.
The M14 EndpointSlice list contract, complete traffic/workload topology and
responsive real-data evidence are archived in
`docs/changes/2026-07-27-complete-ingress-backend-topology.md`.
The M15 fixed Node/Pod Metrics contracts, optional capability behavior and
responsive Dashboard evidence are archived in
`docs/changes/2026-07-27-real-resource-metrics-foundation.md`.
The M16 Metrics Server fixture, real utilization, consumer ranking and
available-path browser evidence are archived in
`docs/changes/2026-07-27-real-metrics-utilization-consumers.md`.
The M17 fixed workload/policy contracts, Secret threat boundary, real-kind
fixtures and responsive workbench evidence are archived in
`docs/changes/2026-07-27-common-workload-policy-coverage.md`.

## Latest Evidence-Based Diagnosis Expansion

M18 adds `node.pressure.v1`, `persistentvolumeclaim.pending.v1`,
`horizontalpodautoscaler.saturated.v1` and `ingress.backend_unavailable.v1`.
The diagnosis endpoint, OpenAPI, frontend API and Workloads actions use the
same fixed resource contract. Replayable positive/negative fixtures are stored
in `backend/internal/diagnosis/testdata/m18-fixtures.json`.

The real kind run passed all seven diagnoses and retained cluster ID 39. It
created a synthetic Ready+MemoryPressure Node only for the diagnosis call and
deleted it in `finally`; the PVC Warning Event is linked by exact UID and the
HPA status is patched immediately before reading because the controller may
overwrite it. M18 deliberately does not claim sustained restart behavior: a
single Pod snapshot and cumulative restart count are not a time window.
See `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`.

## Stable Baseline

The functional MVP is complete: authentication/RBAC and user management, encrypted cluster onboarding, bounded Kubernetes reads, eleven deterministic diagnosis rules, workflow/SLA/assignment, append-only audit with safe CSV export, and cited AI explanations with runtime guardrails and quality feedback.

M8 extends the deterministic read-only diagnosis surface to Node and Deployment.
`node.not_ready.v1` retains Node Conditions, and
`deployment.replicas_unavailable.v1` retains desired/current readiness counters.
The Workloads page exposes both actions. M9 validates both rules against a
disposable real kind v1.34.0 cluster using only observer RBAC. The final gate passed all Go packages,
frontend typecheck plus 12 Vitest files / 55 tests, production and Docker image
builds, Compose health, Kustomize 16/5/22/3 and runtime HTTP checks.

## Latest Common Workload and Policy Coverage

M17 adds fixed bounded list/detail contracts for StatefulSet, DaemonSet,
ReplicaSet, Job, CronJob, HPA, ResourceQuota, LimitRange and Secret. The API,
OpenAPI document, route drift tests and target-cluster observer RBAC move
together; no arbitrary GVK, API path, YAML or write proxy was introduced. New
Pod templates expose only container names/images, while HPA selectors and
behavior internals are omitted.

Secret has a separate public model that exposes only minimal metadata, type,
immutable and sorted data key names. Raw response verification found none of
the fixture value, annotation bait, `data`, labels or annotations. This is an
application-model boundary, not field-level Kubernetes authorization: the
ServiceAccount can read raw Secret objects because Kubernetes RBAC cannot
grant key-name-only access. Production use therefore requires explicit threat
acceptance and strict protection of the platform identity/runtime.

The retained kind v1.34.0 environment is `demo-kind-20260727-152828`, platform
cluster ID 35, with Metrics Server v0.8.0 and a one-hour credential created
around 15:28 +08:00. All nine list/detail fixtures passed; counts were
1/1/11/1/1/1/1/1/1 at E2E capture time for StatefulSet/DaemonSet/ReplicaSet/
Job/CronJob/HPA/ResourceQuota/LimitRange/Secret. RBAC returned Secret list=yes,
Secret create=no and HPA list=yes. The downstream three diagnoses, remediation
execution/idempotent replay and real Node/Pod metrics also passed.

Desktop 1280x720 validated the four categories, eight workload tabs, exact
deep links, representative details and zero document overflow. Mobile 390x844
kept categories and kinds in two-column grids, contained wide tables inside the
resource panel, and used a 375px full-width drawer. Browser validation exposed
and fixed an M17 Condition DOM/grid mismatch that made HPA messages overlap on
mobile; the corrected cards are 326px wide, messages are 281px wide, and the
browser warning/error log is empty. Screenshots are under ignored
`.artifacts/browser-m17`.

## Latest Metrics Available Path

M16 keeps Metrics Server optional while adding a checksum-pinned v0.8.0 kind
fixture for the real available path. The upstream manifest remains
byte-equivalent and is verified against SHA-256
`ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b`.
The local runtime patch uses the reachable pinned mirror and
`--kubelet-insecure-tls` only for kind; neither change enters normal platform or
managed-cluster Kustomize bases.

Dashboard matches Node Metrics to the same Node's `status.allocatable`, so CPU
and memory percentages are emitted only with valid real denominators. Pod
Metrics are ranked independently by CPU or memory, bounded to five visible
consumers and labeled with loaded/total sample coverage. Every row preserves
cluster, kind, Namespace and name in the `/workloads` deep link.

The retained kind v1.34.0 environment is `demo-kind-20260727-142712`, platform
cluster ID 34, with Metrics Server v0.8.0 and a one-hour credential created
around 14:27 +08:00. Direct and platform reads both returned 1 Node and 12 Pods;
the downstream diagnosis/remediation gate also passed. Desktop 1280x720 showed
CPU 479m/3.0%, memory 911.4 MiB/11.7% and 12/12 Pod coverage. Memory ranking and
the exact Pod detail deep link passed. Mobile 390x844 remained 375/375 with a
single-column ranking, no overflowing elements and no browser warning/error
logs.

## Latest Real Resource Metrics Foundation

M15 adds fixed bounded Node and Pod Metrics routes under each explicit cluster
ID. Public responses retain only metadata, timestamp, window, CPU/memory usage
and Pod container names. The gateway maps an absent metrics.k8s.io API to
`424 METRICS_API_UNAVAILABLE`; it does not return zeros or hide the failure as
an empty list.

Dashboard loads Metrics independently from core Node, Pod, Deployment,
Service, Event and diagnosis data. CPU and memory cards show absolute Node
totals plus Pod totals only when real quantities exist. The parser covers CPU
`n/u/m/core` and binary/decimal memory units. No utilization percentage is
claimed without a real denominator.

The retained kind v1.34.0 environment is `demo-kind-20260727-134215`, platform
cluster ID 33, with a one-hour credential created around 13:42 +08:00. Metrics
Server is absent: core Node returned 200/1 while both Metrics routes returned
424. SubjectAccessReview confirmed get/list=true and create=false for metrics.
Desktop 1280x720 remained 1265/1265 and mobile 390x844 remained 375/375 with no
browser warning/error logs.

## Latest Complete Ingress Backend Topology

M14 adds the fixed bounded `GET /clusters/{cluster_id}/endpointslices` contract.
The public model retains only metadata, address type, port identity, endpoint
conditions/node/targetRef and the Service identity derived from the standard
label. Empty endpoint collections are normalized, and arbitrary discovery
paths or writes remain impossible.

`/topology` now renders Ingress, Service, EndpointSlice, Pod and Deployment.
Relationships require exact same-Namespace backend names, standard Service
labels or Pod targetRefs. Service selector fallback runs only when no matching
EndpointSlice exists. Selecting the real `healthy-nginx` Ingress highlighted
exactly one Ingress, Service, EndpointSlice, Pod and Deployment.

The retained kind v1.34.0 environment is `demo-kind-20260727-130453`, platform
cluster ID 32, with a one-hour credential created around 13:04 +08:00. Desktop
1280 validation had document and topology canvas overflow at zero. At 390x844,
the document remained 375/375 while the 928px topology scrolled only inside its
277px canvas. The browser run exposed and fixed a null EndpointSlice collection
white-screen regression; the current asset has no warning/error logs.

## Latest Expanded Read-Only Resource Workbench

M13 adds fixed list/detail contracts for Ingress, PersistentVolumeClaim,
StorageClass and sanitized ConfigMap metadata/key names. ConfigMap values and
StorageClass parameters cannot enter public response models. `/workloads`
groups eight resource types under workload, network, storage and configuration
categories while retaining the existing path and deep-link query contract.

All eight detail drawers load exact involvedObject Events independently from
the resource detail, so Event failure does not hide the resource. The retained
kind v1.34.0 browser run validated the new four types, PVC
WaitForFirstConsumer and Pod ImagePullBackOff Events, ConfigMap value absence,
StorageClass parameter absence and responsive 390x844 layout with no document
overflow or browser warning/error logs.

The target observer role remains `get/list` only. The demo now renders 10
resources and the retained environment is `demo-kind-20260727-114759` with a
one-hour credential. Real-kind demo preparation also exposed and fixed the
generic Pending rule taking precedence over ImagePullBackOff; all three demo
diagnoses and idempotent remediation passed after the fix.

## Latest Deep-Link Resource Workbench

M12 replaces the long mixed Workloads page with Pod, Deployment, Service and
Node inventory tabs. The server now exposes fixed GET details for Node,
Deployment and Service alongside the existing Pod detail route; all paths are
registered in Gin and OpenAPI and remain within the bounded read-only gateway.

Resource selection is encoded as `cluster/kind/namespace/name` query state.
Refresh restores the selected drawer, closing removes resource selection, and
Dashboard/Topology preserve cluster and resource context when navigating into
the workbench. The retained kind cluster returned 14 Pods, 6 Deployments,
4 Services and 1 Node. All four live detail workflows, topology navigation and
direct-link refresh passed. Desktop and 390x844 checks had no page-level
overflow or browser warning/error logs; the mobile table scrolled only inside
its panel and the detail drawer occupied the complete 375px document width.

## Latest Operations Cockpit and Resource Topology

M11 turns Dashboard into a live selected-cluster cockpit backed by existing
bounded Node, Pod, Deployment, Service and Event reads. It adds six operational
KPIs, health meters, recent Warning signals and direct navigation into topology,
resources, events and diagnoses. `/topology` derives Service -> Pod <- Deployment
relationships only from complete label-selector matches in the same Namespace;
empty selectors and cross-Namespace lookalikes never produce links.

The retained kind v1.34.0 demo showed 1/1 Ready Node, 11/14 Healthy Pods, 4/6
Available Deployments, 4 Services and 19 Warning Events. Selecting the healthy
Service highlighted exactly one Service, one Pod and one Deployment. Desktop
and 390x844 browser checks passed with no page-level horizontal overflow or
browser warning/error logs. The temporary viewport override was reset.

## Latest Event Center and UI Unification

M10 adds `/events` as the real Kubernetes Event Center for every authenticated
role. It supports cluster, Namespace, type, resource kind and resource-name
filters, prioritizes modern Event series timestamps/counts and exposes a safe
detail drawer. `/notifications` remains the diagnosis webhook outbox and is now
named Notification Delivery, preserving its administrator/auditor boundary.

Dashboard now consumes the shared `ConsoleLayout`; a topbar actions slot keeps
its refresh command without duplicating navigation, account security or logout.
The retained kind demo returned 11 Events (5 Warning, 6 Normal, 3 resources).
Desktop 1440x1000 and mobile 390x844 checks passed for both Events and
Dashboard: document-level horizontal overflow remained zero, the wide Event
table scrolled only inside its panel, the mobile drawer was viewport-width with
single-column fields, and browser logs were empty. Focused verification passed
frontend typecheck, 8 Vitest files / 28 tests, production build and backend
Kubernetes/HTTP packages. Exact scope is archived in
`docs/changes/2026-07-26-event-center-ui-unification.md`.

## Latest Node/Deployment kind Validation

M9 adds a separate `deploy/diagnosis-e2e` fixture set and
`scripts/e2e-diagnosis-kind.ps1`. A timestamped kind v0.30.0 cluster running
Kubernetes v1.34.0 produced a synthetic Node with Ready=False and a Deployment
with desired/current/ready/available/unavailable counts of 2/2/0/0/2. The
platform matched `node.not_ready.v1` and
`deployment.replicas_unavailable.v1`, persisted 2 Node Condition evidence rows
and 1 Deployment status evidence row, and confirmed RBAC yes/yes/no/no for
list Nodes, get Deployments, patch Deployments and patch Nodes.

The script deleted the temporary platform cluster and its diagnosis records,
the kind cluster, kubeconfig and status-patch file in `finally`; `aiops-test`
remained present. The retained defense demo fixtures and three diagnosis
scenarios were not modified. Exact results are archived in
`docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`.

## Latest Delivery Packaging

M5 is complete. `scripts/verify.ps1` provides the one-command code/build/runtime
gate and `scripts/e2e-kind.ps1` provides the repeatable real Kubernetes
diagnosis/remediation gate. The latter uses a one-hour ServiceAccount token only
in memory, deletes its temporary platform cluster in `finally`, and writes only
sanitized evidence under ignored `.artifacts`.

Fresh 2026-07-26 results passed all backend packages with Go 1.25, frontend
typecheck plus 8 Vitest files / 26 tests, both production builds, Compose health,
16/5/7 Kustomize resources and backend/frontend/proxy HTTP checks. Real kind
v1.34.0 matched all three rule IDs, executed and idempotently replayed the
allowlisted rollout restart, confirmed the expected RBAC yes/yes/no/no matrix,
and left zero platform cluster/diagnosis/remediation QA rows.

Thesis diagrams, test matrix, environment record, dependency-license report,
reference attribution and the 10-minute defense script are indexed by
`docs/thesis/README.md`. Exact results and compatibility fixes are archived in
`docs/changes/2026-07-26-delivery-packaging.md`.

## Latest Defense Demo Readiness

`scripts/demo-up.ps1` now prepares and retains a fully populated real-kind
environment; `scripts/demo-down.ps1` removes the retained platform record and
optionally the Namespace/RBAC. A from-zero run passed after making Pod polling
tolerant of normal Kubernetes state transitions. The current retained cluster
is `demo-kind-20260726-170601`, Ready on Kubernetes v1.34.0 with three diagnosis
records and one succeeded/idempotently replayed remediation.

`scripts/capture-thesis-screenshots.ps1` uses installed Edge/Chrome through the
standard DevTools Protocol and needs no npm/browser installation. Four 1440x1000
authenticated screenshots were captured and visually checked under
`docs/thesis/screenshots`. The ignored browser profile is removed after every
run. See `docs/changes/2026-07-26-defense-demo-readiness.md`.

The final regression passed at 17:16:02 and the default ephemeral E2E passed
again at 17:16:21. Current intentional demo state is one Ready platform cluster,
three diagnosis rows and one remediation plan. All Compose services are healthy.

## Latest Real kind Validation

The previously blocked Kubernetes environment gate is now complete. A real
`kind-aiops-test` cluster running Kubernetes v1.34.0 accepted the managed-cluster
RBAC and repeatable `aiops-demo` workloads. The platform imported a short-lived
ServiceAccount kubeconfig, reported all three readiness Conditions True, read
core resources and Events, matched all three deterministic diagnosis rules,
and completed confirmed/idempotent Deployment rollout remediation. RBAC denied
Pod deletion and cross-namespace Deployment patch. The unprivileged Nginx fix,
exact results, cleanup contract and apply-manager pitfalls are archived in
`docs/changes/2026-07-17-real-kind-diagnosis-remediation.md`.

The 2026-07-17 user-management stage passed all Go tests and build, frontend typecheck, 19 Vitest tests and production build, PostgreSQL/API concurrency checks, and browser workflow verification. Temporary users and processes were removed after validation.

## Latest Kubernetes Compatibility Work

Service diagnosis now prefers bounded `discovery.k8s.io/v1` EndpointSlice reads, converts ready/not-ready addresses into the existing evidence shape, and falls back to core/v1 Endpoints only on discovery 404. Permission and transport failures remain visible. Focused tests cover ready true/false/nil, multi-address counts, fallback and non-fallback errors. Full verification is recorded in `docs/changes/2026-07-17-endpointslice-compatibility.md` and ADR 0016.

This earlier environment limitation is superseded by the 2026-07-17 real kind validation described above.

## Latest Cluster Security Work

System administrators can replace a registered cluster kubeconfig through `PUT /clusters/{id}/credentials` and the cluster UI. Parsing/encryption happens before an atomic database swap; stale probe fields are cleared, Conditions become Unknown, and the cached client is invalidated only after commit. Invalid input leaves the original API server and credential unchanged. Real API/DB verification confirmed encrypted storage, no audit secret leakage, explicit-probe state, and success/failure/denied audit results. See `docs/changes/2026-07-17-cluster-credential-rotation.md` and ADR 0017.

## Latest Account Security Work

All authenticated roles can change their own password from `/account/security`. Current-password verification, reuse rejection and compare-and-swap storage protect against stale concurrent updates. Success increments `auth_version`, revokes all refresh sessions, clears the cookie and forces re-login. Real API verification confirmed old access/refresh/password rejection, new-password login, success/error audit outcomes and zero password leakage. See `docs/changes/2026-07-17-self-service-password-change.md` and ADR 0018.

The same page now lists the user's active refresh sessions with a current marker and supports revoking one or all other sessions. Repository transactions require the active current Cookie summary and scope every row by user ID. Real two/three-session verification passed; see `docs/changes/2026-07-17-session-device-management.md` and ADR 0019.

## Latest Observability and Contract Work

The service now exposes `GET /metrics` with Prometheus-compatible counters and
duration aggregates. Labels are restricted to method, registered Gin route
template and status class; raw paths, identifiers, query strings and request
bodies are excluded. The scrape endpoint is intentionally unauthenticated and
must be bound or firewalled to a trusted monitoring network. A hand-reviewed
OpenAPI 3.0.3 baseline covers all current public/auth/user/cluster/Kubernetes/
diagnosis/AI/audit route families. Focused HTTP tests and YAML parsing passed.
See `docs/changes/2026-07-17-openapi-and-http-metrics.md`, ADR 0020 and
`docs/api/openapi.yaml`.

This stage's verification passed: `go test ./...`, `go build ./cmd/server`,
OpenAPI route drift and YAML validation, frontend `pnpm typecheck`, frontend
Vitest (8 files / 25 tests), and frontend `pnpm build`.

## Latest Contract and Container Work

`TestRegisteredRoutesMatchOpenAPI` now registers the full conditional Gin
router and compares its method/path set in both directions with the OpenAPI
document. A route changed on only one side fails the normal Go suite. Container
build drift was also corrected: the backend builder now matches `go.mod` at Go
1.25, the frontend explicitly installs `pnpm@11.7.0`, and the workspace policy
file is present before install so the allowed esbuild lifecycle script runs.

The frontend image built and served the SPA with HTTP 200. A real local API
process against PostgreSQL passed live/ready and verified that a concrete user
identifier does not enter route-template metrics. Docker Hub initially reset
OAuth connections, but a later retry pulled the bases and successfully built
the non-root backend image. See
`docs/changes/2026-07-17-contract-and-container-build-gates.md`.

## Latest Completed Work

Administrator password reset with full session invalidation is complete. Migration `000011_user_auth_version` adds a monotonic credential version; reset writes the bcrypt hash, increments the version and revokes refresh tokens in one transaction. Old access tokens, refresh sessions and passwords were rejected in real PostgreSQL/API verification. The system-admin UI exposes reset for other users, and `user.password.reset` records success/failure without the request body.

Verification baseline after this stage: all Go tests and server build, frontend typecheck, 20 Vitest tests and production build. See `docs/changes/2026-07-17-admin-password-reset.md` and ADR 0015.

## Recommended Next Work

The Git baseline, private remote, hosted CI and dependency governance are
already archived. Next, register the isolated `aiops-kind` runner, evaluate
OIDC/MFA, then validate signed audit archives, production backup/PITR and HA
behavior. Application-key re-encryption is accepted locally and in hosted CI.
Only after those reviews should the
project choose a registry identity, artifact-signing policy, license and formal
release tag. Keep MFA/SSO as a separate identity-provider project.

## Controlled Remediation Contract (Archived)

Started after the durable-notification milestone. The intended first action is
a bounded Deployment rollout restart linked to a confirmed Pod diagnosis. The
preview must use Kubernetes server-side dry-run, persist an expiring plan and
return a one-time confirmation token whose hash alone is stored. Execution must
require the token plus an idempotency key, enforce the captured target identity
and resource version, use an exact allowlisted patch, and append an audit
result. No arbitrary manifest, path, verb or patch body may cross the API
boundary. Implementation and isolated verification are complete; the next
environment gate is applying the target-cluster RBAC and platform deployment
to a real kind or safe Kubernetes context.

## Latest Kubernetes Deployment Work

`deploy/kubernetes/` now contains a Kustomize baseline with namespace, service
accounts, PostgreSQL StatefulSet/PVC, backend and frontend Deployments,
ClusterIP Services, TLS Ingress and default-deny NetworkPolicies. The backend
and database are not Ingress targets; only frontend pods may call the API,
while a labeled in-cluster monitoring namespace may scrape `/metrics`. The
Secret template is deliberately excluded from the default Kustomization and
contains only replacement markers. Application pods run non-root with probes,
resource limits, dropped capabilities and read-only roots. See ADR 0021 and
`docs/changes/2026-07-17-kubernetes-deployment-baseline.md`.

Offline Kustomize rendering produced 16 resources and deployment-manifest
checks pass in `go test ./...`. The frontend image was rebuilt and served HTTP
200 as UID/GID 101 with read-only root plus writable tmpfs mounts. Actual kind
apply remains unverified because kind is not installed and kubectl has no
current context. Both application images now build successfully. A complete
Compose smoke run reached healthy PostgreSQL/backend/frontend containers and
verified direct health, frontend API proxy, backend metrics, and that the
frontend does not proxy `/metrics`; application containers were removed after
verification while the development PostgreSQL container was retained.

Final regression for this stage passed `go vet ./...`, `go test ./...`,
`go build ./cmd/server`, frontend typecheck, 8 Vitest files / 25 tests,
frontend production build, `docker compose config --quiet`, both Docker image
builds, Kustomize rendering (16 resources) and the Compose runtime smoke checks
above. With notifications disabled by default, the rebuilt containers reached
healthy status; live, ready, SPA, frontend API proxy, backend metrics and
frontend non-proxy behavior all returned the expected HTTP results.

The controlled-remediation regression then passed the same Go vet/test/build
gate, frontend typecheck, 8 Vitest files / 26 tests, frontend production build,
`docker compose config --quiet`, both rebuilt application images, platform
Kustomize rendering (16 resources), managed-cluster RBAC Kustomize rendering
(5 resources), and a Compose health smoke with backend/frontend healthy and
live/ready/SPA/API-proxy checks returning 200. `kubectl apply --dry-run=client`
was not claimed because this machine has no usable Kubernetes context; offline
Kustomize rendering is the available manifest gate.

## Latest Diagnosis Notification Work

Migration `000012_diagnosis_notification_outbox` adds a singleton enable flag,
durable delivery rows and an after-trigger that emits only
`diagnosis.created`, `diagnosis.status_changed` and `diagnosis.assigned`. The
worker claims rows with `FOR UPDATE SKIP LOCKED`, signs the exact JSON body with
HMAC-SHA256, rejects redirects, retries with capped exponential backoff and
marks terminal failures `dead`. Payloads are an explicit allowlist and never
contain evidence, workflow comments, credentials or response bodies.

The Event Center exposes safe delivery metadata to system administrators and
security auditors. Only system administrators can retry a dead row; the retry
is audited as `notification.delivery.retry`. Notification configuration is
disabled by default, requires an HTTPS URL and a 32-character secret in
production, and is wired into Compose/Kubernetes templates without putting the
secret in ConfigMap.

Verification included focused Go/config/HTTP tests, frontend API/UI tests,
OpenAPI route drift coverage, real PostgreSQL trigger checks for all three
events, authenticated API checks (including disabled retry and audit outcome),
and a loopback webhook smoke in which all three rows reached `delivered` with
valid signatures. QA rows, the receiver, generated binary and enabled setting
were cleaned up after the smoke. See ADR 0022 and
`docs/changes/2026-07-17-durable-diagnosis-notifications.md`.

An archive review caught and fixed the combined-update edge case: when one SQL
statement changes both diagnosis status and assignee, the trigger now appends
both event types instead of prioritizing one. A real PostgreSQL transaction
confirmed one created, one status-changed and one assigned event, then rolled
back to leave the database clean. The full Go suite and backend image build
passed again after this correction.

## Latest Controlled Remediation Work

ADR 0023 and migration `000013_controlled_remediation` define the first
mutation primitive: `deployment.rollout_restart`. It is available only for a
confirmed Pod diagnosis and a Deployment in the same namespace whose selector
matches the current Pod labels. Preview captures UID/resourceVersion, creates
the exact server-generated annotation patch and submits Kubernetes
`dryRun=All`; only an accepted dry-run creates an expiring plan.

Execution stores only a SHA-256 confirmation-token hash, requires an
`Idempotency-Key`, atomically claims the plan and reuses the captured
resourceVersion. Same-key replay returns the stored result without another
PATCH; a stale lease can recover after a process failure. Different keys,
expired plans, invalid tokens and changed targets are rejected. The API never
accepts a Kubernetes path, verb, raw patch or manifest. System/operations
administrators can write; all logged-in roles can read safe plan metadata.

The target-cluster RBAC example under `deploy/managed-cluster/` keeps the
observer role read-only and grants Deployment `get`/`patch` only in an
explicitly approved namespace.

Real isolated verification completed against PostgreSQL and a TLS Kubernetes
stub: the primary scenario produced one dry-run and one real PATCH; same-key
replay produced no second PATCH; different-key/invalid-token/expired-plan
returned 409/403/410; stale same-key recovery succeeded; viewer read/write
authorization and success/failure/denied audit entries were confirmed. QA
resources and processes were removed afterward. See ADR 0023 and
`docs/changes/2026-07-17-controlled-remediation.md`.

## M19 Controlled Operations Catalog

ADR 0024 and migration `000014_controlled_operations_catalog` preserve the
diagnosis-bound `deployment.rollout_restart` flow and add exactly three
resource-originated actions: `deployment.scale`, `cronjob.suspend` and
`cronjob.resume`. Resource preview accepts only action, Namespace, target name
and the typed desired replica count required by scale. Unknown or irrelevant
fields, replica counts outside 0..1000 and no-change requests are rejected.

Every plan captures the current UID/resourceVersion and typed before/after
value, uses a complete server-generated patch, and must pass Kubernetes
server-side dry-run before persistence. Execution reuses the one-time token,
idempotency, stale-claim recovery, audit and sanitized-result controls from ADR
0023. Resource operation history is bounded to 50 and readable by authenticated
roles; only system and operations administrators may preview or execute. The
remediator Role grants namespaced Deployment and CronJob `get`/`patch`, while
the observer remains read-only and `kube-system` mutation remains denied.

The full M19 gate passed with 128 Go test entries, 12 Vitest files / 56 tests,
Kustomize 16/5/22/3 and three healthy Compose services. Real kind v1.34.0
verified Deployment scale plus same-key replay and restoration, CronJob
resume/suspend plus restoration, all seven diagnoses and the positive/negative
RBAC matrix. Desktop and 390x844 browser checks passed with one overlay
scrollbar and no warning/error logs. Evidence is archived in
`.artifacts/verification/verify-20260727-180428.json`,
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json` and
`docs/changes/2026-07-27-controlled-operations-catalog.md`.

Deployment rollback is explicitly deferred. The gateway does not yet expose
exact ReplicaSet revision and Pod-template history, so preview cannot bind to
an immutable selected revision without accepting unsafe client-owned patch
content. See ADR 0024 for the admission requirements.

## M20 Bounded Fleet Health

ADR 0025 adds authenticated `GET /api/v1/fleet/health` with hard limits of 20
enabled clusters, four concurrent cluster workers, four seconds per cluster and
100 sampled Nodes, Pods, Deployments and Events. Reads remain fixed and
sequential inside each worker. Each cluster reports health counts, explicit
sample coverage, Warning count, latency and sanitized failure scopes/codes.
Timeout, truncation or one resource failure stays local to that item; only the
platform cluster-directory read can fail the whole request.

Dashboard renders the response as one compact comparison table and can switch
the existing selected-cluster cockpit from a row. It does not move Kubernetes
fan-out into the browser, introduce arbitrary GVK/path input or widen target
RBAC. The retained runtime validates one real kind data path; deterministic
tests validate ordering, two-worker concurrency, timeouts and partial failure.
Phase 2 now adds physically distinct real-cluster evidence:
`scripts/e2e-fleet-kind.ps1` creates two kind clusters and an isolated platform
runtime, verifies direct resource counts, ordering, limits, timeout, recovery,
unavailable isolation and RBAC, then requires complete cleanup. See
`docs/changes/2026-07-27-bounded-multi-cluster-health.md` and
`docs/changes/2026-07-27-two-cluster-fleet-e2e.md`.

Phase 3 adds ADR 0026 and `GET /api/v1/fleet/resources/search`. It searches only
Pod, Deployment, Service and Ingress by bounded name substring and optional
Namespace, reports known-result and enabled-cluster coverage separately, and
localizes fixed-kind failures. `/search` keeps the reviewed query shape in the
URL and opens matches in the existing Workloads drawer. Phase 4 now adds the
owner-scoped persistence contract from ADR 0027 without expanding the query
shape. See `docs/changes/2026-07-27-bounded-global-resource-search.md` and
`docs/changes/2026-07-27-user-owned-global-search-filters.md`.

Phase 5 adds physically distinct real-cluster evidence through
`scripts/e2e-global-search-kind.ps1`. Its isolated runtime validates fixed
kinds, stable cluster/kind/Namespace/name ordering, result and cluster
coverage, truncation, timeout/recovery/query-failure isolation, observer RBAC
and complete cleanup. See
`docs/changes/2026-07-27-two-cluster-global-search-e2e.md`.

## Latest Diagnosis Rule Expansion

M7 adds two deterministic Pod rules without widening the Kubernetes write
boundary: `pod.pending.v1` matches the Pending phase with scheduling conditions
and Warning Events, while `pod.oom_killed.v1` matches current or previous
container termination with reason `OOMKilled`. The rule chain evaluates these
specific conditions before ImagePullBackOff and CrashLoopBackOff, preserving
the most actionable root cause when a container is both OOM-killed and in a
restart backoff. The Workloads view now exposes diagnosis actions for Pending
and OOMKilled Pods. Unit coverage and documentation are archived in
`docs/changes/2026-07-26-diagnosis-rule-expansion.md`.

## Latest Node and Deployment Diagnosis Expansion

M8 adds `node.not_ready.v1` and `deployment.replicas_unavailable.v1`. Node
diagnosis treats a missing or non-True Ready Condition as a match and preserves
the complete Condition set. Deployment diagnosis compares the Kubernetes
default-aware desired replica count with Ready and Available replicas while
retaining all rollout counters. The shared diagnosis API accepts Node without a
Namespace and still requires Namespace for Pod, Service and Deployment. No new
mutation path was added. Full verification and scope are archived in
`docs/changes/2026-07-26-node-deployment-diagnosis.md`.

## Important Invariants

- Deterministic diagnosis remains usable when AI is disabled or fails.
- Sensitive values never enter responses, audit details, logs or Git.
- ConfigMap values and StorageClass parameters never enter public Kubernetes resource responses.
- Secret values, labels and annotations never enter public responses; Secret observer RBAC still requires explicit threat acceptance because Kubernetes cannot field-filter the raw object.
- User disable/role removal is effective on the next authenticated request.
- At least one active `system_admin` must always remain.
- Password reset must invalidate both refresh sessions and already-issued access tokens.
- Audit and diagnosis histories are append-only through business APIs.
- Kubernetes mutation is limited to the fixed catalog: diagnosis-bound Deployment rollout restart plus resource-originated Deployment scale and CronJob suspend/resume; no arbitrary write proxy exists.
- Every operation preview must pass server-side dry-run, capture target UID/resourceVersion and a typed before/after value, and expire without execution.
- Operation execution requires a one-time token, an idempotency key and a matching target precondition; repeated same-key calls do not repeat the Kubernetes patch.
- Deployment rollback remains unavailable until an immutable ReplicaSet revision/template snapshot can be selected and bound to preview and execution.
- Fleet queries must remain bounded by reviewed cluster concurrency, per-cluster timeout and per-kind sample limits; partial or truncated data cannot be labeled healthy.
- Global search must remain limited to the reviewed name/Namespace/four-kind query shape; omitted clusters, truncated results and fixed-kind failures cannot be labeled complete.

## Local Verification

PostgreSQL development container is `k8s-aiops-postgres-1`, exposed on `localhost:15432`. Before handoff, run:

```powershell
.\scripts\verify.ps1
.\scripts\e2e-fleet-kind.ps1
.\scripts\e2e-global-search-kind.ps1
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
.\scripts\e2e-kind.ps1
```

The verify script uses a compatible host Go when available and otherwise runs
Go 1.25 in Docker with the repository mounted read-only. `frontend/dist` and
`.artifacts` are generated outputs and must not be treated as source or
committed. The development PostgreSQL container remains healthy on port 15432;
saved-filter acceptance rows are zero and migration 000015 is the latest
applied version.

## Next Priorities After Current Work

1. Revisit required `Backend`, `Frontend`, `Manifests` and `Compose runtime`
   checks when the private repository plan supports branch protection, or
   after an explicitly approved public-repository decision.
2. Register a dedicated non-production `aiops-kind` runner using
   `docs/ci-release.md`.
3. Continue production hardening with OIDC/MFA evaluation, signed audit
   archives, production backup/PITR policy and HA validation; isolated logical
   restore and application-key re-encryption gates are complete locally.
4. Decide registry identity, artifact signing and provenance only after the
   release actor and key-management policy are reviewed.
5. Evaluate explicit saved-filter ordering/pinning only if repeated operator
   evidence justifies it; sharing, schedules and alerts remain out of scope.
6. Re-capture revision-bound screenshots and rehearse the M20 defense flow
   after security, backup/restore and HA gates are accepted.

## Real kind Final Verification

Fresh verification on 2026-07-17 passed backend format/vet, all Go packages
with cache disabled, and server build. Frontend typecheck passed, Vitest passed
8 files / 26 tests, and the production Vite build completed. Kustomize rendered
16 platform resources, 5 managed-cluster RBAC resources and 7 demo resources;
Compose configuration and real-cluster server-side dry-runs passed. The final
kind state retained two Ready control workloads plus the intended
CrashLoopBackOff and ImagePullBackOff workloads. The temporary platform cluster
row is zero, the generated credential file contains no credential material,
and the validation API process is stopped.
