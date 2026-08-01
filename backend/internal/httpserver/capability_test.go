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

	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/requestctx"
)

// stubCapabilityMetrics is a controllable MetricsProvider for handler tests.
type stubCapabilityMetrics struct {
	result capability.MetricsResult
	err    error
	query  capability.MetricsQuery
}

func (m *stubCapabilityMetrics) QueryMetrics(_ context.Context, query capability.MetricsQuery) (capability.MetricsResult, error) {
	m.query = query
	if m.err != nil {
		return capability.MetricsResult{}, m.err
	}
	return m.result, nil
}

func (m *stubCapabilityMetrics) Name() string { return "stub-metrics" }

// stubCapabilityLogs is a controllable LogProvider for handler tests.
type stubCapabilityLogs struct {
	result capability.LogResult
	err    error
	query  capability.LogQuery
}

func (m *stubCapabilityLogs) QueryLogs(_ context.Context, query capability.LogQuery) (capability.LogResult, error) {
	m.query = query
	if m.err != nil {
		return capability.LogResult{}, m.err
	}
	return m.result, nil
}

func (m *stubCapabilityLogs) Name() string { return "stub-logs" }

func newCapabilityRouter(handler capabilityHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{ActorID: 1, RequestID: "capability-test"}))
		c.Next()
	})
	router.GET("/api/v1/capability/metrics", handler.queryMetrics)
	router.POST("/api/v1/capability/logs", handler.queryLogs)
	return router
}

const (
	capTestStart = "2026-01-01T00:00:00Z"
	capTestEnd   = "2026-01-01T00:01:00Z"
)

func TestCapabilityMetricsNilProviderReturns503(t *testing.T) {
	router := newCapabilityRouter(capabilityHandler{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capability/metrics?cluster_id=1&namespace=default&template=request_rate&start="+capTestStart+"&end="+capTestEnd+"&step=30s", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !containsCode(recorder.Body.String(), "CAPABILITY_UNAVAILABLE") {
		t.Fatalf("body = %q, want code CAPABILITY_UNAVAILABLE", recorder.Body.String())
	}
}

func TestCapabilityLogsNilProviderReturns503(t *testing.T) {
	router := newCapabilityRouter(capabilityHandler{})
	body, _ := json.Marshal(map[string]any{"cluster_id": 1, "namespace": "default", "start": capTestStart, "end": capTestEnd})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/capability/logs", bytes.NewReader(body)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !containsCode(recorder.Body.String(), "CAPABILITY_UNAVAILABLE") {
		t.Fatalf("body = %q, want code CAPABILITY_UNAVAILABLE", recorder.Body.String())
	}
}

func TestCapabilityMetricsValidReturns200(t *testing.T) {
	provider := &stubCapabilityMetrics{result: capability.MetricsResult{State: capability.StateComplete, Template: capability.TemplateRequestRate, SchemaVersion: capability.MetricsSchemaVersion}}
	router := newCapabilityRouter(capabilityHandler{metricsProvider: provider})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capability/metrics?cluster_id=1&namespace=default&service=api&template=request_rate&start="+capTestStart+"&end="+capTestEnd+"&step=30s", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result capability.MetricsResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != capability.StateComplete || result.Template != capability.TemplateRequestRate {
		t.Fatalf("result = %#v", result)
	}
	if provider.query.ClusterID != 1 || provider.query.Namespace != "default" || provider.query.ServiceName != "api" || provider.query.Step != 30*time.Second {
		t.Fatalf("provider query = %#v", provider.query)
	}
}

func TestCapabilityLogsValidReturns200(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete, TotalReturned: 1}}
	router := newCapabilityRouter(capabilityHandler{logProvider: provider})
	body, _ := json.Marshal(map[string]any{
		"cluster_id": 1, "namespace": "default", "pod": "api-0", "container": "api",
		"text_filter": "error", "start": capTestStart, "end": capTestEnd, "direction": "forward", "limit": 100,
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/capability/logs", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result capability.LogResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TotalReturned != 1 {
		t.Fatalf("total = %d", result.TotalReturned)
	}
	if provider.query.ClusterID != 1 || provider.query.PodName != "api-0" || provider.query.TextFilter != "error" || provider.query.Direction != capability.DirectionForward {
		t.Fatalf("provider query = %#v", provider.query)
	}
}

func TestCapabilityMetricsMissingRequiredParamsReturns400(t *testing.T) {
	provider := &stubCapabilityMetrics{}
	router := newCapabilityRouter(capabilityHandler{metricsProvider: provider})
	tests := []struct {
		name  string
		query string
	}{
		{"missing cluster_id", "namespace=default&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd + "&step=30s"},
		{"invalid cluster_id", "cluster_id=0&namespace=default&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd + "&step=30s"},
		{"missing namespace", "cluster_id=1&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd + "&step=30s"},
		{"missing template", "cluster_id=1&namespace=default&start=" + capTestStart + "&end=" + capTestEnd + "&step=30s"},
		{"unsupported template", "cluster_id=1&namespace=default&template=bogus&start=" + capTestStart + "&end=" + capTestEnd + "&step=30s"},
		{"missing start", "cluster_id=1&namespace=default&template=request_rate&end=" + capTestEnd + "&step=30s"},
		{"missing end", "cluster_id=1&namespace=default&template=request_rate&start=" + capTestStart + "&step=30s"},
		{"missing step", "cluster_id=1&namespace=default&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd},
		{"invalid step", "cluster_id=1&namespace=default&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd + "&step=abc"},
		{"non-positive step", "cluster_id=1&namespace=default&template=request_rate&start=" + capTestStart + "&end=" + capTestEnd + "&step=0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capability/metrics?"+tc.query, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestCapabilityLogsRejectsInvalidBody(t *testing.T) {
	provider := &stubCapabilityLogs{}
	router := newCapabilityRouter(capabilityHandler{logProvider: provider})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/capability/logs", bytes.NewReader([]byte("not-json"))))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCapabilityMetricsMapsProviderValidationError(t *testing.T) {
	provider := &stubCapabilityMetrics{err: capability.ErrInvalidMetricsQuery}
	router := newCapabilityRouter(capabilityHandler{metricsProvider: provider})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capability/metrics?cluster_id=1&namespace=default&template=request_rate&start="+capTestStart+"&end="+capTestEnd+"&step=30s", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCapabilityLogsDefaultsForwardDirection(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete}}
	router := newCapabilityRouter(capabilityHandler{logProvider: provider})
	body, _ := json.Marshal(map[string]any{"cluster_id": 1, "namespace": "default", "start": capTestStart, "end": capTestEnd})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/capability/logs", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.query.Direction != capability.DirectionForward {
		t.Fatalf("direction = %q, want %q", provider.query.Direction, capability.DirectionForward)
	}
}
