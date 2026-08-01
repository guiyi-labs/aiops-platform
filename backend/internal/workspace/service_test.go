package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"k8s-aiops.local/backend/internal/auth"
)

// fakeRepository is an in-memory Repository for service-level tests. It is
// safe for sequential use within a single test; concurrent access is guarded
// by a mutex so it does not race when the service fans out reads.
type fakeRepository struct {
	mu                          sync.Mutex
	workspaces                  map[int64]Workspace
	workspaceSeq                int64
	memberships                 map[int64]WorkspaceMembership
	membershipSeq               int64
	quotas                      map[int64]WorkspaceQuota
	grants                      map[int64]UserWorkspaceGrant
	grantSeq                    int64
	audit                       []WorkspaceRoleBindingAudit
	createWorkspaceFn           func(Workspace) (Workspace, error)
	listMembershipsByClusterErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		workspaces:  make(map[int64]Workspace),
		memberships: make(map[int64]WorkspaceMembership),
		quotas:      make(map[int64]WorkspaceQuota),
		grants:      make(map[int64]UserWorkspaceGrant),
	}
}

func (r *fakeRepository) CreateWorkspace(ctx context.Context, ws Workspace) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createWorkspaceFn != nil {
		return r.createWorkspaceFn(ws)
	}
	for _, existing := range r.workspaces {
		if existing.Name == ws.Name {
			return Workspace{}, ErrWorkspaceAlreadyExists
		}
	}
	r.workspaceSeq++
	ws.ID = r.workspaceSeq
	r.workspaces[ws.ID] = ws
	return ws, nil
}

func (r *fakeRepository) GetWorkspace(ctx context.Context, id int64) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ws, ok := r.workspaces[id]
	if !ok {
		return Workspace{}, ErrWorkspaceNotFound
	}
	return ws, nil
}

func (r *fakeRepository) GetWorkspaceByName(ctx context.Context, name string) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ws := range r.workspaces {
		if ws.Name == name {
			return ws, nil
		}
	}
	return Workspace{}, ErrWorkspaceNotFound
}

func (r *fakeRepository) ListWorkspaces(ctx context.Context, ownerUserID int64) ([]Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		if ownerUserID > 0 && ws.OwnerUserID != ownerUserID {
			continue
		}
		out = append(out, ws)
	}
	return out, nil
}

func (r *fakeRepository) UpdateWorkspace(ctx context.Context, ws Workspace) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workspaces[ws.ID]; !ok {
		return Workspace{}, ErrWorkspaceNotFound
	}
	r.workspaces[ws.ID] = ws
	return ws, nil
}

func (r *fakeRepository) DeleteWorkspace(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workspaces[id]; !ok {
		return ErrWorkspaceNotFound
	}
	delete(r.workspaces, id)
	for gid, g := range r.grants {
		if g.WorkspaceID == id {
			delete(r.grants, gid)
		}
	}
	return nil
}

func (r *fakeRepository) AddMembership(ctx context.Context, membership WorkspaceMembership) (WorkspaceMembership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.memberships {
		if existing.ClusterID == membership.ClusterID && existing.Namespace == membership.Namespace {
			return WorkspaceMembership{}, ErrMembershipAlreadyExists
		}
	}
	r.membershipSeq++
	membership.ID = r.membershipSeq
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *fakeRepository) ListMemberships(ctx context.Context, workspaceID int64) ([]WorkspaceMembership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]WorkspaceMembership, 0)
	for _, m := range r.memberships {
		if m.WorkspaceID == workspaceID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeRepository) ListMembershipsByCluster(ctx context.Context, clusterID int64) ([]WorkspaceMembership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listMembershipsByClusterErr != nil {
		return nil, r.listMembershipsByClusterErr
	}
	out := make([]WorkspaceMembership, 0)
	for _, m := range r.memberships {
		if m.ClusterID == clusterID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeRepository) RemoveMembership(ctx context.Context, workspaceID, clusterID int64, namespace string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, m := range r.memberships {
		if m.WorkspaceID == workspaceID && m.ClusterID == clusterID && m.Namespace == namespace {
			delete(r.memberships, id)
			return nil
		}
	}
	return ErrMembershipNotFound
}

func (r *fakeRepository) GetQuota(ctx context.Context, workspaceID int64) (WorkspaceQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.quotas[workspaceID]
	if !ok {
		return WorkspaceQuota{WorkspaceID: workspaceID}, nil
	}
	return q, nil
}

func (r *fakeRepository) UpsertQuota(ctx context.Context, quota WorkspaceQuota) (WorkspaceQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotas[quota.WorkspaceID] = quota
	return quota, nil
}

func (r *fakeRepository) CreateGrant(ctx context.Context, grant UserWorkspaceGrant) (UserWorkspaceGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.grants {
		if existing.UserID == grant.UserID && existing.WorkspaceID == grant.WorkspaceID {
			return UserWorkspaceGrant{}, ErrWorkspaceGrantAlreadyExists
		}
	}
	r.grantSeq++
	grant.ID = r.grantSeq
	r.grants[grant.ID] = grant
	return grant, nil
}

func (r *fakeRepository) GetGrant(ctx context.Context, userID, workspaceID int64) (UserWorkspaceGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			return g, nil
		}
	}
	return UserWorkspaceGrant{}, ErrWorkspaceGrantNotFound
}

func (r *fakeRepository) ListGrants(ctx context.Context, workspaceID int64) ([]UserWorkspaceGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]UserWorkspaceGrant, 0)
	for _, g := range r.grants {
		if g.WorkspaceID == workspaceID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *fakeRepository) UpdateGrantRole(ctx context.Context, userID, workspaceID int64, role string) (UserWorkspaceGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			g.Role = role
			r.grants[id] = g
			return g, nil
		}
	}
	return UserWorkspaceGrant{}, ErrWorkspaceGrantNotFound
}

func (r *fakeRepository) DeleteGrant(ctx context.Context, userID, workspaceID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, g := range r.grants {
		if g.UserID == userID && g.WorkspaceID == workspaceID {
			delete(r.grants, id)
			return nil
		}
	}
	return ErrWorkspaceGrantNotFound
}

func (r *fakeRepository) ListUserWorkspaces(ctx context.Context, userID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *fakeRepository) AppendAudit(ctx context.Context, entry WorkspaceRoleBindingAudit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audit = append(r.audit, entry)
	return nil
}

func (r *fakeRepository) ListAudit(ctx context.Context, workspaceID int64, limit int) ([]WorkspaceRoleBindingAudit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	out := make([]WorkspaceRoleBindingAudit, 0)
	for i := len(r.audit) - 1; i >= 0 && len(out) < limit; i-- {
		if r.audit[i].WorkspaceID == workspaceID {
			out = append(out, r.audit[i])
		}
	}
	return out, nil
}

// seedWorkspace creates a workspace with an owner grant and returns the
// workspace. Useful as a setup helper for tests that exercise role semantics.
func seedWorkspace(t *testing.T, repo *fakeRepository, ownerID int64, name string) Workspace {
	t.Helper()
	ctx := context.Background()
	ws, err := repo.CreateWorkspace(ctx, Workspace{
		Name:         name,
		DisplayName:  name,
		OwnerUserID:  ownerID,
		MetadataJSON: json.RawMessage("{}"),
	})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	_, err = repo.CreateGrant(ctx, UserWorkspaceGrant{
		UserID:      ownerID,
		WorkspaceID: ws.ID,
		Role:        RoleAdmin,
	})
	if err != nil {
		t.Fatalf("seed owner grant: %v", err)
	}
	return ws
}

// seedGrant attaches a grant to an existing workspace.
func seedGrant(t *testing.T, repo *fakeRepository, userID, workspaceID int64, role string) {
	t.Helper()
	_, err := repo.CreateGrant(context.Background(), UserWorkspaceGrant{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

const (
	adminActor   = int64(1)
	editorActor  = int64(2)
	viewerActor  = int64(3)
	outsideActor = int64(4)
	ownerID      = int64(10)
)

var adminRoles = []string{auth.SystemAdmin}
var nonAdminRoles = []string{auth.Viewer}

// ============================================================================
// CreateWorkspace
// ============================================================================

func TestCreateWorkspaceSystemAdminSeedsOwnerGrant(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws, err := svc.CreateWorkspace(context.Background(), adminActor, adminRoles, CreateWorkspaceInput{
		Name:        "team-alpha",
		DisplayName: "Team Alpha",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws.OwnerUserID != adminActor {
		t.Fatalf("owner = %d, want %d", ws.OwnerUserID, adminActor)
	}
	grant, err := repo.GetGrant(context.Background(), adminActor, ws.ID)
	if err != nil {
		t.Fatalf("owner grant not seeded: %v", err)
	}
	if grant.Role != RoleAdmin {
		t.Fatalf("owner grant role = %q, want %q", grant.Role, RoleAdmin)
	}
	audit, _ := repo.ListAudit(context.Background(), ws.ID, 10)
	if len(audit) != 1 || audit[0].Action != AuditActionGranted {
		t.Fatalf("audit not recorded: %+v", audit)
	}
}

func TestCreateWorkspaceNonAdminDeniedAs404(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	_, err := svc.CreateWorkspace(context.Background(), editorActor, nonAdminRoles, CreateWorkspaceInput{
		Name:        "team-beta",
		DisplayName: "Team Beta",
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound (anti-leakage)", err)
	}
}

func TestCreateWorkspaceInvalidName(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	cases := []string{"", "UPPER", "with space", "-leading", "trailing-", "too" + string(make([]byte, 64))}
	for _, name := range cases {
		_, err := svc.CreateWorkspace(context.Background(), adminActor, adminRoles, CreateWorkspaceInput{
			Name:        name,
			DisplayName: "x",
		})
		if err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
}

func TestCreateWorkspaceDuplicateName(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	_, _ = svc.CreateWorkspace(context.Background(), adminActor, adminRoles, CreateWorkspaceInput{
		Name: "dup", DisplayName: "Dup",
	})
	_, err := svc.CreateWorkspace(context.Background(), adminActor, adminRoles, CreateWorkspaceInput{
		Name: "dup", DisplayName: "Dup",
	})
	if !errors.Is(err, ErrWorkspaceAlreadyExists) {
		t.Fatalf("error = %v, want ErrWorkspaceAlreadyExists", err)
	}
}

// ============================================================================
// GetWorkspace + anti-leakage
// ============================================================================

func TestGetWorkspaceMemberSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	_, err := svc.GetWorkspace(context.Background(), viewerActor, nonAdminRoles, ws.ID)
	if err != nil {
		t.Fatalf("member get: %v", err)
	}
}

func TestGetWorkspaceNonMemberDeniedAs404(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.GetWorkspace(context.Background(), outsideActor, nonAdminRoles, ws.ID)
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound (anti-leakage)", err)
	}
}

func TestGetWorkspaceSystemAdminBypass(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.GetWorkspace(context.Background(), adminActor, adminRoles, ws.ID)
	if err != nil {
		t.Fatalf("admin bypass: %v", err)
	}
}

// ============================================================================
// ListWorkspaces
// ============================================================================

func TestListWorkspacesNonAdminSeesOnlyGranted(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws1 := seedWorkspace(t, repo, ownerID, "ws1")
	ws2 := seedWorkspace(t, repo, ownerID, "ws2")
	seedGrant(t, repo, viewerActor, ws1.ID, RoleViewer)
	list, err := svc.ListWorkspaces(context.Background(), viewerActor, nonAdminRoles)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != ws1.ID {
		t.Fatalf("list = %+v, want only ws1", list)
	}
	_ = ws2
}

func TestListWorkspacesSystemAdminSeesAll(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	seedWorkspace(t, repo, ownerID, "ws1")
	seedWorkspace(t, repo, ownerID, "ws2")
	list, err := svc.ListWorkspaces(context.Background(), adminActor, adminRoles)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
}

// ============================================================================
// UpdateWorkspace role hierarchy
// ============================================================================

func TestUpdateWorkspaceAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.UpdateWorkspace(context.Background(), ownerID, nonAdminRoles, ws.ID, UpdateWorkspaceInput{
		DisplayName: "New Name",
	})
	if err != nil {
		t.Fatalf("admin update: %v", err)
	}
}

func TestUpdateWorkspaceViewerDeniedAs404(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	_, err := svc.UpdateWorkspace(context.Background(), viewerActor, nonAdminRoles, ws.ID, UpdateWorkspaceInput{
		DisplayName: "Hack",
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound (anti-leakage)", err)
	}
}

func TestUpdateWorkspaceEditorDeniedAs404(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, editorActor, ws.ID, RoleEditor)
	_, err := svc.UpdateWorkspace(context.Background(), editorActor, nonAdminRoles, ws.ID, UpdateWorkspaceInput{
		DisplayName: "Hack",
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound (anti-leakage)", err)
	}
}

// ============================================================================
// DeleteWorkspace
// ============================================================================

func TestDeleteWorkspaceSystemAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	if err := svc.DeleteWorkspace(context.Background(), adminActor, adminRoles, ws.ID); err != nil {
		t.Fatalf("admin delete: %v", err)
	}
}

func TestDeleteWorkspaceAdminDeniedAs404(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	err := svc.DeleteWorkspace(context.Background(), ownerID, nonAdminRoles, ws.ID)
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
}

// ============================================================================
// Membership
// ============================================================================

func TestAddMembershipAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.AddMembership(context.Background(), ownerID, nonAdminRoles, ws.ID, 5, "prod")
	if err != nil {
		t.Fatalf("add membership: %v", err)
	}
}

func TestAddMembershipViewerDenied(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	_, err := svc.AddMembership(context.Background(), viewerActor, nonAdminRoles, ws.ID, 5, "prod")
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestRemoveMembershipAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, _ = svc.AddMembership(context.Background(), ownerID, nonAdminRoles, ws.ID, 5, "prod")
	if err := svc.RemoveMembership(context.Background(), ownerID, nonAdminRoles, ws.ID, 5, "prod"); err != nil {
		t.Fatalf("remove membership: %v", err)
	}
}

func TestListMembershipsViewerSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, _ = svc.AddMembership(context.Background(), ownerID, nonAdminRoles, ws.ID, 5, "prod")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	items, err := svc.ListMemberships(context.Background(), viewerActor, nonAdminRoles, ws.ID)
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
}

// ============================================================================
// Quota
// ============================================================================

func TestSetQuotaAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	cpu := 8.0
	_, err := svc.SetQuota(context.Background(), ownerID, nonAdminRoles, ws.ID, SetQuotaInput{HardCPUCores: &cpu})
	if err != nil {
		t.Fatalf("set quota: %v", err)
	}
}

func TestSetQuotaViewerDenied(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	cpu := 8.0
	_, err := svc.SetQuota(context.Background(), viewerActor, nonAdminRoles, ws.ID, SetQuotaInput{HardCPUCores: &cpu})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestSetQuotaNegativeRejected(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	cpu := -1.0
	_, err := svc.SetQuota(context.Background(), ownerID, nonAdminRoles, ws.ID, SetQuotaInput{HardCPUCores: &cpu})
	if err == nil {
		t.Fatal("expected error for negative quota")
	}
}

func TestGetQuotaViewerSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	_, err := svc.GetQuota(context.Background(), viewerActor, nonAdminRoles, ws.ID)
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
}

// ============================================================================
// Role bindings
// ============================================================================

func TestGrantRoleAdminCreatesNewGrant(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	grant, err := svc.GrantRole(context.Background(), ownerID, nonAdminRoles, GrantRoleInput{
		UserID:      viewerActor,
		WorkspaceID: ws.ID,
		Role:        RoleViewer,
	})
	if err != nil {
		t.Fatalf("grant role: %v", err)
	}
	if grant.Role != RoleViewer {
		t.Fatalf("role = %q, want %q", grant.Role, RoleViewer)
	}
	audit, _ := repo.ListAudit(context.Background(), ws.ID, 10)
	if len(audit) != 1 || audit[0].Action != AuditActionGranted {
		t.Fatalf("audit not recorded: %+v", audit)
	}
}

func TestGrantRoleAdminReplacesExisting(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	grant, err := svc.GrantRole(context.Background(), ownerID, nonAdminRoles, GrantRoleInput{
		UserID:      viewerActor,
		WorkspaceID: ws.ID,
		Role:        RoleEditor,
	})
	if err != nil {
		t.Fatalf("grant role: %v", err)
	}
	if grant.Role != RoleEditor {
		t.Fatalf("role = %q, want %q", grant.Role, RoleEditor)
	}
	audit, _ := repo.ListAudit(context.Background(), ws.ID, 10)
	if len(audit) != 1 || audit[0].Action != AuditActionChanged {
		t.Fatalf("audit should record changed: %+v", audit)
	}
}

func TestGrantRoleInvalidRoleRejected(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.GrantRole(context.Background(), ownerID, nonAdminRoles, GrantRoleInput{
		UserID:      viewerActor,
		WorkspaceID: ws.ID,
		Role:        "superuser",
	})
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("error = %v, want ErrInvalidRole", err)
	}
}

func TestGrantRoleOwnerRoleFixed(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_, err := svc.GrantRole(context.Background(), ownerID, nonAdminRoles, GrantRoleInput{
		UserID:      ownerID,
		WorkspaceID: ws.ID,
		Role:        RoleViewer,
	})
	if err == nil {
		t.Fatal("expected error when downgrading owner")
	}
}

func TestGrantRoleViewerDenied(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	_, err := svc.GrantRole(context.Background(), viewerActor, nonAdminRoles, GrantRoleInput{
		UserID:      editorActor,
		WorkspaceID: ws.ID,
		Role:        RoleEditor,
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestRevokeRoleAdminSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	if err := svc.RevokeRole(context.Background(), ownerID, nonAdminRoles, ws.ID, viewerActor); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}

func TestRevokeRoleOwnerCannotBeRevoked(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	err := svc.RevokeRole(context.Background(), ownerID, nonAdminRoles, ws.ID, ownerID)
	if err == nil {
		t.Fatal("expected error when revoking owner")
	}
}

func TestListGrantsViewerSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	items, err := svc.ListGrants(context.Background(), viewerActor, nonAdminRoles, ws.ID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("grants len = %d, want 2 (owner + viewer)", len(items))
	}
}

func TestListAuditViewerSuccess(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	_ = svc.AppendAuditForTest(context.Background(), ws.ID, ownerID)
	seedGrant(t, repo, viewerActor, ws.ID, RoleViewer)
	items, err := svc.ListAudit(context.Background(), viewerActor, nonAdminRoles, ws.ID, 10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("audit len = %d, want 1", len(items))
	}
}

// AppendAuditForTest is a test-only helper that appends a synthetic audit
// entry directly via the repository. It exists so ListAudit tests do not
// depend on the full GrantRole flow.
func (s *Service) AppendAuditForTest(ctx context.Context, workspaceID, userID int64) error {
	return s.repo.AppendAudit(ctx, WorkspaceRoleBindingAudit{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        RoleAdmin,
		Action:      AuditActionGranted,
		GrantedBy:   &userID,
	})
}

// ============================================================================
// Metadata normalization
// ============================================================================

func TestNormalizeMetadataNilBecomesEmptyObject(t *testing.T) {
	out, err := normalizeMetadata(nil)
	if err != nil {
		t.Fatalf("normalize nil: %v", err)
	}
	if string(out) != "{}" {
		t.Fatalf("output = %s, want {}", out)
	}
}

func TestNormalizeMetadataNonObjectRejected(t *testing.T) {
	_, err := normalizeMetadata(json.RawMessage(`[1,2,3]`))
	if err == nil {
		t.Fatal("expected error for non-object metadata")
	}
}

// ============================================================================
// Role helpers
// ============================================================================

func TestRoleRank(t *testing.T) {
	cases := []struct {
		role string
		want int
	}{
		{RoleAdmin, 3},
		{RoleEditor, 2},
		{RoleViewer, 1},
		{"unknown", 0},
	}
	for _, tc := range cases {
		if got := RoleRank(tc.role); got != tc.want {
			t.Fatalf("RoleRank(%q) = %d, want %d", tc.role, got, tc.want)
		}
	}
}

func TestIsValidRole(t *testing.T) {
	if !IsValidRole(RoleAdmin) || !IsValidRole(RoleEditor) || !IsValidRole(RoleViewer) {
		t.Fatal("fixed roles should be valid")
	}
	if IsValidRole("superuser") {
		t.Fatal("superuser should be invalid")
	}
}

// ============================================================================
// NamespacesForWorkspaceFilter (M47 visibility narrowing)
// ============================================================================

// seedMembership attaches a (cluster_id, namespace) tuple to a workspace
// directly via the repository, bypassing the service's role check. This is
// the right setup level for filter-method tests: we are testing the read
// path, not the membership-grant authorization.
func seedMembership(t *testing.T, repo *fakeRepository, workspaceID, clusterID int64, namespace string) {
	t.Helper()
	_, err := repo.AddMembership(context.Background(), WorkspaceMembership{
		WorkspaceID: workspaceID,
		ClusterID:   clusterID,
		Namespace:   namespace,
	})
	if err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

// TestNamespacesForWorkspaceFilter_ZeroIDsReturnsNil verifies the filter is
// disabled (returns nil) when workspaceID or clusterID is non-positive. This
// is the contract that lets list handlers treat nil as "no filter applied".
func TestNamespacesForWorkspaceFilter_ZeroIDsReturnsNil(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedMembership(t, repo, ws.ID, 5, "prod")

	cases := []struct {
		name        string
		clusterID   int64
		workspaceID int64
	}{
		{"zero workspace", 5, 0},
		{"negative workspace", 5, -1},
		{"zero cluster", 0, ws.ID},
		{"negative cluster", -1, ws.ID},
		{"both zero", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := svc.NamespacesForWorkspaceFilter(context.Background(), tc.clusterID, tc.workspaceID)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if out != nil {
				t.Fatalf("out = %v, want nil (filter disabled)", out)
			}
		})
	}
}

// TestNamespacesForWorkspaceFilter_ReturnsOnlyMatchingWorkspace verifies the
// filter returns only the namespaces bound to the requested workspace on the
// requested cluster, excluding memberships of other workspaces and other
// clusters.
func TestNamespacesForWorkspaceFilter_ReturnsOnlyMatchingWorkspace(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws1 := seedWorkspace(t, repo, ownerID, "ws1")
	ws2 := seedWorkspace(t, repo, ownerID, "ws2")

	// ws1 on cluster 5: prod, staging
	seedMembership(t, repo, ws1.ID, 5, "prod")
	seedMembership(t, repo, ws1.ID, 5, "staging")
	// ws1 on cluster 6 (different cluster): should NOT appear when filtering cluster 5
	seedMembership(t, repo, ws1.ID, 6, "other-cluster-ns")
	// ws2 on cluster 5: dev — should NOT appear when filtering ws1
	seedMembership(t, repo, ws2.ID, 5, "dev")

	out, err := svc.NamespacesForWorkspaceFilter(context.Background(), 5, ws1.ID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2; out=%v", len(out), out)
	}
	got := make(map[string]bool, len(out))
	for _, ns := range out {
		got[ns] = true
	}
	if !got["prod"] || !got["staging"] {
		t.Fatalf("out = %v, want {prod, staging}", out)
	}
	if got["dev"] {
		t.Fatal("dev (ws2) leaked into ws1 filter")
	}
	if got["other-cluster-ns"] {
		t.Fatal("other-cluster-ns (cluster 6) leaked into cluster 5 filter")
	}
}

// TestNamespacesForWorkspaceFilter_UnknownWorkspaceReturnsEmpty verifies
// anti-leakage: a non-existent workspaceID yields an empty (not nil, not
// error) slice so the list handler renders an empty collection (200 with
// items: []) rather than 404, which would leak the workspace's absence.
//
// Note: the filter returns empty (not nil) because workspaceID > 0 took the
// active-filter path; the middleware's narrowScopeByWorkspace then maps an
// empty set to an empty scope. Returning empty here (rather than nil) is
// intentional — nil is reserved for "filter disabled".
func TestNamespacesForWorkspaceFilter_UnknownWorkspaceReturnsEmpty(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	seedMembership(t, repo, ws.ID, 5, "prod")

	out, err := svc.NamespacesForWorkspaceFilter(context.Background(), 5, 99999)
	if err != nil {
		t.Fatalf("error: %v (anti-leakage must not surface as error)", err)
	}
	if len(out) != 0 {
		t.Fatalf("out = %v, want empty (unknown workspace)", out)
	}
}

// TestNamespacesForWorkspaceFilter_WorkspaceWithNoMembershipsOnCluster
// verifies that a workspace which exists but has no memberships on the
// requested cluster returns empty (not an error).
func TestNamespacesForWorkspaceFilter_WorkspaceWithNoMembershipsOnCluster(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ws := seedWorkspace(t, repo, ownerID, "ws1")
	// Membership exists on cluster 5, but we query cluster 7.
	seedMembership(t, repo, ws.ID, 5, "prod")

	out, err := svc.NamespacesForWorkspaceFilter(context.Background(), 7, ws.ID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("out = %v, want empty (no memberships on cluster 7)", out)
	}
}

// TestNamespacesForWorkspaceFilter_RepositoryErrorPropagates verifies that a
// repository failure surfaces as an error (the middleware maps this to 500)
// rather than being silently swallowed into an empty result.
func TestNamespacesForWorkspaceFilter_RepositoryErrorPropagates(t *testing.T) {
	repo := newFakeRepository()
	repo.listMembershipsByClusterErr = errors.New("db unavailable")
	svc := NewService(repo)

	_, err := svc.NamespacesForWorkspaceFilter(context.Background(), 5, 1)
	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
}
