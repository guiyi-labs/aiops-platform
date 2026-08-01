package copyops

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var claimLease = 15 * time.Second

// Repository is the persistence contract for copy_plans.
type Repository interface {
	Create(context.Context, Plan) (Plan, error)
	GetByID(context.Context, string) (Plan, error)
	ListByUser(context.Context, int64, int, int) ([]Plan, int, error)
	ListByCluster(context.Context, int64, int, int) ([]Plan, int, error)
	ClaimAndLoad(context.Context, string, string, []byte, string) (Plan, error)
	UpdateExecution(context.Context, Plan) error
	UpdateStatus(context.Context, string, string, string) error
}

type GormRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db, now: time.Now}
}

func (r *GormRepository) Create(ctx context.Context, plan Plan) (Plan, error) {
	if err := r.db.WithContext(ctx).Create(&plan).Error; err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (r *GormRepository) GetByID(ctx context.Context, id string) (Plan, error) {
	var plan Plan
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&plan).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Plan{}, ErrNotFound
	}
	return plan, err
}

func (r *GormRepository) ListByUser(ctx context.Context, userID int64, offset, limit int) ([]Plan, int, error) {
	var total int64
	base := r.db.WithContext(ctx).Model(&Plan{}).Where("requested_by_user_id = ?", userID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var plans []Plan
	if limit <= 0 {
		limit = 20
	}
	err := base.Order("created_at DESC").Offset(offset).Limit(limit).Find(&plans).Error
	return plans, int(total), err
}

func (r *GormRepository) ListByCluster(ctx context.Context, clusterID int64, offset, limit int) ([]Plan, int, error) {
	var total int64
	base := r.db.WithContext(ctx).Model(&Plan{}).
		Where("source_cluster_id = ? OR target_cluster_id = ?", clusterID, clusterID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var plans []Plan
	if limit <= 0 {
		limit = 20
	}
	err := base.Order("created_at DESC").Offset(offset).Limit(limit).Find(&plans).Error
	return plans, int(total), err
}

// ClaimAndLoad atomically transitions a plan from awaiting_confirmation to
// executing, re-checks the confirmation token and idempotency constraints,
// and takes a short lease on the plan (locked_at) so a concurrent Execute
// will see ErrInProgress.
//
// Idempotency replay: if the plan already has an idempotency key (from a
// prior Execute) the caller must present the same key. When the same key is
// presented on an already-completed (succeeded/failed/expired) plan we
// return the plan as-is so the Service can short-circuit and mirror the
// prior result.
func (r *GormRepository) ClaimAndLoad(ctx context.Context, id, idempotencyKey string, tokenHash []byte, newIdempotencyKey string) (Plan, error) {
	for {
		plan, err := r.GetByID(ctx, id)
		if err != nil {
			return Plan{}, err
		}
		// Idempotency replay: a *finished* plan with the same idempotency
		// key is returned unchanged. This is the success path for Execute
		// replays. Mismatched keys on finished plans surface the conflict.
		switch plan.Status {
		case StatusSucceeded, StatusFailed, StatusExpired:
			if plan.IdempotencyKey != "" {
				if plan.IdempotencyKey != idempotencyKey {
					return Plan{}, ErrInvalidIdempotency
				}
				return plan, nil
			}
			// No idempotency key stored on a finished plan is unusual —
			// surface it as already-executed so callers don't re-apply.
			return Plan{}, ErrAlreadyExecuted
		case StatusExecuting:
			if plan.LockedAt != nil && r.now().Sub(*plan.LockedAt) < claimLease {
				return Plan{}, ErrInProgress
			}
			// lease expired → fall through and treat it as resumable
			// (rebind back to awaiting_confirmation semantics below would
			// race, so we just claim the executing status again with a
			// fresh locked_at — no state transition needed).
		case StatusAwaitingConfirmation:
			// proceed with the confirmation check below.
		default:
			return Plan{}, ErrAlreadyExecuted
		}
		if r.now().After(plan.ExpiresAt) && plan.Status == StatusAwaitingConfirmation {
			// Best-effort mark expired; caller can re-request Preview.
			_ = r.UpdateStatus(ctx, plan.ID, StatusExpired, "")
			return Plan{}, ErrExpired
		}
		if plan.Status == StatusAwaitingConfirmation {
			if !constantTimeEqual(plan.ConfirmationTokenHash, tokenHash) {
				return Plan{}, ErrConfirmationInvalid
			}
			if plan.IdempotencyKey != "" {
				if plan.IdempotencyKey != idempotencyKey {
					return Plan{}, ErrInvalidIdempotency
				}
				// Same key on awaiting_confirmation means a previous
				// claim+execute attempt left the plan in an odd state.
				return plan, nil
			}
		}
		now := r.now()
		statusForUpdate := plan.Status
		expectedStatus := statusForUpdate
		// Awaiting plans transition to executing on claim; already-executing
		// plans just refresh their locked_at lease.
		if statusForUpdate == StatusAwaitingConfirmation {
			statusForUpdate = StatusExecuting
		}
		res := r.db.WithContext(ctx).Model(&Plan{}).
			Where("id = ? AND status = ? AND (locked_at IS NULL OR locked_at < ?)", plan.ID, expectedStatus, now.Add(-claimLease)).
			Updates(map[string]any{
				"status":          statusForUpdate,
				"locked_at":       now,
				"idempotency_key": newIdempotencyKey,
				"updated_at":      now,
			})
		if res.Error != nil {
			return Plan{}, res.Error
		}
		if res.RowsAffected == 0 {
			continue // lost the race — retry the loop.
		}
		// Reload the updated row.
		return r.GetByID(ctx, id)
	}
}

// UpdateExecution persists the post-execute plan (status, last_error,
// executed_at, and updated resource_items JSONB with per-item execution
// bookkeeping).
func (r *GormRepository) UpdateExecution(ctx context.Context, plan Plan) error {
	now := r.now()
	plan.UpdatedAt = now
	return r.db.WithContext(ctx).Model(&Plan{}).Where("id = ?", plan.ID).Updates(map[string]any{
		"status":         plan.Status,
		"last_error":     plan.LastError,
		"executed_at":    plan.ExecutedAt,
		"resource_items": plan.ResourceItems,
		"locked_at":      nil,
		"updated_at":     now,
	}).Error
}

// UpdateStatus is a best-effort helper for mark-expired / mark-failed
// transitions outside the ClaimAndLoad state machine.
func (r *GormRepository) UpdateStatus(ctx context.Context, id, status, lastError string) error {
	return r.db.WithContext(ctx).Model(&Plan{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"last_error": lastError,
		"updated_at": r.now(),
	}).Error
}

// constantTimeEqual compares two byte slices in constant-ish time (up to
// the length of the longer slice). Used for comparing SHA256 token hashes.
func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
