package main

// Coverage for the golden EngineContracts adapter that wires the live
// engine catalogs into the M56/M82 golden replay contracts.

import (
	"testing"

	"k8s-aiops.local/backend/internal/posture"
)

func TestGoldenEngineContractsSanity(t *testing.T) {
	c := goldenEngineContracts()
	if c.Versions.SignalVersion == "" || c.Versions.SLOVersion == "" || c.Versions.CorrelationVersion == "" {
		t.Errorf("engine contracts versions incomplete: %+v", c.Versions)
	}
	if c.AnalyzerDiscovery == nil {
		t.Fatal("AnalyzerDiscovery contract missing")
	}
	if len(c.ValidPlanStatuses) == 0 || len(c.ValidVerificationStatuses) == 0 {
		t.Errorf("status maps empty: %+v %+v", c.ValidPlanStatuses, c.ValidVerificationStatuses)
	}
	if !c.ValidPlanStatuses["approved"] {
		t.Error("approved plan status should be valid")
	}
	if !c.ValidVerificationStatuses["effective"] {
		t.Error("effective verification status should be valid")
	}
}

func TestAnalyzerDiscoveryContract(t *testing.T) {
	d := goldenAnalyzerDiscovery()
	if d == nil {
		t.Fatal("goldenAnalyzerDiscovery() = nil")
	}
	if d.SchemaVersion != "1.0" {
		t.Errorf("schema version = %q", d.SchemaVersion)
	}
	if len(d.PostureDomains) == 0 {
		t.Error("posture domains should be non-empty")
	}
	if len(d.InsightKinds) == 0 {
		t.Error("insight kinds should be non-empty")
	}
	if len(d.DiagnosisRules) == 0 {
		t.Error("diagnosis rules should be non-empty")
	}
	if len(d.InspectionRules) == 0 {
		t.Error("inspection rules should be non-empty")
	}
	if len(d.Operations) == 0 {
		t.Error("operations should be non-empty")
	}
}

func TestDomainsToStrings(t *testing.T) {
	if got := domainsToStrings(nil); len(got) != 0 {
		t.Errorf("domainsToStrings(nil) = %v", got)
	}
	got := domainsToStrings([]posture.Domain{"network", "image"})
	if len(got) != 2 || got[0] != "network" || got[1] != "image" {
		t.Errorf("domainsToStrings = %v", got)
	}
}
