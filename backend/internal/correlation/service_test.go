package correlation

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository for service-layer tests.
type fakeRepository struct {
	cases            map[int64]Case
	nextCaseID       int64
	signalLinks      map[int64][]SignalLink
	resourceLinks    map[int64][]ResourceLink
	changeCandidates map[int64][]ChangeCandidate
	nextLinkID       int64
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		cases:            make(map[int64]Case),
		signalLinks:      make(map[int64][]SignalLink),
		resourceLinks:    make(map[int64][]ResourceLink),
		changeCandidates: make(map[int64][]ChangeCandidate),
		nextCaseID:       1,
		nextLinkID:       1,
	}
}

func (r *fakeRepository) UpsertResult(_ context.Context, result *CorrelationResult) (Case, error) {
	// Idempotent by case_key for active cases.
	for id, c := range r.cases {
		if c.CaseKey == result.Case.CaseKey && c.Status == CaseStatusActive {
			result.Case.ID = id
			r.cases[id] = result.Case
			r.persistLinks(id, result)
			return result.Case, nil
		}
	}
	id := r.nextCaseID
	r.nextCaseID++
	result.Case.ID = id
	r.cases[id] = result.Case
	r.persistLinks(id, result)
	return result.Case, nil
}

func (r *fakeRepository) persistLinks(caseID int64, result *CorrelationResult) {
	for i := range result.SignalLinks {
		result.SignalLinks[i].ID = r.nextLinkID
		r.nextLinkID++
		result.SignalLinks[i].CaseID = caseID
	}
	r.signalLinks[caseID] = append(r.signalLinks[caseID], result.SignalLinks...)

	for i := range result.ResourceLinks {
		result.ResourceLinks[i].ID = r.nextLinkID
		r.nextLinkID++
		result.ResourceLinks[i].CaseID = caseID
	}
	r.resourceLinks[caseID] = append(r.resourceLinks[caseID], result.ResourceLinks...)

	for i := range result.ChangeCandidates {
		result.ChangeCandidates[i].ID = r.nextLinkID
		r.nextLinkID++
		result.ChangeCandidates[i].CaseID = caseID
	}
	r.changeCandidates[caseID] = append(r.changeCandidates[caseID], result.ChangeCandidates...)
}

func (r *fakeRepository) GetCase(_ context.Context, id int64) (CaseView, error) {
	c, ok := r.cases[id]
	if !ok {
		return CaseView{}, ErrCaseNotFound
	}
	return CaseView{
		Case:             c,
		SignalLinks:      r.signalLinks[id],
		ResourceLinks:    r.resourceLinks[id],
		ChangeCandidates: r.changeCandidates[id],
		GeneratedAt:      time.Now().UTC(),
	}, nil
}

func (r *fakeRepository) ListCases(_ context.Context, filter CaseFilter) ([]Case, int64, error) {
	var items []Case
	for _, c := range r.cases {
		if filter.ClusterID > 0 && c.ClusterID != filter.ClusterID {
			continue
		}
		if filter.Namespace != "" && c.PrimaryResource.Namespace != filter.Namespace {
			continue
		}
		if filter.RuleID != "" && c.RuleID != filter.RuleID {
			continue
		}
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		if filter.Confidence != "" && c.Confidence != filter.Confidence {
			continue
		}
		if filter.PrimaryKind != "" && c.PrimaryResource.Kind != filter.PrimaryKind {
			continue
		}
		items = append(items, c)
	}
	return items, int64(len(items)), nil
}

func (r *fakeRepository) ListTimeline(ctx context.Context, filter CaseFilter) ([]Case, int64, error) {
	return r.ListCases(ctx, filter)
}

func (r *fakeRepository) ListSignalLinks(_ context.Context, caseID int64) ([]SignalLink, error) {
	return r.signalLinks[caseID], nil
}

func (r *fakeRepository) ListResourceLinks(_ context.Context, caseID int64) ([]ResourceLink, error) {
	return r.resourceLinks[caseID], nil
}

func (r *fakeRepository) ListChangeCandidates(_ context.Context, caseID int64) ([]ChangeCandidate, error) {
	return r.changeCandidates[caseID], nil
}

func (r *fakeRepository) ResolveCaseStatus(_ context.Context, caseID int64, status CaseStatus, now time.Time) error {
	c, ok := r.cases[caseID]
	if !ok {
		return ErrCaseNotFound
	}
	c.Status = status
	c.UpdatedAt = now
	r.cases[caseID] = c
	return nil
}

// fakeInputProvider returns canned inputs for CorrelateNamespace tests.
type fakeInputProvider struct {
	signals   []SignalOccurrenceInput
	changes   []ChangeEventInput
	edges     []TopologyEdgeInput
	diagnoses []DiagnosisRef
}

func (p fakeInputProvider) ActiveSignals(context.Context, int64, string, time.Duration) ([]SignalOccurrenceInput, error) {
	return p.signals, nil
}
func (p fakeInputProvider) RecentChanges(context.Context, int64, string, time.Duration) ([]ChangeEventInput, error) {
	return p.changes, nil
}
func (p fakeInputProvider) TopologyEdges(context.Context, int64, string) ([]TopologyEdgeInput, error) {
	return p.edges, nil
}
func (p fakeInputProvider) RecentDiagnoses(context.Context, int64, string, time.Duration) ([]DiagnosisRef, error) {
	return p.diagnoses, nil
}

func TestServiceCorrelateNamespace(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	provider := fakeInputProvider{
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
	svc := NewService(repo, nil, provider, WithNow(func() time.Time { return now }))

	result, err := svc.CorrelateNamespace(context.Background(), 7, "app")
	if err != nil {
		t.Fatalf("CorrelateNamespace: %v", err)
	}
	if result.InputsGathered == 0 {
		t.Error("expected inputs gathered > 0")
	}
	if result.ResultsProduced == 0 {
		t.Error("expected results produced > 0")
	}
	if result.CasesUpserted == 0 {
		t.Error("expected cases upserted > 0")
	}
	if result.Partial {
		t.Error("expected non-partial result")
	}
}

func TestServiceCorrelateNamespaceNopProvider(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil) // nil provider → NopInputProvider

	result, err := svc.CorrelateNamespace(context.Background(), 1, "ns")
	if err != nil {
		t.Fatalf("CorrelateNamespace: %v", err)
	}
	if result.InputsGathered != 0 {
		t.Errorf("expected 0 inputs gathered, got %d", result.InputsGathered)
	}
	if result.ResultsProduced != 0 {
		t.Errorf("expected 0 results produced, got %d", result.ResultsProduced)
	}
}

func TestServiceGetCase(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil)

	// NopRepository returns ErrCaseNotFound.
	_, err := svc.GetCase(context.Background(), 999)
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("expected ErrCaseNotFound, got %v", err)
	}
}

func TestServiceListCases(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListCases(context.Background(), CaseFilter{ClusterID: 1})
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total, got %d", resp.Total)
	}
}

func TestServiceListTimeline(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListTimeline(context.Background(), CaseFilter{ClusterID: 1})
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total, got %d", resp.Total)
	}
}

func TestServiceGetCaseGraph(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil)

	// Non-existent case → ErrCaseNotFound propagated.
	_, err := svc.GetCaseGraph(context.Background(), 999)
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("expected ErrCaseNotFound, got %v", err)
	}
}

func TestServiceListActionCandidatesNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, nil)

	_, err := svc.ListActionCandidates(context.Background(), 999)
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("expected ErrCaseNotFound, got %v", err)
	}
}

func TestServiceListActionCandidatesRollback(t *testing.T) {
	repo := newFakeRepository()
	caseID := int64(1)
	repo.cases[caseID] = Case{
		ID:              caseID,
		CaseKey:         "key-1",
		ClusterID:       1,
		RuleID:          "correlation.rollout_causes_unavailable_deployment.v1",
		PrimaryResource: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid-001"},
		Status:          CaseStatusActive,
		Confidence:      ConfidenceConfirmed,
	}
	repo.changeCandidates[caseID] = []ChangeCandidate{{
		ID:         10,
		CaseID:     caseID,
		RuleID:     "correlation.rollout_causes_unavailable_deployment.v1",
		Confidence: ConfidenceConfirmed,
		Rank:       1,
	}}
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListActionCandidates(context.Background(), caseID)
	if err != nil {
		t.Fatalf("ListActionCandidates: %v", err)
	}
	if resp.Total == 0 {
		t.Fatal("expected at least one action candidate")
	}
	if resp.Items[0].Code != "deployment.rollback" {
		t.Errorf("expected deployment.rollback, got %q", resp.Items[0].Code)
	}
	if !resp.Items[0].Eligible {
		t.Error("expected eligible=true")
	}
}

func TestServiceListActionCandidatesPodRolloutRestart(t *testing.T) {
	repo := newFakeRepository()
	caseID := int64(2)
	repo.cases[caseID] = Case{
		ID:              caseID,
		CaseKey:         "key-2",
		ClusterID:       1,
		RuleID:          "correlation.rollout_causes_pod_failure.v1",
		PrimaryResource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: "pod-uid-001"},
		Status:          CaseStatusActive,
		Confidence:      ConfidenceCandidate,
	}
	repo.resourceLinks[caseID] = []ResourceLink{{
		ID:       20,
		CaseID:   caseID,
		Resource: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid-001"},
		Relation: ResourceRelationUpstream,
	}}
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListActionCandidates(context.Background(), caseID)
	if err != nil {
		t.Fatalf("ListActionCandidates: %v", err)
	}
	if resp.Total == 0 {
		t.Fatal("expected at least one action candidate")
	}
	if resp.Items[0].Code != "deployment.rollout_restart" {
		t.Errorf("expected deployment.rollout_restart, got %q", resp.Items[0].Code)
	}
}

func TestServiceListActionCandidatesServiceBacking(t *testing.T) {
	repo := newFakeRepository()
	caseID := int64(3)
	repo.cases[caseID] = Case{
		ID:              caseID,
		CaseKey:         "key-3",
		ClusterID:       1,
		RuleID:          "correlation.rollout_causes_no_endpoints.v1",
		PrimaryResource: ResourceCitation{Kind: "Service", Namespace: "app", Name: "web-svc", UID: "svc-uid-001"},
		Status:          CaseStatusActive,
		Confidence:      ConfidenceCandidate,
	}
	repo.resourceLinks[caseID] = []ResourceLink{{
		ID:       30,
		CaseID:   caseID,
		Resource: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid-001"},
		Relation: ResourceRelationDownstream,
	}}
	svc := NewService(repo, nil, nil)

	resp, err := svc.ListActionCandidates(context.Background(), caseID)
	if err != nil {
		t.Fatalf("ListActionCandidates: %v", err)
	}
	if resp.Total == 0 {
		t.Fatal("expected at least one action candidate")
	}
	if resp.Items[0].Code != "deployment.rollout_restart" {
		t.Errorf("expected deployment.rollout_restart, got %q", resp.Items[0].Code)
	}
}
