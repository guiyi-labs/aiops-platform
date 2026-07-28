# ADR 0023: Confirmed and idempotent controlled remediation

- Status: Accepted
- Date: 2026-07-17

## Context

The platform currently diagnoses failures but deliberately has no Kubernetes
write path. A generic proxy, arbitrary manifest endpoint or client-supplied
patch would turn stored cluster credentials into an unrestricted remote
execution capability. A simple confirmation dialog is also insufficient: it
does not prove that admission accepts the change, prevent duplicate execution,
or bind approval to the resource that was previewed.

## Decision

The first remediation action is the single allowlisted
`deployment.rollout_restart` operation. It is available only for a confirmed
Pod diagnosis and a Deployment in the same namespace whose selector matches
the currently observed Pod labels.

Preview reads both resources, captures Deployment UID/resourceVersion, builds
the complete restart annotation patch on the server, and sends it to the
Kubernetes API with `dryRun=All`. Only after admission accepts the dry-run is
an expiring plan persisted. A random confirmation token is returned once; only
its SHA-256 hash is stored.

Execution requires the confirmation token and an `Idempotency-Key`. The plan
repository atomically claims the plan, rejects expired or mismatched requests,
and permits stale recovery only for the same key. The exact previewed patch is
reconstructed from persisted fields and includes the captured resourceVersion.
Success/failure state and bounded error text are persisted. APIs never accept a
Kubernetes path, verb, raw patch, annotation or manifest from the client.

System and operations administrators may preview and execute. Every write API
is covered by platform audit; plan history is readable by all authenticated
roles without exposing the confirmation hash or raw token.

## Consequences

The platform gains a small, explainable mutation surface rather than general
cluster administration. Repeating an execution with the same key is safe and
returns the existing result; another key cannot execute the plan. A process
failure after the Kubernetes patch but before local success persistence can be
recovered because the same annotation values are replayed after the claim
lease expires.

This action does not claim to fix image names, application configuration or
Service selectors. It only requests a new ReplicaSet for a specifically
validated Deployment. Additional actions require separate allowlists, dry-run
semantics, risk analysis and ADR updates.
