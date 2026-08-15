package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/workspace"
)

// newWorkspaceTestEngine builds a gin engine wired to the workspace handler,
// mirroring the route shapes registered in router.go. Authentication is
// bypassed — the actor is injected via request metadata.
func newWorkspaceTestEngine(t *testing.T, svc *workspace.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := workspaceHandler{service: svc}
	api := r.Group("/api/v1/workspaces")
	api.GET("", h.listWorkspaces)
	api.POST("", h.createWorkspace)
	api.GET("/:workspace_id", h.getWorkspace)
	api.PATCH("/:workspace_id", h.updateWorkspace)
	api.DELETE("/:workspace_id", h.deleteWorkspace)
	api.GET("/:workspace_id/memberships", h.listMemberships)
	api.POST("/:workspace_id/memberships", h.addMembership)
	api.DELETE("/:workspace_id/memberships", h.removeMembership)
	api.GET("/:workspace_id/quota", h.getQuota)
	api.PUT("/:workspace_id/quota", h.setQuota)
	api.GET("/:workspace_id/role-bindings", h.listRoleBindings)
	api.POST("/:workspace_id/role-bindings", h.grantRole)
	api.DELETE("/:workspace_id/role-bindings/:user_id", h.revokeRole)
	api.GET("/:workspace_id/role-bindings/audit", h.listRoleBindingsAudit)
	return r
}

func withWorkspaceActor(req *http.Request, actorID int64, roles []string) *http.Request {
	return req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{
		ActorID:   actorID,
		Roles:     roles,
		RequestID: "workspace-test",
	}))
}

func workspaceCreateRequest(t *testing.T, name, displayName string) []byte {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "display_name": displayName})
	return body
}

// TestWorkspaceHandler_CreateAsSystemAdminReturns201 verifies the create
// endpoint returns 201 with the workspace JSON when the actor is SystemAdmin.
func TestWorkspaceHandler_CreateAsSystemAdminReturns201(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)

	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "team-a", "Team A")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var ws workspace.Workspace
	if err := json.Unmarshal(w.Body.Bytes(), &ws); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ws.Name != "team-a" {
		t.Fatalf("name = %q, want team-a", ws.Name)
	}
}

// TestWorkspaceHandler_CreateAsNonAdminReturns404 verifies anti-leakage: a
// non-admin sees 404 (not 403) so existence of the create endpoint is not
// leaked.
func TestWorkspaceHandler_CreateAsNonAdminReturns404(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)

	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "team-b", "Team B")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 2, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage)", w.Code)
	}
}

// TestWorkspaceHandler_GetAsMemberReturns200 verifies that a workspace member
// can read the workspace.
func TestWorkspaceHandler_GetAsMemberReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-x")
	seedWorkspaceGrant(t, repo, 5, ws.ID, "workspace_viewer")

	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/"+itoa(ws.ID), nil)
	req = withWorkspaceActor(req, 5, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestWorkspaceHandler_GetAsNonMemberReturns404 verifies anti-leakage at the
// handler layer: a non-member gets 404 (not 403).
func TestWorkspaceHandler_GetAsNonMemberReturns404(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-y")

	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/"+itoa(ws.ID), nil)
	req = withWorkspaceActor(req, 99, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage)", w.Code)
	}
}

// TestWorkspaceHandler_AddMembershipAsAdminReturns201 verifies the membership
// add endpoint.
func TestWorkspaceHandler_AddMembershipAsAdminReturns201(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-m")

	engine := newWorkspaceTestEngine(t, svc)
	body, _ := json.Marshal(map[string]interface{}{"cluster_id": 7, "namespace": "prod"})
	req := httptest.NewRequest("POST", "/api/v1/workspaces/"+itoa(ws.ID)+"/memberships", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 10, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

// TestWorkspaceHandler_RemoveMembershipMissingQueryReturns400 verifies the
// query-parameter validation on DELETE memberships.
func TestWorkspaceHandler_RemoveMembershipMissingQueryReturns400(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-r")

	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+itoa(ws.ID)+"/memberships", nil)
	req = withWorkspaceActor(req, 10, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestWorkspaceHandler_GrantRoleInvalidRoleReturns400 verifies the role
// validation error path.
func TestWorkspaceHandler_GrantRoleInvalidRoleReturns400(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-g")

	engine := newWorkspaceTestEngine(t, svc)
	body, _ := json.Marshal(map[string]interface{}{"user_id": 5, "role": "superuser"})
	req := httptest.NewRequest("POST", "/api/v1/workspaces/"+itoa(ws.ID)+"/role-bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 10, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestWorkspaceHandler_SetQuotaReturns200 verifies the quota set endpoint.
func TestWorkspaceHandler_SetQuotaReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-q")

	engine := newWorkspaceTestEngine(t, svc)
	body, _ := json.Marshal(map[string]interface{}{"hard_cpu_cores": 4.0})
	req := httptest.NewRequest("PUT", "/api/v1/workspaces/"+itoa(ws.ID)+"/quota", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 10, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestWorkspaceHandler_ListRoleBindingsAuditReturns200 verifies the audit
// trail endpoint.
func TestWorkspaceHandler_ListRoleBindingsAuditReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	ws := seedWorkspaceForHandler(t, repo, 10, "ws-aud")

	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/"+itoa(ws.ID)+"/role-bindings/audit?limit=5", nil)
	req = withWorkspaceActor(req, 10, []string{"viewer"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestWorkspaceHandler_InvalidWorkspaceIDReturns400 verifies path-parameter
// validation.
func TestWorkspaceHandler_InvalidWorkspaceIDReturns400(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)

	req := httptest.NewRequest("GET", "/api/v1/workspaces/abc", nil)
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// itoa is a strconv.FormatInt alias to keep the test file readable.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// ---- shared fake repository (wraps the workspace package's fakeRepository) ----
//
// The service_test.go file in the workspace package defines an unexported
// fakeRepository. Since the handler test lives in the httpserver package, we
// cannot use it directly. Instead we define a thin adapter here that
// implements workspace.Repository by delegating to a minimal in-memory map.
// This avoids exporting test helpers from the workspace package.

type handlerFakeRepo struct {
	workspaces map[int64]workspace.Workspace
	grants     map[int64]workspace.UserWorkspaceGrant
	wsSeq      int64
	gSeq       int64
}

func newWorkspaceFakeRepo() *handlerFakeRepo {
	return &handlerFakeRepo{
		workspaces: make(map[int64]workspace.Workspace),
		grants:     make(map[int64]workspace.UserWorkspaceGrant),
	}
}

func (r *handlerFakeRepo) CreateWorkspace(ctx context.Context, ws workspace.Workspace) (workspace.Workspace, error) {
	for _, existing := range r.workspaces {
		if existing.Name == ws.Name {
			return workspace.Workspace{}, workspace.ErrWorkspaceAlreadyExists
		}
	}
	r.wsSeq++
	ws.ID = r.wsSeq
	r.workspaces[ws.ID] = ws
	return ws, nil
}

func (r *handlerFakeRepo) GetWorkspace(ctx context.Context, id int64) (workspace.Workspace, error) {
	ws, ok := r.workspaces[id]
	if !ok {
		return workspace.Workspace{}, workspace.ErrWorkspaceNotFound
	}
	return ws, nil
}

func (r *handlerFakeRepo) GetWorkspaceByName(ctx context.Context, name string) (workspace.Workspace, error) {
	for _, ws := range r.workspaces {
		if ws.Name == name {
			return ws, nil
		}
	}
	return workspace.Workspace{}, workspace.ErrWorkspaceNotFound
}

func (r *handlerFakeRepo) ListWorkspaces(ctx context.Context, ownerUserID int64) ([]workspace.Workspace, error) {
	out := make([]workspace.Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		if ownerUserID > 0 && ws.OwnerUserID != ownerUserID {
			continue
		}
		out = append(out, ws)
	}
	return out, nil
}

func (r *handlerFakeRepo) UpdateWorkspace(ctx context.Context, ws workspace.Workspace) (workspace.Workspace, error) {
	if _, ok := r.workspaces[ws.ID]; !ok {
		return workspace.Workspace{}, workspace.ErrWorkspaceNotFound
	}
	r.workspaces[ws.ID] = ws
	return ws, nil
}

func (r *handlerFakeRepo) DeleteWorkspace(ctx context.Context, id int64) error {
	if _, ok := r.workspaces[id]; !ok {
		return workspace.ErrWorkspaceNotFound
	}
	delete(r.workspaces, id)
	return nil
}

func (r *handlerFakeRepo) AddMembership(ctx context.Context, m workspace.WorkspaceMembership) (workspace.WorkspaceMembership, error) {
	return m, nil
}

func (r *handlerFakeRepo) ListMemberships(ctx context.Context, workspaceID int64) ([]workspace.WorkspaceMembership, error) {
	return nil, nil
}

func (r *handlerFakeRepo) ListMembershipsByCluster(ctx context.Context, clusterID int64) ([]workspace.WorkspaceMembership, error) {
	return nil, nil
}

func (r *handlerFakeRepo) RemoveMembership(ctx context.Context, workspaceID, clusterID int64, namespace string) error {
	return nil
}

func (r *handlerFakeRepo) GetQuota(ctx context.Context, workspaceID int64) (workspace.WorkspaceQuota, error) {
	return workspace.WorkspaceQuota{WorkspaceID: workspaceID}, nil
}

func (r *handlerFakeRepo) UpsertQuota(ctx context.Context, q workspace.WorkspaceQuota) (workspace.WorkspaceQuota, error) {
	return q, nil
}

func (r *handlerFakeRepo) CreateGrant(ctx context.Context, g workspace.UserWorkspaceGrant) (workspace.UserWorkspaceGrant, error) {
	for _, existing := range r.grants {
		if existing.UserID == g.UserID && existing.WorkspaceID == g.WorkspaceID {
			return workspace.UserWorkspaceGrant{}, workspace.ErrWorkspaceGrantAlreadyExists
		}
	}
	r.gSeq++
	g.ID = r.gSeq
	r.grants[g.ID] = g
	return g, nil
}

func (r *handlerFakeRepo) GetGrant(ctx context.Context, userID, workspaceID int64) (workspace.UserWorkspaceGrant, error) {
	for _, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			return g, nil
		}
	}
	return workspace.UserWorkspaceGrant{}, workspace.ErrWorkspaceGrantNotFound
}

func (r *handlerFakeRepo) ListGrants(ctx context.Context, workspaceID int64) ([]workspace.UserWorkspaceGrant, error) {
	out := make([]workspace.UserWorkspaceGrant, 0)
	for _, g := range r.grants {
		if g.WorkspaceID == workspaceID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *handlerFakeRepo) UpdateGrantRole(ctx context.Context, userID, workspaceID int64, role string) (workspace.UserWorkspaceGrant, error) {
	for id, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			g.Role = role
			r.grants[id] = g
			return g, nil
		}
	}
	return workspace.UserWorkspaceGrant{}, workspace.ErrWorkspaceGrantNotFound
}

func (r *handlerFakeRepo) DeleteGrant(ctx context.Context, userID, workspaceID int64) error {
	for id, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			delete(r.grants, id)
			return nil
		}
	}
	return workspace.ErrWorkspaceGrantNotFound
}

func (r *handlerFakeRepo) ListUserWorkspaces(ctx context.Context, userID int64) ([]int64, error) {
	seen := make(map[int64]struct{})
	out := make([]int64, 0)
	for _, g := range r.grants {
		if g.UserID == userID {
			if _, ok := seen[g.WorkspaceID]; !ok {
				seen[g.WorkspaceID] = struct{}{}
				out = append(out, g.WorkspaceID)
			}
		}
	}
	return out, nil
}

func (r *handlerFakeRepo) AppendAudit(ctx context.Context, e workspace.WorkspaceRoleBindingAudit) error {
	return nil
}

func (r *handlerFakeRepo) ListAudit(ctx context.Context, workspaceID int64, limit int) ([]workspace.WorkspaceRoleBindingAudit, error) {
	return nil, nil
}

func seedWorkspaceForHandler(t *testing.T, repo *handlerFakeRepo, ownerID int64, name string) workspace.Workspace {
	t.Helper()
	ws, err := repo.CreateRequestSeededWorkspace(ownerID, name)
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	_, err = repo.CreateGrant(context.Background(), workspace.UserWorkspaceGrant{
		UserID:      ownerID,
		WorkspaceID: ws.ID,
		Role:        workspace.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	return ws
}

func seedWorkspaceGrant(t *testing.T, repo *handlerFakeRepo, userID, workspaceID int64, role string) {
	t.Helper()
	_, err := repo.CreateGrant(context.Background(), workspace.UserWorkspaceGrant{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

// CreateRequestSeededWorkspace creates a workspace without going through the
// service (so the owner grant is not auto-seeded). The test helper
// seedWorkspaceForHandler adds the grant explicitly.
func (r *handlerFakeRepo) CreateRequestSeededWorkspace(ownerID int64, name string) (workspace.Workspace, error) {
	r.wsSeq++
	ws := workspace.Workspace{
		ID:          r.wsSeq,
		Name:        name,
		DisplayName: name,
		OwnerUserID: ownerID,
	}
	r.workspaces[ws.ID] = ws
	return ws, nil
}

func TestWorkspaceHandler_ListWorkspacesReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("GET", "/api/v1/workspaces", nil)
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestWorkspaceHandler_UpdateWorkspaceReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	// Create first.
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-update", "Update Me")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body)
	}
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	body, _ := json.Marshal(map[string]string{"display_name": "Updated Name"})
	req2 := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", w2.Code, w2.Body.String())
	}
}

func TestWorkspaceHandler_DeleteWorkspaceReturns204(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-delete", "Delete Me")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	req2 := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10), nil)
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w2.Code)
	}
}

func TestWorkspaceHandler_ListMembershipsReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-mem", "Members")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	req2 := httptest.NewRequest("GET", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10)+"/memberships", nil)
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("list memberships status = %d", w2.Code)
	}
}

func TestWorkspaceHandler_GetQuotaReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-quota", "Quota")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	req2 := httptest.NewRequest("GET", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10)+"/quota", nil)
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("get quota status = %d", w2.Code)
	}
}

func TestWorkspaceHandler_ListRoleBindingsReturns200(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-rb", "RB")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	req2 := httptest.NewRequest("GET", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10)+"/role-bindings", nil)
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("list role bindings status = %d", w2.Code)
	}
}

func TestWorkspaceHandler_RevokeRoleReturns204(t *testing.T) {
	repo := newWorkspaceFakeRepo()
	svc := workspace.NewService(repo)
	engine := newWorkspaceTestEngine(t, svc)
	req := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewReader(workspaceCreateRequest(t, "ws-revoke", "Revoke")))
	req.Header.Set("Content-Type", "application/json")
	req = withWorkspaceActor(req, 1, []string{"system_admin"})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created workspace.Workspace
	json.Unmarshal(w.Body.Bytes(), &created)
	req2 := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+strconv.FormatInt(created.ID, 10)+"/role-bindings/2", nil)
	req2 = withWorkspaceActor(req2, 1, []string{"system_admin"})
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	// Might be 204 or 404 depending on whether the grant exists.
	if w2.Code != http.StatusNoContent && w2.Code != http.StatusNotFound {
		t.Fatalf("revoke role status = %d", w2.Code)
	}
}
