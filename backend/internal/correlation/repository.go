package correlation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists correlation cases, signal/resource links and change
// candidates. The engine is pure and stateless; the service is the only writer
// via this repository.
type Repository interface {
	// UpsertResult persists a CorrelationResult idempotently. When an active
	// case with the same case_key already exists, the case's factors,
	// observed window, confidence and links are merged; new change candidates
	// and signal/resource links are inserted (deduplicated by unique index).
	// The returned Case carries the persisted ID so the service can wire
	// candidate/link CaseID fields.
	UpsertResult(ctx context.Context, result *CorrelationResult) (Case, error)
	// GetCase returns the full CaseView (case + signal links + resource links
	// + change candidates) for one case ID.
	GetCase(ctx context.Context, id int64) (CaseView, error)
	// ListCases returns cases matching the filter, ordered by
	// last_observed_at DESC then id DESC.
	ListCases(ctx context.Context, filter CaseFilter) ([]Case, int64, error)
	// ListTimeline returns cases ordered by first_observed_at ASC for the
	// bounded timeline view.
	ListTimeline(ctx context.Context, filter CaseFilter) ([]Case, int64, error)
	// ListSignalLinks returns the signal links for one case.
	ListSignalLinks(ctx context.Context, caseID int64) ([]SignalLink, error)
	// ListResourceLinks returns the resource links for one case (the impact
	// graph).
	ListResourceLinks(ctx context.Context, caseID int64) ([]ResourceLink, error)
	// ListChangeCandidates returns the change candidates for one case,
	// ordered by rank ASC.
	ListChangeCandidates(ctx context.Context, caseID int64) ([]ChangeCandidate, error)
	// ResolveCaseStatus updates a case's status when its linked signals
	// resolve. Used by the service layer's status reconciler.
	ResolveCaseStatus(ctx context.Context, caseID int64, status CaseStatus, now time.Time) error
}

// ErrCaseNotFound is returned when a case does not exist.
var ErrCaseNotFound = errors.New("correlation case not found")

// --- GORM models ---

type caseRow struct {
	ID                    int64     `gorm:"column:id;primaryKey;autoIncrement"`
	CaseKey               string    `gorm:"column:case_key;not null"`
	ClusterID             int64     `gorm:"column:cluster_id;not null"`
	RuleID                string    `gorm:"column:rule_id;not null"`
	CorrelationVersion    string    `gorm:"column:correlation_version;not null;default:1.0"`
	PrimaryKind           string    `gorm:"column:primary_kind;not null"`
	PrimaryNamespace      string    `gorm:"column:primary_namespace;not null;default:''"`
	PrimaryName           string    `gorm:"column:primary_name;not null"`
	PrimaryUID            string    `gorm:"column:primary_uid;not null;default:''"`
	PrimaryIncomplete     bool      `gorm:"column:primary_incomplete;not null;default:false"`
	Status                string    `gorm:"column:status;not null;default:active"`
	Confidence            string    `gorm:"column:confidence;not null;default:unknown"`
	EvidenceCompleteness  string    `gorm:"column:evidence_completeness;not null;default:insufficient"`
	Factors               JSONB     `gorm:"column:factors;not null;default:'[]'"`
	DiagnosisIDs          JSONB     `gorm:"column:diagnosis_ids;not null;default:'[]'"`
	RootChangeCandidateID *int64    `gorm:"column:root_change_candidate_id"`
	FirstObservedAt       time.Time `gorm:"column:first_observed_at;not null"`
	LastObservedAt        time.Time `gorm:"column:last_observed_at;not null"`
	CreatedAt             time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (caseRow) TableName() string { return "correlation_cases" }

type signalLinkRow struct {
	ID                 int64      `gorm:"column:id;primaryKey;autoIncrement"`
	CaseID             int64      `gorm:"column:case_id;not null"`
	SignalOccurrenceID int64      `gorm:"column:signal_occurrence_id;not null"`
	Relation           string     `gorm:"column:relation;not null"`
	SignalID           string     `gorm:"column:signal_id;not null"`
	Producer           string     `gorm:"column:producer;not null"`
	ObservedAt         time.Time  `gorm:"column:observed_at;not null"`
	Coverage           string     `gorm:"column:coverage;not null;default:complete"`
	Freshness          *time.Time `gorm:"column:freshness"`
	WindowStart        *time.Time `gorm:"column:window_start"`
	WindowEnd          *time.Time `gorm:"column:window_end"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null;default:NOW()"`
}

func (signalLinkRow) TableName() string { return "correlation_signal_links" }

type resourceLinkRow struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	CaseID       int64     `gorm:"column:case_id;not null"`
	Kind         string    `gorm:"column:kind;not null"`
	Namespace    string    `gorm:"column:namespace;not null;default:''"`
	Name         string    `gorm:"column:name;not null"`
	UID          string    `gorm:"column:uid;not null;default:''"`
	Incomplete   bool      `gorm:"column:incomplete;not null;default:false"`
	Relation     string    `gorm:"column:relation;not null"`
	TopologyPath JSONB     `gorm:"column:topology_path;not null;default:'[]'"`
	EdgeIDs      JSONB     `gorm:"column:edge_ids;not null;default:'[]'"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;default:NOW()"`
}

func (resourceLinkRow) TableName() string { return "correlation_resource_links" }

type changeCandidateRow struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement"`
	CaseID            int64     `gorm:"column:case_id;not null"`
	ChangeEventID     int64     `gorm:"column:change_event_id;not null"`
	RuleID            string    `gorm:"column:rule_id;not null"`
	Confidence        string    `gorm:"column:confidence;not null;default:unknown"`
	Rank              int       `gorm:"column:rank;not null;default:1"`
	Factors           JSONB     `gorm:"column:factors;not null;default:'[]'"`
	EvidenceRefs      JSONB     `gorm:"column:evidence_refs;not null;default:'[]'"`
	ContradictingRefs JSONB     `gorm:"column:contradicting_refs;not null;default:'[]'"`
	ReasonCode        string    `gorm:"column:reason_code;not null;default:''"`
	CreatedAt         time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (changeCandidateRow) TableName() string { return "correlation_change_candidates" }

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

func (NopRepository) UpsertResult(context.Context, *CorrelationResult) (Case, error) {
	return Case{}, nil
}
func (NopRepository) GetCase(context.Context, int64) (CaseView, error) {
	return CaseView{}, ErrCaseNotFound
}
func (NopRepository) ListCases(context.Context, CaseFilter) ([]Case, int64, error) {
	return nil, 0, nil
}
func (NopRepository) ListTimeline(context.Context, CaseFilter) ([]Case, int64, error) {
	return nil, 0, nil
}
func (NopRepository) ListSignalLinks(context.Context, int64) ([]SignalLink, error) { return nil, nil }
func (NopRepository) ListResourceLinks(context.Context, int64) ([]ResourceLink, error) {
	return nil, nil
}
func (NopRepository) ListChangeCandidates(context.Context, int64) ([]ChangeCandidate, error) {
	return nil, nil
}
func (NopRepository) ResolveCaseStatus(context.Context, int64, CaseStatus, time.Time) error {
	return nil
}

// UpsertResult persists a CorrelationResult idempotently. The case is
// upserted on case_key (active); when an existing active case is found its
// factors, confidence and observed window are merged. Signal links, resource
// links and change candidates are inserted with ON CONFLICT DO NOTHING so
// duplicate delivery yields the same rows.
func (r *GormRepository) UpsertResult(ctx context.Context, result *CorrelationResult) (Case, error) {
	c := result.Case
	row := caseToRow(&c)

	// Upsert the case on case_key where active. On conflict, merge factors and
	// widen the observed window; promote confidence only when the new one is
	// strictly better.
	var existing caseRow
	findErr := r.db.WithContext(ctx).
		Where("case_key = ? AND status = ?", c.CaseKey, string(CaseStatusActive)).
		First(&existing).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		// Insert new case.
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return Case{}, err
		}
		c.ID = row.ID
	} else if findErr != nil {
		return Case{}, findErr
	} else {
		// Merge into existing row.
		existing.Factors = mergeFactorsJSON(existing.Factors, row.Factors)
		if c.FirstObservedAt.Before(existing.FirstObservedAt) {
			existing.FirstObservedAt = c.FirstObservedAt
		}
		if c.LastObservedAt.After(existing.LastObservedAt) {
			existing.LastObservedAt = c.LastObservedAt
		}
		if confidenceRank(c.Confidence) < confidenceRank(ConfidenceClass(existing.Confidence)) {
			existing.Confidence = string(c.Confidence)
		}
		if rankCompleteness(c.EvidenceCompleteness) > rankCompleteness(EvidenceCompleteness(existing.EvidenceCompleteness)) {
			existing.EvidenceCompleteness = string(c.EvidenceCompleteness)
		}
		existing.UpdatedAt = c.UpdatedAt
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return Case{}, err
		}
		c.ID = existing.ID
	}

	// Insert signal links (idempotent via unique index).
	for i := range result.SignalLinks {
		result.SignalLinks[i].CaseID = c.ID
		lr := signalLinkToRow(&result.SignalLinks[i])
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&lr).Error; err != nil {
			return Case{}, err
		}
	}

	// Insert resource links (idempotent via unique index).
	for i := range result.ResourceLinks {
		result.ResourceLinks[i].CaseID = c.ID
		lr := resourceLinkToRow(&result.ResourceLinks[i])
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&lr).Error; err != nil {
			return Case{}, err
		}
	}

	// Insert change candidates (idempotent via unique index on (case_id, change_event_id)).
	for i := range result.ChangeCandidates {
		result.ChangeCandidates[i].CaseID = c.ID
		cr := changeCandidateToRow(&result.ChangeCandidates[i])
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&cr).Error; err != nil {
			return Case{}, err
		}
	}

	// If a confirmed root candidate is asserted, point the case at it.
	if c.RootChangeCandidateID != nil {
		var top changeCandidateRow
		if err := r.db.WithContext(ctx).
			Where("case_id = ? AND confidence = ?", c.ID, string(ConfidenceConfirmed)).
			Order("rank ASC, id ASC").First(&top).Error; err == nil {
			_ = r.db.WithContext(ctx).Model(&caseRow{}).
				Where("id = ?", c.ID).
				Update("root_change_candidate_id", top.ID).Error
		}
	}

	return c, nil
}

func (r *GormRepository) GetCase(ctx context.Context, id int64) (CaseView, error) {
	var row caseRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CaseView{}, ErrCaseNotFound
		}
		return CaseView{}, err
	}
	c := rowToCase(&row)

	links, err := r.ListSignalLinks(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	resLinks, err := r.ListResourceLinks(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	cands, err := r.ListChangeCandidates(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	return CaseView{
		Case:             c,
		SignalLinks:      links,
		ResourceLinks:    resLinks,
		ChangeCandidates: cands,
		GeneratedAt:      time.Now().UTC(),
	}, nil
}

func (r *GormRepository) ListCases(ctx context.Context, filter CaseFilter) ([]Case, int64, error) {
	q := r.db.WithContext(ctx).Model(&caseRow{})
	q = applyCaseFilter(q, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []caseRow
	if err := q.Order("last_observed_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Case, 0, len(rows))
	for i := range rows {
		items = append(items, rowToCase(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) ListTimeline(ctx context.Context, filter CaseFilter) ([]Case, int64, error) {
	q := r.db.WithContext(ctx).Model(&caseRow{})
	q = applyCaseFilter(q, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []caseRow
	if err := q.Order("first_observed_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Case, 0, len(rows))
	for i := range rows {
		items = append(items, rowToCase(&rows[i]))
	}
	return items, total, nil
}

func (r *GormRepository) ListSignalLinks(ctx context.Context, caseID int64) ([]SignalLink, error) {
	var rows []signalLinkRow
	if err := r.db.WithContext(ctx).Where("case_id = ?", caseID).
		Order("observed_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]SignalLink, 0, len(rows))
	for i := range rows {
		items = append(items, rowToSignalLink(&rows[i]))
	}
	return items, nil
}

func (r *GormRepository) ListResourceLinks(ctx context.Context, caseID int64) ([]ResourceLink, error) {
	var rows []resourceLinkRow
	if err := r.db.WithContext(ctx).Where("case_id = ?", caseID).
		Order("relation ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]ResourceLink, 0, len(rows))
	for i := range rows {
		items = append(items, rowToResourceLink(&rows[i]))
	}
	return items, nil
}

func (r *GormRepository) ListChangeCandidates(ctx context.Context, caseID int64) ([]ChangeCandidate, error) {
	var rows []changeCandidateRow
	if err := r.db.WithContext(ctx).Where("case_id = ?", caseID).
		Order("rank ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]ChangeCandidate, 0, len(rows))
	for i := range rows {
		items = append(items, rowToChangeCandidate(&rows[i]))
	}
	return items, nil
}

func (r *GormRepository) ResolveCaseStatus(ctx context.Context, caseID int64, status CaseStatus, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&caseRow{}).
		Where("id = ?", caseID).
		Updates(map[string]interface{}{
			"status":     string(status),
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCaseNotFound
	}
	return nil
}

// applyCaseFilter is shared by ListCases and ListTimeline.
func applyCaseFilter(q *gorm.DB, filter CaseFilter) *gorm.DB {
	if filter.ClusterID > 0 {
		q = q.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.Namespace != "" {
		q = q.Where("primary_namespace = ?", filter.Namespace)
	}
	if filter.RuleID != "" {
		q = q.Where("rule_id = ?", filter.RuleID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", string(filter.Status))
	}
	if filter.Confidence != "" {
		q = q.Where("confidence = ?", string(filter.Confidence))
	}
	if filter.PrimaryKind != "" {
		q = q.Where("primary_kind = ?", filter.PrimaryKind)
	}
	if filter.PrimaryUID != "" {
		q = q.Where("primary_uid = ?", filter.PrimaryUID)
	}
	if filter.StartTime != nil {
		q = q.Where("last_observed_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		q = q.Where("first_observed_at <= ?", *filter.EndTime)
	}
	return q
}

// --- row conversion helpers ---

func caseToRow(c *Case) caseRow {
	return caseRow{
		ID:                    c.ID,
		CaseKey:               c.CaseKey,
		ClusterID:             c.ClusterID,
		RuleID:                c.RuleID,
		CorrelationVersion:    c.CorrelationVersion,
		PrimaryKind:           c.PrimaryResource.Kind,
		PrimaryNamespace:      c.PrimaryResource.Namespace,
		PrimaryName:           c.PrimaryResource.Name,
		PrimaryUID:            c.PrimaryResource.UID,
		PrimaryIncomplete:     c.PrimaryResource.Incomplete,
		Status:                string(c.Status),
		Confidence:            string(c.Confidence),
		EvidenceCompleteness:  string(c.EvidenceCompleteness),
		Factors:               mustMarshalJSONB(c.Factors),
		DiagnosisIDs:          mustMarshalJSONB(c.DiagnosisIDs),
		RootChangeCandidateID: c.RootChangeCandidateID,
		FirstObservedAt:       c.FirstObservedAt,
		LastObservedAt:        c.LastObservedAt,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}
}

func rowToCase(row *caseRow) Case {
	c := Case{
		ID:                 row.ID,
		CaseKey:            row.CaseKey,
		ClusterID:          row.ClusterID,
		RuleID:             row.RuleID,
		CorrelationVersion: row.CorrelationVersion,
		PrimaryResource: ResourceCitation{
			Kind:       row.PrimaryKind,
			Namespace:  row.PrimaryNamespace,
			Name:       row.PrimaryName,
			UID:        row.PrimaryUID,
			Incomplete: row.PrimaryIncomplete,
		},
		Status:                CaseStatus(row.Status),
		Confidence:            ConfidenceClass(row.Confidence),
		EvidenceCompleteness:  EvidenceCompleteness(row.EvidenceCompleteness),
		Factors:               unmarshalFactors(row.Factors),
		DiagnosisIDs:          unmarshalInt64s(row.DiagnosisIDs),
		RootChangeCandidateID: row.RootChangeCandidateID,
		FirstObservedAt:       row.FirstObservedAt,
		LastObservedAt:        row.LastObservedAt,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
	return c
}

func signalLinkToRow(l *SignalLink) signalLinkRow {
	return signalLinkRow{
		ID:                 l.ID,
		CaseID:             l.CaseID,
		SignalOccurrenceID: l.SignalOccurrenceID,
		Relation:           string(l.Relation),
		SignalID:           l.SignalID,
		Producer:           l.Producer,
		ObservedAt:         l.ObservedAt,
		Coverage:           l.Coverage,
		Freshness:          l.Freshness,
		WindowStart:        l.WindowStart,
		WindowEnd:          l.WindowEnd,
		CreatedAt:          l.CreatedAt,
	}
}

func rowToSignalLink(row *signalLinkRow) SignalLink {
	return SignalLink{
		ID:                 row.ID,
		CaseID:             row.CaseID,
		SignalOccurrenceID: row.SignalOccurrenceID,
		Relation:           SignalRelation(row.Relation),
		SignalID:           row.SignalID,
		Producer:           row.Producer,
		ObservedAt:         row.ObservedAt,
		Coverage:           row.Coverage,
		Freshness:          row.Freshness,
		WindowStart:        row.WindowStart,
		WindowEnd:          row.WindowEnd,
		CreatedAt:          row.CreatedAt,
	}
}

func resourceLinkToRow(l *ResourceLink) resourceLinkRow {
	return resourceLinkRow{
		ID:           l.ID,
		CaseID:       l.CaseID,
		Kind:         l.Resource.Kind,
		Namespace:    l.Resource.Namespace,
		Name:         l.Resource.Name,
		UID:          l.Resource.UID,
		Incomplete:   l.Resource.Incomplete,
		Relation:     string(l.Relation),
		TopologyPath: mustMarshalJSONB(l.TopologyPath),
		EdgeIDs:      mustMarshalJSONB(l.EdgeIDs),
		CreatedAt:    l.CreatedAt,
	}
}

func rowToResourceLink(row *resourceLinkRow) ResourceLink {
	return ResourceLink{
		ID:     row.ID,
		CaseID: row.CaseID,
		Resource: ResourceCitation{
			Kind:       row.Kind,
			Namespace:  row.Namespace,
			Name:       row.Name,
			UID:        row.UID,
			Incomplete: row.Incomplete,
		},
		Relation:     ResourceRelation(row.Relation),
		TopologyPath: unmarshalStrings(row.TopologyPath),
		EdgeIDs:      unmarshalInt64s(row.EdgeIDs),
		CreatedAt:    row.CreatedAt,
	}
}

func changeCandidateToRow(c *ChangeCandidate) changeCandidateRow {
	return changeCandidateRow{
		ID:                c.ID,
		CaseID:            c.CaseID,
		ChangeEventID:     c.ChangeEventID,
		RuleID:            c.RuleID,
		Confidence:        string(c.Confidence),
		Rank:              c.Rank,
		Factors:           mustMarshalJSONB(c.Factors),
		EvidenceRefs:      mustMarshalJSONB(c.EvidenceRefs),
		ContradictingRefs: mustMarshalJSONB(c.ContradictingRefs),
		ReasonCode:        c.ReasonCode,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
}

func rowToChangeCandidate(row *changeCandidateRow) ChangeCandidate {
	return ChangeCandidate{
		ID:                row.ID,
		CaseID:            row.CaseID,
		ChangeEventID:     row.ChangeEventID,
		RuleID:            row.RuleID,
		Confidence:        ConfidenceClass(row.Confidence),
		Rank:              row.Rank,
		Factors:           unmarshalFactors(row.Factors),
		EvidenceRefs:      unmarshalEvidenceRefs(row.EvidenceRefs),
		ContradictingRefs: unmarshalEvidenceRefs(row.ContradictingRefs),
		ReasonCode:        row.ReasonCode,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

// --- JSON helpers ---

func mustMarshalJSONB(v interface{}) JSONB {
	b, err := json.Marshal(v)
	if err != nil {
		return JSONB("[]")
	}
	return JSONB(b)
}

func unmarshalFactors(j JSONB) []Factor {
	if len(j) == 0 {
		return nil
	}
	var out []Factor
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalEvidenceRefs(j JSONB) []EvidenceRef {
	if len(j) == 0 {
		return nil
	}
	var out []EvidenceRef
	if err := json.Unmarshal([]byte(j), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalInt64s(j JSONB) []int64 {
	if len(j) == 0 {
		return nil
	}
	var out []int64
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

// mergeFactorsJSON merges two JSONB factor arrays, keeping the highest weight
// per factor kind. Used during case upsert.
func mergeFactorsJSON(existing, new JSONB) JSONB {
	existingFactors := unmarshalFactors(existing)
	newFactors := unmarshalFactors(new)
	merged := mergeFactors(existingFactors, newFactors)
	return mustMarshalJSONB(merged)
}

// rankCompleteness ranks completeness so higher = more complete.
func rankCompleteness(c EvidenceCompleteness) int {
	switch c {
	case CompletenessComplete:
		return 2
	case CompletenessPartial:
		return 1
	default:
		return 0
	}
}
