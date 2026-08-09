package metricshistory

import (
	"testing"
	"time"
)

// BenchmarkEvaluateWindow measures the pure metrics-window evaluator that runs
// on every trend query. Fixtures: a 300-point series (the default window).
func BenchmarkEvaluateWindow(b *testing.B) {
	base := time.Now().Add(-6 * time.Hour)
	points := make([]Point, 300)
	for i := range points {
		points[i] = Point{
			Value:              int64(i) * 100,
			SourceTimestamp:    base.Add(time.Duration(i) * time.Minute),
			WindowMilliseconds: 60_000,
			CollectedAt:        base.Add(time.Duration(i) * time.Minute).Add(2 * time.Second),
		}
	}
	series := SeriesResponse{
		Series: Series{
			ClusterID:    1,
			ResourceKind: "node",
			ResourceName: "node-1",
			MetricName:   "node_cpu_seconds_total",
			Unit:         "1",
		},
		From:     base,
		To:       base.Add(5 * time.Hour),
		Points:   points,
		Coverage: QueryCoverage{Collections: 300, Succeeded: 300, Points: 300},
		Limits:   QueryLimits{MaxWindowSeconds: 6 * 3600, MaxPoints: 360},
	}
	rule := EvaluationRule{Operator: OperatorGreaterThanOrEqual, Threshold: 900, ForSeconds: 600, MinimumPoints: 3}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := EvaluateWindow(series, rule); err != nil {
			b.Fatalf("EvaluateWindow: %v", err)
		}
	}
}
