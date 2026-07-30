package restore

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const (
	ActionRehearseRestore = "restore.rehearse"

	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusExpired              = "expired"

	// RestorePhase values observed from Velero Restore CR status.phase.
	PhaseNew             = "New"
	PhaseInProgress      = "InProgress"
	PhaseCompleted       = "Completed"
	PhasePartiallyFailed = "PartiallyFailed"
	PhaseFailed          = "Failed"

	// Allowlisted resource kinds for the Velero Restore.
	KindDeployment     = "Deployment"
	KindStatefulSet    = "StatefulSet"
	KindDaemonSet      = "DaemonSet"
	KindCronJob        = "CronJob"
	KindConfigMap      = "ConfigMap"
	KindSecret         = "Secret"
	KindServiceAccount = "ServiceAccount"

	// Quarantine constants.
	QuarantineNetworkPolicyName = "quarantine-default-deny"
	QuarantineResourceQuotaName = "quarantine-zero-pods"

	// Restoration bounds.
	MaxProjectedRestoredResources = 100
	RestorePollAttempts           = 60
	RestorePollInterval           = 5 * time.Second
)

// AllowedKinds is the fixed allowlist of resource kinds Restored by the V1
// rehearsal. Order matters for dry-run stability; the slice is the canonical
// source for both manifest construction and audit display.
var AllowedKinds = []string{
	KindDeployment,
	KindStatefulSet,
	KindDaemonSet,
	KindCronJob,
	KindConfigMap,
	KindSecret,
	KindServiceAccount,
}

var (
	ErrNotFound               = errors.New("restore plan not found")
	ErrInvalidRequest         = errors.New("restore request parameters are invalid")
	ErrVeleroNotInstalled     = errors.New("Velero is not installed on the target cluster")
	ErrSourceBackupNotFound   = errors.New("source backup not found")
	ErrSourceBackupIncomplete = errors.New("source backup is not in Completed phase")
	ErrSourceBackupScope      = errors.New("source backup scope is not M28-compatible")
	ErrDestinationExists      = errors.New("destination namespace already exists")
	ErrDestinationCollision   = errors.New("destination namespace collides with an active restore plan")
	ErrRestoreNameConflict    = errors.New("velero restore name already exists on the target cluster")
	ErrQuarantineDryRunFailed = errors.New("quarantine resource dry-run failed")
	ErrRestoreDryRunFailed    = errors.New("velero restore dry-run failed")
	ErrProjectionTruncated    = errors.New("projected restored resources exceed the bounded limit")
	ErrConfirmationInvalid    = errors.New("restore confirmation is invalid")
	ErrInvalidIdempotency     = errors.New("restore idempotency key is invalid")
	ErrExpired                = errors.New("restore plan expired")
	ErrInProgress             = errors.New("restore execution is in progress")
	ErrAlreadyExecuted        = errors.New("restore plan already used with another idempotency key")
	ErrStaleSource            = errors.New("source backup has changed since preview")
	ErrExecutionFailed        = errors.New("restore execution failed")
	ErrQuarantineFailed       = errors.New("quarantine controls were not established before restore")
	ErrRestorePollTimeout     = errors.New("velero restore did not reach a terminal phase within the bounded wait")
	ErrPartialRestore         = errors.New("velero restore completed with partial failures; quarantine target retained")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// QuarantineStatus records the dry-run and observed state of the two
// quarantine controls created before the Velero Restore.
type QuarantineStatus struct {
	NamespaceCreated     bool   `json:"namespace_created"`
	NamespaceUID         string `json:"namespace_uid,omitempty"`
	NetworkPolicyName    string `json:"network_policy_name"`
	NetworkPolicyCreated bool   `json:"network_policy_created"`
	ResourceQuotaName    string `json:"resource_quota_name"`
	ResourceQuotaCreated bool   `json:"resource_quota_created"`
	DryRunValidated      bool   `json:"dry_run_validated"`
}

// QuarantineStatusJSON wraps QuarantineStatus for GORM JSONB storage.
type QuarantineStatusJSON QuarantineStatus

func (s QuarantineStatusJSON) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *QuarantineStatusJSON) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	}
	return errors.New("invalid quarantine_status type")
}

// RestoredItem records one resource Restored by Velero. Low-cardinality only;
// no Secret/ConfigMap values are captured.
type RestoredItem struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ExecutionResult records what happened during execution.
type ExecutionResult struct {
	RestoreCreated        bool           `json:"restore_created"`
	RestorePhase          string         `json:"restore_phase,omitempty"`
	RestoreUID            string         `json:"restore_uid,omitempty"`
	RestoredItems         []RestoredItem `json:"restored_items,omitempty"`
	RestoredItemCount     int            `json:"restored_item_count"`
	TruncatedItems        bool           `json:"truncated_items"`
	QuarantineEstablished bool           `json:"quarantine_established"`
	FailureReason         string         `json:"failure_reason,omitempty"`
	Partial               bool           `json:"partial"`
}

// ExecutionResultJSON wraps ExecutionResult for GORM JSONB storage.
type ExecutionResultJSON ExecutionResult

func (r ExecutionResultJSON) Value() (driver.Value, error) {
	if r.RestoredItems == nil {
		r.RestoredItems = []RestoredItem{}
	}
	return json.Marshal(r)
}

func (r *ExecutionResultJSON) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, r)
	case string:
		return json.Unmarshal([]byte(v), r)
	}
	return errors.New("invalid execution_result type")
}

// Plan represents a controlled restore rehearsal plan persisted in restore_plans.
type Plan struct {
	ID                          string               `gorm:"primaryKey;size:36" json:"id"`
	ClusterID                   int64                `json:"cluster_id"`
	Status                      string               `json:"status"`
	SourceBackupName            string               `gorm:"column:source_backup_name" json:"source_backup_name"`
	SourceBackupNamespace       string               `gorm:"column:source_backup_namespace" json:"source_backup_namespace"`
	SourceBackupUID             string               `gorm:"column:source_backup_uid" json:"source_backup_uid"`
	SourceBackupResourceVersion string               `gorm:"column:source_backup_resource_version" json:"source_backup_resource_version"`
	SourceBackupPhase           string               `gorm:"column:source_backup_phase" json:"source_backup_phase"`
	DestinationNamespace        string               `gorm:"column:destination_namespace" json:"destination_namespace"`
	DestinationNamespaceUID     string               `gorm:"column:destination_namespace_uid" json:"destination_namespace_uid"`
	VeleroRestoreName           string               `gorm:"column:velero_restore_name" json:"velero_restore_name"`
	VeleroRestoreNamespace      string               `gorm:"column:velero_restore_namespace" json:"velero_restore_namespace"`
	VeleroRestoreUID            string               `gorm:"column:velero_restore_uid" json:"velero_restore_uid"`
	QuarantineStatus            QuarantineStatusJSON `gorm:"column:quarantine_status;type:jsonb" json:"quarantine_status"`
	ExecutionResult             *ExecutionResultJSON `gorm:"column:execution_result;type:jsonb" json:"execution_result,omitempty"`
	ConfirmationTokenHash       []byte               `gorm:"column:confirmation_token_hash" json:"-"`
	RequestedByUserID           *int64               `gorm:"column:requested_by_user_id" json:"-"`
	RequestedByName             string               `gorm:"column:requested_by_name" json:"-"`
	ExpiresAt                   time.Time            `json:"expires_at"`
	IdempotencyKey              string               `gorm:"column:idempotency_key" json:"-"`
	LockedAt                    *time.Time           `gorm:"column:locked_at" json:"-"`
	ExecutedAt                  *time.Time           `json:"executed_at,omitempty"`
	LastError                   string               `json:"last_error,omitempty"`
	CreatedAt                   time.Time            `json:"created_at"`
	UpdatedAt                   time.Time            `json:"updated_at"`
	ConfirmationToken           string               `gorm:"-" json:"confirmation_token,omitempty"`
}

func (Plan) TableName() string { return "restore_plans" }

func (p Plan) RequestedBy() ActorRef {
	id := int64(0)
	if p.RequestedByUserID != nil {
		id = *p.RequestedByUserID
	}
	return ActorRef{ID: id, Name: p.RequestedByName}
}

// PlanResponse is the HTTP-facing projection of a Plan.
type PlanResponse struct {
	Plan
	RequestedBy     ActorRef      `json:"requested_by"`
	AllowedKinds    []string      `json:"allowed_kinds"`
	ExcludedKinds   []string      `json:"excluded_kinds"`
	SourceSnapshot  SourceSummary `json:"source_snapshot"`
	DestinationName string        `json:"destination_name"`
}

// SourceSummary is the immutable projection of the source Backup captured at
// preview time.
type SourceSummary struct {
	Name               string   `json:"name"`
	Namespace          string   `json:"namespace"`
	UID                string   `json:"uid"`
	ResourceVersion    string   `json:"resource_version"`
	Phase              string   `json:"phase"`
	IncludedNamespaces []string `json:"included_namespaces,omitempty"`
}

// Request is the input DTO for creating a restore plan preview.
type Request struct {
	SourceBackupName      string
	SourceBackupNamespace string
}

// ExcludedKinds is the explicit list of resource kinds the V1 restore refuses
// to project. It is documented in ADR 0047 §1 and surfaced in the API response
// so the UI can show the exact exclusions.
var ExcludedKinds = []string{
	"Pod",
	"Job",
	"Service",
	"Ingress",
	"Endpoints",
	"EndpointSlice",
	"PersistentVolumeClaim",
	"PersistentVolume",
	"VolumeSnapshot",
	"VolumeSnapshotContent",
	"ResourceQuota",
	"LimitRange",
	"NetworkPolicy",
	"RoleBinding",
	"ClusterRoleBinding",
	"Role",
	"ClusterRole",
	"MutatingWebhookConfiguration",
	"ValidatingWebhookConfiguration",
}

func Response(plan Plan) PlanResponse {
	return PlanResponse{
		Plan:          plan,
		RequestedBy:   plan.RequestedBy(),
		AllowedKinds:  append([]string(nil), AllowedKinds...),
		ExcludedKinds: append([]string(nil), ExcludedKinds...),
		SourceSnapshot: SourceSummary{
			Name:               plan.SourceBackupName,
			Namespace:          plan.SourceBackupNamespace,
			UID:                plan.SourceBackupUID,
			ResourceVersion:    plan.SourceBackupResourceVersion,
			Phase:              plan.SourceBackupPhase,
			IncludedNamespaces: nil,
		},
		DestinationName: plan.DestinationNamespace,
	}
}
