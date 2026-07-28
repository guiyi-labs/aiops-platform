# Change Record: Real kind diagnosis and remediation validation

- Date: 2026-07-17
- Scope: Complete the previously interrupted real Kubernetes validation stage

## Delivered

- Added `deploy/demo-scenarios` as a repeatable, namespace-isolated fixture for
  a healthy workload plus ImagePullBackOff, CrashLoopBackOff and Service without
  ready endpoints.
- Added deployment contract tests that require all three deterministic fault
  shapes and require the non-fault Nginx workloads to use the unprivileged image
  on port 8080.
- Aligned the namespaced remediation Role example with `aiops-demo` while
  retaining only Deployment `get` and `patch`.
- Documented the correct Namespace-first server-side dry-run order and the
  reason actual demo updates consistently use client-side apply.

## TDD evidence

1. `TestDemoScenariosCoverTheThreeDeterministicDiagnoses` first failed because
   `deploy/demo-scenarios/kustomization.yaml` did not exist.
2. After the initial manifests passed the contract test, the real cluster
   showed that the two intended healthy Nginx Deployments also entered
   CrashLoopBackOff. Previous logs identified the exact failure:
   Nginx attempted to `chown` its cache while all Linux capabilities were
   dropped.
3. The contract test was extended first and failed on `nginx:1.27-alpine`.
   The manifests then moved to `nginxinc/nginx-unprivileged:1.27-alpine` and
   container port 8080; both intended healthy Deployments rolled out Ready.

## Real cluster evidence

The isolated cluster was `kind-aiops-test`, Kubernetes `v1.34.0`. The platform
used a one-hour ServiceAccount token and the repository RBAC examples.

- Cluster creation, enable and explicit probe returned Ready with
  `CredentialValid=True`, `Reachable=True` and `Ready=True`.
- The platform read one demo Namespace, four current Pods, four Deployments,
  two Services and live Events through the encrypted imported credential.
- Diagnosis IDs 16, 17 and 18 matched respectively:
  `pod.image_pull_backoff.v1`, `pod.crash_loop_backoff.v1` and
  `service.no_ready_endpoints.v1`.
- Diagnosis 16 was confirmed. Remediation preview passed Kubernetes dry-run and
  created plan `0fbc1b5f-d74b-4dcf-a365-78adceeeff8a`.
- Execution added only the server-generated
  `k8s-aiops.local/remediation-id` and `k8s-aiops.local/restarted-at`
  annotations. Replaying the same idempotency key returned the same succeeded
  plan without a second mutation.
- RBAC checks allowed cluster-wide reads, Pod logs and Deployment patch only in
  `aiops-demo`; Pod deletion and Deployment patch in `kube-system` were denied.
- Audit queries contained cluster create/enable/probe, three diagnosis runs,
  remediation preview and both idempotent execute requests.

## Issues found during real validation

- A combined server-side dry-run cannot create an ephemeral Namespace for
  subsequent objects in the same request sequence. The Namespace is now
  applied first.
- Switching an existing object from client-side actual apply to server-side
  apply while changing a named container port produced an old/new list merge
  conflict. The documented local workflow keeps the apply manager consistent.
- Capability-dropped stock Nginx was not a healthy control workload. The demo
  now uses the non-root Nginx distribution instead of weakening the security
  context.

## Cleanup and security

The generated kubeconfig was stored only under ignored `.tools`; after use its
credential content was overwritten with a non-sensitive regeneration note. The
platform cluster row was deleted through the API, which removed its encrypted
credential and cascade-owned QA diagnosis/remediation rows. The developer-local
kind cluster, demo resources and temporary RBAC remain available for inspection;
the documented cleanup commands can remove them when they are no longer needed.
No token or kubeconfig content is included in this archive.
## Final verification

- Backend: `go fmt ./...`, `go vet ./...`, `go test -p=1 -count=1 ./...`
  and a fresh server build all exited successfully.
- Frontend: typecheck, 8 Vitest files / 26 tests and production build passed
  with Node v24.14.0.
- Deployment: Kustomize rendered 16 platform, 5 managed-cluster and 7 demo
  resources; Compose configuration and real-cluster server-side dry-run passed.
- Runtime: the two control workloads were Ready; fault Pods were respectively
  CrashLoopBackOff and ImagePullBackOff; the platform cluster row count was 0
  after cleanup and the temporary API process was stopped.
- Security scan found no private key, embedded certificate/token, JWT or Bearer
  credential material in tracked project sources and documents.