# M63: Deprecated / Removed API Check

- Date: 2026-08-01
- Status: Development Complete
- Commit: `9c81927`（与 M61/M62 合并提交）

## Summary

Adds a read-only analyzer flagging objects that use deprecated or removed
`apiVersion`s relative to a target Kubernetes minor version
(`backend/internal/deprecatedapi`). Compiled-in catalog (pluto/kubent style),
severity `removed` (critical) / `deprecated` (warning), emits
`internal/finding`-shaped findings.

## Files

- `backend/internal/deprecatedapi/` — new package: catalog + evaluation + tests.

## Notes

Target version is supplied in the observation bundle; no cluster writes.