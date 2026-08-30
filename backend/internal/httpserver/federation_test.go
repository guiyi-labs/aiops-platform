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

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/federation"
	"k8s-aiops.local/backend/internal/requestctx"
)

// newFederationTestEngine builds a gin engine wired to the federation handler,
// mirroring the route shapes registered in router.go. Authentication and authz
// are bypassed — the actor is injected via request metadata, and authz is nil
// so authorizedClusterFilter returns "all clusters visible".
func newFederationTestEngine(t *testing.T, svc *federation.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := federationHandler{service: svc, authz: nil}
	api := r.Group("/api/v1/federation")
	api.GET("/overview", h.overview)
	api.GET("/events", h.listEvents)
	api.GET("/resources/summary", h.resourceSummary)
	api.POST("/clusters/register", h.registerCluster)
	api.DELETE("/clusters/:cluster_id", h.deregisterCluster)
	api.POST("/clusters/:cluster_id/promote", h.promoteCluster)
	api.POST("/clusters/:cluster_id/demote", h.demoteCluster)
	api.POST("/clusters/:cluster_id/heartbeat", h.heartbeat)
	api.PATCH("/clusters/:cluster_id/status", h.updateStatus)
	api.GET("/clusters/:cluster_id/events", h.listClusterEvents)
	// P2d cross-cluster diagnosis aggregation.
	api.GET("/diagnoses", h.listDiagnoses)
	api.GET("/diagnoses/stats", h.diagnosisStats)
	return r
}

func withFederationActor(req *http.Request, actorID int64, roles []string) *http.Request {
	return req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{
		ActorID:   actorID,
		Roles:     roles,
		RequestID: "federation-test",
	}))
}

// handlerFedRepo is a thin in-memory adapter implementing
// federation.Repository for handler-level tests. It mirrors the
// handlerFakeRepo pattern used by workspace_test.go.
type handlerFedRepo struct {
	clusters map[int64]cluster.Cluster
	events   []federation.FederationEvent
	eventSeq int64
}

func newHandlerFedRepo() *handlerFedRepo {
	return &handlerFedRepo{clusters: make(map[int64]cluster.Cluster)}
}

func (r *handlerFedRepo) ListClusters(context.Context) ([]cluster.Cluster, error) {
	out := make([]cluster.Cluster, 0, len(r.clusters))
	for _, c := range r.clusters {
		out = append(out, c)
	}
	return out, nil
}

func (r *handlerFedRepo) GetCluster(_ context.Context, id int64) (cluster.Cluster, error) {
	c, ok := r.clusters[id]
	if !ok {
		return cluster.Cluster{}, federation.ErrClusterNotFound
	}
	return c, nil
}

func (r *handlerFedRepo) SetClusterRole(_ context.Context, id int64, role string, now time.Time) error {
	c, ok := r.clusters[id]
	if !ok {
		return federation.ErrClusterNotFound
	}
	c.ClusterRole = role
	c.UpdatedAt = now
	r.clusters[id] = c
	return nil
}

func (r *handlerFedRepo) SetFederationStatus(_ context.Context, id int64, status string, now time.Time) error {
	c, ok := r.clusters[id]
	if !ok {
		return federation.ErrClusterNotFound
	}
	c.FederationStatus = status
	c.UpdatedAt = now
	r.clusters[id] = c
	return nil
}

func (r *handlerFedRepo) TouchHeartbeat(_ context.Context, id int64, now time.Time) error {
	c, ok := r.clusters[id]
	if !ok {
		return federation.ErrClusterNotFound
	}
	hb := now
	c.LastHeartbeatAt = &hb
	c.UpdatedAt = now
	r.clusters[id] = c
	return nil
}

func (r *handlerFedRepo) CountHost(context.Context) (int64, error) {
	var count int64
	for _, c := range r.clusters {
		if c.ClusterRole == cluster.ClusterRoleHost {
			count++
		}
	}
	return count, nil
}

func (r *handlerFedRepo) AppendEvent(_ context.Context, event federation.FederationEvent) error {
	r.eventSeq++
	event.ID = r.eventSeq
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	r.events = append(r.events, event)
	return nil
}

func (r *handlerFedRepo) ListEvents(_ context.Context, limit int) ([]federation.FederationEvent, error) {
	if limit <= 0 || limit > federation.MaxEventsLimit {
		limit = federation.DefaultEventsLimit
	}
	out := make([]federation.FederationEvent, 0)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, r.events[i])
	}
	return out, nil
}

func (r *handlerFedRepo) ListEventsByCluster(_ context.Context, clusterID int64, limit int) ([]federation.FederationEvent, error) {
	if limit <= 0 || limit > federation.MaxEventsLimit {
		limit = federation.DefaultEventsLimit
	}
	out := make([]federation.FederationEvent, 0)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		if r.events[i].ClusterID == clusterID {
			out = append(out, r.events[i])
		}
	}
	return out, nil
}

func seedFedCluster(repo *handlerFedRepo, id int64, name, role, status string) cluster.Cluster {
	c := cluster.Cluster{
		ID:               id,
		Name:             name,
		APIServer:        "https://" + name + ".example",
		Enabled:          true,
		Status:           cluster.StatusReady,
		ClusterRole:      role,
		FederationStatus: status,
	}
	repo.clusters[id] = c
	return c
}

// ============================================================================
// Overview
// ============================================================================

func TestFederationHandler_OverviewReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "host", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedFedCluster(repo, 2, "m1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/overview", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var overview federation.Overview
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if overview.Host == nil {
		t.Fatal("host is nil")
	}
	if len(overview.Members) != 1 {
		t.Fatalf("members len = %d, want 1", len(overview.Members))
	}
	if overview.TotalClusters != 2 {
		t.Fatalf("total = %d, want 2", overview.TotalClusters)
	}
}

func TestFederationHandler_OverviewServiceNilReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := federationHandler{service: nil, authz: nil}
	r.GET("/api/v1/federation/overview", h.overview)

	req := httptest.NewRequest("GET", "/api/v1/federation/overview", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// ============================================================================
// RegisterCluster
// ============================================================================

func TestFederationHandler_RegisterClusterReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"cluster_id": 1, "role": "member"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var c cluster.Cluster
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.ClusterRole != cluster.ClusterRoleMember {
		t.Fatalf("role = %q, want member", c.ClusterRole)
	}
}

func TestFederationHandler_RegisterClusterInvalidBodyReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/register", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFederationHandler_RegisterClusterAlreadyRegisteredReturns409(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"cluster_id": 1, "role": "member"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_RegisterClusterNotFoundReturns404(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"cluster_id": 999, "role": "member"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage)", w.Code)
	}
}

// ============================================================================
// DeregisterCluster
// ============================================================================

func TestFederationHandler_DeregisterClusterReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("DELETE", "/api/v1/federation/clusters/1", nil)
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_DeregisterHostReturns409(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("DELETE", "/api/v1/federation/clusters/1", nil)
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestFederationHandler_DeregisterInvalidClusterIDReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("DELETE", "/api/v1/federation/clusters/abc", nil)
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ============================================================================
// Promote / Demote
// ============================================================================

func TestFederationHandler_PromoteClusterReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/1/promote", nil)
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_DemoteClusterReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"target_role": "member"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/1/demote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_DemoteClusterInvalidRoleReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"target_role": "host"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/1/demote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ============================================================================
// Heartbeat
// ============================================================================

func TestFederationHandler_HeartbeatReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusRegistered)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"status": "healthy"})
	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/1/heartbeat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_HeartbeatEmptyBodyReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("POST", "/api/v1/federation/clusters/1/heartbeat", nil)
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty body is optional); body=%s", w.Code, w.Body.String())
	}
}

// ============================================================================
// UpdateStatus
// ============================================================================

func TestFederationHandler_UpdateStatusReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"status": "degraded", "message": "probe timeout"})
	req := httptest.NewRequest("PATCH", "/api/v1/federation/clusters/1/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_UpdateStatusInvalidReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	body, _ := json.Marshal(map[string]interface{}{"status": "unknown"})
	req := httptest.NewRequest("PATCH", "/api/v1/federation/clusters/1/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withFederationActor(req, 1, []string{"operations_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ============================================================================
// Events
// ============================================================================

func TestFederationHandler_ListEventsReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	// Seed an event via the service.
	_, _ = svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusHealthy)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/events", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []federation.FederationEvent `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
}

func TestFederationHandler_ListEventsInvalidLimitReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/events?limit=abc", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFederationHandler_ListClusterEventsReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil)
	_, _ = svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusHealthy)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/clusters/1/events", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_ListClusterEventsInvalidClusterIDReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/clusters/abc/events", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ============================================================================
// ResourceSummary
// ============================================================================

func TestFederationHandler_ResourceSummaryReturns200(t *testing.T) {
	repo := newHandlerFedRepo()
	seedFedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(repo, nil) // nil lister → empty items
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/resources/summary", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var summary federation.ResourceSummary
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summary.Items) != 0 {
		t.Fatalf("items len = %d, want 0 (nil lister)", len(summary.Items))
	}
}

// ============================================================================
// P2d: Cross-cluster diagnosis aggregation
// ============================================================================

// handlerFedDiagRepo is a thin in-memory mock implementing
// federation.FederationDiagnosisRepository for handler-level P2d tests.
type handlerFedDiagRepo struct {
	rows []diagnosis.FederationDiagnosisRow
	stats diagnosis.FederationDiagnosisStats
}

func (r *handlerFedDiagRepo) ListByClusters(_ context.Context, clusters []int64, status, severity string, limit int) ([]diagnosis.FederationDiagnosisRow, error) {
	visible := make(map[int64]bool, len(clusters))
	for _, id := range clusters {
		visible[id] = true
	}
	out := make([]diagnosis.FederationDiagnosisRow, 0, len(r.rows))
	for _, row := range r.rows {
		if len(clusters) > 0 && !visible[row.ClusterID] {
			continue
		}
		if status != "" && row.Status != status {
			continue
		}
		if severity != "" && row.Severity != severity {
			continue
		}
		out = append(out, row)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *handlerFedDiagRepo) StatsByClusters(_ context.Context, _ []int64) (diagnosis.FederationDiagnosisStats, error) {
	return r.stats, nil
}

func TestFederationHandler_ListDiagnosesReturns200(t *testing.T) {
	fedRepo := newHandlerFedRepo()
	seedFedCluster(fedRepo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedFedCluster(fedRepo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := federation.NewService(fedRepo, nil).WithDiagnosisRepository(&handlerFedDiagRepo{
		rows: []diagnosis.FederationDiagnosisRow{
			{ID: 1, ClusterID: 1, RuleID: "pod.crash_loop_backoff.v1", Severity: "high", ResourceKind: "Pod", ResourceName: "nginx-0", Status: "open", Summary: "crash loop", ObservedAt: time.Now().UTC()},
			{ID: 2, ClusterID: 2, RuleID: "node.not_ready.v1", Severity: "critical", ResourceKind: "Node", ResourceName: "node-1", Status: "resolved", Summary: "node not ready", ObservedAt: time.Now().UTC()},
		},
	})
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []federation.FederationDiagnosis `json:"items"`
		Total int                              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	// Verify cluster names are enriched.
	for _, item := range resp.Items {
		if item.ClusterName == "" {
			t.Fatalf("cluster_name is empty for cluster_id=%d", item.ClusterID)
		}
	}
}

func TestFederationHandler_ListDiagnosesStatusFilter(t *testing.T) {
	fedRepo := newHandlerFedRepo()
	seedFedCluster(fedRepo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := federation.NewService(fedRepo, nil).WithDiagnosisRepository(&handlerFedDiagRepo{
		rows: []diagnosis.FederationDiagnosisRow{
			{ID: 1, ClusterID: 1, RuleID: "pod.crash_loop_backoff.v1", Severity: "high", Status: "open", Summary: "crash loop", ObservedAt: time.Now().UTC()},
			{ID: 2, ClusterID: 1, RuleID: "node.not_ready.v1", Severity: "critical", Status: "resolved", Summary: "node not ready", ObservedAt: time.Now().UTC()},
		},
	})
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses?status=open", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []federation.FederationDiagnosis `json:"items"`
		Total int                              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1 (filtered by status=open)", resp.Total)
	}
	if resp.Items[0].Status != "open" {
		t.Fatalf("status = %s, want open", resp.Items[0].Status)
	}
}

func TestFederationHandler_ListDiagnosesInvalidStatusReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses?status=bogus", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_ListDiagnosesInvalidSeverityReturns400(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil)
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses?severity=ultra", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestFederationHandler_DiagnosisStatsReturns200(t *testing.T) {
	fedRepo := newHandlerFedRepo()
	seedFedCluster(fedRepo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := federation.NewService(fedRepo, nil).WithDiagnosisRepository(&handlerFedDiagRepo{
		stats: diagnosis.FederationDiagnosisStats{
			Total:      5,
			ByStatus:   map[string]int64{"open": 3, "resolved": 2},
			BySeverity: map[string]int64{"high": 3, "medium": 2},
			ByCluster:  []diagnosis.FederationClusterCount{{ClusterID: 1, Count: 5}},
		},
	})
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses/stats", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var stats federation.FederationDiagnosisStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats.Total != 5 {
		t.Fatalf("total = %d, want 5", stats.Total)
	}
	if stats.ByStatus["open"] != 3 {
		t.Fatalf("by_status.open = %d, want 3", stats.ByStatus["open"])
	}
	if len(stats.ByCluster) != 1 {
		t.Fatalf("by_cluster len = %d, want 1", len(stats.ByCluster))
	}
}

func TestFederationHandler_DiagnosisStatsNilRepoReturnsZeros(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil) // no diagnosis repo → nil
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses/stats", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var stats federation.FederationDiagnosisStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats.Total != 0 {
		t.Fatalf("total = %d, want 0 (nil repo)", stats.Total)
	}
}

func TestFederationHandler_ListDiagnosesNilRepoReturnsEmpty(t *testing.T) {
	repo := newHandlerFedRepo()
	svc := federation.NewService(repo, nil) // no diagnosis repo → nil
	engine := newFederationTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/federation/diagnoses", nil)
	req = withFederationActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []federation.FederationDiagnosis `json:"items"`
		Total int                              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("total = %d, want 0 (nil repo)", resp.Total)
	}
}
