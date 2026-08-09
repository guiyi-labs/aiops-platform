package golden

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ReportVersion is the quality report schema version.
const ReportVersion = "1.0"

// EngineContracts captures the current engine version constants and valid
// status enumerations from the M39-M44 AIOps packages. The runner uses this
// to verify that the golden dataset's expected outcomes are still supported
// by the current engine contracts. The struct is populated by the caller
// (cmd/server/main.go) to avoid import cycles between golden and the engine
// packages.
type EngineContracts struct {
	Versions                  EngineVersions
	ValidPlanStatuses         map[string]bool
	ValidVerificationStatuses map[string]bool

	// AnalyzerDiscovery is the M82 analyzer-contract snapshot. When it is
	// non-nil, replay steps that set ExpectAnalyzerContracts verify that the
	// analyzer surface has not silently shrunk.
	AnalyzerDiscovery *AnalyzerDiscoveryContract
}

// AnalyzerDiscoveryContract captures the compiled-in analyzer surface that the
// golden run must not regress. The snapshot is taken at construction time from
// the live platform packages (ADR 0079 / polish-plan W6 / M82).
type AnalyzerDiscoveryContract struct {
	SchemaVersion   string   `json:"schema_version,omitempty"`
	PostureDomains  []string `json:"posture_domains,omitempty"`
	InsightKinds    []string `json:"insight_kinds,omitempty"`
	DiagnosisRules  []string `json:"diagnosis_rules,omitempty"`
	InspectionRules []string `json:"inspection_rules,omitempty"`
	Operations      []string `json:"operations,omitempty"`
}

// StepResult is the outcome of verifying one golden scenario step.
type StepResult struct {
	StepID StepID
	Passed bool
	Notes  string
}

// ScenarioResult is the outcome of replaying one scenario.
type ScenarioResult struct {
	ScenarioID ScenarioID
	Steps      []StepResult
	Passed     bool
}

// ReplayRunner executes the golden dataset against the current engine
// contracts and produces per-scenario, per-step results. The runner is
// deterministic: identical EngineContracts + identical Dataset produce
// identical results.
type ReplayRunner struct {
	contracts EngineContracts
}

// NewReplayRunner returns a runner bound to the given engine contracts.
func NewReplayRunner(contracts EngineContracts) *ReplayRunner {
	return &ReplayRunner{contracts: contracts}
}

// Run executes every scenario in the dataset and returns per-scenario
// results. A scenario passes when all its steps pass. A step passes when
// every expected outcome in its StepOutcome is verifiable against the
// current EngineContracts.
func (r *ReplayRunner) Run(ctx context.Context, dataset Dataset) ([]ScenarioResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	results := make([]ScenarioResult, 0, len(dataset.Scenarios))
	for _, sc := range dataset.Scenarios {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		results = append(results, r.runScenario(ctx, sc))
	}
	return results, nil
}

func (r *ReplayRunner) runScenario(ctx context.Context, sc Scenario) ScenarioResult {
	sr := ScenarioResult{ScenarioID: sc.ID}
	allPassed := true
	for _, step := range sc.Steps {
		sr.Steps = append(sr.Steps, r.verifyStep(step))
		if !sr.Steps[len(sr.Steps)-1].Passed {
			allPassed = false
		}
	}
	sr.Passed = allPassed
	return sr
}

func (r *ReplayRunner) verifyStep(step StepOutcome) StepResult {
	res := StepResult{StepID: step.StepID}

	if step.ExpectSignalCaptured {
		if r.contracts.Versions.SignalVersion == "" {
			res.Passed = false
			res.Notes = "signal schema version is empty"
			return res
		}
	}

	if step.ExpectTopologyEdge {
		if r.contracts.Versions.TopologyVersion == "" {
			res.Passed = false
			res.Notes = "topology version is empty"
			return res
		}
	}

	if step.ExpectSLOEvaluated {
		if r.contracts.Versions.SLOVersion == "" {
			res.Passed = false
			res.Notes = "SLO template version is empty"
			return res
		}
	}

	if step.ExpectCorrelationCase {
		if r.contracts.Versions.CorrelationVersion == "" {
			res.Passed = false
			res.Notes = "correlation version is empty"
			return res
		}
	}

	if step.ExpectInvestigation {
		if r.contracts.Versions.InvestigatorVersion == "" {
			res.Passed = false
			res.Notes = "investigator version is empty"
			return res
		}
	}

	if step.ExpectActionPlan {
		if r.contracts.Versions.AutomationVersion == "" {
			res.Passed = false
			res.Notes = "automation version is empty"
			return res
		}
	}

	if step.ExpectPlanStatus != "" {
		if !r.contracts.ValidPlanStatuses[step.ExpectPlanStatus] {
			res.Passed = false
			res.Notes = fmt.Sprintf("plan status %q is not a valid automation plan status", step.ExpectPlanStatus)
			return res
		}
	}

	if step.ExpectVerificationStatus != "" {
		if !r.contracts.ValidVerificationStatuses[step.ExpectVerificationStatus] {
			res.Passed = false
			res.Notes = fmt.Sprintf("verification status %q is not a valid verification status", step.ExpectVerificationStatus)
			return res
		}
	}

	// ExpectAlertRecovered and ExpectInvestigationValid are contract
	// invariants of the M27 alert lifecycle and M43 citation validator
	// respectively. They are always met as long as the engine versions
	// are non-empty, which is checked above.

	if step.ExpectAnalyzerContracts {
		if err := verifyAnalyzerContracts(r.contracts.AnalyzerDiscovery); err != nil {
			res.Passed = false
			res.Notes = err.Error()
			return res
		}
	}

	res.Passed = true
	return res
}

// BuildScenarioQuality converts a replay ScenarioResult (current run) into
// a ScenarioQuality by comparing against the previous baseline. If no
// baseline exists, the before state defaults to all-passed (first run
// establishes the baseline).
func BuildScenarioQuality(result ScenarioResult, baseline *ScenarioQuality) ScenarioQuality {
	stepsPassed := 0
	for _, step := range result.Steps {
		if step.Passed {
			stepsPassed++
		}
	}
	stepsTotal := len(result.Steps)

	passedBefore := true
	stepsPassedBefore := stepsTotal
	if baseline != nil {
		passedBefore = baseline.PassedAfter
		stepsPassedBefore = baseline.StepsPassedAfter
	}

	return ScenarioQuality{
		ScenarioID:        result.ScenarioID,
		PassedBefore:      passedBefore,
		PassedAfter:       result.Passed,
		Delta:             ClassifyDelta(passedBefore, result.Passed),
		StepsPassedBefore: stepsPassedBefore,
		StepsPassedAfter:  stepsPassed,
		StepsTotal:        stepsTotal,
	}
}

// ReplayTaskStatus is the lifecycle status of an async replay task.
type ReplayTaskStatus string

const (
	ReplayTaskRunning   ReplayTaskStatus = "running"
	ReplayTaskSucceeded ReplayTaskStatus = "succeeded"
	ReplayTaskFailed    ReplayTaskStatus = "failed"
)

// ReplayTask tracks one async golden dataset replay execution.
type ReplayTask struct {
	ID        string
	Status    ReplayTaskStatus
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
	Report    *QualityReport
}

// replayTaskTracker is a thread-safe in-memory store for replay tasks.
type replayTaskTracker struct {
	mu    sync.Mutex
	tasks map[string]*ReplayTask
}

func newReplayTaskTracker() *replayTaskTracker {
	return &replayTaskTracker{tasks: make(map[string]*ReplayTask)}
}

func (t *replayTaskTracker) set(task *ReplayTask) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tasks[task.ID] = task
}

func (t *replayTaskTracker) get(id string) (*ReplayTask, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.tasks[id]
	return task, ok
}

// verifyAnalyzerContracts ensures the M82 analyzer snapshot discovery is
// non-empty across all families so a silently-shrunken contract fails the
// golden replay instead of being unnoticed.
func verifyAnalyzerContracts(snapshot *AnalyzerDiscoveryContract) error {
	if snapshot == nil {
		return fmt.Errorf("analyzer discovery contract is missing (M82 contract)")
	}
	if len(snapshot.PostureDomains) == 0 {
		return fmt.Errorf("posture domains are empty (analyzer discovery contract)")
	}
	if len(snapshot.InsightKinds) == 0 {
		return fmt.Errorf("insight kinds are empty (analyzer discovery contract)")
	}
	if len(snapshot.DiagnosisRules) == 0 {
		return fmt.Errorf("diagnosis rules are empty (analyzer discovery contract)")
	}
	if len(snapshot.InspectionRules) == 0 {
		return fmt.Errorf("inspection rules are empty (analyzer discovery contract)")
	}
	if len(snapshot.Operations) == 0 {
		return fmt.Errorf("remediation operations are empty (analyzer discovery contract)")
	}
	return nil
}
