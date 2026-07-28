# ADR 0025: Bounded multi-cluster health fan-out

- Status: Accepted
- Date: 2026-07-27

## Context

The existing Dashboard reads one selected cluster. Repeating those browser
requests for every registered cluster would make client performance depend on
fleet size, duplicate authorization and timeout behavior, and turn one slow
cluster into an unbounded page load. A global proxy that accepts arbitrary
resource kinds would also bypass the platform's fixed Kubernetes API boundary.

The first M20 slice needs a comparable fleet summary before global search or
saved filters. It must keep failures attributable to one cluster and must not
claim complete health when only part of a resource collection was sampled.

## Decision

Add authenticated `GET /api/v1/fleet/health` with these fixed server limits:

- at most 20 enabled clusters per response;
- at most four clusters executing concurrently;
- one four-second context budget per cluster;
- at most 100 sampled Nodes, Pods, Deployments and Events per cluster.

Only enabled clusters already visible through the authenticated cluster
directory participate. The endpoint adds no write permission and does not
accept cluster credentials, Kubernetes paths, selectors or arbitrary resource
kinds. Cluster ordering is stable by platform cluster ID. The optional `limit`
query is restricted to 1 through 20, and the response reports total and
remaining enabled clusters.

Each cluster returns sampled/total/complete coverage for Node, Pod and
Deployment health, sampled Event coverage and Warning count, a bounded status,
duration and stable failure codes. A per-resource failure, timeout or truncated
sample produces `partial` or `timed_out` data for that cluster while the overall
request remains HTTP 200. Only failure to read the platform cluster directory
fails the complete request. Raw upstream errors, API server addresses,
credentials and resource objects are not returned.

## Consequences

Dashboard can compare cluster health with one bounded request and then switch
into the existing single-cluster workbench. Four concurrent workers bound
cluster-level fan-out; resource reads inside each worker remain sequential so
the maximum active upstream request count is also four. Sample truncation is
visible and can never be labeled healthy.

The phase does not provide global resource search, saved filters, historical
metrics or true multi-cluster environment validation. Those require separate
contracts and tests. Production deployments may later make the fixed defaults
configurable, but no configuration may exceed reviewed server caps without a
new capacity and abuse analysis.
