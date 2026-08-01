// Package deprecatedapi performs a read-only check for Kubernetes API objects
// that use deprecated or removed apiVersions, mirroring the behaviour of
// tools such as pluto and kubent. It is intentionally read-only and NEVER
// mutates any cluster object; it only reports which objects would break on a
// cluster upgrade to a given target version.
//
// The catalog of known deprecations is compiled into the binary (see
// catalog.go) so the check is deterministic and has no external dependency.
package deprecatedapi

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// ResourceObject is the minimal contract the analyzer needs from any
// Kubernetes object. Callers extract these fields from the raw list JSON that
// the Kubernetes API always returns (apiVersion + kind are present on every
// item); the typed gateway structs in package kubernetes omit apiVersion at
// the top level, so the service layer is responsible for populating it.
type ResourceObject struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	UID        string `json:"uid,omitempty"`
}

// FindingStatus classifies how severe a deprecation is relative to the
// target cluster version being evaluated against.
const (
	// StatusRemoved means the object's apiVersion was removed in a version at
	// or before the target version; the object will fail to apply after
	// upgrade and is a blocking risk.
	StatusRemoved = "removed"
	// StatusDeprecated means the apiVersion still works on the target version
	// but is scheduled for removal in a later release.
	StatusDeprecated = "deprecated"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend can render it uniformly.
type Finding = k8sfinding.Finding

const (
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// CodeRemovedAPI and CodeDeprecatedAPI are the stable finding codes emitted
// by this analyzer.
const (
	CodeRemovedAPI    = "DEPRECATED_API_REMOVED"
	CodeDeprecatedAPI = "DEPRECATED_API_DEPRECATED"
)

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	TargetMinor int       `json:"target_minor"`
	Total       int       `json:"total"`
	Removed     int       `json:"removed"`
	Deprecated  int       `json:"deprecated"`
	Clean       int       `json:"clean"`
	Findings    []Finding `json:"findings"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// SeverityFor returns the posture severity implied by a finding status.
func SeverityFor(status string) string {
	if status == StatusRemoved {
		return SeverityCritical
	}
	return SeverityWarning
}
