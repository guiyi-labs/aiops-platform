# ADR 0069: Helm Application Catalog + Controlled Deploy Plans (M57)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M57
- Supersedes: none
- Related: ADR 0024 (resource-originated controlled operations),
  ADR 0041 (fixed cross-cluster promotion), ADR 0049 (route descriptor
  contract and RBAC inventory), ADR 0064 (CRD discovery and browsing),
  ADR 0008 (sanitized append-only audit trail)

## Context

Phase 3 of the post-M45 roadmap introduces "simplified Helm application
catalog" as M57. Before M57, the platform could browse Kubernetes
resources (M49 CRD discovery) and promote workloads across clusters (M19
cross-cluster promotion), but had no way for operators to discover, browse,
and deploy Helm charts in a controlled manner.

The roadmap requires that chart deployment follow the same M19
controlled-operation contract already used by promotion (M19), backup (M22),
restore (M23), maintenance (M24), and remediation (M13): preview builds a
plan with a confirmation token, execute is idempotent with an idempotency
key, and the plan has a bounded TTL. The deploy target is a Flux
HelmRelease CR (already in the M49 CRD whitelist), not a direct `helm
install` — this keeps the platform dependency-free (no Helm SDK) and
delegates reconciliation to the Flux helm-controller.

Key constraints from the project invariants:
- No Helm SDK dependency; chart metadata comes from index.yaml HTTP fetch.
- Credentials in `helm_repositories.credentials_json` are NEVER returned in
  API responses (ADR 0008 / project invariant: credentials never in
  API/UI/logs/audit).
- The controlled deploy reuses the M19 confirmation-token + idempotency-key
  + Claim state machine (mirrors promotion/backup/restore/maintenance).
- 404 > 403 anti-leakage: unauthorized access returns 404.

## Decision

### 1. No Helm SDK — index.yaml HTTP fetch only

Chart metadata is fetched live from each repository's `index.yaml` over
HTTP (read-only). The `HTTPIndexSource` implementation imposes a 10 MiB
body limit and a 15-second timeout. Basic-auth credentials are extracted
from the stored `credentials_json` JSONB column and passed to the HTTP
client. This avoids pulling in the Helm Go SDK, keeps the binary small,
and means the platform never caches stale chart metadata.

### 2. Flux HelmRelease CR as deploy target

The deploy preview builds a Flux `HelmRelease` CR manifest
(`helm.toolkit.fluxcd.io/v2beta1`) and validates it via a server-side
dry-run on the target cluster. The manifest is built once at preview time
and applied verbatim at execute time (deterministic — no re-rendering).
The HelmRelease GVR is already in the M49 `customResourceWhitelist`, so
the bounded kubernetes gateway can create it through the generic
`CreateResource` path. No Helm SDK, no Tiller, no direct cluster-admin
Helm access — just a CR that the Flux helm-controller reconciles.

### 3. M19 controlled-operation contract (mirrors promotion)

The deploy flow reuses the exact M19 state machine from the promotion
package:
- **Preview** (`POST /app-catalog/plans/preview`): validates the request,
  checks target namespace exists, fetches chart metadata from the repo
  index, checks no existing HelmRelease with the same name, builds the CR
  manifest, runs a server-side dry-run, and persists the plan with a
  one-time confirmation token (SHA-256 hashed, never stored in plaintext).
- **Execute** (`POST /app-catalog/plans/:plan_id/execute`): claims the
  plan via `ClaimPlan` (row-level lock + constant-time token compare +
  idempotency key check), applies the HelmRelease CR via
  `CreateResource`, and marks the plan succeeded/failed. Idempotent
  replay with the same idempotency key returns the persisted plan without
  re-applying. A 409 conflict during execute is treated as success (the
  HelmRelease already exists from a previous timed-out attempt).

### 4. Credentials never in API responses

The `Repository` model has a `CredentialsJSON` field tagged `json:"-"`
so it is never serialised in API responses. The handler converts each
`Repository` to a `RepositoryView` (which has no credentials field at
all) via `RepositoryViewFrom`. The `has_auth` boolean is the only
credential-related information exposed.

### 5. Authorization: SystemOpsAdmin for writes, any-auth for reads

Route registration uses the existing `RouteDescriptor` pattern:
- Repository CRUD: GET/list (any-auth), POST/DELETE (SystemOpsAdmin).
- Chart listing/detail: GET (any-auth, read-only index.yaml fetch).
- Deploy preview/execute: POST (SystemOpsAdmin).
- Plan list/get: GET (any-auth).

### 6. 10 routes registered on v1 (not resourceRoutes)

Unlike M49 custom resource routes (which are on `resourceRoutes` with
cluster-context + namespace middleware), M57 app-catalog routes are on
`v1` directly — the same pattern as promotion routes. This is because
app-catalog endpoints don't take a `:cluster_id` path parameter (the
target cluster is in the request body for preview, or in the plan record
for execute). The `cluster_id` for plan listing is a query parameter.

## Consequences

### Positive

- **No Helm SDK dependency**: the binary stays small and the attack
  surface is minimal. Chart metadata is fetched via plain HTTP.
- **Deterministic deploy**: the HelmRelease manifest is built once at
  preview and applied verbatim at execute — no re-rendering means the
  dry-run validated manifest is exactly what gets created.
- **M19 contract reuse**: the Claim state machine, confirmation token,
  idempotency key, and TTL are identical to promotion/backup/restore —
  operators already know the flow.
- **Credentials never leaked**: structurally impossible to return
  credentials in API responses (the `RepositoryView` has no such field).
- **Flux reconciliation**: the HelmRelease CR is reconciled by the Flux
  helm-controller, which handles retries, rollbacks, and drift detection
  — the platform just creates the CR.

### Negative

- **No values schema validation**: the platform validates that
  `values_yaml` is valid YAML but does not validate it against the
  chart's `values.schema.json`. The server-side dry-run catches
  admission-time errors, but semantic validation errors surface only at
  reconciliation time.
- **Repo index freshness**: chart metadata is fetched live on each
  request — there is no caching. For repos with large indexes this may
  add latency. The 15-second timeout bounds the worst case.
- **Single HelmRelease per plan**: the plan deploys exactly one chart
  to one namespace. Multi-chart bundles are not supported (by design —
  the roadmap calls for a "simplified" catalog).
