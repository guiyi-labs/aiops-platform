# ADR 0016: EndpointSlice-first Service Evidence

- Status: Accepted
- Date: 2026-07-17

## Context

core/v1 Endpoints is deprecated for scalable Service backend discovery, while modern Kubernetes controllers publish discovery.k8s.io/v1 EndpointSlice objects. Continuing to depend only on Endpoints risks missing current backend state; blindly falling back on every EndpointSlice error would hide RBAC, timeout and API compatibility failures.

## Decision

Service diagnosis first lists EndpointSlice objects in the Service Namespace with the exact `kubernetes.io/service-name=<name>` label selector. The gateway path and method remain server-constructed, GET-only and response-bounded. Each endpoint address is counted Ready when `conditions.ready` is true or absent, and NotReady only when explicitly false. Slice groups are converted into the existing internal Endpoints-shaped summary so rule matching and persisted evidence counts remain compatible.

Fallback to the same-name core/v1 Endpoints object occurs only when the discovery request maps to HTTP 404. Authorization failures, timeouts, malformed responses and other errors are returned to the diagnosis caller. Evidence adds a non-sensitive `source_api` field identifying `discovery.k8s.io/v1` or `core/v1`.

## Consequences

- Modern clusters use the supported discovery API while older clusters retain a narrow compatibility path.
- RBAC mistakes are observable instead of silently masked by a legacy read.
- Multiple slices and multiple addresses per endpoint are aggregated without persisting full Kubernetes objects.
- Existing rule ID, ready/not-ready counts and deterministic conclusion remain stable; consumers may optionally use the added source field.
- Real kind validation is still required when the local environment provides kind or a safe Kubernetes context.
