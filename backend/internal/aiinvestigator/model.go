// Package aiinvestigator implements the M43 cited and evaluated AI
// investigator. It extends the existing per-diagnosis cited explanation
// (aiexplain) into a case-level investigator bound to M42 correlation cases.
//
// M43 is NOT a general chat console. It produces a structured, cited
// investigation output for one correlation case:
//   - summary / impact
//   - hypotheses[]: claim, confidence, evidence_ids, disconfirming_evidence, next_checks
//   - recommended_runbook_id
//   - uncertainties[]
//
// Invariants:
//   - Every factual claim, impact statement and hypothesis cites an authorized
//     evidence ID. Fabricated, nonexistent or unauthorized citations reject
//     the entire output.
//   - The model cannot upgrade a candidate to confirmed cause, and cannot
//     modify alert/diagnosis severity, owner or state.
//   - Only server-fixed read-only tools may be invoked; no Kubernetes URL,
//     kubectl, SQL, PromQL, LogQL or raw provider query.
//   - Logs, Events, labels, annotations and manifests are untrusted data and
//     cannot alter system/tool instructions.
//   - Model recommendations are runbook IDs already declared eligible by the
//     deterministic Action Catalog (M42 ListActionCandidates).
//   - AI outage, budget exhaustion or schema/citation failure leaves
//     deterministic investigation and manual actions available.
package aiinvestigator

import "time"

// InvestigatorVersion is the schema version of the investigator contract.
// Bumped when the prompt template, evidence schema, runbook catalog or
// citation validation changes. Identical evidence + prompt + runbook +
// investigator versions reproduce identical investigations.
const InvestigatorVersion = "1.0"

// HypothesisConfidence classifies how well the evidence supports a hypothesis.
// The AI may rank hypotheses but cannot promote a correlation candidate to
// confirmed cause — only an operator reviewing the diagnosis record can.
type HypothesisConfidence string

const (
	// HypothesisHigh: multiple independent evidence refs support the claim.
	HypothesisHigh HypothesisConfidence = "high"
	// HypothesisMedium: evidence partially supports the claim.
	HypothesisMedium HypothesisConfidence = "medium"
	// HypothesisLow: evidence is weak or contradictory.
	HypothesisLow HypothesisConfidence = "low"
)

// InvestigationStatus is the lifecycle status of an investigation.
type InvestigationStatus string

const (
	// InvestigationStatusCompleted: the AI produced a valid cited output.
	InvestigationStatusCompleted InvestigationStatus = "completed"
	// InvestigationStatusFailed: the AI call failed (provider error, budget
	// exhausted, invalid output, citation rejection). Deterministic
	// investigation remains available.
	InvestigationStatusFailed InvestigationStatus = "failed"
	// InvestigationStatusStale: superseded by a newer investigation for the
	// same case. Retained for audit.
	InvestigationStatusStale InvestigationStatus = "stale"
)

// EvidenceKind identifies the source type of an evidence ref. The investigator
// only accepts evidence kinds that the platform has authorized — it never
// accepts free-form URLs, kubectl output, or raw provider queries.
type EvidenceKind string

const (
	EvidenceKindSignalOccurrence EvidenceKind = "signal_occurrence"
	EvidenceKindTopologyEdge     EvidenceKind = "topology_edge"
	EvidenceKindChangeEvent      EvidenceKind = "change_event"
	EvidenceKindDiagnosisRecord  EvidenceKind = "diagnosis_record"
	EvidenceKindSLOEvaluation    EvidenceKind = "slo_evaluation"
	EvidenceKindCorrelationCase  EvidenceKind = "correlation_case"
	EvidenceKindChangeCandidate  EvidenceKind = "change_candidate"
)

// EvidenceRef points at a stable, redacted evidence snapshot authorized for
// this investigation. The prompt builder assembles the authorized set; the
// validator rejects any citation outside this set.
type EvidenceRef struct {
	Kind EvidenceKind `json:"kind"`
	ID   int64        `json:"id"`
	// ContentHash is the redacted snapshot hash so the investigation is
	// replayable even if the underlying row is later pruned.
	ContentHash string `json:"content_hash,omitempty"`
}

// Hypothesis is one AI-generated root-cause hypothesis for a case. Every
// claim cites authorized evidence IDs; disconfirming evidence is required
// when present so the AI cannot hide contradictions.
type Hypothesis struct {
	Claim                 string               `json:"claim"`
	Confidence            HypothesisConfidence `json:"confidence"`
	EvidenceIDs           []EvidenceRef        `json:"evidence_ids"`
	DisconfirmingEvidence []EvidenceRef        `json:"disconfirming_evidence,omitempty"`
	NextChecks            []string             `json:"next_checks,omitempty"`
}

// Investigation is the structured, cited output for one correlation case.
// One active investigation per case; newer investigations mark older ones
// stale. The investigation never modifies the case, diagnosis or alert —
// it is a read-only advisory.
type Investigation struct {
	ID                  int64               `json:"id"`
	CaseID              int64               `json:"case_id"`
	InvestigationKey    string              `json:"investigation_key"` // SHA256 over (case_id + investigator_version + prompt_hash)
	InvestigatorVersion string              `json:"investigator_version"`
	Actor               ActorRef            `json:"actor"`
	Provider            string              `json:"provider"`
	Model               string              `json:"model"`
	ProviderResponseID  string              `json:"provider_response_id,omitempty"`
	Status              InvestigationStatus `json:"status"`
	Summary             string              `json:"summary"`
	Impact              string              `json:"impact"`
	Hypotheses          []Hypothesis        `json:"hypotheses"`
	// RecommendedRunbookID is a fixed runbook ID from the server-owned
	// catalog. Must be eligible per the M42 Action Catalog at generation
	// time; the investigator rechecks eligibility before persisting.
	RecommendedRunbookID string     `json:"recommended_runbook_id,omitempty"`
	Uncertainties        []string   `json:"uncertainties,omitempty"`
	Citations            []Citation `json:"citations"`
	InputTokens          int        `json:"input_tokens"`
	OutputTokens         int        `json:"output_tokens"`
	// FailureReason is set when Status == failed (e.g.
	// "provider_error", "budget_exhausted", "invalid_output",
	// "citation_rejected", "runbook_not_eligible").
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ActorRef mirrors aiexplain.ActorRef so the investigator stays independent
// of the aiexplain package while preserving the same audit shape.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Citation mirrors aiexplain.Citation. Every factual claim, impact statement
// and hypothesis must cite an authorized evidence ID via this structure.
type Citation struct {
	EvidenceRef EvidenceRef `json:"evidence_ref"`
	Claim       string      `json:"claim"`
}

// InvestigationFilter bounds investigation queries.
type InvestigationFilter struct {
	CaseID int64
	Status InvestigationStatus
	Limit  int
}

// InvestigationListResponse is the bounded paginated response.
type InvestigationListResponse struct {
	Items     []Investigation `json:"items"`
	Total     int64           `json:"total"`
	Truncated bool            `json:"truncated,omitempty"`
}

// QualityReport summarizes investigation quality for offline evaluation. It
// never self-modifies rules, prompts or policy online.
type QualityReport struct {
	TotalInvestigations     int64     `json:"total_investigations"`
	CompletedInvestigations int64     `json:"completed_investigations"`
	FailedInvestigations    int64     `json:"failed_investigations"`
	CitationValidityRate    float64   `json:"citation_validity_rate"`
	UnsupportedClaimRate    float64   `json:"unsupported_claim_rate"`
	EligibleRunbookRate     float64   `json:"eligible_runbook_rate"`
	GeneratedAt             time.Time `json:"generated_at"`
}

// Bounds enforced by the catalog and migration CHECK constraints.
const (
	MaxHypothesesPerInvestigation = 8
	MaxCitationsPerInvestigation  = 64
	MaxUncertainties              = 16
	MaxNextChecksPerHypothesis    = 8
	MaxRunbookIDLength            = 128
	MaxInvestigationKeyLength     = 64 // SHA256 hex = 64 chars
)

// Prompt is the typed prompt sent to the provider. The prompt builder
// assembles the authorized evidence set; the validator rejects any citation
// outside EvidenceRefs.
type Prompt struct {
	System       string
	Input        string
	EvidenceRefs map[string]EvidenceRef // evidence_id (string form) → ref
}

// ProviderResult is the raw provider output before validation.
type ProviderResult struct {
	Provider             string
	Model                string
	ProviderResponseID   string
	Summary              string
	Impact               string
	Hypotheses           []Hypothesis
	RecommendedRunbookID string
	Uncertainties        []string
	Citations            []Citation
	InputTokens          int
	OutputTokens         int
}
