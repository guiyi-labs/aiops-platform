package diagnosis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	Save(context.Context, *Record) error
	List(context.Context, ListFilter) ([]Record, error)
	Get(context.Context, int64) (Record, error)
	Transition(context.Context, int64, string, ActorRef, string) (Record, error)
	AddFeedback(context.Context, int64, string, ActorRef, string) (Record, error)
	Assign(context.Context, int64, ActorRef, ActorRef, string) (Record, error)
	Summary(context.Context) (Summary, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, record *Record) error {
	if record.SLADueAt.IsZero() {
		record.SLADueAt = SLADeadline(record.Severity, record.ObservedAt)
	}
	rootCauses, _ := json.Marshal(record.RootCauses)
	recommendations, _ := json.Marshal(record.Recommendations)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`INSERT INTO diagnosis_records
			(cluster_id, rule_id, severity, resource_kind, resource_namespace, resource_name, resource_uid, status, summary, root_causes, recommendations, observed_at, sla_due_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSONB), CAST(? AS JSONB), ?, ?)
			RETURNING id, created_at, updated_at`, record.ClusterID, record.RuleID, record.Severity, record.Resource.Kind, record.Resource.Namespace, record.Resource.Name, record.Resource.UID, record.Status, record.Summary, string(rootCauses), string(recommendations), record.ObservedAt, record.SLADueAt).Row()
		if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return fmt.Errorf("insert diagnosis record: %w", err)
		}
		for _, evidence := range record.Evidence {
			content, _ := json.Marshal(evidence.Content)
			if err := tx.Exec(`INSERT INTO diagnosis_evidence (diagnosis_id, evidence_type, source, content) VALUES (?, ?, ?, CAST(? AS JSONB))`, record.ID, evidence.Type, evidence.Source, string(content)).Error; err != nil {
				return fmt.Errorf("insert diagnosis evidence: %w", err)
			}
		}
		return nil
	})
}

type storedRecord struct {
	ID                int64
	ClusterID         int64
	RuleID            string
	Severity          string
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	ResourceUID       string
	Status            string
	Summary           string
	RootCauses        string
	Recommendations   string
	AssigneeID        sql.NullInt64
	AssigneeName      sql.NullString
	ObservedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SLADueAt          time.Time
	ResolvedAt        sql.NullTime
	Overdue           bool
}

func (r *GormRepository) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	return r.list(ctx, filter, "d.created_at")
}

func (r *GormRepository) list(ctx context.Context, filter ListFilter, orderBy string) ([]Record, error) {
	query := `SELECT d.id, d.cluster_id, d.rule_id, d.severity, d.resource_kind, d.resource_namespace, d.resource_name, d.resource_uid, d.status, d.summary,
		d.root_causes::text AS root_causes, d.recommendations::text AS recommendations, d.assigned_to_user_id AS assignee_id,
		u.display_name AS assignee_name, d.observed_at, d.created_at, d.updated_at, d.sla_due_at, d.resolved_at,
		(d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW()) AS overdue
		FROM diagnosis_records d LEFT JOIN users u ON u.id = d.assigned_to_user_id`
	conditions := make([]string, 0, 3)
	args := []any{}
	if filter.ClusterID > 0 {
		conditions = append(conditions, "d.cluster_id = ?")
		args = append(args, filter.ClusterID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "d.status = ?")
		args = append(args, filter.Status)
	}
	if filter.Overdue != nil {
		if *filter.Overdue {
			conditions = append(conditions, "d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW()")
		} else {
			conditions = append(conditions, "NOT (d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW())")
		}
	}
	if filter.Since != nil {
		conditions = append(conditions, "d.observed_at >= ?")
		args = append(args, *filter.Since)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY " + orderBy + " DESC, d.id DESC LIMIT ?"
	args = append(args, filter.Limit)
	var stored []storedRecord
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&stored).Error; err != nil {
		return nil, err
	}
	items := make([]Record, 0, len(stored))
	for _, item := range stored {
		items = append(items, decodeStoredRecord(item))
	}
	return items, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (Record, error) {
	var item storedRecord
	row := r.db.WithContext(ctx).Raw(`SELECT d.id, d.cluster_id, d.rule_id, d.severity, d.resource_kind, d.resource_namespace, d.resource_name, d.resource_uid, d.status, d.summary,
		d.root_causes::text AS root_causes, d.recommendations::text AS recommendations, d.assigned_to_user_id AS assignee_id,
		u.display_name AS assignee_name, d.observed_at, d.created_at, d.updated_at, d.sla_due_at, d.resolved_at,
		(d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW()) AS overdue
		FROM diagnosis_records d LEFT JOIN users u ON u.id = d.assigned_to_user_id WHERE d.id = ?`, id).Row()
	if err := row.Scan(&item.ID, &item.ClusterID, &item.RuleID, &item.Severity, &item.ResourceKind, &item.ResourceNamespace, &item.ResourceName, &item.ResourceUID, &item.Status, &item.Summary, &item.RootCauses, &item.Recommendations, &item.AssigneeID, &item.AssigneeName, &item.ObservedAt, &item.CreatedAt, &item.UpdatedAt, &item.SLADueAt, &item.ResolvedAt, &item.Overdue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, ErrRecordNotFound
		}
		return Record{}, err
	}
	record := decodeStoredRecord(item)
	type storedEvidence struct {
		Type    string
		Source  string
		Content string
	}
	var stored []storedEvidence
	if err := r.db.WithContext(ctx).Raw(`SELECT evidence_type AS type, source, content::text AS content
        FROM diagnosis_evidence WHERE diagnosis_id = ? ORDER BY id`, id).Scan(&stored).Error; err != nil {
		return Record{}, err
	}
	record.Evidence = make([]Evidence, 0, len(stored))
	for _, item := range stored {
		evidence := Evidence{Type: item.Type, Source: item.Source}
		if err := json.Unmarshal([]byte(item.Content), &evidence.Content); err != nil {
			return Record{}, fmt.Errorf("decode diagnosis evidence: %w", err)
		}
		record.Evidence = append(record.Evidence, evidence)
	}
	if err := r.loadWorkflow(ctx, id, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *GormRepository) loadWorkflow(ctx context.Context, id int64, record *Record) error {
	type storedActivity struct {
		ID          int64
		ActorUserID sql.NullInt64
		ActorName   string
		FromStatus  string
		ToStatus    string
		Comment     string
		CreatedAt   time.Time
	}
	var activities []storedActivity
	if err := r.db.WithContext(ctx).Raw(`SELECT id, actor_user_id, actor_name, from_status, to_status, comment, created_at
		FROM diagnosis_activities WHERE diagnosis_id = ? ORDER BY created_at DESC, id DESC`, id).Scan(&activities).Error; err != nil {
		return err
	}
	record.Activities = make([]Activity, 0, len(activities))
	for _, item := range activities {
		record.Activities = append(record.Activities, Activity{ID: item.ID, Actor: ActorRef{ID: item.ActorUserID.Int64, Name: item.ActorName}, FromStatus: item.FromStatus, ToStatus: item.ToStatus, Comment: item.Comment, CreatedAt: item.CreatedAt})
	}
	type storedFeedback struct {
		ID          int64
		ActorUserID sql.NullInt64
		ActorName   string
		Verdict     string
		Comment     string
		CreatedAt   time.Time
	}
	var feedback []storedFeedback
	if err := r.db.WithContext(ctx).Raw(`SELECT id, actor_user_id, actor_name, verdict, comment, created_at
		FROM diagnosis_feedback WHERE diagnosis_id = ? ORDER BY created_at DESC, id DESC`, id).Scan(&feedback).Error; err != nil {
		return err
	}
	record.Feedback = make([]Feedback, 0, len(feedback))
	for _, item := range feedback {
		record.Feedback = append(record.Feedback, Feedback{ID: item.ID, Actor: ActorRef{ID: item.ActorUserID.Int64, Name: item.ActorName}, Verdict: item.Verdict, Comment: item.Comment, CreatedAt: item.CreatedAt})
	}
	type storedAssignment struct {
		ID                 int64
		ActorUserID        sql.NullInt64
		ActorName          string
		FromAssigneeUserID sql.NullInt64
		FromAssigneeName   string
		ToAssigneeUserID   sql.NullInt64
		ToAssigneeName     string
		Comment            string
		CreatedAt          time.Time
	}
	var assignments []storedAssignment
	if err := r.db.WithContext(ctx).Raw(`SELECT id, actor_user_id, actor_name, from_assignee_user_id, from_assignee_name,
		to_assignee_user_id, to_assignee_name, comment, created_at FROM diagnosis_assignments
		WHERE diagnosis_id = ? ORDER BY created_at DESC, id DESC`, id).Scan(&assignments).Error; err != nil {
		return err
	}
	record.Assignments = make([]Assignment, 0, len(assignments))
	for _, item := range assignments {
		assignment := Assignment{ID: item.ID, Actor: ActorRef{ID: item.ActorUserID.Int64, Name: item.ActorName}, ToAssignee: ActorRef{ID: item.ToAssigneeUserID.Int64, Name: item.ToAssigneeName}, Comment: item.Comment, CreatedAt: item.CreatedAt}
		if item.FromAssigneeName != "" || item.FromAssigneeUserID.Valid {
			assignment.FromAssignee = &ActorRef{ID: item.FromAssigneeUserID.Int64, Name: item.FromAssigneeName}
		}
		record.Assignments = append(record.Assignments, assignment)
	}
	return nil
}

func (r *GormRepository) Transition(ctx context.Context, id int64, to string, actor ActorRef, comment string) (Record, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var from string
		var currentAssigneeID sql.NullInt64
		if err := tx.Raw(`SELECT status, assigned_to_user_id FROM diagnosis_records WHERE id = ? FOR UPDATE`, id).Row().Scan(&from, &currentAssigneeID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRecordNotFound
			}
			return err
		}
		if !CanTransition(from, to) {
			return ErrInvalidTransition
		}
		if err := tx.Exec(`UPDATE diagnosis_records SET status = ?, assigned_to_user_id = COALESCE(assigned_to_user_id, ?),
			resolved_at = CASE WHEN ? = 'resolved' THEN NOW() WHEN ? = 'open' THEN NULL ELSE resolved_at END,
			updated_at = NOW() WHERE id = ?`, to, actor.ID, to, to, id).Error; err != nil {
			return err
		}
		if !currentAssigneeID.Valid {
			if err := tx.Exec(`INSERT INTO diagnosis_assignments
				(diagnosis_id, actor_user_id, actor_name, from_assignee_user_id, from_assignee_name, to_assignee_user_id, to_assignee_name, comment)
				VALUES (?, ?, ?, NULL, '', ?, ?, '')`, id, actor.ID, actor.Name, actor.ID, actor.Name).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO diagnosis_activities (diagnosis_id, actor_user_id, actor_name, from_status, to_status, comment) VALUES (?, ?, ?, ?, ?, ?)`, id, actor.ID, actor.Name, from, to, comment).Error
	})
	if err != nil {
		return Record{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) Assign(ctx context.Context, id int64, assignee, actor ActorRef, comment string) (Record, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var currentID sql.NullInt64
		var currentName string
		if err := tx.Raw(`SELECT d.assigned_to_user_id, COALESCE(u.display_name, '') FROM diagnosis_records d
			LEFT JOIN users u ON u.id = d.assigned_to_user_id WHERE d.id = ? FOR UPDATE OF d`, id).Row().Scan(&currentID, &currentName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRecordNotFound
			}
			return err
		}
		if currentID.Valid && currentID.Int64 == assignee.ID {
			return ErrAlreadyAssigned
		}
		if err := tx.Exec(`UPDATE diagnosis_records SET assigned_to_user_id = ?, updated_at = NOW() WHERE id = ?`, assignee.ID, id).Error; err != nil {
			return err
		}
		var fromID any
		if currentID.Valid {
			fromID = currentID.Int64
		}
		return tx.Exec(`INSERT INTO diagnosis_assignments
			(diagnosis_id, actor_user_id, actor_name, from_assignee_user_id, from_assignee_name, to_assignee_user_id, to_assignee_name, comment)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, actor.ID, actor.Name, fromID, currentName, assignee.ID, assignee.Name, comment).Error
	})
	if err != nil {
		return Record{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) AddFeedback(ctx context.Context, id int64, verdict string, actor ActorRef, comment string) (Record, error) {
	result := r.db.WithContext(ctx).Exec(`INSERT INTO diagnosis_feedback (diagnosis_id, actor_user_id, actor_name, verdict, comment)
		SELECT id, ?, ?, ?, ? FROM diagnosis_records WHERE id = ?`, actor.ID, actor.Name, verdict, comment, id)
	if result.Error != nil {
		return Record{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Record{}, ErrRecordNotFound
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) Summary(ctx context.Context) (Summary, error) {
	type storedSummary struct {
		Total     int64
		Open      int64
		Confirmed int64
		Resolved  int64
		Dismissed int64
		Overdue   int64
	}
	var stored storedSummary
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS total,
		COUNT(*) FILTER (WHERE status = 'open') AS open,
		COUNT(*) FILTER (WHERE status = 'confirmed') AS confirmed,
		COUNT(*) FILTER (WHERE status = 'resolved') AS resolved,
		COUNT(*) FILTER (WHERE status = 'dismissed') AS dismissed,
		COUNT(*) FILTER (WHERE status IN ('open', 'confirmed') AND sla_due_at < NOW()) AS overdue FROM diagnosis_records`).Scan(&stored).Error; err != nil {
		return Summary{}, err
	}
	summary := Summary{Total: stored.Total, Open: stored.Open, Confirmed: stored.Confirmed, Resolved: stored.Resolved, Dismissed: stored.Dismissed, Overdue: stored.Overdue}
	recent, err := r.list(ctx, ListFilter{Limit: 5}, "d.updated_at")
	if err != nil {
		return Summary{}, err
	}
	summary.Recent = recent
	return summary, nil
}

func decodeStoredRecord(item storedRecord) Record {
	record := Record{ID: item.ID, ClusterID: item.ClusterID, RuleID: item.RuleID, Severity: item.Severity, Resource: ResourceRef{Kind: item.ResourceKind, Namespace: item.ResourceNamespace, Name: item.ResourceName, UID: item.ResourceUID}, Status: item.Status, Summary: item.Summary, ObservedAt: item.ObservedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, SLADueAt: item.SLADueAt, Overdue: item.Overdue}
	if item.ResolvedAt.Valid {
		value := item.ResolvedAt.Time
		record.ResolvedAt = &value
	}
	if item.AssigneeID.Valid {
		record.Assignee = &ActorRef{ID: item.AssigneeID.Int64, Name: item.AssigneeName.String}
	}
	_ = json.Unmarshal([]byte(item.RootCauses), &record.RootCauses)
	_ = json.Unmarshal([]byte(item.Recommendations), &record.Recommendations)
	return record
}
