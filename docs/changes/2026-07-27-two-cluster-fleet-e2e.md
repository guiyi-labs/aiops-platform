# M20 Phase 2 Disposable Two-Cluster Fleet E2E

- Status: Accepted
- Date: 2026-07-27
- Scope: physically distinct kind fan-out, fault isolation, recovery and cleanup evidence

## Outcome

M20 Phase 2 adds `scripts/e2e-fleet-kind.ps1`, a self-contained gate for ADR
0025. The script creates two physically distinct single-node kind v1.34.0
clusters, a private Docker network, a fresh PostgreSQL container and a backend
image built from the current source. It bootstraps the isolated platform with
random run-scoped keys and credentials, so the gate does not read the retained
administrator password, use the development database or modify demo records.

Each kind cluster receives the existing observer RBAC and a separate 20-minute
ServiceAccount token kept only in process memory. Cluster A has one scale-zero
fixture Deployment and cluster B has three. This makes their resource totals
distinguishable without pulling workload images. The platform result is checked
against direct fixed Node, Pod, Deployment and Event API totals for both
clusters.

The gate verifies:

1. anonymous fleet access is 401 and an authenticated invalid limit is 400;
2. two enabled clusters are sorted by platform ID, while `limit=1` returns the
   lowest ID with `total=2` and `remaining=1`;
3. advertised limits remain 20 clusters, four workers, four seconds and 100
   objects per resource kind;
4. both baselines have complete, failure-free samples matching direct resource
   reads;
5. pausing cluster B produces four sanitized `TIMEOUT` failures and
   `timed_out` at 4003ms while cluster A remains available;
6. unpausing cluster B restores its counts, then stopping it produces four
   `QUERY_FAILED` failures and `unavailable` while cluster A remains available;
7. observer RBAC can list Nodes and Events but cannot create Deployments;
8. platform records, both kind clusters, backend/PostgreSQL containers, Docker
   network, temporary backend tag and all credential files are removed.

No new ADR is required because the runtime fan-out contract did not change;
this gate supplies the real multi-cluster evidence required by ADR 0025.

## Accepted Evidence

The accepted run is
`.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json`:

- kind v0.30.0 with two Kubernetes v1.34.0 control planes;
- cluster IDs `[1, 2]` in stable order;
- baseline A: 1 Node, 9 Pods, 3 Deployments, 59 Events and 5 Warnings;
- baseline B: 1 Node, 9 Pods, 5 Deployments, 58 Events and 4 Warnings;
- direct and platform totals matched for every fixed resource kind;
- timeout, recovery and unavailable isolation all passed;
- all eight cleanup assertions are `true`, and the pre-existing `aiops-test`
  kind cluster set was preserved.

The post-archive full gate is
`.artifacts/verification/verify-20260727-194724.json` (223.18 seconds). Go vet,
all backend packages and the server build passed; frontend typecheck, 13 Vitest
files / 57 tests and the production build passed. PostgreSQL, backend and
frontend were healthy, Kustomize rendered 16/5/22/3 resources, and backend,
frontend and proxy runtime checks passed.

Event totals are observations from this run, not fixed fixture expectations.
The script always derives expected totals directly before comparing the fleet
response. Evidence contains no password, access token, ServiceAccount token,
kubeconfig, CA data, database URL or private key.

## Reproduction

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-fleet-kind.ps1
```

The default kind image is pinned by v1.34.0 digest. A different reviewed image
may be supplied with `-KindNodeImage`. The run writes only a sanitized JSON
summary to `.artifacts/fleet-e2e` and fails if the initial kind cluster set is
not restored.
