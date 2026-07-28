# Change Record: Confirmed controlled remediation

- Date: 2026-07-17
- Scope: Deployment rollout restart preview, confirmation, execution and history

## Delivered

- Added migration `000013_controlled_remediation` and a plan repository with
  expiring confirmation, atomic execution claim, idempotent replay, stale-lease
  recovery and append-only operational history.
- Added the single allowlisted `deployment.rollout_restart` action for
  confirmed Pod diagnoses. Preview verifies current Pod UID, namespace and
  Deployment selector, captures target UID/resourceVersion, and requires a
  successful Kubernetes `dryRun=All` patch before persisting the plan.
- Added a one-time 256-bit confirmation token. Only its SHA-256 hash is stored;
  list responses omit the hash, raw token, idempotency key and generated patch.
- Added exact strategic-merge PATCH support to the internal Kubernetes gateway
  with a 4 KiB body limit, fixed content type, response limit and redirect
  rejection. No arbitrary mutation endpoint was exposed.
- Added plan list/preview/execute APIs, system/operations administrator write
  authorization, viewer-readable history, and `remediation.preview` /
  `remediation.execute` audit actions.
- Added an explicit frontend dry-run and confirmation flow inside diagnosis
  detail, including fixed target/resourceVersion display and execution history.
- Added least-privilege managed-cluster RBAC examples: cluster-wide observation
  remains read-only, while Deployment `get`/`patch` is granted per approved
  namespace only.

## Verification

- Focused Go tests cover exact PATCH transport, redirect rejection, server
  dry-run, selector/diagnosis boundaries, one-time token hashing, exact patch
  replay, idempotent stored results and sanitized failure text.
- Frontend typecheck and 8 test files / 26 tests passed after adding preview,
  list and execute contract coverage.
- A real PostgreSQL/API run against an isolated TLS Kubernetes stub created a
  diagnosed Pod, confirmed it, previewed and executed a Deployment restart.
  The stored plan had a 32-byte token hash and no raw token; the public list
  contained neither confirmation nor idempotency material.
- Same-key replay returned the stored success without an extra Kubernetes
  PATCH. Another key returned 409, a bad confirmation returned 403, an expired
  plan returned 410, and a stale same-key claim recovered successfully.
- The isolated Kubernetes stub observed four dry-runs and two intentional real
  executions across the positive, negative and stale-recovery scenarios. The
  primary success scenario observed exactly one dry-run and one real patch.
- Viewer plan history returned 200 while viewer preview returned 403. Audit
  rows covered success, failure and denied outcomes without bodies or secrets.
- QA cluster/user/diagnoses/plans, temporary credentials and test processes
  were removed after verification.

Final repository-wide build/container results are recorded in
`docs/development-handoff.md`.
