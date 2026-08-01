package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists action plans and verifications. The service is the
// only writer.
type Repository interface {
	// SavePlan persists a new action plan. The plan must be in draft or
	// previewed status. Subsequent transitions use the targeted update
	// methods (MarkPreviewed, Approve, Claim, Complete, Fail, MarkVerified,
	// Cancel, Expire).
	SavePlan(ctx context.Context, plan *ActionPlan) error
	// GetPlan returns one action plan by ID.
	GetPlan(ctx context.Context, id string) (ActionPlan, error)
	// GetPlanForExecute loads a plan with a row lock for the execute
	// claim transition. Returns ErrPlanNotFound when missing.
	GetPlanForExecute(ctx context.Context, id string) (ActionPlan, error)
	// ListPlans returns plans matching the filter, ordered by created_at DESC.
	ListPlans(ctx context.Context, filter ActionPlanFilter) ([]ActionPlan, int64, error)
	// CountAttemptsSince returns the number of non-cancelled plans for
	// the same cluster_id + target_uid within the window. Used by the
	// attempt_cap gate.
	CountAttemptsSince(ctx context.Context, clusterID int64, targetUID string, since time.Time) (int, error)
	// CountConcurrentPlans returns the number of non-terminal plans for
	// the same cluster_id + target_uid (excluding the supplied plan ID).
	// Used by the concurrent_plans gate.
	CountConcurrentPlans(ctx context.Context, clusterID int64, targetUID, excludePlanID string) (int, error)
	// MarkPreviewed transitions a draft plan to previewed with the policy
	// gate results. Returns the updated plan.
	MarkPreviewed(ctx context.Context, id string, gates []PolicyGate, now time.Time) (ActionPlan, error)
	// Approve records the approver and transitions a previewed plan to
	// approved. The service enforces four-eyes; the repository trusts the
	// caller.
	Approve(ctx context.Context, id string, approver ActorRef, now time.Time) (ActionPlan, error)
	// Claim takes an idempotent executing claim on an approved plan.
	// Mirrors remediation/maintenance Claim: returns shouldExecute=true
	// exactly once per idempotency key; replay returns the persisted plan
	// with shouldExecute=false.
	Claim(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (ActionPlan, bool, error)
	// Complete transitions an executing plan to succeeded.
	Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time) (ActionPlan, error)
	// Fail transitions an executing plan to failed with a sanitized last
	// error message.
	Fail(ctx context.Context, id, idempotencyKey, message string) (ActionPlan, error)
	// MarkVerified links the verification ID and transitions the plan to
	// verified.
	MarkVerified(ctx context.Context, id string, verificationID int64, now time.Time) (ActionPlan, error)
	// Cancel transitions a non-terminal plan to cancelled.
	Cancel(ctx context.Context, id string, now time.Time) (ActionPlan, error)
	// ExpireStale transitions awaiting plans past their TTL to expired.
	// Returns the count of expired plans.
	ExpireStale(ctx context.Context, now time.Time) (int64, error)

	// SaveVerification persists a new verification row. One verification
	// per (plan_id, verification_key); duplicate inserts are ignored.
	SaveVerification(ctx context.Context, verification *ActionVerification) error
	// GetVerification returns one verification by ID.
	GetVerification(ctx context.Context, id int64) (ActionVerification, error)
	// GetVerificationByPlan returns the most recent verification for a
	// plan, or ErrVerificationNotFound when none exists.
	GetVerificationByPlan(ctx context.Context, planID string) (ActionVerification, error)
	// UpdateVerification patches a pending verification with the post
	// snapshot and final status. Used by the verifier worker after the
	// cooldown elapses.
	UpdateVerification(ctx context.Context, id int64, update VerificationUpdate) (ActionVerification, error)
}

// VerificationUpdate is the patch applied by the verifier worker. All
// fields are optional; only supplied fields are updated.
type VerificationUpdate struct {
	Status             VerificationStatus
	EvidenceComparison EvidenceComparison
	PostSnapshot       *EvidenceSnapshot
	MissingEvidence    *bool
	VerifiedAt         *time.Time
	Reason             string
	RollbackTriggered  *bool
	RollbackPlanID     *string
}

// ErrPlanNotFound is returned when an action plan does not exist.
var ErrPlanNotFound = errors.New("action plan not found")

// ErrVerificationNotFound is returned when a verification does not exist.
var ErrVerificationNotFound = errors.New("action verification not found")

// Sentinel errors for plan lifecycle transitions. These mirror the
// remediation/maintenance patterns so HTTP handlers can map them to stable
// responses.
var (
	// ErrConfirmationInvalid: confirmation token does not match.
	ErrConfirmationInvalid = errors.New("action plan confirmation is invalid")
	// ErrExpired: plan TTL elapsed before approval or execute.
	ErrExpired = errors.New("action plan expired")
	// ErrInProgress: plan is currently executing under another worker; the
	// claim has not exceeded the stale-before cutoff.
	ErrInProgress = errors.New("action plan execution is in progress")
	// ErrAlreadyExecuted: plan already executed under a different
	// idempotency key.
	ErrAlreadyExecuted = errors.New("action plan already used with another idempotency key")
	// ErrNotApproved: plan is not in the approved status required to claim.
	ErrNotApproved = errors.New("action plan is not approved")
)

// --- GORM models ---

type actionPlanRow struct {
	ID                                string     `gorm:"column:id;primaryKey"`
	PlanKey                           string     `gorm:"column:plan_key;not null"`
	AutomationVersion                 string     `gorm:"column:automation_version;not null;default:1.0"`
	CaseID                            int64      `gorm:"column:case_id;not null"`
	InvestigationID                   *int64     `gorm:"column:investigation_id"`
	ActionCandidateID                 *int64     `gorm:"column:action_candidate_id"`
	RunbookID                         string     `gorm:"column:runbook_id;not null"`
	ActionCode                        string     `gorm:"column:action_code;not null"`
	ClusterID                         int64      `gorm:"column:cluster_id;not null"`
	TargetKind                        string     `gorm:"column:target_kind;not null"`
	TargetNamespace                   string     `gorm:"column:target_namespace;not null"`
	TargetName                        string     `gorm:"column:target_name;not null"`
	TargetUID                         string     `gorm:"column:target_uid;not null;default:''"`
	TargetResourceVersion             string     `gorm:"column:target_resource_version;not null;default:''"`
	DesiredReplicas                   *int32     `gorm:"column:desired_replicas"`
	BeforeReplicas                    *int32     `gorm:"column:before_replicas"`
	DesiredSuspended                  *bool      `gorm:"column:desired_suspended"`
	BeforeSuspended                   *bool      `gorm:"column:before_suspended"`
	ContainerName                     string     `gorm:"column:container_name;not null;default:''"`
	BeforeImage                       string     `gorm:"column:before_image;not null;default:''"`
	DesiredImage                      string     `gorm:"column:desired_image;not null;default:''"`
	RollbackRevision                  *int32     `gorm:"column:rollback_revision"`
	RollbackReplicaSetName            string     `gorm:"column:rollback_replicaset_name;not null;default:''"`
	RollbackReplicaSetUID             string     `gorm:"column:rollback_replicaset_uid;not null;default:''"`
	RollbackReplicaSetResourceVersion string     `gorm:"column:rollback_replicaset_resource_version;not null;default:''"`
	PolicyGates                       JSONB      `gorm:"column:policy_gates;not null;default:'[]'"`
	Status                            string     `gorm:"column:status;not null;default:draft"`
	Level                             string     `gorm:"column:level;not null;default:L2"`
	ApprovalType                      string     `gorm:"column:approval_type;not null;default:single"`
	RequestedByUserID                 *int64     `gorm:"column:requested_by_user_id"`
	RequestedByName                   string     `gorm:"column:requested_by_name;not null;default:''"`
	ApproverUserID                    *int64     `gorm:"column:approver_user_id"`
	ApproverName                      string     `gorm:"column:approver_name;not null;default:''"`
	ApprovedAt                        *time.Time `gorm:"column:approved_at"`
	ConfirmationTokenHash             []byte     `gorm:"column:confirmation_token_hash;not null"`
	IdempotencyKey                    string     `gorm:"column:idempotency_key;not null;default:''"`
	ExpiresAt                         time.Time  `gorm:"column:expires_at;not null"`
	LockedAt                          *time.Time `gorm:"column:locked_at"`
	ExecutedAt                        *time.Time `gorm:"column:executed_at"`
	AttemptCount                      int        `gorm:"column:attempt_count;not null;default:0"`
	LastError                         string     `gorm:"column:last_error;not null;default:''"`
	VerificationID                    *int64     `gorm:"column:verification_id"`
	CorrelationRequestID              string     `gorm:"column:correlation_request_id;not null;default:''"`
	CreatedAt                         time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt                         time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (actionPlanRow) TableName() string { return "action_plans" }

type actionVerificationRow struct {
	ID                    int64      `gorm:"column:id;primaryKey;autoIncrement"`
	PlanID                string     `gorm:"column:plan_id;not null"`
	VerificationKey       string     `gorm:"column:verification_key;not null"`
	VerifierVersion       string     `gorm:"column:verifier_version;not null;default:1.0"`
	Status                string     `gorm:"column:status;not null;default:pending"`
	EvidenceComparison    string     `gorm:"column:evidence_comparison;not null;default:insufficient"`
	PreSnapshot           JSONB      `gorm:"column:pre_snapshot;not null;default:'{}'"`
	PostSnapshot          JSONB      `gorm:"column:post_snapshot;not null;default:'{}'"`
	SLOEvaluationBeforeID *int64     `gorm:"column:slo_evaluation_before_id"`
	SLOEvaluationAfterID  *int64     `gorm:"column:slo_evaluation_after_id"`
	MissingEvidence       bool       `gorm:"column:missing_evidence;not null;default:false"`
	CooldownSeconds       int        `gorm:"column:cooldown_seconds;not null;default:300"`
	VerifiedAt            *time.Time `gorm:"column:verified_at"`
	Reason                string     `gorm:"column:reason;not null;default:''"`
	RollbackTriggered     bool       `gorm:"column:rollback_triggered;not null;default:false"`
	RollbackPlanID        *string    `gorm:"column:rollback_plan_id"`
	CreatedAt             time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt             time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (actionVerificationRow) TableName() string { return "action_verifications" }

// JSONB is a json.Marshaler/Unmarshaler wrapper for JSONB columns, mirroring
// the M43 aiinvestigator pattern.
type JSONB json.RawMessage

func (j JSONB) Value() (interface{}, error) {
	if len(j) == 0 {
		return []byte("[]"), nil
	}
	return []byte(j), nil
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = JSONB("[]")
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append(JSONB{}, v...)
	case string:
		*j = append(JSONB{}, v...)
	}
	return nil
}

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("[]"), nil
	}
	return []byte(j), nil
}

func (j *JSONB) UnmarshalJSON(data []byte) error {
	if j == nil {
		return nil
	}
	*j = append(JSONB{}, data...)
	return nil
}

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// NopRepository is a no-op repository for disabled/testing mode.
type NopRepository struct{}

func (NopRepository) SavePlan(context.Context, *ActionPlan) error { return nil }
func (NopRepository) GetPlan(context.Context, string) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) GetPlanForExecute(context.Context, string) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) ListPlans(context.Context, ActionPlanFilter) ([]ActionPlan, int64, error) {
	return nil, 0, nil
}
func (NopRepository) CountAttemptsSince(context.Context, int64, string, time.Time) (int, error) {
	return 0, nil
}
func (NopRepository) CountConcurrentPlans(context.Context, int64, string, string) (int, error) {
	return 0, nil
}
func (NopRepository) MarkPreviewed(context.Context, string, []PolicyGate, time.Time) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) Approve(context.Context, string, ActorRef, time.Time) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) Claim(context.Context, string, []byte, string, time.Time, time.Time) (ActionPlan, bool, error) {
	return ActionPlan{}, false, ErrPlanNotFound
}
func (NopRepository) Complete(context.Context, string, string, time.Time) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) Fail(context.Context, string, string, string) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) MarkVerified(context.Context, string, int64, time.Time) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) Cancel(context.Context, string, time.Time) (ActionPlan, error) {
	return ActionPlan{}, ErrPlanNotFound
}
func (NopRepository) ExpireStale(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (NopRepository) SaveVerification(context.Context, *ActionVerification) error { return nil }
func (NopRepository) GetVerification(context.Context, int64) (ActionVerification, error) {
	return ActionVerification{}, ErrVerificationNotFound
}
func (NopRepository) GetVerificationByPlan(context.Context, string) (ActionVerification, error) {
	return ActionVerification{}, ErrVerificationNotFound
}
func (NopRepository) UpdateVerification(context.Context, int64, VerificationUpdate) (ActionVerification, error) {
	return ActionVerification{}, ErrVerificationNotFound
}

// --- SavePlan / GetPlan / ListPlans ---

func (r *GormRepository) SavePlan(ctx context.Context, plan *ActionPlan) error {
	row := planToRow(plan)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	plan.ID = row.ID
	plan.CreatedAt = row.CreatedAt
	plan.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *GormRepository) GetPlan(ctx context.Context, id string) (ActionPlan, error) {
	var row actionPlanRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ActionPlan{}, ErrPlanNotFound
		}
		return ActionPlan{}, err
	}
	return rowToPlan(&row), nil
}

func (r *GormRepository) GetPlanForExecute(ctx context.Context, id string) (ActionPlan, error) {
	var row actionPlanRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&row).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ActionPlan{}, ErrPlanNotFound
		}
		return ActionPlan{}, err
	}
	return rowToPlan(&row), nil
}

func (r *GormRepository) ListPlans(ctx context.Context, filter ActionPlanFilter) ([]ActionPlan, int64, error) {
	q := r.db.WithContext(ctx).Model(&actionPlanRow{})
	if filter.CaseID > 0 {
		q = q.Where("case_id = ?", filter.CaseID)
	}
	if filter.ClusterID > 0 {
		q = q.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.Namespace != "" {
		q = q.Where("target_namespace = ?", filter.Namespace)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", string(filter.Status))
	}
	if filter.RunbookID != "" {
		q = q.Where("runbook_id = ?", filter.RunbookID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []actionPlanRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ActionPlan, 0, len(rows))
	for i := range rows {
		items = append(items, rowToPlan(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) CountAttemptsSince(ctx context.Context, clusterID int64, targetUID string, since time.Time) (int, error) {
	if targetUID == "" {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("cluster_id = ? AND target_uid = ? AND created_at >= ? AND status != ?",
			clusterID, targetUID, since, string(StatusCancelled)).
		Count(&count).Error
	return int(count), err
}

func (r *GormRepository) CountConcurrentPlans(ctx context.Context, clusterID int64, targetUID, excludePlanID string) (int, error) {
	if targetUID == "" {
		return 0, nil
	}
	q := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("cluster_id = ? AND target_uid = ? AND status IN ?",
			clusterID, targetUID,
			[]string{string(StatusDraft), string(StatusPreviewed), string(StatusApproved), string(StatusExecuting)})
	if excludePlanID != "" {
		q = q.Where("id <> ?", excludePlanID)
	}
	var count int64
	err := q.Count(&count).Error
	return int(count), err
}

// --- transitions ---

func (r *GormRepository) MarkPreviewed(ctx context.Context, id string, gates []PolicyGate, now time.Time) (ActionPlan, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status = ?", id, string(StatusDraft)).
		Updates(map[string]any{
			"status":       string(StatusPreviewed),
			"policy_gates": mustMarshalJSONB(gates),
			"updated_at":   now,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) Approve(ctx context.Context, id string, approver ActorRef, now time.Time) (ActionPlan, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status = ?", id, string(StatusPreviewed)).
		Updates(map[string]any{
			"status":           string(StatusApproved),
			"approver_user_id": approver.ID,
			"approver_name":    approver.Name,
			"approved_at":      now,
			"updated_at":       now,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) Claim(ctx context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (ActionPlan, bool, error) {
	var row actionPlanRow
	var claimErr error
	shouldExecute := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				claimErr = ErrPlanNotFound
				return nil
			}
			return err
		}
		// Confirmation token check (constant time).
		if !bytesEqual(row.ConfirmationTokenHash, tokenHash) {
			claimErr = ErrConfirmationInvalid
			return nil
		}
		// Expired?
		if row.Status == string(StatusExpired) || (row.Status == string(StatusApproved) && !row.ExpiresAt.After(now)) {
			if row.Status != string(StatusExpired) {
				if err := tx.Model(&row).Updates(map[string]any{"status": string(StatusExpired), "updated_at": now}).Error; err != nil {
					return err
				}
				row.Status = string(StatusExpired)
			}
			claimErr = ErrExpired
			return nil
		}
		switch PlanStatus(row.Status) {
		case StatusApproved:
			if err := tx.Model(&row).Updates(map[string]any{
				"status":          string(StatusExecuting),
				"idempotency_key": idempotencyKey,
				"locked_at":       now,
				"last_error":      "",
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
			row.Status = string(StatusExecuting)
			row.IdempotencyKey = idempotencyKey
			row.LockedAt = &now
			shouldExecute = true
		case StatusExecuting:
			if row.IdempotencyKey != idempotencyKey {
				claimErr = ErrAlreadyExecuted
				return nil
			}
			if row.LockedAt != nil && row.LockedAt.After(staleBefore) {
				claimErr = ErrInProgress
				return nil
			}
			if err := tx.Model(&row).Updates(map[string]any{"locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			row.LockedAt = &now
			shouldExecute = true
		case StatusSucceeded, StatusFailed:
			if row.IdempotencyKey != idempotencyKey {
				claimErr = ErrAlreadyExecuted
				return nil
			}
			// Idempotent replay: no business side effect.
		default:
			claimErr = ErrAlreadyExecuted
		}
		return nil
	})
	if err != nil {
		return ActionPlan{}, false, err
	}
	if claimErr != nil {
		return rowToPlan(&row), shouldExecute, claimErr
	}
	return rowToPlan(&row), shouldExecute, nil
}

func (r *GormRepository) Complete(ctx context.Context, id, idempotencyKey string, executedAt time.Time) (ActionPlan, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status = ? AND idempotency_key = ?", id, string(StatusExecuting), idempotencyKey).
		Updates(map[string]any{
			"status":      string(StatusSucceeded),
			"executed_at": executedAt,
			"locked_at":   nil,
			"last_error":  "",
			"updated_at":  executedAt,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) Fail(ctx context.Context, id, idempotencyKey, message string) (ActionPlan, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status = ? AND idempotency_key = ?", id, string(StatusExecuting), idempotencyKey).
		Updates(map[string]any{
			"status":     string(StatusFailed),
			"locked_at":  nil,
			"last_error": message,
			"updated_at": now,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) MarkVerified(ctx context.Context, id string, verificationID int64, now time.Time) (ActionPlan, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status IN ?", id, []string{string(StatusSucceeded), string(StatusFailed)}).
		Updates(map[string]any{
			"status":          string(StatusVerified),
			"verification_id": verificationID,
			"updated_at":      now,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) Cancel(ctx context.Context, id string, now time.Time) (ActionPlan, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("id = ? AND status IN ?", id, []string{string(StatusDraft), string(StatusPreviewed), string(StatusApproved)}).
		Updates(map[string]any{
			"status":     string(StatusCancelled),
			"updated_at": now,
		})
	if result.Error != nil {
		return ActionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionPlan{}, ErrPlanNotFound
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) ExpireStale(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&actionPlanRow{}).
		Where("status IN ? AND expires_at <= ?", []string{string(StatusDraft), string(StatusPreviewed), string(StatusApproved)}, now).
		Update("status", string(StatusExpired))
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// --- verifications ---

func (r *GormRepository) SaveVerification(ctx context.Context, verification *ActionVerification) error {
	row := verificationToRow(verification)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	verification.ID = row.ID
	verification.CreatedAt = row.CreatedAt
	verification.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *GormRepository) GetVerification(ctx context.Context, id int64) (ActionVerification, error) {
	var row actionVerificationRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ActionVerification{}, ErrVerificationNotFound
		}
		return ActionVerification{}, err
	}
	return rowToVerification(&row), nil
}

func (r *GormRepository) GetVerificationByPlan(ctx context.Context, planID string) (ActionVerification, error) {
	var row actionVerificationRow
	if err := r.db.WithContext(ctx).Where("plan_id = ?", planID).Order("created_at DESC, id DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ActionVerification{}, ErrVerificationNotFound
		}
		return ActionVerification{}, err
	}
	return rowToVerification(&row), nil
}

func (r *GormRepository) UpdateVerification(ctx context.Context, id int64, update VerificationUpdate) (ActionVerification, error) {
	patch := map[string]any{}
	if update.Status != "" {
		patch["status"] = string(update.Status)
	}
	if update.EvidenceComparison != "" {
		patch["evidence_comparison"] = string(update.EvidenceComparison)
	}
	if update.PostSnapshot != nil {
		patch["post_snapshot"] = mustMarshalJSONB(*update.PostSnapshot)
	}
	if update.MissingEvidence != nil {
		patch["missing_evidence"] = *update.MissingEvidence
	}
	if update.VerifiedAt != nil {
		patch["verified_at"] = *update.VerifiedAt
	}
	if update.Reason != "" {
		patch["reason"] = update.Reason
	}
	if update.RollbackTriggered != nil {
		patch["rollback_triggered"] = *update.RollbackTriggered
	}
	if update.RollbackPlanID != nil {
		patch["rollback_plan_id"] = *update.RollbackPlanID
	}
	if len(patch) == 0 {
		return r.GetVerification(ctx, id)
	}
	patch["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&actionVerificationRow{}).Where("id = ?", id).Updates(patch)
	if result.Error != nil {
		return ActionVerification{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ActionVerification{}, ErrVerificationNotFound
	}
	return r.GetVerification(ctx, id)
}

// --- row conversion helpers ---

func planToRow(plan *ActionPlan) actionPlanRow {
	return actionPlanRow{
		ID:                                plan.ID,
		PlanKey:                           plan.PlanKey,
		AutomationVersion:                 plan.AutomationVersion,
		CaseID:                            plan.CaseID,
		InvestigationID:                   plan.InvestigationID,
		ActionCandidateID:                 plan.ActionCandidateID,
		RunbookID:                         plan.RunbookID,
		ActionCode:                        plan.ActionCode,
		ClusterID:                         plan.ClusterID,
		TargetKind:                        plan.TargetKind,
		TargetNamespace:                   plan.TargetNamespace,
		TargetName:                        plan.TargetName,
		TargetUID:                         plan.TargetUID,
		TargetResourceVersion:             plan.TargetResourceVersion,
		DesiredReplicas:                   plan.DesiredReplicas,
		BeforeReplicas:                    plan.BeforeReplicas,
		DesiredSuspended:                  plan.DesiredSuspended,
		BeforeSuspended:                   plan.BeforeSuspended,
		ContainerName:                     plan.ContainerName,
		BeforeImage:                       plan.BeforeImage,
		DesiredImage:                      plan.DesiredImage,
		RollbackRevision:                  plan.RollbackRevision,
		RollbackReplicaSetName:            plan.RollbackReplicaSetName,
		RollbackReplicaSetUID:             plan.RollbackReplicaSetUID,
		RollbackReplicaSetResourceVersion: plan.RollbackReplicaSetResourceVersion,
		PolicyGates:                       mustMarshalJSONB(plan.PolicyGates),
		Status:                            string(plan.Status),
		Level:                             string(plan.Level),
		ApprovalType:                      string(plan.ApprovalType),
		RequestedByUserID:                 plan.RequestedByUserID,
		RequestedByName:                   plan.RequestedByName,
		ApproverUserID:                    plan.ApproverUserID,
		ApproverName:                      plan.ApproverName,
		ApprovedAt:                        plan.ApprovedAt,
		ConfirmationTokenHash:             plan.ConfirmationTokenHash,
		IdempotencyKey:                    plan.IdempotencyKey,
		ExpiresAt:                         plan.ExpiresAt,
		LockedAt:                          plan.LockedAt,
		ExecutedAt:                        plan.ExecutedAt,
		AttemptCount:                      plan.AttemptCount,
		LastError:                         plan.LastError,
		VerificationID:                    plan.VerificationID,
		CorrelationRequestID:              plan.CorrelationRequestID,
		CreatedAt:                         plan.CreatedAt,
		UpdatedAt:                         plan.UpdatedAt,
	}
}

func rowToPlan(row *actionPlanRow) ActionPlan {
	return ActionPlan{
		ID:                                row.ID,
		PlanKey:                           row.PlanKey,
		AutomationVersion:                 row.AutomationVersion,
		CaseID:                            row.CaseID,
		InvestigationID:                   row.InvestigationID,
		ActionCandidateID:                 row.ActionCandidateID,
		RunbookID:                         row.RunbookID,
		ActionCode:                        row.ActionCode,
		ClusterID:                         row.ClusterID,
		TargetKind:                        row.TargetKind,
		TargetNamespace:                   row.TargetNamespace,
		TargetName:                        row.TargetName,
		TargetUID:                         row.TargetUID,
		TargetResourceVersion:             row.TargetResourceVersion,
		DesiredReplicas:                   row.DesiredReplicas,
		BeforeReplicas:                    row.BeforeReplicas,
		DesiredSuspended:                  row.DesiredSuspended,
		BeforeSuspended:                   row.BeforeSuspended,
		ContainerName:                     row.ContainerName,
		BeforeImage:                       row.BeforeImage,
		DesiredImage:                      row.DesiredImage,
		RollbackRevision:                  row.RollbackRevision,
		RollbackReplicaSetName:            row.RollbackReplicaSetName,
		RollbackReplicaSetUID:             row.RollbackReplicaSetUID,
		RollbackReplicaSetResourceVersion: row.RollbackReplicaSetResourceVersion,
		PolicyGates:                       unmarshalPolicyGates(row.PolicyGates),
		Status:                            PlanStatus(row.Status),
		Level:                             AutomationLevel(row.Level),
		ApprovalType:                      ApprovalType(row.ApprovalType),
		RequestedByUserID:                 row.RequestedByUserID,
		RequestedByName:                   row.RequestedByName,
		ApproverUserID:                    row.ApproverUserID,
		ApproverName:                      row.ApproverName,
		ApprovedAt:                        row.ApprovedAt,
		ConfirmationTokenHash:             row.ConfirmationTokenHash,
		IdempotencyKey:                    row.IdempotencyKey,
		ExpiresAt:                         row.ExpiresAt,
		LockedAt:                          row.LockedAt,
		ExecutedAt:                        row.ExecutedAt,
		AttemptCount:                      row.AttemptCount,
		LastError:                         row.LastError,
		VerificationID:                    row.VerificationID,
		CorrelationRequestID:              row.CorrelationRequestID,
		CreatedAt:                         row.CreatedAt,
		UpdatedAt:                         row.UpdatedAt,
	}
}

func verificationToRow(v *ActionVerification) actionVerificationRow {
	return actionVerificationRow{
		ID:                    v.ID,
		PlanID:                v.PlanID,
		VerificationKey:       v.VerificationKey,
		VerifierVersion:       v.VerifierVersion,
		Status:                string(v.Status),
		EvidenceComparison:    string(v.EvidenceComparison),
		PreSnapshot:           mustMarshalJSONB(v.PreSnapshot),
		PostSnapshot:          mustMarshalJSONB(v.PostSnapshot),
		SLOEvaluationBeforeID: v.SLOEvaluationBeforeID,
		SLOEvaluationAfterID:  v.SLOEvaluationAfterID,
		MissingEvidence:       v.MissingEvidence,
		CooldownSeconds:       v.CooldownSeconds,
		VerifiedAt:            v.VerifiedAt,
		Reason:                v.Reason,
		RollbackTriggered:     v.RollbackTriggered,
		RollbackPlanID:        v.RollbackPlanID,
		CreatedAt:             v.CreatedAt,
		UpdatedAt:             v.UpdatedAt,
	}
}

func rowToVerification(row *actionVerificationRow) ActionVerification {
	return ActionVerification{
		ID:                    row.ID,
		PlanID:                row.PlanID,
		VerificationKey:       row.VerificationKey,
		VerifierVersion:       row.VerifierVersion,
		Status:                VerificationStatus(row.Status),
		EvidenceComparison:    EvidenceComparison(row.EvidenceComparison),
		PreSnapshot:           unmarshalEvidenceSnapshot(row.PreSnapshot),
		PostSnapshot:          unmarshalEvidenceSnapshot(row.PostSnapshot),
		SLOEvaluationBeforeID: row.SLOEvaluationBeforeID,
		SLOEvaluationAfterID:  row.SLOEvaluationAfterID,
		MissingEvidence:       row.MissingEvidence,
		CooldownSeconds:       row.CooldownSeconds,
		VerifiedAt:            row.VerifiedAt,
		Reason:                row.Reason,
		RollbackTriggered:     row.RollbackTriggered,
		RollbackPlanID:        row.RollbackPlanID,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}

func mustMarshalJSONB(v interface{}) JSONB {
	if v == nil {
		return JSONB("[]")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return JSONB("[]")
	}
	return JSONB(b)
}

func unmarshalPolicyGates(j JSONB) []PolicyGate {
	if len(j) == 0 {
		return nil
	}
	var out []PolicyGate
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalEvidenceSnapshot(j JSONB) EvidenceSnapshot {
	if len(j) == 0 {
		return EvidenceSnapshot{}
	}
	var out EvidenceSnapshot
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return EvidenceSnapshot{}
	}
	return out
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ Repository = (*GormRepository)(nil)
var _ Repository = (*NopRepository)(nil)
