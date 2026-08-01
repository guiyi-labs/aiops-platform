package federation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"k8s-aiops.local/backend/internal/cluster"
)

// Repository is the persistence boundary for the federation layer. All methods
// are context-aware and return sentinel errors defined in model.go.
type Repository interface {
	// ListClusters returns all clusters (host + members + standalone). The
	// service layer classifies them.
	ListClusters(ctx context.Context) ([]cluster.Cluster, error)
	// GetCluster returns a single cluster by ID.
	GetCluster(ctx context.Context, id int64) (cluster.Cluster, error)

	// SetClusterRole updates cluster_role on the cluster row. Callers must
	// validate the role value before calling.
	SetClusterRole(ctx context.Context, id int64, role string, now time.Time) error
	// SetFederationStatus updates federation_status on the cluster row.
	SetFederationStatus(ctx context.Context, id int64, status string, now time.Time) error
	// TouchHeartbeat updates last_heartbeat_at on the cluster row.
	TouchHeartbeat(ctx context.Context, id int64, now time.Time) error
	// CountHost returns the number of clusters with cluster_role = 'host'.
	// Used to enforce the single-host invariant.
	CountHost(ctx context.Context) (int64, error)

	// AppendEvent appends a federation event. The events table is append-only;
	// no UPDATE/DELETE method is exposed.
	AppendEvent(ctx context.Context, event FederationEvent) error
	// ListEvents returns the most recent federation events across all
	// clusters, newest first. limit is bounded by the service.
	ListEvents(ctx context.Context, limit int) ([]FederationEvent, error)
	// ListEventsByCluster returns the most recent federation events for a
	// single cluster, newest first.
	ListEventsByCluster(ctx context.Context, clusterID int64, limit int) ([]FederationEvent, error)
}

// GormRepository is the production Repository backed by GORM/PostgreSQL.
type GormRepository struct{ db *gorm.DB }

// NewGormRepository constructs a GormRepository.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) ListClusters(ctx context.Context) ([]cluster.Cluster, error) {
	var items []cluster.Cluster
	err := r.db.WithContext(ctx).Preload("Conditions").Order("id ASC").Find(&items).Error
	return items, err
}

func (r *GormRepository) GetCluster(ctx context.Context, id int64) (cluster.Cluster, error) {
	var item cluster.Cluster
	err := r.db.WithContext(ctx).Preload("Conditions").First(&item, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cluster.Cluster{}, ErrClusterNotFound
	}
	return item, err
}

func (r *GormRepository) SetClusterRole(ctx context.Context, id int64, role string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&cluster.Cluster{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"cluster_role": role,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrClusterNotFound
	}
	return nil
}

func (r *GormRepository) SetFederationStatus(ctx context.Context, id int64, status string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&cluster.Cluster{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"federation_status": status,
			"updated_at":        now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrClusterNotFound
	}
	return nil
}

func (r *GormRepository) TouchHeartbeat(ctx context.Context, id int64, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&cluster.Cluster{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_heartbeat_at": now,
			"updated_at":        now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrClusterNotFound
	}
	return nil
}

func (r *GormRepository) CountHost(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&cluster.Cluster{}).
		Where("cluster_role = ?", cluster.ClusterRoleHost).
		Count(&count).Error
	return count, err
}

func (r *GormRepository) AppendEvent(ctx context.Context, event FederationEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(&event).Error
}

func (r *GormRepository) ListEvents(ctx context.Context, limit int) ([]FederationEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var events []FederationEvent
	err := r.db.WithContext(ctx).
		Order("occurred_at DESC, id DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *GormRepository) ListEventsByCluster(ctx context.Context, clusterID int64, limit int) ([]FederationEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var events []FederationEvent
	err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("occurred_at DESC, id DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

// nopRepository is a no-op Repository used by tests and when federation is
// disabled. All read methods return empty results; all write methods return
// nil. It is exported indirectly via NewNopRepository so test packages can
// construct it without depending on the concrete type.
type nopRepository struct{}

// NewNopRepository returns a Repository that performs no I/O. It is intended
// for tests where the federation service is constructed but not exercised.
func NewNopRepository() Repository { return nopRepository{} }

func (nopRepository) ListClusters(context.Context) ([]cluster.Cluster, error) {
	return nil, nil
}
func (nopRepository) GetCluster(context.Context, int64) (cluster.Cluster, error) {
	return cluster.Cluster{}, ErrClusterNotFound
}
func (nopRepository) SetClusterRole(context.Context, int64, string, time.Time) error { return nil }
func (nopRepository) SetFederationStatus(context.Context, int64, string, time.Time) error {
	return nil
}
func (nopRepository) TouchHeartbeat(context.Context, int64, time.Time) error     { return nil }
func (nopRepository) CountHost(context.Context) (int64, error)                   { return 0, nil }
func (nopRepository) AppendEvent(context.Context, FederationEvent) error         { return nil }
func (nopRepository) ListEvents(context.Context, int) ([]FederationEvent, error) { return nil, nil }
func (nopRepository) ListEventsByCluster(context.Context, int64, int) ([]FederationEvent, error) {
	return nil, nil
}

var _ Repository = (*GormRepository)(nil)
