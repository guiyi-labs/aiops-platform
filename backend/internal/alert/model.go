package alert

import (
	"errors"
	"time"
)

const (
	ResourceKindNode = "Node"
	MetricCPU        = "cpu"
	MetricMemory     = "memory"

	OperatorGTE = "gte"
	OperatorLTE = "lte"

	StateFiring           = "firing"
	StateResolved         = "resolved"
	EvalStateFiring       = "firing"
	EvalStateNormal       = "normal"
	EvalStateInsufficient = "insufficient_data"
	EvalStateError        = "error"

	MaxRulesPerCluster = 20
	MinForSeconds      = 60
	MaxForSeconds      = 21600
	MinMinimumPoints   = 2
	MaxMinimumPoints   = 360

	MaxDisplayNameLength  = 128
	MaxResourceNameLength = 253
)

var (
	ErrRuleNotFound        = errors.New("alert rule not found")
	ErrRuleDeleted         = errors.New("alert rule is deleted")
	ErrRuleImmutable       = errors.New("alert rule evaluation fields are immutable")
	ErrRuleUnresolvedAlert = errors.New("cannot delete rule with unresolved alert instance")
	ErrClusterLimit        = errors.New("cluster has reached the maximum number of alert rules")
	ErrDuplicateName       = errors.New("alert rule name already exists in this cluster")
	ErrInvalidRule         = errors.New("alert rule validation failed")
	ErrAlertNotFound       = errors.New("alert instance not found")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Rule struct {
	ID                  int64      `json:"id"`
	ClusterID           int64      `json:"cluster_id"`
	DisplayName         string     `json:"display_name"`
	ResourceKind        string     `json:"resource_kind"`
	ResourceName        string     `json:"resource_name"`
	MetricName          string     `json:"metric_name"`
	Operator            string     `json:"operator"`
	Threshold           int64      `json:"threshold"`
	ForSeconds          int        `json:"for_seconds"`
	MinimumPoints       int        `json:"minimum_points"`
	Enabled             bool       `json:"enabled"`
	Deleted             bool       `json:"deleted"`
	LastEvaluationState string     `json:"last_evaluation_state"`
	LastEvaluationAt    *time.Time `json:"last_evaluation_at,omitempty"`
	LastErrorCode       string     `json:"last_error_code"`
	NextDueAt           time.Time  `json:"next_due_at"`
	ClaimExpiresAt      *time.Time `json:"claim_expires_at,omitempty"`
	Creator             ActorRef   `json:"creator"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (Rule) TableName() string { return "alert_rules" }

type CreateRuleInput struct {
	ClusterID     int64  `json:"cluster_id"`
	DisplayName   string `json:"display_name"`
	ResourceKind  string `json:"resource_kind"`
	ResourceName  string `json:"resource_name"`
	MetricName    string `json:"metric_name"`
	Operator      string `json:"operator"`
	Threshold     int64  `json:"threshold"`
	ForSeconds    int    `json:"for_seconds"`
	MinimumPoints int    `json:"minimum_points"`
}

type PatchRuleInput struct {
	DisplayName *string `json:"display_name,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

func ValidateCreate(input CreateRuleInput) error {
	if input.ClusterID <= 0 {
		return ErrInvalidRule
	}
	if len(input.DisplayName) < 1 || len(input.DisplayName) > MaxDisplayNameLength {
		return ErrInvalidRule
	}
	if input.ResourceKind != ResourceKindNode {
		return ErrInvalidRule
	}
	if len(input.ResourceName) < 1 || len(input.ResourceName) > MaxResourceNameLength {
		return ErrInvalidRule
	}
	if input.MetricName != MetricCPU && input.MetricName != MetricMemory {
		return ErrInvalidRule
	}
	if input.Operator != OperatorGTE && input.Operator != OperatorLTE {
		return ErrInvalidRule
	}
	if input.Threshold < 0 {
		return ErrInvalidRule
	}
	if input.ForSeconds < MinForSeconds || input.ForSeconds > MaxForSeconds {
		return ErrInvalidRule
	}
	if input.MinimumPoints < MinMinimumPoints || input.MinimumPoints > MaxMinimumPoints {
		return ErrInvalidRule
	}
	return nil
}

func ValidatePatch(input PatchRuleInput) error {
	if input.DisplayName != nil && (len(*input.DisplayName) < 1 || len(*input.DisplayName) > MaxDisplayNameLength) {
		return ErrInvalidRule
	}
	return nil
}

type Instance struct {
	ID                   int64      `json:"id"`
	RuleID               int64      `json:"rule_id"`
	DiagnosisID          int64      `json:"diagnosis_id"`
	State                string     `json:"state"`
	FirstFiredAt         time.Time  `json:"first_fired_at"`
	LastFiredAt          time.Time  `json:"last_fired_at"`
	ResolvedAt           *time.Time `json:"resolved_at,omitempty"`
	LatestEvidenceAnchor string     `json:"latest_evidence_anchor"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (Instance) TableName() string { return "alert_instances" }

type RuleListFilter struct {
	ClusterID      int64
	IncludeDeleted bool
	Limit          int
}

type InstanceListFilter struct {
	ClusterID int64
	State     string
	RuleID    int64
	Limit     int
}
