# Change Record: Kubernetes deployment baseline

- Date: 2026-07-17
- Scope: Platform deployment assets and runtime hardening

## Delivered

- Added `deploy/kubernetes/kustomization.yaml` and a single-platform baseline
  for namespace, service accounts, configuration, PostgreSQL, backend,
  frontend, TLS Ingress and NetworkPolicies.
- Kept credentials in an explicit `secret.example.yaml` template that is not
  included by the default Kustomization and contains only `CHANGE_ME` markers.
- Added live/readiness/startup probes, resource requests/limits, non-root
  security contexts, dropped capabilities, read-only root filesystems for the
  application containers and disabled service-account token automounting.
- Kept backend and PostgreSQL Services internal. The frontend is the only
  Ingress target; monitoring access to `/metrics` is cluster-internal.
- Changed the frontend Nginx listener to 8080 so the Kubernetes frontend pod
  can run as UID/GID 101 without privileged ports. Compose health checks and
  port mappings were updated accordingly.

## Verification

- `kubectl kustomize deploy/kubernetes` rendered 16 resources successfully.
- Backend deployment-manifest tests passed as part of `go test ./...`, checking
  required resources, secret separation, probes, limits, security contexts,
  internal metrics annotations and frontend-only Ingress.
- `docker compose config --quiet` passed.
- The frontend image rebuilt successfully and served the SPA with HTTP 200
  under a read-only filesystem and UID/GID 101 using writable tmpfs mounts.
- Backend and frontend images both built successfully. A full Compose smoke
  test reached healthy status for all three services and verified the frontend
  API proxy. `/metrics` was present on the backend port and was not proxied by
  the frontend.
- A live local API process passed PostgreSQL-backed liveness/readiness and
  metrics privacy checks.
- Real cluster apply/kind validation remains unclaimed because this machine
  has no kind installation and no current kubectl context.
