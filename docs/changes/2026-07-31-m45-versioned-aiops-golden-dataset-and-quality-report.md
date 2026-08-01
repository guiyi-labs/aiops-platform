# M45: Versioned AIOps Golden Dataset And Quality Report

- Date: 2026-07-31
- Status: Development Complete (local development deliverables)
- ADR: [0060](../adr/0060-versioned-aiops-golden-dataset-and-quality-report.md)
- Fast gate: passed (full L2 verify.ps1, 140.05s; per-package spot checks green)
- Integration verification: 2026-07-31 daily integration report — see `2026-07-31-daily-integration-verification.md`

## Summary

Introduced the versioned AIOps golden dataset and quality report as the
M45 development-side deliverable. The golden dataset is the replayable
contract: identical dataset version + identical engine versions (M39-M44)
produce identical scenario outcomes. Any rule/correlation/prompt/model/
provider/evidence-schema change must produce a machine-readable
before/after quality report rather than silently replacing the baseline.

The dataset contains 3 scenarios:
1. `mandatory_end_to_end` — the 10-step mandatory golden scenario
   (healthy service → bad image → signals → impact graph → cause
   candidate → AI investigation → preview/approve rollback → execute/
   verify → recover alert → cleanup).
2. `negative_misattribution` — an unrelated simultaneous change in
   another Namespace must NOT be attributed to the primary case.
3. `negative_partial_evidence` — when one metrics/log provider is
   stopped, the case must be partial/unknown rather than falsely healthy
   or resolved (M41 fail-closed invariant).

The quality report structure records before/after comparison per
scenario, aggregated summary metrics, changed components, and human
review state. It is JSON-serializable so CI can diff before/after and
block regressions.

M45 production gates (hosted CI, production OIDC/MFA, HA PostgreSQL,
signed releases, real-kind E2E) remain external and are not closed by
this development deliverable.

## Changes

### New files

- `backend/internal/golden/model.go`: defines `DatasetVersion = "1.0"`,
  `ScenarioVersion = "1.0"`, 10 `StepID` constants
  (establish_healthy_service/publish_bad_image/capture_signals/
  build_impact_graph/rank_cause_candidate/generate_investigation/
  preview_approve_rollback/execute_verify/recover_alert/cleanup),
  `AllSteps` ordered list, 3 `ScenarioID` constants
  (mandatory_end_to_end/negative_misattribution/
  negative_partial_evidence), `StepOutcome` (with expected signal/
  topology/SLO/correlation/investigation/action plan/verification/alert
  recovery flags), `Scenario` (ID/Version/Description/Steps/Negative),
  `Dataset` (Version/Scenarios), `DefaultDataset()`, and the 3 scenario
  constructors.
- `backend/internal/golden/quality.go`: defines `QualityReport`
  (ReportVersion/DatasetVersionBefore/DatasetVersionAfter/
  EngineVersionsBefore/EngineVersionsAfter/ScenarioResults/Summary/
  GeneratedAt/ChangedComponents/Reviewer/Approved), `EngineVersions`
  (SignalVersion/TopologyVersion/SLOVersion/CorrelationVersion/
  InvestigatorVersion/AutomationVersion/VerifierVersion tracking M39-M44),
  `ScenarioQuality` (ScenarioID/PassedBefore/PassedAfter/Delta/
  StepsPassedBefore/StepsPassedAfter/StepsTotal/Notes), `QualitySummary`
  (TotalScenarios/PassedBefore/PassedAfter/Improved/Regressed/Preserved/
  Unchanged/TotalStepsBefore/TotalStepsAfter/TotalSteps),
  `ClassifyDelta(before, after)` returning preserved/improved/regressed/
  unchanged, `Summarize(results)` aggregating per-scenario deltas.
- `backend/internal/golden/model_test.go`: 9 tests —
  `TestDatasetVersion` (version non-empty + equals "1.0"),
  `TestDefaultDatasetIntegrity` (3 scenarios, unique IDs, mandatory has
  all 10 steps in order, negative companions marked),
  `TestMandatoryScenarioStepCoverage` (exercises signal/topology/SLO/
  correlation/investigation/action plan/verification/alert recovery),
  `TestNegativeMisattributionScenario` (expects correlation case, does
  NOT expect action plan),
  `TestNegativePartialEvidenceScenario` (expects valid advisory
  investigation, does NOT expect alert recovery),
  `TestDatasetDeterminism` (DefaultDataset returns same scenarios on
  every call),
  `TestClassifyDelta` (4 cases: preserved/improved/regressed/unchanged),
  `TestSummarize` (aggregation math),
  `TestQualityReportEndToEnd` (construct report from 3 scenario results
  with 1 regression).
- `docs/adr/0060-versioned-aiops-golden-dataset-and-quality-report.md`:
  5 decisions covering golden dataset package, mandatory 10-step
  scenario, negative companions, quality report structure, dataset as
  replayable contract.
- `docs/changes/2026-07-31-m45-versioned-aiops-golden-dataset-and-quality-report.md`:
  this change record.

### Modified files

- `docs/roadmap.md`: M45 status section added.
- `docs/thesis/test-matrix.md`: M45 addendum with 9 test counts.
- `docs/development-handoff.md`: updated to M45 baseline.
- `CHANGELOG.md`: M45 entry.

## Test counts

| Package | Tests |
|---|---|
| `internal/golden` | 9 |
| **Total** | **9** |

## Deferred (M45 production gates — external)

- Hosted CI with Linux race detector and full real-kind matrix
- Production OIDC/MFA and break-glass evidence
- Backend/frontend multi-replica deployment with PDB, topology spread,
  rolling-update evidence
- External HA PostgreSQL with WAL/PITR, measured RPO/RTO and
  failover/failback
- Multi-instance collectors, correlators, alert scheduler, outbox and
  operation claims produce no duplicate business effect
- Signed multi-arch release with SBOM, provenance, support matrix and
  upgrade/rollback evidence
- Real-kind E2E for the full 10-step mandatory golden scenario
- Real Prometheus/Loki/AI-provider replay in CI
- Frontend quality dashboard
- CI integration that generates the quality report on every PR
