package signal

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComputeFingerprint_StableAcrossRedelivery(t *testing.T) {
	req := IngestRequest{
		SignalID:    "diag.pod.pending.v1",
		ClusterID:   1,
		Resource:    ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx", UID: "uid-123"},
		WindowStart: ptrTime(time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)),
		WindowEnd:   ptrTime(time.Date(2026, 7, 31, 10, 5, 0, 0, time.UTC)),
	}
	fp1 := ComputeFingerprint(req)
	// Simulate a second delivery at a different ObservedAt — the fingerprint
	// must not change because ObservedAt is excluded by contract.
	req.ObservedAt = time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC)
	fp2 := ComputeFingerprint(req)
	if fp1 != fp2 {
		t.Fatalf("fingerprint changed across redelivery: %s != %s", fp1, fp2)
	}
	if fp1 == "" {
		t.Fatal("fingerprint must not be empty")
	}
}

func TestComputeFingerprint_NameOnlyFallback(t *testing.T) {
	req := IngestRequest{
		SignalID:  "alert.firing.v1",
		ClusterID: 1,
		Resource:  ResourceCitation{Kind: "Node", Name: "node-1"},
	}
	fp := ComputeFingerprint(req)
	if fp == "" {
		t.Fatal("name-only fingerprint must still be non-empty")
	}
}

func TestBuildOccurrence_FailClosedForUnregisteredSignal(t *testing.T) {
	req := IngestRequest{
		SignalID:   "bogus.signal.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Name: "x"},
		ObservedAt: time.Now(),
	}
	_, err := BuildOccurrence(req, time.Now())
	if err == nil {
		t.Fatal("expected error for unregistered signal")
	}
}

func TestBuildOccurrence_RequiresResourceKindAndName(t *testing.T) {
	req := IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{},
		ObservedAt: time.Now(),
	}
	_, err := BuildOccurrence(req, time.Now())
	if err == nil {
		t.Fatal("expected error for missing resource kind/name")
	}
}

func TestBuildOccurrence_MarksIncompleteWhenUIDMissing(t *testing.T) {
	req := IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx"},
		ObservedAt: time.Now(),
	}
	occ, err := BuildOccurrence(req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !occ.Resource.Incomplete {
		t.Fatal("expected Incomplete=true when UID is empty")
	}
}

func TestBuildOccurrence_SetsExpiryFromRetention(t *testing.T) {
	req := IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Name: "nginx", UID: "uid"},
		ObservedAt: time.Now(),
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	occ, err := BuildOccurrence(req, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if occ.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	expected := now.Add(DefaultRetention)
	if !occ.ExpiresAt.Equal(expected) {
		t.Fatalf("expected expires_at %v, got %v", expected, *occ.ExpiresAt)
	}
}

func TestMapSeverity_FallbackWhenNotMapped(t *testing.T) {
	desc, ok := Lookup("diag.pod.pending.v1")
	if !ok {
		t.Fatal("descriptor not found")
	}
	if got := MapSeverity(desc, "unknown"); got != SeverityWarning {
		t.Fatalf("expected fallback warning, got %s", got)
	}
}

func TestLookup_AllDescriptorsHaveConsistentSchemaVersion(t *testing.T) {
	for _, d := range All() {
		if d.SchemaVersion != SchemaVersionV1 {
			t.Errorf("descriptor %s has schema_version %q, want %q", d.Code, d.SchemaVersion, SchemaVersionV1)
		}
		if d.Retention <= 0 {
			t.Errorf("descriptor %s has non-positive retention", d.Code)
		}
		if d.Domain == "" {
			t.Errorf("descriptor %s has empty domain", d.Code)
		}
	}
}

// --- Service tests with mock repository ---

func TestService_Ingest_Dedup(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(ServiceOptions{Repository: repo, Now: fixedNow})
	req := IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx", UID: "uid-1"},
		ObservedAt: fixedNow(),
	}
	if _, err := svc.Ingest(context.Background(), req); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), req); err != nil {
		t.Fatalf("second ingest (dedup): %v", err)
	}
	if repo.upsertCount != 2 {
		t.Fatalf("expected 2 upserts, got %d", repo.upsertCount)
	}
}

func TestService_Ingest_FailClosedForUnregistered(t *testing.T) {
	svc := NewService(ServiceOptions{Repository: &mockRepo{}, Now: fixedNow})
	req := IngestRequest{
		SignalID:   "bogus.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Name: "x"},
		ObservedAt: fixedNow(),
	}
	_, err := svc.Ingest(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unregistered signal")
	}
}

func TestService_List_ClampsLimit(t *testing.T) {
	svc := NewService(ServiceOptions{Repository: &mockRepo{}, Now: fixedNow, ListLimit: 50})
	_, _, err := svc.List(context.Background(), ListFilter{Limit: 9999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Overview_PartialFlag(t *testing.T) {
	svc := NewService(ServiceOptions{Repository: &mockRepo{}, Now: fixedNow})
	sr := &mockSourceReader{completeness: map[Producer]Coverage{
		ProducerDiagnosis: CoverageComplete,
		ProducerAlert:     CoverageUnavailable,
	}}
	ov, err := svc.Overview(context.Background(), nil, "", sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ov.Partial {
		t.Fatal("expected Partial=true when a producer is unavailable")
	}
}

func TestService_CleanupRetention(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(ServiceOptions{Repository: repo, Now: fixedNow})
	n, err := svc.CleanupRetention(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deletions from mock, got %d", n)
	}
	if repo.deleteExpiredCount != 1 {
		t.Fatalf("expected DeleteExpired called once, got %d", repo.deleteExpiredCount)
	}
}

// --- helpers and mocks ---

func ptrTime(t time.Time) *time.Time { return &t }

func fixedNow() time.Time {
	return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
}

type mockRepo struct {
	upsertCount        int
	deleteExpiredCount int
}

func (m *mockRepo) Upsert(_ context.Context, _ *Occurrence) error {
	m.upsertCount++
	return nil
}
func (m *mockRepo) Get(_ context.Context, _ int64) (Occurrence, error) {
	return Occurrence{}, ErrSignalNotFound
}
func (m *mockRepo) List(_ context.Context, _ ListFilter) ([]Occurrence, int64, error) {
	return nil, 0, nil
}
func (m *mockRepo) CountBySignal(_ context.Context, _ *int64, _ string, _ time.Time, _ int) ([]OverviewSignal, error) {
	return nil, nil
}
func (m *mockRepo) DeleteExpired(_ context.Context, _ time.Time, _ int) (int64, error) {
	m.deleteExpiredCount++
	return 0, nil
}

type mockSourceReader struct {
	completeness map[Producer]Coverage
}

func (m mockSourceReader) Completeness(context.Context, *int64, string) map[Producer]Coverage {
	return m.completeness
}
func (m mockSourceReader) ActiveDiagnoses(context.Context, *int64, string) int64 { return 0 }
func (m mockSourceReader) RecentChanges(context.Context, *int64, string, time.Time) ([]OverviewChange, OverviewOutcomes, error) {
	return nil, OverviewOutcomes{}, nil
}

func TestService_GetNotFoundWithNopRepository(t *testing.T) {
	svc := NewService(ServiceOptions{}) // nil repo -> NopRepository
	if _, err := svc.Get(context.Background(), 123); !errors.Is(err, ErrSignalNotFound) {
		t.Fatalf("Get err = %v, want ErrSignalNotFound", err)
	}
}

func TestService_IngestBatchCountsSuccesses(t *testing.T) {
	svc := NewService(ServiceOptions{}) // nil repo -> NopRepository, writes ignored
	reqs := []IngestRequest{
		{SignalID: "diag.pod.pending.v1", ClusterID: 1, Resource: ResourceCitation{Kind: "Pod", Namespace: "default", Name: "web-0"}, ObservedAt: time.Now()},
		{SignalID: "bogus.signal.v1", ClusterID: 1, Resource: ResourceCitation{Kind: "Pod", Name: "x"}, ObservedAt: time.Now()},
		{SignalID: "diag.pod.pending.v1", ClusterID: 1, Resource: ResourceCitation{Kind: "Pod", Name: "web-1"}, ObservedAt: time.Now()},
	}
	ok, err := svc.IngestBatch(context.Background(), reqs)
	if err == nil {
		t.Fatal("expected first error from batch with unregistered signal")
	}
	if ok != 2 {
		t.Fatalf("IngestBatch ok = %d, want 2", ok)
	}
}

func TestNopSourceReaderDefaults(t *testing.T) {
	r := NopSourceReader{}
	if got := r.Completeness(context.Background(), nil, ""); got == nil || len(got) != 0 {
		t.Fatalf("Completeness = %v, want empty map", got)
	}
	if got := r.ActiveDiagnoses(context.Background(), nil, ""); got != 0 {
		t.Fatalf("ActiveDiagnoses = %d, want 0", got)
	}
	if items, outcomes, err := r.RecentChanges(context.Background(), nil, "", time.Now()); err != nil || items != nil || outcomes != (OverviewOutcomes{}) {
		t.Fatalf("RecentChanges = %v/%v/%v, want nil/empty/nil", items, outcomes, err)
	}
}

func TestBuildOccurrence_RejectsZeroClusterAndMissingObservedAt(t *testing.T) {
	base := IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		Resource:   ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx"},
		ObservedAt: time.Now(),
	}
	zeroCluster := base
	zeroCluster.ClusterID = 0
	if _, err := BuildOccurrence(zeroCluster, time.Now()); err == nil {
		t.Fatal("expected error for cluster_id <= 0")
	}
	zeroObserved := base
	zeroObserved.ClusterID = 1
	zeroObserved.ObservedAt = time.Time{}
	if _, err := BuildOccurrence(zeroObserved, time.Now()); err == nil {
		t.Fatal("expected error for zero observed_at")
	}
}

func TestBuildOccurrence_FallsBackToObservedAtForFreshness(t *testing.T) {
	observed := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	occ, err := BuildOccurrence(IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		ClusterID:  1,
		Resource:   ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx"},
		ObservedAt: observed,
	}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !occ.Freshness.Equal(observed) {
		t.Fatalf("Freshness = %s, want observed_at %s", occ.Freshness, observed)
	}
}

func TestService_OverviewRejectsRecentChangesError(t *testing.T) {
	svc := NewService(ServiceOptions{Repository: &mockRepo{}, Now: fixedNow, OverviewWindow: time.Hour})
	_, err := svc.Overview(context.Background(), nil, "", &errorSourceReader{})
	if err == nil {
		t.Fatal("Overview must propagate RecentChanges errors")
	}
}

type errorSourceReader struct{ mockSourceReader }

func (errorSourceReader) RecentChanges(context.Context, *int64, string, time.Time) ([]OverviewChange, OverviewOutcomes, error) {
	return nil, OverviewOutcomes{}, errors.New("changes unavailable")
}
