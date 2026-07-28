package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("cluster not found")

type Repository interface {
	List(context.Context) ([]Cluster, error)
	Find(context.Context, int64) (Cluster, Credential, error)
	Create(context.Context, *Cluster, Credential) error
	UpdateCredential(context.Context, int64, string, Credential, time.Time, []Condition) error
	SetEnabled(context.Context, int64, bool) error
	UpdateProbe(context.Context, int64, string, string, time.Time, []Condition) error
	Delete(context.Context, int64) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) List(ctx context.Context) ([]Cluster, error) {
	var clusters []Cluster
	err := r.db.WithContext(ctx).Preload("Conditions").Order("created_at DESC").Find(&clusters).Error
	return clusters, err
}

func (r *GormRepository) Find(ctx context.Context, id int64) (Cluster, Credential, error) {
	var item Cluster
	if err := r.db.WithContext(ctx).Preload("Conditions").First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Cluster{}, Credential{}, ErrNotFound
		}
		return Cluster{}, Credential{}, err
	}
	var credential Credential
	if err := r.db.WithContext(ctx).First(&credential, "cluster_id = ?", id).Error; err != nil {
		return Cluster{}, Credential{}, err
	}
	return item, credential, nil
}

func (r *GormRepository) Create(ctx context.Context, item *Cluster, credential Credential) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		credential.ClusterID = item.ID
		return tx.Create(&credential).Error
	})
}

func (r *GormRepository) UpdateCredential(ctx context.Context, id int64, apiServer string, credential Credential, now time.Time, conditions []Condition) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item Cluster
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		status := StatusUnknown
		if !item.Enabled {
			status = StatusDisabled
		}
		if err := tx.Model(&Cluster{}).Where("id = ?", id).Updates(map[string]any{
			"api_server": apiServer, "status": status, "kubernetes_version": "", "last_probed_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		credential.ClusterID = id
		result := tx.Model(&Credential{}).Where("cluster_id = ?", id).Updates(map[string]any{
			"encrypted_kubeconfig": credential.EncryptedKubeconfig, "encryption_key_version": credential.EncryptionKeyVersion, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("cluster credential is missing")
		}
		for i := range conditions {
			conditions[i].ClusterID = id
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "cluster_id"}, {Name: "type"}},
				DoUpdates: clause.Assignments(map[string]any{
					"status": conditions[i].Status, "reason": conditions[i].Reason, "message": conditions[i].Message, "last_transition_time": conditions[i].LastTransitionTime,
				}),
			}).Create(&conditions[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	status := StatusUnknown
	if !enabled {
		status = StatusDisabled
	}
	result := r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).Updates(map[string]any{"enabled": enabled, "status": status})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) UpdateProbe(ctx context.Context, id int64, status, version string, probedAt time.Time, conditions []Condition) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Cluster{}).Where("id = ?", id).Updates(map[string]any{
			"status": status, "kubernetes_version": version, "last_probed_at": probedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		for i := range conditions {
			conditions[i].ClusterID = id
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "cluster_id"}, {Name: "type"}},
				DoUpdates: clause.Assignments(map[string]any{
					"status":               conditions[i].Status,
					"reason":               conditions[i].Reason,
					"message":              conditions[i].Message,
					"last_transition_time": gorm.Expr("CASE WHEN cluster_conditions.status = ? THEN cluster_conditions.last_transition_time ELSE ? END", conditions[i].Status, conditions[i].LastTransitionTime),
				}),
			}).Create(&conditions[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Cluster{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
