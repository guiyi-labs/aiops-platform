package slo

import "fmt"

// Catalog enumerates the server-owned SLI templates and validates SLO
// definitions against their contracts. The catalog is the single source of
// truth for which templates exist, what they require and which missing-data
// policies they admit. Adding a template is a contract change, not a runtime
// configuration.

// TemplateDescriptor describes one server-owned SLI template.
type TemplateDescriptor struct {
	Template          SLITemplate
	RequiresTraffic   bool // true for request_* templates; false for workload_readiness
	AllowsFailOpen    bool // workload_readiness may fail_open; request_* never
	RequiresLatencyMs bool // true for request_latency_target_ratio
}

// catalog is the compiled template catalog. Lookups via LookupTemplate fail
// closed for unlisted templates.
var catalog = map[SLITemplate]TemplateDescriptor{
	TemplateRequestSuccessRatio: {
		Template:          TemplateRequestSuccessRatio,
		RequiresTraffic:   true,
		AllowsFailOpen:    false,
		RequiresLatencyMs: false,
	},
	TemplateRequestLatencyTargetRatio: {
		Template:          TemplateRequestLatencyTargetRatio,
		RequiresTraffic:   true,
		AllowsFailOpen:    false,
		RequiresLatencyMs: true,
	},
	TemplateWorkloadReadiness: {
		Template:          TemplateWorkloadReadiness,
		RequiresTraffic:   false,
		AllowsFailOpen:    true,
		RequiresLatencyMs: false,
	},
}

// LookupTemplate returns the descriptor for a template. ok=false when the
// template is not in the catalog (fail-closed).
func LookupTemplate(t SLITemplate) (TemplateDescriptor, bool) {
	d, ok := catalog[t]
	return d, ok
}

// AllTemplates returns every registered template. Used by the catalog API.
func AllTemplates() []TemplateDescriptor {
	out := make([]TemplateDescriptor, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, d)
	}
	return out
}

// ValidateDefinition validates an SLO definition against the catalog and the
// M41 bounds. Returns an error describing the first violation. This is the
// single validation entry point used by Create, Patch and the HTTP layer.
func ValidateDefinition(def *Definition) error {
	if def == nil {
		return fmt.Errorf("slo definition is nil")
	}
	if def.ClusterID <= 0 {
		return fmt.Errorf("cluster_id must be a positive integer")
	}
	if def.Service.Kind == "" || def.Service.Name == "" {
		return fmt.Errorf("service kind and name are required")
	}
	if len(def.Service.Name) > MaxServiceNameLength {
		return fmt.Errorf("service name exceeds %d chars", MaxServiceNameLength)
	}
	if def.Service.Namespace != "" && len(def.Service.Namespace) > MaxNamespaceLength {
		return fmt.Errorf("namespace exceeds %d chars", MaxNamespaceLength)
	}
	desc, ok := LookupTemplate(def.Template)
	if !ok {
		return fmt.Errorf("unsupported SLI template: %s", def.Template)
	}
	if def.Objective < MinObjective || def.Objective > MaxObjective {
		return fmt.Errorf("objective must be in [%g, %g]", MinObjective, MaxObjective)
	}
	// A 1.0 objective means zero error budget; burn rate is undefined. We
	// admit it but evaluations will report StateHealthy only when
	// good == total exactly, otherwise StateBreached.
	if def.RollingWindowSeconds < MinRollingWindowSeconds || def.RollingWindowSeconds > MaxRollingWindowSeconds {
		return fmt.Errorf("rolling_window_seconds must be in [%d, %d]", MinRollingWindowSeconds, MaxRollingWindowSeconds)
	}
	switch def.MissingDataPolicy {
	case MissingDataUnavailable, MissingDataFailOpen:
	default:
		return fmt.Errorf("unsupported missing_data_policy: %s", def.MissingDataPolicy)
	}
	if def.MissingDataPolicy == MissingDataFailOpen && !desc.AllowsFailOpen {
		return fmt.Errorf("template %s does not allow fail_open missing-data policy", def.Template)
	}
	if desc.RequiresLatencyMs {
		if def.LatencyThresholdMs < MinLatencyThresholdMs || def.LatencyThresholdMs > MaxLatencyThresholdMs {
			return fmt.Errorf("latency_threshold_ms must be in [%d, %d] for template %s", MinLatencyThresholdMs, MaxLatencyThresholdMs, def.Template)
		}
	} else if def.LatencyThresholdMs != 0 {
		return fmt.Errorf("latency_threshold_ms must be 0 for template %s", def.Template)
	}
	if def.FastBurnRate < MinBurnRate {
		return fmt.Errorf("fast_burn_rate must be >= %g", MinBurnRate)
	}
	if def.SlowBurnRate < MinBurnRate {
		return fmt.Errorf("slow_burn_rate must be >= %g", MinBurnRate)
	}
	if def.FastBurnWindowSeconds < MinBurnWindowSeconds || def.FastBurnWindowSeconds > MaxBurnWindowSeconds {
		return fmt.Errorf("fast_burn_window_seconds must be in [%d, %d]", MinBurnWindowSeconds, MaxBurnWindowSeconds)
	}
	if def.SlowBurnWindowSeconds < MinBurnWindowSeconds || def.SlowBurnWindowSeconds > MaxBurnWindowSeconds {
		return fmt.Errorf("slow_burn_window_seconds must be in [%d, %d]", MinBurnWindowSeconds, MaxBurnWindowSeconds)
	}
	// Fast window must be <= slow window so a fast burn is a strictly
	// shorter-term signal than a slow burn.
	if def.FastBurnWindowSeconds > def.SlowBurnWindowSeconds {
		return fmt.Errorf("fast_burn_window_seconds must be <= slow_burn_window_seconds")
	}
	if def.TemplateVersion == "" {
		return fmt.Errorf("template_version is required")
	}
	return nil
}

// ValidateCreate validates a CreateDefinitionInput before it is persisted.
func ValidateCreate(input CreateDefinitionInput) error {
	def := &Definition{
		ClusterID:             input.ClusterID,
		Service:               input.Service,
		Template:              input.Template,
		TemplateVersion:       TemplateVersion,
		Objective:             input.Objective,
		RollingWindowSeconds:  input.RollingWindowSeconds,
		MissingDataPolicy:     input.MissingDataPolicy,
		LatencyThresholdMs:    input.LatencyThresholdMs,
		Owner:                 input.Owner,
		FastBurnRate:          input.FastBurnRate,
		FastBurnWindowSeconds: input.FastBurnWindowSeconds,
		SlowBurnRate:          input.SlowBurnRate,
		SlowBurnWindowSeconds: input.SlowBurnWindowSeconds,
		Enabled:               input.Enabled,
	}
	if err := ValidateDefinition(def); err != nil {
		return err
	}
	if input.Creator.ID <= 0 {
		return fmt.Errorf("creator id is required")
	}
	if input.Owner.ID <= 0 {
		return fmt.Errorf("owner id is required")
	}
	return nil
}

// DefaultMissingDataPolicy returns the safe default policy for a template.
func DefaultMissingDataPolicy(t SLITemplate) MissingDataPolicy {
	desc, ok := LookupTemplate(t)
	if !ok {
		return MissingDataUnavailable
	}
	if desc.AllowsFailOpen {
		// Even for workload_readiness the default is "unavailable" —
		// fail_open must be an explicit operator choice.
		return MissingDataUnavailable
	}
	return MissingDataUnavailable
}
