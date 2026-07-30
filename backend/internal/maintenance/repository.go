package maintenance

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
	Claim(context.Context, string, []byte, string, time.Time, time.Time) (Plan, bool, error)
	Complete(context.Context, string, string, time.Time, Plan, *ExecutionResultJSON) (Plan, error)
	Fail(context.Context, string, string, string, *ExecutionResultJSON) (Plan, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, plan *Plan) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

func (r *GormRepository) List(ctx context.Context, clusterID int64) ([]Plan, error) {
	if err := r.db.WithContext(ctx).Model(&Plan{}).
		Where("cluster_id = ? AND status = ? AND expires_at <= NOW()", clusterID, StatusAwaitingConfirmation).
		Updates(map[string]any{"status": StatusExpired, "updated_at": time.Now().UTC()}).Error; err != nil {
		return nil, err
	}
	var plans []Plan
	err := r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Order("created_at DESC, id DESC").Limit(50).Find(&plans).Error
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

func (r *GormRepository) Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time, plan Plan, result *ExecutionResultJSON) (Plan, error) {
	updates := map[string]any{
		"status":      StatusSucceeded,
		"executed_at": executedAt,
		"locked_at":   nil,
		"last_error":  "",
		"updated_at":  executedAt,
	}
	if result != nil {
		updates["execution_result"] = *result
		updates["node_unschedulable"] = result.UnschedulableNow
	}
	result2 := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(updates)
	if result2.Error != nil {
		return Plan{}, result2.Error
	}
	if result2.RowsAffected == 0 {
		return Plan{}, ErrNotFound
	}
	return r.get(ctx, id)
}

func (r *GormRepository) Fail(ctx context.Context, id, idempotencyKey, message string, result *ExecutionResultJSON) (Plan, error) {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":     StatusFailed,
		"locked_at":  nil,
		"last_error": message,
		"updated_at": now,
	}
	if result != nil {
		updates["execution_result"] = *result
		updates["node_unschedulable"] = result.UnschedulableNow
	}
	result2 := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(updates)
	if result2.Error != nil {
		return Plan{}, result2.Error
	}
	if result2.RowsAffected == 0 {
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
