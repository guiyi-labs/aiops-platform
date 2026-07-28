# ADR 0026: Bounded Global Resource Search

- Status: Accepted
- Date: 2026-07-27

## Context

Operators can compare fleet health and inspect one selected cluster, but they
cannot locate a named resource without first knowing its cluster. KRM and Ratel
demonstrate the efficiency value of resource-oriented navigation, while their
generic editing and broad proxy surfaces exceed this platform's security
boundary.

A cross-cluster search multiplies Kubernetes API work by both cluster and kind.
It therefore needs reviewed admission limits and local failure semantics before
the frontend exposes it. The API must not accept an arbitrary GVK, Kubernetes
path, label selector, field selector or raw query.

## Decision

Add authenticated `GET /api/v1/fleet/resources/search` with these fixed rules:

- `q` is a required, trimmed, case-insensitive name substring of 2 through 64
  characters;
- `namespace` is optional and must be one valid Kubernetes Namespace name;
- `kinds` is optional and may contain only `pods`, `deployments`, `services`
  and `ingresses`; omission selects all four;
- at most 20 enabled clusters are selected in stable platform-ID order;
- at most four cluster workers run concurrently, with one four-second budget
  per cluster and sequential kind reads inside each worker;
- each kind contributes at most 100 normalized candidates and the response
  returns at most 100 results;
- results are sorted by cluster ID, fixed kind order, Namespace and name;
- per-cluster/per-kind failures return only `TIMEOUT` or `QUERY_FAILED` and do
  not suppress successful peers;
- the response reports known total matches, remaining results, enabled-cluster
  coverage, failures and a `complete` flag. Result truncation, omitted enabled
  clusters or any failure makes the response incomplete.

The response contains only cluster identity, kind, Namespace, name, health tone
and a bounded status summary. It never contains a raw Kubernetes object,
credential, API Server address or upstream error body. All authenticated roles
may search because the endpoint adds no target-cluster verb and reuses existing
observer credentials.

## Consequences

- The first search slice is useful for navigation but is intentionally not a
  generic Kubernetes explorer.
- Name substring filtering is performed over each fixed list response. Existing
  gateway response-size and request-time limits remain the upstream safety net.
- Saved filters are deferred until the search contract and browser workflow are
  accepted. Their persistence model must reference only this fixed query shape.
- Cross-cluster copy, bulk mutation and arbitrary YAML editing are not implied
  by search results and require separate ADRs.
