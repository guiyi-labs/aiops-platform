# W10: Error-Code Audit + Normalization (M86 partial)

- Date: 2026-08-09
- Status: First slice complete — audit + normalization landed; OpenAPI
  breaking-change check already gated in CI (oasdiff); frontend type
  generation/sync remains a follow-up item.

## Summary

Adds a static error-code audit test in `internal/httpserver` that scans every
`writeError` call and enforces the mapping contract by construction:

- codes must be `UPPER_SNAKE` (no lowercase, hyphens or spaces);
- no error may use a 2xx status;
- the same code may not map to two different HTTP statuses;
- only the documented status family is allowed
  (400/401/403/404/405/409/410/412/422/424/429/500/502/503/504 + 207 for
  partial-operation surfaces).

The audit immediately caught one real drift: `VELERO_UNAVAILABLE` was emitted
with three different statuses across surfaces (422 UnprocessableEntity in
backup/restore, 424 Failed in kubernetes, 503 elsewhere). It is now
normalized to a single 503 `ServiceUnavailable` (dependency unavailable),
keeping `METRICS_API_UNAVAILABLE` at 424, `*_EXPIRED` at 410 Gone and
`PARTIAL_*` at 207 MultiStatus — all of which are pinned by existing tests.

## Files

- `backend/internal/httpserver/error_code_audit_test.go` — new audit tests.
- `backend/internal/httpserver/backup.go` — VELERO_UNAVAILABLE 422 → 503.
- `backend/internal/httpserver/restore.go` — VELERO_UNAVAILABLE 422 → 503.
- `backend/internal/httpserver/kubernetes.go` — VELERO_UNAVAILABLE 424 → 503.

## Verification

```
go test ./internal/httpserver/ -run 'TestErrorCodeAudit|TestNoErrorWithSuccessStatus'  → PASS
go test ./...  → all green
```

## Follow-up

- 前端类型从 `docs/api/openapi.yaml` 生成/校验同步（openapi-typescript 或等价），
  以及 schema 深度 diff 扩展 —— 归入 M86 后续增量。