# ADR 0021: Frontend-only public ingress

- Status: Accepted
- Date: 2026-07-17

## Context

The backend serves user APIs and an unauthenticated Prometheus scrape endpoint.
Publishing the backend Service directly would make the scrape surface depend on
external firewall configuration and would bypass the same-origin frontend
boundary.

## Decision

The Kubernetes baseline keeps the backend and PostgreSQL Services internal
(`ClusterIP`). The Ingress routes only to the frontend Service. The frontend
Nginx proxy forwards `/api/` to the backend, while `/metrics` has no frontend
proxy route. A NetworkPolicy permits backend HTTP ingress from frontend pods
and a labeled in-cluster monitoring namespace only.

## Consequences

The default deployment does not publicly expose metrics or the backend port,
and the frontend remains the browser same-origin boundary. Monitoring must run
in a namespace labeled `kubernetes.io/metadata.name=monitoring`, or the policy
must be adapted to the chosen monitoring topology. External Kubernetes API and
AI provider egress is intentionally allowed on HTTPS/6443 and should be
narrowed with cluster-specific CIDRs when known.
