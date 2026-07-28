# Change Record: Contract and container build gates

- Date: 2026-07-17
- Scope: OpenAPI drift prevention and reproducible container builds

## Delivered

- Added `TestRegisteredRoutesMatchOpenAPI`, which registers every conditional
  Gin route and compares method/path pairs bidirectionally with the OpenAPI
  document. Gin `:parameter` paths are normalized to OpenAPI `{parameter}`
  form before comparison.
- Promoted `gopkg.in/yaml.v3` to a direct backend test dependency.
- Updated the backend builder image from Go 1.24 to Go 1.25 to match `go.mod`.
- Replaced the frontend image's outdated Corepack bootstrap with an explicitly
  pinned `pnpm@11.7.0` installation.
- Copied `pnpm-workspace.yaml` before dependency installation so pnpm applies
  the repository's explicit `esbuild` lifecycle-script allowlist.

## Verification

- `go test ./internal/httpserver` passed, including the bidirectional route
  contract test.
- `docker compose config --quiet` passed.
- The frontend image built successfully and served the generated SPA with HTTP
  200 from an isolated verification container.
- A real local API process connected to PostgreSQL and returned healthy live/
  ready responses. An unauthenticated PATCH containing a concrete user ID
  returned 401; the subsequent metrics scrape contained the registered route
  template and did not contain the concrete identifier.
- Docker Hub initially reset/refused OAuth connections, but a later retry
  successfully pulled Go 1.25/Alpine 3.22 and built the non-root backend image.
- A full Compose smoke run reached healthy PostgreSQL, backend and frontend
  containers. Backend live/ready, the SPA, the frontend `/api/` proxy and the
  Prometheus content type all passed. The frontend health check was corrected
  from `localhost` to `127.0.0.1` because Alpine resolved localhost to IPv6
  while Nginx listened on IPv4.
