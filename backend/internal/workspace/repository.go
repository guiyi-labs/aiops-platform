package workspace

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Repository is the persistence boundary for the workspace multi-tenancy layer.
// All methods are context-aware and return sentinel errors defined in model.go.
type Repository interface {
	// Workspace CRUD.
	CreateWorkspace(ctx context.Context, ws Workspace) (Workspace, error)
	GetWorkspace(ctx context.Context, id int64) (Workspace, error)
	GetWorkspaceByName(ctx context.Context, name string) (Workspace, error)
	ListWorkspaces(ctx context.Context, ownerUserID int64) ([]Workspace, error)
	UpdateWorkspace(ctx context.Context, ws Workspace) (Workspace, error)
	DeleteWorkspace(ctx context.Context, id int64) error

	// Membership CRUD.
	AddMembership(ctx context.Context, membership WorkspaceMembership) (WorkspaceMembership, error)
	ListMemberships(ctx context.Context, workspaceID int64) ([]WorkspaceMembership, error)
	// ListMembershipsByCluster returns all memberships on a given cluster.
	// Used by M47 resource-list endpoints to filter by workspace_id without
	// requiring a workspace role on each workspace — the caller already
	// passed ClusterGrant/NamespaceGrant authz, this is a pure visibility
	// filter (ADR 0062 §4).
	ListMembershipsByCluster(ctx context.Context, clusterID int64) ([]WorkspaceMembership, error)
	RemoveMembership(ctx context.Context, workspaceID, clusterID int64, namespace string) error

	// Quota CRUD (one row per workspace).
	GetQuota(ctx context.Context, workspaceID int64) (WorkspaceQuota, error)
	UpsertQuota(ctx context.Context, quota WorkspaceQuota) (WorkspaceQuota, error)

	// Workspace role bindings (user_workspace_grants).
	CreateGrant(ctx context.Context, grant UserWorkspaceGrant) (UserWorkspaceGrant, error)
	GetGrant(ctx context.Context, userID, workspaceID int64) (UserWorkspaceGrant, error)
	ListGrants(ctx context.Context, workspaceID int64) ([]UserWorkspaceGrant, error)
	UpdateGrantRole(ctx context.Context, userID, workspaceID int64, role string) (UserWorkspaceGrant, error)
	DeleteGrant(ctx context.Context, userID, workspaceID int64) error
	// ListUserWorkspaces returns the workspace IDs the user holds any role on.
	ListUserWorkspaces(ctx context.Context, userID int64) ([]int64, error)

	// Audit trail (append-only).
	AppendAudit(ctx context.Context, entry WorkspaceRoleBindingAudit) error
	ListAudit(ctx context.Context, workspaceID int64, limit int) ([]WorkspaceRoleBindingAudit, error)
}

// GormRepository is the production Repository backed by GORM/PostgreSQL.
type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) CreateWorkspace(ctx context.Context, ws Workspace) (Workspace, error) {
	err := r.db.WithContext(ctx).Create(&ws).Error
	if isUniqueViolation(err) {
		return Workspace{}, ErrWorkspaceAlreadyExists
	}
	return ws, err
}

func (r *GormRepository) GetWorkspace(ctx context.Context, id int64) (Workspace, error) {
	var ws Workspace
	err := r.db.WithContext(ctx).First(&ws, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Workspace{}, ErrWorkspaceNotFound
	}
	return ws, err
}

func (r *GormRepository) GetWorkspaceByName(ctx context.Context, name string) (Workspace, error) {
	var ws Workspace
	err := r.db.WithContext(ctx).First(&ws, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Workspace{}, ErrWorkspaceNotFound
	}
	return ws, err
}

func (r *GormRepository) ListWorkspaces(ctx context.Context, ownerUserID int64) ([]Workspace, error) {
	var workspaces []Workspace
	query := r.db.WithContext(ctx).Order("id ASC")
	if ownerUserID > 0 {
		query = query.Where("owner_user_id = ?", ownerUserID)
	}
	err := query.Find(&workspaces).Error
	return workspaces, err
}

func (r *GormRepository) UpdateWorkspace(ctx context.Context, ws Workspace) (Workspace, error) {
	result := r.db.WithContext(ctx).Model(&Workspace{}).
		Where("id = ?", ws.ID).
		Updates(map[string]interface{}{
			"display_name":  ws.DisplayName,
			"metadata_json": ws.MetadataJSON,
			"updated_at":    gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return Workspace{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Workspace{}, ErrWorkspaceNotFound
	}
	return r.GetWorkspace(ctx, ws.ID)
}

func (r *GormRepository) DeleteWorkspace(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Workspace{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

func (r *GormRepository) AddMembership(ctx context.Context, membership WorkspaceMembership) (WorkspaceMembership, error) {
	err := r.db.WithContext(ctx).Create(&membership).Error
	if isUniqueViolation(err) {
		return WorkspaceMembership{}, ErrMembershipAlreadyExists
	}
	return membership, err
}

func (r *GormRepository) ListMemberships(ctx context.Context, workspaceID int64) ([]WorkspaceMembership, error) {
	var memberships []WorkspaceMembership
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("cluster_id ASC, namespace ASC").
		Find(&memberships).Error
	return memberships, err
}

func (r *GormRepository) ListMembershipsByCluster(ctx context.Context, clusterID int64) ([]WorkspaceMembership, error) {
	var memberships []WorkspaceMembership
	err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("workspace_id ASC, namespace ASC").
		Find(&memberships).Error
	return memberships, err
}

func (r *GormRepository) RemoveMembership(ctx context.Context, workspaceID, clusterID int64, namespace string) error {
	result := r.db.WithContext(ctx).
		Where("workspace_id = ? AND cluster_id = ? AND namespace = ?", workspaceID, clusterID, namespace).
		Delete(&WorkspaceMembership{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

func (r *GormRepository) GetQuota(ctx context.Context, workspaceID int64) (WorkspaceQuota, error) {
	var quota WorkspaceQuota
	err := r.db.WithContext(ctx).First(&quota, "workspace_id = ?", workspaceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// A missing quota row is not an error — it means "no quota set". The
		// caller gets a zero-valued quota with WorkspaceID populated so the
		// response shape is consistent.
		return WorkspaceQuota{WorkspaceID: workspaceID, UpdatedAt: time.Now().UTC()}, nil
	}
	return quota, err
}

func (r *GormRepository) UpsertQuota(ctx context.Context, quota WorkspaceQuota) (WorkspaceQuota, error) {
	quota.UpdatedAt = time.Now().UTC()
	err := r.db.WithContext(ctx).Save(&quota).Error
	return quota, err
}

func (r *GormRepository) CreateGrant(ctx context.Context, grant UserWorkspaceGrant) (UserWorkspaceGrant, error) {
	err := r.db.WithContext(ctx).Create(&grant).Error
	if isUniqueViolation(err) {
		return UserWorkspaceGrant{}, ErrWorkspaceGrantAlreadyExists
	}
	return grant, err
}

func (r *GormRepository) GetGrant(ctx context.Context, userID, workspaceID int64) (UserWorkspaceGrant, error) {
	var grant UserWorkspaceGrant
	err := r.db.WithContext(ctx).
		First(&grant, "user_id = ? AND workspace_id = ?", userID, workspaceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserWorkspaceGrant{}, ErrWorkspaceGrantNotFound
	}
	return grant, err
}

func (r *GormRepository) ListGrants(ctx context.Context, workspaceID int64) ([]UserWorkspaceGrant, error) {
	var grants []UserWorkspaceGrant
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("user_id ASC").
		Find(&grants).Error
	return grants, err
}

func (r *GormRepository) UpdateGrantRole(ctx context.Context, userID, workspaceID int64, role string) (UserWorkspaceGrant, error) {
	result := r.db.WithContext(ctx).Model(&UserWorkspaceGrant{}).
		Where("user_id = ? AND workspace_id = ?", userID, workspaceID).
		Update("role", role)
	if result.Error != nil {
		return UserWorkspaceGrant{}, result.Error
	}
	if result.RowsAffected == 0 {
		return UserWorkspaceGrant{}, ErrWorkspaceGrantNotFound
	}
	return r.GetGrant(ctx, userID, workspaceID)
}

func (r *GormRepository) DeleteGrant(ctx context.Context, userID, workspaceID int64) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND workspace_id = ?", userID, workspaceID).
		Delete(&UserWorkspaceGrant{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWorkspaceGrantNotFound
	}
	return nil
}

func (r *GormRepository) ListUserWorkspaces(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&UserWorkspaceGrant{}).
		Where("user_id = ?", userID).
		Distinct("workspace_id").
		Order("workspace_id ASC").
		Pluck("workspace_id", &ids).Error
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *GormRepository) AppendAudit(ctx context.Context, entry WorkspaceRoleBindingAudit) error {
	return r.db.WithContext(ctx).Create(&entry).Error
}

func (r *GormRepository) ListAudit(ctx context.Context, workspaceID int64, limit int) ([]WorkspaceRoleBindingAudit, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var entries []WorkspaceRoleBindingAudit
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("granted_at DESC, id DESC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// isUniqueViolation detects a PostgreSQL unique-constraint violation. GORM
// wraps the underlying driver error; we match gorm.ErrDuplicatedKey.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
