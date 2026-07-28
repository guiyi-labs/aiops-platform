package remediation

import (
	"context"
	"crypto/subtle"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	Save(context.Context, *Plan) error
	List(context.Context, int64) ([]Plan, error)
	ListOperations(context.Context, int64, string, string, string) ([]Plan, error)
	Claim(context.Context, string, []byte, string, time.Time, time.Time) (Plan, bool, error)
	Complete(context.Context, string, string, time.Time) (Plan, error)
	Fail(context.Context, string, string, string) (Plan, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, plan *Plan) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

func (r *GormRepository) List(ctx context.Context, diagnosisID int64) ([]Plan, error) {
	if err := r.db.WithContext(ctx).Model(&Plan{}).Where("diagnosis_id = ? AND status = ? AND expires_at <= NOW()", diagnosisID, StatusAwaitingConfirmation).
		Updates(map[string]any{"status": StatusExpired, "updated_at": time.Now().UTC()}).Error; err != nil {
		return nil, err
	}
	var plans []Plan
	err := r.db.WithContext(ctx).Where("diagnosis_id = ?", diagnosisID).Order("created_at DESC, id DESC").Find(&plans).Error
	return plans, err
}

func (r *GormRepository) ListOperations(ctx context.Context, clusterID int64, namespace, kind, name string) ([]Plan, error) {
	query := r.db.WithContext(ctx).Model(&Plan{}).
		Where("cluster_id = ? AND diagnosis_id IS NULL", clusterID)
	if namespace != "" {
		query = query.Where("target_namespace = ?", namespace)
	}
	if kind != "" {
		query = query.Where("target_kind = ?", kind)
	}
	if name != "" {
		query = query.Where("target_name = ?", name)
	}
	if err := query.Where("status = ? AND expires_at <= NOW()", StatusAwaitingConfirmation).
		Updates(map[string]any{"status": StatusExpired, "updated_at": time.Now().UTC()}).Error; err != nil {
		return nil, err
	}
	var plans []Plan
	err := query.Order("created_at DESC, id DESC").Limit(50).Find(&plans).Error
	return plans, err
}

func (r *GormRepository) Claim(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (Plan, bool, error) {
	var plan Plan
	var claimErr error
	shouldExecute := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				claimErr = ErrNotFound
				return nil
			}
			return err
		}
		if subtle.ConstantTimeCompare(plan.ConfirmationTokenHash, tokenHash) != 1 {
			claimErr = ErrConfirmationInvalid
			return nil
		}
		if plan.Status == StatusExpired || (plan.Status == StatusAwaitingConfirmation && !plan.ExpiresAt.After(now)) {
			if plan.Status != StatusExpired {
				if err := tx.Model(&plan).Updates(map[string]any{"status": StatusExpired, "updated_at": now}).Error; err != nil {
					return err
				}
				plan.Status = StatusExpired
			}
			claimErr = ErrExpired
			return nil
		}
		switch plan.Status {
		case StatusAwaitingConfirmation:
			if err := tx.Model(&plan).Updates(map[string]any{"status": StatusExecuting, "idempotency_key": idempotencyKey, "locked_at": now, "last_error": "", "updated_at": now}).Error; err != nil {
				return err
			}
			plan.Status, plan.IdempotencyKey, plan.LockedAt = StatusExecuting, idempotencyKey, &now
			shouldExecute = true
		case StatusExecuting:
			if plan.IdempotencyKey != idempotencyKey {
				claimErr = ErrAlreadyExecuted
				return nil
			}
			if plan.LockedAt != nil && plan.LockedAt.After(staleBefore) {
				claimErr = ErrInProgress
				return nil
			}
			if err := tx.Model(&plan).Updates(map[string]any{"locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			plan.LockedAt = &now
			shouldExecute = true
		case StatusSucceeded, StatusFailed:
			if plan.IdempotencyKey != idempotencyKey {
				claimErr = ErrAlreadyExecuted
				return nil
			}
		default:
			claimErr = ErrAlreadyExecuted
		}
		return nil
	})
	if err != nil {
		return Plan{}, false, err
	}
	return plan, shouldExecute, claimErr
}

func (r *GormRepository) Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time) (Plan, error) {
	result := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(map[string]any{"status": StatusSucceeded, "executed_at": executedAt, "locked_at": nil, "last_error": "", "updated_at": executedAt})
	if result.Error != nil {
		return Plan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Plan{}, ErrNotFound
	}
	return r.get(ctx, id)
}

func (r *GormRepository) Fail(ctx context.Context, id, idempotencyKey, message string) (Plan, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(map[string]any{"status": StatusFailed, "locked_at": nil, "last_error": message, "updated_at": now})
	if result.Error != nil {
		return Plan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Plan{}, ErrNotFound
	}
	return r.get(ctx, id)
}

func (r *GormRepository) get(ctx context.Context, id string) (Plan, error) {
	var plan Plan
	if err := r.db.WithContext(ctx).First(&plan, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return Plan{}, ErrNotFound
		}
		return Plan{}, err
	}
	return plan, nil
}

var _ Repository = (*GormRepository)(nil)
