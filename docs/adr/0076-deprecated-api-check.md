# ADR 0076: Deprecated / Removed API Check

- Date: 2026-08-01
- Status: Accepted
- Related milestones: M63, ADR 0004 (bounded read-only Kubernetes gateway)
- Supersedes: none

## Context

The in-flight M46–M60 route (KubeSphere-style console, full-stack
observability, AIOps frontend, delivery/ops, production hardening) is broad but
does not cover **upgrade-risk analysis**: which objects in a fleet use an
apiVersion that is deprecated or already removed in the cluster version an
operator is about to upgrade to. Tools such as pluto and kubent fill this gap
in the wider ecosystem, but the platform has no first-class, source-cited signal
for it.

Two constraints shaped the design:

1. A second autonomous agent is actively developing M46–M60 on the main working
   tree (218 uncommitted files as of 2026-08-01). This work MUST NOT touch that
   tree. It was developed in an isolated git worktree
   (`C:/BS/aiops-platform-opt`, branch `opt/m61-m63`) based on the committed HEAD
   (M33), so the two efforts are physically separated.
2. The platform's safety boundary (ADR 0004) is bounded, read-only, least-
   privilege. A deprecated-API check is purely analytical and fits naturally.

A pre-existing baseline compile defect in `internal/cluster` (duplicate
`APIStatusError` declaration in `errors.go` and `registry.go`, slated for removal
per ADR 0048) means any package that transitively imports `cluster`/`kubernetes`
will not build at the M33 baseline. To stay decoupled, this analyzer depends only
on a new dependency-free `internal/finding` package that mirrors
`namespaceposture.Finding`'s JSON contract.

## Decision

1. **New package `internal/deprecatedapi`.** Pure, read-only analyzer.
   - `catalog.go`: a **compiled-in** catalog of known deprecated/removed API
     versions (group/version/kind → deprecation + removal minor + replacement),
     curated from the upstream Kubernetes deprecation guide. Deterministic, zero
     external dependency.
   - `model.go`: `ResourceObject{APIVersion,Kind,Namespace,Name,UID}` input
     contract; `Status` rollup (total/removed/deprecated/clean + findings).
   - `service.go`: `Check(clusterID, targetVersion, []ResourceObject, observedAt)
     → Status`. Classifies each object relative to the target minor:
     `removed` (critical — will fail after upgrade) when targetMinor ≥
     RemovedIn; `deprecated` (warning) within a 3-minor lead-time window.
2. **Reuse the canonical finding contract.** Findings use `internal/finding`, a
   structural mirror of `namespaceposture.Finding`, so the frontend renders them
   uniformly with existing posture findings. `Code` values:
   `DEPRECATED_API_REMOVED`, `DEPRECATED_API_DEPRECATED`.
3. **No mutation, ever.** The analyzer only reports. It never patches, deletes,
   or rewrites objects.
4. **Service-layer ownership deferred.** Building `[]ResourceObject` from the
   Kubernetes API (extracting `apiVersion`+`kind` from raw list JSON, which the
   typed gateway structs omit at the top level) is the responsibility of the
   future API route. The analyzer stays a pure function over already-fetched
   objects, so it is trivially unit-testable today.

## Non-goals

- No mutation of cluster objects (no rewrite/migrate action).
- No live cluster writes, no admission control, no dynamic rule fetching.
- No fix for the baseline `internal/cluster` compile defect (owned by the M33/M34+
  route; out of scope for this optimization branch).
- No GitOps drift detection (covered separately by the M66 proposal).

## Verification

- `go test ./internal/deprecatedapi/` passes (catalog lookup, parse, and the
  removed/deprecated/clean classification across target versions 1.21–1.29).
