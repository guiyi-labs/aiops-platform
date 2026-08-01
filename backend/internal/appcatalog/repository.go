package appcatalog

import (
	"context"
	"crypto/subtle"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DataStore is the persistence boundary for the appcatalog package. It covers
// both helm_repositories CRUD and the M19 controlled-operation Claim state
// machine for app_catalog_plans (mirrors promotion.Repository).
type DataStore interface {
	// Helm repository CRUD.
	SaveRepo(ctx context.Context, repo *Repository) error
	GetRepo(ctx context.Context, id int64) (Repository, error)
	GetRepoByName(ctx context.Context, name string) (Repository, error)
	ListRepos(ctx context.Context) ([]Repository, error)
	DeleteRepo(ctx context.Context, id int64) error

	// Plan lifecycle (M19 controlled-operation contract).
	SavePlan(ctx context.Context, plan *Plan) error
	GetPlan(ctx context.Context, id string) (Plan, error)
	ListPlans(ctx context.Context, clusterID int64, namespace string) ([]Plan, error)
	ClaimPlan(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (Plan, bool, error)
	CompletePlan(ctx context.Context, id, idempotencyKey string, executedAt time.Time) (Plan, error)
	FailPlan(ctx context.Context, id, idempotencyKey, message string) (Plan, error)
	ExpireStalePlans(ctx context.Context, now time.Time) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// ---------------------------------------------------------------------------
// Helm repository CRUD
// ---------------------------------------------------------------------------

func (r *GormRepository) SaveRepo(ctx context.Context, repo *Repository) error {
	return r.db.WithContext(ctx).Save(repo).Error
}

func (r *GormRepository) GetRepo(ctx context.Context, id int64) (Repository, error) {
	var repo Repository
	err := r.db.WithContext(ctx).First(&repo, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return Repository{}, ErrRepoNotFound
		}
		return Repository{}, err
	}
	return repo, nil
}

func (r *GormRepository) GetRepoByName(ctx context.Context, name string) (Repository, error) {
	var repo Repository
	err := r.db.WithContext(ctx).First(&repo, "name = ?", name).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return Repository{}, ErrRepoNotFound
		}
		return Repository{}, err
	}
	return repo, nil
}

func (r *GormRepository) ListRepos(ctx context.Context) ([]Repository, error) {
	var repos []Repository
	err := r.db.WithContext(ctx).Order("name ASC").Find(&repos).Error
	return repos, err
}

func (r *GormRepository) DeleteRepo(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Repository{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRepoNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Plan lifecycle (M19 controlled-operation contract — mirrors promotion)
// ---------------------------------------------------------------------------

func (r *GormRepository) SavePlan(ctx context.Context, plan *Plan) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

func (r *GormRepository) GetPlan(ctx context.Context, id string) (Plan, error) {
	var plan Plan
	err := r.db.WithContext(ctx).First(&plan, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return Plan{}, ErrPlanNotFound
		}
		return Plan{}, err
	}
	return plan, nil
}

func (r *GormRepository) ListPlans(ctx context.Context, clusterID int64, namespace string) ([]Plan, error) {
	if err := r.ExpireStalePlans(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx).Model(&Plan{}).Where("target_cluster_id = ?", clusterID)
	if namespace != "" {
		query = query.Where("target_namespace = ?", namespace)
	}
	var plans []Plan
	err := query.Order("created_at DESC, id DESC").Limit(50).Find(&plans).Error
	return plans, err
}

// ClaimPlan implements the M19 idempotent Claim state machine. It mirrors
// promotion.GormRepository.Claim: acquires a row lock, validates the
// confirmation token, and transitions awaiting_confirmation → executing
// (or resumes an in-progress execution with the same idempotency key).
func (r *GormRepository) ClaimPlan(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (Plan, bool, error) {
	var plan Plan
	var claimErr error
	shouldExecute := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				claimErr = ErrPlanNotFound
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
			if err := tx.Model(&plan).Updates(map[string]any{
				"status":          StatusExecuting,
				"idempotency_key": idempotencyKey,
				"locked_at":       now,
				"last_error":      "",
				"updated_at":      now,
			}).Error; err != nil {
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

func (r *GormRepository) CompletePlan(ctx context.Context, id, idempotencyKey string, executedAt time.Time) (Plan, error) {
	result := r.db.WithContext(ctx).Model(&Plan{}).
		Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(map[string]any{
			"status":      StatusSucceeded,
			"executed_at": executedAt,
			"locked_at":   nil,
			"last_error":  "",
			"updated_at":  executedAt,
		})
	if result.Error != nil {
		return Plan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Plan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) FailPlan(ctx context.Context, id, idempotencyKey, message string) (Plan, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&Plan{}).
		Where("id = ? AND status = ? AND idempotency_key = ?", id, StatusExecuting, idempotencyKey).
		Updates(map[string]any{
			"status":     StatusFailed,
			"locked_at":  nil,
			"last_error": message,
			"updated_at": now,
		})
	if result.Error != nil {
		return Plan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Plan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) ExpireStalePlans(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Model(&Plan{}).
		Where("status = ? AND expires_at <= ?", StatusAwaitingConfirmation, now).
		Updates(map[string]any{"status": StatusExpired, "updated_at": now}).Error
}

var _ DataStore = (*GormRepository)(nil)
