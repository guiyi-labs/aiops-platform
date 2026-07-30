package promotion

import (
	"errors"
	"time"
)

const (
	KindDeployment = "Deployment"
	KindService    = "Service"
	KindIngress    = "Ingress"
	KindConfigMap  = "ConfigMap"
	KindSecret     = "Secret"

	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusPartial              = "partial"
	StatusExpired              = "expired"

	ItemStatusPending = "pending"
	ItemStatusApplied = "applied"
	ItemStatusFailed  = "failed"
	ItemStatusSkipped = "skipped"

	ModeCreate = "create"
)

var (
	ErrNotFound               = errors.New("promotion plan not found")
	ErrInvalidRequest         = errors.New("promotion request is invalid")
	ErrBundleEmpty            = errors.New("promotion bundle must include at least one resource")
	ErrSourceUnavailable      = errors.New("promotion source cluster is unavailable")
	ErrDestinationUnavailable = errors.New("promotion destination cluster is unavailable")
	ErrNamespaceMissing       = errors.New("promotion destination namespace does not exist")
	ErrDependencyUnresolved   = errors.New("promotion dependency is unresolved on the destination cluster")
	ErrConflict               = errors.New("promotion destination resource already exists")
	ErrPreviewFailed          = errors.New("promotion dry-run preview failed")
	ErrConfirmationInvalid    = errors.New("promotion confirmation is invalid")
	ErrInvalidIdempotency     = errors.New("promotion idempotency key is invalid")
	ErrExpired                = errors.New("promotion plan expired")
	ErrInProgress             = errors.New("promotion execution is in progress")
	ErrAlreadyExecuted        = errors.New("promotion plan already used with another idempotency key")
	ErrExecutionFailed        = errors.New("promotion execution failed")
)

// ActorRef mirrors remediation.ActorRef so callers can pass through the
// authenticated operator without importing the remediation package.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// BundleItemRequest selects one source resource to promote.
type BundleItemRequest struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// DependencyMapping maps a referenced ConfigMap/Secret from a source name to a
// destination name. The platform never copies referenced values; it only
// verifies the destination object exists by the mapped name.
type DependencyMapping struct {
	Kind                 string `json:"kind"`
	SourceNamespace      string `json:"source_namespace"`
	SourceName           string `json:"source_name"`
	DestinationNamespace string `json:"destination_namespace"`
	DestinationName      string `json:"destination_name"`
}

// PreviewRequest is the internal DTO consumed by the service.
type PreviewRequest struct {
	SourceClusterID      int64
	DestinationClusterID int64
	SourceNamespace      string
	DestinationNamespace string
	Bundle               []BundleItemRequest
	DependencyMappings   []DependencyMapping
}

// Plan is the persisted promotion plan (table promotion_plans).
type Plan struct {
	ID                    string             `gorm:"primaryKey;size:36" json:"id"`
	SourceClusterID       int64              `json:"source_cluster_id"`
	DestinationClusterID  int64              `json:"destination_cluster_id"`
	SourceNamespace       string             `json:"source_namespace"`
	DestinationNamespace  string             `json:"destination_namespace"`
	Status                string             `json:"status"`
	BundleSummary         JSON               `gorm:"type:jsonb" json:"bundle_summary"`
	DependencySummary     JSON               `gorm:"type:jsonb" json:"dependency_summary"`
	ConfirmationTokenHash []byte             `json:"-"`
	RequestedByUserID     *int64             `json:"-"`
	RequestedByName       string             `json:"-"`
	ExpiresAt             time.Time          `json:"expires_at"`
	IdempotencyKey        string             `json:"-"`
	LockedAt              *time.Time         `json:"-"`
	ExecutedAt            *time.Time         `json:"executed_at,omitempty"`
	LastError             string             `json:"last_error,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	ConfirmationToken     string             `gorm:"-" json:"confirmation_token,omitempty"`
	Items                 []BundleItem       `gorm:"foreignKey:PlanID" json:"items,omitempty"`
	Dependencies          []DependencyRecord `gorm:"foreignKey:PlanID" json:"dependencies,omitempty"`
}

func (Plan) TableName() string { return "promotion_plans" }

// BundleItem is one entry in the promotion bundle (table promotion_bundle_items).
type BundleItem struct {
	ID                    int64  `gorm:"primaryKey" json:"-"`
	PlanID                string `gorm:"size:36" json:"-"`
	Ordinal               int    `json:"ordinal"`
	Kind                  string `json:"kind"`
	SourceNamespace       string `json:"source_namespace"`
	SourceName            string `json:"source_name"`
	SourceUID             string `json:"source_uid"`
	SourceResourceVersion string `json:"source_resource_version"`
	DestinationNamespace  string `json:"destination_namespace"`
	DestinationName       string `json:"destination_name"`
	Manifest              JSON   `gorm:"type:jsonb" json:"-"`
	Diff                  JSON   `gorm:"type:jsonb" json:"diff"`
	ItemStatus            string `json:"item_status"`
	LastError             string `json:"last_error,omitempty"`
}

func (BundleItem) TableName() string { return "promotion_bundle_items" }

// DependencyRecord persists one operator-provided dependency mapping and its
// preflight resolution status (table promotion_dependency_mappings).
type DependencyRecord struct {
	ID                   int64  `gorm:"primaryKey" json:"-"`
	PlanID               string `gorm:"size:36" json:"-"`
	Kind                 string `json:"kind"`
	SourceNamespace      string `json:"source_namespace"`
	SourceName           string `json:"source_name"`
	DestinationNamespace string `json:"destination_namespace"`
	DestinationName      string `json:"destination_name"`
	Resolved             bool   `json:"resolved"`
}

func (DependencyRecord) TableName() string { return "promotion_dependency_mappings" }

// JSON is a []byte that GORM maps to JSONB and encoding/json marshals as raw.
type JSON []byte

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("promotion.JSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[:0], data...)
	return nil
}

// ItemDiff is the typed diff stored on each bundle item.
type ItemDiff struct {
	Mode          string         `json:"mode"`
	Before        map[string]any `json:"before,omitempty"`
	After         map[string]any `json:"after,omitempty"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
}

// BundleSummary is the bundle-level summary persisted on the plan.
type BundleSummary struct {
	ItemCount       int `json:"item_count"`
	DeploymentCount int `json:"deployment_count"`
	ServiceCount    int `json:"service_count"`
	IngressCount    int `json:"ingress_count"`
	PendingCount    int `json:"pending_count"`
	AppliedCount    int `json:"applied_count"`
	FailedCount     int `json:"failed_count"`
	SkippedCount    int `json:"skipped_count"`
}
