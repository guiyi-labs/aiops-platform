package topology

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ChangeNormalizer converts M23-M31 platform-operation outcomes into
// topology.ChangeEvent records for the change timeline. The caller passes a
// typed ChangePlanInput so a single adapter covers rollout, promotion, backup,
// maintenance and restore. The normalizer is a pure mapping function; it does
// not read from or write to the repository directly.
type ChangeNormalizer struct{}

// NewChangeNormalizer creates a ChangeNormalizer.
func NewChangeNormalizer() ChangeNormalizer { return ChangeNormalizer{} }

// ChangePlanInput is the typed input for ChangeNormalizer.FromPlan. The caller
// (handler or service) builds this from the domain-specific Plan struct. This
// keeps the topology package independent of remediation/promotion/backup/
// maintenance/restore packages.
type ChangePlanInput struct {
	// Kind is the change event kind: rollout | promotion | backup |
	// maintenance | restore | audit.
	Kind string
	// ClusterID is the target cluster. For cross-cluster promotion, this is
	// the destination cluster.
	ClusterID int64
	// Namespace is the target namespace. Empty for cluster-scoped operations
	// (e.g., node maintenance).
	Namespace string
	// PlanID is the domain plan identifier (UUID string). Required.
	PlanID string
	// Target is the affected resource citation.
	Target ResourceCitation
	// Action is the specific action (e.g., "cordon", "deployment.rollout_restart").
	Action string
	// SafeDiffHash is the SHA-256 hash of the safe diff, if a diff was
	// produced. Empty when no diff applies.
	SafeDiffHash string
	// Revision is the rollout revision or image tag, when applicable.
	Revision string
	// Actor is the display name of the user who requested the operation.
	Actor string
	// PlanStatus is the domain status string (awaiting_confirmation,
	// executing, succeeded, failed, expired, partial).
	PlanStatus string
	// CreatedAt is the plan creation timestamp (maps to StartedAt).
	CreatedAt time.Time
	// ExecutedAt is the plan execution timestamp (maps to FinishedAt). May
	// be zero when the plan has not executed yet.
	ExecutedAt time.Time
	// AuditID is the linked audit entry ID, when available.
	AuditID *int64
	// RequestID is the request-scoped correlation ID, when available.
	RequestID string
	// Evidence is optional evidence references to attach.
	Evidence []EvidenceRef
	// Source is the event source: platform | k8s_event | delivery_adapter.
	// Defaults to "platform" when empty.
	Source string
}

// FromPlan builds a ChangeEvent from a ChangePlanInput. The domain status is
// normalized to the change_events result enumeration (succeeded | failed |
// pending | partial). Confidence is "high" for platform-sourced events with
// an audit ID, and "low" otherwise.
func (ChangeNormalizer) FromPlan(input ChangePlanInput) (ChangeEvent, error) {
	if input.Kind == "" {
		return ChangeEvent{}, fmt.Errorf("change event kind is required")
	}
	if !isValidChangeKind(input.Kind) {
		return ChangeEvent{}, fmt.Errorf("unsupported change event kind: %s", input.Kind)
	}
	if input.PlanID == "" {
		return ChangeEvent{}, fmt.Errorf("plan_id is required")
	}

	result := normalizePlanStatus(input.PlanStatus)
	source := input.Source
	if source == "" {
		source = "platform"
	}
	if !isValidChangeSource(source) {
		return ChangeEvent{}, fmt.Errorf("unsupported change source: %s", source)
	}

	confidence := "low"
	if source == "platform" && input.AuditID != nil {
		confidence = "high"
	}

	var finishedAt *time.Time
	if !input.ExecutedAt.IsZero() {
		t := input.ExecutedAt.UTC()
		finishedAt = &t
	}

	startedAt := input.CreatedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	action := input.Action
	if action == "" {
		action = defaultActionForKind(input.Kind)
	}

	return ChangeEvent{
		ClusterID:    input.ClusterID,
		Namespace:    input.Namespace,
		Kind:         input.Kind,
		PlanID:       input.PlanID,
		Target:       input.Target,
		Action:       action,
		SafeDiffHash: input.SafeDiffHash,
		Revision:     input.Revision,
		Actor:        input.Actor,
		StartedAt:    startedAt.UTC(),
		FinishedAt:   finishedAt,
		Result:       result,
		AuditID:      input.AuditID,
		RequestID:    input.RequestID,
		Evidence:     input.Evidence,
		Confidence:   confidence,
		Source:       source,
	}, nil
}

// FromAudit builds a ChangeEvent from an audit entry. Audit-sourced events
// have confidence "high" when the audit entry carries a non-empty RequestID,
// and "low" otherwise. The kind defaults to "audit" when empty.
func (ChangeNormalizer) FromAudit(input AuditChangeInput) (ChangeEvent, error) {
	if input.AuditID == 0 {
		return ChangeEvent{}, fmt.Errorf("audit_id is required")
	}
	kind := input.Kind
	if kind == "" {
		kind = "audit"
	}
	if !isValidChangeKind(kind) {
		return ChangeEvent{}, fmt.Errorf("unsupported change event kind: %s", kind)
	}

	result := normalizeAuditResult(input.Result)
	aid := input.AuditID
	confidence := "low"
	if input.RequestID != "" {
		confidence = "high"
	}

	return ChangeEvent{
		ClusterID:  input.ClusterID,
		Namespace:  input.Namespace,
		Kind:       kind,
		PlanID:     input.PlanID,
		Target:     input.Target,
		Action:     input.Action,
		Actor:      input.Actor,
		StartedAt:  input.ObservedAt.UTC(),
		Result:     result,
		AuditID:    &aid,
		RequestID:  input.RequestID,
		Evidence:   input.Evidence,
		Confidence: confidence,
		Source:     "platform",
	}, nil
}

// AuditChangeInput is the typed input for audit-sourced change events.
type AuditChangeInput struct {
	ClusterID  int64
	Namespace  string
	Kind       string
	PlanID     string
	Target     ResourceCitation
	Action     string
	Actor      string
	Result     string // audit result: succeeded | failed | denied | error
	ObservedAt time.Time
	AuditID    int64
	RequestID  string
	Evidence   []EvidenceRef
}

// IngestChangeEvent persists a change event via the repository. It is a thin
// wrapper around Repository.UpsertChangeEvent that validates the event before
// persisting. Callers use the ChangeNormalizer to build the event, then call
// this method (or the Service.IngestChangeEvent method) to persist it.
func (s *Service) IngestChangeEvent(ctx context.Context, event *ChangeEvent) error {
	if s.repository == nil {
		return errors.New("topology repository is not configured")
	}
	if event == nil {
		return errors.New("change event is nil")
	}
	if event.Kind == "" {
		return errors.New("change event kind is required")
	}
	if !isValidChangeKind(event.Kind) {
		return fmt.Errorf("unsupported change event kind: %s", event.Kind)
	}
	if event.PlanID == "" && event.AuditID == nil {
		return errors.New("change event requires plan_id or audit_id")
	}
	if event.Result == "" {
		event.Result = "pending"
	}
	if !isValidChangeResult(event.Result) {
		return fmt.Errorf("unsupported change result: %s", event.Result)
	}
	if event.Confidence == "" {
		event.Confidence = "low"
	}
	if !isValidConfidence(event.Confidence) {
		return fmt.Errorf("unsupported confidence: %s", event.Confidence)
	}
	if event.Source == "" {
		event.Source = "platform"
	}
	if !isValidChangeSource(event.Source) {
		return fmt.Errorf("unsupported change source: %s", event.Source)
	}
	return s.repository.UpsertChangeEvent(ctx, event)
}

// --- helpers ---

var validChangeKinds = map[string]bool{
	"promotion": true, "backup": true, "maintenance": true,
	"restore": true, "rollout": true, "audit": true,
}

func isValidChangeKind(kind string) bool { return validChangeKinds[kind] }

var validChangeResults = map[string]bool{
	"succeeded": true, "failed": true, "pending": true, "partial": true,
}

func isValidChangeResult(result string) bool { return validChangeResults[result] }

var validConfidenceLevels = map[string]bool{"high": true, "low": true}

func isValidConfidence(c string) bool { return validConfidenceLevels[c] }

var validChangeSources = map[string]bool{
	"platform": true, "k8s_event": true, "delivery_adapter": true,
}

func isValidChangeSource(s string) bool { return validChangeSources[s] }

// normalizePlanStatus maps domain plan statuses to change_events result values.
// Domain statuses: awaiting_confirmation, executing, succeeded, failed,
// expired, partial.
// Change results: succeeded, failed, pending, partial.
func normalizePlanStatus(status string) string {
	switch status {
	case "succeeded":
		return "succeeded"
	case "failed", "expired":
		return "failed"
	case "partial":
		return "partial"
	case "awaiting_confirmation", "executing", "":
		return "pending"
	default:
		return "pending"
	}
}

// normalizeAuditResult maps audit result strings to change_events result
// values. Audit results: succeeded, failed, denied, error.
func normalizeAuditResult(result string) string {
	switch result {
	case "succeeded":
		return "succeeded"
	case "failed", "error":
		return "failed"
	case "denied":
		return "failed"
	default:
		return "pending"
	}
}

// defaultActionForKind returns a default action when the input does not
// specify one.
func defaultActionForKind(kind string) string {
	switch kind {
	case "promotion":
		return "promote"
	case "backup":
		return "backup.create"
	case "maintenance":
		return "maintain"
	case "restore":
		return "restore.rehearse"
	case "rollout":
		return "rollout_restart"
	case "audit":
		return "audit"
	default:
		return ""
	}
}

// HashSafeDiff computes a SHA-256 hash of a safe diff payload for storage in
// ChangeEvent.SafeDiffHash. The input must be the redacted, safe diff content
// (never raw manifests with secrets).
func HashSafeDiff(safeDiff []byte) string {
	if len(safeDiff) == 0 {
		return ""
	}
	h := sha256.Sum256(safeDiff)
	return hex.EncodeToString(h[:])
}

// FormatPlanIDHash returns a short, deterministic hash of a plan ID for use
// in evidence references when the full plan ID should not be disclosed.
func FormatPlanIDHash(planID string) string {
	if planID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(planID))
	return hex.EncodeToString(h[:])[:16]
}

// JoinEvidenceKinds returns a comma-separated list of evidence kind labels,
// used for logging and completeness checks.
func JoinEvidenceKinds(evidence []EvidenceRef) string {
	if len(evidence) == 0 {
		return ""
	}
	kinds := make([]string, 0, len(evidence))
	for _, e := range evidence {
		kinds = append(kinds, e.Kind)
	}
	return strings.Join(kinds, ",")
}
