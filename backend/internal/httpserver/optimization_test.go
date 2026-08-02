package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	collector := optimization.NewCollector(lister, nil, nil)
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

// TestOptimizationNetworkAnalyzeWithSuppliedBundle exercises the M67 network
// endpoint with a caller-supplied observation bundle and no collector wired.
func TestOptimizationNetworkAnalyzeWithSuppliedBundle(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/network/analyze", map[string]any{
		"cluster_id": 7,
		"pods": []map[string]any{
			{"namespace": "shop", "name": "web-1", "labels": map[string]string{"app": "web"}},
		},
		"services": []map[string]any{
			{"namespace": "shop", "name": "web", "selector": map[string]string{"app": "gone"}, "ports": []map[string]any{{"port": 80}}},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Failed     int            `json:"failed"`
		BySeverity map[string]int `json:"by_severity"`
		Findings   []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode network status: %v", err)
	}
	if status.Failed < 1 {
		t.Fatalf("expected findings for the backend-less service, got %d", status.Failed)
	}
	var sawNoBackends bool
	for _, f := range status.Findings {
		if f.Code == "NETPOL_SERVICE_NO_BACKENDS" {
			sawNoBackends = true
		}
	}
	if !sawNoBackends {
		t.Fatalf("expected NETPOL_SERVICE_NO_BACKENDS, got %+v", status.Findings)
	}
	if status.BySeverity["critical"] < 1 {
		t.Fatalf("severity rollup missing the critical finding: %v", status.BySeverity)
	}
}

// TestOptimizationNetworkAnalyzeAutoCollect verifies the M65 auto-collection
// path for the network endpoint: an empty bundle triggers a read-only scan and
// the exposed NodePort service without any policy is reported.
func TestOptimizationNetworkAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/api/v1/namespaces": {json.RawMessage(`{"metadata":{"name":"shop"}}`)},
		"/api/v1/pods": {
			json.RawMessage(`{"metadata":{"namespace":"shop","name":"web-1","labels":{"app":"web"}},"spec":{"containers":[{"name":"c","ports":[{"containerPort":8080}]}]}}`),
		},
		"/api/v1/services": {
			json.RawMessage(`{"metadata":{"namespace":"shop","name":"web"},"spec":{"type":"NodePort","selector":{"app":"web"},"ports":[{"port":80,"targetPort":8080,"nodePort":31080}]}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/network/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		PodsTotal       int `json:"pods_total"`
		ExposedServices int `json:"exposed_services"`
		Findings        []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode network status: %v", err)
	}
	if status.PodsTotal != 1 || status.ExposedServices != 1 {
		t.Fatalf("auto-collected inventory wrong: pods=%d exposed=%d", status.PodsTotal, status.ExposedServices)
	}
	var sawExposure bool
	for _, f := range status.Findings {
		if f.Code == "NETPOL_EXPOSED_SERVICE_UNRESTRICTED" {
			sawExposure = true
		}
	}
	if !sawExposure {
		t.Fatalf("expected NETPOL_EXPOSED_SERVICE_UNRESTRICTED from the auto-collected NodePort service, got %+v", status.Findings)
	}
}

func TestOptimizationNetworkAnalyzeRejectsMissingClusterAndInputs(t *testing.T) {
	engine := newOptimizationEngine(t)

	rec := postJSON(t, engine, "/api/v1/optimization/network/analyze", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id; body=%s", rec.Code, rec.Body.String())
	}

	// Without a collector an empty bundle is unanalysable and must be
	// rejected explicitly rather than returning a misleading clean report.
	rec = postJSON(t, engine, "/api/v1/optimization/network/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty bundle without auto-collection; body=%s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Code != "NO_INPUTS" {
		t.Fatalf("error code = %q, want NO_INPUTS; body=%s", errBody.Code, rec.Body.String())
	}
}

// TestOptimizationImageAnalyzeWithSuppliedBundle exercises the M68 image
// endpoint with a caller-supplied observation bundle and no collector wired.
func TestOptimizationImageAnalyzeWithSuppliedBundle(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/image/analyze", map[string]any{
		"cluster_id": 7,
		"usages": []map[string]any{
			{
				"image":     map[string]any{"repository": "registry.io/team/api", "tag": "latest", "pull_policy": "Always"},
				"container": map[string]any{"namespace": "shop", "workload_kind": "Deployment", "workload_name": "api", "container": "app"},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Failed           int            `json:"failed"`
		ImagesTotal      int            `json:"images_total"`
		MutableTagImages int            `json:"mutable_tag_images"`
		BySeverity       map[string]int `json:"by_severity"`
		Findings         []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode image status: %v", err)
	}
	if status.ImagesTotal != 1 || status.MutableTagImages != 1 {
		t.Fatalf("inventory wrong: images=%d mutable=%d", status.ImagesTotal, status.MutableTagImages)
	}
	var sawMutableTag bool
	for _, f := range status.Findings {
		if f.Code == "IMG_MUTABLE_TAG" {
			sawMutableTag = true
		}
	}
	if !sawMutableTag {
		t.Fatalf("expected IMG_MUTABLE_TAG, got %+v", status.Findings)
	}
	if status.BySeverity["warning"] < 1 {
		t.Fatalf("severity rollup missing the warning finding: %v", status.BySeverity)
	}
}

// TestOptimizationImageAnalyzeAutoCollect verifies the M65 auto-collection
// path for the image endpoint: an empty bundle triggers a read-only workload
// scan and the :latest reference is reported.
func TestOptimizationImageAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {
			json.RawMessage(`{"metadata":{"namespace":"shop","name":"api"},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"registry.io/team/api:latest","imagePullPolicy":"Always"}]}}}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/image/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		ImagesTotal     int `json:"images_total"`
		ContainersTotal int `json:"containers_total"`
		Findings        []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode image status: %v", err)
	}
	if status.ImagesTotal != 1 || status.ContainersTotal != 1 {
		t.Fatalf("auto-collected inventory wrong: images=%d containers=%d", status.ImagesTotal, status.ContainersTotal)
	}
	var sawMutableTag, sawPullAlways bool
	for _, f := range status.Findings {
		switch f.Code {
		case "IMG_MUTABLE_TAG":
			sawMutableTag = true
		case "IMG_PULL_ALWAYS_LATEST":
			sawPullAlways = true
		}
	}
	if !sawMutableTag || !sawPullAlways {
		t.Fatalf("expected mutable-tag and pull-always findings from the auto-collected deployment, got %+v", status.Findings)
	}
}

func TestOptimizationImageAnalyzeRejectsMissingClusterAndInputs(t *testing.T) {
	engine := newOptimizationEngine(t)

	rec := postJSON(t, engine, "/api/v1/optimization/image/analyze", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id; body=%s", rec.Code, rec.Body.String())
	}

	// Without a collector an empty bundle is unanalysable and must be
	// rejected explicitly rather than returning a misleading clean report.
	rec = postJSON(t, engine, "/api/v1/optimization/image/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty bundle without auto-collection; body=%s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Code != "NO_INPUTS" {
		t.Fatalf("error code = %q, want NO_INPUTS; body=%s", errBody.Code, rec.Body.String())
	}
}

// TestOptimizationCapacityAnalyzeExplicitBundle feeds a rising CPU utilization
// series (50% -> 80% over six days) and expects the handler to project
// saturation inside the critical window.
func TestOptimizationCapacityAnalyzeExplicitBundle(t *testing.T) {
	engine := newOptimizationEngine(t)
	now := time.Now().UTC()
	sample := func(daysAgo int, value float64) map[string]any {
		return map[string]any{
			"timestamp": now.Add(-time.Duration(daysAgo) * 24 * time.Hour).Format(time.RFC3339Nano),
			"value":     value,
		}
	}
	rec := postJSON(t, engine, "/api/v1/optimization/capacity/analyze", map[string]any{
		"cluster_id": 7,
		"cpu": map[string]any{
			"capacity": 1000,
			"samples":  []map[string]any{sample(6, 500), sample(3, 650), sample(0, 800)},
		},
		"memory": map[string]any{"capacity": 2000},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Total                int     `json:"total"`
		Failed               int     `json:"failed"`
		CPUCapacityNanocores int64   `json:"cpu_capacity_nanocores"`
		CPUCurrentPct        float64 `json:"cpu_current_pct"`
		CPUSaturationInDays  float64 `json:"cpu_saturation_in_days"`
		Findings             []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode capacity status: %v", err)
	}
	if status.Total != 2 {
		t.Fatalf("total = %d, want 2 (cpu + memory)", status.Total)
	}
	if status.CPUCapacityNanocores != 1000 {
		t.Fatalf("cpu_capacity_nanocores = %d, want 1000", status.CPUCapacityNanocores)
	}
	if status.CPUCurrentPct < 0.79 || status.CPUCurrentPct > 0.81 {
		t.Fatalf("cpu_current_pct = %v, want ~0.80", status.CPUCurrentPct)
	}
	if status.CPUSaturationInDays < 3.5 || status.CPUSaturationInDays > 4.5 {
		t.Fatalf("cpu_saturation_in_days = %v, want ~4", status.CPUSaturationInDays)
	}
	if status.Failed != 1 {
		t.Fatalf("failed = %d, want 1 (cpu only)", status.Failed)
	}
	if len(status.Findings) != 1 || status.Findings[0].Code != "CAPACITY_SATURATION_RISK" {
		t.Fatalf("findings = %+v, want one CAPACITY_SATURATION_RISK", status.Findings)
	}
	if status.Findings[0].Severity != "critical" {
		t.Fatalf("severity = %q, want critical (saturates within 7 days)", status.Findings[0].Severity)
	}
}

// TestOptimizationCapacityAnalyzeAutoCollect verifies the collector path sums
// node allocatable capacity. With no usage source wired the bundle carries no
// samples, so no trend can be fitted and no finding is produced.
func TestOptimizationCapacityAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/api/v1/nodes": {
			json.RawMessage(`{"metadata":{"name":"node-a","uid":"n1"},"status":{"allocatable":{"cpu":"4","memory":"8Gi"}}}`),
			json.RawMessage(`{"metadata":{"name":"node-b","uid":"n2"},"status":{"allocatable":{"cpu":"4","memory":"8Gi"}}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/capacity/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		CPUCapacityNanocores int64                    `json:"cpu_capacity_nanocores"`
		MemCapacityBytes     int64                    `json:"mem_capacity_bytes"`
		Failed               int                      `json:"failed"`
		Findings             []map[string]interface{} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode capacity status: %v", err)
	}
	if status.CPUCapacityNanocores != 8_000_000_000 {
		t.Fatalf("cpu_capacity_nanocores = %d, want 8000000000 (2 x 4 cores)", status.CPUCapacityNanocores)
	}
	if status.MemCapacityBytes != 17_179_869_184 {
		t.Fatalf("mem_capacity_bytes = %d, want 17179869184 (2 x 8Gi)", status.MemCapacityBytes)
	}
	if status.Failed != 0 {
		t.Fatalf("failed = %d, want 0 without a usage series", status.Failed)
	}
	if status.Findings == nil {
		t.Fatal("findings must serialize as [] rather than null")
	}
}

func TestOptimizationCapacityAnalyzeRejectsMissingClusterAndInputs(t *testing.T) {
	engine := newOptimizationEngine(t)

	rec := postJSON(t, engine, "/api/v1/optimization/capacity/analyze", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id; body=%s", rec.Code, rec.Body.String())
	}

	rec = postJSON(t, engine, "/api/v1/optimization/capacity/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty bundle without auto-collection; body=%s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Code != "NO_INPUTS" {
		t.Fatalf("error code = %q, want NO_INPUTS; body=%s", errBody.Code, rec.Body.String())
	}
}

func TestOptimizationGitOpsAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/api/v1/namespaces": {
			json.RawMessage(`{"metadata":{"name":"gitops","annotations":{"kustomize.toolkit.fluxcd.io/name":"app"}}}`),
		},
		"/apis/apps/v1/deployments": {
			json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{
				"namespace":"gitops","name":"api","uid":"u1",
				"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"name\":\"api\"},\"spec\":{\"replicas\":3}}"}},"spec":{"replicas":5}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/gitops/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		ResourcesTotal     int `json:"resources_total"`
		DriftedResources   int `json:"drifted_resources"`
		UnmanagedResources int `json:"unmanaged_resources"`
		Findings           []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode gitops status: %v", err)
	}
	if status.ResourcesTotal != 1 {
		t.Fatalf("resources_total = %d, want 1", status.ResourcesTotal)
	}
	if status.DriftedResources != 1 {
		t.Fatalf("drifted_resources = %d, want 1", status.DriftedResources)
	}
	var sawDrift bool
	for _, f := range status.Findings {
		if f.Code == "GITOPS_DRIFT_DETECTED" {
			sawDrift = true
		}
	}
	if !sawDrift {
		t.Fatalf("expected GITOPS_DRIFT_DETECTED, got %+v", status.Findings)
	}
}

func TestOptimizationGitOpsAnalyzeRejectsMissingClusterAndInputs(t *testing.T) {
	engine := newOptimizationEngine(t)

	rec := postJSON(t, engine, "/api/v1/optimization/gitops/analyze", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id; body=%s", rec.Code, rec.Body.String())
	}

	rec = postJSON(t, engine, "/api/v1/optimization/gitops/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty bundle without auto-collection; body=%s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Code != "NO_INPUTS" {
		t.Fatalf("error code = %q, want NO_INPUTS; body=%s", errBody.Code, rec.Body.String())
	}
}

// TestOptimizationPolicyAnalyzeExplicitBundle feeds a workload with no
// requests/limits/probes and expects the container-level findings.
func TestOptimizationPolicyAnalyzeExplicitBundle(t *testing.T) {
	engine := newOptimizationEngine(t)
	rec := postJSON(t, engine, "/api/v1/optimization/policy/analyze", map[string]any{
		"cluster_id": 7,
		"workloads": []map[string]any{{
			"kind": "Deployment", "namespace": "prod", "name": "web",
			"containers": []map[string]any{{"name": "app"}},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		WorkloadsTotal     int `json:"workloads_total"`
		ContainersTotal    int `json:"containers_total"`
		Failed             int `json:"failed"`
		CompliantWorkloads int `json:"compliant_workloads"`
		Findings           []struct {
			Code string `json:"code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode policy status: %v", err)
	}
	if status.WorkloadsTotal != 1 || status.ContainersTotal != 1 {
		t.Fatalf("counters = %d/%d, want 1/1", status.WorkloadsTotal, status.ContainersTotal)
	}
	if status.Failed != 8 { // no cpu req, no mem req, no limits, esc, runasroot, no liveness, no readiness, no startup
		t.Fatalf("failed = %d, want 8; findings=%d", status.Failed, len(status.Findings))
	}
	if status.CompliantWorkloads != 0 {
		t.Fatalf("compliant_workloads = %d, want 0", status.CompliantWorkloads)
	}
	sawCPURequest := false
	for _, f := range status.Findings {
		if f.Code == "POLICY_CONTAINER_NO_CPU_REQUEST" {
			sawCPURequest = true
		}
	}
	if !sawCPURequest {
		t.Fatalf("findings = %+v, want a cpu-request finding", status.Findings)
	}
}

// TestOptimizationPolicyAnalyzeAutoCollect exercises the collector path: a
// Deployment whose container is missing everything yields findings; the
// controller pod template is what is scanned.
func TestOptimizationPolicyAnalyzeAutoCollect(t *testing.T) {
	lister := fakeClusterLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {
			json.RawMessage(`{"metadata":{"namespace":"prod","name":"web","uid":"u-web"},"spec":{"template":{"spec":{"containers":[{"name":"app"}]}}}}`),
		},
	}}
	engine := newOptimizationEngineWithCollector(t, lister)
	rec := postJSON(t, engine, "/api/v1/optimization/policy/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		WorkloadsTotal  int `json:"workloads_total"`
		ContainersTotal int `json:"containers_total"`
		Failed          int `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode policy status: %v", err)
	}
	if status.WorkloadsTotal != 1 || status.ContainersTotal != 1 {
		t.Fatalf("counters = %d/%d, want 1/1", status.WorkloadsTotal, status.ContainersTotal)
	}
	if status.Failed != 8 {
		t.Fatalf("failed = %d, want 8", status.Failed)
	}
}

func TestOptimizationPolicyAnalyzeRejectsMissingClusterAndInputs(t *testing.T) {
	engine := newOptimizationEngine(t)

	rec := postJSON(t, engine, "/api/v1/optimization/policy/analyze", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id; body=%s", rec.Code, rec.Body.String())
	}

	rec = postJSON(t, engine, "/api/v1/optimization/policy/analyze", map[string]any{"cluster_id": 7})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty bundle without auto-collection; body=%s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Code != "NO_INPUTS" {
		t.Fatalf("error code = %q, want NO_INPUTS; body=%s", errBody.Code, rec.Body.String())
	}
}
