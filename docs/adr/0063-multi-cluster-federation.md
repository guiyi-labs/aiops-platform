# ADR 0063: Multi-Cluster Federation (Host/Member Model) (M48)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M48
- Supersedes: none
- Related: ADR 0004 (bounded read-only Kubernetes gateway), ADR 0008 (audit
  trail), ADR 0050 (lightweight cluster and namespace access grants), ADR 0049
  (route descriptor contract and RBAC inventory)

## Context

Through M47 the platform treated every cluster as an independent standalone
unit. The M48 roadmap entry (`docs/post-m45-development-roadmap.md` §M48)
calls for a KubeSphere-style multi-cluster federation model so that operators
can see a unified topology, track federation-level health, and aggregate
resource counts across clusters.

The design space includes heavyweight options (Karmada, KubeFed v2, a Cluster
Agent / Tower side channel, inter-cluster resource sync controllers, a
transparent K8s API proxy). All of these are rejected by the project's
static-extension hard constraints and ADR 0004's bounded-gateway model. The
federation layer must be a **SQL aggregation view** over the existing
`clusters` table — no new control-plane component, no inter-cluster sync, no
side channel.

M48 delivers the minimum viable federation surface:

1. **Host/Member topology**: clusters can be registered as host or member.
   At most one host is permitted. Standalone clusters (the default) are not
   part of the federation but still appear in the overview.
2. **Federation health dimension**: a `federation_status` field
   (registered/healthy/degraded/disconnected) orthogonal to the existing
   `clusters.status` (enabled/disabled/unreachable). The existing cluster
   probe remains the source of truth for `clusters.status`; federation status
   is operator- or heartbeat-driven.
3. **Append-only audit trail**: every federation state transition
   (registration, deregistration, role change, heartbeat, status change) is
   recorded in `cluster_federation_events`.
4. **Cross-cluster resource summary**: a bounded fan-out that aggregates
   resource counts across visible clusters for a fixed GVR whitelist.

## Decision

### 1. Extend `clusters` table; no new CRD or control plane

Add two columns to `clusters`:

- `cluster_role` VARCHAR(16) NOT NULL DEFAULT 'standalone' — CHECK constraint
  bounds to `host`, `member`, `standalone`.
- `federation_status` VARCHAR(16) NOT NULL DEFAULT 'registered' — CHECK
  constraint bounds to `registered`, `healthy`, `degraded`, `disconnected`.
- `registered_at` TIMESTAMPTZ NULL — set when the cluster is first registered
  with the federation.
- `last_heartbeat_at` TIMESTAMPTZ NULL — updated by the heartbeat endpoint.

A partial unique index `clusters_single_host_uq` enforces the single-host
invariant at the database level: `WHERE cluster_role = 'host'`. This is a
defence-in-depth layer on top of the service-level `CountHost` check.

No new CRD is introduced. No Cluster Agent, Tower, or inter-cluster sync
controller is added. Cross-cluster operations still go through the existing
explicit `cluster_id` + fixed GVR whitelist pattern (ADR 0004).

### 2. Append-only `cluster_federation_events` table

Create `cluster_federation_events` with:

- `id` BIGSERIAL PRIMARY KEY
- `cluster_id` BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE
- `event_type` VARCHAR(32) NOT NULL — CHECK constraint bounds to `registered`,
  `deregistered`, `heartbeat`, `status_change`, `role_change`.
- `status` VARCHAR(16) NOT NULL — the federation_status at the time of the
  event.
- `message` VARCHAR(1024) NOT NULL DEFAULT ''
- `occurred_at` TIMESTAMPTZ NOT NULL DEFAULT NOW()

The repository interface exposes `AppendEvent` and `ListEvents` but **no
UPDATE or DELETE method**. This mirrors the platform audit pattern (ADR 0008)
and makes the federation event trail tamper-evident.

### 3. Federation service: register / deregister / promote / demote / heartbeat / status

The `federation.Service` is the single entry point for federation-scoped
operations. Key invariants:

- **RegisterCluster**: promotes an existing standalone cluster to host or
  member. The cluster must already exist (registration does NOT provision a
  new cluster — the kubeconfig-direct-connection model is preserved). If the
  cluster is already a federation member or host, `ErrClusterAlreadyRegistered`
  is returned. If the role is host and another host exists,
  `ErrHostAlreadyExists` is returned. The single-host invariant is checked
  via `repo.CountHost` before the role update.
- **DeregisterCluster**: soft-delete — sets `cluster_role` back to standalone
  and `federation_status` to registered. The cluster row and its kubeconfig
  are preserved. The host cluster **cannot** be deregistered directly; it
  must first be demoted (`ErrCannotDeregisterHost`). Deregistering a
  standalone cluster is idempotent (no-op, no event appended).
- **PromoteToHost**: promotes a member/standalone to host. Enforces the
  single-host invariant. Idempotent when already host (no-op, no event).
- **DemoteHost**: demotes the host to member or standalone. The cluster must
  currently be the host. `targetRole` must be `member` or `standalone`
  (`ErrInvalidClusterRole` otherwise).
- **RecordHeartbeat**: updates `last_heartbeat_at` and optionally transitions
  `federation_status` (when status is non-empty). The heartbeat event is
  appended to the audit trail. When status is empty, the event status
  defaults to `healthy` but `federation_status` is **not** changed.
- **UpdateFederationStatus**: operator-facing path for marking a cluster
  degraded/disconnected without re-probing. A `status_change` event is
  appended. Idempotent when the status is unchanged (no-op, no event).

All operations append a federation event with the event type, status, and a
human-readable message.

### 4. Overview: pure SQL aggregation, no live probing

`GET /api/v1/federation/overview` returns the host cluster, member clusters,
standalone clusters, and aggregated health counts. It is a pure SQL
aggregation — no live probing is performed by the federation service. The
existing cluster probe path remains the source of truth for `clusters.status`.

The caller may pass `visibleClusterIDs` (resolved by `authorizedClusterFilter`
from the authz service) to restrict the result to the caller's authorized
clusters. `nil` means all clusters (SystemAdmin).

Members and standalone slices are sorted by `cluster_id` ASC for stable
rendering. Empty slices (not nil) are always returned so the frontend can
safely iterate.

### 5. Resource summary: bounded fan-out with fixed GVR whitelist

`GET /api/v1/federation/resources/summary` aggregates resource counts across
visible clusters for each GVR in `FixedGVRWhitelist` (9 entries: pods,
services, nodes, namespaces, deployments, statefulsets, daemonsets, jobs,
cronjobs). The fan-out is bounded by `ResourceSummaryMaxClusters` (20) and
uses a per-cluster timeout of `ResourceSummaryTimeout` (4s).

Missing/unreachable clusters contribute zero counts with an error code
(`TIMEOUT` or `QUERY_FAILED`). The aggregate row is still returned so the
caller can see partial results. The `TotalCount` field sums only successful
cluster counts (failed clusters contribute 0).

The `ClusterLister` interface decouples the federation service from the
kubernetes package. The production implementation
(`kubernetesClusterLister` in `cmd/server/`) adapts the typed list methods
on `kubernetes.Service` to the `CountResult` shape. When the lister is nil
(federation disabled or no kubernetes service), `ResourceSummary` returns an
empty result.

### 6. Authorization: route-layer RBAC, 404 > 403 anti-leakage

Federation read operations (`overview`, `events`, `resources/summary`,
`clusters/:cluster_id/events`) require authentication only — the
`authorizedClusterFilter` narrows the visible clusters based on the caller's
authz scope (SystemAdmin sees all; others see only their ClusterGrant
clusters).

Federation write operations (`register`, `deregister`, `promote`, `demote`,
`heartbeat`, `status`) require `operations_admin` role (enforced at the route
layer via `RequiredRoles: rolesSystemOpsAdmin`).

Anti-leakage (404 > 403) is preserved: `ErrClusterNotFound` surfaces as 404
so a missing cluster is indistinguishable from an unauthorized one. The
handler's `writeFederationError` maps each sentinel error to a stable HTTP
status and error code.

### 7. Route registration and OpenAPI contract

All federation routes are registered under `/api/v1/federation` and
documented in `docs/api/openapi.yaml`:

- `GET /federation/overview` — federation topology + health summary
- `GET /federation/events` — recent federation events (bounded, newest first)
- `GET /federation/resources/summary` — cross-cluster resource count
- `POST /federation/clusters/register` — register (operations_admin+)
- `DELETE /federation/clusters/{cluster_id}` — deregister (operations_admin+)
- `POST /federation/clusters/{cluster_id}/promote` — promote to host
- `POST /federation/clusters/{cluster_id}/demote` — demote host
- `POST /federation/clusters/{cluster_id}/heartbeat` — record heartbeat
- `PATCH /federation/clusters/{cluster_id}/status` — update federation_status
- `GET /federation/clusters/{cluster_id}/events` — per-cluster events

Route-contract consistency (ADR 0049) is maintained: the OpenAPI document and
the route registrar agree on path, method, parameters, and response schema.
The `TestRegisteredRoutesMatchOpenAPI` test covers all federation routes.

Federation routes are registered conditionally — only when
`options.FederationService != nil` and `options.Auth != nil`. This keeps the
federation surface opt-in for deployments that do not need it.

## Consequences

- **Positive**: Operators get a unified federation topology and health view
  without deploying a new control-plane component. The SQL aggregation model
  is simple to operate and audit.
- **Positive**: The single-host invariant is enforced at two layers (service
  `CountHost` + database partial unique index), providing defence-in-depth
  against accidental dual-host states.
- **Positive**: The append-only event trail gives a tamper-evidence audit
  record of all federation state transitions, consistent with the platform
  audit pattern (ADR 0008).
- **Positive**: The resource summary's bounded fan-out (20 clusters, 4s
  per-cluster timeout) prevents a single slow cluster from blocking the
  entire summary. Partial results are always returned.
- **Negative**: The federation model is deliberately minimal — there is no
  inter-cluster resource sync, no transparent API proxy, no workload
  scheduling across clusters. Operators expecting Karmada-style capabilities
  will need to integrate an external federation controller. M48 does not
  preclude this; the `cluster_role` and `federation_status` fields are
  compatible with a future controller that consumes them.
- **Negative**: The resource summary fan-out is O(GVRs × clusters) = 9 × 20
  = 180 concurrent list calls in the worst case. This is bounded but not
  trivial; operators should be aware that the summary endpoint is more
  expensive than a typical read.
- **Neutral**: `federation_status` is orthogonal to `clusters.status`. The
  existing cluster probe updates `clusters.status`; the federation heartbeat
  updates `federation_status`. This separation is intentional — a cluster
  can be `ready` (probe succeeded) but `degraded` (federation heartbeat
  reported issues). The frontend should render both dimensions independently.

## Open Questions

None for M48. Future milestones may address:
- M49+: full CRD browsing across federated clusters.
- A federation controller that reconciles `federation_status` from probe
  results automatically (today this is operator-driven via the status
  endpoint).
- Cross-cluster resource scheduling (explicitly out of scope for M48).
