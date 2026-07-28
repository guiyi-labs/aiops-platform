# ADR 0024: Resource-originated controlled operations catalog

- Status: Accepted
- Date: 2026-07-27

## Context

ADR 0023 introduced a diagnosis-bound Deployment rollout restart. That flow is
intentionally coupled to a confirmed Pod diagnosis, selector validation and a
Deployment target. Deployment scale and CronJob schedule control are ordinary
resource operations and cannot be represented honestly as Pod remediation.

Adding a generic patch endpoint would bypass the existing safety properties and
turn the stored target-cluster credential into an arbitrary mutation channel.
Deployment rollback also cannot be implemented safely from the current public
model: the platform does not retrieve exact ReplicaSet revision/template history,
so it cannot preview and bind confirmation to a selected immutable revision.

## Decision

Keep `deployment.rollout_restart` diagnosis-bound. Add a separate resource-origin
preview endpoint with exactly three server-owned actions:

- `deployment.scale`, with an integer desired replica count from 0 through 1000;
- `cronjob.suspend`;
- `cronjob.resume`.

Every preview reads the exact target, captures UID/resourceVersion and the typed
before/after value, generates the complete strategic merge patch on the server,
and submits `dryRun=All`. Only an accepted dry-run creates a ten-minute plan.
Execution reuses ADR 0023 confirmation-token hashing, transactional claim,
idempotency key, stale-claim recovery, bounded failure text and audit behavior.
The patch includes both UID and resourceVersion. Unknown JSON fields, irrelevant
parameters, out-of-range replica counts and no-change operations are rejected.

The API never accepts a Kubernetes path, verb, raw patch, manifest or arbitrary
field name. Target-cluster mutation RBAC remains namespaced and grants only
`get`/`patch` for Deployments and CronJobs in explicitly approved namespaces.

Deployment rollback is deferred until the gateway exposes exact ReplicaSet
revision and Pod-template history and the plan can snapshot one selected
revision. It must not be simulated by accepting a client template or replaying
an arbitrary historical patch.

## Consequences

Operators can perform common reversible actions from resource details while the
platform retains a finite, reviewable write surface. Read-only roles can inspect
bounded operation history but cannot preview or execute. Adding another action
requires a new fixed request contract, typed persisted parameters, dry-run and
execution tests, RBAC analysis, real-cluster restoration and an ADR update.
