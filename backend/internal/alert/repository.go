package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"

	"gorm.io/gorm"
)

type Repository interface {
	CreateRule(ctx context.Context, rule *Rule, minEvaluationInterval time.Duration) error
	GetRule(ctx context.Context, id int64) (Rule, error)
	ListRules(ctx context.Context, filter RuleListFilter) ([]Rule, error)
	PatchRule(ctx context.Context, id int64, input PatchRuleInput, actor ActorRef) (Rule, error)
	DeleteRule(ctx context.Context, id int64) error
	GetUnresolvedInstance(ctx context.Context, ruleID int64) (*Instance, error)
	CreateInstance(ctx context.Context, instance *Instance) error
	CreateFiring(ctx context.Context, record *diagnosis.Record, instance *Instance) error
	TouchInstance(ctx context.Context, ruleID int64, lastFiredAt time.Time, evidenceAnchor string) error
	ResolveInstance(ctx context.Context, ruleID int64, resolvedAt time.Time) error
	ListInstances(ctx context.Context, filter InstanceListFilter) ([]Instance, error)
	GetInstance(ctx context.Context, id int64) (Instance, error)
	ClaimDueRules(ctx context.Context, now time.Time, batchSize int, claimLease time.Duration) ([]Rule, error)
	ReleaseClaim(ctx context.Context, ruleID int64, nextDueAt time.Time, evalState string, evalAt time.Time, errCode string) error
	UpdateRuleHealth(ctx context.Context, ruleID int64, evalState string, evalAt time.Time, errCode string) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) CreateRule(ctx context.Context, rule *Rule, minEvaluationInterval time.Duration) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Raw(`SELECT COUNT(*) FROM alert_rules WHERE cluster_id = ? AND deleted = FALSE`, rule.ClusterID).Row().Scan(&count); err != nil {
			return fmt.Errorf("count alert rules: %w", err)
		}
		if count >= int64(MaxRulesPerCluster) {
			return ErrClusterLimit
		}
		var nameConflict int64
		if err := tx.Raw(`SELECT COUNT(*) FROM alert_rules WHERE cluster_id = ? AND deleted = FALSE AND LOWER(display_name) = LOWER(?)`, rule.ClusterID, rule.DisplayName).Row().Scan(&nameConflict); err != nil {
			return fmt.Errorf("check name uniqueness: %w", err)
		}
		if nameConflict > 0 {
			return ErrDuplicateName
		}
		row := tx.Raw(`INSERT INTO alert_rules
			(cluster_id, display_name, resource_kind, resource_name, metric_name, operator, threshold, for_seconds, minimum_points,
			 enabled, deleted, last_evaluation_state, last_error_code, next_due_at, creator_user_id, creator_name)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE, FALSE, '', '', ?, ?, ?)
			RETURNING id, created_at, updated_at`,
			rule.ClusterID, rule.DisplayName, rule.ResourceKind, rule.ResourceName, rule.MetricName, rule.Operator,
			rule.Threshold, rule.ForSeconds, rule.MinimumPoints, rule.NextDueAt, rule.Creator.ID, rule.Creator.Name).Row()
		if err := row.Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return fmt.Errorf("insert alert rule: %w", err)
		}
		return nil
	})
}

func (r *GormRepository) GetRule(ctx context.Context, id int64) (Rule, error) {
	var rule Rule
	row := r.db.WithContext(ctx).Raw(`SELECT id, cluster_id, display_name, resource_kind, resource_name, metric_name, operator,
		threshold, for_seconds, minimum_points, enabled, deleted, last_evaluation_state, last_evaluation_at, last_error_code,
		next_due_at, claim_expires_at, creator_user_id, creator_name, created_at, updated_at
		FROM alert_rules WHERE id = ?`, id).Row()
	if err := scanRule(row, &rule); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, err
	}
	return rule, nil
}

func (r *GormRepository) ListRules(ctx context.Context, filter RuleListFilter) ([]Rule, error) {
	query := `SELECT id, cluster_id, display_name, resource_kind, resource_name, metric_name, operator,
		threshold, for_seconds, minimum_points, enabled, deleted, last_evaluation_state, last_evaluation_at, last_error_code,
		next_due_at, claim_expires_at, creator_user_id, creator_name, created_at, updated_at
		FROM alert_rules`
	conditions := []string{"cluster_id = ?"}
	args := []any{filter.ClusterID}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "deleted = FALSE")
	}
	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY display_name ASC, id ASC LIMIT ?"
	args = append(args, filter.Limit)
	var rules []Rule
	rows, err := r.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *GormRepository) PatchRule(ctx context.Context, id int64, input PatchRuleInput, actor ActorRef) (Rule, error) {
	return r.patchRuleInTx(ctx, id, func(tx *gorm.DB, rule *Rule) error {
		if rule.Deleted {
			return ErrRuleDeleted
		}
		if input.DisplayName != nil {
			rule.DisplayName = *input.DisplayName
		}
		if input.Enabled != nil {
			rule.Enabled = *input.Enabled
		}
		return nil
	})
}

func (r *GormRepository) DeleteRule(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var deleted bool
		var unresolvedCount int64
		if err := tx.Raw(`SELECT deleted FROM alert_rules WHERE id = ?`, id).Row().Scan(&deleted); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRuleNotFound
			}
			return err
		}
		if deleted {
			return ErrRuleDeleted
		}
		if err := tx.Raw(`SELECT COUNT(*) FROM alert_instances WHERE rule_id = ? AND state = 'firing'`, id).Row().Scan(&unresolvedCount); err != nil {
			return err
		}
		if unresolvedCount > 0 {
			return ErrRuleUnresolvedAlert
		}
		result := tx.Exec(`UPDATE alert_rules SET deleted = TRUE, enabled = FALSE, updated_at = NOW() WHERE id = ?`, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrRuleNotFound
		}
		return nil
	})
}

func (r *GormRepository) GetUnresolvedInstance(ctx context.Context, ruleID int64) (*Instance, error) {
	var inst Instance
	row := r.db.WithContext(ctx).Raw(`SELECT id, rule_id, diagnosis_id, state, first_fired_at, last_fired_at,
		resolved_at, latest_evidence_anchor::text, created_at, updated_at
		FROM alert_instances WHERE rule_id = ? AND state = 'firing'`, ruleID).Row()
	if err := scanInstance(row, &inst); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &inst, nil
}

func (r *GormRepository) CreateInstance(ctx context.Context, instance *Instance) error {
	row := r.db.WithContext(ctx).Raw(`INSERT INTO alert_instances
		(rule_id, diagnosis_id, state, first_fired_at, last_fired_at, latest_evidence_anchor)
		VALUES (?, ?, 'firing', ?, ?, CAST(? AS JSONB))
		RETURNING id, created_at, updated_at`,
		instance.RuleID, instance.DiagnosisID, instance.FirstFiredAt, instance.LastFiredAt, instance.LatestEvidenceAnchor).Row()
	if err := row.Scan(&instance.ID, &instance.CreatedAt, &instance.UpdatedAt); err != nil {
		return fmt.Errorf("insert alert instance: %w", err)
	}
	instance.State = StateFiring
	return nil
}

func (r *GormRepository) CreateFiring(ctx context.Context, record *diagnosis.Record, instance *Instance) error {
	if record.SLADueAt.IsZero() {
		record.SLADueAt = diagnosis.SLADeadline(record.Severity, record.ObservedAt)
	}
	rootCauses, _ := json.Marshal(record.RootCauses)
	recommendations, _ := json.Marshal(record.Recommendations)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`INSERT INTO diagnosis_records
			(cluster_id, rule_id, severity, resource_kind, resource_namespace, resource_name, resource_uid, status, summary, root_causes, recommendations, observed_at, sla_due_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSONB), CAST(? AS JSONB), ?, ?)
			RETURNING id, created_at, updated_at`, record.ClusterID, record.RuleID, record.Severity, record.Resource.Kind, record.Resource.Namespace, record.Resource.Name, record.Resource.UID, record.Status, record.Summary, string(rootCauses), string(recommendations), record.ObservedAt, record.SLADueAt).Row()
		if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return fmt.Errorf("insert alert diagnosis: %w", err)
		}
		for _, evidence := range record.Evidence {
			content, _ := json.Marshal(evidence.Content)
			if err := tx.Exec(`INSERT INTO diagnosis_evidence (diagnosis_id, evidence_type, source, content) VALUES (?, ?, ?, CAST(? AS JSONB))`, record.ID, evidence.Type, evidence.Source, string(content)).Error; err != nil {
				return fmt.Errorf("insert alert diagnosis evidence: %w", err)
			}
		}
		instance.DiagnosisID = record.ID
		row = tx.Raw(`INSERT INTO alert_instances
			(rule_id, diagnosis_id, state, first_fired_at, last_fired_at, latest_evidence_anchor)
			VALUES (?, ?, 'firing', ?, ?, CAST(? AS JSONB))
			RETURNING id, created_at, updated_at`, instance.RuleID, instance.DiagnosisID, instance.FirstFiredAt, instance.LastFiredAt, instance.LatestEvidenceAnchor).Row()
		if err := row.Scan(&instance.ID, &instance.CreatedAt, &instance.UpdatedAt); err != nil {
			return fmt.Errorf("insert alert instance: %w", err)
		}
		instance.State = StateFiring
		return nil
	})
}

func (r *GormRepository) TouchInstance(ctx context.Context, ruleID int64, lastFiredAt time.Time, evidenceAnchor string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_instances SET last_fired_at = ?, latest_evidence_anchor = CAST(? AS JSONB), updated_at = NOW()
		WHERE rule_id = ? AND state = 'firing'`, lastFiredAt, evidenceAnchor, ruleID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAlertNotFound
	}
	return nil
}

func (r *GormRepository) ResolveInstance(ctx context.Context, ruleID int64, resolvedAt time.Time) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_instances SET state = 'resolved', resolved_at = ?, updated_at = NOW()
		WHERE rule_id = ? AND state = 'firing'`, resolvedAt, ruleID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAlertNotFound
	}
	return nil
}

func (r *GormRepository) ListInstances(ctx context.Context, filter InstanceListFilter) ([]Instance, error) {
	conditions := []string{"r.cluster_id = ?"}
	args := []any{filter.ClusterID}
	if filter.State != "" {
		conditions = append(conditions, "i.state = ?")
		args = append(args, filter.State)
	}
	if filter.RuleID > 0 {
		conditions = append(conditions, "i.rule_id = ?")
		args = append(args, filter.RuleID)
	}
	query := `SELECT i.id, i.rule_id, i.diagnosis_id, i.state, i.first_fired_at, i.last_fired_at,
		i.resolved_at, i.latest_evidence_anchor::text, i.created_at, i.updated_at
		FROM alert_instances i JOIN alert_rules r ON r.id = i.rule_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY i.created_at DESC, i.id DESC LIMIT ?`
	args = append(args, filter.Limit)
	var instances []Instance
	rows, err := r.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var inst Instance
		if err := scanInstance(rows, &inst); err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

func (r *GormRepository) GetInstance(ctx context.Context, id int64) (Instance, error) {
	var inst Instance
	row := r.db.WithContext(ctx).Raw(`SELECT i.id, i.rule_id, i.diagnosis_id, i.state, i.first_fired_at, i.last_fired_at,
		i.resolved_at, i.latest_evidence_anchor::text, i.created_at, i.updated_at
		FROM alert_instances i WHERE i.id = ?`, id).Row()
	if err := scanInstance(row, &inst); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Instance{}, ErrAlertNotFound
		}
		return Instance{}, err
	}
	return inst, nil
}

func (r *GormRepository) ClaimDueRules(ctx context.Context, now time.Time, batchSize int, claimLease time.Duration) ([]Rule, error) {
	claimExpiry := now.Add(claimLease)
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_rules SET claim_expires_at = ?, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM alert_rules
			WHERE deleted = FALSE AND enabled = TRUE
				AND next_due_at <= ? AND (claim_expires_at IS NULL OR claim_expires_at < ?)
			ORDER BY next_due_at ASC, id ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		) RETURNING id`, claimExpiry, now, now, batchSize)
	if result.Error != nil {
		return nil, result.Error
	}
	_ = result.RowsAffected
	// Re-query the claimed rules
	var rules []Rule
	rows, err := r.db.WithContext(ctx).Raw(`SELECT id, cluster_id, display_name, resource_kind, resource_name, metric_name, operator,
		threshold, for_seconds, minimum_points, enabled, deleted, last_evaluation_state, last_evaluation_at, last_error_code,
		next_due_at, claim_expires_at, creator_user_id, creator_name, created_at, updated_at
		FROM alert_rules
		WHERE deleted = FALSE AND enabled = TRUE AND claim_expires_at = ?
		ORDER BY next_due_at ASC, id ASC`, claimExpiry).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *GormRepository) ReleaseClaim(ctx context.Context, ruleID int64, nextDueAt time.Time, evalState string, evalAt time.Time, errCode string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_rules SET claim_expires_at = NULL, next_due_at = ?,
		last_evaluation_state = ?, last_evaluation_at = ?, last_error_code = ?, updated_at = NOW() WHERE id = ?`,
		nextDueAt, evalState, evalAt, errCode, ruleID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func (r *GormRepository) UpdateRuleHealth(ctx context.Context, ruleID int64, evalState string, evalAt time.Time, errCode string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_rules SET last_evaluation_state = ?, last_evaluation_at = ?,
		last_error_code = ?, updated_at = NOW() WHERE id = ?`, evalState, evalAt, errCode, ruleID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func (r *GormRepository) patchRuleInTx(ctx context.Context, id int64, mutate func(*gorm.DB, *Rule) error) (Rule, error) {
	var rule Rule
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`SELECT id, cluster_id, display_name, resource_kind, resource_name, metric_name, operator,
			threshold, for_seconds, minimum_points, enabled, deleted, last_evaluation_state, last_evaluation_at, last_error_code,
			next_due_at, claim_expires_at, creator_user_id, creator_name, created_at, updated_at
			FROM alert_rules WHERE id = ? FOR UPDATE`, id).Row()
		if err := scanRule(row, &rule); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRuleNotFound
			}
			return err
		}
		if err := mutate(tx, &rule); err != nil {
			return err
		}
		if rule.DisplayName != "" {
			var nameConflict int64
			if err := tx.Raw(`SELECT COUNT(*) FROM alert_rules WHERE cluster_id = ? AND deleted = FALSE AND LOWER(display_name) = LOWER(?) AND id <> ?`,
				rule.ClusterID, rule.DisplayName, id).Row().Scan(&nameConflict); err != nil {
				return fmt.Errorf("check name uniqueness: %w", err)
			}
			if nameConflict > 0 {
				return ErrDuplicateName
			}
		}
		return tx.Exec(`UPDATE alert_rules SET display_name = ?, enabled = ?, updated_at = NOW() WHERE id = ?`,
			rule.DisplayName, rule.Enabled, id).Error
	})
	if err != nil {
		return Rule{}, err
	}
	return r.GetRule(ctx, id)
}

func scanRule(scanner interface{ Scan(dest ...any) error }, rule *Rule) error {
	var claimExpiresAt, lastEvalAt sql.NullTime
	var creatorUserID sql.NullInt64
	var creatorName sql.NullString
	err := scanner.Scan(&rule.ID, &rule.ClusterID, &rule.DisplayName, &rule.ResourceKind, &rule.ResourceName,
		&rule.MetricName, &rule.Operator, &rule.Threshold, &rule.ForSeconds, &rule.MinimumPoints,
		&rule.Enabled, &rule.Deleted, &rule.LastEvaluationState, &lastEvalAt, &rule.LastErrorCode,
		&rule.NextDueAt, &claimExpiresAt, &creatorUserID, &creatorName, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return err
	}
	if lastEvalAt.Valid {
		rule.LastEvaluationAt = &lastEvalAt.Time
	}
	if claimExpiresAt.Valid {
		rule.ClaimExpiresAt = &claimExpiresAt.Time
	}
	rule.Creator = ActorRef{ID: creatorUserID.Int64, Name: creatorName.String}
	return nil
}

func scanInstance(scanner interface{ Scan(dest ...any) error }, inst *Instance) error {
	var resolvedAt sql.NullTime
	var evidenceAnchor string
	err := scanner.Scan(&inst.ID, &inst.RuleID, &inst.DiagnosisID, &inst.State, &inst.FirstFiredAt, &inst.LastFiredAt,
		&resolvedAt, &evidenceAnchor, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return err
	}
	if resolvedAt.Valid {
		inst.ResolvedAt = &resolvedAt.Time
	}
	inst.LatestEvidenceAnchor = evidenceAnchor
	_ = json.Unmarshal([]byte(evidenceAnchor), &struct{}{})
	return nil
}
