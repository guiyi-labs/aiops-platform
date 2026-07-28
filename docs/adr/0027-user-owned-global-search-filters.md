# ADR 0027: User-Owned Global Search Filters

- Status: Accepted
- Date: 2026-07-27

## Context

ADR 0026 introduced a fixed, bounded global resource search. Repeated operator
workflows benefit from saved filters, but persisting an unchecked URL or generic
Kubernetes query would turn a navigation convenience into a new query language.
Saved state also needs explicit ownership, admission limits, lifecycle and
forward-compatibility behavior.

## Decision

Add authenticated CRUD routes below
`/api/v1/fleet/resources/search/filters`. Every record belongs to the current
authenticated user and stores only:

- a trimmed display name of 1 through 40 characters, unique case-insensitively
  for that user;
- the ADR 0026 name substring, optional exact Namespace and canonical non-empty
  subset of Pod, Deployment, Service and Ingress;
- `schema_version=1` plus created and updated timestamps.

Each user may own at most 20 filters. Creation takes a per-user PostgreSQL
advisory transaction lock before counting and inserting, so concurrent requests
cannot bypass the cap. List, update and delete always include `user_id` in the
repository predicate; another user's ID is indistinguishable from a missing
record. Deleting a user cascades their filters.

Create, update and delete use strict JSON decoding and enter the platform audit
trail. Read and mutation are available to all authenticated roles because the
records are private preferences and do not add a target-cluster verb. Responses
never contain a credential, raw Kubernetes object, API Server, selector or
upstream error.

List tolerates a future stale record. A record whose schema version or query
shape is no longer current is returned as incompatible and cannot be applied;
it may still be renamed, replaced with the complete current query shape or
deleted. The stable incompatibility codes are `SCHEMA_VERSION` and
`QUERY_SHAPE`. Any future change to the searchable Kind catalog requires an explicit
database/API migration and schema-version decision before release.

## Consequences

- Saved filters remain a convenience over the reviewed bounded search rather
  than an arbitrary query proxy.
- The first slice does not add sharing, team ownership, pinning, ordering,
  schedules, alerts or cross-user administration.
- Applying a filter still executes a fresh bounded search; results themselves
  are never persisted.
- Filter audit records contain only the preference identity, not the complete
  query body.
