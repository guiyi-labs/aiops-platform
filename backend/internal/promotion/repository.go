package promotion

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	Save(ctx context.Context, plan *Plan) error
	Get(ctx context.Context, id string) (Plan, error)
	List(ctx context.Context, sourceClusterID int64, namespace string) ([]Plan, error)
	Claim(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (Plan, bool, error)
	Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time, itemStatuses map[int64]string, itemErrors map[int64]string) (Plan, error)
	Fail(ctx context.Context, id, idempotencyKey, message string) (Plan, error)
	ExpireStale(ctx context.Context, now time.Time) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, plan *Plan) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

func (r *GormRepository) Get(ctx context.Context, id string) (Plan, error) {
	var plan Plan
	err := r.db.WithContext(ctx).Preload("Items").Preload("Dependencies").First(&plan, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return Plan{}, ErrNotFound
		}
		return Plan{}, err
	}
	return plan, nil
}

func (r *GormRepository) List(ctx context.Context, sourceClusterID int64, namespace string) ([]Plan, error) {
	if err := r.ExpireStale(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx).Model(&Plan{}).Where("source_cluster_id = ?", sourceClusterID)
	if namespace != "" {
		query = query.Where("source_namespace = ?", namespace)
	}
	var plans []Plan
	err := query.Preload("Items").Order("created_at DESC, id DESC").Limit(50).Find(&plans).Error
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
		case StatusSucceeded, StatusFailed, StatusPartial:
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
	if shouldExecute {
		if items, err := r.loadItems(ctx, id); err == nil {
			plan.Items = items
		}
	}
	return plan, shouldExecute, claimErr
}

func (r *GormRepository) Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time, itemStatuses map[int64]string, itemErrors map[int64]string) (Plan, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan Plan
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		if plan.IdempotencyKey != idempotencyKey || plan.Status != StatusExecuting {
			return ErrNotFound
		}
		applied, failed, skipped := 0, 0, 0
		for _, status := range itemStatuses {
			switch status {
			case ItemStatusApplied:
				applied++
			case ItemStatusFailed:
				failed++
			case ItemStatusSkipped:
				skipped++
			}
		}
		overall := StatusSucceeded
		if (failed > 0 || skipped > 0) && applied > 0 {
			overall = StatusPartial
		} else if failed > 0 || skipped > 0 {
			overall = StatusFailed
		}
		var summary BundleSummary
		if err := json.Unmarshal(plan.BundleSummary, &summary); err != nil {
			return err
		}
		summary.PendingCount = summary.ItemCount - applied - failed - skipped
		if summary.PendingCount < 0 {
			summary.PendingCount = 0
		}
		summary.AppliedCount, summary.FailedCount, summary.SkippedCount = applied, failed, skipped
		if err := tx.Model(&plan).Updates(map[string]any{
			"status":         overall,
			"bundle_summary": mustMarshal(summary),
			"executed_at":    executedAt,
			"locked_at":      nil,
			"last_error":     "",
			"updated_at":     executedAt,
		}).Error; err != nil {
			return err
		}
		for itemID, status := range itemStatuses {
			updates := map[string]any{"item_status": status, "last_error": itemErrors[itemID]}
			if err := tx.Model(&BundleItem{}).Where("id = ?", itemID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return r.Get(ctx, id)
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
	return r.Get(ctx, id)
}

func (r *GormRepository) ExpireStale(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Model(&Plan{}).
		Where("status = ? AND expires_at <= ?", StatusAwaitingConfirmation, now).
		Updates(map[string]any{"status": StatusExpired, "updated_at": now}).Error
}

func (r *GormRepository) loadItems(ctx context.Context, planID string) ([]BundleItem, error) {
	var items []BundleItem
	err := r.db.WithContext(ctx).Where("plan_id = ?", planID).Order("ordinal ASC").Find(&items).Error
	return items, err
}

var _ Repository = (*GormRepository)(nil)
