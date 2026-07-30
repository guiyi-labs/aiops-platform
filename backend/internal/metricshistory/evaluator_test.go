package metricshistory

import (
	"testing"
	"time"
)

func TestEvaluateWindowDetectsSustainedWindowsAcrossFullSeries(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	series := evaluationSeries(start, []int64{90, 10, 80, 70}, QueryCoverage{Collections: 4, Succeeded: 4, Points: 4})
	response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: 60, MinimumPoints: 2})
	if err != nil {
		t.Fatal(err)
	}
	if response.State != EvaluationStateFiring || response.PointsEvaluated != 4 {
		t.Fatalf("response = %#v", response)
	}
	if len(response.SustainedWindows) != 1 {
		t.Fatalf("expected 1 sustained window (single-breach filtered), got %d: %#v", len(response.SustainedWindows), response.SustainedWindows)
	}
	if response.SustainedWindows[0].BreachingPoints != 2 || response.SustainedWindows[0].SpanSeconds != 60 {
		t.Fatalf("sustained window wrong: %#v", response.SustainedWindows[0])
	}
	if response.LatestFiringWindow == nil || response.LatestFiringWindow.BreachingPoints != 2 {
		t.Fatalf("latest firing window wrong: %#v", response.LatestFiringWindow)
	}
	if response.BreachingPoints != 2 || response.ObservedSpanSeconds != 60 {
		t.Fatalf("legacy fields should match latest window: %#v", response)
	}
	if series.Points[0].Value != 90 {
		t.Fatal("evaluator mutated the input series")
	}
}

func TestEvaluateWindowDetectsNonTrailingSustainedWindow(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	series := evaluationSeries(start, []int64{10, 90, 80, 10}, QueryCoverage{Collections: 4, Succeeded: 4, Points: 4})
	response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: 60, MinimumPoints: 2})
	if err != nil {
		t.Fatal(err)
	}
	if response.State != EvaluationStateFiring {
		t.Fatalf("expected firing with non-trailing sustained window, got state=%s", response.State)
	}
	if len(response.SustainedWindows) != 1 || response.SustainedWindows[0].BreachingPoints != 2 {
		t.Fatalf("expected 1 sustained window with 2 breaches, got %#v", response.SustainedWindows)
	}
	if response.LatestFiringWindow == nil || response.LatestFiringWindow.BreachingPoints != 2 {
		t.Fatalf("latest firing window wrong: %#v", response.LatestFiringWindow)
	}
}

func TestEvaluateWindowFiltersShortOrSparseWindows(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	series := evaluationSeries(start, []int64{90, 10, 90, 10}, QueryCoverage{Collections: 4, Succeeded: 4, Points: 4})
	response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: 60, MinimumPoints: 2})
	if err != nil {
		t.Fatal(err)
	}
	if response.State != EvaluationStateNormal {
		t.Fatalf("expected normal (alternating breaches never form sustained window), got state=%s", response.State)
	}
	if len(response.SustainedWindows) != 0 {
		t.Fatalf("expected no sustained windows, got %#v", response.SustainedWindows)
	}
	if response.LatestFiringWindow != nil {
		t.Fatalf("expected nil latest firing window, got %#v", response.LatestFiringWindow)
	}
}

func TestEvaluateWindowSortsPointsAndSupportsLTE(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	series := evaluationSeries(start, []int64{30, 20, 10}, QueryCoverage{Collections: 3, Succeeded: 3, Points: 3})
	series.Points[0], series.Points[2] = series.Points[2], series.Points[0]
	response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorLessThanOrEqual, Threshold: 30, ForSeconds: 120, MinimumPoints: 3})
	if err != nil {
		t.Fatal(err)
	}
	if response.State != EvaluationStateFiring || response.BreachingPoints != 3 || response.ObservedSpanSeconds != 120 {
		t.Fatalf("response = %#v", response)
	}
}

func TestEvaluateWindowReturnsNormalForBrokenOrShortTrailingWindow(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		values     []int64
		forSec     int
		wantState  string
		wantWindow int
	}{
		{name: "non-trailing sustained window still fires", values: []int64{80, 90, 10}, forSec: 60, wantState: EvaluationStateFiring, wantWindow: 1},
		{name: "span too short", values: []int64{10, 80, 90}, forSec: 120, wantState: EvaluationStateNormal, wantWindow: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			series := evaluationSeries(start, test.values, QueryCoverage{Collections: 3, Succeeded: 3, Points: 3})
			response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: test.forSec, MinimumPoints: 2})
			if err != nil {
				t.Fatal(err)
			}
			if response.State != test.wantState {
				t.Fatalf("expected state=%s, got state=%s: %#v", test.wantState, response.State, response)
			}
			if len(response.SustainedWindows) != test.wantWindow {
				t.Fatalf("expected %d sustained windows, got %d: %#v", test.wantWindow, len(response.SustainedWindows), response.SustainedWindows)
			}
		})
	}
}

func TestEvaluateWindowReturnsInsufficientDataForCoverageGapsOrTooFewPoints(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		coverage  QueryCoverage
		values    []int64
		truncated bool
	}{
		{name: "missing", coverage: QueryCoverage{Collections: 3, Points: 2, Missing: 1}, values: []int64{80, 90}},
		{name: "unavailable", coverage: QueryCoverage{Collections: 3, Points: 2, Unavailable: 1}, values: []int64{80, 90}},
		{name: "timed out", coverage: QueryCoverage{Collections: 3, Points: 2, TimedOut: 1}, values: []int64{80, 90}},
		{name: "failed", coverage: QueryCoverage{Collections: 3, Points: 2, Failed: 1}, values: []int64{80, 90}},
		{name: "too few points", coverage: QueryCoverage{Collections: 1, Points: 1}, values: []int64{90}},
		{name: "truncated", coverage: QueryCoverage{Collections: 3, Points: 3}, values: []int64{80, 90}, truncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			series := evaluationSeries(start, test.values, test.coverage)
			series.Truncated = test.truncated
			response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: 60, MinimumPoints: 2})
			if err != nil {
				t.Fatal(err)
			}
			if response.State != EvaluationStateInsufficientData {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func TestEvaluateWindowValidatesRuleAndDefaultsMinimumPoints(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	series := evaluationSeries(start, []int64{80, 90}, QueryCoverage{Collections: 2, Succeeded: 2, Points: 2})
	response, err := EvaluateWindow(series, EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 50, ForSeconds: 60})
	if err != nil || response.MinimumPoints != 2 || response.State != EvaluationStateFiring {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	invalid := []EvaluationRule{
		{Operator: "gt", Threshold: 1, ForSeconds: 60, MinimumPoints: 2},
		{Operator: OperatorGreaterThanOrEqual, Threshold: -1, ForSeconds: 60, MinimumPoints: 2},
		{Operator: OperatorGreaterThanOrEqual, Threshold: 1, ForSeconds: 59, MinimumPoints: 2},
		{Operator: OperatorGreaterThanOrEqual, Threshold: 1, ForSeconds: 86401, MinimumPoints: 2},
		{Operator: OperatorGreaterThanOrEqual, Threshold: 1, ForSeconds: 60, MinimumPoints: 1},
		{Operator: OperatorGreaterThanOrEqual, Threshold: 1, ForSeconds: 60, MinimumPoints: 1441},
	}
	for _, rule := range invalid {
		if _, err := EvaluateWindow(series, rule); err != ErrInvalidEvaluation {
			t.Fatalf("rule=%#v err=%v", rule, err)
		}
	}
}

func evaluationSeries(start time.Time, values []int64, coverage QueryCoverage) SeriesResponse {
	points := make([]Point, 0, len(values))
	for index, value := range values {
		at := start.Add(time.Duration(index) * time.Minute)
		points = append(points, Point{Value: value, SourceTimestamp: at, CollectedAt: at, WindowMilliseconds: 15000})
	}
	return SeriesResponse{
		Series: Series{ClusterID: 7, ResourceKind: ResourceNode, ResourceName: "worker-a", MetricName: MetricCPU, Unit: UnitNanocores},
		From:   start, To: start.Add(time.Hour), Points: points, Coverage: coverage,
	}
}
