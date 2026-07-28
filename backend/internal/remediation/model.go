package remediation

import (
	"errors"
	"time"
)

const (
	ActionDeploymentRolloutRestart = "deployment.rollout_restart"
	ActionDeploymentScale          = "deployment.scale"
	ActionCronJobSuspend           = "cronjob.suspend"
	ActionCronJobResume            = "cronjob.resume"
	StatusAwaitingConfirmation     = "awaiting_confirmation"
	StatusExecuting                = "executing"
	StatusSucceeded                = "succeeded"
	StatusFailed                   = "failed"
	StatusExpired                  = "expired"
)

var (
	ErrNotFound             = errors.New("remediation plan not found")
	ErrUnsupportedAction    = errors.New("unsupported remediation action")
	ErrDiagnosisNotEligible = errors.New("diagnosis is not eligible for remediation")
	ErrTargetMismatch       = errors.New("remediation target does not match diagnosis")
	ErrTargetChanged        = errors.New("remediation target changed after diagnosis")
	ErrConfirmationInvalid  = errors.New("remediation confirmation is invalid")
	ErrInvalidIdempotency   = errors.New("remediation idempotency key is invalid")
	ErrExpired              = errors.New("remediation plan expired")
	ErrInProgress           = errors.New("remediation execution is in progress")
	ErrAlreadyExecuted      = errors.New("remediation plan already used with another idempotency key")
	ErrExecutionFailed      = errors.New("remediation execution failed")
	ErrInvalidOperation     = errors.New("controlled operation parameters are invalid")
	ErrOperationNoChange    = errors.New("controlled operation would not change the target")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TargetRef struct {
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
}

type Plan struct {
	ID                    string     `gorm:"primaryKey;size:36" json:"id"`
	DiagnosisID           *int64     `json:"diagnosis_id,omitempty"`
	ClusterID             int64      `json:"cluster_id"`
	Action                string     `json:"action"`
	Status                string     `json:"status"`
	TargetKind            string     `json:"-"`
	TargetNamespace       string     `json:"-"`
	TargetName            string     `json:"-"`
	TargetUID             string     `json:"-"`
	TargetResourceVersion string     `json:"-"`
	RestartAt             *time.Time `json:"restart_at,omitempty"`
	BeforeReplicas        *int32     `json:"-"`
	DesiredReplicas       *int32     `json:"-"`
	BeforeSuspended       *bool      `json:"-"`
	DesiredSuspended      *bool      `json:"-"`
	ConfirmationTokenHash []byte     `json:"-"`
	RequestedByUserID     *int64     `json:"-"`
	RequestedByName       string     `json:"-"`
	ExpiresAt             time.Time  `json:"expires_at"`
	IdempotencyKey        string     `json:"-"`
	LockedAt              *time.Time `json:"-"`
	ExecutedAt            *time.Time `json:"executed_at,omitempty"`
	LastError             string     `json:"last_error,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ConfirmationToken     string     `gorm:"-" json:"confirmation_token,omitempty"`
}

func (Plan) TableName() string { return "remediation_plans" }

func (p Plan) Target() TargetRef {
	return TargetRef{Kind: p.TargetKind, Namespace: p.TargetNamespace, Name: p.TargetName, UID: p.TargetUID, ResourceVersion: p.TargetResourceVersion}
}

func (p Plan) RequestedBy() ActorRef {
	id := int64(0)
	if p.RequestedByUserID != nil {
		id = *p.RequestedByUserID
	}
	return ActorRef{ID: id, Name: p.RequestedByName}
}

type PlanResponse struct {
	Plan
	Target      TargetRef           `json:"target"`
	RequestedBy ActorRef            `json:"requested_by"`
	Parameters  OperationParameters `json:"parameters"`
	Change      *OperationChange    `json:"change,omitempty"`
}

type OperationParameters struct {
	DesiredReplicas  *int32 `json:"desired_replicas,omitempty"`
	DesiredSuspended *bool  `json:"desired_suspended,omitempty"`
}

type OperationChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

func Response(plan Plan) PlanResponse {
	response := PlanResponse{Plan: plan, Target: plan.Target(), RequestedBy: plan.RequestedBy()}
	switch plan.Action {
	case ActionDeploymentScale:
		response.Parameters.DesiredReplicas = plan.DesiredReplicas
		if plan.BeforeReplicas != nil && plan.DesiredReplicas != nil {
			response.Change = &OperationChange{Field: "spec.replicas", Before: *plan.BeforeReplicas, After: *plan.DesiredReplicas}
		}
	case ActionCronJobSuspend, ActionCronJobResume:
		response.Parameters.DesiredSuspended = plan.DesiredSuspended
		if plan.BeforeSuspended != nil && plan.DesiredSuspended != nil {
			response.Change = &OperationChange{Field: "spec.suspend", Before: *plan.BeforeSuspended, After: *plan.DesiredSuspended}
		}
	}
	return response
}
