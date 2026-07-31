# ADR 0048: Restricted client-go Migration

- Date: 2026-07-31
- Status: Accepted
- Related milestones: M33, ADR 0004 (bounded read-only Kubernetes gateway)
- Supersedes: the raw HTTP gateway portion of ADR 0004 (read/write transport layer only; the bounded-access, redaction and least-privilege invariants of ADR 0004 remain in force)

## Context

ADR 0004 required the platform to use `client-go` and fake-client testing before
adding any write path. The repository deferred this obligation and built
`backend/internal/cluster/registry.go` as a raw `net/http` gateway that manually
constructs URLs, sets headers and reads response bodies. M23-M31 added multiple
controlled write paths (Deployment/CronJob patch, Node patch, eviction, Velero
Backup/Restore CRD create, cross-cluster promotion create/update) on top of this
raw gateway.

The raw gateway has four problems:

1. It bypasses official Kubernetes client retry, discovery, type validation and
   error classification. Every 404/409/5xx is a bare `APIStatusError{StatusCode}`
   that callers must re-derive.
2. It cannot use `client-go` fake clients for unit testing, forcing every write
   test to mock at the `Gateway` interface level rather than at the Kubernetes
   API surface. This hides real serialization/patch-format bugs.
3. Credential rotation closes idle connections but cannot invalidate an in-flight
   `*http.Client`. `client-go` `rest.Config` has a well-defined transport lifecycle.
4. The raw gateway accepts any path string. A future caller could accidentally
   construct an unreviewed write URL. Typed clients and code-owned GVR lists make
   the writable surface explicit.

M33 is the P0 technical debt milestone in `docs/kubesphere-optimization-plan.md`
§4.1. No new write path may be added to the raw gateway after `baseline-m32-20260731`.

## Decision

### 1. Replace the transport, keep the interfaces

The four existing interfaces (`Prober`, `Gateway`, `PatchGateway`, `CreateGateway`)
in `backend/internal/kubernetes/service.go` and `backend/internal/cluster/service.go`
remain unchanged. A new `ClusterClientProvider` in `backend/internal/cluster/`
implements all four using `k8s.io/client-go`. The existing `*Registry` is removed
once all call sites are migrated.

`main.go` is the only assembly point: `cluster.NewRegistry(timeout)` is replaced
by `cluster.NewClientProvider(config)`.

### 2. Dependency alignment

Add `k8s.io/api` and `k8s.io/client-go` at exactly `v0.34.0`, matching the existing
`k8s.io/apimachinery v0.34.0`. The three modules must never diverge in version.

### 3. ClusterClientProvider cache

The provider caches per `cluster_id` + credential generation digest:

- sanitized `*rest.Config` (no kubeconfig bytes, no token string in logs);
- `kubernetes.Interface` typed clientset;
- `dynamic.Interface` for code-owned CRD GVRs only (Velero, optional Gateway API);
- `discovery.DiscoveryInterface` for server version and resource discovery.

Concurrent first use for one cluster/generation builds one clientset only
(`sync.Once` per cache key). Credential rotation, cluster disable and cluster
delete invalidate the cache entry after the database commit succeeds. Failed
rotation leaves the old client usable until the commit succeeds.

### 4. Kubeconfig parsing unchanged

`ParseKubeconfig` in `kubeconfig.go` continues to enforce: no local file references,
no `exec` auth provider, no non-HTTPS API endpoint. It returns a `ClientConfig`
that the provider converts to a `*rest.Config` with the same TLS transport, bearer
token, timeout, QPS, Burst and User-Agent.

### 5. Fixed client configuration

- QPS: 20; Burst: 40 (consistent with fleet fan-out bounds);
- per-request timeout: the existing `ClusterProbeTimeout` config value;
- User-Agent: `k8s-aiops-platform/0.1` (unchanged);
- response byte cap: preserved per call site (1<<20 to 10<<20);
- `CheckRedirect`: never follow (preserved);
- no informer/watch cache for any resource in M33.

### 6. Migration order (write paths first)

1. `Patch`: Deployment, CronJob, Node — typed `AppsV1().Deployments().Patch`,
   `BatchV1().CronJobs().Patch`, `CoreV1().Nodes().Patch` with
   `types.StrategicMergePatchType`. Body size 1-4096 bytes preserved.
2. `Create`: Velero Backup/Restore CRD and promotion resources — `dynamic.Interface`
   for CRDs, typed clientset for Deployment/Service/Ingress. Body size 1-262144
   bytes preserved. `application/json` content type preserved.
3. `Get` (read path): `rest.Interface.Get().AbsPath(path).DoRaw()` for the
   generic `getJSON`/`getRaw`/`RawManifest`/`ResourceExists` calls. Pod logs use
   `CoreV1().Pods(ns).GetLogs(name, opts).DoRaw(ctx)`.
4. `Probe`: `Discovery().ServerVersion()` replaces raw `/version` GET.

### 7. Error mapping

`APIStatusError{StatusCode}` is replaced by `k8s.io/apimachinery/pkg/api/errors`
status errors. The three existing `errors.As` call sites
(`mapGatewayError`, `mapCreateGatewayError`, `ResourceExists`) are updated to use
`apierrors.IsNotFound`, `apierrors.IsConflict` etc. No new error types are introduced.

### 8. Test strategy

- `k8s.io/client-go/kubernetes/fake` provides typed fake clientsets for unit tests.
- `k8s.io/client-go/dynamic/fake` provides fake dynamic clients for CRD tests.
- The existing `Gateway` interface mocks in `kubernetes/service_test.go` remain
  unchanged — they validate interface contract stability.
- New unit tests at the `ClusterClientProvider` level cover: 200, 403, 404, 409,
  timeout, stale UID/resourceVersion, dry-run, concurrent first-use, credential
  rotation invalidation.
- Linux `go test -race -count=1 ./...` must pass (externally blocked if no gcc).

## Non-goals

- No informer/watch cache for Kubernetes objects.
- No RESTMapper-driven arbitrary resource access.
- No generic dynamic client API exposed above the infrastructure package.
- No change to any public API contract, OpenAPI, frontend type, RBAC, audit action
  or migration. M33 is a transport-layer refactor.
- No new writable resource kinds beyond those already accepted in M23-M31.
