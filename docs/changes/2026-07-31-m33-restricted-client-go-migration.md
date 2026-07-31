# M33: Restricted client-go Migration

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0048](../adr/0048-restricted-client-go-migration.md)
- Baseline: `baseline-m32-20260731`
- Fast gate: 85.91s (26 backend packages, 73 frontend tests, Compose/Kustomize contracts)

## Summary

Replaced the raw `net/http` Kubernetes gateway (`cluster.Registry`) with a
`k8s.io/client-go`-backed `ClusterClientProvider`. The transport layer is now
the official Kubernetes client, satisfying ADR 0004's requirement that the
platform use `client-go` before adding any write path. The four gateway
interfaces (`Prober`, `Gateway`, `PatchGateway`, `CreateGateway`) are unchanged;
`service.go` and all call sites are untouched.

## Changes

### Dependencies

- `backend/go.mod`: added `k8s.io/api v0.34.0` and `k8s.io/client-go v0.34.0`,
  aligned with the existing `k8s.io/apimachinery v0.34.0`.

### New files

- `backend/internal/cluster/clientprovider.go`: `ClientProvider` implements
  `Prober`, `Gateway`, `PatchGateway`, `CreateGateway` using
  `kubernetes.Interface` (typed clientset), `rest.Interface` (generic
  Get/Patch/Post), and `discovery.DiscoveryInterface` (Probe). Per-cluster
  cache with `sync.RWMutex`. Error mapping converts `*apierrors.StatusError`
  to the legacy `APIStatusError` so existing callers are unchanged.
- `backend/internal/cluster/errors.go`: `APIStatusError` type definition moved
  out of the deleted `registry.go`.
- `docs/adr/0048-restricted-client-go-migration.md`: architecture decision
  record.

### Deleted files

- `backend/internal/cluster/registry.go`: raw `net/http` gateway, superseded
  by `clientprovider.go`.

### Modified files

- `backend/cmd/server/main.go`: `cluster.NewRegistry` → `cluster.NewClientProvider`
  (line 83, the only assembly point).
- `backend/internal/cluster/cluster_test.go`: 3 tests switched from `NewRegistry`
  to `NewClientProvider`. All 3 pass: Probe version, Patch method/body/dryRun,
  no-redirect invariant.

## Preserved invariants

- `Patch`: only `application/strategic-merge-patch+json`, body 1–4096 bytes.
- `Create`: only `application/json`, body 1–262144 bytes.
- `Get`/`Patch`/`Create`: path must start with `/` and not `//`.
- Response byte cap (`maxBytes`) checked after `DoRaw`.
- `CheckRedirect`: `http.ErrUseLastResponse` (never follow redirects).
- `User-Agent`: `k8s-aiops-platform/0.1`.
- Bearer token from `ParseKubeconfig`; no kubeconfig bytes cached.
- `Invalidate` closes idle connections and deletes the cache entry.
- `APIStatusError{StatusCode}` returned to callers for `errors.As` checks.

## New capabilities (not in legacy Registry)

- `ClientProvider.Clientset(clusterID, kubeconfig)`: typed clientset for
  future typed-client migration (M33.2 write paths in `service.go`).
- `ClientProvider.DynamicClient(clusterID, kubeconfig)`: dynamic client for
  CRD operations (Velero Backup/Restore).
- `ClientProvider.Discovery(clusterID, kubeconfig)`: discovery client for
  capability checks.
- `rest.Config` with QPS=20, Burst=40 (consistent with fleet fan-out bounds).

## Verification

- `go build ./...`: pass
- `go vet ./...`: pass
- `go test -count=1 ./...`: 26 packages pass (including `cluster` and
  `kubernetes`)
- `scripts/verify-fast.ps1 -Scope All`: 85.91s, all green
- `TestRegistryProbe`, `TestRegistryPatchUsesExactMethodBodyAndDryRun`,
  `TestRegistryDoesNotFollowKubernetesRedirects`: all pass with
  `NewClientProvider`

## Non-goals

- No typed-client migration of `service.go` call sites (PatchDeployment,
  PatchCronJob, PatchNode, CreateResource). The generic `rest.Interface`
  path-based API is used, preserving the exact interface contract. Typed
  clients are exposed via `Clientset()` for future milestone use.
- No informer/watch cache.
- No change to any public API, OpenAPI, frontend type, RBAC, audit action,
  or migration.
- No `apierrors.IsNotFound`/`IsConflict` replacement of `APIStatusError`;
  the legacy error type is preserved for interface stability.

## Real-kind E2E

Deferred. The existing M23-M31 E2E scripts exercise the same Gateway interface
through the platform's HTTP API; a real-kind regression run will validate the
transport swap end-to-end. The local fast gate and unit tests verify the
transport-level contract.
