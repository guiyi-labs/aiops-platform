// Package incident implements the M98 incident workspace: a collaborative
// wrapper around a persisted diagnosis (or a client-observed finding) with a
// stable incident number, assignee, followers, a system/note-separated
// timeline, an explicit status machine with CAS versioning, and a read-only
// postmortem view. All state-changing writes are versioned; concurrent
// updates with a stale expected_version fail with a conflict.
package incident

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	StatusOpen      = "open"
	StatusConfirmed = "confirmed"
	StatusResolved  = "resolved"
	StatusDismissed = "dismissed"

	SourceTypeDiagnosis  = "diagnosis"
	SourceTypeFinding    = "finding"
	SourceTypeAlert      = "alert"
	SourceTypeInspection = "inspection"

	EventTypeSystem = "system"
	EventTypeNote   = "note"

	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

var (
	ErrNotFound          = errors.New("incident not found")
	ErrVersionConflict   = errors.New("incident version conflict")
	ErrInvalidTransition = errors.New("invalid incident status transition")
	ErrSourceAlreadyUsed = errors.New("source already has an incident")
	ErrInvalidSource     = errors.New("invalid incident source")
	ErrInvalidNote       = errors.New("incident note content is required")
	ErrInvalidTitle      = errors.New("incident title is required")
	ErrFollowerDuplicate = errors.New("user already follows this incident")
	ErrFollowerNotFound  = errors.New("user does not follow this incident")
	ErrAssigneeNotFound  = errors.New("assignee user does not exist")
	ErrPostmortemLocked  = errors.New("postmortem is only writable for resolved incidents")
)

// ActorRef identifies a platform user in timeline and assignment context.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ResourceRef snapshots the Kubernetes resource the incident is about.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}

// TimelineEvent is one entry on the incident timeline. Human notes use
// EventTypeNote and never merge into generic audit details; system events
// (state changes, handoffs, follower changes) use EventTypeSystem.
type TimelineEvent struct {
	ID        int64     `json:"id"`
	EventType string    `json:"event_type"`
	Actor     ActorRef  `json:"actor"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Follower is a user watching the incident.
type Follower struct {
	UserID  int64     `json:"user_id"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"added_at"`
}

// Incident is the M98 collaborative workspace aggregate.
type Incident struct {
	ID         int64           `json:"id"`
	Number     string          `json:"number"`
	Title      string          `json:"title"`
	SourceType string          `json:"source_type"`
	SourceRef  string          `json:"source_ref"`
	ClusterID  int64           `json:"cluster_id"`
	Resource   ResourceRef     `json:"resource"`
	Severity   string          `json:"severity"`
	Status     string          `json:"status"`
	Summary    string          `json:"summary"`
	Postmortem string          `json:"postmortem,omitempty"`
	Assignee   *ActorRef       `json:"assignee,omitempty"`
	Followers  []Follower      `json:"followers,omitempty"`
	Timeline   []TimelineEvent `json:"timeline,omitempty"`
	Version    int64           `json:"version"`
	ObservedAt time.Time       `json:"observed_at"`
	SLADueAt   time.Time       `json:"sla_due_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
	Overdue    bool            `json:"overdue"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ListFilter filters incident listings.
type ListFilter struct {
	ClusterID  int64
	Status     string
	AssigneeID int64
	FollowerID int64
	Limit      int
}

// Summary is the incident count board.
type Summary struct {
	Total     int64 `json:"total"`
	Open      int64 `json:"open"`
	Confirmed int64 `json:"confirmed"`
	Resolved  int64 `json:"resolved"`
	Dismissed int64 `json:"dismissed"`
	Overdue   int64 `json:"overdue"`
}

// CanTransition is the explicit incident status machine:
//
//	open      -> confirmed | dismissed
//	confirmed -> resolved  | dismissed
//	resolved/dismissed -> open (reopen)
func CanTransition(from, to string) bool {
	switch from {
	case StatusOpen:
		return to == StatusConfirmed || to == StatusDismissed
	case StatusConfirmed:
		return to == StatusResolved || to == StatusDismissed
	case StatusResolved, StatusDismissed:
		return to == StatusOpen
	default:
		return false
	}
}

// ValidSeverity reports whether severity is one of the platform severities.
func ValidSeverity(severity string) bool {
	switch severity {
	case SeverityInfo, SeverityWarning, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

// SLADeadline mirrors the diagnosis SLA contract (critical 1h, high 4h,
// warning 24h, everything else 72h).
func SLADeadline(severity string, observedAt time.Time) time.Time {
	var duration time.Duration
	switch severity {
	case SeverityCritical:
		duration = time.Hour
	case SeverityHigh:
		duration = 4 * time.Hour
	case SeverityWarning:
		duration = 24 * time.Hour
	default:
		duration = 72 * time.Hour
	}
	return observedAt.UTC().Add(duration)
}

// SourceRefForDiagnosis builds the stable dedup identity for a diagnosis.
func SourceRefForDiagnosis(diagnosisID int64) string {
	return "diagnosis:" + strconv.FormatInt(diagnosisID, 10)
}

// SourceRefForAlert builds the stable dedup identity for a firing alert
// instance promoted into an incident workspace.
func SourceRefForAlert(alertID int64) string {
	return "alert:" + strconv.FormatInt(alertID, 10)
}

// SourceRefForInspection builds the stable dedup identity for an inspection
// result promoted into an incident workspace.
func SourceRefForInspection(resultID int64) string {
	return "inspection:" + strconv.FormatInt(resultID, 10)
}

// SourceRefForFinding builds the stable dedup identity for a client-observed
// finding: cluster, code and the resource identity (uid preferred, else name).
func SourceRefForFinding(clusterID int64, code, kind, namespace, name, uid string) string {
	parts := []string{
		"finding",
		strconv.FormatInt(clusterID, 10),
		code,
		kind,
		strings.TrimSpace(namespace),
		name,
	}
	if uid != "" {
		parts = append(parts, uid)
	}
	return strings.Join(parts, ":")
}
