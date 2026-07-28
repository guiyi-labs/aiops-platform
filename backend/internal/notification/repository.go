package notification

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	SetEnabled(context.Context, bool) error
	Claim(context.Context, int, time.Time) ([]Delivery, error)
	MarkDelivered(context.Context, int64, time.Time) error
	MarkFailed(context.Context, int64, int, time.Time, string) error
	List(context.Context, ListFilter) (ListResponse, error)
	Retry(context.Context, int64) error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) SetEnabled(ctx context.Context, enabled bool) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO notification_settings (id, enabled, updated_at)
		VALUES (1, ?, NOW()) ON CONFLICT (id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()`, enabled).Error
}

type storedDelivery struct {
	ID            int64
	DiagnosisID   int64
	EventType     string
	Payload       string
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	DeliveredAt   sql.NullTime
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r *GormRepository) Claim(ctx context.Context, limit int, staleBefore time.Time) ([]Delivery, error) {
	var stored []storedDelivery
	err := r.db.WithContext(ctx).Raw(`WITH candidates AS (
		SELECT id FROM notification_deliveries
		WHERE (status = 'pending' AND next_attempt_at <= NOW())
		   OR (status = 'delivering' AND locked_at < ?)
		ORDER BY next_attempt_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT ?
	)
	UPDATE notification_deliveries AS delivery
	SET status = 'delivering', locked_at = NOW(), updated_at = NOW()
	FROM candidates
	WHERE delivery.id = candidates.id
	RETURNING delivery.id, delivery.diagnosis_id, delivery.event_type,
		delivery.payload::text AS payload, delivery.status, delivery.attempts,
		delivery.next_attempt_at, delivery.delivered_at, delivery.last_error,
		delivery.created_at, delivery.updated_at`, staleBefore, limit).Scan(&stored).Error
	if err != nil {
		return nil, err
	}
	return decodeDeliveries(stored), nil
}

func (r *GormRepository) MarkDelivered(ctx context.Context, id int64, deliveredAt time.Time) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notification_deliveries
		SET status = 'delivered', attempts = attempts + 1, delivered_at = ?, locked_at = NULL,
			last_error = '', updated_at = NOW() WHERE id = ?`, deliveredAt, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *GormRepository) MarkFailed(ctx context.Context, id int64, maxAttempts int, nextAttempt time.Time, message string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notification_deliveries
		SET attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= ? THEN 'dead' ELSE 'pending' END,
			next_attempt_at = ?, locked_at = NULL, delivered_at = NULL,
			last_error = ?, updated_at = NOW() WHERE id = ?`, maxAttempts, nextAttempt, message, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *GormRepository) List(ctx context.Context, filter ListFilter) (ListResponse, error) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if filter.DiagnosisID > 0 {
		conditions = append(conditions, "diagnosis_id = ?")
		args = append(args, filter.DiagnosisID)
	}
	if filter.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, filter.EventType)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM notification_deliveries"+where, args...).Row().Scan(&total); err != nil {
		return ListResponse{}, err
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit)
	var stored []storedDelivery
	if err := r.db.WithContext(ctx).Raw(`SELECT id, diagnosis_id, event_type, payload::text AS payload,
		status, attempts, next_attempt_at, delivered_at, last_error, created_at, updated_at
		FROM notification_deliveries`+where+` ORDER BY created_at DESC, id DESC LIMIT ?`, queryArgs...).Scan(&stored).Error; err != nil {
		return ListResponse{}, err
	}
	items := decodeDeliveries(stored)
	remaining := total - int64(len(items))
	if remaining < 0 {
		remaining = 0
	}
	return ListResponse{Items: items, Total: total, Remaining: remaining}, nil
}

func (r *GormRepository) Retry(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notification_deliveries SET status = 'pending', attempts = 0,
		next_attempt_at = NOW(), locked_at = NULL, delivered_at = NULL, last_error = '', updated_at = NOW()
		WHERE id = ? AND status = 'dead'`, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var exists bool
	if err := r.db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM notification_deliveries WHERE id = ?)`, id).Row().Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrDeliveryNotFound
	}
	return ErrDeliveryNotRetryable
}

func decodeDeliveries(stored []storedDelivery) []Delivery {
	items := make([]Delivery, 0, len(stored))
	for _, item := range stored {
		delivery := Delivery{ID: item.ID, DiagnosisID: item.DiagnosisID, EventType: item.EventType,
			Status: item.Status, Attempts: item.Attempts, NextAttemptAt: item.NextAttemptAt,
			LastError: item.LastError, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
			Payload: []byte(item.Payload)}
		if item.DeliveredAt.Valid {
			value := item.DeliveredAt.Time
			delivery.DeliveredAt = &value
		}
		items = append(items, delivery)
	}
	return items
}

var _ Repository = (*GormRepository)(nil)
