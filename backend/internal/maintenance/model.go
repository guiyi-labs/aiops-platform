package maintenance

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const (
	ActionCordon   = "cordon"
	ActionUncordon = "uncordon"
	ActionDrain    = "drain"

	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusExpired              = "expired"
)

var (
	ErrNotFound            = errors.New("maintenance plan not found")
	ErrInvalidRequest      = errors.New("maintenance request parameters are invalid")
	ErrNodeNotFound        = errors.New("target node not found")
	ErrControlPlaneNode    = errors.New("control-plane nodes cannot be maintained")
	ErrAlreadyCordoned     = errors.New("node is already cordoned")
	ErrAlreadyUncordoned   = errors.New("node is already schedulable")
	ErrNotCordoned         = errors.New("node must be cordoned before drain")
	ErrTooManyPods         = errors.New("node has too many resident pods for bounded drain")
	ErrUnmanagedPod        = errors.New("node has unmanaged pods that block drain")
	ErrEmptyDirPod         = errors.New("node has pods using emptyDir that block drain")
	ErrPDBUnavailable      = errors.New("pdb evidence unavailable for drain")
	ErrStaleTarget         = errors.New("node or pod evidence has changed since preview")
	ErrConfirmationInvalid = errors.New("maintenance confirmation is invalid")
	ErrInvalidIdempotency  = errors.New("maintenance idempotency key is invalid")
	ErrExpired             = errors.New("maintenance plan expired")
	ErrInProgress          = errors.New("maintenance execution is in progress")
	ErrAlreadyExecuted     = errors.New("maintenance plan already used with another idempotency key")
	ErrExecutionFailed     = errors.New("maintenance execution failed")
	ErrPartialDrain        = errors.New("drain completed with partial failures; node remains cordoned")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PodClassification categorizes a resident Pod for drain planning.
type PodClassification string

const (
	PodRetained  PodClassification = "retained"  // DaemonSet / mirror / static — kept, not evicted
	PodEvictable PodClassification = "evictable" // managed, no emptyDir, PDB evidence available
	PodBlocking  PodClassification = "blocking"  // unmanaged, emptyDir, or unknown owner
)

// PodEvidence captures the immutable facts about a resident Pod at preview time.
type PodEvidence struct {
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	UID              string            `json:"uid"`
	ResourceVersion  string            `json:"resource_version"`
	OwnerKind        string            `json:"owner_kind"`
	OwnerName        string            `json:"owner_name"`
	HasEmptyDir      bool              `json:"has_empty_dir"`
	Classification   PodClassification `json:"classification"`
	PDBName          string            `json:"pdb_name,omitempty"`
	PDBDisruptionsOK int32             `json:"pdb_disruptions_allowed,omitempty"`
}

// PodOutcome records the final state of one Pod during drain execution.
type PodOutcome struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Outcome   string `json:"outcome"` // evicted, retained, failed, timeout
	Detail    string `json:"detail,omitempty"`
}

// PreviewEvidence is the immutable snapshot captured at preview time.
type PreviewEvidence struct {
	NodeUID             string        `json:"node_uid"`
	NodeResourceVersion string        `json:"node_resource_version"`
	NodeUnschedulable   bool          `json:"node_unschedulable"`
	Pods                []PodEvidence `json:"pods"`
	RetainedCount       int           `json:"retained_count"`
	EvictableCount      int           `json:"evictable_count"`
	BlockingCount       int           `json:"blocking_count"`
}

// ExecutionResult records what happened during execution.
type ExecutionResult struct {
	NodePatched      bool         `json:"node_patched"`
	UnschedulableNow bool         `json:"unschedulable_now"`
	PodOutcomes      []PodOutcome `json:"pod_outcomes,omitempty"`
	EvictedCount     int          `json:"evicted_count"`
	FailedCount      int          `json:"failed_count"`
	Partial          bool         `json:"partial"`
}

// PreviewEvidenceJSON wraps PreviewEvidence for GORM JSONB storage.
type PreviewEvidenceJSON PreviewEvidence

func (e PreviewEvidenceJSON) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e *PreviewEvidenceJSON) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, e)
	case string:
		return json.Unmarshal([]byte(v), e)
	}
	return errors.New("invalid preview_evidence type")
}

// ExecutionResultJSON wraps ExecutionResult for GORM JSONB storage.
type ExecutionResultJSON ExecutionResult

func (r ExecutionResultJSON) Value() (driver.Value, error) {
	if r.PodOutcomes == nil {
		r.PodOutcomes = []PodOutcome{}
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

// Plan represents a controlled node maintenance plan persisted in maintenance_plans.
type Plan struct {
	ID                    string               `gorm:"primaryKey;size:36" json:"id"`
	ClusterID             int64                `json:"cluster_id"`
	Status                string               `json:"status"`
	Action                string               `json:"action"`
	NodeName              string               `gorm:"column:node_name" json:"node_name"`
	NodeUID               string               `gorm:"column:node_uid" json:"node_uid"`
	NodeResourceVersion   string               `gorm:"column:node_resource_version" json:"node_resource_version"`
	NodeUnschedulable     bool                 `gorm:"column:node_unschedulable" json:"node_unschedulable"`
	PreviewEvidence       PreviewEvidenceJSON  `gorm:"column:preview_evidence;type:jsonb" json:"preview_evidence"`
	ExecutionResult       *ExecutionResultJSON `gorm:"column:execution_result;type:jsonb" json:"execution_result,omitempty"`
	ConfirmationTokenHash []byte               `gorm:"column:confirmation_token_hash" json:"-"`
	RequestedByUserID     *int64               `gorm:"column:requested_by_user_id" json:"-"`
	RequestedByName       string               `gorm:"column:requested_by_name" json:"-"`
	ExpiresAt             time.Time            `json:"expires_at"`
	IdempotencyKey        string               `gorm:"column:idempotency_key" json:"-"`
	LockedAt              *time.Time           `gorm:"column:locked_at" json:"-"`
	ExecutedAt            *time.Time           `json:"executed_at,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
	ConfirmationToken     string               `gorm:"-" json:"confirmation_token,omitempty"`
}

func (Plan) TableName() string { return "maintenance_plans" }

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
	RequestedBy ActorRef `json:"requested_by"`
}

// Request is the input DTO for creating a maintenance plan preview.
type Request struct {
	Action   string
	NodeName string
}

func Response(plan Plan) PlanResponse {
	return PlanResponse{Plan: plan, RequestedBy: plan.RequestedBy()}
}
