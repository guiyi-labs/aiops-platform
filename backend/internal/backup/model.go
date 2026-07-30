package backup

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
)

const (
	ActionCreateBackup         = "backup.create"
	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusExpired              = "expired"

	DefaultTTL             = "720h"
	DefaultBackupNamespace = "velero"
)

var (
	ErrNotFound                   = errors.New("backup plan not found")
	ErrVeleroNotInstalled         = errors.New("Velero is not installed on the target cluster")
	ErrStorageLocationNotFound    = errors.New("backup storage location not found")
	ErrStorageLocationUnavailable = errors.New("backup storage location is not Available")
	ErrSourceNamespaceNotFound    = errors.New("source namespace not found")
	ErrStaleSourceNamespace       = errors.New("source namespace changed since preview")
	ErrInvalidRequest             = errors.New("backup request parameters are invalid")
	ErrBackupNameConflict         = errors.New("backup name already exists on the target cluster")
	ErrConfirmationInvalid        = errors.New("backup confirmation is invalid")
	ErrInvalidIdempotency         = errors.New("backup idempotency key is invalid")
	ErrExpired                    = errors.New("backup plan expired")
	ErrInProgress                 = errors.New("backup execution is in progress")
	ErrAlreadyExecuted            = errors.New("backup plan already used with another idempotency key")
	ErrExecutionFailed            = errors.New("backup execution failed")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// LabelSelectorMap is a map[string]string stored as JSONB.
type LabelSelectorMap map[string]string

func (m LabelSelectorMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func (m *LabelSelectorMap) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	}
	return errors.New("invalid label_selector type")
}

// Plan represents a controlled backup creation plan persisted in backup_plans.
type Plan struct {
	ID                             string           `gorm:"primaryKey;size:36" json:"id"`
	ClusterID                      int64            `json:"cluster_id"`
	Status                         string           `json:"status"`
	BackupName                     string           `gorm:"column:backup_name" json:"backup_name"`
	BackupNamespace                string           `gorm:"column:backup_namespace" json:"backup_namespace"`
	IncludedNamespaces             StringArray      `gorm:"column:included_namespaces;type:text[]" json:"included_namespaces"`
	SourceNamespaceUID             string           `gorm:"column:source_namespace_uid" json:"source_namespace_uid"`
	SourceNamespaceResourceVersion string           `gorm:"column:source_namespace_resource_version" json:"source_namespace_resource_version"`
	StorageLocation                string           `gorm:"column:storage_location" json:"storage_location"`
	TTL                            string           `gorm:"column:ttl" json:"ttl"`
	IncludeClusterResources        bool             `gorm:"column:include_cluster_resources" json:"include_cluster_resources"`
	SnapshotVolumes                bool             `gorm:"column:snapshot_volumes" json:"snapshot_volumes"`
	LabelSelector                  LabelSelectorMap `gorm:"column:label_selector;type:jsonb" json:"label_selector"`
	VeleroVersion                  string           `gorm:"column:velero_version" json:"velero_version"`
	BackupUID                      string           `gorm:"column:backup_uid" json:"backup_uid,omitempty"`
	BackupResourceVersion          string           `gorm:"column:backup_resource_version" json:"backup_resource_version,omitempty"`
	ConfirmationTokenHash          []byte           `gorm:"column:confirmation_token_hash" json:"-"`
	RequestedByUserID              *int64           `gorm:"column:requested_by_user_id" json:"-"`
	RequestedByName                string           `gorm:"column:requested_by_name" json:"-"`
	ExpiresAt                      time.Time        `json:"expires_at"`
	IdempotencyKey                 string           `gorm:"column:idempotency_key" json:"-"`
	LockedAt                       *time.Time       `gorm:"column:locked_at" json:"-"`
	ExecutedAt                     *time.Time       `json:"executed_at,omitempty"`
	LastError                      string           `json:"last_error,omitempty"`
	CreatedAt                      time.Time        `json:"created_at"`
	UpdatedAt                      time.Time        `json:"updated_at"`
	ConfirmationToken              string           `gorm:"-" json:"confirmation_token,omitempty"`
}

func (Plan) TableName() string { return "backup_plans" }

func (p Plan) RequestedBy() ActorRef {
	id := int64(0)
	if p.RequestedByUserID != nil {
		id = *p.RequestedByUserID
	}
	return ActorRef{ID: id, Name: p.RequestedByName}
}

// StringArray is a []string stored as Postgres text[].
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	return pq.StringArray(a).Value()
}

func (a *StringArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	return (*pq.StringArray)(a).Scan(value)
}

// PlanResponse is the HTTP-facing projection of a Plan.
type PlanResponse struct {
	Plan
	RequestedBy ActorRef   `json:"requested_by"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	IncludedNamespaces      []string          `json:"included_namespaces"`
	StorageLocation         string            `json:"storage_location"`
	TTL                     string            `json:"ttl"`
	IncludeClusterResources bool              `json:"include_cluster_resources"`
	SnapshotVolumes         bool              `json:"snapshot_volumes"`
	LabelSelector           map[string]string `json:"label_selector,omitempty"`
}

// Request is the input DTO for creating a backup plan preview.
type Request struct {
	SourceNamespace string
	StorageLocation string
	TTL             string
}

func Response(plan Plan) PlanResponse {
	resp := PlanResponse{Plan: plan, RequestedBy: plan.RequestedBy()}
	resp.Parameters = Parameters{
		IncludedNamespaces:      plan.IncludedNamespaces,
		StorageLocation:         plan.StorageLocation,
		TTL:                     plan.TTL,
		IncludeClusterResources: plan.IncludeClusterResources,
		SnapshotVolumes:         plan.SnapshotVolumes,
		LabelSelector:           plan.LabelSelector,
	}
	return resp
}
