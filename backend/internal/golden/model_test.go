package golden

import (
	"testing"
)

// TestDatasetVersion is the contract test: the dataset version must be
// non-empty and follow semver-ish format. Bumping the version requires
// a quality report.
func TestDatasetVersion(t *testing.T) {
	if DatasetVersion == "" {
		t.Fatal("DatasetVersion must be non-empty")
	}
	if DatasetVersion != "1.2" {
		t.Errorf("DatasetVersion = %q, want %q", DatasetVersion, "1.2")
	}
}

// TestDefaultDatasetIntegrity verifies that the default dataset is
// well-formed: non-empty, all scenario IDs unique, all steps present in
// the mandatory scenario, negative scenarios marked as negative.
func TestDefaultDatasetIntegrity(t *testing.T) {
	ds := DefaultDataset()
	if ds.Version != DatasetVersion {
		t.Errorf("dataset version = %q, want %q", ds.Version, DatasetVersion)
	}
	if len(ds.Scenarios) != 4 {
		t.Fatalf("expected 4 scenarios, got %d", len(ds.Scenarios))
	}

	seenIDs := make(map[ScenarioID]bool)
	for _, sc := range ds.Scenarios {
		if sc.ID == "" {
			t.Error("scenario has empty ID")
		}
		if seenIDs[sc.ID] {
			t.Errorf("duplicate scenario ID %q", sc.ID)
		}
		seenIDs[sc.ID] = true
		if sc.Version == "" {
			t.Errorf("scenario %q has empty version", sc.ID)
		}
		if sc.Description == "" {
			t.Errorf("scenario %q has empty description", sc.ID)
		}
		if len(sc.Steps) == 0 {
			t.Errorf("scenario %q has no steps", sc.ID)
		}
	}

	// Mandatory scenario must have all 10 steps in order.
	mandatory, ok := findScenario(ds, ScenarioMandatoryEndToEnd)
	if !ok {
		t.Fatal("mandatory scenario not found")
	}
	if len(mandatory.Steps) != len(AllSteps) {
		t.Fatalf("mandatory scenario has %d steps, want %d", len(mandatory.Steps), len(AllSteps))
	}
	for i, want := range AllSteps {
		if mandatory.Steps[i].StepID != want {
			t.Errorf("mandatory step %d: got %q, want %q", i, mandatory.Steps[i].StepID, want)
		}
	}
	if mandatory.Negative {
		t.Error("mandatory scenario must not be negative")
	}

	// Negative companions must be marked.
	negMis, ok := findScenario(ds, ScenarioNegativeMisattribution)
	if !ok {
		t.Fatal("negative misattribution scenario not found")
	}
	if !negMis.Negative {
		t.Error("misattribution scenario must be negative")
	}

	negPartial, ok := findScenario(ds, ScenarioNegativePartialEvidence)
	if !ok {
		t.Fatal("negative partial evidence scenario not found")
	}
	if !negPartial.Negative {
		t.Error("partial evidence scenario must be negative")
	}
}

// TestMandatoryScenarioStepCoverage verifies that the mandatory scenario
// exercises every stage of the AIOps loop: M39 signal, M40 topology,
// M41 SLO, M42 correlation, M43 investigation, M44 automation.
func TestMandatoryScenarioStepCoverage(t *testing.T) {
	ds := DefaultDataset()
	mandatory, _ := findScenario(ds, ScenarioMandatoryEndToEnd)

	checks := map[string]bool{
		"signal_captured":  false,
		"topology_edge":    false,
		"slo_evaluated":    false,
		"correlation_case": false,
		"investigation":    false,
		"action_plan":      false,
		"verification":     false,
		"alert_recovered":  false,
	}
	for _, step := range mandatory.Steps {
		if step.ExpectSignalCaptured {
			checks["signal_captured"] = true
		}
		if step.ExpectTopologyEdge {
			checks["topology_edge"] = true
		}
		if step.ExpectSLOEvaluated {
			checks["slo_evaluated"] = true
		}
		if step.ExpectCorrelationCase {
			checks["correlation_case"] = true
		}
		if step.ExpectInvestigation {
			checks["investigation"] = true
		}
		if step.ExpectActionPlan {
			checks["action_plan"] = true
		}
		if step.ExpectVerificationStatus != "" {
			checks["verification"] = true
		}
		if step.ExpectAlertRecovered {
			checks["alert_recovered"] = true
		}
	}
	for check, found := range checks {
		if !found {
			t.Errorf("mandatory scenario does not exercise %q", check)
		}
	}
}

// TestNegativeMisattributionScenario verifies that the misattribution
// scenario does NOT expect the unrelated change to be a candidate.
// The scenario's rank-cause step expects a correlation case but must
// not attribute the unrelated Namespace B change to the Namespace A case.
func TestNegativeMisattributionScenario(t *testing.T) {
	ds := DefaultDataset()
	sc, _ := findScenario(ds, ScenarioNegativeMisattribution)

	if len(sc.Steps) < 2 {
		t.Fatalf("misattribution scenario has %d steps, want at least 2", len(sc.Steps))
	}

	// Step 2 (rank cause) must expect a correlation case.
	rankStep := sc.Steps[1]
	if !rankStep.ExpectCorrelationCase {
		t.Error("misattribution scenario must expect a correlation case at the rank step")
	}

	// The scenario must NOT expect an action plan — the unrelated change
	// does not trigger automation.
	for _, step := range sc.Steps {
		if step.ExpectActionPlan {
			t.Errorf("misattribution scenario step %q must not expect an action plan", step.StepID)
		}
	}
}

// TestNegativePartialEvidenceScenario verifies that the partial-evidence
// scenario expects the case to be partial/unknown, not falsely healthy.
// The investigation must be valid (advisory with uncertainty), not
// rejected — but it must not claim to confirm root cause.
func TestNegativePartialEvidenceScenario(t *testing.T) {
	ds := DefaultDataset()
	sc, _ := findScenario(ds, ScenarioNegativePartialEvidence)

	if len(sc.Steps) < 3 {
		t.Fatalf("partial evidence scenario has %d steps, want at least 3", len(sc.Steps))
	}

	// The investigation must be valid (advisory with uncertainty).
	invStep := sc.Steps[2]
	if !invStep.ExpectInvestigation {
		t.Error("partial evidence scenario must expect an investigation")
	}
	if !invStep.ExpectInvestigationValid {
		t.Error("partial evidence scenario must expect a valid (advisory) investigation")
	}

	// The scenario must NOT expect alert recovery — partial evidence
	// does not resolve the alert.
	for _, step := range sc.Steps {
		if step.ExpectAlertRecovered {
			t.Errorf("partial evidence scenario step %q must not expect alert recovery", step.StepID)
		}
	}
}

// TestDatasetDeterminism verifies that DefaultDataset returns the same
// scenarios on every call. The dataset is the replayable contract;
// identical calls must produce identical scenarios.
func TestDatasetDeterminism(t *testing.T) {
	ds1 := DefaultDataset()
	ds2 := DefaultDataset()

	if ds1.Version != ds2.Version {
		t.Fatalf("dataset version differs: %q vs %q", ds1.Version, ds2.Version)
	}
	if len(ds1.Scenarios) != len(ds2.Scenarios) {
		t.Fatalf("scenario count differs: %d vs %d", len(ds1.Scenarios), len(ds2.Scenarios))
	}
	for i := range ds1.Scenarios {
		if ds1.Scenarios[i].ID != ds2.Scenarios[i].ID {
			t.Errorf("scenario %d ID differs: %q vs %q", i, ds1.Scenarios[i].ID, ds2.Scenarios[i].ID)
		}
		if ds1.Scenarios[i].Version != ds2.Scenarios[i].Version {
			t.Errorf("scenario %d version differs: %q vs %q", i, ds1.Scenarios[i].Version, ds2.Scenarios[i].Version)
		}
		if len(ds1.Scenarios[i].Steps) != len(ds2.Scenarios[i].Steps) {
			t.Errorf("scenario %d step count differs: %d vs %d", i, len(ds1.Scenarios[i].Steps), len(ds2.Scenarios[i].Steps))
		}
		for j := range ds1.Scenarios[i].Steps {
			if ds1.Scenarios[i].Steps[j].StepID != ds2.Scenarios[i].Steps[j].StepID {
				t.Errorf("scenario %d step %d ID differs: %q vs %q", i, j, ds1.Scenarios[i].Steps[j].StepID, ds2.Scenarios[i].Steps[j].StepID)
			}
		}
	}
}

// TestClassifyDelta verifies the delta classification logic.
func TestClassifyDelta(t *testing.T) {
	cases := []struct {
		before, after bool
		want          string
	}{
		{true, true, "preserved"},
		{false, true, "improved"},
		{true, false, "regressed"},
		{false, false, "unchanged"},
	}
	for _, tc := range cases {
		got := ClassifyDelta(tc.before, tc.after)
		if got != tc.want {
			t.Errorf("ClassifyDelta(%v, %v) = %q, want %q", tc.before, tc.after, got, tc.want)
		}
	}
}

// TestSummarize verifies the summary aggregation.
func TestSummarize(t *testing.T) {
	results := []ScenarioQuality{
		{ScenarioID: "a", PassedBefore: true, PassedAfter: true, Delta: "preserved", StepsPassedBefore: 10, StepsPassedAfter: 10, StepsTotal: 10},
		{ScenarioID: "b", PassedBefore: false, PassedAfter: true, Delta: "improved", StepsPassedBefore: 5, StepsPassedAfter: 10, StepsTotal: 10},
		{ScenarioID: "c", PassedBefore: true, PassedAfter: false, Delta: "regressed", StepsPassedBefore: 10, StepsPassedAfter: 3, StepsTotal: 10},
	}
	s := Summarize(results)
	if s.TotalScenarios != 3 {
		t.Errorf("TotalScenarios = %d, want 3", s.TotalScenarios)
	}
	if s.PassedBefore != 2 {
		t.Errorf("PassedBefore = %d, want 2", s.PassedBefore)
	}
	if s.PassedAfter != 2 {
		t.Errorf("PassedAfter = %d, want 2", s.PassedAfter)
	}
	if s.Improved != 1 {
		t.Errorf("Improved = %d, want 1", s.Improved)
	}
	if s.Regressed != 1 {
		t.Errorf("Regressed = %d, want 1", s.Regressed)
	}
	if s.Preserved != 1 {
		t.Errorf("Preserved = %d, want 1", s.Preserved)
	}
	if s.TotalStepsBefore != 25 {
		t.Errorf("TotalStepsBefore = %d, want 25", s.TotalStepsBefore)
	}
	if s.TotalStepsAfter != 23 {
		t.Errorf("TotalStepsAfter = %d, want 23", s.TotalStepsAfter)
	}
	if s.TotalSteps != 30 {
		t.Errorf("TotalSteps = %d, want 30", s.TotalSteps)
	}
}

// TestQualityReportEndToEnd verifies that a quality report can be
// constructed from two dataset replays.
func TestQualityReportEndToEnd(t *testing.T) {
	results := []ScenarioQuality{
		{
			ScenarioID:        ScenarioMandatoryEndToEnd,
			PassedBefore:      true,
			PassedAfter:       true,
			Delta:             ClassifyDelta(true, true),
			StepsPassedBefore: 10,
			StepsPassedAfter:  10,
			StepsTotal:        10,
		},
		{
			ScenarioID:        ScenarioNegativeMisattribution,
			PassedBefore:      true,
			PassedAfter:       true,
			Delta:             ClassifyDelta(true, true),
			StepsPassedBefore: 2,
			StepsPassedAfter:  2,
			StepsTotal:        2,
		},
		{
			ScenarioID:        ScenarioNegativePartialEvidence,
			PassedBefore:      true,
			PassedAfter:       false,
			Delta:             ClassifyDelta(true, false),
			StepsPassedBefore: 3,
			StepsPassedAfter:  2,
			StepsTotal:        3,
			Notes:             "partial evidence scenario regressed: step 3 failed",
		},
	}
	report := QualityReport{
		ReportVersion:        "1.0",
		DatasetVersionBefore: "1.0",
		DatasetVersionAfter:  "1.0",
		ScenarioResults:      results,
		Summary:              Summarize(results),
		ChangedComponents:    []string{"correlation rule set"},
	}
	if report.Summary.TotalScenarios != 3 {
		t.Errorf("Summary.TotalScenarios = %d, want 3", report.Summary.TotalScenarios)
	}
	if report.Summary.Regressed != 1 {
		t.Errorf("Summary.Regressed = %d, want 1", report.Summary.Regressed)
	}
	if len(report.ChangedComponents) != 1 {
		t.Errorf("ChangedComponents len = %d, want 1", len(report.ChangedComponents))
	}
}

// findScenario returns the scenario with the given ID from the dataset.
func findScenario(ds Dataset, id ScenarioID) (Scenario, bool) {
	for _, sc := range ds.Scenarios {
		if sc.ID == id {
			return sc, true
		}
	}
	return Scenario{}, false
}

// TestDatasetMigrationHint verifies old snapshots remain readable and produce a
// migration hint when their dataset version predates the current unified
// evidence model (M95 acceptance: DatasetVersion upgrades keep old snapshots
// readable with a migration hint).
func TestDatasetMigrationHint(t *testing.T) {
	if got := DatasetMigrationHint(DatasetVersion); got != "" {
		t.Errorf("current version must produce no hint, got %q", got)
	}
	for _, old := range []string{"1.1", "1.0", "0.9", ""} {
		if got := DatasetMigrationHint(old); got == "" {
			t.Errorf("old version %q must produce a migration hint", old)
		}
	}
}
