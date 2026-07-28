package globalsearch

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const savedFilterLockBase int64 = 741027000000

type SavedFilterGormRepository struct{ db *gorm.DB }

func NewSavedFilterGormRepository(db *gorm.DB) *SavedFilterGormRepository {
	return &SavedFilterGormRepository{db: db}
}

type savedFilterRecord struct {
	ID            int64
	UserID        int64
	Name          string
	QueryText     string
	Namespace     string
	Kinds         string
	SchemaVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r *SavedFilterGormRepository) ListSavedFilters(ctx context.Context, userID int64) ([]SavedFilter, error) {
	var records []savedFilterRecord
	if err := r.db.WithContext(ctx).Raw(`SELECT id, user_id, name, query_text, namespace, kinds,
		schema_version, created_at, updated_at FROM saved_global_search_filters
		WHERE user_id = ? ORDER BY created_at ASC, id ASC`, userID).Scan(&records).Error; err != nil {
		return nil, err
	}
	return decodeSavedFilterRecords(records), nil
}

func (r *SavedFilterGormRepository) CreateSavedFilter(ctx context.Context, item SavedFilter, limit int) (SavedFilter, error) {
	var created SavedFilter
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, savedFilterLockBase+item.UserID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Raw(`SELECT COUNT(*) FROM saved_global_search_filters WHERE user_id = ?`, item.UserID).Row().Scan(&count); err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrSavedFilterLimit
		}
		record, err := scanSavedFilter(tx.Raw(`INSERT INTO saved_global_search_filters
			(user_id, name, query_text, namespace, kinds, schema_version)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (user_id, (LOWER(name))) DO NOTHING
			RETURNING id, user_id, name, query_text, namespace, kinds, schema_version, created_at, updated_at`,
			item.UserID, item.Name, item.Query, item.Namespace, encodeKinds(item.Kinds), item.SchemaVersion).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSavedFilterNameExists
		}
		if err != nil {
			return err
		}
		created = decodeSavedFilterRecord(record)
		return nil
	})
	return created, err
}

func (r *SavedFilterGormRepository) UpdateSavedFilter(ctx context.Context, userID, id int64, changes SavedFilterChanges) (SavedFilter, error) {
	var updated SavedFilter
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, savedFilterLockBase+userID).Error; err != nil {
			return err
		}
		current, err := scanSavedFilter(tx.Raw(`SELECT id, user_id, name, query_text, namespace, kinds,
			schema_version, created_at, updated_at FROM saved_global_search_filters
			WHERE id = ? AND user_id = ? FOR UPDATE`, id, userID).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSavedFilterNotFound
		}
		if err != nil {
			return err
		}
		if changes.Name != nil && !strings.EqualFold(current.Name, *changes.Name) {
			var exists bool
			if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM saved_global_search_filters
				WHERE user_id = ? AND LOWER(name) = LOWER(?) AND id <> ?)`, userID, *changes.Name, id).Row().Scan(&exists); err != nil {
				return err
			}
			if exists {
				return ErrSavedFilterNameExists
			}
		}
		sets, args := make([]string, 0, 6), make([]any, 0, 8)
		if changes.Name != nil {
			sets, args = append(sets, "name = ?"), append(args, *changes.Name)
		}
		if changes.Query != nil {
			sets, args = append(sets, "query_text = ?"), append(args, *changes.Query)
		}
		if changes.Namespace != nil {
			sets, args = append(sets, "namespace = ?"), append(args, *changes.Namespace)
		}
		if changes.Kinds != nil {
			sets, args = append(sets, "kinds = ?"), append(args, encodeKinds(*changes.Kinds))
		}
		if changes.SchemaVersion != nil {
			sets, args = append(sets, "schema_version = ?"), append(args, *changes.SchemaVersion)
		}
		sets = append(sets, "updated_at = NOW()")
		args = append(args, id, userID)
		record, err := scanSavedFilter(tx.Raw(`UPDATE saved_global_search_filters SET `+strings.Join(sets, ", ")+`
			WHERE id = ? AND user_id = ? RETURNING id, user_id, name, query_text, namespace,
			kinds, schema_version, created_at, updated_at`, args...).Row())
		if err != nil {
			return err
		}
		updated = decodeSavedFilterRecord(record)
		return nil
	})
	return updated, err
}

func (r *SavedFilterGormRepository) DeleteSavedFilter(ctx context.Context, userID, id int64) error {
	result := r.db.WithContext(ctx).Exec(`DELETE FROM saved_global_search_filters WHERE id = ? AND user_id = ?`, id, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSavedFilterNotFound
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanSavedFilter(row rowScanner) (savedFilterRecord, error) {
	var record savedFilterRecord
	err := row.Scan(&record.ID, &record.UserID, &record.Name, &record.QueryText, &record.Namespace,
		&record.Kinds, &record.SchemaVersion, &record.CreatedAt, &record.UpdatedAt)
	return record, err
}

func decodeSavedFilterRecords(records []savedFilterRecord) []SavedFilter {
	items := make([]SavedFilter, 0, len(records))
	for _, record := range records {
		items = append(items, decodeSavedFilterRecord(record))
	}
	return items
}

func decodeSavedFilterRecord(record savedFilterRecord) SavedFilter {
	rawKinds := strings.Split(record.Kinds, ",")
	kinds := make([]Kind, 0, len(rawKinds))
	for _, value := range rawKinds {
		if value != "" {
			kinds = append(kinds, Kind(value))
		}
	}
	return SavedFilter{ID: record.ID, UserID: record.UserID, Name: record.Name, Query: record.QueryText,
		Namespace: record.Namespace, Kinds: kinds, SchemaVersion: record.SchemaVersion,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func encodeKinds(kinds []Kind) string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return strings.Join(values, ",")
}

var _ SavedFilterRepository = (*SavedFilterGormRepository)(nil)
