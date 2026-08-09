# M82: Golden Analyzer-Discovery Contract (W6)

- Date: 2026-08-09
- Status: Development Complete (local)
- Fast gate: PASSED — backend go vet/build/test green (golden/httpserver/
  insight/diagnosis/inspection/posture/kubernetes/topology).

## Summary

Closes polish-plan W6: the M56 golden replay + quality report now cover the
aggregate governance posture and the analyzer catalog surface. A new
`analyzer_discovery` scenario snapshots the deterministic analyzer contracts
(posture domains, insight kinds, diagnosis rule IDs, inspection rule codes,
dry-run operation candidates) into the replay dataset, so a golden run can
prove the analyzers registered on both sides of the contract.

Dataset version is bumped to 1.1, satisfying the M45 versioned-golden
contract (schema/step/outcome changes require a version bump and quality
report).

## What Changed

### Backend (new)

- `backend/internal/golden/model.go` — `DatasetVersion` → 1.1; scenario
  `ScenarioAnalyzerDiscovery`, step `StepAnalyzerDiscovery`, outcome
  `ExpectAnalyzerContracts`, `analyzerDiscoveryScenario()` added to the
  default scenarios (now 4).
- `backend/internal/golden/runner.go` — `EngineContracts.AnalyzerDiscovery`
  (SchemaVersion / PostureDomains / InsightKinds / DiagnosisRules /
  InspectionRules / Operations); `verifyStep` verifies the analyzer
  contracts against the snapshot and fails with notes when the snapshot is
  missing or any domain set is empty.
- `backend/internal/golden/analyzer_contract_test.go` — missing snapshot
  must fail; complete snapshot must pass.

### Backend (snapshot helpers)

- `backend/internal/insight/snapshot.go` — `Kinds()`, `Operations()`.
- `backend/internal/posture/posture.go` — `Domains()`.
- `backend/internal/diagnosis/model.go` — `RuleIDs()`.
- `backend/internal/inspection/catalog.go` — `RuleCodes(catalog)`.
- `backend/cmd/server/golden_contracts.go` — engine contracts wired from
  the live catalogs.

### Tests / Docs

- `backend/internal/golden/service_test.go` — RunReplay assertions updated to
  the 4-scenario set (synthetic summaries keep the 3-step assumption).
- `backend/internal/httpserver/golden_test.go` — `testEngineContracts()`
  includes `AnalyzerDiscovery`.
- `docs/api/openapi.yaml` — unchanged this workstream (no new route).

## Verification (local)

- `go test ./internal/golden/... ./internal/httpserver/...` — green.
- `go vet ./internal/...`, `go build ./...` — green.

## Notes

- Pure read-only snapshot surface; no cluster access and no new write
  capability (ADR 0004 / ADR 0079 unchanged).
- Next workstream: W7 — topology deepening (Gateway API read-only browse +
  collapse/aggregate view), see docs/changes/2026-08-09-m83-topology-deepening.md.