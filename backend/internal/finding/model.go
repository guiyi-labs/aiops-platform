// Package finding defines the canonical read-only "finding" contract used by
// the platform's posture and analysis modules. It is a structural mirror of
// internal/namespaceposture.Finding so that every analyzer (namespace posture,
// deprecated-API check, FinOps right-sizing, ...) emits findings the frontend
// can render uniformly.
//
// It is intentionally dependency-free (no cluster/kubernetes imports) so that
// any analyzer package can adopt it without pulling in heavier dependencies.
package finding

import "time"

const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// ResourceCitation identifies the Kubernetes object a finding is about.
type ResourceCitation struct {
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace,omitempty"`
	Name            string `json:"name"`
	UID             string `json:"uid,omitempty"`
	ResourceVersion string `json:"resource_version,omitempty"`
}

// Finding is a single read-only observation about a cluster resource.
type Finding struct {
	Code       string            `json:"code"`
	Severity   string            `json:"severity"`
	Summary    string            `json:"summary"`
	Resource   ResourceCitation  `json:"resource"`
	Details    map[string]string `json:"details,omitempty"`
	ObservedAt string            `json:"observed_at"`
}

// RFC3339 formats a time for the ObservedAt field, matching the platform
// convention used across posture modules.
func RFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
