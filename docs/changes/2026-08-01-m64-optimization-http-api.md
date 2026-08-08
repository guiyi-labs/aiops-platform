# M64: Optimization Analyzers HTTP API

- Date: 2026-08-01
- Status: Development Complete
- Commit: `12af27d`（与 `1e71ccd` 重复提交）

## Summary

Wires M61–M63 analyzers into the server (P1-①):

- `POST /api/v1/optimization/cis/analyze` — M62 CIS posture rollup.
- `POST /api/v1/optimization/finops/analyze` — M61 right-sizing +
  waste summary (optional `rate` override).
- `POST /api/v1/optimization/deprecated-api/analyze` — M63 check against a
  target Kubernetes minor version.

All endpoints are read-only and accept the already-collected observation
bundle in the request body; the server never reaches into the cluster itself.

## Files

- `backend/internal/httpserver/` — optimization route group + handlers.
- `docs/api/openapi.yaml` — new operation schemas.

## Notes

Auto-collection was delivered later in M65; these endpoints now fall back to
the M65 collector when the body carries no bundle.