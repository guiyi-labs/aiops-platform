# Change Record: Defense demo readiness

- Date: 2026-07-26
- Scope: Retained real-cluster demo data, deterministic cleanup and thesis screenshots
- Result: Passed

## Delivered

- Added `scripts/demo-up.ps1` to prepare a populated defense environment through the real platform API and Kubernetes API.
- Added `scripts/demo-down.ps1` to delete only `demo-kind-*` platform clusters, with optional removal of the `aiops-demo` Namespace and managed-cluster RBAC.
- Extended `scripts/e2e-kind.ps1` with `KeepPlatformCluster`. The default remains ephemeral and cleanup-first; retained mode keeps the platform cluster only after the entire E2E succeeds. A failed retained run still deletes its cluster in `finally`.
- Added cold-start-safe Pod state polling. Kubernetes Pods may briefly expose running or terminated state before the expected waiting reason exists; strict PowerShell property access no longer aborts during that transition.
- Added dependency-free screenshot capture using system Edge/Chrome and standard Chrome DevTools Protocol. No Playwright package or browser download is required.
- Added four thesis screenshots plus capture metadata under `docs/thesis/screenshots`.
- Added `docs/thesis/demo-environment.md` and updated the 10-minute defense script.

## Full cleanup validation

`scripts/demo-down.ps1 -CleanupDemoResources` deleted:

- platform cluster `demo-kind-20260726-170132` and cascade-owned diagnosis/remediation rows;
- ServiceAccount `kube-system/aiops-platform`;
- observer ClusterRole/ClusterRoleBinding;
- namespaced remediation Role/RoleBinding;
- Namespace `aiops-demo` and all demo workloads.

Post-cleanup database counts were `clusters=0`, `diagnoses=0` and `remediations=0`. The Namespace and ClusterRole were absent before the next preparation run.

## Cold-start preparation validation

A fresh `scripts/demo-up.ps1` run recreated the complete environment from zero and completed at 2026-07-26 17:06:02 +08:00:

- retained platform cluster ID 23, name `demo-kind-20260726-170601`, status Ready;
- Kubernetes v1.34.0, 4 current demo/control Pods at diagnosis time and 2 Services;
- diagnoses 25/26/27 matched ImagePullBackOff, CrashLoopBackOff and Service no-ready-endpoints rules;
- remediation `dcd4f6eb-201e-47ad-866e-5a5fc43243a3` succeeded and same-key replay returned the same plan;
- RBAC results remained yes/yes/no/no for read Pods, patch demo Deployments, delete demo Pods and patch system Deployments;
- evidence: `.artifacts/e2e-kind/e2e-kind-20260726-170601.json` and `.artifacts/demo/demo-ready-20260726-170602.json`.

The retained ServiceAccount credential expires after one hour. `demo-up` must be rerun shortly before a formal demonstration.

## Screenshot validation

`scripts/capture-thesis-screenshots.ps1` logged in through the real frontend, captured a 1440x1000 viewport and destroyed its temporary browser profile afterward.

- `01-dashboard.png`: one registered cluster, three diagnoses and recent real activity.
- `02-clusters.png`: retained kind cluster Ready on Kubernetes v1.34.0.
- `03-workloads.png`: live cluster-wide Pod list including Running, CrashLoopBackOff and ImagePullBackOff states.
- `04-diagnoses.png`: three rule histories with two open and one confirmed record.

All four images were visually inspected. Content was loaded, text fit its containers, controls did not overlap and no credential material was visible. `capture-metadata.json` records `uncommitted-baseline` because no initial Git commit exists.

## Security and remaining item

The administrator password is passed to child processes only through a temporary environment variable and removed by the PowerShell wrapper in `finally`. Browser cookies and profile data live only under ignored `.artifacts` and are recursively removed after capture. Screenshot metadata does not contain credentials.

The demo environment is intentionally left Ready at `http://localhost:18080` for inspection. The remaining release-freeze action is human approval of the initial Git commit, followed by one screenshot recapture so metadata contains the actual revision.

## Final regression

- `scripts/verify.ps1` passed at 2026-07-26 17:16:02 +08:00 in 135.2 seconds; backend packages, 8 frontend files / 26 tests, production builds, Compose health, Kustomize 16/5/7 and HTTP checks all passed. Evidence: `.artifacts/verification/verify-20260726-171602.json`.
- Default `scripts/e2e-kind.ps1` passed at 17:16:21 and reported `platform_cluster_deleted=true` with mode `ephemeral-e2e`, proving retained mode did not change the original cleanup contract. Evidence: `.artifacts/e2e-kind/e2e-kind-20260726-171621.json`.
- Final database state is intentionally `clusters=1`, `diagnoses=3`, `remediations=1`, all owned by retained Ready cluster `demo-kind-20260726-170601`.
- All Compose services are healthy, the temporary browser profile is absent, delivery asset contract tests pass and the sensitive-material scan has no matches.
