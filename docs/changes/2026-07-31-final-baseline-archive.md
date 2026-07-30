# Final Baseline Archive: M32

- Date: 2026-07-31
- Baseline tag: `baseline-m32-20260731`
- Branch: `main`
- Publication: local commit and tag only; no push or remote release

## Scope

This archive supersedes the earlier M27-M32 deferred local-acceptance status.
It includes the final defect fixes, ADR/API/RBAC alignment, responsive UI
repair, M27-M31 disposable real-environment suites and fresh repository gates.

## Acceptance Ledger

| Area | Accepted result |
|---|---|
| Backend | Full `go vet ./...` and `go test ./...` passed; server and four offline administration commands built |
| Frontend | Typecheck, 17 Vitest files/73 tests and Vite production build passed |
| Runtime | PostgreSQL/backend/frontend healthy; direct and proxied readiness passed |
| Delivery | Compose config and Kustomize 16/5/22/3 passed |
| Database | 24 up/down pairs; 000024 applied |
| API | OpenAPI/registered-route contract test passed |
| Scripts | 27 PowerShell files passed AST parsing |
| Browser | 390x844 and 1280x720 passed without page overflow or warning/error logs |
| Real environment | M27, M28, M29, M30 and M31 disposable kind suites passed and cleaned |
| Race | Not run: `gcc` unavailable on this Windows host; environment blocker recorded, not passed |

Fresh repository evidence:
`.artifacts/verification/verify-20260731-015255.json`.

Real-environment evidence:

- M27: `.artifacts/m27-alert-lifecycle-kind/m27-alert-lifecycle-kind-20260731013733-e4a6e270.json`
- M28: `.artifacts/m28-backup-creation-kind/summary.json`
- M29: `.artifacts/m29-governance-posture-kind/summary.json`
- M30: `.artifacts/m30-node-maintenance-kind/summary.json`
- M31: `.artifacts/m31-isolated-restore-kind/summary.json`

## Archive Invariants

- The tag must point to the archive commit.
- The worktree must be clean after commit/tag creation.
- No kind cluster or temporary managed-cluster registration may remain.
- No kubeconfig, bearer token, MinIO credential or Velero credential file is
  committed or retained by the acceptance scripts.
- Origin divergence is reported, but this archive does not authorize push.
- Production OIDC/MFA, PITR and HA remain external gates, not local claims.

## Re-entry

Future work starts from `baseline-m32-20260731`. Changes to fixed write
surfaces require a new ADR, route/OpenAPI/RBAC parity, focused regression,
fresh disposable real-environment evidence and a new baseline tag.
