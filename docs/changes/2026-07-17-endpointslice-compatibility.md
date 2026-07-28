# 2026-07-17 EndpointSlice Compatibility

## Scope

- Add discovery.k8s.io/v1 EndpointSlice models and bounded Service-scoped reads.
- Select slices with `kubernetes.io/service-name=<service>` rather than exposing arbitrary discovery queries.
- Aggregate ready and not-ready addresses across slices; treat an absent ready condition as ready.
- Fall back to core/v1 Endpoints only when the discovery API returns 404.
- Preserve the Service diagnosis rule ID and evidence counts, adding only a `source_api` marker.
- Keep all paths read-only; no remediation or Kubernetes mutation was introduced.

## Verification

- Focused Kubernetes and diagnosis tests cover EndpointSlice paths, label selectors, ready true/false/nil, multiple addresses, evidence source and group count.
- A discovery 404 performs exactly one legacy fallback; a discovery 403 returns the original error and does not call Endpoints.
- Full `go test ./...` and server build passed. Frontend typecheck, 20 Vitest tests and production build passed unchanged.
- Real kind validation was not executed: kind is not installed and kubectl has no configured context on this workstation.

## Boundaries

- The gateway still uses bounded raw HTTPS rather than client-go informers or watches.
- EndpointSlice ports, topology hints and zone metadata are not persisted because the current rule only needs backend readiness counts.
- A 404 caused by a missing Namespace also attempts the legacy path, whose result remains authoritative for the final error.
