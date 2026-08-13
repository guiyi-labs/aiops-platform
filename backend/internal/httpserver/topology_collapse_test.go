package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/topology"
)

func TestCollapseEdges(t *testing.T) {
	mk := func(uid string, kind topology.EdgeKind) topology.Edge {
		return topology.Edge{
			Kind:      kind,
			Source:    topology.ResourceCitation{Kind: "Service", Namespace: "demo", Name: "svc", UID: "svc-1"},
			Target:    topology.ResourceCitation{Kind: "Pod", Namespace: "demo", Name: "pod-" + uid, UID: "pod-" + uid},
			ValidFrom: time.Now(),
		}
	}
	edges := []topology.Edge{
		mk("a", topology.EdgeRoutesTo),
		mk("a", topology.EdgeRoutesTo), // duplicate pair+kind
		mk("b", topology.EdgeRoutesTo),
		mk("b", topology.EdgeBackedBy),
	}
	got := collapseEdges(edges)
	if len(got) != 3 {
		t.Fatalf("expected 3 collapsed edges, got %d", len(got))
	}
	// The duplicate pair+kind must collapse into one with AggregateCount == 2.
	found := false
	for _, e := range got {
		if e.Target.UID == "pod-a" && e.Kind == topology.EdgeRoutesTo {
			found = true
			if e.AggregateCount != 2 {
				t.Errorf("collapsed edge AggregateCount = %d, want 2", e.AggregateCount)
			}
		}
	}
	if !found {
		t.Error("collapsed duplicate edge not found")
	}
}

func TestCollapseEdges_Empty(t *testing.T) {
	if got := collapseEdges(nil); len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestTopologyGraph_CollapseQueryParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := topologyHandler{service: nil} // service nil -> 503; collapse parse still exercised via handler wiring below

	// Use a stub service? Simpler: exercise collapseEdges directly is covered above.
	// Here we only assert that the handler exists and binds collapse=1 as non-erroring path setup.
	api := r.Group("/api/v1/aiops/topology")
	api.GET("/graph", h.getTopologyGraph)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1&namespace=demo&collapse=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no topology service in test), got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_JSONAggregateCount(t *testing.T) {
	// Prove the AggregateCount field survives JSON round-trip on the API
	// projection so collapsed graphs are visible to the frontend.
	e := topology.Edge{Kind: topology.EdgeRoutesTo, AggregateCount: 3}
	e.Source = topology.ResourceCitation{Kind: "Service", Name: "svc", UID: "u1"}
	e.Target = topology.ResourceCitation{Kind: "Pod", Name: "pod", UID: "u2"}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["aggregate_count"] != float64(3) {
		t.Fatalf("aggregate_count missing: %v", back)
	}
}

// --- M109: topology handler validation + success branches ---

type topologyRepoStub struct {
	edges      []topology.Edge
	edgeTotal  int64
	listErr    error
	events     []topology.ChangeEvent
	eventTotal int64
	eventsErr  error
}

func (s *topologyRepoStub) UpsertEdge(context.Context, *topology.Edge) error { return nil }
func (s *topologyRepoStub) CloseEdge(context.Context, int64, topology.EdgeKind, string, string, topology.DerivationMethod, time.Time) error {
	return nil
}
func (s *topologyRepoStub) ListEdges(_ context.Context, _ topology.EdgeFilter) ([]topology.Edge, int64, error) {
	return s.edges, s.edgeTotal, s.listErr
}
func (s *topologyRepoStub) UpsertChangeEvent(context.Context, *topology.ChangeEvent) error {
	return nil
}
func (s *topologyRepoStub) ListChangeEvents(_ context.Context, _ topology.ChangeTimelineFilter) ([]topology.ChangeEvent, int64, error) {
	return s.events, s.eventTotal, s.eventsErr
}

func newTopologyRouter(stub *topologyRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := topology.NewService(nil, stub, nil)
	h := topologyHandler{service: svc}
	r := gin.New()
	api := r.Group("/api/v1/aiops/topology")
	api.GET("/graph", h.getTopologyGraph)
	api.GET("/changes", h.listChangeEvents)
	return r
}

func TestTopologyGraph_MissingClusterID(t *testing.T) {
	// service must be non-nil so validation branches execute
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "cluster_id is required") {
		t.Fatalf("expected 400 cluster_id required, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_BadClusterID(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "must be a positive integer") {
		t.Fatalf("expected 400 invalid cluster_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_MissingNamespace(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "namespace is required") {
		t.Fatalf("expected 400 namespace required, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_BadLimit(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1&namespace=demo&limit=nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "limit must be a positive integer") {
		t.Fatalf("expected 400 invalid limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_NegativeLimit(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1&namespace=demo&limit=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "limit must be a positive integer") {
		t.Fatalf("expected 400 negative limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_ServiceError(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{listErr: errors.New("repo down")})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1&namespace=demo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "TOPOLOGY_QUERY_FAILED") {
		t.Fatalf("expected 500 TOPOLOGY_QUERY_FAILED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyGraph_SuccessCollapse(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{edgeTotal: 1, edges: []topology.Edge{
		{Kind: topology.EdgeRoutesTo, Source: topology.ResourceCitation{UID: "a"}, Target: topology.ResourceCitation{UID: "b"}},
		{Kind: topology.EdgeRoutesTo, Source: topology.ResourceCitation{UID: "a"}, Target: topology.ResourceCitation{UID: "b"}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/graph?cluster_id=1&namespace=demo&collapse=1&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !contains(w.Body.String(), `"aggregate_count":2`) {
		t.Fatalf("expected 200 with collapsed count 2, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_MissingClusterID(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "cluster_id is required") {
		t.Fatalf("expected 400 cluster_id required, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_BadClusterID(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "must be a positive integer") {
		t.Fatalf("expected 400 invalid cluster_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_BadStartTime(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=1&start=bad-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "start must be RFC3339") {
		t.Fatalf("expected 400 bad start, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_BadEndTime(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=1&end=bad-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "end must be RFC3339") {
		t.Fatalf("expected 400 bad end, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_BadLimit(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=1&limit=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "limit must be a positive integer") {
		t.Fatalf("expected 400 invalid limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_Success(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{eventTotal: 2, events: []topology.ChangeEvent{
		{ID: 1, ClusterID: 1, Kind: "promotion"},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=1&namespace=demo&kind=promotion&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !contains(w.Body.String(), `"total":2`) {
		t.Fatalf("expected 200 with total 2, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTopologyChanges_ServiceError(t *testing.T) {
	r := newTopologyRouter(&topologyRepoStub{eventsErr: errors.New("repo down")})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/topology/changes?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "TOPOLOGY_QUERY_FAILED") {
		t.Fatalf("expected 500 TOPOLOGY_QUERY_FAILED, got %d: %s", w.Code, w.Body.String())
	}
}
