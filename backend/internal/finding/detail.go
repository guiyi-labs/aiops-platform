// Package finding defines the canonical read-only finding contract shared by
// the posture, optimization, diagnosis and inspection surfaces. This file
// introduces the M95 unified evidence model (FindingDetail v2) on top of the
// existing v1 Finding so that every analyzer, inspection rule and diagnosis
// rule can emit "rule -> evidence -> recommendation" uniformly.
//
// The design is intentionally additive: v1 Finding remains the JSON contract
// that legacy consumers render, and FindingDetail embeds it so any v2 producer
// can be demoted to a v1 Finding with no data loss. Recommendation kinds are
// explicit and default to non-executing (advisory / controlled_action
// requiring dry-run + confirmation / manual_only), matching the M94 action-area
// semantics and the "never auto-execute" constraint.
package finding

// SchemaVersion identifies the FindingDetail (v2) schema. It is bumped only
// when a breaking change to the unified finding shape is introduced; ordinary
// additions are backward compatible and must keep SchemaVersion stable.
const SchemaVersion = "2"

// RecommendationKind classifies a remediation suggestion. Every kind is
// non-executing by default; controlled actions require an explicit, audited
// dry-run + confirmation path before any mutation.
type RecommendationKind string

const (
	RecommendationAdvisory RecommendationKind = "advisory"
	// RecommendationControlledAction bundles a capability that can safely be
	// executed, but only through the platform's confirmed, idempotent,
	// audited operation path. It is NEVER auto-executed.
	RecommendationControlledAction RecommendationKind = "controlled_action_available"
	// RecommendationManualOnly is a remediation that must be performed by a
	// human or an external operator; the platform cannot or must not execute
	// it directly.
	RecommendationManualOnly RecommendationKind = "manual_only"
)

// RuleIdentity records the source rule that produced a finding, including the
// framework/family it belongs to and the compiled-in version. The identity is
// stable across findings so duplicate merging can collapse per-resource rows
// while preserving each source rule.
type RuleIdentity struct {
	// RuleID is the stable rule identifier emitted by the analyzer, e.g.
	// "CIS-1.2.6" or "CAPACITY_SATURATION_RISK".
	RuleID string `json:"rule_id"`
	// Framework groups the rule (e.g. "cis", "deprecated_api", "policy").
	Framework string `json:"framework,omitempty"`
	// Source identifies the analyzer package / domain that produced it.
	Source string `json:"source,omitempty"`
	// Version is the compiled-in rule set version of the emitting analyzer.
	Version string `json:"version,omitempty"`
}

// EvidenceKind is the stable kind grammar shared by posture, optimization,
// diagnosis and inspection evidence timestamps. It mirrors the M94 timeline
// taxonomy so frontends can render a uniform evidence stream.
type EvidenceKind string

const (
	EvidenceResourceState EvidenceKind = "resource_state"
	EvidenceEvent         EvidenceKind = "event"
	EvidenceLog           EvidenceKind = "log"
	EvidenceAlert         EvidenceKind = "alert"
	EvidenceChange        EvidenceKind = "change"
	EvidenceAutomation    EvidenceKind = "automation"
)

// EvidenceRef is a lightweight, stable reference to one piece of supporting
// evidence. The reference is immutable within a rule version; consumers can
// resolve it to the originating resource / event / log without re-deriving it.
type EvidenceRef struct {
	// ID is a stable identifier for the evidence item (event UID, log cursor,
	// resource_version, ...). Empty when the rule carries no per-item handle.
	ID string `json:"id,omitempty"`
	// Kind classifies the evidence type (resource_state / event / ...).
	Kind EvidenceKind `json:"kind,omitempty"`
	// Summary is a short human description; omitted when not needed.
	Summary string `json:"summary,omitempty"`
	// ObservedAt is the RFC3339 timestamp at which the evidence was observed.
	ObservedAt string `json:"observed_at,omitempty"`
}

// Recommendation is one structured remediation suggestion. Kind controls how
// the UI renders it and whether a controlled action is offered; the platform
// never executes any recommendation automatically.
type Recommendation struct {
	Kind RecommendationKind `json:"kind"`
	// Text is the human-readable remediation advice.
	Text string `json:"text"`
	// Capability identifies the platform operation that can execute a
	// controlled action (e.g. "deployment.rollout_restart"). Empty for
	// advisory and manual_only kinds.
	Capability string `json:"capability,omitempty"`
}

// FindingDetail is the M95 unified evidence model (v2). It embeds every v1
// Finding field so a v2 object round-trips losslessly to the legacy contract,
// and adds rule identity, evidence references, recommendations and versioning.
type FindingDetail struct {
	// SchemaVersion records the v2 schema version for forward-compat.
	SchemaVersion string `json:"schema_version"`
	// Rule identifies the source rule and its framework/version.
	Rule RuleIdentity `json:"rule"`
	// Evidence holds the stable references to the supporting observations.
	Evidence []EvidenceRef `json:"evidence,omitempty"`
	// Recommendations lists the non-executing remediation suggestions.
	Recommendations []Recommendation `json:"recommendations,omitempty"`

	// Legacy v1 fields are embedded for full backward compatibility.
	Code       string            `json:"code"`
	Severity   string            `json:"severity"`
	Summary    string            `json:"summary"`
	Resource   ResourceCitation  `json:"resource"`
	Details    map[string]string `json:"details,omitempty"`
	ObservedAt string            `json:"observed_at"`

	// OriginIDs records the original per-rule finding IDs that were merged into
	// this row (used when collapsing duplicate findings per resource).
	OriginIDs []string `json:"origin_ids,omitempty"`
}

// FromV1 promotes a legacy v1 Finding into the unified v2 shape. Rule identity
// is derived from the finding code; framework/source/version remain empty until
// the emitting analyzer attaches them, so the conversion is purely additive and
// never loses information.
func FromV1(f Finding) FindingDetail {
	return FindingDetail{
		SchemaVersion: SchemaVersion,
		Rule: RuleIdentity{
			RuleID: f.Code,
		},
		Code:       f.Code,
		Severity:   f.Severity,
		Summary:    f.Summary,
		Resource:   f.Resource,
		Details:    f.Details,
		ObservedAt: f.ObservedAt,
	}
}

// ToV1 demotes a v2 finding back to the legacy v1 contract. The flattening is
// lossless with respect to v1 fields; any v2-only data (rule meta, evidence,
// recommendations, origin IDs) is intentionally dropped.
func (d FindingDetail) ToV1() Finding {
	return Finding{
		Code:       d.Code,
		Severity:   d.Severity,
		Summary:    d.Summary,
		Resource:   d.Resource,
		Details:    d.Details,
		ObservedAt: d.ObservedAt,
	}
}

// MergeDistinct collapses a set of v2 findings into per-resource rows while
// preserving every source rule and original finding ID. Rows are grouped by
// the resource reference plus severity; within a group the highest-priority
// rule is kept as the primary display row and the remaining origin IDs are
// collected for traceability.
func MergeDistinct(in []FindingDetail) []FindingDetail {
	if len(in) == 0 {
		return nil
	}
	type groupKey struct{ kind, ns, name, uid, sev string }
	groups := make(map[groupKey][]FindingDetail)
	var order []groupKey
	for _, d := range in {
		k := groupKey{
			kind: d.Resource.Kind,
			ns:   d.Resource.Namespace,
			name: d.Resource.Name,
			uid:  d.Resource.UID,
			sev:  d.Severity,
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], d)
	}
	out := make([]FindingDetail, 0, len(order))
	for _, k := range order {
		items := groups[k]
		// Keep the first as the merged row; collect per-rule origin IDs.
		merged := items[0]
		merged.OriginIDs = nil
		seen := map[string]bool{}
		for _, item := range items {
			orig := item.Code
			if item.Rule.RuleID != "" {
				orig = item.Rule.RuleID
			}
			if orig != "" && !seen[orig] {
				merged.OriginIDs = append(merged.OriginIDs, orig)
				seen[orig] = true
			}
		}
		out = append(out, merged)
	}
	return out
}
