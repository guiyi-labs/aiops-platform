package golden

import (
	"context"
	"testing"
)

// TestReplayRunner_AnalyzerScenarioDetectsMissingSnapshot locks the M82
// behavior: the analyzer-discovery scenario must FAIL when the snapshot is
// nil or empty, so a silently-shrunk analyzer surface cannot pass.
func TestReplayRunner_AnalyzerScenarioDetectsMissingSnapshot(t *testing.T) {
	contracts := testContracts()
	contracts.AnalyzerDiscovery = nil
	runner := NewReplayRunner(contracts)
	results, err := runner.Run(context.Background(), DefaultDataset())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	for _, sr := range results {
		if sr.ScenarioID != ScenarioAnalyzerDiscovery {
			continue
		}
		if sr.Passed {
			t.Fatal("analyzer discovery scenario must fail when the snapshot is nil")
		}
		if len(sr.Steps) != 1 || sr.Steps[0].Notes == "" {
			t.Fatalf("expected a failing step with notes, got %+v", sr.Steps)
		}
		return
	}
	t.Fatal("analyzer discovery scenario not found in dataset")
}

// TestReplayRunner_AnalyzerScenarioPassesWithSnapshot verifies the happy path.
func TestReplayRunner_AnalyzerScenarioPassesWithSnapshotContractSnapshot(t *testing.T) {
	contracts := testContracts() // includes testAnalyzerSnapshot()
	runner := NewReplayRunner(contracts)
	results, err := runner.Run(context.Background(), DefaultDataset())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	for _, sr := range results {
		if sr.ScenarioID == ScenarioAnalyzerDiscovery && !sr.Passed {
			t.Fatalf("analyzer discovery scenario must pass with a populated snapshot: %+v", sr.Steps)
		}
	}
}