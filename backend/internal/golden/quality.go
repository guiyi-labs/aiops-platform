package golden

import "time"

// QualityReport is the machine-readable before/after quality report
// required by the M45 plan when rule, correlation, prompt, model,
// provider or evidence-schema changes occur. The report is generated
// offline; it never self-modifies rules, prompts or policy online.
//
// The report compares two dataset replays:
//   - Before: the baseline replay (previous dataset version).
//   - After: the candidate replay (new dataset version or changed engine).
//
// Each metric is reported as (before, after, delta) so a reviewer can
// see at a glance whether the change improved, regressed or preserved
// quality.
type QualityReport struct {
	// ReportVersion is the quality report schema version.
	ReportVersion string `json:"report_version"`

	// DatasetVersionBefore is the dataset version used for the baseline.
	DatasetVersionBefore string `json:"dataset_version_before"`

	// DatasetVersionAfter is the dataset version used for the candidate.
	DatasetVersionAfter string `json:"dataset_version_after"`

	// EngineVersionsBefore is the package versions used for the baseline.
	EngineVersionsBefore EngineVersions `json:"engine_versions_before"`

	// EngineVersionsAfter is the package versions used for the candidate.
	EngineVersionsAfter EngineVersions `json:"engine_versions_after"`

	// ScenarioResults compares each scenario's outcome before and after.
	ScenarioResults []ScenarioQuality `json:"scenario_results"`

	// Summary aggregates the scenario results.
	Summary QualitySummary `json:"summary"`

	// GeneratedAt is when the report was produced.
	GeneratedAt time.Time `json:"generated_at"`

	// ChangedComponents lists the components that changed between before
	// and after (e.g. "correlation rule set", "aiinvestigator prompt",
	// "automation gate evaluator"). Empty means no component changed.
	ChangedComponents []string `json:"changed_components,omitempty"`

	// Reviewer is the human reviewer (empty until reviewed).
	Reviewer string `json:"reviewer,omitempty"`

	// Approved is true when the report has been reviewed and approved.
	Approved bool `json:"approved,omitempty"`
}

// EngineVersions records the package versions used in a replay. The
// versions are the package-level constants from each AIOps package.
type EngineVersions struct {
	SignalVersion       string `json:"signal_version"`       // M39
	TopologyVersion     string `json:"topology_version"`     // M40
	SLOVersion          string `json:"slo_version"`          // M41
	CorrelationVersion  string `json:"correlation_version"`  // M42
	InvestigatorVersion string `json:"investigator_version"` // M43
	AutomationVersion   string `json:"automation_version"`   // M44
	VerifierVersion     string `json:"verifier_version"`     // M44 verifier
}

// ScenarioQuality compares one scenario's outcome before and after.
type ScenarioQuality struct {
	ScenarioID ScenarioID `json:"scenario_id"`

	// PassedBefore is true when the scenario passed in the baseline.
	PassedBefore bool `json:"passed_before"`

	// PassedAfter is true when the scenario passed in the candidate.
	PassedAfter bool `json:"passed_after"`

	// Delta is "preserved" (both pass), "improved" (fail→pass),
	// "regressed" (pass→fail), or "unchanged" (both fail).
	Delta string `json:"delta"`

	// StepsPassedBefore is the number of steps that passed in the baseline.
	StepsPassedBefore int `json:"steps_passed_before"`

	// StepsPassedAfter is the number of steps that passed in the candidate.
	StepsPassedAfter int `json:"steps_passed_after"`

	// StepsTotal is the total number of steps in the scenario.
	StepsTotal int `json:"steps_total"`

	// Notes is a human-readable note (e.g. regression reason).
	Notes string `json:"notes,omitempty"`
}

// QualitySummary aggregates the scenario results.
type QualitySummary struct {
	TotalScenarios   int `json:"total_scenarios"`
	PassedBefore     int `json:"passed_before"`
	PassedAfter      int `json:"passed_after"`
	Improved         int `json:"improved"`
	Regressed        int `json:"regressed"`
	Preserved        int `json:"preserved"`
	Unchanged        int `json:"unchanged"`
	TotalStepsBefore int `json:"total_steps_before"`
	TotalStepsAfter  int `json:"total_steps_after"`
	TotalSteps       int `json:"total_steps"`
}

// DatasetMigrationHint returns a human-readable upgrade hint when the loaded
// dataset version is older than the current compiled-in DatasetVersion. Older
// snapshots remain readable (backward compatible), and the hint tells the
// reviewer which dataset generation produced them and how to interpret the
// unified evidence model introduced by M95.
func DatasetMigrationHint(loadedVersion string) string {
	if loadedVersion == "" {
		return "quality report missing dataset version; treat as legacy pre-M82 snapshot"
	}
	if loadedVersion == DatasetVersion {
		return ""
	}
	switch loadedVersion {
	case "1.1":
		return "snapshot was generated with the M82-M94 dataset (v1.1); findings legacy, unified v2 model added in M95"
	case "1.0":
		return "snapshot was generated with the pre-M82 dataset (v1.0); findings legacy, unified v2 model added in M95"
	default:
		return "snapshot dataset version " + loadedVersion + " predates the current " + DatasetVersion + " unified evidence model; findings remain readable via v1 contract"
	}
}

// ClassifyDelta returns the delta classification for a scenario.
func ClassifyDelta(passedBefore, passedAfter bool) string {
	switch {
	case passedBefore && passedAfter:
		return "preserved"
	case !passedBefore && passedAfter:
		return "improved"
	case passedBefore && !passedAfter:
		return "regressed"
	default:
		return "unchanged"
	}
}

// Summarize produces the QualitySummary from scenario results.
func Summarize(results []ScenarioQuality) QualitySummary {
	s := QualitySummary{
		TotalScenarios: len(results),
	}
	for _, r := range results {
		s.TotalStepsBefore += r.StepsPassedBefore
		s.TotalStepsAfter += r.StepsPassedAfter
		s.TotalSteps += r.StepsTotal
		if r.PassedBefore {
			s.PassedBefore++
		}
		if r.PassedAfter {
			s.PassedAfter++
		}
		switch r.Delta {
		case "improved":
			s.Improved++
		case "regressed":
			s.Regressed++
		case "preserved":
			s.Preserved++
		case "unchanged":
			s.Unchanged++
		}
	}
	return s
}
