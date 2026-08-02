package topology

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
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
	if repo.upsertCount.Load() == 0 {
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
// canned data. The counters are atomics because CollectCluster now runs
// namespaces concurrently and the workers share this repository.
type countingRepository struct {
	edges        []Edge
	edgeTotal    int64
	changeEvents []ChangeEvent
	changeTotal  int64
	upsertCount  atomic.Int64
	closeCount   atomic.Int64
}

func (r *countingRepository) UpsertEdge(ctx context.Context, edge *Edge) error {
	r.upsertCount.Add(1)
	return nil
}
func (r *countingRepository) CloseEdge(ctx context.Context, clusterID int64, kind EdgeKind, sourceUID, targetUID string, derivation DerivationMethod, validTo time.Time) error {
	r.closeCount.Add(1)
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

// stubNamespaceLister returns the canned namespace list.
type stubNamespaceLister struct{ namespaces []string }

func (l stubNamespaceLister) VisibleNamespaces(context.Context, int64) ([]string, error) {
	return l.namespaces, nil
}

// TestServiceCollectCluster_ConcurrentAggregates verifies that CollectCluster
// processes every namespace and aggregates edge counts without losing updates
// under concurrency. The counting repository is mutex-guarded so the parallel
// workers cannot corrupt the counters.
func TestServiceCollectCluster_ConcurrentAggregates(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	namespaces := []string{"ns-1", "ns-2", "ns-3", "ns-4", "ns-5", "ns-6"}

	// Each namespace yields exactly one Owns edge (RS → Pod).
	var rs k8sgateway.ReplicaSet
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc","uid":"rs-uid-1","namespace":"default"}}`, &rs)
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc-xyz","uid":"pod-uid-1","namespace":"default","ownerReferences":[{"kind":"ReplicaSet","name":"web-abc","uid":"rs-uid-1","controller":true}]}}`, &pod)

	reader := &stubReader{replicaSets: []k8sgateway.ReplicaSet{rs}, pods: []k8sgateway.Pod{pod}}
	collector := NewCollector(reader, 100)
	repo := &countingRepository{}
	svc := NewService(collector, repo, stubNamespaceLister{namespaces: namespaces}, WithNamespaceConcurrency(2))

	result, err := svc.CollectCluster(context.Background(), 1, now)
	if err != nil {
		t.Fatalf("CollectCluster failed: %v", err)
	}
	if result.Namespaces != len(namespaces) {
		t.Errorf("namespaces = %d, want %d", result.Namespaces, len(namespaces))
	}
	if result.TotalSeen != len(namespaces) {
		t.Errorf("total_seen = %d, want %d (one edge per namespace)", result.TotalSeen, len(namespaces))
	}
	if result.TotalUpserted != len(namespaces) {
		t.Errorf("total_upserted = %d, want %d", result.TotalUpserted, len(namespaces))
	}
	if repo.upsertCount.Load() != int64(len(namespaces)) {
		t.Errorf("repository upsert calls = %d, want %d", repo.upsertCount.Load(), len(namespaces))
	}
	if result.Partial || len(result.Errors) != 0 {
		t.Errorf("unexpected partial result: partial=%v errors=%v", result.Partial, result.Errors)
	}
}

// TestServiceCollectCluster_PartialFailure verifies namespace-level failures
// are aggregated and flagged partial while the remaining namespaces still run.
func TestServiceCollectCluster_PartialFailure(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	namespaces := []string{"good", "bad"}

	// The stub fails every resource read, so "bad" namespace collection errors.
	reader := &stubReader{err: errors.New("cluster unreachable")}
	collector := NewCollector(reader, 100)
	svc := NewService(collector, &countingRepository{}, stubNamespaceLister{namespaces: namespaces}, WithNamespaceConcurrency(2))

	result, err := svc.CollectCluster(context.Background(), 1, now)
	if err != nil {
		t.Fatalf("CollectCluster returned error: %v", err)
	}
	if !result.Partial {
		t.Error("expected partial=true when a namespace fails")
	}
	if len(result.Errors) != 2 {
		t.Errorf("errors = %d, want 2 (one per namespace)", len(result.Errors))
	}
	if result.TotalSeen != 0 {
		t.Errorf("total_seen = %d, want 0 (all reads failed)", result.TotalSeen)
	}
}

// TestCollectorSnapshot_Concurrent verifies the snapshot fetch is safe under
// the race detector and returns all eight kinds.
func TestCollectorSnapshot_Concurrent(t *testing.T) {
	var dep k8sgateway.Deployment
	mustDecodeJSON(t, `{"metadata":{"name":"web","uid":"dep-uid-1","namespace":"default"}}`, &dep)
	reader := &stubReader{
		deployments:    []k8sgateway.Deployment{dep},
		replicaSets:    []k8sgateway.ReplicaSet{{}},
		pods:           []k8sgateway.Pod{{}},
		services:       []k8sgateway.ServiceResource{{}},
		ingresses:      []k8sgateway.Ingress{{}},
		endpointSlices: []k8sgateway.EndpointSlice{{}},
		hpas:           []k8sgateway.HorizontalPodAutoscaler{{}},
		pdbs:           []k8sgateway.PodDisruptionBudget{{}},
	}
	collector := NewCollector(reader, 100)
	snap, err := collector.Snapshot(context.Background(), 1, "default")
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snap.Deployments) != 1 || len(snap.ReplicaSets) != 1 || len(snap.Pods) != 1 ||
		len(snap.Services) != 1 || len(snap.Ingresses) != 1 || len(snap.EndpointSlices) != 1 ||
		len(snap.HPAs) != 1 || len(snap.PDBs) != 1 {
		t.Errorf("snapshot did not capture every kind: %+v", snap)
	}
}

// gateReader wraps a stubReader and gates every Pods call so a test can
// observe how many namespace collections run in flight. It is used to verify
// the worker-pool concurrency bound.
type gateReader struct {
	inner     *stubReader
	entered   chan struct{}
	release   chan struct{}
	inFlight  *atomic.Int64
	maxFlight *atomic.Int64
}

func (g *gateReader) Deployments(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return g.inner.Deployments(ctx, clusterID, namespace, q)
}
func (g *gateReader) ReplicaSets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ReplicaSet], error) {
	return g.inner.ReplicaSets(ctx, clusterID, namespace, q)
}
func (g *gateReader) Pods(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	in := g.inFlight.Add(1)
	for {
		cur := g.maxFlight.Load()
		if in <= cur || g.maxFlight.CompareAndSwap(cur, in) {
			break
		}
	}
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-g.release
	defer g.inFlight.Add(-1)
	return g.inner.Pods(ctx, clusterID, namespace, q)
}
func (g *gateReader) Services(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error) {
	return g.inner.Services(ctx, clusterID, namespace, q)
}
func (g *gateReader) Ingresses(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error) {
	return g.inner.Ingresses(ctx, clusterID, namespace, q)
}
func (g *gateReader) EndpointSlices(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.EndpointSlice], error) {
	return g.inner.EndpointSlices(ctx, clusterID, namespace, q)
}
func (g *gateReader) HorizontalPodAutoscalers(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.HorizontalPodAutoscaler], error) {
	return g.inner.HorizontalPodAutoscalers(ctx, clusterID, namespace, q)
}
func (g *gateReader) PodDisruptionBudgets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
	return g.inner.PodDisruptionBudgets(ctx, clusterID, namespace, q)
}

// TestServiceCollectCluster_ConcurrencyBounded verifies that at most the
// configured number of namespace collections run at the same time.
func TestServiceCollectCluster_ConcurrencyBounded(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	namespaces := []string{"ns-1", "ns-2", "ns-3", "ns-4", "ns-5", "ns-6", "ns-7", "ns-8"}
	reader := &gateReader{
		inner:     &stubReader{},
		entered:   make(chan struct{}, 8),
		release:   make(chan struct{}),
		inFlight:  &atomic.Int64{},
		maxFlight: &atomic.Int64{},
	}
	collector := NewCollector(reader, 100)
	svc := NewService(collector, &countingRepository{}, stubNamespaceLister{namespaces: namespaces}, WithNamespaceConcurrency(3))

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.CollectCluster(context.Background(), 1, now)
	}()

	// Wait until the pool is saturated: with concurrency 3 the max in-flight
	// must never exceed 3.
	deadline := time.After(5 * time.Second)
	for {
		if reader.maxFlight.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("worker pool never saturated")
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := reader.maxFlight.Load(); got > 3 {
		t.Errorf("max in-flight = %d, want <= 3", got)
	}
	close(reader.release)
	<-done
}
