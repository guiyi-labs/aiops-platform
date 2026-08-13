package signal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// signalRow is the GORM model for the signal_occurrences table. It mirrors
// migration 000028 and carries JSONB columns as []byte.
type signalRow struct {
	ID                 int64      `gorm:"column:id;primaryKey;autoIncrement"`
	SignalID           string     `gorm:"column:signal_id;not null"`
	SignalCode         string     `gorm:"column:signal_code;not null"`
	SchemaVersion      string     `gorm:"column:schema_version;not null;default:1.0"`
	Producer           string     `gorm:"column:producer;not null"`
	ClusterID          int64      `gorm:"column:cluster_id;not null"`
	Namespace          string     `gorm:"column:namespace;not null;default:''"`
	ResourceKind       string     `gorm:"column:resource_kind;not null"`
	ResourceNamespace  string     `gorm:"column:resource_namespace;not null;default:''"`
	ResourceName       string     `gorm:"column:resource_name;not null"`
	ResourceUID        string     `gorm:"column:resource_uid;not null;default:''"`
	ResourceIncomplete bool       `gorm:"column:resource_incomplete;not null;default:false"`
	Severity           string     `gorm:"column:severity;not null"`
	State              string     `gorm:"column:state;not null"`
	Fingerprint        string     `gorm:"column:fingerprint;not null"`
	Coverage           string     `gorm:"column:coverage;not null"`
	Freshness          time.Time  `gorm:"column:freshness;not null"`
	WindowStart        *time.Time `gorm:"column:window_start"`
	WindowEnd          *time.Time `gorm:"column:window_end"`
	ObservedAt         time.Time  `gorm:"column:observed_at;not null"`
	IngestedAt         time.Time  `gorm:"column:ingested_at;not null;default:NOW()"`
	ExpiresAt          *time.Time `gorm:"column:expires_at"`
	Attributes         []byte     `gorm:"column:attributes;type:jsonb;not null;default:'{}'::jsonb"`
	Evidence           []byte     `gorm:"column:evidence;type:jsonb;not null;default:'[]'::jsonb"`
	IngestionRunID     string     `gorm:"column:ingestion_run_id;not null;default:''"`
}

func (signalRow) TableName() string { return "signal_occurrences" }

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

// NewGormRepository returns a GORM-backed repository.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// NopRepository is a no-op repository used in tests and when the signal
// feature is disabled. All writes are ignored; all reads return empty.
type NopRepository struct{}

func (NopRepository) Upsert(context.Context, *Occurrence) error { return nil }
func (NopRepository) Get(context.Context, int64) (Occurrence, error) {
	return Occurrence{}, ErrSignalNotFound
}
func (NopRepository) List(context.Context, ListFilter) ([]Occurrence, int64, error) {
	return nil, 0, nil
}
func (NopRepository) CountBySignal(context.Context, *int64, string, time.Time, int) ([]OverviewSignal, error) {
	return nil, nil
}
func (NopRepository) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func (r *GormRepository) Upsert(ctx context.Context, occ *Occurrence) error {
	row, err := occurrenceToRow(occ)
	if err != nil {
		return err
	}
	// ON CONFLICT (signal_id, fingerprint) DO UPDATE: refresh state, freshness,
	// observed_at, evidence and ingested_at. Identity fields are immutable.
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "signal_id"}, {Name: "fingerprint"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"state":        row.State,
			"severity":     row.Severity,
			"coverage":     row.Coverage,
			"freshness":    row.Freshness,
			"observed_at":  row.ObservedAt,
			"ingested_at":  row.IngestedAt,
			"expires_at":   row.ExpiresAt,
			"attributes":   row.Attributes,
			"evidence":     row.Evidence,
			"window_start": row.WindowStart,
			"window_end":   row.WindowEnd,
		}),
	}).Create(&row).Error
}

// Get returns a single occurrence by id.
func (r *GormRepository) Get(ctx context.Context, id int64) (Occurrence, error) {
	var row signalRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Occurrence{}, ErrSignalNotFound
	}
	if err != nil {
		return Occurrence{}, err
	}
	return rowToOccurrence(&row)
}

func (r *GormRepository) List(ctx context.Context, filter ListFilter) ([]Occurrence, int64, error) {
	q := r.db.WithContext(ctx).Model(&signalRow{})
	if filter.ClusterID != nil {
		q = q.Where("cluster_id = ?", *filter.ClusterID)
	}
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	if filter.SignalID != "" {
		q = q.Where("signal_id = ?", filter.SignalID)
	}
	if filter.Producer != "" {
		q = q.Where("producer = ?", filter.Producer)
	}
	if filter.State != "" {
		q = q.Where("state = ?", filter.State)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.WindowStart != nil {
		q = q.Where("observed_at >= ?", *filter.WindowStart)
	}
	if filter.WindowEnd != nil {
		q = q.Where("observed_at <= ?", *filter.WindowEnd)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []signalRow
	if err := q.Order("observed_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Occurrence, 0, len(rows))
	for i := range rows {
		occ, err := rowToOccurrence(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		items = append(items, occ)
	}
	return items, total, nil
}

func (r *GormRepository) CountBySignal(ctx context.Context, clusterID *int64, namespace string, since time.Time, limit int) ([]OverviewSignal, error) {
	type aggRow struct {
		SignalID  string    `gorm:"column:signal_id"`
		Producer  string    `gorm:"column:producer"`
		Severity  string    `gorm:"column:severity"`
		Count     int64     `gorm:"column:cnt"`
		LastSeen  time.Time `gorm:"column:last_seen"`
		Namespace string    `gorm:"column:namespace"`
	}
	q := r.db.WithContext(ctx).Table("signal_occurrences").
		Select("signal_id, producer, severity, COUNT(*) AS cnt, MAX(observed_at) AS last_seen, MAX(namespace) AS namespace").
		Where("state = ? AND observed_at >= ?", StateActive, since)
	if clusterID != nil {
		q = q.Where("cluster_id = ?", *clusterID)
	}
	if namespace != "" {
		q = q.Where("namespace = ?", namespace)
	}
	q = q.Group("signal_id, producer, severity").Order("cnt DESC")
	if limit > 0 {
		q = q.Limit(limit)
	} else {
		q = q.Limit(20)
	}
	var rows []aggRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OverviewSignal, 0, len(rows))
	for _, row := range rows {
		out = append(out, OverviewSignal{
			SignalID:  row.SignalID,
			Producer:  Producer(row.Producer),
			Severity:  Severity(row.Severity),
			Count:     row.Count,
			LastSeen:  row.LastSeen,
			Namespace: row.Namespace,
		})
	}
	return out, nil
}

func (r *GormRepository) DeleteExpired(ctx context.Context, now time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	result := r.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now).
		Delete(&signalRow{}).Limit(batchSize)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// --- row conversion helpers ---

func occurrenceToRow(occ *Occurrence) (signalRow, error) {
	attrs, err := json.Marshal(occ.Attributes)
	if err != nil {
		return signalRow{}, err
	}
	if len(attrs) == 0 || string(attrs) == "null" {
		attrs = []byte("{}")
	}
	ev, err := json.Marshal(occ.Evidence)
	if err != nil {
		return signalRow{}, err
	}
	if len(ev) == 0 || string(ev) == "null" {
		ev = []byte("[]")
	}
	return signalRow{
		SignalID:           occ.SignalID,
		SignalCode:         occ.SignalCode,
		SchemaVersion:      occ.SchemaVersion,
		Producer:           string(occ.Producer),
		ClusterID:          occ.ClusterID,
		Namespace:          occ.Namespace,
		ResourceKind:       occ.Resource.Kind,
		ResourceNamespace:  occ.Resource.Namespace,
		ResourceName:       occ.Resource.Name,
		ResourceUID:        occ.Resource.UID,
		ResourceIncomplete: occ.Resource.Incomplete,
		Severity:           string(occ.Severity),
		State:              string(occ.State),
		Fingerprint:        occ.Fingerprint,
		Coverage:           string(occ.Coverage),
		Freshness:          occ.Freshness,
		WindowStart:        occ.WindowStart,
		WindowEnd:          occ.WindowEnd,
		ObservedAt:         occ.ObservedAt,
		IngestedAt:         occ.IngestedAt,
		ExpiresAt:          occ.ExpiresAt,
		Attributes:         attrs,
		Evidence:           ev,
		IngestionRunID:     occ.IngestionRunID,
	}, nil
}

func rowToOccurrence(row *signalRow) (Occurrence, error) {
	var attrs map[string]string
	if len(row.Attributes) > 0 {
		if err := json.Unmarshal(row.Attributes, &attrs); err != nil {
			return Occurrence{}, err
		}
	}
	var evidence []EvidenceRef
	if len(row.Evidence) > 0 {
		if err := json.Unmarshal(row.Evidence, &evidence); err != nil {
			return Occurrence{}, err
		}
	}
	return Occurrence{
		ID:            row.ID,
		SignalID:      row.SignalID,
		SignalCode:    row.SignalCode,
		SchemaVersion: row.SchemaVersion,
		Producer:      Producer(row.Producer),
		ClusterID:     row.ClusterID,
		Namespace:     row.Namespace,
		Resource: ResourceCitation{
			Kind:       row.ResourceKind,
			Namespace:  row.ResourceNamespace,
			Name:       row.ResourceName,
			UID:        row.ResourceUID,
			Incomplete: row.ResourceIncomplete,
		},
		Severity:       Severity(row.Severity),
		State:          State(row.State),
		Fingerprint:    row.Fingerprint,
		Coverage:       Coverage(row.Coverage),
		Freshness:      row.Freshness,
		WindowStart:    row.WindowStart,
		WindowEnd:      row.WindowEnd,
		ObservedAt:     row.ObservedAt,
		IngestedAt:     row.IngestedAt,
		ExpiresAt:      row.ExpiresAt,
		Attributes:     attrs,
		Evidence:       evidence,
		IngestionRunID: row.IngestionRunID,
	}, nil
}

// ErrSignalNotFound is returned when a Get-by-id operation matches no row.
var ErrSignalNotFound = errors.New("signal occurrence not found")
