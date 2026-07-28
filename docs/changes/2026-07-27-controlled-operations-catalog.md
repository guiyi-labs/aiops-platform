# M19 Controlled Operations Catalog

- Status: Accepted
- Date: 2026-07-27
- Scope: fixed resource-originated operations, typed diff, namespaced RBAC, Workloads UI and real kind evidence

## Outcome

M19 preserves the confirmed-diagnosis rollout restart and adds three fixed
resource actions: `deployment.scale`, `cronjob.suspend` and `cronjob.resume`.
The resource preview API accepts only action, Namespace, target name and the
typed replica parameter required by scale. Strict JSON decoding rejects unknown
or irrelevant fields; there is no YAML editor, generic patch or arbitrary GVK
surface.

Migration `000014_controlled_operations_catalog` makes the diagnosis link
nullable only for resource-origin plans and stores typed before/after replica or
suspend values. Database checks bind each action to its valid parameter shape.
Plans continue to store only the confirmation-token hash, exact target UID and
resourceVersion, expiry, claim lease, idempotency key and bounded outcome.

The Kubernetes gateway adds one fixed CronJob patch path. Deployment and CronJob
patches carry UID/resourceVersion and use server-side dry-run before persistence.
Execution dispatches only by the persisted action. The managed-cluster Role now
grants namespaced `get`/`patch` for Deployments and CronJobs; observer permissions
remain read-only and delete, Secret creation, exec and cluster-system mutation
remain denied.

## Product Surface

Deployment detail provides a bounded numeric replica control. CronJob detail
shows exactly one state-aware suspend or resume command. A successful dry-run
opens a confirmation dialog with target, resourceVersion, field-level before and
after values, expiry and an explicit checkbox. Resource operation history is
bounded to 50 plans and readable by authenticated roles; only system and
operations administrators can preview or execute.

Browser acceptance used the real retained cluster at 1280x720 and 390x844. The
Deployment scale control, CronJob resume control, diff dialog and history were
visible; all controls stayed inside their containers with no horizontal overflow.
A discovered double-scrollbar issue was fixed by locking the underlying page
while an overlay is open. The final mobile drawer used one scrollbar and browser
warning/error logs were empty.

## Real Kind Evidence

The Kubernetes v1.34.0 run at
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json` passed all seven diagnosis
rules and rollout-remediation regression, then:

- scaled `healthy-nginx` from 1 to 2, replayed the same idempotency key without a
  second plan, and restored replicas to 1 through another controlled plan;
- resumed the initially suspended `demo-cleanup` CronJob and suspended it again;
- confirmed Deployment and CronJob patch permission in `aiops-demo`, denial in
  `kube-system`, Pod-delete denial and the existing observer checks;
- restored both fixture values in the main path and again in `finally`, removed
  the temporary pressure Node and deleted the ephemeral platform cluster record.

## Explicit Deferral

Deployment rollback is not included. The current gateway and public models do
not provide exact ReplicaSet revision/template history, so the platform cannot
bind a dry-run and confirmation to an immutable selected revision. Accepting a
client template or an arbitrary historical patch would violate the fixed-action
boundary. ADR 0024 records the evidence required before rollback can be added.

## Verification

- Full Go vet/test suite and server build: passed; 128 Go `Test*` entries.
- Frontend typecheck, 12 Vitest files / 56 tests and production build: passed.
- Compose migration/runtime: `000014` applied; three services healthy.
- Kustomize renders: `16/5/22/3`.
- Full gate: `.artifacts/verification/verify-20260727-180428.json`, 143.85 seconds.
- Real kind: `.artifacts/e2e-kind/e2e-kind-20260727-180557.json`.
- Backend warning/error/panic/fatal log scan after verification: zero matches.
