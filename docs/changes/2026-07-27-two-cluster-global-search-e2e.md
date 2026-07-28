# M20 Phase 5 Disposable Two-Cluster Global Search E2E

- Status: Accepted
- Date: 2026-07-27
- Scope: physically distinct fixed-kind search, bounded coverage, fault isolation and cleanup evidence

## Outcome

M20 Phase 5 adds `scripts/e2e-global-search-kind.ps1`, a self-contained real
cluster gate for the ADR 0026 search contract. It creates two physically
distinct single-node Kubernetes v1.34.0 kind clusters, a private Docker
network, fresh PostgreSQL and a backend image built from the current source.
Random run-scoped platform credentials and separate 20-minute observer tokens
keep the run isolated from retained development data and credentials.

Both clusters receive Pod, zero-replica Deployment, Service and Ingress names
matching `search` in Namespace `search-e2e`; cluster B receives one extra Pod.
This gives nine controlled matches and makes cluster attribution, fixed kind
order and global truncation observable without an ingress controller or
additional platform persistence.

The gate verifies:

1. anonymous search is 401, while short queries, unknown kinds and invalid
   cluster/result limits are 400;
2. omitted `cluster_limit` searches both enabled clusters and reports coverage
   `2/2/0`, while `cluster_limit=1` retains the lowest platform ID and reports
   one omitted cluster;
3. all nine results follow stable cluster ID, kind, Namespace and name order;
   only Pod, Deployment, Service and Ingress can appear;
4. selecting `services,pods` is normalized to the fixed Pod/Service order, and
   `limit=3` returns a stable three-item prefix with `total=9`, `remaining=6`
   and `complete=false`;
5. pausing cluster B yields four localized `TIMEOUT` failures while all four
   cluster A results remain usable;
6. unpausing restores nine complete results, then stopping cluster B yields
   four localized `QUERY_FAILED` failures while cluster A remains usable;
7. observer RBAC can list each fixed resource kind and cannot create any of
   them in the fixture Namespace;
8. platform records, both kind clusters, backend/PostgreSQL containers, Docker
   network, temporary backend image tag and credential files are removed, and
   the pre-existing `aiops-test` kind cluster is preserved.

No new ADR is required because this phase does not change the accepted search
contract. It adds the missing physical two-cluster evidence for ADR 0026.

## Accepted Evidence

The accepted run is
`.artifacts/search-e2e/search-e2e-20260727-225358.json`:

- kind v0.30.0 with two Kubernetes v1.34.0 control planes;
- cluster IDs `[1, 2]` in stable order;
- nine complete fixed-kind matches with coverage `total=2`, `searched=2`,
  `remaining=0`;
- selected-kind total 5, cluster-limit coverage `2/1/1`, and result truncation
  `total=9`, `returned=3`, `remaining=6`;
- four `TIMEOUT` failures, complete recovery, then four `QUERY_FAILED`
  failures, with four healthy-peer matches retained in both fault states;
- read=`yes` and create=`no` for all four fixed kinds on both clusters;
- all eight cleanup assertions are `true`, including preservation of the
  pre-existing kind cluster set.

The artifact contains no password, access token, ServiceAccount token,
kubeconfig, CA data, database URL, API server or private key.

## Reproduction

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-global-search-kind.ps1
```

The default kind image is digest-pinned. The script writes only a sanitized
JSON summary to `.artifacts/search-e2e` and fails if any disposable runtime
asset remains or the initial kind cluster set changes.

## Final Verification

The post-archive full gate passed at 2026-07-27 23:02:04 +08:00. Evidence is
`.artifacts/verification/verify-20260727-230204.json` (158.94 seconds). Go
format/vet/all packages/server build passed with 151 `Test*` entries; frontend
typecheck, 14 Vitest files / 59 tests and production build passed. PostgreSQL,
backend and frontend were healthy, Kustomize rendered 16/5/22/3 resources, and
backend/frontend/proxy runtime checks passed.

The final sensitive-material scan excluded `.git`, `.tools`, `.artifacts`,
`node_modules` and `dist` and returned zero matches. Compose logs returned zero
panic/fatal/error matches. The development database contained zero
`search-e2e-*` cluster rows and zero saved filters; no disposable kind cluster,
container, network, image tag or temporary directory remained. The retained
`aiops-test` cluster and three healthy Compose services were preserved, and the
rebuilt backend was reconnected to the Docker `kind` network.

## Boundaries And Follow-Up

This phase does not add arbitrary GVKs, selectors, Kubernetes API paths, raw
objects, writes, saved results, cross-user behavior, schedules or alerts. The
next priority is a human-reviewed initial Git baseline and versioned release
pipeline, followed by OIDC/MFA evaluation, application-key re-encryption,
signed audit archives, backup/restore and HA validation.
