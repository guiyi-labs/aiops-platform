package authz

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"k8s-aiops.local/backend/internal/auth"
)

// fakeRepository is an in-memory Repository stub for exercising the policy
// evaluator and grant manager without a database.
type fakeRepository struct {
	clusterGrants   map[int64]map[int64]ClusterGrant // userID -> clusterID -> grant
	namespaceGrants map[int64]map[string]NamespaceGrant
	visible         []int64
	visibleErr      error
	scopeErr        error
	createErr       error
	deleteErr       error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		clusterGrants:   make(map[int64]map[int64]ClusterGrant),
		namespaceGrants: make(map[int64]map[string]NamespaceGrant),
	}
}

func (r *fakeRepository) CreateClusterGrant(_ context.Context, userID, clusterID int64) (ClusterGrant, error) {
	if r.createErr != nil {
		return ClusterGrant{}, r.createErr
	}
	if _, ok := r.clusterGrants[userID][clusterID]; ok {
		return ClusterGrant{}, ErrGrantAlreadyExists
	}
	if r.clusterGrants[userID] == nil {
		r.clusterGrants[userID] = make(map[int64]ClusterGrant)
	}
	grant := ClusterGrant{ID: int64(len(r.clusterGrants[userID]) + 1), UserID: userID, ClusterID: clusterID}
	r.clusterGrants[userID][clusterID] = grant
	return grant, nil
}

func (r *fakeRepository) DeleteClusterGrant(_ context.Context, userID, clusterID int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.clusterGrants[userID][clusterID]; !ok {
		return ErrGrantNotFound
	}
	delete(r.clusterGrants[userID], clusterID)
	return nil
}

func (r *fakeRepository) ListClusterGrants(_ context.Context, userID int64) ([]ClusterGrant, error) {
	grants := make([]ClusterGrant, 0, len(r.clusterGrants[userID]))
	for _, g := range r.clusterGrants[userID] {
		grants = append(grants, g)
	}
	return grants, nil
}

func (r *fakeRepository) CreateNamespaceGrant(_ context.Context, userID, clusterID int64, namespace string) (NamespaceGrant, error) {
	if r.createErr != nil {
		return NamespaceGrant{}, r.createErr
	}
	key := namespaceKey(clusterID, namespace)
	if _, ok := r.namespaceGrants[userID][key]; ok {
		return NamespaceGrant{}, ErrGrantAlreadyExists
	}
	if r.namespaceGrants[userID] == nil {
		r.namespaceGrants[userID] = make(map[string]NamespaceGrant)
	}
	grant := NamespaceGrant{ID: int64(len(r.namespaceGrants[userID]) + 1), UserID: userID, ClusterID: clusterID, Namespace: namespace}
	r.namespaceGrants[userID][key] = grant
	return grant, nil
}

func (r *fakeRepository) DeleteNamespaceGrant(_ context.Context, userID, clusterID int64, namespace string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	key := namespaceKey(clusterID, namespace)
	if _, ok := r.namespaceGrants[userID][key]; !ok {
		return ErrGrantNotFound
	}
	delete(r.namespaceGrants[userID], key)
	return nil
}

func (r *fakeRepository) ListNamespaceGrants(_ context.Context, userID int64) ([]NamespaceGrant, error) {
	grants := make([]NamespaceGrant, 0, len(r.namespaceGrants[userID]))
	for _, g := range r.namespaceGrants[userID] {
		grants = append(grants, g)
	}
	return grants, nil
}

func (r *fakeRepository) ClusterScope(_ context.Context, userID, clusterID int64) (ClusterScope, error) {
	if r.scopeErr != nil {
		return ClusterScope{}, r.scopeErr
	}
	if _, ok := r.clusterGrants[userID][clusterID]; ok {
		return ClusterScope{ClusterID: clusterID, AllNamespaces: true}, nil
	}
	scope := ClusterScope{ClusterID: clusterID, AllNamespaces: false}
	for _, g := range r.namespaceGrants[userID] {
		if g.ClusterID == clusterID {
			scope.NamespaceGrants = append(scope.NamespaceGrants, g.Namespace)
		}
	}
	return scope, nil
}

func (r *fakeRepository) VisibleClusters(_ context.Context, _ int64) ([]int64, error) {
	return r.visible, r.visibleErr
}

func (r *fakeRepository) HasClusterGrant(_ context.Context, userID, clusterID int64) (bool, error) {
	_, ok := r.clusterGrants[userID][clusterID]
	return ok, nil
}

func namespaceKey(clusterID int64, namespace string) string {
	return strconv.FormatInt(clusterID, 10) + "/" + namespace
}

func TestIsSystemAdmin(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  bool
	}{
		{name: "empty", roles: nil, want: false},
		{name: "viewer only", roles: []string{auth.Viewer}, want: false},
		{name: "admin present", roles: []string{auth.Viewer, auth.SystemAdmin}, want: true},
		{name: "admin only", roles: []string{auth.SystemAdmin}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSystemAdmin(tt.roles); got != tt.want {
				t.Fatalf("IsSystemAdmin(%v) = %v, want %v", tt.roles, got, tt.want)
			}
		})
	}
}

func TestCanAccessClusterSystemAdminBypassesGrants(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	decision, err := service.CanAccessCluster(context.Background(), 1, []string{auth.SystemAdmin}, 42)
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessClusterClusterGrantAllowsAccess(t *testing.T) {
	repo := newFakeRepository()
	if _, err := repo.CreateClusterGrant(context.Background(), 7, 11); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	decision, err := service.CanAccessCluster(context.Background(), 7, []string{auth.Viewer}, 11)
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessClusterNamespaceGrantAllowsAccess(t *testing.T) {
	repo := newFakeRepository()
	if _, err := repo.CreateNamespaceGrant(context.Background(), 7, 11, "prod"); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	decision, err := service.CanAccessCluster(context.Background(), 7, []string{auth.Viewer}, 11)
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessClusterDeniesWithoutGrant(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	decision, err := service.CanAccessCluster(context.Background(), 7, []string{auth.Viewer}, 11)
	if err != nil || decision.Allowed || decision.Reason != "cluster_not_authorized" {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessClusterRepoErrorReturnsInternalError(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeRepository{scopeErr: boom}
	service := NewService(repo)
	_, err := service.CanAccessCluster(context.Background(), 7, []string{auth.Viewer}, 11)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestCanAccessNamespaceSystemAdminBypassesGrants(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	decision, err := service.CanAccessNamespace(context.Background(), 1, []string{auth.SystemAdmin}, 42, "prod")
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessNamespaceClusterGrantAllowsAllNamespaces(t *testing.T) {
	repo := newFakeRepository()
	if _, err := repo.CreateClusterGrant(context.Background(), 7, 11); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	decision, err := service.CanAccessNamespace(context.Background(), 7, []string{auth.Viewer}, 11, "any")
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessNamespaceNamespaceGrantAllowsExactNamespace(t *testing.T) {
	repo := newFakeRepository()
	if _, err := repo.CreateNamespaceGrant(context.Background(), 7, 11, "prod"); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	decision, err := service.CanAccessNamespace(context.Background(), 7, []string{auth.Viewer}, 11, "prod")
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestCanAccessNamespaceDeniesUnauthorizedNamespace(t *testing.T) {
	repo := newFakeRepository()
	if _, err := repo.CreateNamespaceGrant(context.Background(), 7, 11, "prod"); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	decision, err := service.CanAccessNamespace(context.Background(), 7, []string{auth.Viewer}, 11, "staging")
	if err != nil || decision.Allowed || decision.Reason != "namespace_not_authorized" {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
}

func TestVisibleClustersSystemAdminReturnsNil(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	visible, err := service.VisibleClusters(context.Background(), 1, []string{auth.SystemAdmin})
	if err != nil || visible != nil {
		t.Fatalf("visible = %v err=%v", visible, err)
	}
}

func TestVisibleClustersDelegatesToRepo(t *testing.T) {
	expected := []int64{1, 2, 3}
	repo := &fakeRepository{visible: expected}
	service := NewService(repo)
	visible, err := service.VisibleClusters(context.Background(), 7, []string{auth.Viewer})
	if err != nil || len(visible) != 3 {
		t.Fatalf("visible = %v err=%v", visible, err)
	}
}

func TestClusterScopeSystemAdminReturnsAllNamespaces(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	scope, err := service.ClusterScope(context.Background(), 1, []string{auth.SystemAdmin}, 42)
	if err != nil || !scope.AllNamespaces || scope.ClusterID != 42 {
		t.Fatalf("scope = %#v err=%v", scope, err)
	}
}

func TestGrantManagerCreateClusterGrantDelegates(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	grant, err := manager.CreateClusterGrant(context.Background(), 7, 11)
	if err != nil || grant.UserID != 7 || grant.ClusterID != 11 {
		t.Fatalf("grant = %#v err=%v", grant, err)
	}
}

func TestGrantManagerCreateClusterGrantDuplicateReturnsConflict(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	if _, err := manager.CreateClusterGrant(context.Background(), 7, 11); err != nil {
		t.Fatal(err)
	}
	_, err := manager.CreateClusterGrant(context.Background(), 7, 11)
	if !errors.Is(err, ErrGrantAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestGrantManagerDeleteClusterGrantMissingReturnsNotFound(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	err := manager.DeleteClusterGrant(context.Background(), 7, 11)
	if !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestGrantManagerListClusterGrantsReturnsAll(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	for _, id := range []int64{3, 1, 2} {
		if _, err := manager.CreateClusterGrant(context.Background(), 7, id); err != nil {
			t.Fatal(err)
		}
	}
	grants, err := manager.ListClusterGrants(context.Background(), 7)
	if err != nil || len(grants) != 3 {
		t.Fatalf("grants = %#v err=%v", grants, err)
	}
}

func TestGrantManagerCreateNamespaceGrantDelegates(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	grant, err := manager.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
	if err != nil || grant.UserID != 7 || grant.ClusterID != 11 || grant.Namespace != "prod" {
		t.Fatalf("grant = %#v err=%v", grant, err)
	}
}

func TestGrantManagerCreateNamespaceGrantDuplicateReturnsConflict(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	if _, err := manager.CreateNamespaceGrant(context.Background(), 7, 11, "prod"); err != nil {
		t.Fatal(err)
	}
	_, err := manager.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
	if !errors.Is(err, ErrGrantAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestGrantManagerDeleteNamespaceGrantMissingReturnsNotFound(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	err := manager.DeleteNamespaceGrant(context.Background(), 7, 11, "prod")
	if !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestGrantManagerListNamespaceGrantsReturnsAll(t *testing.T) {
	repo := newFakeRepository()
	manager := NewGrantManager(repo)
	for _, ns := range []string{"prod", "staging", "dev"} {
		if _, err := manager.CreateNamespaceGrant(context.Background(), 7, 11, ns); err != nil {
			t.Fatal(err)
		}
	}
	grants, err := manager.ListNamespaceGrants(context.Background(), 7)
	if err != nil || len(grants) != 3 {
		t.Fatalf("grants = %#v err=%v", grants, err)
	}
}

func TestAuthorizedNamespaces(t *testing.T) {
	errSentinel := errors.New("db down sentinel")
	type expected struct {
		AllNamespaces   bool
		NamespaceGrants []string
		err             error
	}
	tests := []struct {
		name               string
		userID             int64
		roles              []string
		clusterID          int64
		requestedNamespace string
		setupRepo          func(*fakeRepository)
		want               expected
	}{
		{
			name:               "system_admin_any_cluster_any_namespace_all",
			userID:             1,
			roles:              []string{auth.SystemAdmin},
			clusterID:          42,
			requestedNamespace: "",
			want:               expected{AllNamespaces: true},
		},
		{
			name:               "system_admin_specific_namespace_allowed",
			userID:             1,
			roles:              []string{auth.SystemAdmin},
			clusterID:          42,
			requestedNamespace: "anything",
			want:               expected{AllNamespaces: true},
		},
		{
			name:               "viewer_cluster_grant_allows_all_namespaces_empty",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateClusterGrant(context.Background(), 7, 11)
			},
			want: expected{AllNamespaces: true},
		},
		{
			name:               "viewer_cluster_grant_allows_any_specific_namespace",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "arbitrary",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateClusterGrant(context.Background(), 7, 11)
			},
			want: expected{AllNamespaces: true, NamespaceGrants: []string{"arbitrary"}},
		},
		{
			name:               "viewer_namespace_grants_returns_granted_list_on_empty",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 11, "staging")
			},
			want: expected{AllNamespaces: false, NamespaceGrants: []string{"prod", "staging"}},
		},
		{
			name:               "viewer_namespace_grant_allows_specific_authorized",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "prod",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
			},
			want: expected{AllNamespaces: false, NamespaceGrants: []string{"prod"}},
		},
		{
			name:               "viewer_namespace_grant_denies_specific_unauthorized",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "hidden",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
			},
			want: expected{err: ErrAccessDenied},
		},
		{
			name:               "viewer_no_grants_returns_empty_scope_on_empty",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "",
			want:               expected{AllNamespaces: false},
		},
		{
			name:               "viewer_no_grants_specific_namespace_denied",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "any",
			want:               expected{err: ErrAccessDenied},
		},
		{
			name:               "repo_scope_error_propagates",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "",
			setupRepo: func(r *fakeRepository) {
				r.scopeErr = errSentinel
			},
			want: expected{err: errSentinel},
		},
		{
			name:               "namespace_grants_filtered_to_requested_cluster_only",
			userID:             7,
			roles:              []string{auth.Viewer},
			clusterID:          11,
			requestedNamespace: "",
			setupRepo: func(r *fakeRepository) {
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 11, "prod")
				_, _ = r.CreateNamespaceGrant(context.Background(), 7, 99, "other")
			},
			want: expected{AllNamespaces: false, NamespaceGrants: []string{"prod"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			service := NewService(repo)
			scope, err := service.AuthorizedNamespaces(context.Background(), tt.userID, tt.roles, tt.clusterID, tt.requestedNamespace)
			if tt.want.err != nil {
				if !errors.Is(err, tt.want.err) {
					t.Fatalf("err = %v, want %v", err, tt.want.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if scope.AllNamespaces != tt.want.AllNamespaces {
				t.Fatalf("AllNamespaces = %v, want %v", scope.AllNamespaces, tt.want.AllNamespaces)
			}
			gotGranted := scope.NamespaceGrants
			wantGranted := tt.want.NamespaceGrants
			if len(gotGranted) != len(wantGranted) {
				t.Fatalf("NamespaceGrants len = %v, want %v", gotGranted, wantGranted)
			}
			// For slice equality, check elements (fake repo returns in creation order for namespace grants)
			wantSet := make(map[string]struct{}, len(wantGranted))
			for _, ns := range wantGranted {
				wantSet[ns] = struct{}{}
			}
			for _, ns := range gotGranted {
				if _, ok := wantSet[ns]; !ok {
					t.Fatalf("NamespaceGrants = %v, want members %v", gotGranted, wantGranted)
				}
			}
			if scope.ClusterID != tt.clusterID {
				t.Fatalf("ClusterID = %d, want %d", scope.ClusterID, tt.clusterID)
			}
		})
	}
}
