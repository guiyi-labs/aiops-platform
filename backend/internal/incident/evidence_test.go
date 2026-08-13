package incident

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeEvidenceRepo is a minimal in-memory Repository for evidence tests.
type fakeEvidenceRepo struct {
	byID    map[int64]Incident
	nextID  int64
	sources map[string]int64
}

func newFakeEvidenceRepo() *fakeEvidenceRepo {
	return &fakeEvidenceRepo{byID: map[int64]Incident{}, nextID: 1, sources: map[string]int64{}}
}

func (r *fakeEvidenceRepo) Create(_ context.Context, record *Incident) error {
	if _, exists := r.sources[record.SourceType+"|"+record.SourceRef]; exists {
		return ErrSourceAlreadyUsed
	}
	r.sources[record.SourceType+"|"+record.SourceRef] = -1
	record.ID = r.nextID
	r.nextID++
	record.Number = "INC-" + strconv.FormatInt(record.ID, 10)
	record.Status = StatusOpen
	r.byID[record.ID] = *record
	return nil
}

func (r *fakeEvidenceRepo) Get(_ context.Context, id int64) (Incident, error) {
	record, ok := r.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	return record, nil
}

func (r *fakeEvidenceRepo) List(context.Context, ListFilter) ([]Incident, error) { return nil, nil }
func (r *fakeEvidenceRepo) Summary(context.Context) (Summary, error)             { return Summary{}, nil }
func (r *fakeEvidenceRepo) Transition(context.Context, int64, int64, string, ActorRef, string) (Incident, error) {
	return Incident{}, errors.New("unused")
}
func (r *fakeEvidenceRepo) Assign(context.Context, int64, int64, int64, ActorRef, string) (Incident, error) {
	return Incident{}, errors.New("unused")
}
func (r *fakeEvidenceRepo) AddFollower(context.Context, int64, int64, ActorRef) (Incident, error) {
	return Incident{}, errors.New("unused")
}
func (r *fakeEvidenceRepo) RemoveFollower(context.Context, int64, int64, ActorRef) (Incident, error) {
	return Incident{}, errors.New("unused")
}
func (r *fakeEvidenceRepo) AddNote(context.Context, int64, int64, ActorRef, string) (Incident, error) {
	return Incident{}, errors.New("unused")
}
func (r *fakeEvidenceRepo) SetPostmortem(context.Context, int64, int64, ActorRef, string) (Incident, error) {
	return Incident{}, errors.New("unused")
}

type staticEvidenceResolver struct {
	item EvidenceItem
	err  error
}

func (r staticEvidenceResolver) ResolveEvidence(context.Context, string, string, int64) (EvidenceItem, error) {
	return r.item, r.err
}

func TestService_Evidence_SnapshotFallback(t *testing.T) {
	repo := newFakeEvidenceRepo()
	svc := NewService(repo)
	input := CreateInput{
		SourceType: SourceTypeFinding,
		SourceRef:  "finding:1:code:kind:ns:name",
		ClusterID:  7,
		Title:      "manual finding",
		Severity:   SeverityWarning,
		Summary:    "reported",
		Resource:   ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0"},
	}
	created, err := svc.Create(context.Background(), input)
	require.NoError(t, err)

	items, err := svc.Evidence(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	require.Equal(t, SourceTypeFinding, item.SourceType)
	require.Equal(t, "manual finding", item.Title)
	require.Equal(t, "reported", item.Summary)
	require.Equal(t, "Pod", item.Resource.Kind)
	require.Equal(t, "/incidents", item.DeepLink)
	require.NotEmpty(t, item.ObservedAt)
}

func TestService_Evidence_ResolverEnrichment(t *testing.T) {
	repo := newFakeEvidenceRepo()
	svc := NewService(repo).WithEvidenceResolver(staticEvidenceResolver{
		item: EvidenceItem{
			SourceType: SourceTypeDiagnosis,
			SourceRef:  "diagnosis:3",
			Title:      "node-not-ready",
			Summary:    "node K8S-W1 not ready",
			Severity:   SeverityCritical,
			Resource:   ResourceRef{Kind: "Node", Name: "K8S-W1"},
			ObservedAt: "2026-08-13T00:00:00.000Z",
			DeepLink:   "/diagnoses",
			Fields:     []EvidenceField{{Label: "集群", Value: "1"}},
		},
	})
	input := CreateInput{
		SourceType: SourceTypeDiagnosis,
		SourceRef:  "diagnosis:3",
		ClusterID:  1,
		Title:      "node-not-ready snapshot",
		Severity:   SeverityHigh,
		Resource:   ResourceRef{Kind: "Node", Name: "K8S-W1"},
	}
	created, err := svc.Create(context.Background(), input)
	require.NoError(t, err)

	items, err := svc.Evidence(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	require.Equal(t, "node-not-ready", item.Title) // resolver enriched
	require.Equal(t, "/diagnoses", item.DeepLink)
	require.Len(t, item.Fields, 1)
}

func TestService_Evidence_ResolverErrorFallsBackToSnapshot(t *testing.T) {
	repo := newFakeEvidenceRepo()
	svc := NewService(repo).WithEvidenceResolver(staticEvidenceResolver{err: errors.New("boom")})
	input := CreateInput{
		SourceType: SourceTypeAlert,
		SourceRef:  "alert:9",
		ClusterID:  2,
		Title:      "cpu-high",
		Severity:   SeverityHigh,
		Resource:   ResourceRef{Kind: "Deployment", Name: "api"},
	}
	created, err := svc.Create(context.Background(), input)
	require.NoError(t, err)

	items, err := svc.Evidence(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "cpu-high", items[0].Title) // snapshot title preserved
	require.Equal(t, "/alerts", items[0].DeepLink)
}

func TestService_Evidence_NotFound(t *testing.T) {
	repo := newFakeEvidenceRepo()
	svc := NewService(repo)
	_, err := svc.Evidence(context.Background(), 999)
	require.ErrorIs(t, err, ErrNotFound)
}
