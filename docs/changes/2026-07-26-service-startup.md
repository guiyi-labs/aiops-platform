# Change Record: Current source startup verification

- Date: 2026-07-26
- Scope: Rebuild and start the current graduation-project source tree

## Runtime result

- Docker Desktop WSL2 storage on `E:\DockerData` started normally.
- PostgreSQL, backend and frontend images were started from the current Compose project.
- Frontend is available at `http://localhost:18080`.
- Backend is available at `http://localhost:8080`.
- PostgreSQL is exposed at `localhost:15432`.
- Frontend HTML, frontend API proxy, backend readiness and administrator login all passed.
- Database migration `000013_controlled_remediation.up.sql` is the latest applied migration.

## Port correction

Windows currently reserves TCP ports `5139-5238`, which includes the previous
frontend host port `5173`. Compose now uses
`${FRONTEND_PORT:-18080}:8080`, and `.env.example` documents the override.
The container port and frontend-to-backend proxy contract are unchanged.

## Verification

- All backend Go package tests passed with the build cache disabled.
- Frontend typecheck passed.
- Vitest passed 8 files and 26 tests.
- All three Compose services reached healthy state.

## Current data

The development database currently contains one `system_admin`, zero imported
clusters and zero diagnosis records. The feature surface is available, but the
dashboard remains mostly empty until a Kubernetes cluster is imported again.
