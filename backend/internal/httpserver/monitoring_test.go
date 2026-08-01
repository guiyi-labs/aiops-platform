package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/monitoring"
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/workspace"
)

// stubWorkspaceLister is a controllable monitoring.WorkspaceMembershipLister
// for handler-level tests. It mirrors the monitoring package's fakeLister but
// lives in the httpserver test package.
type stubWorkspaceLister struct {
	memberships []workspace.WorkspaceMembership
	err         error
}

func (s *stubWorkspaceLister) ListMemberships(_ context.Context, _ int64, _ []string, _ int64) ([]workspace.WorkspaceMembership, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.memberships, nil
}

const (
	monTestFrom = "2026-08-01T10:00:00Z"
	monTestTo   = "2026-08-01T11:00:00Z"
)

// newMonitoringRouter builds a test gin engine that wraps the monitoring
// handler with a middleware stub that populates requestctx.Metadata. The
// optional scopeSetter allows tests to inject a restrictive namespace scope
// (for the logs/query anti-leakage test).
func newMonitoringRouter(handler monitoringHandler, clusterID int64, scopeSetter func(c *gin.Context)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID:   1,
			Roles:     []string{auth.SystemAdmin},
			ClusterID: clusterID,
			RequestID: "mon-test",
		}))
		if scopeSetter != nil {
			scopeSetter(c)
		}
		c.Next()
	})
	router.GET("/api/v1/clusters/:cluster_id/monitoring/dashboard/:template", handler.clusterDashboard)
	router.GET("/api/v1/workspaces/:workspace_id/monitoring/dashboard", handler.workspaceDashboard)
	router.POST("/api/v1/clusters/:cluster_id/logs/query", handler.queryLogs)
	return router
}

// ============================================================================
// Cluster dashboard
// ============================================================================

func TestMonitoringClusterDashboard200(t *testing.T) {
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/1/monitoring/dashboard/node_overview?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var resp monitoring.ClusterDashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if resp.Template != monitoring.TemplateNodeOverview {
		t.Fatalf("template = %q, want %q", resp.Template, monitoring.TemplateNodeOverview)
	}
	if resp.ClusterID != 1 {
		t.Fatalf("cluster_id = %d, want 1", resp.ClusterID)
	}
	if len(resp.Panels) != 2 {
		t.Fatalf("panels = %d, want 2", len(resp.Panels))
	}
}

func TestMonitoringClusterDashboardInvalidTemplate400(t *testing.T) {
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/1/monitoring/dashboard/custom_promql?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "INVALID_TEMPLATE") {
		t.Fatalf("body = %q, want code INVALID_TEMPLATE", rec.Body.String())
	}
}

func TestMonitoringClusterDashboardInvalidWindow400(t *testing.T) {
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/1/monitoring/dashboard/node_overview?from="+monTestFrom+"&to="+monTestFrom, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "INVALID_WINDOW") {
		t.Fatalf("body = %q, want code INVALID_WINDOW", rec.Body.String())
	}
}

func TestMonitoringClusterDashboardMissingFrom400(t *testing.T) {
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/1/monitoring/dashboard/node_overview?to="+monTestTo, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestMonitoringClusterDashboardNilService503(t *testing.T) {
	router := newMonitoringRouter(monitoringHandler{}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/1/monitoring/dashboard/node_overview?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "MONITORING_UNAVAILABLE") {
		t.Fatalf("body = %q, want code MONITORING_UNAVAILABLE", rec.Body.String())
	}
}

// ============================================================================
// Workspace dashboard
// ============================================================================

func TestMonitoringWorkspaceDashboard200(t *testing.T) {
	lister := &stubWorkspaceLister{memberships: []workspace.WorkspaceMembership{
		{WorkspaceID: 1, ClusterID: 2, Namespace: "ns-a"},
		{WorkspaceID: 1, ClusterID: 1, Namespace: "ns-b"},
	}}
	svc := monitoring.NewService(monitoring.Config{}, nil, lister)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 0, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/1/monitoring/dashboard?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var resp monitoring.WorkspaceDashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if resp.Template != monitoring.TemplateWorkspaceOverview {
		t.Fatalf("template = %q, want %q", resp.Template, monitoring.TemplateWorkspaceOverview)
	}
	if len(resp.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(resp.Clusters))
	}
	// Clusters sorted ascending.
	if resp.Clusters[0].ClusterID != 1 || resp.Clusters[1].ClusterID != 2 {
		t.Fatalf("cluster order = %d %d, want 1 2", resp.Clusters[0].ClusterID, resp.Clusters[1].ClusterID)
	}
}

func TestMonitoringWorkspaceDashboardNotFound404(t *testing.T) {
	lister := &stubWorkspaceLister{err: workspace.ErrWorkspaceNotFound}
	svc := monitoring.NewService(monitoring.Config{}, nil, lister)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 0, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/999/monitoring/dashboard?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "WORKSPACE_NOT_FOUND") {
		t.Fatalf("body = %q, want code WORKSPACE_NOT_FOUND", rec.Body.String())
	}
}

func TestMonitoringWorkspaceDashboardNilService503(t *testing.T) {
	router := newMonitoringRouter(monitoringHandler{}, 0, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/1/monitoring/dashboard?from="+monTestFrom+"&to="+monTestTo, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// Logs query
// ============================================================================

func TestMonitoringLogsQuery200(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete, TotalReturned: 1}}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "pod": "api-0", "text_filter": "error",
		"start": monTestFrom, "end": monTestTo, "direction": "forward", "limit": 100,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var result capability.LogResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if result.State != capability.StateComplete {
		t.Fatalf("state = %q, want %q", result.State, capability.StateComplete)
	}
	if provider.query.ClusterID != 1 {
		t.Fatalf("provider query cluster_id = %d, want 1", provider.query.ClusterID)
	}
	if provider.query.Namespace != "default" {
		t.Fatalf("provider query namespace = %q, want default", provider.query.Namespace)
	}
}

func TestMonitoringLogsQueryNilProvider503(t *testing.T) {
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "start": monTestFrom, "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "LOG_PROVIDER_UNAVAILABLE") {
		t.Fatalf("body = %q, want code LOG_PROVIDER_UNAVAILABLE", rec.Body.String())
	}
}

func TestMonitoringLogsQueryInvalidBody400(t *testing.T) {
	provider := &stubCapabilityLogs{}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader([]byte("{bad"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestMonitoringLogsQueryInvalidTimestamp400(t *testing.T) {
	provider := &stubCapabilityLogs{}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "start": "not-a-timestamp", "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestMonitoringLogsQueryNamespaceScopeDenied404(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete}}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	// Inject a restrictive scope that only allows "allowed-ns"; the request
	// asks for "denied-ns" → 404 anti-leakage.
	scopeSetter := func(c *gin.Context) {
		c.Set(namespaceScopeKey, authz.ClusterScope{
			ClusterID:       1,
			AllNamespaces:   false,
			NamespaceGrants: []string{"allowed-ns"},
		})
	}
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, scopeSetter)
	body, _ := json.Marshal(map[string]any{
		"namespace": "denied-ns", "start": monTestFrom, "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage) body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "RESOURCE_NOT_FOUND") {
		t.Fatalf("body = %q, want code RESOURCE_NOT_FOUND", rec.Body.String())
	}
}

func TestMonitoringLogsQueryNamespaceScopeAllowed200(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete}}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	scopeSetter := func(c *gin.Context) {
		c.Set(namespaceScopeKey, authz.ClusterScope{
			ClusterID:       1,
			AllNamespaces:   false,
			NamespaceGrants: []string{"allowed-ns"},
		})
	}
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, scopeSetter)
	body, _ := json.Marshal(map[string]any{
		"namespace": "allowed-ns", "start": monTestFrom, "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
}

func TestMonitoringLogsQueryInvalidLogQuery400(t *testing.T) {
	provider := &stubCapabilityLogs{err: capability.ErrInvalidLogQuery}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "start": monTestFrom, "end": monTestTo, "limit": 99999,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestMonitoringLogsQueryDefaultDirectionForward(t *testing.T) {
	provider := &stubCapabilityLogs{result: capability.LogResult{State: capability.StateComplete}}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "start": monTestFrom, "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	if provider.query.Direction != capability.DirectionForward {
		t.Fatalf("direction = %q, want %q", provider.query.Direction, capability.DirectionForward)
	}
}

// TestMonitoringLogsQueryProviderError500 ensures a non-validation provider
// error surfaces as 500 (not a leak of internal details).
func TestMonitoringLogsQueryProviderError500(t *testing.T) {
	provider := &stubCapabilityLogs{err: context.DeadlineExceeded}
	svc := monitoring.NewService(monitoring.Config{}, nil, nil)
	router := newMonitoringRouter(monitoringHandler{service: svc, logProvider: provider}, 1, nil)
	body, _ := json.Marshal(map[string]any{
		"namespace": "default", "start": monTestFrom, "end": monTestTo,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/logs/query", bytes.NewReader(body)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 body=%s", rec.Code, rec.Body.String())
	}
	if !containsCode(rec.Body.String(), "LOG_QUERY_FAILED") {
		t.Fatalf("body = %q, want code LOG_QUERY_FAILED", rec.Body.String())
	}
}
