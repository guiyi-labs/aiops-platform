package topology

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestServiceCollectNamespace_DisabledCollector(t *testing.T) {
	svc := NewService(nil, NopRepository{}, nil)
	_, err := svc.CollectNamespace(context.Background(), 1, "default", time.Now())
	if err == nil {
		t.Error("expected error when collector is disabled")
	}
}

func TestServiceCollectNamespace_EmptyNamespace(t *testing.T) {
	reader := &stubReader{}
	collector := NewCollector(reader, 100)
	svc := NewService(collector, NopRepository{}, nil)
	_, err := svc.CollectNamespace(context.Background(), 1, "", time.Now())
	if err == nil {
		t.Error("expected error for empty namespace")
	}
}

func TestServiceCollectNamespace_Success(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	var dep k8sgateway.Deployment
	json.Unmarshal([]byte(`{"metadata":{"name":"web","uid":"dep-uid-1","namespace":"default","labels":{"app":"web"}},"spec":{"replicas":2}}`), &dep)
	var pod k8sgateway.Pod
	json.Unmarshal([]byte(`{"metadata":{"name":"web-1","uid":"pod-uid-1","namespace":"default","labels":{"app":"web"}},"spec":{"nodeName":"node-1","containers":[{"name":"web","image":"nginx"}]}}`), &pod)
	var svc k8sgateway.ServiceResource
	json.Unmarshal([]byte(`{"metadata":{"name":"web-svc","uid":"svc-uid-1","namespace":"default"},"spec":{"type":"ClusterIP","selector":{"app":"web"}}}`), &svc)

	reader := &stubReader{
		deployments: []k8sgateway.Deployment{dep},
		pods:        []k8sgateway.Pod{pod},
		services:    []k8sgateway.ServiceResource{svc},
	}
	collector := NewCollector(reader, 100)
	repo := &countingRepository{}
	svc2 := NewService(collector, repo, nil)

	result, err := svc2.CollectNamespace(context.Background(), 1, "default", now)
	if err != nil {
		t.Fatalf("CollectNamespace failed: %v", err)
	}
	if result.EdgesSeen == 0 {
		t.Error("expected at least 1 edge seen")
	}
	if result.EdgesUpserted != result.EdgesSeen {
		t.Errorf("expected all edges upserted, seen=%d upserted=%d", result.EdgesSeen, result.EdgesUpserted)
	}
	if repo.upsertCount == 0 {
		t.Error("repository should have received upsert calls")
	}
}

func TestServiceGetTopologyGraph_Empty(t *testing.T) {
	svc := NewService(nil, NopRepository{}, nil)
	graph, err := svc.GetTopologyGraph(context.Background(), 1, "default", 200)
	if err != nil {
		t.Fatalf("GetTopologyGraph failed: %v", err)
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(graph.Edges))
	}
	if graph.Completeness.State != "partial" {
		t.Errorf("expected completeness state partial for empty graph, got %s", graph.Completeness.State)
	}
}

func TestServiceGetTopologyGraph_WithEdges(t *testing.T) {
	now := time.Now().UTC()
	repo := &countingRepository{
		edges: []Edge{
			{
				ID:              1,
				ClusterID:       1,
				Kind:            EdgeOwns,
				Source:          ResourceCitation{Kind: "ReplicaSet", Name: "rs", UID: "rs-uid"},
				Target:          ResourceCitation{Kind: "Pod", Name: "pod", UID: "pod-uid"},
				Derivation:      DerivationOwnerReference,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			},
		},
		edgeTotal: 1,
	}
	svc := NewService(nil, repo, nil)
	graph, err := svc.GetTopologyGraph(context.Background(), 1, "default", 200)
	if err != nil {
		t.Fatalf("GetTopologyGraph failed: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(graph.Edges))
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes (source+target), got %d", len(graph.Nodes))
	}
	if graph.Completeness.State != "complete" {
		t.Errorf("expected completeness complete, got %s", graph.Completeness.State)
	}
}

func TestServiceGetChangeTimeline(t *testing.T) {
	now := time.Now().UTC()
	repo := &countingRepository{
		changeEvents: []ChangeEvent{
			{
				ID:         1,
				ClusterID:  1,
				Kind:       "promotion",
				PlanID:     "plan-1",
				Target:     ResourceCitation{Kind: "Deployment", Name: "web"},
				Action:     "promote",
				StartedAt:  now,
				Result:     "succeeded",
				Confidence: "high",
				Source:     "platform",
			},
		},
		changeTotal: 1,
	}
	svc := NewService(nil, repo, nil)
	resp, err := svc.GetChangeTimeline(context.Background(), ChangeTimelineFilter{
		ClusterID: 1,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("GetChangeTimeline failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 change event, got %d", len(resp.Items))
	}
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
}

// countingRepository is a test Repository that counts calls and returns
// canned data.
type countingRepository struct {
	edges        []Edge
	edgeTotal    int64
	changeEvents []ChangeEvent
	changeTotal  int64
	upsertCount  int
	closeCount   int
}

func (r *countingRepository) UpsertEdge(ctx context.Context, edge *Edge) error {
	r.upsertCount++
	return nil
}
func (r *countingRepository) CloseEdge(ctx context.Context, clusterID int64, kind EdgeKind, sourceUID, targetUID string, derivation DerivationMethod, validTo time.Time) error {
	r.closeCount++
	return nil
}
func (r *countingRepository) ListEdges(ctx context.Context, filter EdgeFilter) ([]Edge, int64, error) {
	return r.edges, r.edgeTotal, nil
}
func (r *countingRepository) UpsertChangeEvent(ctx context.Context, event *ChangeEvent) error {
	return nil
}
func (r *countingRepository) ListChangeEvents(ctx context.Context, filter ChangeTimelineFilter) ([]ChangeEvent, int64, error) {
	return r.changeEvents, r.changeTotal, nil
}
