# 2026-07-17 Cluster Credential Rotation

## Scope

- Add system-admin-only `PUT /api/v1/clusters/{cluster_id}/credentials`.
- Validate and encrypt replacement kubeconfig before entering the database transaction.
- Atomically replace encrypted credential and API Server metadata while clearing stale probe output.
- Set Ready, Reachable and CredentialValid to `Unknown / CredentialsUpdated` until explicit probe.
- Invalidate the per-cluster cached client only after successful commit.
- Add `cluster.credentials.rotate` audit mapping and a credential replacement form to each manageable cluster card.

## Verification

- Go tests cover pre-storage validation, randomized encrypted output, current key version, Unknown Conditions and cache invalidation.
- Frontend typecheck and 21 Vitest tests pass, including the PUT request/body contract.
- Real PostgreSQL/API verification: invalid kubeconfig returned 400 and preserved the old API Server; valid rotation returned the new API Server with unknown status and no stale version/probe time.
- Database ciphertext did not contain the replacement token, key version was `v1`, and all three Conditions were `Unknown / CredentialsUpdated` after the semantic correction.
- Viewer access returned 403. Audit contained failure/400, success/200 and denied/403, with zero replacement-token matches in audit details.
- Final `go test ./...`, server build, frontend typecheck, 21 Vitest tests and production build all passed.

## Boundaries

- Rotation does not automatically probe or enable a cluster.
- The replacement kubeconfig still follows the onboarding restriction: embedded token or client certificate only, HTTPS server, no exec/auth-provider or local file paths.
- Application master-key bulk rotation is not implemented by this endpoint.
