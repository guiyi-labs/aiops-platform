package signal

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
)

type fakeDiagnosisListReader struct {
	records []diagnosis.Record
	err     error
}

func (f *fakeDiagnosisListReader) List(_ context.Context, filter diagnosis.ListFilter) ([]diagnosis.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []diagnosis.Record
	for _, r := range f.records {
		if filter.UpdatedAfter == nil || r.UpdatedAt.After(*filter.UpdatedAfter) {
			out = append(out, r)
		}
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

type drainCaptureRepo struct {
	occurrences []Occurrence
	err         error
}

func (r *drainCaptureRepo) Upsert(_ context.Context, occ *Occurrence) error {
	if r.err != nil {
		return r.err
	}
	r.occurrences = append(r.occurrences, *occ)
	return nil
}
func (r *drainCaptureRepo) Get(context.Context, int64) (Occurrence, error) {
	return Occurrence{}, ErrSignalNotFound
}
func (r *drainCaptureRepo) List(context.Context, ListFilter) ([]Occurrence, int64, error) {
	return nil, 0, nil
}
func (r *drainCaptureRepo) CountBySignal(context.Context, *int64, string, time.Time, int) ([]OverviewSignal, error) {
	return nil, nil
}
func (r *drainCaptureRepo) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func drainRecord(id int64, ruleID string, updatedAt time.Time) diagnosis.Record {
	return diagnosis.Record{
		ID:         id,
		ClusterID:  7,
		RuleID:     ruleID,
		Severity:   "critical",
		Status:     "open",
		Resource:   diagnosis.ResourceRef{Kind: "Node", Namespace: "", Name: "demo-node", UID: "uid-node"},
		ObservedAt: updatedAt,
		UpdatedAt:  updatedAt,
	}
}

func TestDiagnosisDrainIngestsMappedRecords(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	reader := &fakeDiagnosisListReader{records: []diagnosis.Record{
		drainRecord(1, diagnosis.RuleNodeNotReady, now),
		drainRecord(2, "ai.unmapped.v1", now.Add(time.Second)),
	}}
	repo := &drainCaptureRepo{}
	svc := NewService(ServiceOptions{Repository: repo, Now: func() time.Time { return now }})
	d := NewDiagnosisDrain(DrainConfig{Interval: time.Second, PageSize: 10}, reader, svc, nil)
	d.watermark = now.Add(-time.Minute)
	d.drainOnce(context.Background())
	if len(repo.occurrences) != 1 {
		t.Fatalf("ingested = %d, want 1 (unmapped rule skipped)", len(repo.occurrences))
	}
	got := repo.occurrences[0]
	if got.SignalID != "diag.node.not_ready.v1" || got.ClusterID != 7 {
		t.Errorf("occurrence wrong: %+v", got)
	}
	if got.State != StateActive {
		t.Errorf("state = %q, want active", got.State)
	}
	if d.watermark.Before(now) {
		t.Errorf("watermark = %v, want advanced to %v", d.watermark, now)
	}
}

func TestDiagnosisDrainAdvancesWatermark(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	record := drainRecord(1, diagnosis.RuleNodeNotReady, now)
	reader := &fakeDiagnosisListReader{records: []diagnosis.Record{record}}
	repo := &drainCaptureRepo{}
	svc := NewService(ServiceOptions{Repository: repo, Now: func() time.Time { return now }})
	d := NewDiagnosisDrain(DrainConfig{Interval: time.Second, PageSize: 10}, reader, svc, nil)
	d.watermark = now.Add(-time.Minute)
	d.drainOnce(context.Background())
	if len(repo.occurrences) != 1 {
		t.Fatalf("first pass ingested = %d, want 1", len(repo.occurrences))
	}
	// Second pass: strict-updated_after cursor excludes the same record.
	d.drainOnce(context.Background())
	if len(repo.occurrences) != 1 {
		t.Fatalf("second pass re-ingested, total = %d, want 1", len(repo.occurrences))
	}
}

func TestDiagnosisDrainToleratesListAndIngestFailures(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	// List failure: logged, no crash, watermark unchanged.
	svc := NewService(ServiceOptions{Repository: &drainCaptureRepo{}, Now: func() time.Time { return now }})
	d := NewDiagnosisDrain(DrainConfig{Interval: time.Second, PageSize: 10}, &fakeDiagnosisListReader{err: errors.New("list boom")}, svc, nil)
	d.watermark = now.Add(-time.Minute)
	d.drainOnce(context.Background())
	if !d.watermark.Equal(now.Add(-time.Minute)) {
		t.Errorf("watermark moved on list failure: %v", d.watermark)
	}
	// Ingest failure: logged, other records still processed, watermark advances.
	reader := &fakeDiagnosisListReader{records: []diagnosis.Record{
		drainRecord(1, diagnosis.RuleNodeNotReady, now.Add(-time.Minute)),
		drainRecord(2, diagnosis.RulePodOOMKilled, now),
	}}
	repo := &drainCaptureRepo{err: errors.New("ingest boom")}
	svc = NewService(ServiceOptions{Repository: repo, Now: func() time.Time { return now }})
	d = NewDiagnosisDrain(DrainConfig{Interval: time.Second, PageSize: 10}, reader, svc, nil)
	d.watermark = now.Add(-2 * time.Minute)
	d.drainOnce(context.Background())
	if !d.watermark.Equal(now) {
		t.Errorf("watermark = %v, want advanced despite ingest failures", d.watermark)
	}
}

func TestDiagnosisDrainRunLifecycle(t *testing.T) {
	// Run() sets its watermark to the current time on start; the record must
	// be strictly newer so the immediate pass picks it up.
	recordTime := time.Now().Add(2 * time.Second)
	reader := &fakeDiagnosisListReader{records: []diagnosis.Record{
		drainRecord(1, diagnosis.RuleNodeNotReady, recordTime),
	}}
	repo := &drainCaptureRepo{}
	svc := NewService(ServiceOptions{Repository: repo})
	d := NewDiagnosisDrain(DrainConfig{Interval: time.Hour, PageSize: 10}, reader, svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
	if len(repo.occurrences) != 1 {
		t.Fatalf("Run ingested %d occurrences, want 1 (immediate pass)", len(repo.occurrences))
	}
}

func TestDiagnosisDrainDefaultsForZeroConfig(t *testing.T) {
	d := NewDiagnosisDrain(DrainConfig{}, &fakeDiagnosisListReader{}, nil, nil)
	if d.config.Interval != DefaultDrainInterval || d.config.PageSize != DefaultDrainPageSize {
		t.Fatalf("defaults not applied: %+v", d.config)
	}
	if d.logger == nil {
		t.Fatal("nil logger must default to a Nop logger")
	}
}
