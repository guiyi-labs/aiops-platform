# ADR 0080 — M83 Topology Deepening: Gateway API read-only browse + collapse view (W7)

- Date: 2026-08-09
- Status: Accepted
- Related milestones: M83 (polish-plan W7)
- Supersedes / context: extends the existing read-only CRD browse (ADR 0064)
  and M40 topology. Gateway Gateway/HTTPRoute/GatewayClass remain read-only.

## Context

The polish-roadmap asks whether we can yet answer about the topology layer at
scale: Gateway API resources (the modern Ingress successor) are not browsable,
and large graphs (500-node fixtures) are not bounded by a collapse view.
polish-plan W7 explicitly asks for "Gateway API read-only + 分页/折叠".

## Decision

1. **Gateway API read-only browse**: add `gateway.networking.k8s.io/v1`
   `gateways`, `httproutes` and `gatewayclasses` to the operator-curated CRD
   whitelist. Scoping matches the real API: GatewayClass is cluster-scoped,
   Gateway and HTTPRoute are namespaced. No write, no runtime discovery; the
   whitelist remains static (ADR 0064 §2/§4 anti-leakage). Adding entries is a
   contract change that must keep the Golden analyzer-discovery replay green.
2. **Collapse view for large graphs**: `GET /aiops/topology/graph?collapse=1`
   collapses repeated edges between the same source/target pair+kind into one
   representative edge carrying an advisory `aggregate_count`. It is a pure
   read-only view transform (ADR 0004); collection and persistence are
   unchanged. Wired behind an optional query param so default behaviour and
   the existing 200-edge bound are untouched.

## Consequences

- Gateway API records are browsable read-only with correct namespacing; the
  frontend can surface them as topology evidence.
- Large-graph responses can be bounded by the collapse view without a data
  migration or a new schema.
- Both additions are additive and unit-testable without a live cluster.