// Package correlation implements the M42 multi-signal correlation and
// deterministic root-cause-candidate model.
//
// M42 uses versioned, replayable rules to link M39 signal occurrences, M40
// topology edges/change events and existing diagnosis records into bounded
// cases. It does not introduce a black-box anomaly score or a second incident
// workflow — `diagnosis_records` remains the human status, assignment, SLA,
// feedback and audit source of truth.
//
// Correlation factors are explicit: same UID, topology distance, bounded time
// distance, reviewed change-symptom rule, signal freshness/completeness and
// deterministic diagnosis match. Temporal proximity alone is never causality.
// Conflicting or insufficient evidence yields `unknown` or a candidate, never
// a confirmed root cause.
package correlation

import "time"

// CorrelationVersion is the schema version of the correlation rule contract.
// Bumped when a rule's factor evaluation, case_key derivation or confidence
// classification changes. Identical evidence + rule + correlation versions
// reproduce identical factors, reason codes and links.
const CorrelationVersion = "1.0"

// ConfidenceClass classifies how well the evidence supports a case or
// candidate. The class never upgrades itself; AI (M43) cannot promote a
// candidate to confirmed — only an operator reviewing the diagnosis record
// can do that, and the promotion is recorded in the diagnosis lifecycle.
type ConfidenceClass string

const (
	// ConfidenceConfirmed: deterministic factors fully match a reviewed
	// change-symptom rule. The linked change event is the asserted root
	// cause. Reserved for scenarios in the golden replay set where the
	// rule's required factors are all present and no contradicting factor
	// is observed.
	ConfidenceConfirmed ConfidenceClass = "confirmed"
	// ConfidenceCandidate: factors partially match. The linked change event
	// is a plausible cause but evidence is incomplete or a contradicting
	// factor exists. M43 may rank candidates but cannot promote them.
	ConfidenceCandidate ConfidenceClass = "candidate"
	// ConfidenceContradicted: a contradicting factor is observed (e.g. the
	// change succeeded but symptoms persist, or a different UID is involved).
	// The candidate is retained for audit but not ranked as a cause.
	ConfidenceContradicted ConfidenceClass = "contradicted"
	// ConfidenceUnknown: insufficient evidence to classify. The case is
	// retained so M43 can disclose uncertainty; no root cause is asserted.
	ConfidenceUnknown ConfidenceClass = "unknown"
)

// CaseStatus is the lifecycle status of a correlation case. A case is active
// while any linked signal occurrence is active; it resolves when all linked
// signals resolve. Cases never auto-close — a case with unknown confidence
// remains active until an operator dismisses the underlying diagnosis.
type CaseStatus string

const (
	CaseStatusActive   CaseStatus = "active"
	CaseStatusResolved CaseStatus = "resolved"
	CaseStatusStale    CaseStatus = "stale" // no new signals within stale window
)

// EvidenceCompleteness describes whether the case has enough evidence to
// classify confidence. Mirrors signal.Coverage and slo.EvaluationCoverage.
type EvidenceCompleteness string

const (
	CompletenessComplete     EvidenceCompleteness = "complete"
	CompletenessPartial      EvidenceCompleteness = "partial"
	CompletenessInsufficient EvidenceCompleteness = "insufficient"
)

// SignalRelation describes how a signal occurrence relates to a case.
type SignalRelation string

const (
	// SignalRelationTrigger: the signal that opened the case (e.g. the
	// first pod-failure signal in a rollout-correlation case).
	SignalRelationTrigger SignalRelation = "trigger"
	// SignalRelationContext: a co-observed signal that adds context (e.g.
	// a metric-breach signal alongside a pod-failure signal).
	SignalRelationContext SignalRelation = "context"
	// SignalRelationChange: a change signal (from M40 change_events
	// normalized into signal_occurrences) that is a candidate root cause.
	SignalRelationChange SignalRelation = "change"
	// SignalRelationOutcome: a signal observed after a remediation action
	// (e.g. resolution signal after a rollback).
	SignalRelationOutcome SignalRelation = "outcome"
)

// ResourceRelation describes how a topology resource relates to a case.
type ResourceRelation string

const (
	// ResourceRelationPrimary: the resource the case is about (the case_key
	// is derived from this resource's UID).
	ResourceRelationPrimary ResourceRelation = "primary"
	// ResourceRelationUpstream: a resource that owns/routes-to/scales the
	// primary resource (e.g. Deployment owning a crashing Pod).
	ResourceRelationUpstream ResourceRelation = "upstream"
	// ResourceRelationDownstream: a resource owned/backed-by the primary
	// (e.g. Pod backed by a Service).
	ResourceRelationDownstream ResourceRelation = "downstream"
	// ResourceRelationRelated: a resource connected via a non-hierarchical
	// edge (RunsOn, Mounts, ProtectedBy).
	ResourceRelationRelated ResourceRelation = "related"
)

// ResourceCitation mirrors signal.ResourceCitation and topology.ResourceCitation
// so the correlation package stays independent of those packages. The primary
// resource key is cluster_id + kind + UID; name-only is marked Incomplete.
type ResourceCitation struct {
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	UID        string `json:"uid,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
}

// EvidenceRef points at a stable, redacted evidence snapshot. Mirrors
// signal.EvidenceRef and topology.EvidenceRef.
type EvidenceRef struct {
	Kind        string `json:"kind"`
	ID          int64  `json:"id"`
	ContentHash string `json:"content_hash,omitempty"`
}

// Factor is one deterministic factor that contributed to a case or candidate.
// Factors are explicit, versioned and replayable — never a black-box score.
type Factor struct {
	// Kind identifies the factor type: "same_uid", "topology_distance",
	// "time_distance", "change_symptom_rule", "signal_freshness",
	// "signal_completeness", "diagnosis_match", "contradicting_signal".
	Kind string `json:"kind"`
	// Value is the factor's computed value (e.g. "3" for topology distance,
	// "120s" for time distance, "match" for diagnosis match).
	Value string `json:"value"`
	// Weight is the factor's contribution weight (0..1). The catalog rule
	// fixes the weight; the engine never adjusts it at runtime.
	Weight float64 `json:"weight"`
	// Evidence references the signals/topology/diagnosis rows that support
	// this factor.
	Evidence []EvidenceRef `json:"evidence,omitempty"`
}

// Case is the aggregation unit. One active case per deterministic case_key.
// N duplicate symptoms form one active case only when case_key matches; all
// occurrences are preserved as SignalLinks. Different UID, authorization
// scope or unrelated topology never merges.
type Case struct {
	ID                   int64                `json:"id"`
	CaseKey              string               `json:"case_key"` // SHA256 over stable identity
	ClusterID            int64                `json:"cluster_id"`
	RuleID               string               `json:"rule_id"` // correlation rule that produced this case
	CorrelationVersion   string               `json:"correlation_version"`
	PrimaryResource      ResourceCitation     `json:"primary_resource"`
	Status               CaseStatus           `json:"status"`
	Confidence           ConfidenceClass      `json:"confidence"`
	EvidenceCompleteness EvidenceCompleteness `json:"evidence_completeness"`
	Factors              []Factor             `json:"factors"`
	FirstObservedAt      time.Time            `json:"first_observed_at"`
	LastObservedAt       time.Time            `json:"last_observed_at"`
	// DiagnosisIDs links the case to existing diagnosis_records rows. The
	// diagnosis record remains the human status/SLA/feedback source of truth.
	DiagnosisIDs []int64 `json:"diagnosis_ids,omitempty"`
	// RootChangeCandidateID is the ID of the top-ranked ChangeCandidate when
	// confidence is confirmed. Nil when confidence is candidate/contradicted/
	// unknown — M43 may rank candidates but cannot promote them.
	RootChangeCandidateID *int64    `json:"root_change_candidate_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// SignalLink links a signal occurrence to a case. One signal occurrence may
// be linked to multiple cases (e.g. a pod-failure signal may be trigger for
// one case and context for another). The relation is recorded per link.
type SignalLink struct {
	ID                 int64          `json:"id"`
	CaseID             int64          `json:"case_id"`
	SignalOccurrenceID int64          `json:"signal_occurrence_id"`
	Relation           SignalRelation `json:"relation"`
	// SignalID copied from the occurrence at link time so queries don't
	// need to join signal_occurrences for the common case.
	SignalID string `json:"signal_id"`
	// Producer copied from the occurrence at link time.
	Producer string `json:"producer"`
	// ObservedAt copied from the occurrence at link time for timeline ordering.
	ObservedAt time.Time `json:"observed_at"`
	// M99-D: data metadata copied from the occurrence at link time so cases
	// expose missing samples (coverage) and data latency (freshness, window)
	// without re-joining signal_occurrences.
	Coverage    string     `json:"coverage,omitempty"`
	Freshness   *time.Time `json:"freshness,omitempty"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ResourceLink links a topology resource to a case. The topology path from
// the primary resource to the linked resource is recorded as a JSON array of
// edge kinds so callers can render the impact graph without re-querying
// topology.
type ResourceLink struct {
	ID       int64            `json:"id"`
	CaseID   int64            `json:"case_id"`
	Resource ResourceCitation `json:"resource"`
	Relation ResourceRelation `json:"relation"`
	// TopologyPath is the sequence of EdgeKind values from the primary
	// resource to this resource (e.g. ["Owns"] for Deployment→Pod,
	// ["RoutesTo","BackedBy"] for Ingress→Service→Pod). Empty for primary.
	TopologyPath []string `json:"topology_path,omitempty"`
	// EdgeIDs references the topology_edges rows that form the path.
	EdgeIDs   []int64   `json:"edge_ids,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ChangeCandidate is a change event that may be the root cause of a case.
// The candidate carries the deterministic factors, confidence class and
// evidence/contradiction refs. Rank is 1-based; rank 1 is the top candidate.
// M43 may reorder candidates by AI confidence but cannot promote a candidate
// to confirmed — only an operator can, via the diagnosis lifecycle.
type ChangeCandidate struct {
	ID            int64           `json:"id"`
	CaseID        int64           `json:"case_id"`
	ChangeEventID int64           `json:"change_event_id"`
	RuleID        string          `json:"rule_id"` // change-symptom rule that matched
	Confidence    ConfidenceClass `json:"confidence"`
	Rank          int             `json:"rank"` // 1 = top candidate
	Factors       []Factor        `json:"factors"`
	// EvidenceRefs points at signals/topology/diagnosis rows supporting the
	// candidate.
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
	// ContradictingRefs points at rows that contradict the candidate (e.g.
	// a resolution signal before the change, or a different UID).
	ContradictingRefs []EvidenceRef `json:"contradicting_refs,omitempty"`
	// ReasonCode is a stable, reviewed code explaining why this candidate
	// was ranked here (e.g. "rollout_precedes_pod_failure",
	// "change_succeeded_but_symptoms_persist", "unrelated_topology").
	ReasonCode string    `json:"reason_code"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CaseFilter bounds case queries.
type CaseFilter struct {
	ClusterID   int64
	Namespace   string
	RuleID      string
	Status      CaseStatus
	Confidence  ConfidenceClass
	PrimaryKind string
	PrimaryUID  string
	StartTime   *time.Time
	EndTime     *time.Time
	Limit       int
}

// CaseListResponse is the bounded paginated response.
type CaseListResponse struct {
	Items     []Case `json:"items"`
	Total     int64  `json:"total"`
	Truncated bool   `json:"truncated,omitempty"`
}

// CaseView is the full case view returned by the API. It includes the case,
// its signal links, resource links and change candidates so callers can
// answer "what is the impact graph and what are the root-cause candidates?"
// in one read.
type CaseView struct {
	Case             Case              `json:"case"`
	SignalLinks      []SignalLink      `json:"signal_links"`
	ResourceLinks    []ResourceLink    `json:"resource_links"`
	ChangeCandidates []ChangeCandidate `json:"change_candidates"`
	GeneratedAt      time.Time         `json:"generated_at"`
}

// CaseTimelineResponse is the bounded timeline of cases for a scope.
type CaseTimelineResponse struct {
	Items     []Case `json:"items"`
	Total     int64  `json:"total"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ActionCandidate is a fixed, read-only action candidate derived from a case.
// It contains a fixed code, eligibility, blocked reasons and exact target
// identity. It is NOT an execute endpoint — M44 reuses existing
// preview/confirmation/idempotency/audit paths.
type ActionCandidate struct {
	// Code is a fixed action code from the M19 controlled-operations
	// catalog (e.g. "deployment.rollback", "deployment.scale",
	// "cronjob.suspend"). The code is server-fixed; callers cannot inject
	// arbitrary actions.
	Code string `json:"code"`
	// Target is the exact resource the action would apply to. Identity is
	// verified at preview time (M44 rechecks UID/resourceVersion).
	Target ResourceCitation `json:"target"`
	// Eligible is true when the action is currently eligible (scope,
	// PDB/replica/blast radius, SLO, maintenance/freeze window, etc.).
	// M44 rechecks eligibility at preview time; this field is a hint.
	Eligible bool `json:"eligible"`
	// BlockedReasons lists the reasons the action is not eligible (e.g.
	// "maintenance_window_active", "pdb_min_available", "insufficient_quota").
	// Empty when Eligible is true.
	BlockedReasons []string `json:"blocked_reasons,omitempty"`
	// SourceCaseID links the action candidate back to the case.
	SourceCaseID int64 `json:"source_case_id"`
	// SourceCandidateID optionally links to the change candidate that
	// suggested this action.
	SourceCandidateID *int64 `json:"source_candidate_id,omitempty"`
}

// ActionCandidateListResponse is the bounded action-candidate response.
type ActionCandidateListResponse struct {
	Items []ActionCandidate `json:"items"`
	Total int64             `json:"total"`
}

// Bounds enforced by the catalog and migration CHECK constraints.
const (
	MaxCaseFactors          = 32
	MaxChangeCandidates     = 10
	MaxSignalLinksPerCase   = 100
	MaxResourceLinksPerCase = 50
	MaxTopologyPathLen      = 8
	MaxReasonCodeLength     = 128
	MaxRuleIDLength         = 128
	MaxCaseKeyLength        = 64 // SHA256 hex = 64 chars
	MinTimeDistanceSecs     = 0
	MaxTimeDistanceSecs     = 24 * 3600 // 24h — beyond this, temporal correlation is not causal
)

// CorrelationResult is the engine's output for one trigger signal + rule
// evaluation. The engine is pure; the service layer persists the result.
// A single Correlate call may produce multiple results (one per matching
// rule per trigger signal).
type CorrelationResult struct {
	Case             Case
	ChangeCandidates []ChangeCandidate
	// SignalLinks are the signal→case relations derived during correlation.
	// The trigger signal is always a SignalRelationTrigger link; context
	// signals (co-observed) are added by the service layer.
	SignalLinks []SignalLink
	// ResourceLinks are the topology resources linked to the case. The
	// primary resource is always a ResourceRelationPrimary link; upstream/
	// downstream resources are derived from the topology path.
	ResourceLinks []ResourceLink
}
