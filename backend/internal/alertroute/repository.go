package alertroute

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository abstracts persistence for the alertroute package.
type Repository interface {
	// Receivers
	CreateReceiver(ctx context.Context, receiver *Receiver) error
	GetReceiver(ctx context.Context, id, creatorID int64) (Receiver, error)
	GetReceiverByID(ctx context.Context, id int64) (Receiver, error) // for internal dispatch, no creator scope
	ListReceivers(ctx context.Context, creatorID int64) ([]Receiver, error)
	DeleteReceiver(ctx context.Context, id, creatorID int64) error

	// Routes
	CreateRoute(ctx context.Context, route *Route) error
	GetRoute(ctx context.Context, id, creatorID int64) (Route, error)
	ListRoutes(ctx context.Context, creatorID int64) ([]Route, error)
	UpdateRoute(ctx context.Context, id, creatorID int64, input PatchRouteInput) (Route, error)
	DeleteRoute(ctx context.Context, id, creatorID int64) error
	ListEnabledRoutes(ctx context.Context) ([]Route, error) // for matching

	// Silences
	CreateSilence(ctx context.Context, silence *Silence) error
	ListSilences(ctx context.Context, filter SilenceListFilter) ([]Silence, error)
	DeleteSilence(ctx context.Context, id, creatorID int64) error
	ListActiveSilences(ctx context.Context, now time.Time) ([]Silence, error)

	// Inhibits (M51)
	CreateInhibit(ctx context.Context, inhibit *Inhibit) error
	ListInhibits(ctx context.Context, filter InhibitListFilter) ([]Inhibit, error)
	DeleteInhibit(ctx context.Context, id, creatorID int64) error
	ListEnabledInhibits(ctx context.Context) ([]Inhibit, error)
	// HasFiringSource reports whether any non-resolved delivery exists for the
	// source match within the active window. Used by IsInhibited to decide
	// whether the source is currently firing.
	HasFiringSource(ctx context.Context, source MatchAlert, activeWindow time.Duration) (bool, error)

	// Deliveries
	CreateDelivery(ctx context.Context, delivery *Delivery) error
	FindActiveDelivery(ctx context.Context, routeID int64, dedupeKey, eventType string) (*Delivery, error)
	ClaimDeliveries(ctx context.Context, batchSize int, now time.Time) ([]Delivery, error)
	MarkDelivered(ctx context.Context, id int64, deliveredAt time.Time) error
	MarkFailed(ctx context.Context, id int64, maxAttempts int, nextAttempt time.Time, message string) error
	ListDeliveries(ctx context.Context, filter DeliveryListFilter) (ListResponse[Delivery], error)
}

// PatchRouteInput holds optional route updates.
type PatchRouteInput struct {
	Priority       *int
	Enabled        *bool
	GroupInterval  *time.Duration
	RepeatInterval *time.Duration
}

// SilenceListFilter filters silences.
type SilenceListFilter struct {
	CreatorID *int64
	ClusterID *int64
	Active    *bool
}

// InhibitListFilter filters inhibits (M51).
type InhibitListFilter struct {
	CreatorID *int64
	Enabled   *bool
}

// DeliveryListFilter filters deliveries.
type DeliveryListFilter struct {
	ReceiverID *int64
	Status     string
	Limit      int
	Offset     int
}

// ListResponse is a generic paginated list response.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

// NewGormRepository returns a GORM-backed repository.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// --- Receivers ---

func (r *GormRepository) CreateReceiver(ctx context.Context, receiver *Receiver) error {
	return r.db.WithContext(ctx).Create(receiver).Error
}

func (r *GormRepository) GetReceiver(ctx context.Context, id, creatorID int64) (Receiver, error) {
	var receiver Receiver
	err := r.db.WithContext(ctx).
		Where("id = ? AND creator_id = ?", id, creatorID).
		First(&receiver).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Receiver{}, ErrReceiverNotFound
		}
		return Receiver{}, err
	}
	return receiver, nil
}

func (r *GormRepository) GetReceiverByID(ctx context.Context, id int64) (Receiver, error) {
	var receiver Receiver
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&receiver).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Receiver{}, ErrReceiverNotFound
		}
		return Receiver{}, err
	}
	return receiver, nil
}

func (r *GormRepository) ListReceivers(ctx context.Context, creatorID int64) ([]Receiver, error) {
	var receivers []Receiver
	if err := r.db.WithContext(ctx).
		Where("creator_id = ?", creatorID).
		Order("id ASC").
		Find(&receivers).Error; err != nil {
		return nil, err
	}
	return receivers, nil
}

func (r *GormRepository) DeleteReceiver(ctx context.Context, id, creatorID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var routeCount int64
		if err := tx.Model(&Route{}).
			Where("receiver_id = ? AND creator_id = ?", id, creatorID).
			Count(&routeCount).Error; err != nil {
			return err
		}
		if routeCount > 0 {
			return ErrReceiverInUse
		}
		result := tx.Where("id = ? AND creator_id = ?", id, creatorID).
			Delete(&Receiver{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrReceiverNotFound
		}
		return nil
	})
}

// --- Routes ---

func (r *GormRepository) CreateRoute(ctx context.Context, route *Route) error {
	return r.db.WithContext(ctx).Create(route).Error
}

func (r *GormRepository) GetRoute(ctx context.Context, id, creatorID int64) (Route, error) {
	var route Route
	err := r.db.WithContext(ctx).
		Where("id = ? AND creator_id = ?", id, creatorID).
		First(&route).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}
	return route, nil
}

func (r *GormRepository) ListRoutes(ctx context.Context, creatorID int64) ([]Route, error) {
	var routes []Route
	if err := r.db.WithContext(ctx).
		Where("creator_id = ?", creatorID).
		Order("priority ASC, id ASC").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

func (r *GormRepository) UpdateRoute(ctx context.Context, id, creatorID int64, input PatchRouteInput) (Route, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{}
		if input.Priority != nil {
			updates["priority"] = *input.Priority
		}
		if input.Enabled != nil {
			updates["enabled"] = *input.Enabled
		}
		if input.GroupInterval != nil {
			updates["group_interval"] = int64(*input.GroupInterval)
		}
		if input.RepeatInterval != nil {
			updates["repeat_interval"] = int64(*input.RepeatInterval)
		}
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = gorm.Expr("NOW()")
		result := tx.Model(&Route{}).
			Where("id = ? AND creator_id = ?", id, creatorID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrRouteNotFound
		}
		return nil
	})
	if err != nil {
		return Route{}, err
	}
	return r.GetRoute(ctx, id, creatorID)
}

func (r *GormRepository) DeleteRoute(ctx context.Context, id, creatorID int64) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND creator_id = ?", id, creatorID).
		Delete(&Route{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRouteNotFound
	}
	return nil
}

func (r *GormRepository) ListEnabledRoutes(ctx context.Context) ([]Route, error) {
	var routes []Route
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

// --- Silences ---

func (r *GormRepository) CreateSilence(ctx context.Context, silence *Silence) error {
	return r.db.WithContext(ctx).Create(silence).Error
}

func (r *GormRepository) ListSilences(ctx context.Context, filter SilenceListFilter) ([]Silence, error) {
	query := r.db.WithContext(ctx).Model(&Silence{})
	if filter.CreatorID != nil {
		query = query.Where("creator_id = ?", *filter.CreatorID)
	}
	if filter.ClusterID != nil {
		query = query.Where("cluster_id = ?", *filter.ClusterID)
	}
	if filter.Active != nil {
		now := time.Now().UTC()
		if *filter.Active {
			query = query.Where("starts_at <= ? AND ends_at > ?", now, now)
		} else {
			query = query.Where("ends_at <= ?", now)
		}
	}
	var silences []Silence
	if err := query.Order("ends_at ASC, id ASC").Find(&silences).Error; err != nil {
		return nil, err
	}
	return silences, nil
}

func (r *GormRepository) DeleteSilence(ctx context.Context, id, creatorID int64) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND creator_id = ?", id, creatorID).
		Delete(&Silence{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSilenceNotFound
	}
	return nil
}

func (r *GormRepository) ListActiveSilences(ctx context.Context, now time.Time) ([]Silence, error) {
	var silences []Silence
	if err := r.db.WithContext(ctx).
		Where("starts_at <= ? AND ends_at > ?", now, now).
		Order("id ASC").
		Find(&silences).Error; err != nil {
		return nil, err
	}
	return silences, nil
}

// --- Inhibits (M51) ---

func (r *GormRepository) CreateInhibit(ctx context.Context, inhibit *Inhibit) error {
	return r.db.WithContext(ctx).Create(inhibit).Error
}

func (r *GormRepository) ListInhibits(ctx context.Context, filter InhibitListFilter) ([]Inhibit, error) {
	query := r.db.WithContext(ctx).Model(&Inhibit{})
	if filter.CreatorID != nil {
		query = query.Where("creator_id = ?", *filter.CreatorID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var inhibits []Inhibit
	if err := query.Order("id ASC").Find(&inhibits).Error; err != nil {
		return nil, err
	}
	return inhibits, nil
}

func (r *GormRepository) DeleteInhibit(ctx context.Context, id, creatorID int64) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND creator_id = ?", id, creatorID).
		Delete(&Inhibit{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInhibitNotFound
	}
	return nil
}

func (r *GormRepository) ListEnabledInhibits(ctx context.Context) ([]Inhibit, error) {
	var inhibits []Inhibit
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&inhibits).Error; err != nil {
		return nil, err
	}
	return inhibits, nil
}

// HasFiringSource reports whether any non-resolved (pending/delivering)
// delivery exists for the source match within the active window. The source
// alert is considered firing while at least one such delivery exists.
func (r *GormRepository) HasFiringSource(ctx context.Context, source MatchAlert, activeWindow time.Duration) (bool, error) {
	now := time.Now().UTC()
	since := now.Add(-activeWindow)
	query := r.db.WithContext(ctx).Model(&Delivery{}).
		Where("event_type = ?", EventTypeFiring).
		Where("status IN ?", []string{DeliveryStatusPending, DeliveryStatusDelivering}).
		Where("updated_at >= ?", since)
	if source.ClusterID != 0 {
		query = query.Where("cluster_id = ?", source.ClusterID)
	}
	if source.RuleName != "" {
		query = query.Where("rule_name = ?", source.RuleName)
	}
	if source.Severity != "" {
		query = query.Where("severity = ?", source.Severity)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- Deliveries ---

func (r *GormRepository) CreateDelivery(ctx context.Context, delivery *Delivery) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(delivery).Error
}

func (r *GormRepository) FindActiveDelivery(ctx context.Context, routeID int64, dedupeKey, eventType string) (*Delivery, error) {
	var delivery Delivery
	err := r.db.WithContext(ctx).
		Where("route_id = ? AND dedupe_key = ? AND event_type = ?", routeID, dedupeKey, eventType).
		Order("id DESC").
		First(&delivery).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &delivery, nil
}

func (r *GormRepository) ClaimDeliveries(ctx context.Context, batchSize int, now time.Time) ([]Delivery, error) {
	var stored []Delivery
	err := r.db.WithContext(ctx).Raw(`WITH candidates AS (
		SELECT id FROM alert_route_deliveries
		WHERE (status = ? AND next_attempt_at <= ?)
		   OR (status = ? AND next_attempt_at <= ?)
		ORDER BY next_attempt_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT ?
	)
	UPDATE alert_route_deliveries AS delivery
	SET status = ?, locked_at = ?, next_attempt_at = ?, updated_at = ?
	FROM candidates
	WHERE delivery.id = candidates.id
	RETURNING delivery.id, delivery.route_id, delivery.receiver_id, delivery.alert_instance_id,
		delivery.cluster_id, delivery.rule_name, delivery.severity,
		delivery.event_type, delivery.dedupe_key, delivery.status, delivery.attempts,
		delivery.next_attempt_at, delivery.delivered_at, delivery.last_error,
		delivery.locked_at, delivery.created_at, delivery.updated_at`,
		DeliveryStatusPending, now,
		DeliveryStatusDelivering, now,
		batchSize,
		DeliveryStatusDelivering, now, now.Add(2*time.Minute), now).Scan(&stored).Error
	if err != nil {
		return nil, err
	}
	return stored, nil
}

func (r *GormRepository) MarkDelivered(ctx context.Context, id int64, deliveredAt time.Time) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_route_deliveries
		SET status = ?, attempts = attempts + 1, delivered_at = ?, last_error = '', locked_at = NULL, updated_at = ?
		WHERE id = ?`, DeliveryStatusDelivered, deliveredAt, deliveredAt, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, maxAttempts int, nextAttempt time.Time, message string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE alert_route_deliveries
		SET attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= ? THEN ? ELSE ? END,
			next_attempt_at = ?, delivered_at = NULL, last_error = ?, locked_at = NULL, updated_at = ?
		WHERE id = ?`, maxAttempts, DeliveryStatusDead, DeliveryStatusPending, nextAttempt, message, nextAttempt, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *GormRepository) ListDeliveries(ctx context.Context, filter DeliveryListFilter) (ListResponse[Delivery], error) {
	query := r.db.WithContext(ctx).Model(&Delivery{})
	if filter.ReceiverID != nil {
		query = query.Where("receiver_id = ?", *filter.ReceiverID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ListResponse[Delivery]{}, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	var items []Delivery
	if err := query.Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&items).Error; err != nil {
		return ListResponse[Delivery]{}, err
	}
	return ListResponse[Delivery]{Items: items, Total: int(total)}, nil
}

// Compile-time assertion that GormRepository implements Repository.
var _ Repository = (*GormRepository)(nil)
