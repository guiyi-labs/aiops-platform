# 2026-07-31 Daily Integration Verification Report

- Date: 2026-07-31
- Scope: M34–M45 same-day development (route descriptor/RBAC inventory,
  access grants, OIDC/MFA, capability adapters, CI/Helm/supply chain,
  signal model, temporal topology, SLO/error budget, multi-signal
  correlation, AI investigator, policy-constrained automation, golden
  dataset/quality report)
- Result: **PASS** — all gates green, 0 failures

## 1. L2 full gate (`scripts/verify.ps1`)

| Step | Check | Result |
|---|---|---|
| 1/8 | `go vet` + `go test ./...` (all backend packages) | PASS |
| 2/8 | `go build` server + 4 offline admin commands | PASS |
| 3/8 | `pnpm typecheck` | PASS |
| 4/8 | `pnpm test -- --run` | PASS |
| 5/8 | `pnpm build` | PASS |
| 6/8 | `docker compose config` + build + healthy containers | PASS |
| 7/8 | `kubectl kustomize` (4 deployment dirs) | PASS |
| 8/8 | `/api/v1/health/ready` + frontend proxy | PASS |

- Duration: **140.05s**
- Backend packages verified: 30+ (including `automation`, `aiinvestigator`,
  `correlation`, `golden`, `slo`, `topology`, `signal`, `capability`,
  `alertroute`, `authz`, `oidc`)
- Frontend tests: 81 tests / 18 files green

## 2. Per-package spot checks (M34–M45 surface area)

Each package was re-run in isolation with `go test -count=1` and `-v` to
count individual test functions including subtests. All passed with exit
code 0.

| Package | Tests (incl. subtests) | Result |
|---|---:|---|
| `internal/automation` (M44) | 128 | PASS |
| `internal/aiinvestigator` (M43) | 78 | PASS |
| `internal/correlation` (M42) | 28 | PASS |
| `internal/golden` (M45) | 9 | PASS |
| `internal/slo` (M41) | 75 | PASS |
| `internal/topology` (M40) | 49 | PASS |
| `internal/signal` (M39) | 25 | PASS |
| `internal/capability` (M37) | 31 | PASS |
| `internal/alertroute` (M37) | 65 | PASS |
| `internal/authz` (M35) | 37 | PASS |
| `internal/oidc` (M36) | 122 | PASS |
| `internal/httpserver` (M34–M45 routes) | 254 | PASS |
| **Total** | **901** | **0 failures** |

## 3. OpenAPI contract consistency

- Test: `TestRegisteredRoutesMatchOpenAPI` in `internal/httpserver`
- Result: **PASS** (0.230s)
- Every Gin-registered route has a matching OpenAPI path; every OpenAPI
  path has a matching registered route. Covers M34 route descriptors,
  M35 access-grants, M36 OIDC (when enabled), M37 alert routes, M40
  topology, M41 SLO, M42 correlation, M43 investigator, M44 automation.

## 4. Migration script consistency

All migration pairs for today's schema changes are present with matching
`up`/`down` files:

| Migration | Schema | up | down |
|---|---|---|---|
| 000027 | alert_routes_and_silences (M37) | yes | yes |
| 000028 | signal_occurrences (M39) | yes | yes |
| 000029 | topology_edges_and_change_events (M40) | yes | yes |
| 000030 | slo_definitions_and_evaluations (M41) | yes | yes |
| 000031 | diagnosis_correlation (M42) | yes | yes |
| 000032 | aiinvestigator (M43) | yes | yes |
| 000033 | policy_constrained_automation (M44) | yes | yes |

7/7 pairs complete. No orphaned up or down files.

## 5. Deployment / supply-chain contracts

- `TestHelmChart*` (10 Helm chart contract tests): **PASS**
- `TestLicenseAllowlist*` (license allowlist contract tests): **PASS**
- `helm lint --strict` integrated in CI (`ci.yml`) and `verify-fast.ps1`

## 6. Invariants preserved (per project memory)

| Invariant | Status |
|---|---|
| client-go v0.34.x (no raw HTTP) | preserved (M33 baseline) |
| Authorization failure → 404 (not 403) | preserved |
| SystemAdmin bypasses access grants | preserved |
| `:namespace` routes use `requireNamespaceAccess` | preserved |
| OIDC disabled by default; no automatic email linking | preserved |
| OIDC session uses refresh rotation + auth_version revocation | preserved |
| OIDC signing key ≥ 32 bytes | preserved |
| No PromQL/LogQL accepted; 3 server-owned SLI templates | preserved |
| Missing data fail-closed (except `workload_readiness` opt-in) | preserved |
| Versioned definitions + append-only evaluations | preserved |
| Burn transitions via `BurnAlertSink` → M27 lifecycle | preserved |
| 404 > 503 precedence in `EvaluateSLO` | preserved |
| AI advisory-only; cannot upgrade candidate→confirmed | preserved |
| Citations mandatory and bounded to authorized evidence | preserved |
| On provider/citation failure, `failed` investigation persisted | preserved |
| L2 human approval default; no L4 autonomous execution | preserved |
| Four-eyes approval for rollback and image_update | preserved |
| Policy gates rechecked at execution | preserved |
| Idempotent claims require confirmation + idempotency key | preserved |
| Server-owned rollback contract (safe→plan, unsafe→human) | preserved |
| Audit logs append-only with redacted execution errors | preserved |

## 7. Known deferrals (external gates, not regressions)

These items are explicitly out of scope for same-day local development
and do not affect the PASS result:

- Hosted CI Linux race detector + full real-kind E2E matrix
- Real organization IdP (OIDC/MFA) operation
- Real Prometheus/Loki/AI-provider integration tests
- PostgreSQL integration tests (full Compose stack)
- Real-kind E2E for M35 access grants, M40 topology, M41 SLO burn→M27,
  M42 correlation, M43 investigator, M44 automation, M45 golden scenario
- Frontend UI for access grants, topology, SLO, correlation,
  investigator, automation, quality dashboard
- Cosign image signing
- Background workers (collection, correlation, verification)

## 8. Conclusion

All M34–M45 same-day development is verified at the local development
gate. The L2 full gate is green, 901 per-package tests pass with 0
failures, OpenAPI/route contracts are bidirectionally consistent, all 7
migration pairs are complete, and Helm/license supply-chain contracts
hold. All hard invariants from project memory are preserved. The
remaining deferrals are external production gates that require hosted CI,
real IdP, real Prometheus/Loki, or real-kind clusters.
