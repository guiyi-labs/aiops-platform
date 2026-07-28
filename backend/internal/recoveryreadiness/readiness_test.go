package recoveryreadiness

import (
	"path/filepath"
	"testing"
	"time"
)

var evaluationTime = time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

func TestEvaluateAcceptsCompleteRecoveryContract(t *testing.T) {
	policy, evidence := loadFixtures(t)
	report := Evaluate(policy, evidence, evaluationTime)
	if !report.ReadyForPITRHAImplementation || report.ProductionRecoveryValidated || report.Failed != 0 || report.Passed != 15 {
		t.Fatalf("Evaluate() = %#v, want 15-check implementation-ready report without production validation", report)
	}
}

func TestEvaluateRejectsRecoveryDowngrades(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Policy, *LogicalRestoreEvidence)
		code   string
	}{
		{"one backup copy", func(p *Policy, _ *LogicalRestoreEvidence) { p.Storage.IndependentCopies = 1 }, "recovery.storage"},
		{"PITR gap exceeds RPO", func(p *Policy, _ *LogicalRestoreEvidence) { p.PITR.WALArchiveIntervalSeconds = 301 }, "recovery.pitr"},
		{"HA lacks fencing", func(p *Policy, _ *LogicalRestoreEvidence) { p.HA.WriterFencing = false }, "recovery.ha"},
		{"stale restore", func(_ *Policy, e *LogicalRestoreEvidence) { e.VerifiedAt = "2026-01-01T00:00:00Z" }, "evidence.freshness"},
		{"snapshot mismatch", func(_ *Policy, e *LogicalRestoreEvidence) { e.RestoredSnapshot["users"] = 1 }, "evidence.integrity"},
		{"cleanup incomplete", func(_ *Policy, e *LogicalRestoreEvidence) { e.Cleanup.TemporaryFilesDeleted = false }, "evidence.cleanup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, evidence := loadFixtures(t)
			tt.mutate(&policy, &evidence)
			report := Evaluate(policy, evidence, evaluationTime)
			if report.ReadyForPITRHAImplementation || !failedCheck(report, tt.code) {
				t.Fatalf("Evaluate() = %#v, want failed %s", report, tt.code)
			}
		})
	}
}

func TestEvaluateAllowsCurrentBoundedRiskAcceptance(t *testing.T) {
	policy, evidence := loadFixtures(t)
	policy.PITR = PITRPolicy{RiskAcceptanceOwner: "database-risk-owner", RiskAcceptanceExpiresAt: "2026-08-31T00:00:00Z"}
	policy.Backup.FullIntervalHours = 1
	policy.Objectives.RPOMinutes = 60
	policy.Drills.PITRIntervalDays = 0
	policy.HA = HAPolicy{RiskAcceptanceOwner: "availability-risk-owner", RiskAcceptanceExpiresAt: "2026-08-31T00:00:00Z"}
	policy.Drills.FailoverIntervalDays = 0
	report := Evaluate(policy, evidence, evaluationTime)
	if !report.ReadyForPITRHAImplementation {
		t.Fatalf("Evaluate() = %#v, want current explicit risk acceptance to pass policy readiness", report)
	}
}

func TestLoadPolicyRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	for name, contents := range map[string]string{
		"unknown":  `{"format":"aiops.recovery-readiness-policy/v1","password":"forbidden"}`,
		"trailing": `{} trailing`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.json")
			if err := writeTestFile(path, []byte(contents)); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPolicy(path); err == nil {
				t.Fatal("LoadPolicy() error = nil, want strict JSON rejection")
			}
		})
	}
}

func loadFixtures(t *testing.T) (Policy, LogicalRestoreEvidence) {
	t.Helper()
	policy, err := LoadPolicy(filepath.Join("testdata", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := LoadEvidence(filepath.Join("testdata", "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	return policy, evidence
}

func failedCheck(report Report, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code {
			return !check.Passed
		}
	}
	return false
}
