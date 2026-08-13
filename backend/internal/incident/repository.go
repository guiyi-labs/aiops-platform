package incident

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Repository is the persistence contract for the incident aggregate.
type Repository interface {
	Create(context.Context, *Incident) error
	Get(context.Context, int64) (Incident, error)
	FindBySource(context.Context, string, string) (Incident, error)
	List(context.Context, ListFilter) ([]Incident, error)
	Summary(context.Context) (Summary, error)
	Transition(context.Context, int64, int64, string, ActorRef, string) (Incident, error)
	Assign(context.Context, int64, int64, int64, ActorRef, string) (Incident, error)
	AddFollower(context.Context, int64, int64, ActorRef) (Incident, error)
	RemoveFollower(context.Context, int64, int64, ActorRef) (Incident, error)
	AddNote(context.Context, int64, int64, ActorRef, string) (Incident, error)
	SetPostmortem(context.Context, int64, int64, ActorRef, string) (Incident, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

type storedIncident struct {
	ID                int64
	Number            string
	Title             string
	SourceType        string
	SourceRef         string
	ClusterID         int64
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	ResourceUID       string
	Severity          string
	Status            string
	Summary           string
	Postmortem        string
	AssigneeID        sql.NullInt64
	AssigneeName      sql.NullString
	Version           int64
	ObservedAt        time.Time
	SLADueAt          time.Time
	ResolvedAt        sql.NullTime
	Overdue           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

const incidentSelect = `SELECT i.id, i.number, i.title, i.source_type, i.source_ref, i.cluster_id,
	i.resource_kind, i.resource_namespace, i.resource_name, i.resource_uid, i.severity, i.status,
	i.summary, i.postmortem, i.assigned_to_user_id AS assignee_id, u.display_name AS assignee_name,
	i.version, i.observed_at, i.sla_due_at, i.resolved_at,
	(i.status IN ('open', 'confirmed') AND i.sla_due_at < NOW()) AS overdue,
	i.created_at, i.updated_at
	FROM incidents i LEFT JOIN users u ON u.id = i.assigned_to_user_id`

func (r *GormRepository) Create(ctx context.Context, record *Incident) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`INSERT INTO incidents
			(title, source_type, source_ref, cluster_id, resource_kind, resource_namespace,
			 resource_name, resource_uid, severity, status, summary, observed_at, sla_due_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)
			RETURNING id, number, version, created_at, updated_at`,
			record.Title, record.SourceType, record.SourceRef, record.ClusterID,
			record.Resource.Kind, record.Resource.Namespace, record.Resource.Name, record.Resource.UID,
			record.Severity, record.Summary, record.ObservedAt, record.SLADueAt).Row()
		if err := row.Scan(&record.ID, &record.Number, &record.Version, &record.CreatedAt, &record.UpdatedAt); err != nil {
			if isUniqueViolation(err) {
				return ErrSourceAlreadyUsed
			}
			return fmt.Errorf("insert incident: %w", err)
		}
		if err := insertTimeline(tx, record.ID, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     ActorRef{ID: 0, Name: "system"},
			Content:   "incident created from " + record.SourceType + " source " + record.SourceRef,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		return nil
	})
}

// ListSLAEligible returns open/confirmed incidents whose SLA deadline falls
// within [dueAfter, dueBefore] and that have no notification delivery of the
// given event type yet. Used by the SLA monitor; the caller decides whether a
// deadline is approaching or breached.
func (r *GormRepository) ListSLAEligible(ctx context.Context, eventType string, dueAfter, dueBefore time.Time, limit int) ([]SLACandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var stored []struct {
		IncidentID   int64
		Number       string
		Title        string
		Severity     string
		Status       string
		Summary      string
		AssigneeID   int64
		AssigneeName string
		ObservedAt   time.Time
		SLADueAt     time.Time
	}
	err := r.db.WithContext(ctx).Raw(`SELECT i.id, i.number, i.title, i.severity, i.status, i.summary,
		COALESCE(i.assigned_to_user_id, 0) AS assignee_id, COALESCE(u.display_name, '') AS assignee_name,
		i.observed_at, i.sla_due_at
		FROM incidents i
		LEFT JOIN users u ON u.id = i.assigned_to_user_id
		WHERE i.status IN ('open', 'confirmed')
		  AND i.sla_due_at >= ?
		  AND i.sla_due_at <= ?
		  AND NOT EXISTS (
		      SELECT 1 FROM notification_deliveries nd
		      WHERE nd.incident_id = i.id AND nd.event_type = ?
		  )
		ORDER BY i.sla_due_at, i.id
		LIMIT ?`, dueAfter, dueBefore, eventType, limit).Scan(&stored).Error
	if err != nil {
		return nil, err
	}
	candidates := make([]SLACandidate, 0, len(stored))
	for _, item := range stored {
		candidates = append(candidates, SLACandidate{
			IncidentID: item.IncidentID, Number: item.Number, Title: item.Title,
			Severity: item.Severity, Status: item.Status, Summary: item.Summary,
			AssigneeID: item.AssigneeID, AssigneeName: item.AssigneeName,
			ObservedAt: item.ObservedAt, SLADueAt: item.SLADueAt,
		})
	}
	return candidates, nil
}

func (r *GormRepository) Get(ctx context.Context, id int64) (Incident, error) {
	var stored storedIncident
	if err := r.db.WithContext(ctx).Raw(incidentSelect+" WHERE i.id = ?", id).Scan(&stored).Error; err != nil {
		return Incident{}, err
	}
	if stored.ID == 0 {
		return Incident{}, ErrNotFound
	}
	return r.assemble(ctx, stored)
}

// FindBySource returns the single incident tied to a (source_type,
// source_ref) pair. The incidents_source_unique constraint guarantees at most
// one row; ErrNotFound is returned when no incident exists yet.
func (r *GormRepository) FindBySource(ctx context.Context, sourceType, sourceRef string) (Incident, error) {
	var stored storedIncident
	if err := r.db.WithContext(ctx).Raw(incidentSelect+" WHERE i.source_type = ? AND i.source_ref = ?", sourceType, sourceRef).Scan(&stored).Error; err != nil {
		return Incident{}, err
	}
	if stored.ID == 0 {
		return Incident{}, ErrNotFound
	}
	return r.assemble(ctx, stored)
}

func (r *GormRepository) List(ctx context.Context, filter ListFilter) ([]Incident, error) {
	query := incidentSelect
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if filter.ClusterID > 0 {
		conditions = append(conditions, "i.cluster_id = ?")
		args = append(args, filter.ClusterID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, filter.Status)
	}
	if filter.AssigneeID > 0 {
		conditions = append(conditions, "i.assigned_to_user_id = ?")
		args = append(args, filter.AssigneeID)
	}
	if filter.FollowerID > 0 {
		query += " JOIN incident_followers f ON f.incident_id = i.id"
		conditions = append(conditions, "f.user_id = ?")
		args = append(args, filter.FollowerID)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query += " ORDER BY i.created_at DESC LIMIT ?"
	args = append(args, limit)

	var stored []storedIncident
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&stored).Error; err != nil {
		return nil, err
	}
	result := make([]Incident, 0, len(stored))
	for _, s := range stored {
		incident, err := r.assemble(ctx, s)
		if err != nil {
			return nil, err
		}
		result = append(result, incident)
	}
	return result, nil
}

func (r *GormRepository) Summary(ctx context.Context) (Summary, error) {
	var summary Summary
	query := `SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE status = 'open') AS open,
		COUNT(*) FILTER (WHERE status = 'confirmed') AS confirmed,
		COUNT(*) FILTER (WHERE status = 'resolved') AS resolved,
		COUNT(*) FILTER (WHERE status = 'dismissed') AS dismissed,
		COUNT(*) FILTER (WHERE status IN ('open', 'confirmed') AND sla_due_at < NOW()) AS overdue
		FROM incidents`
	if err := r.db.WithContext(ctx).Raw(query).Scan(&summary).Error; err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (r *GormRepository) Transition(ctx context.Context, id, expectedVersion int64, toStatus string, actor ActorRef, comment string) (Incident, error) {
	var stored storedIncident
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(incidentSelect+" WHERE i.id = ?", id).Scan(&stored).Error; err != nil {
			return err
		}
		if stored.ID == 0 {
			return ErrNotFound
		}
		if stored.Version != expectedVersion {
			return ErrVersionConflict
		}
		if !CanTransition(stored.Status, toStatus) {
			return ErrInvalidTransition
		}
		resolvedClause := ""
		args := []any{toStatus, time.Now().UTC(), id, stored.Version}
		if toStatus == StatusResolved {
			resolvedClause = ", resolved_at = ?"
			args = []any{toStatus, time.Now().UTC(), time.Now().UTC(), id, stored.Version}
		}
		if toStatus == StatusOpen {
			resolvedClause = ", resolved_at = NULL"
			args = []any{toStatus, time.Now().UTC(), id, stored.Version}
		}
		result := tx.Exec(`UPDATE incidents SET status = ?, version = version + 1, updated_at = ?`+resolvedClause+`
			WHERE id = ? AND version = ?`, args...)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		content := "status changed from " + stored.Status + " to " + toStatus
		if strings.TrimSpace(comment) != "" {
			content += ": " + strings.TrimSpace(comment)
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     actor,
			Content:   content,
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) Assign(ctx context.Context, id, expectedVersion, assigneeUserID int64, actor ActorRef, comment string) (Incident, error) {
	var stored storedIncident
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(incidentSelect+" WHERE i.id = ?", id).Scan(&stored).Error; err != nil {
			return err
		}
		if stored.ID == 0 {
			return ErrNotFound
		}
		if stored.Version != expectedVersion {
			return ErrVersionConflict
		}
		var assigneeName string
		if err := tx.Raw(`SELECT display_name FROM users WHERE id = ?`, assigneeUserID).Scan(&assigneeName).Error; err != nil {
			return err
		}
		if assigneeName == "" {
			return ErrAssigneeNotFound
		}
		result := tx.Exec(`UPDATE incidents SET assigned_to_user_id = ?, version = version + 1, updated_at = ?
			WHERE id = ? AND version = ?`, assigneeUserID, time.Now().UTC(), id, stored.Version)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		content := "handoff from " + strings.TrimSpace(stored.AssigneeName.String)
		if !stored.AssigneeName.Valid || strings.TrimSpace(stored.AssigneeName.String) == "" {
			content = "handoff from unassigned"
		}
		content += " to " + assigneeName
		if strings.TrimSpace(comment) != "" {
			content += ": " + strings.TrimSpace(comment)
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     actor,
			Content:   content,
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) AddFollower(ctx context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists int64
		if err := tx.Raw(`SELECT COUNT(*) FROM incidents WHERE id = ?`, id).Scan(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		var userName string
		if err := tx.Raw(`SELECT display_name FROM users WHERE id = ?`, userID).Scan(&userName).Error; err != nil {
			return err
		}
		if userName == "" {
			return ErrAssigneeNotFound
		}
		result := tx.Exec(`INSERT INTO incident_followers (incident_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrFollowerDuplicate
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     actor,
			Content:   userName + " is now following this incident",
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) RemoveFollower(ctx context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userName string
		if err := tx.Raw(`SELECT display_name FROM users WHERE id = ?`, userID).Scan(&userName).Error; err != nil {
			return err
		}
		result := tx.Exec(`DELETE FROM incident_followers WHERE incident_id = ? AND user_id = ?`, id, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrFollowerNotFound
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     actor,
			Content:   userName + " stopped following this incident",
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) AddNote(ctx context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	if strings.TrimSpace(content) == "" {
		return Incident{}, ErrInvalidNote
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE incidents SET version = version + 1, updated_at = ? WHERE id = ? AND version = ?`,
			time.Now().UTC(), id, expectedVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var exists int64
			if err := tx.Raw(`SELECT COUNT(*) FROM incidents WHERE id = ?`, id).Scan(&exists).Error; err != nil {
				return err
			}
			if exists == 0 {
				return ErrNotFound
			}
			return ErrVersionConflict
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeNote,
			Actor:     actor,
			Content:   strings.TrimSpace(content),
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) SetPostmortem(ctx context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	var stored storedIncident
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(incidentSelect+" WHERE i.id = ?", id).Scan(&stored).Error; err != nil {
			return err
		}
		if stored.ID == 0 {
			return ErrNotFound
		}
		if stored.Status != StatusResolved {
			return ErrPostmortemLocked
		}
		if stored.Version != expectedVersion {
			return ErrVersionConflict
		}
		result := tx.Exec(`UPDATE incidents SET postmortem = ?, version = version + 1, updated_at = ?
			WHERE id = ? AND version = ?`, strings.TrimSpace(content), time.Now().UTC(), id, stored.Version)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		return insertTimeline(tx, id, TimelineEvent{
			EventType: EventTypeSystem,
			Actor:     actor,
			Content:   "postmortem updated",
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return Incident{}, err
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) assemble(ctx context.Context, stored storedIncident) (Incident, error) {
	incident := Incident{
		ID:         stored.ID,
		Number:     stored.Number,
		Title:      stored.Title,
		SourceType: stored.SourceType,
		SourceRef:  stored.SourceRef,
		ClusterID:  stored.ClusterID,
		Resource: ResourceRef{
			Kind:      stored.ResourceKind,
			Namespace: stored.ResourceNamespace,
			Name:      stored.ResourceName,
			UID:       stored.ResourceUID,
		},
		Severity:   stored.Severity,
		Status:     stored.Status,
		Summary:    stored.Summary,
		Postmortem: stored.Postmortem,
		Version:    stored.Version,
		ObservedAt: stored.ObservedAt,
		SLADueAt:   stored.SLADueAt,
		Overdue:    stored.Overdue,
		CreatedAt:  stored.CreatedAt,
		UpdatedAt:  stored.UpdatedAt,
	}
	if stored.ResolvedAt.Valid {
		t := stored.ResolvedAt.Time
		incident.ResolvedAt = &t
	}
	if stored.AssigneeID.Valid {
		incident.Assignee = &ActorRef{ID: stored.AssigneeID.Int64, Name: stored.AssigneeName.String}
	}
	var followers []Follower
	if err := r.db.WithContext(ctx).Raw(`SELECT f.user_id, u.display_name AS name, f.created_at AS added_at
		FROM incident_followers f JOIN users u ON u.id = f.user_id
		WHERE f.incident_id = ? ORDER BY f.created_at`, stored.ID).Scan(&followers).Error; err != nil {
		return Incident{}, err
	}
	incident.Followers = followers
	var timelineRows []timelineRow
	if err := r.db.WithContext(ctx).Raw(`SELECT t.id, t.event_type, t.actor_user_id, t.actor_name, t.content, t.created_at
		FROM incident_timeline_events t WHERE t.incident_id = ? ORDER BY t.created_at, t.id`, stored.ID).Scan(&timelineRows).Error; err != nil {
		return Incident{}, err
	}
	timeline := make([]TimelineEvent, 0, len(timelineRows))
	for _, row := range timelineRows {
		timeline = append(timeline, TimelineEvent{
			ID:        row.ID,
			EventType: row.EventType,
			Actor:     ActorRef{ID: row.ActorID, Name: row.ActorName},
			Content:   row.Content,
			CreatedAt: row.CreatedAt,
		})
	}
	incident.Timeline = timeline
	return incident, nil
}

func insertTimeline(tx *gorm.DB, incidentID int64, event TimelineEvent) error {
	var actorID any
	if event.Actor.ID > 0 {
		actorID = event.Actor.ID
	}
	return tx.Exec(`INSERT INTO incident_timeline_events
		(incident_id, event_type, actor_user_id, actor_name, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		incidentID, event.EventType, actorID, event.Actor.Name, event.Content, event.CreatedAt).Error
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505"))
}

type timelineRow struct {
	ID        int64
	EventType string
	ActorID   int64
	ActorName string
	Content   string
	CreatedAt time.Time
}
