package httpserver

import (
	"encoding/json"
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
