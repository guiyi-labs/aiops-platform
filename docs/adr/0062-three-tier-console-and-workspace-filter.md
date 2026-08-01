# ADR 0062: Three-Tier Console Navigation + Workspace Resource Filter (M47)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M47
- Supersedes: none
- Related: ADR 0061 (workspace multi-tenancy), ADR 0050 (lightweight cluster
  and namespace access grants), ADR 0004 (bounded read-only Kubernetes
  gateway), ADR 0049 (route descriptor contract and RBAC inventory)

## Context

Through M46 the platform had a workspace aggregation dimension
(`workspaces`, `workspace_memberships`, `user_workspace_grants`) but no
console-level navigation that exposed it. The M47 roadmap entry
(`docs/post-m45-development-roadmap.md` §M47) calls for a KubeSphere-style
three-tier console (platform / workspace / cluster) and a resource management
view that filters by workspace membership.

The backend increment for M47 is intentionally narrow: it must unblock the
frontend's three-tier navigation and resource filtering without committing
to the full CRD browsing surface (deferred to M49) or a dynamic GVR proxy
(forbidden by ADR 0004 and the project's static-extension hard constraints).
Two backend capabilities are required:

1. A read-only **CRD discovery preview** endpoint that returns the union of a
   fixed operator-curated GVR whitelist and the cluster's dynamically
   discovered API resources. This gives the frontend enough metadata to
   render a resource catalog today and is refined into full CRD browsing in
   M49.
2. An optional **`workspace_id` query parameter** on the existing resource
   list endpoints that narrows the returned namespaces to a workspace's
   member namespaces on the current cluster.

The design tension is between the frontend's desire for a single
"filter-by-workspace" affordance and the platform's 2D authorization matrix
invariant (ADR 0050, ADR 0061): a workspace role must **never** grant
namespace read access. The workspace filter must therefore be a pure
visibility narrowing on top of the existing ClusterGrant/NamespaceGrant
authorization, not an authorization decision of its own.

## Decision

### 1. CRD discovery preview: fixed whitelist + dynamic discovery, graceful degradation

Add `GET /api/v1/clusters/:cluster_id/api-resources` returning
`{ "items": [APIResource] }`. Each `APIResource` carries `group`, `version`,
`resource`, `kind`, `namespaced`, and `source` (`whitelist` or `discovery`).

The endpoint is the M47 preview of M49's full CRD browsing:

- **Fixed whitelist** (`fixedAPIResources` in `internal/kubernetes/service.go`):
  a static slice mirroring the resource families already exposed by the
  typed list methods on `Service` (Pods, Deployments, Services, ...). This is
  always present, even when discovery fails, so the console always renders a
  usable catalog.
- **Dynamic discovery**: when a `DiscoveryProvider` is wired and the cluster
  is reachable, `ServerGroupsAndResources` is called and the discovered GVRs
  are merged on top of the whitelist. Subresources (`pods/log`) and
  non-listable/non-gettable resources are skipped; whitelist entries are
  deduplicated (discovery never replaces a whitelist entry).
- **Graceful degradation**: discovery failures (nil provider, credential
  error, discovery API error, partial `ServerGroupsAndResources` error) are
  non-fatal — the whitelist is returned and the discovery source is simply
  omitted. This keeps the endpoint available on air-gapped clusters, with
  stale kubeconfigs, or under API-server throttling.
- **Authorization**: cluster-scoped only. The existing `requireClusterAccess`
  middleware gates access; there is no namespace dimension. 404 > 403
  anti-leakage is preserved by the middleware (an unauthorized cluster is
  indistinguishable from a missing one).
- **Sorting**: the output is sorted by group, version, then resource for
  stable frontend rendering. The sort is applied to the whitelist copy
  before any early-return fallback path so all return paths produce a sorted
  catalog.

M49 will refine the dynamic subset (CRD-only filtering, instance browsing);
M47 deliberately returns only GVR metadata, never resource instances.

### 2. `workspace_id` query parameter: pure visibility narrowing

Add an optional `workspace_id` query parameter to the existing
namespace-scoped resource list endpoints (Pods, Deployments, Services, ...).
When present and positive, the returned namespaces are narrowed to the
workspace's member namespaces on the current cluster.

The narrowing is implemented by the `withWorkspaceNamespaceFilter`
middleware, which runs **after** `requireClusterAccess` and
`requireNamespaceQueryAccess`. The ordering is load-bearing:

1. `requireClusterAccess` — gates cluster access (404 on denial).
2. `requireNamespaceAccess` — gates the `:namespace` path parameter (404).
3. `requireNamespaceQueryAccess` — resolves the caller's
   `authz.ClusterScope` for the cluster and stores it in the Gin context.
4. `withWorkspaceNamespaceFilter` — narrows the resolved scope by the
   workspace's member namespaces.

The narrowing logic (`narrowScopeByWorkspace`) is a pure function:

- **AllNamespaces scope** (SystemAdmin or cluster-grant): narrowed to the
  workspace's member namespaces on this cluster.
- **Namespace-grant scope**: intersected with the workspace's member
  namespaces. Namespaces the user cannot see are dropped; namespaces outside
  the workspace are dropped.
- **Empty workspace membership** (workspace does not exist, or has no
  memberships on this cluster): the scope becomes empty, so list handlers
  return an empty collection (200 with `items: []`). This is the
  anti-leakage contract — the workspace's existence is not leaked via 404.

### 3. Workspace filter is NOT an authorization decision

The `workspace_id` parameter is a **visibility filter**, not an
authorization decision. This is the key invariant that preserves the 2D
authorization matrix (ADR 0050, ADR 0061 §2):

- The caller has already passed `requireClusterAccess` +
  `requireNamespaceQueryAccess` for the cluster and namespace dimensions
  before the filter runs. The filter only narrows the already-authorized
  scope; it never expands it.
- The filter deliberately does **not** enforce `workspace_viewer`
  authorization. A user with a ClusterGrant on the cluster but no workspace
  role may still apply `workspace_id` — they will simply see the
  intersection of their cluster scope and the workspace's namespaces. This
  is correct: the workspace role governs workspace metadata/membership/quota
  edits (ADR 0061), not namespace reads.
- Conversely, a user with a `workspace_viewer` role but no ClusterGrant or
  NamespaceGrant on the cluster sees nothing — the authorization middleware
  returned 404 before the filter ran.

### 4. Anti-leakage and the empty-list contract

The workspace filter inherits the platform's 404 > 403 anti-leakage
discipline (ADR 0050, ADR 0061):

- An unauthorized cluster is reported as 404 by `requireClusterAccess`
  (before the filter runs).
- An unauthorized namespace (path parameter) is reported as 404 by
  `requireNamespaceAccess` (before the filter runs).
- A non-existent or unauthorized workspace is **not** reported as 404.
  Instead, the filter produces an empty scope and the list handler returns
  200 with `items: []`. This avoids leaking the workspace's existence via
  error differentiation: a caller cannot distinguish "workspace missing" from
  "workspace has no namespaces on this cluster" from "I am not a workspace
  member" — all three yield an empty list. This is consistent with the
  workspace service's own `NamespacesForWorkspaceFilter` contract, which
  returns `(nil, nil)` when the filter is disabled and `([], nil)` when the
  workspace is unknown.

### 5. Route registration and OpenAPI contract

The new endpoint and query parameter are registered in the route table and
documented in `docs/api/openapi.yaml`:

- `GET /clusters/:cluster_id/api-resources` — new endpoint, registered with
  `AuditAction: "kubernetes.api_resources.read"`, `AuditResource:
  "APIResource"`. Cluster-scoped authorization only.
- `WorkspaceIDQuery` parameter — reusable parameter added to the
  namespace-scoped resource list routes. Optional, positive integer.
- `APIResource` and `APIResourceList` schemas — added to the OpenAPI
  components for the discovery response shape.

Route-contract consistency (ADR 0049) is maintained: the OpenAPI document
and the route registrar agree on path, method, parameters, and response
schema.

## Consequences

- **Positive**: The frontend can build the three-tier navigation and
  workspace-filtered resource tree on a stable backend contract. The
  discovery preview unblocks resource catalog rendering without waiting for
  M49. The graceful-degradation contract means the console works on
  air-gapped or partially-connected clusters.
- **Positive**: The pure-visibility-filter model keeps the 2D authorization
  matrix intact. No new authorization path is introduced; the workspace
  layer remains orthogonal to ClusterGrant/NamespaceGrant (ADR 0061 §2).
- **Positive**: Anti-leakage is preserved end-to-end. The workspace filter
  never leaks workspace existence via 404; the discovery endpoint never
  leaks unauthorized cluster existence via 403.
- **Negative**: The `workspace_id` filter performs an in-memory intersection
  on every request (it loads all memberships for the cluster, then filters
  by workspace). This is acceptable for M47's scale; if it becomes a
  bottleneck, a `workspace_id`-indexed query on `workspace_memberships` can
  replace the full-cluster scan without changing the contract.
- **Negative**: The discovery preview calls `ServerGroupsAndResources`,
  which is one request per API group. This is bounded by cluster scope and
  cached client-side by client-go's discovery memcache, but operators should
  be aware that the first call on a cold cache is expensive. M49 may
  introduce server-side caching or a narrower discovery call.
- **Neutral**: The `source` field (`whitelist` vs `discovery`) is part of
  the response contract so the frontend can badge resources by origin. M49
  may extend this (e.g. `crd` vs `builtin`) but the M47 values are fixed.

## Open Questions

None for M47. M49 will address full CRD browsing (instance listing, CRD
metadata detail, schema rendering) and may revisit the discovery caching
strategy.
