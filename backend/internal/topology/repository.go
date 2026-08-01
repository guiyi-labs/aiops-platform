package topology

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists topology edges and change events.
type Repository interface {
	// UpsertEdge inserts a new edge or, when an active edge with the same
	// identity (cluster, kind, source_uid, target_uid, derivation) exists,
	// refreshes its last_observed_at. Returns the stored edge.
	UpsertEdge(ctx context.Context, edge *Edge) error
	// CloseEdge sets valid_to on the active edge matching the identity,
	// ending its validity. Returns ErrEdgeNotFound when no active edge exists.
	CloseEdge(ctx context.Context, clusterID int64, kind EdgeKind, sourceUID, targetUID string, derivation DerivationMethod, validTo time.Time) error
	// ListEdges returns edges matching the filter. When ValidAt is set, only
	// edges valid at that time are returned (valid_from <= t AND (valid_to IS
	// NULL OR valid_to > t)).
	ListEdges(ctx context.Context, filter EdgeFilter) ([]Edge, int64, error)
	// UpsertChangeEvent inserts or updates a change event by (kind, plan_id).
	UpsertChangeEvent(ctx context.Context, event *ChangeEvent) error
	// ListChangeEvents returns change events matching the filter.
	ListChangeEvents(ctx context.Context, filter ChangeTimelineFilter) ([]ChangeEvent, int64, error)
}

// ErrEdgeNotFound is returned when CloseEdge finds no active edge.
var ErrEdgeNotFound = errors.New("topology edge not found")

// --- GORM models ---

type edgeRow struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ClusterID        int64      `gorm:"column:cluster_id;not null"`
	Kind             string     `gorm:"column:kind;not null"`
	SourceKind       string     `gorm:"column:source_kind;not null"`
	SourceNamespace  string     `gorm:"column:source_namespace;not null;default:''"`
	SourceName       string     `gorm:"column:source_name;not null"`
	SourceUID        string     `gorm:"column:source_uid;not null;default:''"`
	SourceIncomplete bool       `gorm:"column:source_incomplete;not null;default:false"`
	TargetKind       string     `gorm:"column:target_kind;not null"`
	TargetNamespace  string     `gorm:"column:target_namespace;not null;default:''"`
	TargetName       string     `gorm:"column:target_name;not null"`
	TargetUID        string     `gorm:"column:target_uid;not null;default:''"`
	TargetIncomplete bool       `gorm:"column:target_incomplete;not null;default:false"`
	Derivation       string     `gorm:"column:derivation;not null"`
	FirstObservedAt  time.Time  `gorm:"column:first_observed_at;not null"`
	LastObservedAt   time.Time  `gorm:"column:last_observed_at;not null"`
	ValidFrom        time.Time  `gorm:"column:valid_from;not null"`
	ValidTo          *time.Time `gorm:"column:valid_to"`
	ReviewEvidence   []byte     `gorm:"column:review_evidence;type:jsonb;not null;default:'[]'::jsonb"`
	SourceHash       string     `gorm:"column:source_hash;not null;default:''"`
}

func (edgeRow) TableName() string { return "topology_edges" }

type changeEventRow struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ClusterID        int64      `gorm:"column:cluster_id;not null"`
	Namespace        string     `gorm:"column:namespace;not null;default:''"`
	Kind             string     `gorm:"column:kind;not null"`
	PlanID           string     `gorm:"column:plan_id;not null;default:''"`
	TargetKind       string     `gorm:"column:target_kind;not null"`
	TargetNamespace  string     `gorm:"column:target_namespace;not null;default:''"`
	TargetName       string     `gorm:"column:target_name;not null"`
	TargetUID        string     `gorm:"column:target_uid;not null;default:''"`
	TargetIncomplete bool       `gorm:"column:target_incomplete;not null;default:false"`
	Action           string     `gorm:"column:action;not null;default:''"`
	SafeDiffHash     string     `gorm:"column:safe_diff_hash;not null;default:''"`
	Revision         string     `gorm:"column:revision;not null;default:''"`
	Actor            string     `gorm:"column:actor;not null;default:''"`
	StartedAt        time.Time  `gorm:"column:started_at;not null"`
	FinishedAt       *time.Time `gorm:"column:finished_at"`
	Result           string     `gorm:"column:result;not null;default:pending"`
	AuditID          *int64     `gorm:"column:audit_id"`
	RequestID        string     `gorm:"column:request_id;not null;default:''"`
	Evidence         []byte     `gorm:"column:evidence;type:jsonb;not null;default:'[]'::jsonb"`
	Confidence       string     `gorm:"column:confidence;not null;default:high"`
	Source           string     `gorm:"column:source;not null;default:platform"`
	IngestedAt       time.Time  `gorm:"column:ingested_at;not null;default:NOW()"`
}

func (changeEventRow) TableName() string { return "change_events" }

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// NopRepository is a no-op repository for disabled/testing mode.
type NopRepository struct{}

func (NopRepository) UpsertEdge(context.Context, *Edge) error { return nil }
func (NopRepository) CloseEdge(context.Context, int64, EdgeKind, string, string, DerivationMethod, time.Time) error {
	return nil
}
func (NopRepository) ListEdges(context.Context, EdgeFilter) ([]Edge, int64, error) {
	return nil, 0, nil
}
func (NopRepository) UpsertChangeEvent(context.Context, *ChangeEvent) error { return nil }
func (NopRepository) ListChangeEvents(context.Context, ChangeTimelineFilter) ([]ChangeEvent, int64, error) {
	return nil, 0, nil
}

func (r *GormRepository) UpsertEdge(ctx context.Context, edge *Edge) error {
	row, err := edgeToRow(edge)
	if err != nil {
		return err
	}
	// Upsert on (cluster_id, kind, source_uid, target_uid, derivation) where
	// valid_to IS NULL. On conflict, refresh last_observed_at and review_evidence.
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "cluster_id"}, {Name: "kind"}, {Name: "source_uid"},
			{Name: "target_uid"}, {Name: "derivation"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"last_observed_at": row.LastObservedAt,
			"review_evidence":  row.ReviewEvidence,
			"source_hash":      row.SourceHash,
		}),
	}).Create(&row).Error
}

func (r *GormRepository) CloseEdge(ctx context.Context, clusterID int64, kind EdgeKind, sourceUID, targetUID string, derivation DerivationMethod, validTo time.Time) error {
	result := r.db.WithContext(ctx).Model(&edgeRow{}).
		Where("cluster_id = ? AND kind = ? AND source_uid = ? AND target_uid = ? AND derivation = ? AND valid_to IS NULL",
			clusterID, string(kind), sourceUID, targetUID, string(derivation)).
		Update("valid_to", validTo)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

func (r *GormRepository) ListEdges(ctx context.Context, filter EdgeFilter) ([]Edge, int64, error) {
	q := r.db.WithContext(ctx).Model(&edgeRow{}).Where("cluster_id = ?", filter.ClusterID)
	if filter.Namespace != "" {
		q = q.Where("source_namespace = ?", filter.Namespace)
	}
	if filter.EdgeKind != "" {
		q = q.Where("kind = ?", string(filter.EdgeKind))
	}
	if filter.SourceUID != "" {
		q = q.Where("source_uid = ?", filter.SourceUID)
	}
	if filter.TargetUID != "" {
		q = q.Where("target_uid = ?", filter.TargetUID)
	}
	if filter.ValidAt != nil {
		t := *filter.ValidAt
		q = q.Where("valid_from <= ? AND (valid_to IS NULL OR valid_to > ?)", t, t)
	} else {
		q = q.Where("valid_to IS NULL")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []edgeRow
	if err := q.Order("last_observed_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Edge, 0, len(rows))
	for i := range rows {
		edge, err := rowToEdge(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		items = append(items, edge)
	}
	return items, total, nil
}

func (r *GormRepository) UpsertChangeEvent(ctx context.Context, event *ChangeEvent) error {
	row, err := changeEventToRow(event)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "kind"}, {Name: "plan_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"result":      row.Result,
			"finished_at": row.FinishedAt,
			"evidence":    row.Evidence,
			"audit_id":    row.AuditID,
			"request_id":  row.RequestID,
			"ingested_at": row.IngestedAt,
		}),
	}).Create(&row).Error
}

func (r *GormRepository) ListChangeEvents(ctx context.Context, filter ChangeTimelineFilter) ([]ChangeEvent, int64, error) {
	q := r.db.WithContext(ctx).Model(&changeEventRow{}).Where("cluster_id = ?", filter.ClusterID)
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	if filter.Kind != "" {
		q = q.Where("kind = ?", filter.Kind)
	}
	if filter.StartTime != nil {
		q = q.Where("started_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		q = q.Where("started_at <= ?", *filter.EndTime)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []changeEventRow
	if err := q.Order("started_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ChangeEvent, 0, len(rows))
	for i := range rows {
		ev, err := rowToChangeEvent(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		items = append(items, ev)
	}
	return items, total, nil
}

// --- row conversion helpers ---

func edgeToRow(edge *Edge) (edgeRow, error) {
	ev, err := marshalEvidence(edge.ReviewEvidence)
	if err != nil {
		return edgeRow{}, err
	}
	return edgeRow{
		ClusterID:        edge.ClusterID,
		Kind:             string(edge.Kind),
		SourceKind:       edge.Source.Kind,
		SourceNamespace:  edge.Source.Namespace,
		SourceName:       edge.Source.Name,
		SourceUID:        edge.Source.UID,
		SourceIncomplete: edge.Source.Incomplete,
		TargetKind:       edge.Target.Kind,
		TargetNamespace:  edge.Target.Namespace,
		TargetName:       edge.Target.Name,
		TargetUID:        edge.Target.UID,
		TargetIncomplete: edge.Target.Incomplete,
		Derivation:       string(edge.Derivation),
		FirstObservedAt:  edge.FirstObservedAt,
		LastObservedAt:   edge.LastObservedAt,
		ValidFrom:        edge.ValidFrom,
		ValidTo:          edge.ValidTo,
		ReviewEvidence:   ev,
		SourceHash:       edge.SourceHash,
	}, nil
}

func rowToEdge(row *edgeRow) (Edge, error) {
	ev, err := unmarshalEvidence(row.ReviewEvidence)
	if err != nil {
		return Edge{}, err
	}
	return Edge{
		ID:              row.ID,
		ClusterID:       row.ClusterID,
		Kind:            EdgeKind(row.Kind),
		Source:          ResourceCitation{Kind: row.SourceKind, Namespace: row.SourceNamespace, Name: row.SourceName, UID: row.SourceUID, Incomplete: row.SourceIncomplete},
		Target:          ResourceCitation{Kind: row.TargetKind, Namespace: row.TargetNamespace, Name: row.TargetName, UID: row.TargetUID, Incomplete: row.TargetIncomplete},
		Derivation:      DerivationMethod(row.Derivation),
		FirstObservedAt: row.FirstObservedAt,
		LastObservedAt:  row.LastObservedAt,
		ValidFrom:       row.ValidFrom,
		ValidTo:         row.ValidTo,
		ReviewEvidence:  ev,
		SourceHash:      row.SourceHash,
	}, nil
}

func changeEventToRow(event *ChangeEvent) (changeEventRow, error) {
	ev, err := marshalEvidence(event.Evidence)
	if err != nil {
		return changeEventRow{}, err
	}
	return changeEventRow{
		ClusterID:        event.ClusterID,
		Namespace:        event.Namespace,
		Kind:             event.Kind,
		PlanID:           event.PlanID,
		TargetKind:       event.Target.Kind,
		TargetNamespace:  event.Target.Namespace,
		TargetName:       event.Target.Name,
		TargetUID:        event.Target.UID,
		TargetIncomplete: event.Target.Incomplete,
		Action:           event.Action,
		SafeDiffHash:     event.SafeDiffHash,
		Revision:         event.Revision,
		Actor:            event.Actor,
		StartedAt:        event.StartedAt,
		FinishedAt:       event.FinishedAt,
		Result:           event.Result,
		AuditID:          event.AuditID,
		RequestID:        event.RequestID,
		Evidence:         ev,
		Confidence:       event.Confidence,
		Source:           event.Source,
		IngestedAt:       time.Now().UTC(),
	}, nil
}

func rowToChangeEvent(row *changeEventRow) (ChangeEvent, error) {
	ev, err := unmarshalEvidence(row.Evidence)
	if err != nil {
		return ChangeEvent{}, err
	}
	return ChangeEvent{
		ID:           row.ID,
		ClusterID:    row.ClusterID,
		Namespace:    row.Namespace,
		Kind:         row.Kind,
		PlanID:       row.PlanID,
		Target:       ResourceCitation{Kind: row.TargetKind, Namespace: row.TargetNamespace, Name: row.TargetName, UID: row.TargetUID, Incomplete: row.TargetIncomplete},
		Action:       row.Action,
		SafeDiffHash: row.SafeDiffHash,
		Revision:     row.Revision,
		Actor:        row.Actor,
		StartedAt:    row.StartedAt,
		FinishedAt:   row.FinishedAt,
		Result:       row.Result,
		AuditID:      row.AuditID,
		RequestID:    row.RequestID,
		Evidence:     ev,
		Confidence:   row.Confidence,
		Source:       row.Source,
	}, nil
}

func marshalEvidence(evidence []EvidenceRef) ([]byte, error) {
	if len(evidence) == 0 {
		return []byte("[]"), nil
	}
	b, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func unmarshalEvidence(data []byte) ([]EvidenceRef, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var evidence []EvidenceRef
	if err := json.Unmarshal(data, &evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}
