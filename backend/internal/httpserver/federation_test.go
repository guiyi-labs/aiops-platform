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
