package capacity

import (
	"testing"
	"time"
)

func testNow() time.Time {
	return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
}

// mkSamples builds a usage series from per-day offsets (in days; negative =
// past relative to now) and fractions of capacity.
func mkSamples(capacity int64, offsets []float64, fracs []float64, now time.Time) []Sample {
	s := make([]Sample, len(offsets))
	for i := range offsets {
		s[i] = Sample{
			Timestamp: now.Add(time.Duration(offsets[i] * 24 * float64(time.Hour))),
			Value:     fracs[i] * float64(capacity),
		}
	}
	return s
}

func linearOffsets(n int) []float64 {
	offsets := make([]float64, n)
	for i := 0; i < n; i++ {
		offsets[i] = float64(-(n - 1) + i) // -10..0 for n=11
	}
	return offsets
}

func TestEvaluate_EmptyBundle_SkipsAndReturnsNonNullFindings(t *testing.T) {
	status := Evaluate(7, Inputs{}, testNow())
	if status.Findings == nil {
		t.Fatal("Findings must never be nil")
	}
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings for empty bundle, got %d", len(status.Findings))
	}
	if status.Total != 2 || status.Failed != 0 || status.Passed != 2 {
		t.Fatalf("Total/Failed/Passed = %d/%d/%d, want 2/0/2", status.Total, status.Failed, status.Passed)
	}
}

func TestEvaluate_CapacityOnlyNoSamples_NoFinding(t *testing.T) {
	in := Inputs{
		CPU:    ResourceTrend{Capacity: 100},
		Memory: ResourceTrend{Capacity: 1000},
	}
	status := Evaluate(7, in, testNow())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings without a usage series, got %d", len(status.Findings))
	}
	if status.CPUCapacityNanocores != 100 || status.MemCapacityBytes != 1000 {
		t.Fatalf("capacity not propagated: cpu=%d mem=%d", status.CPUCapacityNanocores, status.MemCapacityBytes)
	}
}

func TestEvaluate_SingleSample_NoTrend(t *testing.T) {
	now := testNow()
	in := Inputs{
		CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, []float64{0}, []float64{0.5}, now)},
	}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 0 {
		t.Fatalf("a single sample cannot yield a trend; got %d findings", len(status.Findings))
	}
}

func TestEvaluate_FlatTrend_NoFinding(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.5 // constant
	}
	in := Inputs{CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)}}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 0 {
		t.Fatalf("flat trend must not raise a finding; got %d", len(status.Findings))
	}
}

func TestEvaluate_DecliningTrend_NoFinding(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.6 - 0.05*float64(i) // 0.6 -> 0.1
	}
	in := Inputs{CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)}}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 0 {
		t.Fatalf("declining trend must not raise a finding; got %d", len(status.Findings))
	}
}

func TestEvaluate_GrowingTrend_WarningWithDaysToSaturation(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.1 + 0.05*float64(i) // 0.1 -> 0.6
	}
	in := Inputs{
		CPU:    ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)},
		Memory: ResourceTrend{Capacity: 1000, Samples: mkSamples(1000, offs, fracs, now)},
	}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 2 {
		t.Fatalf("expected 2 findings (cpu+mem both growing), got %d: %+v", len(status.Findings), status.Findings)
	}
	if status.Failed != 2 || status.Passed != 0 {
		t.Fatalf("Failed/Passed = %d/%d, want 2/0", status.Failed, status.Passed)
	}
	cpu := status.Findings[0]
	if cpu.Severity != SeverityWarning {
		t.Errorf("cpu severity = %q, want warning", cpu.Severity)
	}
	if cpu.Details["days_to_saturation"] != "8" {
		t.Errorf("cpu days_to_saturation = %q, want 8", cpu.Details["days_to_saturation"])
	}
	if cpu.Resource.Name != "cpu" {
		t.Errorf("cpu resource name = %q, want cpu", cpu.Resource.Name)
	}
	if status.CPUSaturationInDays < 0 {
		t.Errorf("cpu saturation in days = %v, want >= 0", status.CPUSaturationInDays)
	}
}

func TestEvaluate_AlreadyOverCapacity_Critical(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.8 + 0.05*float64(i) // 0.8 -> 1.3
	}
	in := Inputs{CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)}}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(status.Findings))
	}
	if status.Findings[0].Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical", status.Findings[0].Severity)
	}
	if status.Findings[0].Details["days_to_saturation"] != "inf" {
		t.Errorf("days_to_saturation = %q, want inf (already over)", status.Findings[0].Details["days_to_saturation"])
	}
}

func TestEvaluate_SaturatesWithinCriticalWindow_Critical(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.9 + 0.02*float64(i) // 0.9 -> 1.1
	}
	in := Inputs{CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)}}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(status.Findings))
	}
	if status.Findings[0].Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical (saturates <=7d)", status.Findings[0].Severity)
	}
}

func TestEvaluate_CustomHorizonChangesSeverity(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	fracs := make([]float64, len(offs))
	for i := range fracs {
		fracs[i] = 0.5 + 0.02*float64(i) // 0.5 -> 0.7, slope 0.02
	}
	// Default horizon (30d): projected = 0.7 + 0.02*30 = 1.3 -> warning,
	// daysToSaturation = (1-0.7)/0.02 = 15 (>7) -> warning.
	def := Evaluate(7, Inputs{CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, fracs, now)}}, now)
	if len(def.Findings) != 1 || def.Findings[0].Severity != SeverityWarning {
		t.Fatalf("default horizon: want 1 warning, got %d/%s", len(def.Findings), sevOrNone(def.Findings))
	}
	// Horizon 10d: projected = 0.7 + 0.02*10 = 0.9 -> warning, but
	// daysToSaturation = 15 which is unchanged, so still warning. Use a steeper
	// slope to force critical within a short horizon.
	steep := make([]float64, len(offs))
	for i := range steep {
		steep[i] = 0.9 + 0.02*float64(i) // 0.9 -> 1.1, slope 0.02 -> 5d to saturate
	}
	short := Evaluate(7, Inputs{HorizonDays: 10, CPU: ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, steep, now)}}, now)
	if len(short.Findings) != 1 || short.Findings[0].Severity != SeverityCritical {
		t.Fatalf("horizon 10d with 5d-to-saturation: want 1 critical, got %d/%s", len(short.Findings), sevOrNone(short.Findings))
	}
}

func TestEvaluate_FindingsSortedCriticalFirst(t *testing.T) {
	now := testNow()
	offs := linearOffsets(11)
	// CPU: critical within window.
	cpuFracs := make([]float64, len(offs))
	for i := range cpuFracs {
		cpuFracs[i] = 0.9 + 0.02*float64(i)
	}
	// Memory: warning only.
	memFracs := make([]float64, len(offs))
	for i := range memFracs {
		memFracs[i] = 0.1 + 0.05*float64(i)
	}
	in := Inputs{
		CPU:    ResourceTrend{Capacity: 100, Samples: mkSamples(100, offs, cpuFracs, now)},
		Memory: ResourceTrend{Capacity: 1000, Samples: mkSamples(1000, offs, memFracs, now)},
	}
	status := Evaluate(7, in, now)
	if len(status.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(status.Findings))
	}
	if status.Findings[0].Severity != SeverityCritical {
		t.Errorf("first finding severity = %q, want critical first", status.Findings[0].Severity)
	}
	if status.Findings[1].Severity != SeverityWarning {
		t.Errorf("second finding severity = %q, want warning", status.Findings[1].Severity)
	}
}

func sevOrNone(f []Finding) string {
	if len(f) == 0 {
		return "none"
	}
	return f[0].Severity
}
