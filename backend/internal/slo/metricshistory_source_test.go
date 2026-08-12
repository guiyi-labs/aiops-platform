package slo

import (
	"context"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
)

// fakeMetricsRepo implements metricshistory.Repository over in-memory points.
type fakeMetricsRepo struct {
	points map[string][]metricshistory.Point // keyed by metric_name
}

func (f *fakeMetricsRepo) SaveCollection(context.Context, metricshistory.Collection) (int64, error) {
	return 1, nil
}
func (f *fakeMetricsRepo) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (f *fakeMetricsRepo) QuerySeries(_ context.Context, q metricshistory.SeriesQuery) (metricshistory.RepositorySeriesResult, error) {
	pts := append([]metricshistory.Point(nil), f.points[q.MetricName]...)
	return metricshistory.RepositorySeriesResult{Points: pts, Total: len(pts)}, nil
}

func readinessDef() *Definition {
	return &Definition{
		ClusterID: 1,
		Service:   ServiceRef{Kind: "Deployment", Namespace: "default", Name: "web", UID: "dep-7"},
		Template:  TemplateWorkloadReadiness, TemplateVersion: "1.0",
		Objective: 0.99, RollingWindowSeconds: 3600,
		MissingDataPolicy: MissingDataUnavailable,
		FastBurnRate:      14.4, FastBurnWindowSeconds: 3600,
		SlowBurnRate: 1.0, SlowBurnWindowSeconds: 21600,
		Enabled: true, Version: 1,
	}
}

func readinessPoint(t time.Time, value int64) metricshistory.Point {
	return metricshistory.Point{Value: value, SourceTimestamp: t}
}

func TestMetricshistorySource_QuerySLIWorkloadReadiness(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	svc, err := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{
		metricshistory.MetricReadinessReady: {
			readinessPoint(base, 3), readinessPoint(base.Add(time.Minute), 2), readinessPoint(base.Add(2*time.Minute), 3),
		},
		metricshistory.MetricReadinessTotal: {
			readinessPoint(base, 3), readinessPoint(base.Add(time.Minute), 3), readinessPoint(base.Add(2*time.Minute), 3),
		},
	}})
	if err != nil {
		t.Fatalf("NewService err=%v", err)
	}
	src := NewMetricshistorySource(svc)
	series, err := src.QuerySLI(context.Background(), readinessDef(), base, base.Add(5*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("QuerySLI err=%v", err)
	}
	if len(series.Good) != 3 || len(series.Total) != 3 {
		t.Fatalf("len(Good)=%d len(Total)=%d, want 3 each", len(series.Good), len(series.Total))
	}
	wantGood := []float64{3, 5, 8}
	wantTotal := []float64{3, 6, 9}
	for i := range wantGood {
		if series.Good[i].Value != wantGood[i] || series.Total[i].Value != wantTotal[i] {
			t.Errorf("sample %d: good=%v total=%v, want %v/%v", i, series.Good[i].Value, series.Total[i].Value, wantGood[i], wantTotal[i])
		}
		if series.Good[i].Timestamp.After(series.Total[i].Timestamp) {
			t.Errorf("sample %d timestamps out of order", i)
		}
	}
	if series.Source != "metricshistory" {
		t.Errorf("source = %q", series.Source)
	}
	if series.ExpectedSamples != 0 {
		t.Errorf("expected samples = %d, want 0", series.ExpectedSamples)
	}
}

func TestMetricshistorySource_DropsUnpairedObservations(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	svc, _ := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{
		metricshistory.MetricReadinessReady: {readinessPoint(base, 3), readinessPoint(base.Add(2*time.Minute), 3)},
		metricshistory.MetricReadinessTotal: {readinessPoint(base, 3), readinessPoint(base.Add(time.Minute), 3), readinessPoint(base.Add(2*time.Minute), 3)},
	}})
	series, err := NewMetricshistorySource(svc).QuerySLI(context.Background(), readinessDef(), base, base.Add(5*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("QuerySLI err=%v", err)
	}
	// The 10:01 run lacks readiness_ready; it must be dropped, not counted as unready.
	if len(series.Good) != 2 || series.Good[0].Value != 3 || series.Good[1].Value != 6 {
		t.Fatalf("unpaired observation handling wrong: %+v", series.Good)
	}
}

func TestMetricshistorySource_RequestTemplateReturnsNoData(t *testing.T) {
	svc, _ := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{
		metricshistory.MetricReadinessReady: {readinessPoint(time.Now(), 3)},
	}})
	def := readinessDef()
	def.Template = TemplateRequestSuccessRatio
	series, err := NewMetricshistorySource(svc).QuerySLI(context.Background(), def, time.Now().Add(-time.Minute), time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("QuerySLI err=%v", err)
	}
	if len(series.Good) != 0 || len(series.Total) != 0 {
		t.Fatalf("request template must yield no data, got %+v", series)
	}
	if series.Source != "metricshistory" {
		t.Errorf("source = %q", series.Source)
	}
}

func TestEvaluator_MetricshistorySteadyHealthy(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	svc, _ := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{
		metricshistory.MetricReadinessReady: {
			readinessPoint(base.Add(-3*time.Minute), 3), readinessPoint(base.Add(-2*time.Minute), 3), readinessPoint(base.Add(-time.Minute), 3),
		},
		metricshistory.MetricReadinessTotal: {
			readinessPoint(base.Add(-3*time.Minute), 3), readinessPoint(base.Add(-2*time.Minute), 3), readinessPoint(base.Add(-time.Minute), 3),
		},
	}})
	eval, err := NewEvaluator(NewMetricshistorySource(svc)).Evaluate(context.Background(), readinessDef(), base)
	if err != nil {
		t.Fatalf("Evaluate err=%v", err)
	}
	if eval.State != StateHealthy {
		t.Errorf("state = %s, want healthy", eval.State)
	}
	if eval.Ratio != 1.0 {
		t.Errorf("ratio = %v, want 1.0", eval.Ratio)
	}
	if eval.Coverage != CoverageComplete {
		t.Errorf("coverage = %s, want complete", eval.Coverage)
	}
}

func TestEvaluator_MetricshistoryRolloutBreachesBudget(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	svc, _ := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{
		metricshistory.MetricReadinessReady: {
			readinessPoint(base.Add(-3*time.Minute), 3), readinessPoint(base.Add(-2*time.Minute), 1), readinessPoint(base.Add(-time.Minute), 1),
		},
		metricshistory.MetricReadinessTotal: {
			readinessPoint(base.Add(-3*time.Minute), 3), readinessPoint(base.Add(-2*time.Minute), 3), readinessPoint(base.Add(-time.Minute), 3),
		},
	}})
	eval, err := NewEvaluator(NewMetricshistorySource(svc)).Evaluate(context.Background(), readinessDef(), base)
	if err != nil {
		t.Fatalf("Evaluate err=%v", err)
	}
	if eval.State != StateBreached {
		t.Errorf("state = %s, want breached (readiness dropped 3->1)", eval.State)
	}
	if eval.BurnRate < readinessDef().FastBurnRate {
		t.Errorf("burn rate = %v, want >= fast burn %v", eval.BurnRate, readinessDef().FastBurnRate)
	}
}

func TestEvaluator_MetricshistoryNoDataHonorsMissingPolicy(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	svc, _ := metricshistory.NewService(metricshistory.Config{Retention: time.Hour}, &fakeMetricsRepo{points: map[string][]metricshistory.Point{}})
	def := readinessDef()
	eval, err := NewEvaluator(NewMetricshistorySource(svc)).Evaluate(context.Background(), def, base)
	if err != nil {
		t.Fatalf("Evaluate err=%v", err)
	}
	if eval.State != StateUnavailable {
		t.Errorf("state = %s, want unavailable under fail-closed missing-data policy", eval.State)
	}
}
