package copyops

import (
	"encoding/json"
	"errors"
	"time"
)

// Allowed kinds for M58 cross-cluster copy. Matches the whitelist in the
// migration comment (Deployments, StatefulSets, DaemonSets, CronJobs,
// ConfigMaps, Secrets, Services, Ingresses, ServiceAccounts).
const (
	KindDeployment     = "Deployment"
	KindStatefulSet    = "StatefulSet"
	KindDaemonSet      = "DaemonSet"
	KindCronJob        = "CronJob"
	KindService        = "Service"
	KindIngress        = "Ingress"
	KindServiceAccount = "ServiceAccount"
	KindConfigMap      = "ConfigMap"
	KindSecret         = "Secret"

	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusExpired              = "expired"

	ItemStatusPending = "pending"
	ItemStatusApplied = "applied"
	ItemStatusFailed  = "failed"
	ItemStatusSkipped = "skipped"

	ModeCreate = "create"
	MaxBundle  = 20
)

var (
	ErrNotFound               = errors.New("copy plan not found")
	ErrInvalidRequest         = errors.New("copy request is invalid")
	ErrBundleEmpty            = errors.New("copy bundle must include at least one resource")
	ErrBundleTooLarge         = errors.New("copy bundle is too large")
	ErrKindDisallowed         = errors.New("copy resource kind is not in the operator-curated whitelist")
	ErrSourceUnavailable      = errors.New("copy source cluster is unavailable")
	ErrDestinationUnavailable = errors.New("copy destination cluster is unavailable")
	ErrSourceNotFound         = errors.New("copy source resource was not found on the source cluster")
	ErrNamespaceMissing       = errors.New("copy destination namespace does not exist")
	ErrConflict               = errors.New("copy destination resource already exists")
	ErrPreviewFailed          = errors.New("copy server-side dry-run preview failed")
	ErrConfirmationInvalid    = errors.New("copy confirmation token is invalid")
	ErrInvalidIdempotency     = errors.New("copy idempotency key is invalid or mismatched")
	ErrExpired                = errors.New("copy plan has expired")
	ErrInProgress             = errors.New("copy execution is already in progress")
	ErrAlreadyExecuted        = errors.New("copy plan already executed with another idempotency key")
	ErrExecutionFailed        = errors.New("copy execution failed for one or more items")
	ErrCrossClusterSame       = errors.New("copy source and destination clusters must be different")
)

// ActorRef mirrors the existing remediation/promotion ActorRef.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// BundleItemRequest selects one source resource to copy (namespace-scoped).
type BundleItemRequest struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// ResourceItem is a single entry stored in the `resource_items` JSONB column.
// It holds the group/version/resource tuple plus the stripped manifest
// (post-scrub, pre-dry-run) and execution bookkeeping fields.
type ResourceItem struct {
	Group                 string         `json:"group"`
	Version               string         `json:"version"`
	Resource              string         `json:"resource"`
	Kind                  string         `json:"kind"`
	SourceNamespace       string         `json:"source_namespace"`
	SourceName            string         `json:"source_name"`
	SourceUID             string         `json:"source_uid,omitempty"`
	SourceResourceVersion string         `json:"source_resource_version,omitempty"`
	DestinationNamespace  string         `json:"destination_namespace"`
	DestinationName       string         `json:"destination_name"`
	Manifest              map[string]any `json:"manifest"`
	// Diff holds the ItemDiff (mode + before/after) projected for this item.
	// It is persisted as JSON inside the resource_items array for GUI display.
	Diff JSON `json:"diff,omitempty"`
	// Execution-only fields (populated at Execute time).
	ItemStatus    string `json:"item_status,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	DryRunError   string `json:"dry_run_error,omitempty"`
	AppliedUID    string `json:"applied_uid,omitempty"`
	AppliedRV     string `json:"applied_rv,omitempty"`
	PreflightSkip string `json:"preflight_skip,omitempty"`
}

// CopySummaryItem is a lightweight projection shown in the GUI.
type CopySummaryItem struct {
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	DestNamespace string `json:"destination_namespace"`
	DestName      string `json:"destination_name"`
}

// PlanDiff is the typed structure persisted in the `diff` JSONB column.
type PlanDiff struct {
	ResourceCount         int      `json:"resource_count"`
	TargetNamespaceExists bool     `json:"target_namespace_exists"`
	WillCreateCount       int      `json:"will_create_count"`
	WillSkipCount         int      `json:"will_skip_count"`
	DryRunErrors          []string `json:"dry_run_errors,omitempty"`
}

// ItemDiff is the per-item comparison projected by Preview(). It is stored
// inside each ResourceItem.Diff JSON and rendered by the GUI diff viewer.
// Mode is one of create|update|delete|no-op. ChangedFields is a list of
// JSON-path style selectors that differ (may be `.*` for the entire object
// during a create/delete).
type ItemDiff struct {
	Mode          string         `json:"mode"`
	Before        map[string]any `json:"before,omitempty"`
	After         map[string]any `json:"after,omitempty"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
}

// PreviewRequest is the internal DTO consumed by the service.
type PreviewRequest struct {
	SourceClusterID         int64
	SourceNamespace         string
	TargetClusterID         int64
	TargetNamespace         string
	Bundle                  []BundleItemRequest
	StripSecrets            bool
	StripLabelPrefixes      []string
	StripAnnotationPrefixes []string
}

// Plan is the persisted copy plan (table copy_plans). Note: the table uses
// JSONB arrays (`resource_items`, `copy_summary`, `diff`) instead of
// separate child tables so a single write captures the entire plan.
type Plan struct {
	ID                    string     `gorm:"primaryKey;size:36" json:"id"`
	Status                string     `json:"status"`
	SourceClusterID       int64      `gorm:"column:source_cluster_id" json:"source_cluster_id"`
	SourceNamespace       string     `json:"source_namespace"`
	SourceNamespaceUID    string     `gorm:"column:source_namespace_uid" json:"source_namespace_uid"`
	SourceNamespaceRV     string     `gorm:"column:source_namespace_resource_version" json:"source_namespace_resource_version"`
	TargetClusterID       int64      `gorm:"column:target_cluster_id" json:"target_cluster_id"`
	TargetNamespace       string     `json:"target_namespace"`
	ResourceItems         JSON       `gorm:"column:resource_items;type:jsonb" json:"-"`
	CopySummary           JSON       `gorm:"column:copy_summary;type:jsonb" json:"copy_summary"`
	Diff                  JSON       `gorm:"column:diff;type:jsonb" json:"diff"`
	ConfirmationTokenHash []byte     `gorm:"column:confirmation_token_hash" json:"-"`
	RequestedByUserID     *int64     `gorm:"column:requested_by_user_id" json:"-"`
	RequestedByName       string     `gorm:"column:requested_by_name" json:"-"`
	ExpiresAt             time.Time  `gorm:"column:expires_at" json:"expires_at"`
	IdempotencyKey        string     `gorm:"column:idempotency_key" json:"-"`
	LockedAt              *time.Time `gorm:"column:locked_at" json:"-"`
	ExecutedAt            *time.Time `gorm:"column:executed_at" json:"executed_at,omitempty"`
	LastError             string     `gorm:"column:last_error" json:"last_error,omitempty"`
	CreatedAt             time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at" json:"updated_at"`
	// Transient field returned from Preview() only — never persisted.
	ConfirmationToken string `gorm:"-" json:"confirmation_token,omitempty"`
	// Strip options (not persisted on the plan itself; they're applied
	// during Preview when building ResourceItems and are reflected in the
	// manifests). Exposed in the response via transient fields if needed.
}

func (Plan) TableName() string { return "copy_plans" }

// JSON is a []byte alias that round-trips through encoding/json and GORM
// jsonb columns without double-encoding. Mirrors promotion.JSON.
type JSON []byte

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("copyops.JSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[:0], data...)
	return nil
}

// --- typed JSON marshal helpers -------------------------------------------------

func MarshalResourceItems(items []ResourceItem) JSON {
	if len(items) == 0 {
		data, _ := json.Marshal([]ResourceItem{})
		return JSON(data)
	}
	data, _ := json.Marshal(items)
	return JSON(data)
}

func UnmarshalResourceItems(j JSON) []ResourceItem {
	if len(j) == 0 {
		return nil
	}
	var items []ResourceItem
	_ = json.Unmarshal(j, &items)
	return items
}

func MarshalCopySummary(items []CopySummaryItem) JSON {
	if len(items) == 0 {
		data, _ := json.Marshal([]CopySummaryItem{})
		return JSON(data)
	}
	data, _ := json.Marshal(items)
	return JSON(data)
}

func UnmarshalCopySummary(j JSON) []CopySummaryItem {
	if len(j) == 0 {
		return nil
	}
	var items []CopySummaryItem
	_ = json.Unmarshal(j, &items)
	return items
}

func MarshalPlanDiff(d PlanDiff) JSON {
	data, _ := json.Marshal(d)
	return JSON(data)
}

func UnmarshalPlanDiff(j JSON) PlanDiff {
	if len(j) == 0 {
		return PlanDiff{}
	}
	var d PlanDiff
	_ = json.Unmarshal(j, &d)
	return d
}

// MarshalDiff serializes an item-level ItemDiff (stored inside each
// ResourceItem's Diff field in the JSONB array).
func MarshalDiff(d ItemDiff) JSON {
	data, _ := json.Marshal(d)
	return JSON(data)
}

// UnmarshalDiff returns the typed item diff or zero value.
func UnmarshalDiff(j JSON) ItemDiff {
	if len(j) == 0 {
		return ItemDiff{}
	}
	var d ItemDiff
	_ = json.Unmarshal(j, &d)
	return d
}
