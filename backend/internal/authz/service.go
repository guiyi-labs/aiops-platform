package authz

import (
	"context"
	"errors"

	"k8s-aiops.local/backend/internal/auth"
)

// ErrAccessDenied is returned when the policy evaluator denies access. Callers
// should map this to a 404 (not 403) for resource detail routes so that hidden
// cluster/resource existence is not leaked, per M35 acceptance standard: "An
// unauthorized target is absent from lists/fan-out and cannot be distinguished
// through direct IDs or error details."
var ErrAccessDenied = errors.New("access denied")

// Service is the policy evaluator. It is the single authorization point used by
// HTTP middleware, fleet fan-out and global search. SystemAdmin bypasses grant
// checks entirely; every other role requires an explicit grant.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// IsSystemAdmin returns whether the role set includes system_admin.
func IsSystemAdmin(roles []string) bool {
	for _, r := range roles {
		if r == auth.SystemAdmin {
			return true
		}
	}
	return false
}

// CanAccessCluster evaluates whether the user may access a cluster at all.
// SystemAdmin always returns true. Other roles require a cluster grant or at
// least one namespace grant for that cluster.
func (s *Service) CanAccessCluster(ctx context.Context, userID int64, roles []string, clusterID int64) (AccessDecision, error) {
	if IsSystemAdmin(roles) {
		return AccessDecision{Allowed: true}, nil
	}
	scope, err := s.repo.ClusterScope(ctx, userID, clusterID)
	if err != nil {
		return AccessDecision{Reason: "internal_error"}, err
	}
	if scope.AllNamespaces || len(scope.NamespaceGrants) > 0 {
		return AccessDecision{Allowed: true}, nil
	}
	return AccessDecision{Reason: "cluster_not_authorized"}, nil
}

// CanAccessNamespace evaluates whether the user may access a specific namespace
// in a cluster. SystemAdmin always returns true. Other roles require either a
// cluster grant (all namespaces) or a namespace grant for that exact namespace.
func (s *Service) CanAccessNamespace(ctx context.Context, userID int64, roles []string, clusterID int64, namespace string) (AccessDecision, error) {
	if IsSystemAdmin(roles) {
		return AccessDecision{Allowed: true}, nil
	}
	scope, err := s.repo.ClusterScope(ctx, userID, clusterID)
	if err != nil {
		return AccessDecision{Reason: "internal_error"}, err
	}
	if scope.AllNamespaces {
		return AccessDecision{Allowed: true}, nil
	}
	for _, ns := range scope.NamespaceGrants {
		if ns == namespace {
			return AccessDecision{Allowed: true}, nil
		}
	}
	return AccessDecision{Reason: "namespace_not_authorized"}, nil
}

// VisibleClusters returns the cluster IDs the user may access. SystemAdmin
// returns nil (meaning all enabled clusters); other roles return the distinct
// set from cluster and namespace grants.
func (s *Service) VisibleClusters(ctx context.Context, userID int64, roles []string) ([]int64, error) {
	if IsSystemAdmin(roles) {
		return nil, nil
	}
	return s.repo.VisibleClusters(ctx, userID)
}

// ClusterScope returns the user's namespace scope for a cluster. SystemAdmin
// returns AllNamespaces=true; other roles delegate to the repository.
func (s *Service) ClusterScope(ctx context.Context, userID int64, roles []string, clusterID int64) (ClusterScope, error) {
	if IsSystemAdmin(roles) {
		return ClusterScope{ClusterID: clusterID, AllNamespaces: true}, nil
	}
	return s.repo.ClusterScope(ctx, userID, clusterID)
}

// AuthorizedNamespaces resolves the effective namespace list for a list-style
// query. Returns (scope, specificNamespace, err) where:
//   - scope.AllNamespaces=true means the caller may query all namespaces.
//   - otherwise NamespaceGrants lists exactly which namespaces may be queried.
//     If requestedNamespace is non-empty, this function validates that the
//     caller has access and returns a single-element slice.
//     If requestedNamespace is empty and the caller has no cluster-level
//     grant, NamespaceGrants is the list of namespace-grants. If the caller
//     has no grants at all, NamespaceGrants is empty and ErrAccessDenied is
//     NOT returned (caller must treat empty list as "no results").
//
// This helper is intended for list-endpoint handlers where namespace is a
// query parameter rather than a path parameter.
func (s *Service) AuthorizedNamespaces(ctx context.Context, userID int64, roles []string, clusterID int64, requestedNamespace string) (ClusterScope, error) {
	if IsSystemAdmin(roles) {
		return ClusterScope{ClusterID: clusterID, AllNamespaces: true}, nil
	}
	scope, err := s.repo.ClusterScope(ctx, userID, clusterID)
	if err != nil {
		return ClusterScope{}, err
	}
	if requestedNamespace != "" {
		if scope.AllNamespaces {
			return ClusterScope{ClusterID: clusterID, AllNamespaces: true, NamespaceGrants: []string{requestedNamespace}}, nil
		}
		for _, ns := range scope.NamespaceGrants {
			if ns == requestedNamespace {
				return ClusterScope{ClusterID: clusterID, AllNamespaces: false, NamespaceGrants: []string{requestedNamespace}}, nil
			}
		}
		return ClusterScope{ClusterID: clusterID}, ErrAccessDenied
	}
	// Empty requested namespace: caller is listing "all".
	return scope, nil
}

// GrantManager provides SystemAdmin-only CRUD over access grants. It is a thin
// wrapper over the repository; the HTTP layer enforces the system_admin role.
type GrantManager struct {
	repo Repository
}

func NewGrantManager(repo Repository) *GrantManager { return &GrantManager{repo: repo} }

func (g *GrantManager) CreateClusterGrant(ctx context.Context, userID, clusterID int64) (ClusterGrant, error) {
	return g.repo.CreateClusterGrant(ctx, userID, clusterID)
}

func (g *GrantManager) DeleteClusterGrant(ctx context.Context, userID, clusterID int64) error {
	return g.repo.DeleteClusterGrant(ctx, userID, clusterID)
}

func (g *GrantManager) ListClusterGrants(ctx context.Context, userID int64) ([]ClusterGrant, error) {
	return g.repo.ListClusterGrants(ctx, userID)
}

func (g *GrantManager) CreateNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) (NamespaceGrant, error) {
	return g.repo.CreateNamespaceGrant(ctx, userID, clusterID, namespace)
}

func (g *GrantManager) DeleteNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) error {
	return g.repo.DeleteNamespaceGrant(ctx, userID, clusterID, namespace)
}

func (g *GrantManager) ListNamespaceGrants(ctx context.Context, userID int64) ([]NamespaceGrant, error) {
	return g.repo.ListNamespaceGrants(ctx, userID)
}
