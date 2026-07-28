package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	Save(context.Context, *Entry) error
	List(context.Context, Filter) (ListResponse, error)
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, entry *Entry) error {
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	var actorID any
	if entry.Actor.ID > 0 {
		actorID = entry.Actor.ID
	}
	var clusterID any
	if entry.ClusterID != nil && *entry.ClusterID > 0 {
		clusterID = *entry.ClusterID
	}
	row := r.db.WithContext(ctx).Raw(`INSERT INTO audit_logs
		(actor_user_id, actor_name, cluster_id, action, resource_type, resource_namespace, resource_name, result, request_id, status_code, ip_address, user_agent, details)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSONB)) RETURNING id, created_at`,
		actorID, entry.Actor.Name, clusterID, entry.Action, entry.Resource.Type, entry.Resource.Namespace, entry.Resource.Name,
		entry.Result, entry.RequestID, entry.StatusCode, entry.IPAddress, entry.UserAgent, string(details)).Row()
	if err := row.Scan(&entry.ID, &entry.CreatedAt); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

type storedEntry struct {
	ID                int64
	ActorUserID       sql.NullInt64
	ActorName         string
	ClusterID         sql.NullInt64
	Action            string
	ResourceType      string
	ResourceNamespace string
	ResourceName      string
	Result            string
	RequestID         string
	StatusCode        int
	IPAddress         string
	UserAgent         string
	Details           string
	CreatedAt         time.Time
}

func (r *GormRepository) List(ctx context.Context, filter Filter) (ListResponse, error) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if filter.ClusterID > 0 {
		conditions = append(conditions, "cluster_id = ?")
		args = append(args, filter.ClusterID)
	}
	if filter.Action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Result != "" {
		conditions = append(conditions, "result = ?")
		args = append(args, filter.Result)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM audit_logs"+where, args...).Row().Scan(&total); err != nil {
		return ListResponse{}, err
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit)
	var stored []storedEntry
	if err := r.db.WithContext(ctx).Raw(`SELECT id, actor_user_id, actor_name, cluster_id, action,
		COALESCE(resource_type, '') AS resource_type, COALESCE(resource_namespace, '') AS resource_namespace,
		COALESCE(resource_name, '') AS resource_name, result, request_id, status_code, ip_address, user_agent,
		details::text AS details, created_at FROM audit_logs`+where+` ORDER BY created_at DESC, id DESC LIMIT ?`, queryArgs...).Scan(&stored).Error; err != nil {
		return ListResponse{}, err
	}
	items := make([]Entry, 0, len(stored))
	for _, item := range stored {
		entry := Entry{ID: item.ID, Actor: Actor{ID: item.ActorUserID.Int64, Name: item.ActorName}, Action: item.Action,
			Resource: ResourceRef{Type: item.ResourceType, Namespace: item.ResourceNamespace, Name: item.ResourceName}, Result: item.Result,
			RequestID: item.RequestID, StatusCode: item.StatusCode, IPAddress: item.IPAddress, UserAgent: item.UserAgent, CreatedAt: item.CreatedAt}
		if item.ClusterID.Valid {
			value := item.ClusterID.Int64
			entry.ClusterID = &value
		}
		if err := json.Unmarshal([]byte(item.Details), &entry.Details); err != nil {
			return ListResponse{}, fmt.Errorf("decode audit details: %w", err)
		}
		items = append(items, entry)
	}
	remaining := total - int64(len(items))
	if remaining < 0 {
		remaining = 0
	}
	return ListResponse{Items: items, Total: total, Remaining: remaining}, nil
}
