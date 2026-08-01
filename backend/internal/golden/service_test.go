package golden

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// --- EngineContracts test fixtures ---

func testContracts() EngineContracts {
	return EngineContracts{
		Versions: EngineVersions{
			SignalVersion:       "1.0",
			TopologyVersion:     "1.0",
			SLOVersion:          "1.0",
			CorrelationVersion:  "1.0",
			InvestigatorVersion: "1.0",
			AutomationVersion:   "1.0",
			VerifierVersion:     "1.0",
		},
		ValidPlanStatuses: map[string]bool{
			"draft": true, "previewed": true, "approved": true,
			"executing": true, "succeeded": true, "failed": true,
			"expired": true, "cancelled": true, "verified": true,
		},
		ValidVerificationStatuses: map[string]bool{
			"pending": true, "effective": true, "ineffective": true,
			"failed": true, "unknown": true,
		},
	}
}

// --- ReplayRunner tests ---

func TestReplayRunner_AllScenariosPass(t *testing.T) {
	runner := NewReplayRunner(testContracts())
	ds := DefaultDataset()
	results, err := runner.Run(context.Background(), ds)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 scenario results, got %d", len(results))
	}
	for _, sr := range results {
		if !sr.Passed {
			t.Errorf("scenario %s did not pass", sr.ScenarioID)
			for _, step := range sr.Steps {
				if !step.Passed {
					t.Errorf("  step %s failed: %s", step.StepID, step.Notes)
				}
			}
		}
	}
}

func TestReplayRunner_MandatoryScenarioHas10Steps(t *testing.T) {
	runner := NewReplayRunner(testContracts())
	ds := DefaultDataset()
	results, _ := runner.Run(context.Background(), ds)

	var mandatory *ScenarioResult
	for i := range results {
		if results[i].ScenarioID == ScenarioMandatoryEndToEnd {
			mandatory = &results[i]
			break
		}
	}
	if mandatory == nil {
		t.Fatal("mandatory scenario not found in results")
	}
	if len(mandatory.Steps) != 10 {
		t.Errorf("mandatory scenario has %d steps, want 10", len(mandatory.Steps))
	}
}

func TestReplayRunner_MissingSignalVersion(t *testing.T) {
	c := testContracts()
	c.Versions.SignalVersion = ""
	runner := NewReplayRunner(c)
	ds := DefaultDataset()
	results, err := runner.Run(context.Background(), ds)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Scenarios with ExpectSignalCaptured steps should fail.
	for _, sr := range results {
		for _, step := range sr.Steps {
			// Find the capture_signals step
			if step.StepID == StepCaptureSignals && !step.Passed {
				// Good — this step should fail
				goto nextScenario
			}
		}
		// If all steps passed, that's wrong for scenarios with signal capture
		if sr.ScenarioID == ScenarioMandatoryEndToEnd {
			t.Errorf("mandatory scenario should have failed signal capture step")
		}
	nextScenario:
	}
}

func TestReplayRunner_InvalidPlanStatus(t *testing.T) {
	c := testContracts()
	delete(c.ValidPlanStatuses, "approved")
	runner := NewReplayRunner(c)
	ds := DefaultDataset()
	results, _ := runner.Run(context.Background(), ds)

	mandatory := findResult(results, ScenarioMandatoryEndToEnd)
	if mandatory == nil {
		t.Fatal("mandatory scenario not found")
	}
	for _, step := range mandatory.Steps {
		if step.StepID == StepPreviewApproveRollback && step.Passed {
			t.Error("preview/approve step should fail with invalid 'approved' status")
		}
	}
}

func TestReplayRunner_InvalidVerificationStatus(t *testing.T) {
	c := testContracts()
	delete(c.ValidVerificationStatuses, "effective")
	runner := NewReplayRunner(c)
	ds := DefaultDataset()
	results, _ := runner.Run(context.Background(), ds)

	mandatory := findResult(results, ScenarioMandatoryEndToEnd)
	if mandatory == nil {
		t.Fatal("mandatory scenario not found")
	}
	for _, step := range mandatory.Steps {
		if step.StepID == StepExecuteVerify && step.Passed {
			t.Error("execute/verify step should fail with invalid 'effective' verification status")
		}
	}
}

func TestReplayRunner_ContextCancel(t *testing.T) {
	runner := NewReplayRunner(testContracts())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err := runner.Run(ctx, DefaultDataset())
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

// --- BuildScenarioQuality tests ---

func TestBuildScenarioQuality_NoBaseline(t *testing.T) {
	result := ScenarioResult{
		ScenarioID: ScenarioMandatoryEndToEnd,
		Passed:     true,
		Steps:      []StepResult{{StepID: StepCaptureSignals, Passed: true}},
	}
	sq := BuildScenarioQuality(result, nil)
	if !sq.PassedBefore {
		t.Error("PassedBefore should be true when no baseline (first run establishes baseline)")
	}
	if !sq.PassedAfter {
		t.Error("PassedAfter should be true")
	}
	if sq.Delta != "preserved" {
		t.Errorf("Delta = %q, want %q", sq.Delta, "preserved")
	}
}

func TestBuildScenarioQuality_WithBaseline(t *testing.T) {
	result := ScenarioResult{
		ScenarioID: ScenarioMandatoryEndToEnd,
		Passed:     false,
		Steps:      []StepResult{{StepID: StepCaptureSignals, Passed: false}},
	}
	baseline := &ScenarioQuality{
		ScenarioID:       ScenarioMandatoryEndToEnd,
		PassedAfter:      true,
		StepsPassedAfter: 10,
	}
	sq := BuildScenarioQuality(result, baseline)
	if !sq.PassedBefore {
		t.Error("PassedBefore should be true from baseline")
	}
	if sq.PassedAfter {
		t.Error("PassedAfter should be false")
	}
	if sq.Delta != "regressed" {
		t.Errorf("Delta = %q, want %q", sq.Delta, "regressed")
	}
	if sq.StepsPassedBefore != 10 {
		t.Errorf("StepsPassedBefore = %d, want 10", sq.StepsPassedBefore)
	}
}

// --- FileReportStorage tests ---

func TestFileReportStorage_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileReportStorage(dir)

	report := QualityReport{
		ReportVersion:        ReportVersion,
		DatasetVersionBefore: "1.0",
		DatasetVersionAfter:  "1.0",
		ScenarioResults:      []ScenarioQuality{},
		Summary:              QualitySummary{TotalScenarios: 3},
		GeneratedAt:          time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}

	if err := storage.Save(report); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := storage.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest failed: %v", err)
	}
	if loaded.ReportVersion != ReportVersion {
		t.Errorf("ReportVersion = %q, want %q", loaded.ReportVersion, ReportVersion)
	}
	if loaded.Summary.TotalScenarios != 3 {
		t.Errorf("TotalScenarios = %d, want 3", loaded.Summary.TotalScenarios)
	}
}

func TestFileReportStorage_LoadLatest_Empty(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileReportStorage(dir)
	_, err := storage.LoadLatest()
	if err != ErrNoReport {
		t.Errorf("expected ErrNoReport, got %v", err)
	}
}

func TestFileReportStorage_LoadLatest_NonExistentDir(t *testing.T) {
	storage := NewFileReportStorage(filepath.Join(t.TempDir(), "nonexistent"))
	_, err := storage.LoadLatest()
	if err != ErrNoReport {
		t.Errorf("expected ErrNoReport, got %v", err)
	}
}

func TestFileReportStorage_LoadLatest_PicksMostRecent(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileReportStorage(dir)

	older := QualityReport{
		ReportVersion:       ReportVersion,
		DatasetVersionAfter: "1.0",
		GeneratedAt:         time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		Summary:             QualitySummary{TotalScenarios: 3, PassedAfter: 2},
	}
	newer := QualityReport{
		ReportVersion:       ReportVersion,
		DatasetVersionAfter: "1.0",
		GeneratedAt:         time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		Summary:             QualitySummary{TotalScenarios: 3, PassedAfter: 3},
	}

	if err := storage.Save(older); err != nil {
		t.Fatalf("Save older failed: %v", err)
	}
	if err := storage.Save(newer); err != nil {
		t.Fatalf("Save newer failed: %v", err)
	}

	loaded, err := storage.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest failed: %v", err)
	}
	if loaded.Summary.PassedAfter != 3 {
		t.Errorf("Loaded PassedAfter = %d, want 3 (newer report)", loaded.Summary.PassedAfter)
	}
}

func TestNopReportStorage(t *testing.T) {
	s := NopReportStorage{}
	if err := s.Save(QualityReport{}); err != nil {
		t.Errorf("NopReportStorage.Save should not fail: %v", err)
	}
	_, err := s.LoadLatest()
	if err != ErrNoReport {
		t.Errorf("NopReportStorage.LoadLatest should return ErrNoReport, got %v", err)
	}
}

// --- Service tests ---

func TestService_GetLatestReport_NoReport(t *testing.T) {
	svc := NewService(testContracts(), NopReportStorage{}, zap.NewNop())
	_, err := svc.GetLatestReport()
	if err != ErrNoReport {
		t.Errorf("expected ErrNoReport, got %v", err)
	}
}

func TestService_RunReplay_Success(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileReportStorage(dir)
	svc := NewService(testContracts(), storage, zap.NewNop())

	taskID, err := svc.RunReplay(context.Background())
	if err != nil {
		t.Fatalf("RunReplay failed: %v", err)
	}
	if taskID == "" {
		t.Error("taskID should not be empty")
	}

	// Wait for async completion.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("replay did not complete within 5s")
		default:
		}
		view, ok := svc.GetTask(taskID)
		if !ok {
			t.Fatal("task not found")
		}
		if view.Status == ReplayTaskSucceeded {
			break
		}
		if view.Status == ReplayTaskFailed {
			t.Fatalf("replay failed: %s", view.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify the report was saved.
	report, err := svc.GetLatestReport()
	if err != nil {
		t.Fatalf("GetLatestReport failed: %v", err)
	}
	if report.ReportVersion != ReportVersion {
		t.Errorf("ReportVersion = %q, want %q", report.ReportVersion, ReportVersion)
	}
	if len(report.ScenarioResults) != 3 {
		t.Errorf("ScenarioResults len = %d, want 3", len(report.ScenarioResults))
	}
	if report.Summary.TotalScenarios != 3 {
		t.Errorf("Summary.TotalScenarios = %d, want 3", report.Summary.TotalScenarios)
	}
	if report.Summary.Regressed != 0 {
		t.Errorf("Summary.Regressed = %d, want 0 (first run, all preserved)", report.Summary.Regressed)
	}
}

func TestService_RunReplay_DetectsRegression(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileReportStorage(dir)

	// Save a baseline report where all scenarios pass.
	baseline := QualityReport{
		ReportVersion:       ReportVersion,
		DatasetVersionAfter: "1.0",
		EngineVersionsAfter: testContracts().Versions,
		GeneratedAt:         time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		ScenarioResults: []ScenarioQuality{
			{ScenarioID: ScenarioMandatoryEndToEnd, PassedAfter: true, StepsPassedAfter: 10, StepsTotal: 10, Delta: "preserved"},
			{ScenarioID: ScenarioNegativeMisattribution, PassedAfter: true, StepsPassedAfter: 2, StepsTotal: 2, Delta: "preserved"},
			{ScenarioID: ScenarioNegativePartialEvidence, PassedAfter: true, StepsPassedAfter: 3, StepsTotal: 3, Delta: "preserved"},
		},
		Summary: QualitySummary{TotalScenarios: 3, PassedAfter: 3, Preserved: 3},
	}
	if err := storage.Save(baseline); err != nil {
		t.Fatalf("Save baseline failed: %v", err)
	}

	// Run replay with broken contracts (missing correlation version → mandatory scenario fails).
	brokenContracts := testContracts()
	brokenContracts.Versions.CorrelationVersion = ""
	svc := NewService(brokenContracts, storage, zap.NewNop())

	taskID, err := svc.RunReplay(context.Background())
	if err != nil {
		t.Fatalf("RunReplay failed: %v", err)
	}

	// Wait for completion.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("replay did not complete within 5s")
		default:
		}
		view, _ := svc.GetTask(taskID)
		if view.Status == ReplayTaskSucceeded || view.Status == ReplayTaskFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Load the report and check for regression.
	report, err := svc.GetLatestReport()
	if err != nil {
		t.Fatalf("GetLatestReport failed: %v", err)
	}

	// At least one scenario should have regressed (mandatory uses correlation).
	foundRegression := false
	for _, sq := range report.ScenarioResults {
		if sq.Delta == "regressed" {
			foundRegression = true
			break
		}
	}
	if !foundRegression {
		t.Error("expected at least one regressed scenario, but none found")
	}
}

func TestService_GetTask_UnknownID(t *testing.T) {
	svc := NewService(testContracts(), NopReportStorage{}, zap.NewNop())
	_, ok := svc.GetTask("nonexistent")
	if ok {
		t.Error("GetTask should return false for unknown task ID")
	}
}

// --- Helpers ---

func findResult(results []ScenarioResult, id ScenarioID) *ScenarioResult {
	for i := range results {
		if results[i].ScenarioID == id {
			return &results[i]
		}
	}
	return nil
}

// Ensure the test binary cleans up temp dirs.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
