package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/optimization"
)

// newOptimizationEngine builds a gin engine with the M64 optimization routes
// registered. Auth is left nil so requests pass straight to the handlers;
// these tests exercise handler logic, not the auth middleware.
func newOptimizationEngine(t *testing.T) *gin.Engine {
	t.Helper()
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:        probeStub{},
		Optimization: optimization.NewService(finops.DefaultCostRate(), nil),
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}
	return engine
}

func postJSON(t *testing.T, engine *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestOptimizationCISAnalyze(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/cis/analyze", map[string]any{
		"cluster_id": 7,
		"workloads": []map[string]any{
			{
				"kind":      "Pod",
				"namespace": "default",
				"name":      "privileged-pod",
				"containers": []map[string]any{
					{"name": "c", "privileged": true},
				},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Failed   int `json:"failed"`
		Findings []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode CIS status: %v", err)
	}
	if status.Failed < 1 {
		t.Fatalf("expected at least one CIS finding for a privileged container, got %d", status.Failed)
	}
}

func TestOptimizationFinOpsAnalyze(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/finops/analyze", map[string]any{
		"inputs": []map[string]any{
			{
				"cluster_id":     7,
				"namespace":      "default",
				"workload_kind":  "Deployment",
				"workload_name":  "api",
				"container_name": "c",
				"requests":       map[string]any{"cpu_request": 1000000000, "mem_request": 1073741824},
				"limits":         map[string]any{"cpu_limit": 1000000000, "mem_limit": 1073741824},
				"cpu_usage_p95":  100000000,
				"mem_usage_p95":  100000000,
				"replicas":       2,
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var summary struct {
		MonthlyWasteUSD float64 `json:"monthly_waste_usd"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode FinOps summary: %v", err)
	}
	if summary.MonthlyWasteUSD <= 0 {
		t.Fatalf("expected positive monthly waste for an over-provisioned container, got %v", summary.MonthlyWasteUSD)
	}
}

func TestOptimizationDeprecatedAPIAnalyze(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/deprecated-api/analyze", map[string]any{
		"cluster_id":     7,
		"target_version": "1.25",
		"objects": []map[string]any{
			{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "name": "legacy"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode deprecated-API status: %v", err)
	}
	if status.Removed < 1 {
		t.Fatalf("expected at least one removed-api finding for extensions/v1beta1 Ingress, got %d", status.Removed)
	}
}

func TestOptimizationAnalyzeValidation(t *testing.T) {
	engine := newOptimizationEngine(t)
	// Missing cluster_id must be rejected with 400.
	rec := postJSON(t, engine, "/api/v1/optimization/cis/analyze", map[string]any{
		"workloads": []map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing cluster_id", rec.Code)
	}
	// Missing target_version must be rejected with 400.
	rec = postJSON(t, engine, "/api/v1/optimization/deprecated-api/analyze", map[string]any{
		"cluster_id": 7,
		"objects":    []map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing target_version", rec.Code)
	}
}

// fakeClusterLister is an in-memory optimization.ClusterLister used to exercise
// the server-side auto-collection path without a real cluster. It returns the
// canned items registered for each path, or an empty list for unknown paths
// (mimicking a resource type that is not installed / not listable).
type fakeClusterLister struct {
	data map[string][]json.RawMessage
}

func (f fakeClusterLister) List(_ context.Context, _ int64, path string) ([]json.RawMessage, error) {
	if items, ok := f.data[path]; ok {
		return items, nil
	}
	return []json.RawMessage{}, nil
}

// newOptimizationEngineWithCollector builds a gin engine whose optimization
// service has an auto-collector backed by the given fake lister (no metrics
// source, so FinOps degrades to request/limit-only collection).
func newOptimizationEngineWithCollector(t *testing.T, lister optimization.ClusterLister) *gin.Engine {
	t.Helper()
	collector := optimization.NewCollector(lister, nil)
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:        probeStub{},
		Optimization: optimization.NewService(finops.DefaultCostRate(), collector),
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}
	return engine
}

// TestOptimizationCISAnalyzeAutoCollect verifies that when the request body
// carries no observation bundle but the service has a collector, the handler
// auto-collects from the cluster and still returns findings.
func TestOptimizationCISAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/api/v1/pods": {
			json.RawMessage(`{"metadata":{"namespace":"default","name":"priv","uid":"uid-1"},"spec":{"containers":[{"name":"c","securityContext":{"privileged":true}}]}}`),
		},
		"/api/v1/namespaces": {
			json.RawMessage(`{"metadata":{"name":"default","uid":"ns-1","labels":{"pod-security.kubernetes.io/enforce":"privileged"}}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/cis/analyze", map[string]any{
		"cluster_id": 7,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode CIS status: %v", err)
	}
	if status.Failed < 1 {
		t.Fatalf("expected at least one CIS finding from auto-collected privileged pod, got %d", status.Failed)
	}
}

// TestOptimizationDeprecatedAPIAnalyzeAutoCollect verifies the same
// auto-collect wiring for the deprecated-API endpoint.
func TestOptimizationDeprecatedAPIAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {
			json.RawMessage(`{"apiVersion":"apps/v1beta1","kind":"Deployment","metadata":{"namespace":"default","name":"legacy","uid":"u1"}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/deprecated-api/analyze", map[string]any{
		"cluster_id":     7,
		"target_version": "1.25",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode deprecated-API status: %v", err)
	}
	if status.Removed < 1 {
		t.Fatalf("expected at least one removed-api finding from auto-collected apps/v1beta1 Deployment, got %d", status.Removed)
	}
}
