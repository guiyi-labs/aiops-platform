package correlation

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/topology"
)

type fakeSignalReader struct {
	filter signal.ListFilter
	items  []signal.Occurrence
	err    error
}

func (f *fakeSignalReader) List(_ context.Context, filter signal.ListFilter) ([]signal.Occurrence, int64, error) {
	f.filter = filter
	return f.items, int64(len(f.items)), f.err
}

type fakeTopologyReader struct {
	edgeFilter   topology.EdgeFilter
	changeFilter topology.ChangeTimelineFilter
	edges        []topology.Edge
	changes      []topology.ChangeEvent
	err          error
}

func (f *fakeTopologyReader) ListEdges(_ context.Context, filter topology.EdgeFilter) ([]topology.Edge, int64, error) {
	f.edgeFilter = filter
	return f.edges, int64(len(f.edges)), f.err
}

func (f *fakeTopologyReader) ListChangeEvents(_ context.Context, filter topology.ChangeTimelineFilter) ([]topology.ChangeEvent, int64, error) {
	f.changeFilter = filter
	return f.changes, int64(len(f.changes)), f.err
}

type fakeDiagnosisReader struct {
	filter diagnosis.ListFilter
	items  []diagnosis.Record
	err    error
}

func (f *fakeDiagnosisReader) List(_ context.Context, filter diagnosis.ListFilter) ([]diagnosis.Record, error) {
	f.filter = filter
	return f.items, f.err
}

func newTestProvider(signals *fakeSignalReader, graph *fakeTopologyReader, diag *fakeDiagnosisReader) *RepositoryInputProvider {
	p := NewRepositoryInputProvider(signals, graph, diag)
	p.now = func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }
	return p
}

func TestProviderActiveSignalsMapsAndFilters(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	occ := signal.Occurrence{
		ID:         11,
		SignalID:   "diag.pod.pending.v1",
		Producer:   signal.ProducerDiagnosis,
		ClusterID:  7,
		Namespace:  "app",
		Resource:   signal.ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: ""},
		Severity:   signal.SeverityCritical,
		State:      signal.StateActive,
		Coverage:   signal.CoveragePartial,
		Freshness:  now.Add(-2 * time.Minute),
		ObservedAt: now.Add(-3 * time.Minute),
		Evidence:   []signal.EvidenceRef{{Kind: "diagnosis_record", ID: 42, ContentHash: "abc"}},
	}
	signals := &fakeSignalReader{items: []signal.Occurrence{occ}}
	p := newTestProvider(signals, &fakeTopologyReader{}, &fakeDiagnosisReader{})

	inputs, err := p.ActiveSignals(context.Background(), 7, "app", 4*time.Hour)
	if err != nil {
		t.Fatalf("ActiveSignals: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("want 1 input, got %d", len(inputs))
	}
	in := inputs[0]
	if in.ID != 11 || in.SignalID != "diag.pod.pending.v1" || in.Producer != "diagnosis" {
		t.Errorf("identity mapping wrong: %+v", in)
	}
	if in.Coverage != "partial" || in.Severity != "critical" || in.State != "active" {
		t.Errorf("string field mapping wrong: coverage=%q severity=%q state=%q", in.Coverage, in.Severity, in.State)
	}
	if !in.Resource.Incomplete || in.Resource.UID != "" || in.Resource.Name != "web-abc" {
		t.Errorf("incomplete resource mapping wrong: %+v", in.Resource)
	}
	if len(in.Evidence) != 1 || in.Evidence[0].ID != 42 || in.Evidence[0].ContentHash != "abc" {
		t.Errorf("evidence mapping wrong: %+v", in.Evidence)
	}
	if signals.filter.ClusterID == nil || *signals.filter.ClusterID != 7 {
		t.Errorf("cluster filter wrong: %+v", signals.filter.ClusterID)
	}
	if signals.filter.Namespace != "app" || signals.filter.State != signal.StateActive {
		t.Errorf("scope filter wrong: %+v", signals.filter)
	}
	wantSince := now.Add(-4 * time.Hour)
	if signals.filter.WindowStart == nil || !signals.filter.WindowStart.Equal(wantSince) {
		t.Errorf("window start = %v, want %v", signals.filter.WindowStart, wantSince)
	}
	if signals.filter.Limit != providerSignalsLimit {
		t.Errorf("limit = %d, want %d", signals.filter.Limit, providerSignalsLimit)
	}
}

func TestProviderActiveSignalsPropagatesError(t *testing.T) {
	signals := &fakeSignalReader{err: errors.New("boom")}
	p := newTestProvider(signals, &fakeTopologyReader{}, &fakeDiagnosisReader{})
	if _, err := p.ActiveSignals(context.Background(), 7, "app", time.Hour); err == nil {
		t.Fatal("want error propagated")
	}
}

func TestProviderRecentChangesMapsAndFilters(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	ev := topology.ChangeEvent{
		ID:         22,
		ClusterID:  7,
		Namespace:  "app",
		Kind:       "rollout",
		PlanID:     "plan-9",
		Target:     topology.ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid"},
		Action:     "restart",
		Result:     "succeeded",
		Actor:      "alice",
		StartedAt:  now.Add(-10 * time.Minute),
		Evidence:   []topology.EvidenceRef{{Kind: "audit_entry", ID: 7, ContentHash: "def"}},
		Confidence: "high",
		Source:     "platform",
	}
	graph := &fakeTopologyReader{changes: []topology.ChangeEvent{ev}}
	p := newTestProvider(&fakeSignalReader{}, graph, &fakeDiagnosisReader{})

	inputs, err := p.RecentChanges(context.Background(), 7, "app", time.Hour)
	if err != nil {
		t.Fatalf("RecentChanges: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("want 1 input, got %d", len(inputs))
	}
	in := inputs[0]
	if in.Kind != "rollout" || in.PlanID != "plan-9" || in.Result != "succeeded" || in.Actor != "alice" {
		t.Errorf("change mapping wrong: %+v", in)
	}
	if in.Target.UID != "deploy-uid" || in.Target.Incomplete {
		t.Errorf("target mapping wrong: %+v", in.Target)
	}
	if len(in.Evidence) != 1 || in.Evidence[0].Kind != "audit_entry" || in.Evidence[0].ContentHash != "def" {
		t.Errorf("evidence mapping wrong: %+v", in.Evidence)
	}
	if graph.changeFilter.ClusterID != 7 || graph.changeFilter.Namespace != "app" {
		t.Errorf("change scope filter wrong: %+v", graph.changeFilter)
	}
	wantSince := now.Add(-time.Hour)
	if graph.changeFilter.StartTime == nil || !graph.changeFilter.StartTime.Equal(wantSince) {
		t.Errorf("start time = %v, want %v", graph.changeFilter.StartTime, wantSince)
	}
	if graph.changeFilter.Limit != providerChangesLimit {
		t.Errorf("limit = %d, want %d", graph.changeFilter.Limit, providerChangesLimit)
	}
}

func TestProviderTopologyEdgesMapsAndFilters(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	edge := topology.Edge{
		ID:        33,
		ClusterID: 7,
		Kind:      topology.EdgeOwns,
		Source:    topology.ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "d-1"},
		Target:    topology.ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: ""},
		ValidFrom: now.Add(-48 * time.Hour),
	}
	graph := &fakeTopologyReader{edges: []topology.Edge{edge}}
	p := newTestProvider(&fakeSignalReader{}, graph, &fakeDiagnosisReader{})

	inputs, err := p.TopologyEdges(context.Background(), 7, "app")
	if err != nil {
		t.Fatalf("TopologyEdges: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("want 1 input, got %d", len(inputs))
	}
	in := inputs[0]
	if in.Kind != "Owns" || in.Source.UID != "d-1" || !in.Target.Incomplete {
		t.Errorf("edge mapping wrong: %+v", in)
	}
	if graph.edgeFilter.ClusterID != 7 || graph.edgeFilter.Namespace != "app" {
		t.Errorf("edge scope filter wrong: %+v", graph.edgeFilter)
	}
	if graph.edgeFilter.ValidAt == nil || !graph.edgeFilter.ValidAt.Equal(now) {
		t.Errorf("valid-at = %v, want %v", graph.edgeFilter.ValidAt, now)
	}
	if graph.edgeFilter.Limit != providerEdgesLimit {
		t.Errorf("limit = %d, want %d", graph.edgeFilter.Limit, providerEdgesLimit)
	}
}

func TestProviderRecentDiagnosesMapsAndFilters(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	rec := diagnosis.Record{
		ID:         44,
		ClusterID:  7,
		RuleID:     diagnosis.RuleCrashLoopBackOff,
		Resource:   diagnosis.ResourceRef{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: "pod-1"},
		Severity:   "warning",
		Status:     "open",
		ObservedAt: now.Add(-15 * time.Minute),
	}
	diag := &fakeDiagnosisReader{items: []diagnosis.Record{rec}}
	p := newTestProvider(&fakeSignalReader{}, &fakeTopologyReader{}, diag)

	inputs, err := p.RecentDiagnoses(context.Background(), 7, "app", 2*time.Hour)
	if err != nil {
		t.Fatalf("RecentDiagnoses: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("want 1 input, got %d", len(inputs))
	}
	in := inputs[0]
	if in.ID != 44 || in.RuleID != diagnosis.RuleCrashLoopBackOff || in.Severity != "warning" || in.Status != "open" {
		t.Errorf("diagnosis mapping wrong: %+v", in)
	}
	if in.Resource.UID != "pod-1" || in.Resource.Incomplete {
		t.Errorf("resource mapping wrong: %+v", in.Resource)
	}
	if diag.filter.ClusterID != 7 || diag.filter.Limit != providerDiagnosesLimit {
		t.Errorf("diagnosis scope filter wrong: %+v", diag.filter)
	}
	wantSince := now.Add(-2 * time.Hour)
	if diag.filter.Since == nil || !diag.filter.Since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", diag.filter.Since, wantSince)
	}
}
