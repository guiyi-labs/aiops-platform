package correlation

// Covers the service/worker branches the existing tests leave open: the
// lookback option, partial input-gathering failures, persist failures, list
// truncation and error propagation, the impact-graph read, worker config
// defaults and every runScope outcome.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"k8s-aiops.local/backend/internal/cluster"
)

// --- fakes ---

// failingInputProvider fails every gather call with its configured error.
type failingInputProvider struct {
	signalsErr, changesErr, edgesErr, diagnosesErr error
}

func (p failingInputProvider) ActiveSignals(context.Context, int64, string, time.Duration) ([]SignalOccurrenceInput, error) {
	return nil, p.signalsErr
}
func (p failingInputProvider) RecentChanges(context.Context, int64, string, time.Duration) ([]ChangeEventInput, error) {
	return nil, p.changesErr
}
func (p failingInputProvider) TopologyEdges(context.Context, int64, string) ([]TopologyEdgeInput, error) {
	return nil, p.edgesErr
}
func (p failingInputProvider) RecentDiagnoses(context.Context, int64, string, time.Duration) ([]DiagnosisRef, error) {
	return nil, p.diagnosesErr
}

// diagnosesFailingProvider returns working signals/changes/edges but fails the
// diagnosis gather, so the pass is partial yet still produces results.
type diagnosesFailingProvider struct {
	fakeInputProvider
	err error
}

func (p diagnosesFailingProvider) RecentDiagnoses(context.Context, int64, string, time.Duration) ([]DiagnosisRef, error) {
	return nil, p.err
}

// lookbackRecordingProvider captures the lookback window the service passes to
// each gather call.
type lookbackRecordingProvider struct {
	active, changes, diagnoses time.Duration
}

func (p *lookbackRecordingProvider) ActiveSignals(_ context.Context, _ int64, _ string, d time.Duration) ([]SignalOccurrenceInput, error) {
	p.active = d
	return nil, nil
}
func (p *lookbackRecordingProvider) RecentChanges(_ context.Context, _ int64, _ string, d time.Duration) ([]ChangeEventInput, error) {
	p.changes = d
	return nil, nil
}
func (p *lookbackRecordingProvider) TopologyEdges(context.Context, int64, string) ([]TopologyEdgeInput, error) {
	return nil, nil
}
func (p *lookbackRecordingProvider) RecentDiagnoses(_ context.Context, _ int64, _ string, d time.Duration) ([]DiagnosisRef, error) {
	p.diagnoses = d
	return nil, nil
}

// upsertFailingRepository counts persist attempts and always fails.
type upsertFailingRepository struct {
	*fakeRepository
	err   error
	calls int
}

func (r *upsertFailingRepository) UpsertResult(context.Context, *CorrelationResult) (Case, error) {
	r.calls++
	return Case{}, r.err
}

// listStubRepository returns canned list results so truncation and error
// propagation can be asserted without storage.
type listStubRepository struct {
	NopRepository
	cases         []Case
	casesTotal    int64
	timeline      []Case
	timelineTotal int64
	err           error
}

func (r listStubRepository) ListCases(context.Context, CaseFilter) ([]Case, int64, error) {
	return r.cases, r.casesTotal, r.err
}
func (r listStubRepository) ListTimeline(context.Context, CaseFilter) ([]Case, int64, error) {
	return r.timeline, r.timelineTotal, r.err
}

// graphStubRepository serves a case view plus its impact graph.
type graphStubRepository struct {
	NopRepository
	view  CaseView
	links []ResourceLink
	err   error
}

func (r graphStubRepository) GetCase(context.Context, int64) (CaseView, error) {
	return r.view, r.err
}
func (r graphStubRepository) ListResourceLinks(context.Context, int64) ([]ResourceLink, error) {
	return r.links, nil
}

// stubCorrelator returns a canned pass result.
type stubCorrelator struct {
	result CorrelateResult
	err    error
}

func (c stubCorrelator) CorrelateNamespace(context.Context, int64, string) (CorrelateResult, error) {
	return c.result, c.err
}

// countingCorrelator counts passes and signals each call on a channel.
type countingCorrelator struct {
	mu    sync.Mutex
	calls int
	ch    chan struct{}
}

func (c *countingCorrelator) CorrelateNamespace(context.Context, int64, string) (CorrelateResult, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	select {
	case c.ch <- struct{}{}:
	default:
	}
	return CorrelateResult{}, nil
}

func (c *countingCorrelator) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// serviceCorrelationInputs mirrors the golden pod-failure scenario: a pod
// signal, the rollout that preceded it and the owning edge between them.
func serviceCorrelationInputs(now time.Time) fakeInputProvider {
	return fakeInputProvider{
		signals: []SignalOccurrenceInput{{
			ID:         100,
			SignalID:   "diag.pod.image_pull_backoff.v1",
			Producer:   "diagnosis",
			ClusterID:  7,
			Namespace:  "app",
			Resource:   ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: "pod-uid-001"},
			Severity:   "critical",
			State:      "active",
			Coverage:   "complete",
			ObservedAt: now.Add(-5 * time.Minute),
		}},
		changes: []ChangeEventInput{{
			ID:        200,
			ClusterID: 7,
			Namespace: "app",
			Kind:      "rollout",
			Target:    ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid-001"},
			Result:    "succeeded",
			StartedAt: now.Add(-30 * time.Minute),
			Source:    "platform",
		}},
		edges: []TopologyEdgeInput{{
			ID:        300,
			ClusterID: 7,
			Kind:      "Owns",
			Source:    ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid-001"},
			Target:    ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: "pod-uid-001"},
			ValidFrom: now.Add(-24 * time.Hour),
		}},
	}
}

// --- Service ---

func TestServiceWithLookbackBoundsGatherWindow(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	t.Run("custom window reaches the provider", func(t *testing.T) {
		custom := 90 * time.Minute
		rec := &lookbackRecordingProvider{}
		svc := NewService(newFakeRepository(), nil, rec, WithLookback(custom), WithNow(func() time.Time { return now }))
		if svc.lookback != custom {
			t.Fatalf("service lookback = %v, want %v", svc.lookback, custom)
		}
		if _, err := svc.CorrelateNamespace(context.Background(), 7, "app"); err != nil {
			t.Fatalf("CorrelateNamespace: %v", err)
		}
		if rec.active != custom || rec.changes != custom || rec.diagnoses != custom {
			t.Fatalf("provider lookbacks = %v/%v/%v, want %v", rec.active, rec.changes, rec.diagnoses, custom)
		}
	})

	t.Run("default window when unset", func(t *testing.T) {
		rec := &lookbackRecordingProvider{}
		svc := NewService(newFakeRepository(), nil, rec, WithNow(func() time.Time { return now }))
		if svc.lookback != DefaultLookback {
			t.Fatalf("default lookback = %v, want %v", svc.lookback, DefaultLookback)
		}
		if _, err := svc.CorrelateNamespace(context.Background(), 7, "app"); err != nil {
			t.Fatalf("CorrelateNamespace: %v", err)
		}
		if rec.active != DefaultLookback {
			t.Fatalf("provider lookback = %v, want %v", rec.active, DefaultLookback)
		}
	})
}

func TestServiceCorrelateNamespaceRejectsUnconfiguredProvider(t *testing.T) {
	svc := &Service{
		engine:   NewEngine(),
		repo:     newFakeRepository(),
		now:      time.Now,
		lookback: DefaultLookback,
	}
	result, err := svc.CorrelateNamespace(context.Background(), 1, "app")
	if !errors.Is(err, ErrCorrelationDisabled) {
		t.Fatalf("err = %v, want ErrCorrelationDisabled", err)
	}
	if result != (CorrelateResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}
}

func TestServiceCorrelateNamespaceRecordsPartialGatherFailures(t *testing.T) {
	boom := errors.New("gather blew up")
	cases := []struct {
		name       string
		provider   failingInputProvider
		wantPhrase string
	}{
		{"signals", failingInputProvider{signalsErr: boom}, "gather signals"},
		{"changes", failingInputProvider{changesErr: boom}, "gather changes"},
		{"edges", failingInputProvider{edgesErr: boom}, "gather edges"},
		{"diagnoses", failingInputProvider{diagnosesErr: boom}, "gather diagnoses"},
		{
			"all four keeps the first error",
			failingInputProvider{signalsErr: boom, changesErr: boom, edgesErr: boom, diagnosesErr: boom},
			"gather signals",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository(), nil, tc.provider)
			result, err := svc.CorrelateNamespace(context.Background(), 7, "app")

			// Gather failures are recorded, not fatal: the pass still returns.
			if err != nil {
				t.Fatalf("CorrelateNamespace returned %v, want nil (partial pass)", err)
			}
			if !result.Partial {
				t.Error("Partial = false, want true")
			}
			if result.Error == nil {
				t.Fatal("Error = nil, want the wrapped gather error")
			}
			if !errors.Is(result.Error, boom) {
				t.Errorf("Error = %v, want it to wrap the provider error", result.Error)
			}
			if !strings.Contains(result.Error.Error(), tc.wantPhrase) {
				t.Errorf("Error = %q, want it to mention %q", result.Error, tc.wantPhrase)
			}
			if result.InputsGathered != 0 || result.ResultsProduced != 0 || result.CasesUpserted != 0 {
				t.Errorf("counters = %+v, want all zero", result)
			}
			if result.ClusterID != 7 || result.Namespace != "app" {
				t.Errorf("scope = %d/%q, want 7/app", result.ClusterID, result.Namespace)
			}
		})
	}
}

func TestServiceCorrelateNamespacePartialFailureStillCorrelates(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	boom := errors.New("diagnosis store down")
	repo := newFakeRepository()
	provider := diagnosesFailingProvider{fakeInputProvider: serviceCorrelationInputs(now), err: boom}
	svc := NewService(repo, nil, provider, WithNow(func() time.Time { return now }))

	result, err := svc.CorrelateNamespace(context.Background(), 7, "app")
	if err != nil {
		t.Fatalf("CorrelateNamespace: %v", err)
	}
	if !result.Partial {
		t.Error("Partial = false, want true")
	}
	if !errors.Is(result.Error, boom) {
		t.Fatalf("Error = %v, want the diagnosis gather error", result.Error)
	}
	// The gathered signals/changes/edges must still have been correlated and
	// persisted despite the failed gather.
	if result.InputsGathered == 0 {
		t.Error("InputsGathered = 0, want the successfully gathered inputs")
	}
	if result.ResultsProduced == 0 {
		t.Error("ResultsProduced = 0, want results from the partial inputs")
	}
	if result.CasesUpserted != result.ResultsProduced {
		t.Errorf("CasesUpserted = %d, ResultsProduced = %d, want equal", result.CasesUpserted, result.ResultsProduced)
	}
	if len(repo.cases) != result.CasesUpserted {
		t.Errorf("persisted cases = %d, want %d", len(repo.cases), result.CasesUpserted)
	}
}

func TestServiceCorrelateNamespacePersistFailureIsRecorded(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	boom := errors.New("write conflict")
	repo := &upsertFailingRepository{fakeRepository: newFakeRepository(), err: boom}
	svc := NewService(repo, nil, serviceCorrelationInputs(now), WithNow(func() time.Time { return now }))

	result, err := svc.CorrelateNamespace(context.Background(), 7, "app")
	if err != nil {
		t.Fatalf("CorrelateNamespace returned %v, want nil (persist failures are recorded)", err)
	}
	if !result.Partial {
		t.Error("Partial = false, want true")
	}
	if result.Error == nil || !errors.Is(result.Error, boom) {
		t.Fatalf("Error = %v, want the wrapped persist error", result.Error)
	}
	if !strings.Contains(result.Error.Error(), "persist result") {
		t.Errorf("Error = %q, want it to mention the persist step", result.Error)
	}
	if result.CasesUpserted != 0 {
		t.Errorf("CasesUpserted = %d, want 0", result.CasesUpserted)
	}
	// Every produced result is attempted: one failure must not abort the rest.
	if result.ResultsProduced == 0 {
		t.Fatal("ResultsProduced = 0, want the engine results")
	}
	if repo.calls != result.ResultsProduced {
		t.Errorf("persist attempts = %d, want %d", repo.calls, result.ResultsProduced)
	}
}

func TestServiceListCasesTruncationAndError(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	item := Case{ID: 1, CaseKey: "k1", ClusterID: 7, PrimaryResource: ResourceCitation{Kind: "Pod", Name: "web"}, FirstObservedAt: now}

	t.Run("truncated when the page is smaller than the total", func(t *testing.T) {
		svc := NewService(listStubRepository{cases: []Case{item}, casesTotal: 5}, nil, nil)
		resp, err := svc.ListCases(context.Background(), CaseFilter{ClusterID: 7})
		if err != nil {
			t.Fatalf("ListCases: %v", err)
		}
		if !resp.Truncated {
			t.Error("Truncated = false, want true")
		}
		if resp.Total != 5 || len(resp.Items) != 1 {
			t.Fatalf("resp = %+v, want 1 item of total 5", resp)
		}
	})

	t.Run("not truncated when the page covers the total", func(t *testing.T) {
		svc := NewService(listStubRepository{cases: []Case{item}, casesTotal: 1}, nil, nil)
		resp, err := svc.ListCases(context.Background(), CaseFilter{ClusterID: 7})
		if err != nil {
			t.Fatalf("ListCases: %v", err)
		}
		if resp.Truncated {
			t.Error("Truncated = true, want false")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		boom := errors.New("list failed")
		svc := NewService(listStubRepository{err: boom}, nil, nil)
		resp, err := svc.ListCases(context.Background(), CaseFilter{})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the repository error", err)
		}
		if resp.Total != 0 || resp.Items != nil || resp.Truncated {
			t.Errorf("resp = %+v, want zero value", resp)
		}
	})
}

func TestServiceListTimelineTruncationAndError(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	item := Case{ID: 1, CaseKey: "k1", ClusterID: 7, FirstObservedAt: now}

	t.Run("truncated when the page is smaller than the total", func(t *testing.T) {
		svc := NewService(listStubRepository{timeline: []Case{item}, timelineTotal: 4}, nil, nil)
		resp, err := svc.ListTimeline(context.Background(), CaseFilter{ClusterID: 7})
		if err != nil {
			t.Fatalf("ListTimeline: %v", err)
		}
		if !resp.Truncated {
			t.Error("Truncated = false, want true")
		}
		if resp.Total != 4 || len(resp.Items) != 1 {
			t.Fatalf("resp = %+v, want 1 item of total 4", resp)
		}
	})

	t.Run("not truncated when the page covers the total", func(t *testing.T) {
		svc := NewService(listStubRepository{timeline: []Case{item}, timelineTotal: 1}, nil, nil)
		resp, err := svc.ListTimeline(context.Background(), CaseFilter{ClusterID: 7})
		if err != nil {
			t.Fatalf("ListTimeline: %v", err)
		}
		if resp.Truncated {
			t.Error("Truncated = true, want false")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		boom := errors.New("timeline failed")
		svc := NewService(listStubRepository{err: boom}, nil, nil)
		resp, err := svc.ListTimeline(context.Background(), CaseFilter{})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the repository error", err)
		}
		if resp.Total != 0 || resp.Items != nil || resp.Truncated {
			t.Errorf("resp = %+v, want zero value", resp)
		}
	})
}

func TestServiceGetCaseGraphReturnsImpactLinks(t *testing.T) {
	caseID := int64(5)
	links := []ResourceLink{
		{ID: 1, CaseID: caseID, Resource: ResourceCitation{Kind: "Deployment", Name: "web"}, Relation: ResourceRelationUpstream},
		{ID: 2, CaseID: caseID, Resource: ResourceCitation{Kind: "Service", Name: "web-svc"}, Relation: ResourceRelationRelated},
	}
	repo := graphStubRepository{
		view:  CaseView{Case: Case{ID: caseID, CaseKey: "k"}},
		links: links,
	}
	svc := NewService(repo, nil, nil)

	got, err := svc.GetCaseGraph(context.Background(), caseID)
	if err != nil {
		t.Fatalf("GetCaseGraph: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("links = %+v, want 2", got)
	}
	if got[0].Relation != ResourceRelationUpstream || got[1].Relation != ResourceRelationRelated {
		t.Errorf("relations = %s/%s", got[0].Relation, got[1].Relation)
	}
	if got[0].CaseID != caseID {
		t.Errorf("case id = %d, want %d", got[0].CaseID, caseID)
	}
}

func TestServiceListActionCandidatesIgnoresUnconfirmedCandidates(t *testing.T) {
	caseID := int64(4)
	repo := newFakeRepository()
	repo.cases[caseID] = Case{
		ID:              caseID,
		CaseKey:         "key-4",
		ClusterID:       1,
		RuleID:          "correlation.rollout_causes_unavailable_deployment.v1",
		PrimaryResource: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web"},
		Status:          CaseStatusActive,
		Confidence:      ConfidenceConfirmed,
	}
	// A candidate that is still a hypothesis must not unlock a rollback.
	repo.changeCandidates[caseID] = []ChangeCandidate{{
		ID:         40,
		CaseID:     caseID,
		RuleID:     "correlation.rollout_causes_unavailable_deployment.v1",
		Confidence: ConfidenceCandidate,
		Rank:       1,
	}}
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListActionCandidates(context.Background(), caseID)
	if err != nil {
		t.Fatalf("ListActionCandidates: %v", err)
	}
	if resp.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("items = %+v, want none for an unconfirmed candidate", resp.Items)
	}
}

func TestFindDeploymentHelpersIgnoreNonMatchingLinks(t *testing.T) {
	links := []ResourceLink{
		// Right relation, wrong kind.
		{Relation: ResourceRelationUpstream, Resource: ResourceCitation{Kind: "Service", Name: "web-svc"}},
		{Relation: ResourceRelationDownstream, Resource: ResourceCitation{Kind: "ConfigMap", Name: "cfg"}},
		// Right kind, wrong relation.
		{Relation: ResourceRelationRelated, Resource: ResourceCitation{Kind: "Deployment", Name: "web"}},
	}
	if got := findUpstreamDeployment(links); got != nil {
		t.Errorf("findUpstreamDeployment = %+v, want nil", got)
	}
	if got := findDownstreamDeployment(links); got != nil {
		t.Errorf("findDownstreamDeployment = %+v, want nil", got)
	}
	if got := findUpstreamDeployment(nil); got != nil {
		t.Errorf("findUpstreamDeployment(nil) = %+v, want nil", got)
	}
	if got := findDownstreamDeployment(nil); got != nil {
		t.Errorf("findDownstreamDeployment(nil) = %+v, want nil", got)
	}
}

// --- Worker ---

func TestNewWorkerAppliesDefaults(t *testing.T) {
	w := NewWorker(WorkerConfig{}, fakeClusterLister{}, fakeNamespaceLister{}, stubCorrelator{}, nil)
	if w.config.Interval != DefaultInterval {
		t.Errorf("Interval = %v, want %v", w.config.Interval, DefaultInterval)
	}
	if w.config.PerClusterTimeout != DefaultPerClusterTimeout {
		t.Errorf("PerClusterTimeout = %v, want %v", w.config.PerClusterTimeout, DefaultPerClusterTimeout)
	}
	if w.logger == nil {
		t.Fatal("logger = nil, want a no-op logger")
	}
	if w.clusters == nil || w.namespaces == nil || w.correlate == nil {
		t.Fatal("dependencies were not retained")
	}
}

func TestNewWorkerKeepsExplicitConfig(t *testing.T) {
	logger := zap.NewNop()
	w := NewWorker(WorkerConfig{Interval: 2 * time.Minute, PerClusterTimeout: 3 * time.Second},
		fakeClusterLister{}, fakeNamespaceLister{}, stubCorrelator{}, logger)
	if w.config.Interval != 2*time.Minute {
		t.Errorf("Interval = %v, want 2m", w.config.Interval)
	}
	if w.config.PerClusterTimeout != 3*time.Second {
		t.Errorf("PerClusterTimeout = %v, want 3s", w.config.PerClusterTimeout)
	}
	if w.logger != logger {
		t.Error("explicit logger was replaced")
	}
}

func TestWorkerRunScopeLogsEveryOutcome(t *testing.T) {
	boom := errors.New("engine exploded")
	cases := []struct {
		name     string
		result   CorrelateResult
		err      error
		wantLvl  zapcore.Level
		wantMsg  string
		wantKeys map[string]interface{}
	}{
		{
			name:    "complete pass",
			result:  CorrelateResult{InputsGathered: 3, ResultsProduced: 2, CasesUpserted: 2},
			wantLvl: zapcore.InfoLevel, wantMsg: "correlation worker: pass complete",
			wantKeys: map[string]interface{}{
				"inputs_gathered": int64(3), "results_produced": int64(2), "cases_upserted": int64(2),
			},
		},
		{
			name:    "partial pass",
			result:  CorrelateResult{Partial: true, InputsGathered: 1, ResultsProduced: 1, CasesUpserted: 0},
			wantLvl: zapcore.WarnLevel, wantMsg: "correlation worker: partial pass",
			wantKeys: map[string]interface{}{
				"inputs_gathered": int64(1), "results_produced": int64(1), "cases_upserted": int64(0),
			},
		},
		{
			name:    "errored pass",
			err:     boom,
			wantLvl: zapcore.WarnLevel, wantMsg: "correlation worker: pass errored",
			wantKeys: map[string]interface{}{"error": "engine exploded"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			w := NewWorker(WorkerConfig{Interval: time.Hour}, fakeClusterLister{}, fakeNamespaceLister{},
				stubCorrelator{result: tc.result, err: tc.err}, zap.New(core))

			w.runScope(context.Background(), 3, "app")

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want exactly 1: %+v", len(entries), entries)
			}
			entry := entries[0]
			if entry.Level != tc.wantLvl {
				t.Errorf("level = %v, want %v", entry.Level, tc.wantLvl)
			}
			if entry.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", entry.Message, tc.wantMsg)
			}
			fields := entry.ContextMap()
			if fields["cluster_id"] != int64(3) {
				t.Errorf("cluster_id = %#v, want 3", fields["cluster_id"])
			}
			if fields["namespace"] != "app" {
				t.Errorf("namespace = %#v, want app", fields["namespace"])
			}
			for k, want := range tc.wantKeys {
				if fields[k] != want {
					t.Errorf("field %s = %#v, want %#v", k, fields[k], want)
				}
			}
		})
	}
}

func TestWorkerRunExecutesPeriodicPasses(t *testing.T) {
	clusters := fakeClusterLister{items: []cluster.Cluster{{ID: 1, Name: "prod", Enabled: true}}}
	namespaces := fakeNamespaceLister{byCluster: map[int64][]string{1: {"app"}}}
	correlate := &countingCorrelator{ch: make(chan struct{}, 16)}
	w := NewWorker(WorkerConfig{Interval: 5 * time.Millisecond, PerClusterTimeout: time.Second},
		clusters, namespaces, correlate, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// The first pass runs immediately (two scopes: "app" and the
	// cluster-scoped pass); reaching four calls proves the ticker fired and a
	// second periodic pass ran.
	deadline := time.After(5 * time.Second)
	for correlate.count() < 4 {
		select {
		case <-deadline:
			t.Fatalf("worker did not run a periodic pass: %d calls", correlate.count())
		case <-time.After(2 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
