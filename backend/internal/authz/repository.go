package authz

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"
)

var (
	ErrGrantNotFound      = errors.New("access grant not found")
	ErrGrantAlreadyExists = errors.New("access grant already exists")
)

// Repository is the persistence boundary for cluster and namespace grants.
type Repository interface {
	// CreateClusterGrant inserts a user-to-cluster grant. Returns
	// ErrGrantAlreadyExists on duplicate (user_id, cluster_id).
	CreateClusterGrant(ctx context.Context, userID, clusterID int64) (ClusterGrant, error)
	// DeleteClusterGrant removes a cluster grant. Returns ErrGrantNotFound if absent.
	DeleteClusterGrant(ctx context.Context, userID, clusterID int64) error
	// ListClusterGrants returns all cluster grants for a user, ordered by cluster_id.
	ListClusterGrants(ctx context.Context, userID int64) ([]ClusterGrant, error)

	// CreateNamespaceGrant inserts a user-to-namespace grant.
	CreateNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) (NamespaceGrant, error)
	// DeleteNamespaceGrant removes a namespace grant.
	DeleteNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) error
	// ListNamespaceGrants returns all namespace grants for a user, ordered by cluster_id then namespace.
	ListNamespaceGrants(ctx context.Context, userID int64) ([]NamespaceGrant, error)

	// ClusterScope returns the user's access scope for one cluster. If the user
	// holds a cluster grant, AllNamespaces=true and NamespaceGrants is empty.
	// Otherwise NamespaceGrants lists exact authorized namespaces (possibly empty).
	ClusterScope(ctx context.Context, userID, clusterID int64) (ClusterScope, error)
	// VisibleClusters returns the distinct cluster IDs the user may access
	// (from cluster grants and namespace grants combined), sorted ascending.
	VisibleClusters(ctx context.Context, userID int64) ([]int64, error)
	// HasClusterGrant returns whether a cluster grant exists for the pair.
	HasClusterGrant(ctx context.Context, userID, clusterID int64) (bool, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) CreateClusterGrant(ctx context.Context, userID, clusterID int64) (ClusterGrant, error) {
	grant := ClusterGrant{UserID: userID, ClusterID: clusterID}
	err := r.db.WithContext(ctx).Create(&grant).Error
	if isUniqueViolation(err) {
		return ClusterGrant{}, ErrGrantAlreadyExists
	}
	return grant, err
}

func (r *GormRepository) DeleteClusterGrant(ctx context.Context, userID, clusterID int64) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND cluster_id = ?", userID, clusterID).Delete(&ClusterGrant{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrGrantNotFound
	}
	return nil
}

func (r *GormRepository) ListClusterGrants(ctx context.Context, userID int64) ([]ClusterGrant, error) {
	var grants []ClusterGrant
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("cluster_id ASC").Find(&grants).Error
	return grants, err
}

func (r *GormRepository) CreateNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) (NamespaceGrant, error) {
	grant := NamespaceGrant{UserID: userID, ClusterID: clusterID, Namespace: namespace}
	err := r.db.WithContext(ctx).Create(&grant).Error
	if isUniqueViolation(err) {
		return NamespaceGrant{}, ErrGrantAlreadyExists
	}
	return grant, err
}

func (r *GormRepository) DeleteNamespaceGrant(ctx context.Context, userID, clusterID int64, namespace string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND cluster_id = ? AND namespace = ?", userID, clusterID, namespace).
		Delete(&NamespaceGrant{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrGrantNotFound
	}
	return nil
}

func (r *GormRepository) ListNamespaceGrants(ctx context.Context, userID int64) ([]NamespaceGrant, error) {
	var grants []NamespaceGrant
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("cluster_id ASC, namespace ASC").Find(&grants).Error
	return grants, err
}

func (r *GormRepository) ClusterScope(ctx context.Context, userID, clusterID int64) (ClusterScope, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&ClusterGrant{}).
		Where("user_id = ? AND cluster_id = ?", userID, clusterID).Count(&count).Error; err != nil {
		return ClusterScope{}, err
	}
	if count > 0 {
		return ClusterScope{ClusterID: clusterID, AllNamespaces: true, NamespaceGrants: nil}, nil
	}
	var nsGrants []NamespaceGrant
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND cluster_id = ?", userID, clusterID).
		Order("namespace ASC").Find(&nsGrants).Error; err != nil {
		return ClusterScope{}, err
	}
	namespaces := make([]string, 0, len(nsGrants))
	for _, g := range nsGrants {
		namespaces = append(namespaces, g.Namespace)
	}
	return ClusterScope{ClusterID: clusterID, AllNamespaces: false, NamespaceGrants: namespaces}, nil
}

func (r *GormRepository) VisibleClusters(ctx context.Context, userID int64) ([]int64, error) {
	var clusterIDs []int64
	if err := r.db.WithContext(ctx).Model(&ClusterGrant{}).
		Where("user_id = ?", userID).Distinct("cluster_id").Pluck("cluster_id", &clusterIDs).Error; err != nil {
		return nil, err
	}
	var nsClusterIDs []int64
	if err := r.db.WithContext(ctx).Model(&NamespaceGrant{}).
		Where("user_id = ?", userID).Distinct("cluster_id").Pluck("cluster_id", &nsClusterIDs).Error; err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(clusterIDs)+len(nsClusterIDs))
	merged := make([]int64, 0, len(clusterIDs)+len(nsClusterIDs))
	for _, id := range clusterIDs {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	for _, id := range nsClusterIDs {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i] < merged[j] })
	return merged, nil
}

func (r *GormRepository) HasClusterGrant(ctx context.Context, userID, clusterID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ClusterGrant{}).
		Where("user_id = ? AND cluster_id = ?", userID, clusterID).Count(&count).Error
	return count > 0, err
}

// isUniqueViolation detects a PostgreSQL unique-constraint violation without
// importing the pg driver package by matching the error message. GORM wraps the
// underlying driver error; the canonical unique-violation SQLSTATE is 23505.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
