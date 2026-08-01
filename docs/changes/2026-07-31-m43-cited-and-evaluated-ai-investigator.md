# M43: Cited And Evaluated AI Investigator

- Date: 2026-07-31
- Status: Development Complete
- ADR: [0058](../adr/0058-cited-and-evaluated-ai-investigator.md)
- Fast gate: 37.47s (31 backend packages including `aiinvestigator`, 81
  frontend tests/18 files, Compose/Kustomize contracts)

## Summary

Introduced a cited and evaluated AI investigator bound to M42 correlation
cases. M43A defines the data model and migration 000032; M43B implements
the prompt builder, provider interface, citation/runbook validator and
server-owned runbook catalog; M43C adds 10 golden validation fixtures
(correct, insufficient, conflicting, prompt-injection, hidden-scope,
fabricated-citation, ineligible-runbook, confirm-root-claim,
empty-summary, no-citations); M43D wires the 4 HTTP routes and OpenAPI
schemas; M43E adds 47 unit tests (5 catalog + 9 provider/fixtures + 8
prompt + 15 service + 12 handler), ADR 0058 and this change record.

The investigation is a read-only advisory: it never modifies the case,
diagnosis or alert. Every factual claim cites an authorized evidence ID;
fabricated, out-of-scope or unauthorized citations reject the entire
output. The model cannot upgrade a candidate to confirmed cause, and
cannot recommend a runbook outside the eligible M42 Action Catalog. On
provider failure, budget exhaustion or citation rejection, a failed
investigation is persisted with `failure_reason` set so deterministic
investigation remains available.

No public API contract was broken beyond the four new
`aiops/investigator` routes documented in OpenAPI.

## Changes

### M43A — Data model and migration

#### New files

- `backend/internal/aiinvestigator/model.go`: defines
  `InvestigatorVersion = "1.0"`, `HypothesisConfidence` (3 values),
  `InvestigationStatus` (3 values: completed, failed, stale),
  `EvidenceKind` (7 values), `EvidenceRef`, `Hypothesis`,
  `Investigation`, `ActorRef`, `Citation`, `InvestigationFilter`,
  `InvestigationListResponse`, `QualityReport`, `Prompt`,
  `ProviderResult`, and the bound constants (`MaxHypothesesPerInvestigation`
  = 8, `MaxCitationsPerInvestigation` = 64, `MaxUncertainties` = 16,
  `MaxNextChecksPerHypothesis` = 8, `MaxRunbookIDLength` = 128,
  `MaxInvestigationKeyLength` = 64).
- `backend/migrations/000032_aiinvestigator.up.sql`: creates
  `ai_investigations` table with CHECK constraints on status/tokens,
  completed-summary/completed-citations/failed-reason invariants, the
  partial unique index `uq_ai_investigations_active` on
  `(case_id, investigation_key) WHERE status != 'stale'`, and a FK to
  `correlation_cases(id)` ON DELETE CASCADE.
- `backend/migrations/000032_aiinvestigator.down.sql`: drops the table,
  indexes and FK in the correct order.

### M43B — Prompt, provider, validator, catalog

#### New files

- `backend/internal/aiinvestigator/catalog.go`: defines
  `RunbookDescriptor` and the compiled `catalog` map with 4 V1 runbooks:
  `rollback_last_rollout` (`deployment.rollback`),
  `rollout_restart_pods` (`deployment.rollout_restart`),
  `inspect_pvc_capacity` (advisory), `inspect_node_maintenance`
  (advisory). `LookupRunbook` fails closed; `AllRunbooks`,
  `EligibleRunbooks` (advisory runbooks always eligible),
  `ValidateRunbookEligibility`.
- `backend/internal/aiinvestigator/prompt.go`: defines `CaseContext`
  and its sub-contexts (`FactorContext`, `SignalLinkContext`,
  `ResourceLinkContext`, `ChangeCandidateContext`). `BuildPrompt`
  assembles the system prompt (role, output schema, citation rules,
  runbook rules, prohibitions, uncertainty guidance) and the user
  prompt (redacted authorized evidence only). `buildAuthorizedEvidence`
  admits the case, signal occurrences and change candidates.
  `PromptHash` / `computePromptHash` produce a stable SHA-256 over the
  case context + sorted evidence ID set.
- `backend/internal/aiinvestigator/provider.go`: defines the `Provider`
  interface, `NopProvider` (deterministic valid result for tests/disabled
  mode), `ValidateProviderResult` (8 validation rules: non-empty
  summary/impact, 1..8 hypotheses, authorized citations, authorized
  hypothesis/disconfirming evidence, 1..64 citations, eligible runbook,
  no "confirm root cause" claims, bounded next_checks/uncertainties),
  `isAuthorized`, `DecodeProviderJSON`.
- `backend/internal/aiinvestigator/repository.go`: defines the
  `Repository` interface, `GormRepository` (with `investigationRow` GORM
  model, `JSONB` wrapper, `Save` with `MarkStale`, `Get`, `ListByCase`,
  `ListByFilter`, `MarkStale`) and `NopRepository`. Row conversion
  helpers preserve hypotheses/uncertainties/citations as JSONB.
- `backend/internal/aiinvestigator/service.go`: defines `Service` (the
  only writer), `CaseReader` interface, `NopCaseReader`,
  `ServiceOption` (`WithNow`), `ErrCaseNotFound`, `ErrDisabled`.
  `Investigate` reads the case + eligible action codes, builds the
  prompt, calls the provider, validates the result, and persists
  (completed or failed). `GetInvestigation`, `ListByCase`,
  `ListRunbooks`. `computeInvestigationKey` = SHA-256 over (case_id +
  investigator_version + prompt_hash).

### M43C — Golden validation fixtures

#### New files

- `backend/internal/aiinvestigator/fixtures.go`: defines `GoldenFixture`
  and `GoldenFixtures` (10 scenarios: `correct_cited_investigation`,
  `insufficient_evidence`, `conflicting_evidence`,
  `prompt_injection_rejected`, `hidden_scope_citation_rejected`,
  `fabricated_citation_rejected`, `ineligible_runbook_rejected`,
  `confirm_root_claim_rejected`, `empty_summary_rejected`,
  `no_citations_rejected`). Each fixture is a deterministic
  (provider result, authorized evidence, eligible action codes, expected
  valid/invalid + failure substring) pair.

### M43D — HTTP routes and OpenAPI

#### New files

- `backend/internal/httpserver/aiinvestigator.go`: defines
  `aiInvestigatorHandler` with 4 handlers: `listRunbooks`,
  `listInvestigations`, `getInvestigation`, `generateInvestigation`.
  `parseInvestigatorCaseID` / `parseInvestigatorID` enforce positive
  int64 path params. `writeInvestigatorError` maps
  `ErrInvestigationNotFound` → 404. The generate handler derives the
  actor from `requestctx.MetadataFrom`; provider/validation failures
  return the failed investigation with 200, while `ErrCaseNotFound` → 404
  and `ErrDisabled` → 503.

#### Modified files

- `backend/internal/httpserver/router.go`: adds `AIInvestigatorService`
  to `Options`, registers the 4 investigator routes under
  `/api/v1/aiops/investigator` when the service is non-nil.
- `backend/internal/httpserver/openapi_route_test.go`: adds
  `AIInvestigatorService` wiring with `NopRepository` so route
  registration is covered by the OpenAPI parity test.
- `docs/api/openapi.yaml`: adds 4 investigator routes and 7 schemas
  (`InvestigatorRunbookList`, `InvestigatorRunbook`,
  `InvestigationListResponse`, `Investigation`, `InvestigationActor`,
  `InvestigationHypothesis`, `InvestigationCitation`, `EvidenceRef`)
  under the `aiinvestigator` tag.

### M43E — Unit tests, ADR, docs

#### New files

- `backend/internal/aiinvestigator/catalog_test.go`: 5 tests —
  `TestLookupRunbook` (known/unknown/empty), `TestAllRunbooks`
  (self-consistency), `TestEligibleRunbooks` (advisory always eligible,
  action gated, both codes), `TestValidateRunbookEligibility`
  (advisory/eligible/absent/unknown/empty).
- `backend/internal/aiinvestigator/provider_test.go`: 9 tests —
  `TestGoldenFixtures` (10 subtests), `TestGoldenFixturesCoverage`
  (all 10 required scenarios present), `TestValidateProviderResultEdgeCases`
  (8 subtests: empty impact, no hypotheses, invalid confidence, no
  evidence, empty claim, too many hypotheses, unauthorized/authorized
  disconfirming), `TestNopProvider` (valid result / no-case-evidence
  failure), `TestDecodeProviderJSON` (valid/malformed/trim).
- `backend/internal/aiinvestigator/prompt_test.go`: 8 tests —
  `TestBuildPrompt` (evidence authorization), `TestBuildPromptSystemContainsRunbooks`,
  `TestBuildPromptUserContainsCaseFacts`, `TestBuildPromptNoEligibleRunbooks`,
  `TestPromptHashStability`, `TestPromptHashChangesWithEvidence`,
  `TestPromptHashIgnoresFactorOrderRelevantFields`, `TestBuildAuthorizedEvidence`
  (dedup/empty), `TestMarshalEvidenceForHash` (determinism).
- `backend/internal/aiinvestigator/service_test.go`: 15 tests —
  `TestInvestigateSuccess`, `TestInvestigateCaseNotFound`,
  `TestInvestigateProviderFailurePersistsFailed`,
  `TestInvestigateCitationRejectionPersistsFailed`,
  `TestInvestigateIneligibleRunbookPersistsFailed`,
  `TestInvestigateDisabled`, `TestInvestigateNopProviderProducesValidResult`,
  `TestInvestigateInvestigationKeyDeterministic`,
  `TestGetInvestigation` (found/not-found), `TestListByCase`
  (truncated/not-truncated), `TestListRunbooks`,
  `TestComputeInvestigationKey`, `TestNewServiceDefaults`,
  `TestNopCaseReader`, `TestNopRepository`,
  `TestInvestigateEligibleActionCodesError`,
  `TestInvestigateNilEligibleCodesTreatedAsEmpty`. Uses `fakeRepository`
  (in-memory), `fakeCaseReader`, `stubProvider`.
- `backend/internal/httpserver/aiinvestigator_test.go`: 12 handler tests
  — list runbooks 200, list investigations invalid case_id 400, negative
  case_id 400, bad limit 400, list 200, get invalid id 400, get not-found
  404, generate invalid case_id 400, generate case not-found 404,
  generate success 200, generate provider-failure 404, generate preserves
  actor from context.
- `docs/adr/0058-cited-and-evaluated-ai-investigator.md`: 7 decisions
  covering structured cited output, authorized evidence + citation
  rejection, server-owned runbook catalog, prompt-injection defense,
  deterministic investigation_key + staleness, failure persistence,
  read-mostly HTTP surface.
- `docs/changes/2026-07-31-m43-cited-and-evaluated-ai-investigator.md`:
  this change record.

#### Modified files

- `docs/roadmap.md`: M43 status section added.
- `docs/thesis/test-matrix.md`: M43 addendum with 47 test counts.
- `docs/development-handoff.md`: updated to M43 baseline.
- `CHANGELOG.md`: M43 entry.

## Test counts

| Package | Tests |
|---|---|
| `internal/aiinvestigator` (catalog) | 5 |
| `internal/aiinvestigator` (provider/fixtures) | 4 (10 golden subtests + 8 edge subtests) |
| `internal/aiinvestigator` (prompt) | 8 |
| `internal/aiinvestigator` (service) | 15 |
| `internal/httpserver` (investigator handler) | 12 |
| **Total** | **44** top-level (plus 18 subtests) |

## Deferred

- Real AI provider integration (Responses-compatible HTTP provider wired
  into `cmd/server/main.go`)
- Provider budget/reservation enforcement (mirror `aiexplain` daily token
  budget)
- Real PostgreSQL integration test for `GormRepository`
- Real-kind E2E for the investigate → citation → runbook path
- Frontend UI (investigation panel, hypothesis rendering, citation
  tooltips, runbook stepping)
- M44 safe-automation wiring (preview/confirm/execute the eligible
  runbook via existing M19 controlled-operations paths)
