package correlation

import "fmt"

// RuleDescriptor describes one server-owned correlation rule. The catalog is
// the single source of truth for which rules exist, what signals they trigger
// on, what change kinds they correlate with, and what factors they require.
// Adding a rule is a contract change, not a runtime configuration.
type RuleDescriptor struct {
	RuleID               string
	TriggerSignals       []string // signal IDs that trigger this rule (e.g. diag.pod.image_pull_backoff.v1)
	ChangeKinds          []string // change_event kinds this rule correlates with (e.g. ["promotion","rollout"])
	PrimaryKind          string   // the primary resource kind for cases produced by this rule
	TimeWindowSecs       int      // max seconds between change start and symptom observation
	RequiredFactors      []string // factor kinds that must all match for ConfidenceConfirmed
	ContradictingFactors []string // factor kinds that downgrade to Contradicted/Candidate
	ReasonCode           string   // stable reason code for confirmed cases
}

// catalog is the compiled correlation rule catalog. Lookups via LookupRule
// fail closed for unlisted rules.
var catalog = map[string]RuleDescriptor{
	"correlation.rollout_causes_pod_failure.v1": {
		RuleID:               "correlation.rollout_causes_pod_failure.v1",
		TriggerSignals:       []string{"diag.pod.image_pull_backoff.v1", "diag.pod.crash_loop_backoff.v1", "diag.pod.oom_killed.v1"},
		ChangeKinds:          []string{"promotion", "rollout"},
		PrimaryKind:          "Pod",
		TimeWindowSecs:       3600, // 1h
		RequiredFactors:      []string{"topology_distance", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "rollout_precedes_pod_failure",
	},
	"correlation.rollout_causes_unavailable_deployment.v1": {
		RuleID:               "correlation.rollout_causes_unavailable_deployment.v1",
		TriggerSignals:       []string{"diag.deployment.replicas_unavailable.v1"},
		ChangeKinds:          []string{"promotion", "rollout"},
		PrimaryKind:          "Deployment",
		TimeWindowSecs:       3600,
		RequiredFactors:      []string{"same_uid", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "rollout_precedes_replicas_unavailable",
	},
	"correlation.rollout_causes_no_endpoints.v1": {
		RuleID:               "correlation.rollout_causes_no_endpoints.v1",
		TriggerSignals:       []string{"diag.service.no_ready_endpoints.v1"},
		ChangeKinds:          []string{"promotion", "rollout"},
		PrimaryKind:          "Service",
		TimeWindowSecs:       3600,
		RequiredFactors:      []string{"topology_distance", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "rollout_precedes_endpoint_loss",
	},
	"correlation.maintenance_causes_node_failure.v1": {
		RuleID:               "correlation.maintenance_causes_node_failure.v1",
		TriggerSignals:       []string{"diag.node.not_ready.v1", "diag.node.pressure.v1"},
		ChangeKinds:          []string{"maintenance"},
		PrimaryKind:          "Node",
		TimeWindowSecs:       7200, // 2h — maintenance may take longer to show symptoms
		RequiredFactors:      []string{"same_uid", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "maintenance_precedes_node_failure",
	},
	"correlation.storage_change_causes_pvc_pending.v1": {
		RuleID:               "correlation.storage_change_causes_pvc_pending.v1",
		TriggerSignals:       []string{"diag.persistentvolumeclaim.pending.v1"},
		ChangeKinds:          []string{"promotion", "maintenance"},
		PrimaryKind:          "PersistentVolumeClaim",
		TimeWindowSecs:       7200,
		RequiredFactors:      []string{"same_uid", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "storage_change_precedes_pvc_pending",
	},
	"correlation.rollout_causes_metric_breach.v1": {
		RuleID:               "correlation.rollout_causes_metric_breach.v1",
		TriggerSignals:       []string{"metric.sustained_breach.v1"},
		ChangeKinds:          []string{"promotion", "rollout"},
		PrimaryKind:          "Pod",
		TimeWindowSecs:       3600,
		RequiredFactors:      []string{"topology_distance", "time_distance", "change_symptom_rule"},
		ContradictingFactors: []string{"contradicting_signal"},
		ReasonCode:           "rollout_precedes_metric_breach",
	},
}

// LookupRule returns the descriptor for a rule. ok=false when the rule is not
// in the catalog (fail-closed).
func LookupRule(ruleID string) (RuleDescriptor, bool) {
	d, ok := catalog[ruleID]
	return d, ok
}

// AllRules returns every registered rule. Used by the catalog API.
func AllRules() []RuleDescriptor {
	out := make([]RuleDescriptor, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, d)
	}
	return out
}

// RulesForTriggerSignal returns all rules triggered by the given signal ID.
// A signal may trigger multiple rules (e.g. a pod-failure signal triggers
// both pod-failure and metric-breach rules).
func RulesForTriggerSignal(signalID string) []RuleDescriptor {
	out := []RuleDescriptor{}
	for _, d := range catalog {
		for _, s := range d.TriggerSignals {
			if s == signalID {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// ValidateRule checks that a rule descriptor is internally consistent. Used
// at catalog compile time and in tests.
func ValidateRule(d RuleDescriptor) error {
	if d.RuleID == "" {
		return fmt.Errorf("rule_id is required")
	}
	if len(d.TriggerSignals) == 0 {
		return fmt.Errorf("rule %s must have at least one trigger signal", d.RuleID)
	}
	if len(d.ChangeKinds) == 0 {
		return fmt.Errorf("rule %s must have at least one change kind", d.RuleID)
	}
	if d.PrimaryKind == "" {
		return fmt.Errorf("rule %s must have a primary kind", d.RuleID)
	}
	if d.TimeWindowSecs <= 0 || d.TimeWindowSecs > MaxTimeDistanceSecs {
		return fmt.Errorf("rule %s time_window_secs must be in (0, %d]", d.RuleID, MaxTimeDistanceSecs)
	}
	if len(d.RequiredFactors) == 0 {
		return fmt.Errorf("rule %s must have at least one required factor", d.RuleID)
	}
	if d.ReasonCode == "" {
		return fmt.Errorf("rule %s must have a reason code", d.RuleID)
	}
	return nil
}
