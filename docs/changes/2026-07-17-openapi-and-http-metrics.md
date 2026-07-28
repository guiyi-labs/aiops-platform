# Change Record: OpenAPI baseline and HTTP metrics

- Date: 2026-07-17
- Scope: API contract documentation and request observability

## Delivered

- Added `docs/api/openapi.yaml` covering health, auth/session, user, cluster,
  Kubernetes resource, diagnosis, AI and audit route families.
- Added `Metrics` middleware and `GET /metrics` Prometheus exposition.
- Bounded labels to method, Gin route template and status class; raw paths,
  IDs, query strings and request bodies are not labels.
- Added focused unit coverage for aggregation, status classes and endpoint
  content type.
- Documented the unauthenticated scrape boundary and required network
  restriction in the API and architecture docs.

## Verification

`go test ./internal/httpserver` passed after the change. The complete Go,
frontend and documentation verification is recorded in the development
handoff after the stage is closed.

## Follow-up

Add an automated OpenAPI route-drift check or generated schema once the public
contract stabilizes. Keep collector authentication/network policy in deployment
configuration rather than adding user-session semantics to `/metrics`.
