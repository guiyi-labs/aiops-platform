package slo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists SLO definitions and evaluations.
type Repository interface {
	// CreateDefinition inserts a new SLO definition. Version starts at 1.
	CreateDefinition(ctx context.Context, def *Definition) error
	// GetDefinition returns the current definition for an ID.
	GetDefinition(ctx context.Context, id int64) (Definition, error)
	// ListDefinitions returns definitions matching the filter.
	ListDefinitions(ctx context.Context, filter DefinitionFilter) ([]Definition, int64, error)
	// UpdateDefinition applies a versioned patch. Version is incremented and
	// updated_at is refreshed. Returns the updated definition.
	UpdateDefinition(ctx context.Context, id int64, patch PatchDefinitionInput, now time.Time) (Definition, error)
	// DeleteDefinition marks a definition disabled (enabled=false) and removes
	// it from the active unique index. Historical evaluations are preserved.
	DeleteDefinition(ctx context.Context, id int64) error
	// InsertEvaluation appends an evaluation. Evaluations are append-only.
	InsertEvaluation(ctx context.Context, eval *Evaluation) error
	// LatestEvaluation returns the most recent evaluation for an SLO.
	LatestEvaluation(ctx context.Context, sloID int64) (Evaluation, error)
	// ListEvaluations returns evaluations matching the filter.
	ListEvaluations(ctx context.Context, filter EvaluationFilter) ([]Evaluation, int64, error)
}

// ErrDefinitionNotFound is returned when an SLO definition does not exist.
var ErrDefinitionNotFound = errors.New("slo definition not found")

// ErrDuplicateDefinition is returned when an active definition already exists
// for the same (cluster, namespace, service, template).
var ErrDuplicateDefinition = errors.New("active slo definition already exists for this service and template")

// --- GORM models ---

type definitionRow struct {
	ID                    int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ClusterID             int64     `gorm:"column:cluster_id;not null"`
	ServiceKind           string    `gorm:"column:service_kind;not null"`
	ServiceNamespace      string    `gorm:"column:service_namespace;not null;default:''"`
	ServiceName           string    `gorm:"column:service_name;not null"`
	ServiceUID            string    `gorm:"column:service_uid;not null;default:''"`
	ServiceIncomplete     bool      `gorm:"column:service_incomplete;not null;default:false"`
	Template              string    `gorm:"column:template;not null"`
	TemplateVersion       string    `gorm:"column:template_version;not null"`
	Objective             float64   `gorm:"column:objective;not null"`
	RollingWindowSeconds  int       `gorm:"column:rolling_window_seconds;not null"`
	MissingDataPolicy     string    `gorm:"column:missing_data_policy;not null;default:unavailable"`
	LatencyThresholdMs    int       `gorm:"column:latency_threshold_ms;not null;default:0"`
	OwnerID               int64     `gorm:"column:owner_id;not null"`
	OwnerName             string    `gorm:"column:owner_name;not null;default:''"`
	FastBurnRate          float64   `gorm:"column:fast_burn_rate;not null"`
	FastBurnWindowSeconds int       `gorm:"column:fast_burn_window_seconds;not null"`
	SlowBurnRate          float64   `gorm:"column:slow_burn_rate;not null"`
	SlowBurnWindowSeconds int       `gorm:"column:slow_burn_window_seconds;not null"`
	Enabled               bool      `gorm:"column:enabled;not null;default:true"`
	Version               int       `gorm:"column:version;not null;default:1"`
	CreatorID             int64     `gorm:"column:creator_id;not null"`
	CreatorName           string    `gorm:"column:creator_name;not null;default:''"`
	CreatedAt             time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (definitionRow) TableName() string { return "slo_definitions" }

type evaluationRow struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SLOID           int64     `gorm:"column:slo_id;not null"`
	Version         int       `gorm:"column:version;not null"`
	WindowStart     time.Time `gorm:"column:window_start;not null"`
	WindowEnd       time.Time `gorm:"column:window_end;not null"`
	GoodEvents      float64   `gorm:"column:good_events;not null"`
	TotalEvents     float64   `gorm:"column:total_events;not null"`
	Ratio           float64   `gorm:"column:ratio;not null"`
	TargetRatio     float64   `gorm:"column:target_ratio;not null"`
	ErrorBudget     float64   `gorm:"column:error_budget;not null"`
	RemainingBudget float64   `gorm:"column:remaining_budget;not null"`
	BurnRate        float64   `gorm:"column:burn_rate;not null"`
	State           string    `gorm:"column:state;not null"`
	Coverage        string    `gorm:"column:coverage;not null"`
	EvaluatedAt     time.Time `gorm:"column:evaluated_at;not null;default:NOW()"`
}

func (evaluationRow) TableName() string { return "slo_evaluations" }

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// NopRepository is a no-op repository for disabled/testing mode.
type NopRepository struct{}

func (NopRepository) CreateDefinition(context.Context, *Definition) error { return nil }
func (NopRepository) GetDefinition(context.Context, int64) (Definition, error) {
	return Definition{}, ErrDefinitionNotFound
}
func (NopRepository) ListDefinitions(context.Context, DefinitionFilter) ([]Definition, int64, error) {
	return nil, 0, nil
}
func (NopRepository) UpdateDefinition(context.Context, int64, PatchDefinitionInput, time.Time) (Definition, error) {
	return Definition{}, ErrDefinitionNotFound
}
func (NopRepository) DeleteDefinition(context.Context, int64) error       { return nil }
func (NopRepository) InsertEvaluation(context.Context, *Evaluation) error { return nil }
func (NopRepository) LatestEvaluation(context.Context, int64) (Evaluation, error) {
	return Evaluation{}, nil
}
func (NopRepository) ListEvaluations(context.Context, EvaluationFilter) ([]Evaluation, int64, error) {
	return nil, 0, nil
}

func (r *GormRepository) CreateDefinition(ctx context.Context, def *Definition) error {
	row := definitionToRow(def)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	// gorm assigns the generated ID to the row; propagate it back so the
	// created definition returned to the caller carries its identity.
	def.ID = row.ID
	return nil
}

func (r *GormRepository) GetDefinition(ctx context.Context, id int64) (Definition, error) {
	var row definitionRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Definition{}, ErrDefinitionNotFound
		}
		return Definition{}, err
	}
	return rowToDefinition(&row), nil
}

func (r *GormRepository) ListDefinitions(ctx context.Context, filter DefinitionFilter) ([]Definition, int64, error) {
	q := r.db.WithContext(ctx).Model(&definitionRow{}).Where("cluster_id = ?", filter.ClusterID)
	if filter.Namespace != "" {
		q = q.Where("service_namespace = ?", filter.Namespace)
	}
	if filter.Template != "" {
		q = q.Where("template = ?", string(filter.Template))
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if filter.OwnerID > 0 {
		q = q.Where("owner_id = ?", filter.OwnerID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []definitionRow
	if err := q.Order("updated_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Definition, 0, len(rows))
	for i := range rows {
		items = append(items, rowToDefinition(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) UpdateDefinition(ctx context.Context, id int64, patch PatchDefinitionInput, now time.Time) (Definition, error) {
	var row definitionRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Definition{}, ErrDefinitionNotFound
		}
		return Definition{}, err
	}
	applyPatch(&row, patch)
	row.Version = row.Version + 1
	row.UpdatedAt = now
	if err := r.db.WithContext(ctx).Save(&row).Error; err != nil {
		return Definition{}, err
	}
	updated := rowToDefinition(&row)
	if err := ValidateDefinition(&updated); err != nil {
		return Definition{}, err
	}
	return updated, nil
}

func (r *GormRepository) DeleteDefinition(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Model(&definitionRow{}).Where("id = ?", id).Update("enabled", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDefinitionNotFound
	}
	return nil
}

func (r *GormRepository) InsertEvaluation(ctx context.Context, eval *Evaluation) error {
	row := evaluationToRow(eval)
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(&row).Error
}

func (r *GormRepository) LatestEvaluation(ctx context.Context, sloID int64) (Evaluation, error) {
	var row evaluationRow
	err := r.db.WithContext(ctx).Where("slo_id = ?", sloID).Order("evaluated_at DESC, id DESC").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Evaluation{}, nil
		}
		return Evaluation{}, err
	}
	return rowToEvaluation(&row), nil
}

func (r *GormRepository) ListEvaluations(ctx context.Context, filter EvaluationFilter) ([]Evaluation, int64, error) {
	q := r.db.WithContext(ctx).Model(&evaluationRow{}).Where("slo_id = ?", filter.SLOID)
	if filter.Version != nil {
		q = q.Where("version = ?", *filter.Version)
	}
	if filter.State != "" {
		q = q.Where("state = ?", string(filter.State))
	}
	if filter.StartTime != nil {
		q = q.Where("window_end >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		q = q.Where("window_start <= ?", *filter.EndTime)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []evaluationRow
	if err := q.Order("evaluated_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Evaluation, 0, len(rows))
	for i := range rows {
		items = append(items, rowToEvaluation(&rows[i]))
	}
	return items, total, nil
}

// --- row conversion helpers ---

func definitionToRow(def *Definition) definitionRow {
	return definitionRow{
		ClusterID:             def.ClusterID,
		ServiceKind:           def.Service.Kind,
		ServiceNamespace:      def.Service.Namespace,
		ServiceName:           def.Service.Name,
		ServiceUID:            def.Service.UID,
		ServiceIncomplete:     def.Service.Incomplete,
		Template:              string(def.Template),
		TemplateVersion:       def.TemplateVersion,
		Objective:             def.Objective,
		RollingWindowSeconds:  def.RollingWindowSeconds,
		MissingDataPolicy:     string(def.MissingDataPolicy),
		LatencyThresholdMs:    def.LatencyThresholdMs,
		OwnerID:               def.Owner.ID,
		OwnerName:             def.Owner.Name,
		FastBurnRate:          def.FastBurnRate,
		FastBurnWindowSeconds: def.FastBurnWindowSeconds,
		SlowBurnRate:          def.SlowBurnRate,
		SlowBurnWindowSeconds: def.SlowBurnWindowSeconds,
		Enabled:               def.Enabled,
		Version:               def.Version,
		CreatorID:             def.Creator.ID,
		CreatorName:           def.Creator.Name,
		CreatedAt:             def.CreatedAt,
		UpdatedAt:             def.UpdatedAt,
	}
}

func rowToDefinition(row *definitionRow) Definition {
	return Definition{
		ID:        row.ID,
		ClusterID: row.ClusterID,
		Service: ServiceRef{
			Kind:       row.ServiceKind,
			Namespace:  row.ServiceNamespace,
			Name:       row.ServiceName,
			UID:        row.ServiceUID,
			Incomplete: row.ServiceIncomplete,
		},
		Template:              SLITemplate(row.Template),
		TemplateVersion:       row.TemplateVersion,
		Objective:             row.Objective,
		RollingWindowSeconds:  row.RollingWindowSeconds,
		MissingDataPolicy:     MissingDataPolicy(row.MissingDataPolicy),
		LatencyThresholdMs:    row.LatencyThresholdMs,
		Owner:                 ActorRef{ID: row.OwnerID, Name: row.OwnerName},
		FastBurnRate:          row.FastBurnRate,
		FastBurnWindowSeconds: row.FastBurnWindowSeconds,
		SlowBurnRate:          row.SlowBurnRate,
		SlowBurnWindowSeconds: row.SlowBurnWindowSeconds,
		Enabled:               row.Enabled,
		Version:               row.Version,
		Creator:               ActorRef{ID: row.CreatorID, Name: row.CreatorName},
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}

func evaluationToRow(eval *Evaluation) evaluationRow {
	return evaluationRow{
		ID:              eval.ID,
		SLOID:           eval.SLOID,
		Version:         eval.Version,
		WindowStart:     eval.WindowStart,
		WindowEnd:       eval.WindowEnd,
		GoodEvents:      eval.GoodEvents,
		TotalEvents:     eval.TotalEvents,
		Ratio:           eval.Ratio,
		TargetRatio:     eval.TargetRatio,
		ErrorBudget:     eval.ErrorBudget,
		RemainingBudget: eval.RemainingBudget,
		BurnRate:        eval.BurnRate,
		State:           string(eval.State),
		Coverage:        string(eval.Coverage),
		EvaluatedAt:     eval.EvaluatedAt,
	}
}

func rowToEvaluation(row *evaluationRow) Evaluation {
	return Evaluation{
		ID:              row.ID,
		SLOID:           row.SLOID,
		Version:         row.Version,
		WindowStart:     row.WindowStart,
		WindowEnd:       row.WindowEnd,
		GoodEvents:      row.GoodEvents,
		TotalEvents:     row.TotalEvents,
		Ratio:           row.Ratio,
		TargetRatio:     row.TargetRatio,
		ErrorBudget:     row.ErrorBudget,
		RemainingBudget: row.RemainingBudget,
		BurnRate:        row.BurnRate,
		State:           EvaluationState(row.State),
		Coverage:        EvaluationCoverage(row.Coverage),
		EvaluatedAt:     row.EvaluatedAt,
	}
}

func applyPatch(row *definitionRow, patch PatchDefinitionInput) {
	if patch.Objective != nil {
		row.Objective = *patch.Objective
	}
	if patch.RollingWindowSeconds != nil {
		row.RollingWindowSeconds = *patch.RollingWindowSeconds
	}
	if patch.MissingDataPolicy != nil {
		row.MissingDataPolicy = string(*patch.MissingDataPolicy)
	}
	if patch.LatencyThresholdMs != nil {
		row.LatencyThresholdMs = *patch.LatencyThresholdMs
	}
	if patch.Owner != nil {
		row.OwnerID = patch.Owner.ID
		row.OwnerName = patch.Owner.Name
	}
	if patch.FastBurnRate != nil {
		row.FastBurnRate = *patch.FastBurnRate
	}
	if patch.FastBurnWindowSeconds != nil {
		row.FastBurnWindowSeconds = *patch.FastBurnWindowSeconds
	}
	if patch.SlowBurnRate != nil {
		row.SlowBurnRate = *patch.SlowBurnRate
	}
	if patch.SlowBurnWindowSeconds != nil {
		row.SlowBurnWindowSeconds = *patch.SlowBurnWindowSeconds
	}
	if patch.Enabled != nil {
		row.Enabled = *patch.Enabled
	}
}

// marshalEvalJSON is a helper for tests that need to compare evaluation
// payloads; the production path uses GORM directly.
func marshalEvalJSON(eval *Evaluation) ([]byte, error) {
	return json.Marshal(eval)
}

var _ = marshalEvalJSON
