// Package golden is the versioned AIOps golden dataset and quality report
// bound to the release revision. It is the M45 contract: the same dataset
// is replayed across M39-M45 milestones, and any rule/correlation/prompt/
// model/provider/evidence-schema change must produce a machine-readable
// before/after quality report rather than silently replacing the baseline.
//
// The golden dataset is deterministic: identical scenario inputs + identical
// package versions (M39 signal, M40 topology, M41 SLO, M42 correlation,
// M43 aiinvestigator, M44 automation) reproduce identical outcomes. The
// dataset is versioned via DatasetVersion; bumping the version is a
// contract change that requires a quality report.
//
// The dataset does not execute against real Kubernetes, Prometheus, Loki
// or AI providers. Each scenario carries the expected intermediate and
// final states; the test suite verifies that the deterministic engines
// (correlation, aiinvestigator validator, automation gate evaluator)
// reproduce those states from the scenario inputs.
package golden

// DatasetVersion is the version of the golden dataset. Bumped when a
// scenario is added, removed, or its expected outcomes change. A bump
// requires a quality report (before/after) per the M45 plan.
const DatasetVersion = "1.0"

// ScenarioVersion is the per-scenario version. Scenarios may evolve
// independently; the dataset version is the max of all scenario versions.
const ScenarioVersion = "1.0"

// StepID identifies one step of the mandatory end-to-end golden scenario.
// The step IDs are stable across versions so the quality report can
// reference them.
type StepID string

const (
	StepEstablishHealthyService StepID = "establish_healthy_service"
	StepPublishBadImage         StepID = "publish_bad_image"
	StepCaptureSignals          StepID = "capture_signals"
	StepBuildImpactGraph        StepID = "build_impact_graph"
	StepRankCauseCandidate      StepID = "rank_cause_candidate"
	StepGenerateInvestigation   StepID = "generate_investigation"
	StepPreviewApproveRollback  StepID = "preview_approve_rollback"
	StepExecuteVerify           StepID = "execute_verify"
	StepRecoverAlert            StepID = "recover_alert"
	StepCleanup                 StepID = "cleanup"
)

// AllSteps is the ordered list of mandatory golden scenario steps.
var AllSteps = []StepID{
	StepEstablishHealthyService,
	StepPublishBadImage,
	StepCaptureSignals,
	StepBuildImpactGraph,
	StepRankCauseCandidate,
	StepGenerateInvestigation,
	StepPreviewApproveRollback,
	StepExecuteVerify,
	StepRecoverAlert,
	StepCleanup,
}

// ScenarioID identifies one golden scenario.
type ScenarioID string

const (
	// ScenarioMandatoryEndToEnd is the 10-step mandatory golden scenario
	// from the M45 plan: healthy service → bad image → signals → impact
	// graph → cause candidate → AI investigation → preview/approve
	// rollback → execute/verify → recover alert → cleanup.
	ScenarioMandatoryEndToEnd ScenarioID = "mandatory_end_to_end"

	// ScenarioNegativeMisattribution is the negative companion: an
	// unrelated simultaneous change in another Namespace must not be
	// misattributed to the primary cause.
	ScenarioNegativeMisattribution ScenarioID = "negative_misattribution"

	// ScenarioNegativePartialEvidence is the negative companion: when one
	// metrics/log provider is stopped, the case must be partial/unknown
	// rather than falsely healthy or resolved.
	ScenarioNegativePartialEvidence ScenarioID = "negative_partial_evidence"
)

// StepOutcome is the expected outcome of one step in a golden scenario.
// It is the deterministic contract: replaying the scenario with the same
// package versions must reproduce this outcome.
type StepOutcome struct {
	StepID StepID `json:"step_id"`

	// Description is a human-readable summary of what the step verifies.
	Description string `json:"description"`

	// ExpectSignalCaptured is true when the step should capture at least
	// one signal occurrence (M39).
	ExpectSignalCaptured bool `json:"expect_signal_captured,omitempty"`

	// ExpectTopologyEdge is true when the step should produce at least
	// one topology edge (M40).
	ExpectTopologyEdge bool `json:"expect_topology_edge,omitempty"`

	// ExpectSLOEvaluated is true when the step should evaluate the SLO
	// (M41).
	ExpectSLOEvaluated bool `json:"expect_slo_evaluated,omitempty"`

	// ExpectCorrelationCase is true when the step should produce a
	// correlation case (M42).
	ExpectCorrelationCase bool `json:"expect_correlation_case,omitempty"`

	// ExpectInvestigation is true when the step should produce an AI
	// investigation (M43).
	ExpectInvestigation bool `json:"expect_investigation,omitempty"`

	// ExpectInvestigationValid is true when the investigation should
	// pass validation. False means the investigation should be rejected
	// (e.g. fabricated citation).
	ExpectInvestigationValid bool `json:"expect_investigation_valid,omitempty"`

	// ExpectActionPlan is true when the step should produce an action
	// plan (M44).
	ExpectActionPlan bool `json:"expect_action_plan,omitempty"`

	// ExpectPlanStatus is the expected plan status after the step.
	// Empty means the step does not produce a plan.
	ExpectPlanStatus string `json:"expect_plan_status,omitempty"`

	// ExpectVerificationStatus is the expected verification status.
	// Empty means the step does not produce a verification.
	ExpectVerificationStatus string `json:"expect_verification_status,omitempty"`

	// ExpectAlertRecovered is true when the alert should be recovered
	// after this step (M27 lifecycle).
	ExpectAlertRecovered bool `json:"expect_alert_recovered,omitempty"`
}

// Scenario is one golden replay scenario. It is a deterministic
// (description, expected step outcomes) pair. The test suite verifies
// that the deterministic engines reproduce the expected outcomes.
type Scenario struct {
	ID          ScenarioID    `json:"id"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Steps       []StepOutcome `json:"steps"`

	// Negative is true for negative companion scenarios (misattribution,
	// partial/unknown).
	Negative bool `json:"negative,omitempty"`
}

// Dataset is the versioned golden dataset bound to the release revision.
type Dataset struct {
	Version   string     `json:"version"`
	Scenarios []Scenario `json:"scenarios"`
}

// DefaultDataset returns the versioned golden dataset. The dataset is
// immutable; callers must not mutate the returned slice.
func DefaultDataset() Dataset {
	return Dataset{
		Version:   DatasetVersion,
		Scenarios: defaultScenarios(),
	}
}

// defaultScenarios returns the 3 golden scenarios: the mandatory 10-step
// end-to-end scenario plus 2 negative companions.
func defaultScenarios() []Scenario {
	return []Scenario{
		mandatoryEndToEndScenario(),
		negativeMisattributionScenario(),
		negativePartialEvidenceScenario(),
	}
}

// mandatoryEndToEndScenario is the 10-step mandatory golden scenario.
// Each step maps to one stage of the AIOps loop:
//
//  1. Establish a healthy service and SLO.
//  2. Publish a bad image through an accepted fixed operation.
//  3. Capture rollout change, Pod/Event, metric/SLO and optional log signals.
//  4. Build the exact Ingress/Gateway-to-Deployment impact graph and timeline.
//  5. Rank the reviewed rollout as the first deterministic cause candidate.
//  6. Generate an AI investigation whose claims cite only real evidence.
//  7. Preview and approve an exact revision rollback.
//  8. Execute idempotently and verify resource/SLO recovery.
//  9. Recover the alert, record diagnosis/action outcome and notify.
//
// 10. Clean every cluster, provider credential, fixture and artifact.
func mandatoryEndToEndScenario() Scenario {
	return Scenario{
		ID:      ScenarioMandatoryEndToEnd,
		Version: ScenarioVersion,
		Description: "The mandatory 10-step end-to-end golden scenario: " +
			"healthy service → bad image → signals → impact graph → cause " +
			"candidate → AI investigation → preview/approve rollback → " +
			"execute/verify → recover alert → cleanup.",
		Steps: []StepOutcome{
			{
				StepID:               StepEstablishHealthyService,
				Description:          "Establish a healthy service and SLO (M41 healthy state).",
				ExpectSLOEvaluated:   true,
				ExpectSignalCaptured: false,
			},
			{
				StepID:               StepPublishBadImage,
				Description:          "Publish a bad image through an accepted fixed operation (M23 release lifecycle).",
				ExpectSignalCaptured: false,
			},
			{
				StepID:               StepCaptureSignals,
				Description:          "Capture rollout change, Pod/Event, metric/SLO and optional log signals (M39).",
				ExpectSignalCaptured: true,
				ExpectSLOEvaluated:   true,
			},
			{
				StepID:             StepBuildImpactGraph,
				Description:        "Build the exact Ingress/Gateway-to-Deployment impact graph and timeline (M40).",
				ExpectTopologyEdge: true,
			},
			{
				StepID:                StepRankCauseCandidate,
				Description:           "Rank the reviewed rollout as the first deterministic cause candidate (M42).",
				ExpectCorrelationCase: true,
			},
			{
				StepID:                   StepGenerateInvestigation,
				Description:              "Generate an AI investigation whose claims cite only real evidence and disclose uncertainty (M43).",
				ExpectInvestigation:      true,
				ExpectInvestigationValid: true,
			},
			{
				StepID:           StepPreviewApproveRollback,
				Description:      "Preview and approve an exact revision rollback (M44 preview + approve).",
				ExpectActionPlan: true,
				ExpectPlanStatus: "approved",
			},
			{
				StepID:                   StepExecuteVerify,
				Description:              "Execute idempotently and verify resource/SLO recovery (M44 execute + verify).",
				ExpectActionPlan:         true,
				ExpectPlanStatus:         "verified",
				ExpectVerificationStatus: "effective",
			},
			{
				StepID:               StepRecoverAlert,
				Description:          "Recover the alert, record diagnosis/action outcome and send the accepted notification (M27).",
				ExpectAlertRecovered: true,
			},
			{
				StepID:      StepCleanup,
				Description: "Clean every cluster, provider credential, fixture and temporary artifact.",
			},
		},
	}
}

// negativeMisattributionScenario is the negative companion: an unrelated
// simultaneous change in another Namespace must not be misattributed to
// the primary cause. The correlation engine must produce separate cases
// or mark the unrelated change as not-a-candidate.
func negativeMisattributionScenario() Scenario {
	return Scenario{
		ID:      ScenarioNegativeMisattribution,
		Version: ScenarioVersion,
		Description: "Negative companion: an unrelated simultaneous change " +
			"in another Namespace must not be misattributed to the primary " +
			"cause. The correlation engine must produce separate cases or " +
			"exclude the unrelated change from the primary case's candidates.",
		Negative: true,
		Steps: []StepOutcome{
			{
				StepID:               StepCaptureSignals,
				Description:          "Capture signals from both Namespace A (primary) and Namespace B (unrelated).",
				ExpectSignalCaptured: true,
			},
			{
				StepID:                StepRankCauseCandidate,
				Description:           "Correlation produces a case for Namespace A; the Namespace B change is NOT a candidate for the Namespace A case.",
				ExpectCorrelationCase: true,
			},
		},
	}
}

// negativePartialEvidenceScenario is the negative companion: when one
// metrics/log provider is stopped, the case must be partial/unknown
// rather than falsely healthy or resolved. Missing data fails closed
// (M41 invariant) — the case is not falsely healthy.
func negativePartialEvidenceScenario() Scenario {
	return Scenario{
		ID:      ScenarioNegativePartialEvidence,
		Version: ScenarioVersion,
		Description: "Negative companion: when one metrics/log provider is " +
			"stopped, the case must be partial/unknown rather than falsely " +
			"healthy or resolved. Missing data fails closed (M41 invariant).",
		Negative: true,
		Steps: []StepOutcome{
			{
				StepID:               StepCaptureSignals,
				Description:          "Capture partial signals (metrics provider stopped; only Pod/Event signals available).",
				ExpectSignalCaptured: true,
			},
			{
				StepID:                StepRankCauseCandidate,
				Description:           "Correlation produces a case with insufficient evidence completeness (not falsely healthy).",
				ExpectCorrelationCase: true,
			},
			{
				StepID:                   StepGenerateInvestigation,
				Description:              "AI investigation discloses uncertainty and does not claim to confirm root cause.",
				ExpectInvestigation:      true,
				ExpectInvestigationValid: true,
			},
		},
	}
}
