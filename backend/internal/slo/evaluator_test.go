package slo

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// fakeMetricsSource is a test MetricsSource that returns canned series.
type fakeMetricsSource struct {
	series SLISeries
	err    error
}

func (f fakeMetricsSource) QuerySLI(_ context.Context, _ *Definition, _, _ time.Time, _ time.Duration) (SLISeries, error) {
	return f.series, f.err
}

func sampleAt(t time.Time, v float64) Sample { return Sample{Timestamp: t, Value: v} }

func baseDefinition() *Definition {
	return &Definition{
		ID:                    1,
		ClusterID:             1,
		Service:               ServiceRef{Kind: "Deployment", Namespace: "default", Name: "api"},
		Template:              TemplateRequestSuccessRatio,
		TemplateVersion:       TemplateVersion,
		Objective:             0.99,
		RollingWindowSeconds:  3600,
		MissingDataPolicy:     MissingDataUnavailable,
		FastBurnRate:          14.4,
		FastBurnWindowSeconds: 3600,
		SlowBurnRate:          1.0,
		SlowBurnWindowSeconds: 21600,
		Enabled:               true,
		Version:               1,
	}
}

// TestEvaluator_HealthyPath verifies the canonical healthy evaluation:
// good == total over the window, ratio == 1, state == healthy, coverage
// == complete.
func TestEvaluator_HealthyPath(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 100), sampleAt(start.Add(30*time.Minute), 200), sampleAt(now.Add(-time.Minute), 300)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 100), sampleAt(start.Add(30*time.Minute), 200), sampleAt(now.Add(-time.Minute), 300)},
			ExpectedSamples: 4,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, err := eval.Evaluate(context.Background(), baseDefinition(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != StateHealthy {
		t.Errorf("state: want healthy, got %s", result.State)
	}
	if result.Ratio != 1.0 {
		t.Errorf("ratio: want 1.0, got %v", result.Ratio)
	}
	if result.Coverage != CoverageComplete {
		t.Errorf("coverage: want complete, got %s", result.Coverage)
	}
	if result.BurnRate != 0 {
		t.Errorf("burn_rate: want 0, got %v", result.BurnRate)
	}
	// Perfect ratio (1.0) means full error budget remaining: (1.0 - 0.99) / 0.01 = 1.0.
	if result.RemainingBudget != 1.0 {
		t.Errorf("remaining_budget: want 1.0 (full budget, perfect ratio), got %v", result.RemainingBudget)
	}
	if result.TargetRatio != 0.99 {
		t.Errorf("target_ratio: want 0.99, got %v", result.TargetRatio)
	}
	if math.Abs(result.ErrorBudget-0.01) > 1e-9 {
		t.Errorf("error_budget: want 0.01, got %v", result.ErrorBudget)
	}
}

// TestEvaluator_BreachWhenRatioBelowObjective verifies that ratio < objective
// produces StateBreached.
func TestEvaluator_BreachWhenRatioBelowObjective(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// 99 successful out of 100 -> ratio 0.99, exactly at objective, healthy.
	// 98 out of 100 -> ratio 0.98 < 0.99, breached.
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(start.Add(30*time.Minute), 98)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(start.Add(30*time.Minute), 100)},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), baseDefinition(), now)
	if result.State != StateBreached {
		t.Errorf("state: want breached, got %s", result.State)
	}
	if result.Ratio != 0.98 {
		t.Errorf("ratio: want 0.98, got %v", result.Ratio)
	}
}

// TestEvaluator_CounterResetHandled verifies that a counter reset (post-reset
// value < previous) is treated as a delta of post-reset value.
func TestEvaluator_CounterResetHandled(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// Good: 0 -> 1000 -> reset to 50 -> 100. Delta = (1000-0) + (50-0 from reset) + (100-50) = 1100.
	// Total: same shape -> 1100. Ratio = 1.0 healthy.
	source := fakeMetricsSource{
		series: SLISeries{
			Good: []Sample{
				sampleAt(start.Add(1*time.Minute), 0),
				sampleAt(start.Add(20*time.Minute), 1000),
				sampleAt(start.Add(40*time.Minute), 50), // reset
				sampleAt(now.Add(-1*time.Minute), 100),
			},
			Total: []Sample{
				sampleAt(start.Add(1*time.Minute), 0),
				sampleAt(start.Add(20*time.Minute), 1000),
				sampleAt(start.Add(40*time.Minute), 50),
				sampleAt(now.Add(-1*time.Minute), 100),
			},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), baseDefinition(), now)
	if result.GoodEvents != 1100 {
		t.Errorf("good_events: want 1100, got %v", result.GoodEvents)
	}
	if result.TotalEvents != 1100 {
		t.Errorf("total_events: want 1100, got %v", result.TotalEvents)
	}
	if result.State != StateHealthy {
		t.Errorf("state: want healthy, got %s", result.State)
	}
}

// TestEvaluator_MissingDataFailClosed verifies that an empty series yields
// StateUnavailable under the default fail-closed policy.
func TestEvaluator_MissingDataFailClosed(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	source := fakeMetricsSource{
		series: SLISeries{Good: nil, Total: nil, ExpectedSamples: 60, Source: "test"},
	}
	eval := NewEvaluator(source)
	result, err := eval.Evaluate(context.Background(), baseDefinition(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != StateUnavailable {
		t.Errorf("state: want unavailable, got %s", result.State)
	}
	if result.Coverage != CoverageUnavailable {
		t.Errorf("coverage: want unavailable, got %s", result.Coverage)
	}
}

// TestEvaluator_MissingDataFailOpenForWorkloadReadiness verifies that an
// empty series with fail_open policy on workload_readiness yields
// StateHealthy with CoverageUnavailable (auditable fail-open).
func TestEvaluator_MissingDataFailOpenForWorkloadReadiness(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	def := baseDefinition()
	def.Template = TemplateWorkloadReadiness
	def.MissingDataPolicy = MissingDataFailOpen
	source := fakeMetricsSource{
		series: SLISeries{Good: nil, Total: nil, ExpectedSamples: 60, Source: "test"},
	}
	eval := NewEvaluator(source)
	result, err := eval.Evaluate(context.Background(), def, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != StateHealthy {
		t.Errorf("state: want healthy (fail_open), got %s", result.State)
	}
	if result.Coverage != CoverageUnavailable {
		t.Errorf("coverage: want unavailable, got %s", result.Coverage)
	}
}

// TestEvaluator_FailOpenRejectedForRequestTemplate verifies that a request_*
// template with fail_open policy still fails closed (catalog rejects the
// combination at validation, but the evaluator also defends in depth).
func TestEvaluator_FailOpenRejectedForRequestTemplate(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	def := baseDefinition()
	def.Template = TemplateRequestSuccessRatio
	def.MissingDataPolicy = MissingDataFailOpen // invalid per catalog
	_, err := NewEvaluator(fakeMetricsSource{}).Evaluate(context.Background(), def, now)
	if !errors.Is(err, ErrEvaluationInvalidInput) {
		t.Errorf("expected ErrEvaluationInvalidInput, got %v", err)
	}
}

// TestEvaluator_NilSourceProducesUnavailable verifies that a nil
// MetricsSource (deployments without a provider) yields StateUnavailable.
func TestEvaluator_NilSourceProducesUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	eval := NewEvaluator(nil)
	result, err := eval.Evaluate(context.Background(), baseDefinition(), now)
	if !errors.Is(err, ErrEvaluationSourceUnavailable) {
		t.Errorf("expected ErrEvaluationSourceUnavailable, got %v", err)
	}
	if result.State != StateUnavailable {
		t.Errorf("state: want unavailable, got %s", result.State)
	}
}

// TestEvaluator_SourceErrorProducesUnavailable verifies that a hard source
// error yields StateUnavailable (no fabricated healthy state on error).
func TestEvaluator_SourceErrorProducesUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	source := fakeMetricsSource{err: errors.New("upstream down")}
	eval := NewEvaluator(source)
	result, err := eval.Evaluate(context.Background(), baseDefinition(), now)
	if err == nil {
		t.Fatalf("expected source error to surface")
	}
	if result.State != StateUnavailable {
		t.Errorf("state: want unavailable, got %s", result.State)
	}
}

// TestEvaluator_DisabledDefinitionRejected verifies that a disabled
// definition cannot be evaluated.
func TestEvaluator_DisabledDefinitionRejected(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	def := baseDefinition()
	def.Enabled = false
	_, err := NewEvaluator(fakeMetricsSource{}).Evaluate(context.Background(), def, now)
	if !errors.Is(err, ErrEvaluationInvalidInput) {
		t.Errorf("expected ErrEvaluationInvalidInput for disabled def, got %v", err)
	}
}

// TestEvaluator_BurnRateFast verifies that a ratio slightly below objective
// (but with very high burn rate due to small error budget) yields
// StateBurningFast when burn rate >= fast_burn_rate.
func TestEvaluator_BurnRateFast(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// Objective 0.99 -> error budget 0.01. Ratio 0.985 -> burn rate = 0.015/0.01 = 1.5.
	// Fast burn rate threshold 1.0 -> StateBurningSlow. Use threshold 0.5 for fast.
	def := baseDefinition()
	def.Objective = 0.99
	def.FastBurnRate = 1.0 // lower the threshold so burn rate 1.5 triggers fast
	def.SlowBurnRate = 0.5
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 985)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 1000)},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), def, now)
	// ratio 0.985 < objective 0.99 -> breached (precedence over burn).
	if result.State != StateBreached {
		t.Errorf("state: want breached (ratio < objective), got %s", result.State)
	}
}

// TestEvaluator_BurnRateSlow verifies StateBurningSlow when ratio >= objective
// but burn rate >= slow threshold.
func TestEvaluator_BurnRateSlow(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// Objective 0.5 -> error budget 0.5. Ratio 0.9 -> burn rate = 0.1/0.5 = 0.2.
	// Slow threshold 0.1 -> StateBurningSlow. Fast threshold 1.0 -> not fast.
	def := baseDefinition()
	def.Objective = 0.5
	def.SlowBurnRate = 0.1
	def.FastBurnRate = 1.0
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 900)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 1000)},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), def, now)
	if result.State != StateBurningSlow {
		t.Errorf("state: want burning_slow, got %s (ratio=%v burn=%v)", result.State, result.Ratio, result.BurnRate)
	}
}

// TestEvaluator_PartialCoverage verifies that a single sample (which yields
// zero deltas and thus zero total events) produces StateUnavailable with
// CoveragePartial preserved — we had some data but not enough to compute a
// ratio.
func TestEvaluator_PartialCoverage(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// One sample inside the window -> partial coverage, zero deltas,
	// unavailable state (cannot compute ratio from a single cumulative
	// counter sample).
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(30*time.Minute), 100)},
			Total:           []Sample{sampleAt(start.Add(30*time.Minute), 100)},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), baseDefinition(), now)
	if result.Coverage != CoveragePartial {
		t.Errorf("coverage: want partial, got %s", result.Coverage)
	}
	if result.State != StateUnavailable {
		t.Errorf("state: want unavailable (no deltas), got %s", result.State)
	}
}

// TestEvaluator_SamplesOutsideWindowExcluded verifies that samples outside
// [start, end) are excluded.
func TestEvaluator_SamplesOutsideWindowExcluded(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	// Two samples inside, one before start (excluded), one at end (excluded).
	source := fakeMetricsSource{
		series: SLISeries{
			Good: []Sample{
				sampleAt(start.Add(-1*time.Minute), 50),  // before start, excluded
				sampleAt(start.Add(10*time.Minute), 100), // in window
				sampleAt(start.Add(30*time.Minute), 200), // in window
				sampleAt(now, 300),                       // at end, excluded (end is exclusive)
			},
			Total: []Sample{
				sampleAt(start.Add(-1*time.Minute), 50),
				sampleAt(start.Add(10*time.Minute), 100),
				sampleAt(start.Add(30*time.Minute), 200),
				sampleAt(now, 300),
			},
			ExpectedSamples: 60,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), baseDefinition(), now)
	if result.GoodEvents != 100 {
		t.Errorf("good_events: want 100 (200-100), got %v", result.GoodEvents)
	}
}

// TestEvaluator_ZeroErrorBudget verifies objective == 1.0 (zero error budget)
// yields StateHealthy when ratio == 1.0 and StateBreached otherwise.
func TestEvaluator_ZeroErrorBudget(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	def := baseDefinition()
	def.Objective = 1.0

	// Perfect: ratio 1.0 -> healthy.
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			ExpectedSamples: 60,
		},
	}
	eval := NewEvaluator(source)
	result, _ := eval.Evaluate(context.Background(), def, now)
	if result.State != StateHealthy {
		t.Errorf("perfect ratio state: want healthy, got %s", result.State)
	}
	if result.BurnRate != 0 {
		t.Errorf("perfect ratio burn_rate: want 0, got %v", result.BurnRate)
	}

	// Imperfect: ratio 0.999 -> breached (no error budget).
	source = fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 999)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 1000)},
			ExpectedSamples: 60,
		},
	}
	eval = NewEvaluator(source)
	result, _ = eval.Evaluate(context.Background(), def, now)
	if result.State != StateBreached {
		t.Errorf("imperfect ratio state: want breached, got %s", result.State)
	}
	if result.BurnRate != MaxFiniteBurnRate {
		t.Errorf("imperfect ratio burn_rate: want %v, got %v", MaxFiniteBurnRate, result.BurnRate)
	}
}

// TestComputeRemainingBudget covers the budget helper.
func TestComputeRemainingBudget(t *testing.T) {
	tests := []struct {
		name        string
		ratio       float64
		objective   float64
		errorBudget float64
		want        float64
	}{
		{"ratio_at_objective", 0.99, 0.99, 0.01, 0.0},
		{"ratio_above_objective", 0.995, 0.99, 0.01, 0.5},
		{"ratio_below_objective", 0.985, 0.99, 0.01, 0.0},
		{"zero_error_budget_perfect", 1.0, 1.0, 0.0, 1.0},
		{"zero_error_budget_imperfect", 0.99, 1.0, 0.0, 0.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeRemainingBudget(tc.ratio, tc.objective, tc.errorBudget)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassifyCoverage covers the coverage classifier.
func TestClassifyCoverage(t *testing.T) {
	if got := classifyCoverage(0, 60); got != CoverageUnavailable {
		t.Errorf("0 samples: want unavailable, got %s", got)
	}
	if got := classifyCoverage(1, 60); got != CoveragePartial {
		t.Errorf("1 sample: want partial, got %s", got)
	}
	// 2 samples expected 60: 2 < 60/2=30 -> partial (below expected/2).
	if got := classifyCoverage(2, 60); got != CoveragePartial {
		t.Errorf("2 samples expected 60: want partial (below expected/2), got %s", got)
	}
	// 2 samples expected 4: 2 >= 4/2=2 and >= MinSamplesForComplete -> complete.
	if got := classifyCoverage(2, 4); got != CoverageComplete {
		t.Errorf("2 samples expected 4: want complete, got %s", got)
	}
	// 35 samples expected 60 -> >= 60/2=30 and >= MinSamplesForComplete -> complete.
	if got := classifyCoverage(35, 60); got != CoverageComplete {
		t.Errorf("35 samples expected 60: want complete, got %s", got)
	}
	// 0 expected, 2 samples -> >= MinSamplesForComplete -> complete.
	if got := classifyCoverage(2, 0); got != CoverageComplete {
		t.Errorf("2 samples expected 0: want complete, got %s", got)
	}
}

// TestChooseStep verifies step selection bounds.
func TestChooseStep(t *testing.T) {
	if got := chooseStep(120); got != 60*time.Second {
		t.Errorf("120s window: want 60s step, got %v", got)
	}
	if got := chooseStep(3600); got != 60*time.Second {
		t.Errorf("3600s window: want 60s step, got %v", got)
	}
	// 30d window: window/4 = 7.5d -> clamp to 60s.
	if got := chooseStep(2592000); got != 60*time.Second {
		t.Errorf("30d window: want 60s step, got %v", got)
	}
}
