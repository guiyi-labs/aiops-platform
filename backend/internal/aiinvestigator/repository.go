package aiinvestigator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository persists AI investigations. The service is the only writer.
type Repository interface {
	// Save persists an investigation. When an active investigation with the
	// same investigation_key exists, it is marked stale and the new one is
	// inserted (audit history retained).
	Save(ctx context.Context, inv *Investigation) error
	// Get returns one investigation by ID.
	Get(ctx context.Context, id int64) (Investigation, error)
	// ListByCase returns investigations for a case, ordered by created_at DESC.
	ListByCase(ctx context.Context, caseID int64, limit int) ([]Investigation, int64, error)
	// ListByFilter returns investigations matching the filter.
	ListByFilter(ctx context.Context, filter InvestigationFilter) ([]Investigation, int64, error)
	// MarkStale marks older investigations for the same case_key as stale.
	MarkStale(ctx context.Context, caseID int64, investigationKey string, exceptID int64) error
}

// ErrInvestigationNotFound is returned when an investigation does not exist.
var ErrInvestigationNotFound = errors.New("ai investigation not found")

// --- GORM models ---

type investigationRow struct {
	ID                   int64     `gorm:"column:id;primaryKey;autoIncrement"`
	CaseID               int64     `gorm:"column:case_id;not null"`
	InvestigationKey     string    `gorm:"column:investigation_key;not null"`
	InvestigatorVersion  string    `gorm:"column:investigator_version;not null;default:1.0"`
	ActorID              int64     `gorm:"column:actor_id;not null"`
	ActorName            string    `gorm:"column:actor_name;not null"`
	Provider             string    `gorm:"column:provider;not null"`
	Model                string    `gorm:"column:model;not null"`
	ProviderResponseID   string    `gorm:"column:provider_response_id;not null;default:''"`
	Status               string    `gorm:"column:status;not null;default:completed"`
	Summary              string    `gorm:"column:summary;not null;default:''"`
	Impact               string    `gorm:"column:impact;not null;default:''"`
	Hypotheses           JSONB     `gorm:"column:hypotheses;not null;default:'[]'"`
	RecommendedRunbookID string    `gorm:"column:recommended_runbook_id;not null;default:''"`
	Uncertainties        JSONB     `gorm:"column:uncertainties;not null;default:'[]'"`
	Citations            JSONB     `gorm:"column:citations;not null;default:'[]'"`
	InputTokens          int       `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens         int       `gorm:"column:output_tokens;not null;default:0"`
	FailureReason        string    `gorm:"column:failure_reason;not null;default:''"`
	CreatedAt            time.Time `gorm:"column:created_at;not null;default:NOW()"`
}

func (investigationRow) TableName() string { return "ai_investigations" }

// JSONB is a json.Marshaler/Unmarshaler wrapper for JSONB columns.
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

func (NopRepository) Save(context.Context, *Investigation) error { return nil }
func (NopRepository) Get(context.Context, int64) (Investigation, error) {
	return Investigation{}, ErrInvestigationNotFound
}
func (NopRepository) ListByCase(context.Context, int64, int) ([]Investigation, int64, error) {
	return nil, 0, nil
}
func (NopRepository) ListByFilter(context.Context, InvestigationFilter) ([]Investigation, int64, error) {
	return nil, 0, nil
}
func (NopRepository) MarkStale(context.Context, int64, string, int64) error { return nil }

func (r *GormRepository) Save(ctx context.Context, inv *Investigation) error {
	row := investigationToRow(inv)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	inv.ID = row.ID
	// Mark older active investigations for the same case_key as stale.
	return r.MarkStale(ctx, inv.CaseID, inv.InvestigationKey, inv.ID)
}

func (r *GormRepository) Get(ctx context.Context, id int64) (Investigation, error) {
	var row investigationRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Investigation{}, ErrInvestigationNotFound
		}
		return Investigation{}, err
	}
	return rowToInvestigation(&row), nil
}

func (r *GormRepository) ListByCase(ctx context.Context, caseID int64, limit int) ([]Investigation, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := r.db.WithContext(ctx).Model(&investigationRow{}).Where("case_id = ?", caseID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []investigationRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Investigation, 0, len(rows))
	for i := range rows {
		items = append(items, rowToInvestigation(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) ListByFilter(ctx context.Context, filter InvestigationFilter) ([]Investigation, int64, error) {
	q := r.db.WithContext(ctx).Model(&investigationRow{})
	if filter.CaseID > 0 {
		q = q.Where("case_id = ?", filter.CaseID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", string(filter.Status))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []investigationRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Investigation, 0, len(rows))
	for i := range rows {
		items = append(items, rowToInvestigation(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) MarkStale(ctx context.Context, caseID int64, investigationKey string, exceptID int64) error {
	return r.db.WithContext(ctx).
		Model(&investigationRow{}).
		Where("case_id = ? AND investigation_key = ? AND id != ? AND status != ?",
			caseID, investigationKey, exceptID, string(InvestigationStatusStale)).
		Update("status", string(InvestigationStatusStale)).Error
}

// --- row conversion helpers ---

func investigationToRow(inv *Investigation) investigationRow {
	return investigationRow{
		ID:                   inv.ID,
		CaseID:               inv.CaseID,
		InvestigationKey:     inv.InvestigationKey,
		InvestigatorVersion:  inv.InvestigatorVersion,
		ActorID:              inv.Actor.ID,
		ActorName:            inv.Actor.Name,
		Provider:             inv.Provider,
		Model:                inv.Model,
		ProviderResponseID:   inv.ProviderResponseID,
		Status:               string(inv.Status),
		Summary:              inv.Summary,
		Impact:               inv.Impact,
		Hypotheses:           mustMarshalJSONB(inv.Hypotheses),
		RecommendedRunbookID: inv.RecommendedRunbookID,
		Uncertainties:        mustMarshalJSONB(inv.Uncertainties),
		Citations:            mustMarshalJSONB(inv.Citations),
		InputTokens:          inv.InputTokens,
		OutputTokens:         inv.OutputTokens,
		FailureReason:        inv.FailureReason,
		CreatedAt:            inv.CreatedAt,
	}
}

func rowToInvestigation(row *investigationRow) Investigation {
	return Investigation{
		ID:                   row.ID,
		CaseID:               row.CaseID,
		InvestigationKey:     row.InvestigationKey,
		InvestigatorVersion:  row.InvestigatorVersion,
		Actor:                ActorRef{ID: row.ActorID, Name: row.ActorName},
		Provider:             row.Provider,
		Model:                row.Model,
		ProviderResponseID:   row.ProviderResponseID,
		Status:               InvestigationStatus(row.Status),
		Summary:              row.Summary,
		Impact:               row.Impact,
		Hypotheses:           unmarshalHypotheses(row.Hypotheses),
		RecommendedRunbookID: row.RecommendedRunbookID,
		Uncertainties:        unmarshalStrings(row.Uncertainties),
		Citations:            unmarshalCitations(row.Citations),
		InputTokens:          row.InputTokens,
		OutputTokens:         row.OutputTokens,
		FailureReason:        row.FailureReason,
		CreatedAt:            row.CreatedAt,
	}
}

func mustMarshalJSONB(v interface{}) JSONB {
	b, err := json.Marshal(v)
	if err != nil {
		return JSONB("[]")
	}
	return JSONB(b)
}

func unmarshalHypotheses(j JSONB) []Hypothesis {
	if len(j) == 0 {
		return nil
	}
	var out []Hypothesis
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalStrings(j JSONB) []string {
	if len(j) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalCitations(j JSONB) []Citation {
	if len(j) == 0 {
		return nil
	}
	var out []Citation
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}
