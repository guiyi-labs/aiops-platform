package slo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// Evaluator is the deterministic SLI evaluator. Given a Definition and a
// MetricsSource, it computes good/total events over the rolling window,
// derives the error budget, remaining budget and burn rate, and classifies
// the SLO state (healthy / burning_slow / burning_fast / breached /
// unavailable).
//
// The evaluator is pure with respect to inputs: the same Definition and the
// same MetricsSource output must yield the same Evaluation. Counter resets,
// sparse data and clock boundaries are handled deterministically:
//
//   - Counter resets are detected as monotonicity violations and the delta is
//     taken as the post-reset value (counter is treated as having reset to 0).
//   - Sparse data produces CoveragePartial when at least one sample is present
//     but the sample count is below the expected minimum; complete absence
//     yields CoverageUnavailable and StateUnavailable (fail-closed).
//   - Clock boundaries use inclusive window_start and exclusive window_end.
//     Samples whose timestamp is exactly window_start are counted; samples
//     whose timestamp is >= window_end are excluded.
//
// The evaluator never fabricates healthy state. Missing data is reported as
// StateUnavailable unless the template allows fail_open (workload_readiness)
// AND the operator has explicitly selected MissingDataFailOpen — in which
// case an empty window yields StateHealthy with CoverageUnavailable and a
// zero burn rate. This is the only fail-open path and it is auditable.
type Evaluator struct {
	source MetricsSource
}

// MetricsSource is the bounded read interface the evaluator consumes. It is
// a subset of capability.MetricsProvider shaped for SLO evaluation — the
// concrete adapter lives in the slo package's wireup. Implementations must
// return series in chronological order (oldest first) within each call.
//
// Good events and total events are returned as monotonically non-decreasing
// counters (cumulative since process/pod start). The evaluator converts them
// to deltas over the rolling window. A MetricsSource must never accept
// PromQL or arbitrary query languages — the Template field is the only query
// selector.
type MetricsSource interface {
	// QuerySLI returns the raw counter series for good and total events
	// over the [start, end) window, at the requested step. Series are
	// returned oldest-first. An empty result (no samples) is a valid
	// "no data" signal — the caller must NOT fabricate samples.
	QuerySLI(ctx context.Context, def *Definition, start, end time.Time, step time.Duration) (SLISeries, error)
}

// SLISeries is the raw counter series for an SLI evaluation.
type SLISeries struct {
	// Good is the cumulative "good" counter series (e.g. successful
	// requests, requests under latency threshold, ready pods).
	Good []Sample
	// Total is the cumulative "total" counter series (e.g. all requests,
	// all desired pods).
	Total []Sample
	// ExpectedSamples is the number of samples the source expected to
	// return for the window at the requested step. Used to compute
	// CoveragePartial when the source returned fewer samples than expected.
	ExpectedSamples int
	// Source is the provider identifier (sanitized). Used for evidence
	// traceability; never contains endpoints or credentials.
	Source string
}

// Sample is a single timestamped counter value.
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// ErrEvaluationInvalidInput is returned when the evaluator rejects an input
// before contacting the source. The message is safe to surface.
var ErrEvaluationInvalidInput = errors.New("slo evaluation input invalid")

// ErrEvaluationSourceUnavailable is returned when the source itself is
// unavailable and the policy is fail-closed. The Evaluation is still
// produced (with StateUnavailable); this error is informational for the
// caller and for audit.
var ErrEvaluationSourceUnavailable = errors.New("slo source unavailable")

// MinSamplesForComplete is the minimum sample count for CoverageComplete.
// Below this the evaluation is CoveragePartial (if any samples) or
// CoverageUnavailable (if none).
const MinSamplesForComplete = 2

// NewEvaluator constructs an Evaluator backed by the given MetricsSource.
// Passing nil is allowed and produces an evaluator that always reports
// StateUnavailable — used in tests and in deployments without a metrics
// provider.
func NewEvaluator(source MetricsSource) *Evaluator {
	return &Evaluator{source: source}
}

// Evaluate runs a single deterministic evaluation of def over the rolling
// window ending at now. The returned Evaluation is always non-nil; callers
// must persist it via Repository.InsertEvaluation if they want it stored.
//
// The Evaluation.WindowEnd is truncated to the second to keep evaluations
// deterministic across clock skew within the same second.
func (e *Evaluator) Evaluate(ctx context.Context, def *Definition, now time.Time) (Evaluation, error) {
	if err := ValidateDefinition(def); err != nil {
		return Evaluation{}, fmt.Errorf("%w: %v", ErrEvaluationInvalidInput, err)
	}
	if !def.Enabled {
		return Evaluation{}, fmt.Errorf("%w: definition is disabled", ErrEvaluationInvalidInput)
	}
	windowEnd := now.Truncate(time.Second)
	windowStart := windowEnd.Add(-time.Duration(def.RollingWindowSeconds) * time.Second)
	eval := Evaluation{
		SLOID:       def.ID,
		Version:     def.Version,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		TargetRatio: def.Objective,
		ErrorBudget: 1.0 - def.Objective,
		EvaluatedAt: now,
	}

	if e.source == nil {
		// No source configured — fail-closed for every template unless
		// the operator explicitly opted into fail_open for a template
		// that allows it (workload_readiness only).
		return e.failClosedOrOpen(def, eval), ErrEvaluationSourceUnavailable
	}

	// Step is bounded to one minute minimum so long windows do not
	// explode sample counts, and to the rolling window / 4 maximum so
	// short windows still produce enough samples for burn detection.
	step := chooseStep(def.RollingWindowSeconds)

	series, err := e.source.QuerySLI(ctx, def, windowStart, windowEnd, step)
	if err != nil {
		// A hard source error is treated as unavailable. The evaluator
		// never returns a fabricated healthy state on error.
		return e.unavailable(def, eval), err
	}

	good, goodCoverage := deltaCounter(series.Good, series.ExpectedSamples, windowStart, windowEnd)
	total, totalCoverage := deltaCounter(series.Total, series.ExpectedSamples, windowStart, windowEnd)

	coverage := combineCoverage(goodCoverage, totalCoverage)

	if coverage == CoverageUnavailable {
		// No usable data — honor the missing-data policy.
		return e.applyMissingDataPolicy(def, eval), nil
	}

	eval.GoodEvents = good
	eval.TotalEvents = total
	if total > 0 {
		eval.Ratio = clampRatio(good / total)
	} else if def.MissingDataPolicy == MissingDataFailOpen {
		// Zero total events with fail_open: treat as fully healthy
		// (only allowed for workload_readiness). This is auditable
		// via the Coverage field, which remains Partial/Unavailable.
		eval.Ratio = 1.0
	} else {
		// Zero total events with fail-closed: cannot compute a ratio
		// honestly. State is unavailable, but we preserve the sample
		// coverage so callers can distinguish "no samples at all"
		// (CoverageUnavailable) from "had samples but no deltas"
		// (CoveragePartial).
		eval.State = StateUnavailable
		eval.Coverage = coverage
		return eval, nil
	}

	eval.RemainingBudget = computeRemainingBudget(eval.Ratio, eval.TargetRatio, eval.ErrorBudget)
	eval.BurnRate = computeBurnRate(eval.Ratio, eval.TargetRatio, eval.ErrorBudget)
	eval.State = classifyState(def, eval.Ratio, eval.BurnRate, eval.ErrorBudget)
	eval.Coverage = coverage
	return eval, nil
}

// failClosedOrOpen applies the missing-data policy when the source is nil.
func (e *Evaluator) failClosedOrOpen(def *Definition, eval Evaluation) Evaluation {
	return e.applyMissingDataPolicy(def, eval)
}

// applyMissingDataPolicy finalizes an Evaluation under missing data.
func (e *Evaluator) applyMissingDataPolicy(def *Definition, eval Evaluation) Evaluation {
	eval.Coverage = CoverageUnavailable
	if def.MissingDataPolicy == MissingDataFailOpen {
		desc, ok := LookupTemplate(def.Template)
		if ok && desc.AllowsFailOpen {
			eval.State = StateHealthy
			eval.Ratio = 1.0
			eval.BurnRate = 0
			eval.RemainingBudget = eval.ErrorBudget
			return eval
		}
	}
	eval.State = StateUnavailable
	return eval
}

// unavailable marks an Evaluation as unavailable due to a source error.
func (e *Evaluator) unavailable(_ *Definition, eval Evaluation) Evaluation {
	eval.State = StateUnavailable
	eval.Coverage = CoverageUnavailable
	return eval
}

// chooseStep selects the evaluation step for a rolling window.
// - Minimum 60s so a 30d window yields <= 43200 samples.
// - Maximum window/4 so a short window still yields >= 4 samples.
// Falls back to 60s when window < 240s.
func chooseStep(rollingWindowSeconds int) time.Duration {
	minStep := 60 * time.Second
	maxStep := time.Duration(rollingWindowSeconds/4) * time.Second
	if maxStep < minStep {
		return minStep
	}
	return minStep
}

// deltaCounter converts a cumulative counter series into a delta over the
// [start, end) window. It handles counter resets (non-monotonic samples) by
// treating a reset as "counter went to 0 and started again", so the delta
// across a reset is the post-reset value.
//
// Samples outside [start, end) are excluded. The first sample inside the
// window establishes the baseline; subsequent samples contribute
// (value - previous_value) when monotonic, or value when a reset is
// detected. The result is the sum of all such deltas.
//
// Coverage is computed from the sample count vs expected:
//   - 0 samples                -> CoverageUnavailable
//   - 1 sample or < expected/2 -> CoveragePartial (if expected > 0) or Complete (if expected == 0)
//   - >= MinSamplesForComplete and >= expected/2 -> CoverageComplete
//
// When expected == 0 the source did not advertise an expectation; we fall
// back to MinSamplesForComplete as the floor for Complete.
func deltaCounter(samples []Sample, expected int, start, end time.Time) (float64, EvaluationCoverage) {
	if len(samples) == 0 {
		return 0, CoverageUnavailable
	}
	// Filter to [start, end).
	in := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !s.Timestamp.Before(start) && s.Timestamp.Before(end) {
			in = append(in, s)
		}
	}
	if len(in) == 0 {
		return 0, CoverageUnavailable
	}
	var delta float64
	prev := in[0].Value
	// The first sample's value is the baseline; we do not count its
	// absolute value, only deltas from it onward.
	for i := 1; i < len(in); i++ {
		v := in[i].Value
		if v >= prev {
			delta += v - prev
		} else {
			// Counter reset: the counter wrapped or restarted.
			// Treat the post-reset value as the delta for this step.
			delta += v
		}
		prev = v
	}
	return delta, classifyCoverage(len(in), expected)
}

// classifyCoverage maps (sampleCount, expected) to a Coverage value.
func classifyCoverage(count, expected int) EvaluationCoverage {
	if count == 0 {
		return CoverageUnavailable
	}
	if count < MinSamplesForComplete {
		return CoveragePartial
	}
	if expected > 0 && count < expected/2 {
		return CoveragePartial
	}
	return CoverageComplete
}

// combineCoverage returns the worse of two coverages (Unavailable > Partial > Complete).
func combineCoverage(a, b EvaluationCoverage) EvaluationCoverage {
	if a == CoverageUnavailable || b == CoverageUnavailable {
		return CoverageUnavailable
	}
	if a == CoveragePartial || b == CoveragePartial {
		return CoveragePartial
	}
	return CoverageComplete
}

// clampRatio clamps v into [0, 1].
func clampRatio(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	if math.IsNaN(v) {
		return 0
	}
	return v
}

// computeRemainingBudget returns the fraction of error budget remaining.
// remaining = (ratio - objective) / (1 - objective)
// When objective == 1.0 the error budget is 0; remaining is 1.0 when
// ratio == 1.0 (no errors) and 0.0 otherwise.
func computeRemainingBudget(ratio, objective, errorBudget float64) float64 {
	if errorBudget <= 0 {
		if ratio >= 1.0 {
			return 1.0
		}
		return 0.0
	}
	rem := (ratio - objective) / errorBudget
	if rem < 0 {
		return 0
	}
	if rem > 1 {
		return 1
	}
	return rem
}

// MaxFiniteBurnRate is the JSON- and database-safe sentinel used when an SLO
// has no error budget (objective=1) and observes any error. encoding/json
// rejects +/-Inf, so the API must never expose a non-finite float.
const MaxFiniteBurnRate = 1e9

// computeBurnRate returns (1 - ratio) / error_budget. When error_budget is
// 0 the burn rate is the finite sentinel if ratio < 1, else 0.
func computeBurnRate(ratio, objective, errorBudget float64) float64 {
	if errorBudget <= 0 {
		if ratio >= 1.0 {
			return 0
		}
		return MaxFiniteBurnRate
	}
	return (1.0 - ratio) / errorBudget
}

// classifyState derives the EvaluationState from ratio, burn rate and the
// definition's fast/slow burn thresholds.
//
// Precedence (highest severity first):
//  1. StateBreached: ratio < objective for the full rolling window
//     (error budget exhausted).
//  2. StateBurningFast: burn rate >= fast_burn_rate over the fast window.
//  3. StateBurningSlow: burn rate >= slow_burn_rate over the slow window.
//  4. StateHealthy: ratio >= objective.
//
// Note: the burn thresholds are compared against the rolling-window burn
// rate computed by the evaluator. A multi-window implementation (separate
// fast/slow evaluations) is a future enhancement; for V1 the single-window
// burn rate is compared against both thresholds. This is conservative: a
// single-window burn rate that exceeds the fast threshold is unambiguously
// a fast burn.
func classifyState(def *Definition, ratio, burnRate, errorBudget float64) EvaluationState {
	if errorBudget <= 0 {
		// Zero error budget (objective == 1.0): any deviation is a breach.
		if ratio >= def.Objective {
			return StateHealthy
		}
		return StateBreached
	}
	if ratio < def.Objective {
		return StateBreached
	}
	// ratio >= objective but we may still be burning budget too fast.
	// Compare burn rate against thresholds. Use IsInf to handle the
	// objective == 1.0 case where burn rate can be +Inf.
	if math.IsInf(burnRate, 1) || burnRate >= def.FastBurnRate {
		return StateBurningFast
	}
	if burnRate >= def.SlowBurnRate {
		return StateBurningSlow
	}
	return StateHealthy
}
